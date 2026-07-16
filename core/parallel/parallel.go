// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package parallel defines interfaces for optional parallel block execution
// and GPU acceleration in the Lux EVM.
//
// No build tags required. GPU acceleration is auto-detected at init time:
//   - darwin + CGo: Metal GPU via gpu_bridge.go
//   - linux + CGo + CUDA: NVIDIA GPU (future)
//   - otherwise: CPU sequential (zero overhead)
//
// The registration pattern allows platform-specific init() functions
// to register GPU backends without import cycles.
package parallel

import (
	"fmt"
	"sync"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

var (
	mu          sync.RWMutex
	executor    BlockExecutor
	accelerator GPUAccelerator
	txExecutors map[EVMBackend]TransactionExecutor
	activeBack  EVMBackend = GoEVM
)

func init() {
	txExecutors = make(map[EVMBackend]TransactionExecutor)
}

// RegisterExecutor sets the parallel block executor.
//
// CONSENSUS-SAFETY INVARIANT: a registered BlockExecutor MUST apply every
// transaction's writes to the *state.StateDB it is passed, so that when
// ExecuteBlock returns the StateDB holds exactly the post-block state the
// serial path would produce — byte-identical state root and receipts. An
// executor that computes receipts on throwaway state copies without committing
// to the canonical StateDB (as BlockSTMExecutor currently does) would make the
// block's computed root diverge from every other node → a chain fork. No
// executor may be wired here as an execution path until a determinism test
// proves parallel-result == serial-result (state root + receipts). The default
// (nothing registered) is fallbackExecutor, which safely declines via (nil, nil).
func RegisterExecutor(e BlockExecutor) {
	mu.Lock()
	defer mu.Unlock()
	executor = e
}

// RegisterGPU sets the GPU accelerator.
func RegisterGPU(g GPUAccelerator) {
	mu.Lock()
	defer mu.Unlock()
	accelerator = g
}

// RegisterTransactionExecutor registers a per-tx executor for a given backend.
// Call from init() in backend packages (e.g., revmbackend, cevmbackend).
func RegisterTransactionExecutor(backend EVMBackend, e TransactionExecutor) {
	mu.Lock()
	defer mu.Unlock()
	txExecutors[backend] = e
}

// SetBackend selects the active EVM backend for per-transaction execution.
// Use AutoEVM to select the best available.
//
// It fails closed: selecting a modular backend (revm/cevm) that is not
// registered in this build returns an error instead of silently leaving the
// active backend unchanged (or set to a name with no executor, which would make
// DefaultTransactionExecutor return nil and silently run the Go EVM — a
// different EVM than the operator selected). GoEVM and AutoEVM always succeed
// (Auto falls back to GoEVM when no modular backend is wired). On error the
// active backend is left unchanged.
func SetBackend(backend EVMBackend) error {
	mu.Lock()
	defer mu.Unlock()
	switch backend {
	case AutoEVM:
		// Priority: CppEVM > RustEVM > GoEVM (first registered wins).
		for _, b := range []EVMBackend{CppEVM, RustEVM} {
			if _, ok := txExecutors[b]; ok {
				activeBack = b
				return nil
			}
		}
		activeBack = GoEVM
		return nil
	case GoEVM:
		activeBack = GoEVM
		return nil
	default:
		if _, ok := txExecutors[backend]; !ok {
			return fmt.Errorf("evm backend %q is not available in this build (available: %v); rebuild with -tags %s to wire it", backend, availableBackendsLocked(), backend)
		}
		activeBack = backend
		return nil
	}
}

// ActiveBackend returns the currently selected EVM backend.
func ActiveBackend() EVMBackend {
	mu.RLock()
	defer mu.RUnlock()
	return activeBack
}

// AvailableBackends returns all registered backend names.
func AvailableBackends() []EVMBackend {
	mu.RLock()
	defer mu.RUnlock()
	return availableBackendsLocked()
}

// availableBackendsLocked is the lock-free body of AvailableBackends; callers
// must already hold mu (read or write).
func availableBackendsLocked() []EVMBackend {
	backends := make([]EVMBackend, 0, len(txExecutors)+1)
	backends = append(backends, GoEVM) // always available
	for b := range txExecutors {
		if b != GoEVM {
			backends = append(backends, b)
		}
	}
	return backends
}

// CheckTxResult is the single decision point the state processor uses to consume
// the outcome of a per-transaction modular backend (revm/cevm). It fails closed:
//
//   - execErr != nil            → error (the selected backend failed the tx)
//   - receipt == nil, no error  → error: the selected backend produced no
//     receipt, i.e. it is not actually wired. The state processor MUST NOT
//     silently fall back to the Go EVM here — doing so would run a different EVM
//     than the operator selected. Silent degradation is the bug this guards.
//   - receipt != nil, no error  → nil (the caller uses the receipt)
//
// It is only reached when a non-Go backend is active (DefaultTransactionExecutor
// returned non-nil); the default Go EVM path never calls it, so unset builds are
// byte-identical.
func CheckTxResult(backend EVMBackend, receipt *types.Receipt, execErr error) error {
	if execErr != nil {
		return fmt.Errorf("evm backend %q failed to execute transaction: %w", backend, execErr)
	}
	if receipt == nil {
		return fmt.Errorf("evm backend %q is selected but produced no receipt (not wired in this build); refusing to silently fall back to the Go EVM — select %q or rebuild with the backend", backend, GoEVM)
	}
	return nil
}

// DefaultTransactionExecutor returns the tx executor for the active backend.
func DefaultTransactionExecutor() TransactionExecutor {
	mu.RLock()
	defer mu.RUnlock()
	if e, ok := txExecutors[activeBack]; ok {
		return e
	}
	return nil // GoEVM uses native geth path, no TransactionExecutor needed
}

// DefaultExecutor returns the registered parallel executor,
// or a no-op sequential fallback if none was registered.
func DefaultExecutor() BlockExecutor {
	mu.RLock()
	defer mu.RUnlock()
	if executor != nil {
		return executor
	}
	return fallbackExecutor{}
}

// DefaultGPU returns the registered GPU accelerator,
// or a no-op if none was registered.
func DefaultGPU() GPUAccelerator {
	mu.RLock()
	defer mu.RUnlock()
	if accelerator != nil {
		return accelerator
	}
	return fallbackGPU{}
}

// fallbackExecutor falls through to sequential execution.
type fallbackExecutor struct{}

func (fallbackExecutor) ExecuteBlock(
	_ *ethparams.ChainConfig,
	_ *types.Header,
	_ types.Transactions,
	_ *state.StateDB,
	_ vm.Config,
) ([]*types.Receipt, error) {
	return nil, nil
}

// fallbackGPU is the no-op GPU accelerator.
type fallbackGPU struct{}

func (fallbackGPU) Available() bool { return false }

func (fallbackGPU) BatchEcrecover(_ []*types.Transaction) (map[common.Hash]common.Address, error) {
	return nil, nil
}

func (fallbackGPU) BatchKeccak(_ [][]byte) ([]common.Hash, error) {
	return nil, nil
}

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

// SetBackend selects the active EVM backend.
// Use AutoEVM to select the best available.
func SetBackend(backend EVMBackend) {
	mu.Lock()
	defer mu.Unlock()
	if backend == AutoEVM {
		// Priority: CppEVM > RustEVM > GoEVM
		for _, b := range []EVMBackend{CppEVM, RustEVM, GoEVM} {
			if _, ok := txExecutors[b]; ok {
				activeBack = b
				return
			}
		}
		activeBack = GoEVM
	} else {
		activeBack = backend
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
	backends := make([]EVMBackend, 0, len(txExecutors)+1)
	backends = append(backends, GoEVM) // always available
	for b := range txExecutors {
		if b != GoEVM {
			backends = append(backends, b)
		}
	}
	return backends
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

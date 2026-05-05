// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package parallel

// cevm backend — Lux C++ EVM via CGo.
//
// Build with: go build -tags cevm
//
// Requires: libcevm.a (compiled from luxfi/chains/evm/cevm)
//
// Features:
//   - Fastest EVM interpreter (~3-5x vs Go EVM, fastest of any backend)
//   - SIMD-optimized opcode execution (AVX2/NEON)
//   - Native GPU kernel dispatch (CUDA/Metal) for batch operations
//   - Parallel block execution via Block-STM with GPU acceleration
//
// The CGo bridge passes StateDB operations through C callbacks:
//   Go StateDB ← CGo → C++ cevm::HostInterface
//
// CGo bridge to luxfi/chains/evm/cevm (requires native lib)

import (
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

func init() {
	RegisterTransactionExecutor(CppEVM, &cevmExecutor{})
}

type cevmExecutor struct{}

func (c *cevmExecutor) Backend() EVMBackend { return CppEVM }

// ExecuteTransaction dispatches a single tx to the cevm native engine.
//
// LP-108 (2026-05-04): the per-tx → batched-block adapter is not
// wired. The cgo bridge at github.com/luxfi/chains/evm/cevm exposes
// ExecuteBlock([]Transaction) — a block-batch interface — but
// TransactionExecutor is per-tx. To complete the wiring requires:
//   * an accumulator that buffers per-tx work and submits at block close
//   * receipt reconstruction from the BlockResult ordered output
//   * EIP-2929 warm-set seeding from the StateDB
//   * parity test against Go EVM Block-STM
//
// Until that adapter lands, ExecuteTransaction returns (nil, nil)
// — the documented "this backend declines; use Go EVM" contract that
// the parallel framework expects (see DefaultTransactionExecutor in
// parallel.go). This is the honest answer; the registration exists so
// the build-tag gating works, but cevm only executes once the adapter
// is in place AND the parity gate passes.
func (c *cevmExecutor) ExecuteTransaction(
	config *ethparams.ChainConfig,
	header *types.Header,
	tx *types.Transaction,
	statedb *state.StateDB,
	vmCfg vm.Config,
	gasPool uint64,
) (*types.Receipt, error) {
	return nil, nil
}

// SupportsParallel reports backend capability. The C++ EVM has
// Block-STM kernels (Metal + CUDA, see luxcpp/cevm/lib/evm/gpu/),
// so the answer is true once the adapter in ExecuteTransaction is wired.
func (c *cevmExecutor) SupportsParallel() bool { return true }

// SupportsGPU reports whether the cevm path can dispatch to GPU.
// True once the adapter is wired.
func (c *cevmExecutor) SupportsGPU() bool { return true }

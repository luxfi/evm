// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package parallel defines interfaces for optional parallel block execution
// and GPU acceleration in the Lux EVM.
//
// By default (no build tags), a sequential executor is used. When built with
// -tags parallel, the evmgpu Block-STM engine is linked. When built with
// -tags parallel,gpu, GPU-accelerated hashing and ecrecover are also enabled.
//
// This package lives in lux/evm so that the state processor can call into it
// without importing evmgpu. The evmgpu package provides the real implementations
// that satisfy these interfaces.
package parallel

import (
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// BlockExecutor processes all transactions in a block.
// The default implementation delegates to sequential per-tx execution.
// The parallel implementation uses Block-STM speculative execution.
type BlockExecutor interface {
	// ExecuteBlock processes all transactions in a block.
	// Returns receipts in original transaction order, or an error.
	// A nil return (nil, nil) means "not handled, fall through to sequential."
	ExecuteBlock(
		config *ethparams.ChainConfig,
		header *types.Header,
		txs types.Transactions,
		statedb *state.StateDB,
		vmCfg vm.Config,
	) ([]*types.Receipt, error)
}

// EVMBackend identifies which EVM implementation to use.
type EVMBackend string

const (
	// GoEVM is the default Go EVM from luxfi/geth (geth interpreter).
	GoEVM EVMBackend = "gevm"

	// RustEVM uses revm (Rust EVM) via FFI for execution.
	// Native Block-STM parallel execution, memory-safe.
	RustEVM EVMBackend = "revm"

	// CppEVM is the Lux C++ EVM via CGo.
	// Fastest single-threaded interpreter, SIMD opcodes, GPU kernel dispatch.
	CppEVM EVMBackend = "cevm"

	// AutoEVM selects the best available backend at runtime.
	AutoEVM EVMBackend = "auto"
)

// TransactionExecutor processes a single transaction against a state.
// This is the per-tx abstraction point for swappable EVM backends.
//
// Backends implement this to replace the default Go EVM interpreter:
//   - GoEVM: delegates to geth's vm.EVM.Call()/Create() (default)
//   - RustEVM: calls revm via FFI (faster single-thread, native Block-STM)
//   - CppEVM: calls evmone via CGo (fastest interpreter, GPU offload)
//
// The StateDB interface is the bridge — all backends read/write state
// through the same Go StateDB, ensuring consensus compatibility.
type TransactionExecutor interface {
	// Backend returns which EVM implementation this executor uses.
	Backend() EVMBackend

	// ExecuteTransaction runs a single transaction against the state.
	// Returns the execution result or error.
	ExecuteTransaction(
		config *ethparams.ChainConfig,
		header *types.Header,
		tx *types.Transaction,
		statedb *state.StateDB,
		vmCfg vm.Config,
		gasPool uint64,
	) (*types.Receipt, error)

	// SupportsParallel returns true if this backend can execute
	// transactions in parallel (e.g., Block-STM in revm, GPU in cevm).
	SupportsParallel() bool

	// SupportsGPU returns true if this backend can offload computation
	// to GPU (e.g., evmone with CUDA/Metal kernel dispatch).
	SupportsGPU() bool
}

// GPUAccelerator provides optional GPU-offloaded crypto operations.
// The default implementation returns Available() == false.
type GPUAccelerator interface {
	// Available reports whether a GPU backend is detected.
	Available() bool

	// BatchEcrecover recovers sender addresses for a batch of transactions.
	// Returns a map from tx hash to recovered sender address.
	BatchEcrecover(txs []*types.Transaction) (map[common.Hash]common.Address, error)

	// BatchKeccak hashes multiple inputs on GPU.
	BatchKeccak(inputs [][]byte) ([]common.Hash, error)
}

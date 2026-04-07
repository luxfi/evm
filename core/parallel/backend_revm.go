// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build revm

package parallel

// revm backend — Rust EVM via FFI.
//
// Build with: go build -tags revm
//
// Requires: librustc_revm.a (compiled from luxfi/revm)
//
// Features:
//   - Faster single-threaded execution than Go EVM (~2x)
//   - Native Block-STM parallel execution
//   - Rust memory safety guarantees
//
// The FFI bridge passes StateDB operations through cgo callbacks:
//   Go StateDB ← cgo → Rust revm::Database trait impl
//
// TODO: implement FFI bridge in luxfi/revm-ffi

import (
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

func init() {
	RegisterTransactionExecutor(RustEVM, &revmExecutor{})
}

type revmExecutor struct{}

func (r *revmExecutor) Backend() EVMBackend { return RustEVM }

func (r *revmExecutor) ExecuteTransaction(
	config *ethparams.ChainConfig,
	header *types.Header,
	tx *types.Transaction,
	statedb *state.StateDB,
	vmCfg vm.Config,
	gasPool uint64,
) (*types.Receipt, error) {
	// TODO: FFI call to revm
	// For now, return nil to fall through to Go EVM
	return nil, nil
}

func (r *revmExecutor) SupportsParallel() bool { return true }
func (r *revmExecutor) SupportsGPU() bool      { return false }

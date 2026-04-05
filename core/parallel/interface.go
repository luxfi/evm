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

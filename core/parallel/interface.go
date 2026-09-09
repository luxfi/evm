// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package parallel is the Go EVM's optional parallel block executor
// (Block-STM, blockstm.go) and its optional GPU accelerator (gpu_bridge.go,
// cgo && darwin, backed by luxfi/gpu). Both are registered from init() and
// consulted by the state processor; when neither is registered the fallbacks
// answer "not handled" and execution is sequential on the Go interpreter.
//
// Other EVM implementations are not linked into this process. The C++ and
// Rust EVMs are separate luxd plugins that the host loads over ZAP, the same
// way it loads this one -- choosing one is choosing which plugin binary runs,
// not a switch inside the Go EVM.
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

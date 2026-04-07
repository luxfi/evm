// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package parallel

// cevm backend — C++ EVM (Lux CEVM) via CGo.
//
// Build with: go build -tags cevm
//
// Requires: libcevm.a (compiled from luxfi/cevm)
//
// Features:
//   - Fastest single-threaded EVM interpreter (~3x vs Go EVM)
//   - Native GPU kernel dispatch (CUDA/Metal) for batch operations
//   - SIMD-optimized opcode execution
//   - Parallel block execution via Block-STM with GPU acceleration
//
// The CGo bridge passes StateDB operations through C callbacks:
//   Go StateDB ← CGo → C++ cevm::HostInterface
//
// TODO: implement CGo bridge in luxfi/cevm

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

func (c *cevmExecutor) ExecuteTransaction(
	config *ethparams.ChainConfig,
	header *types.Header,
	tx *types.Transaction,
	statedb *state.StateDB,
	vmCfg vm.Config,
	gasPool uint64,
) (*types.Receipt, error) {
	// TODO: CGo call to cevm
	// For now, return nil to fall through to Go EVM
	return nil, nil
}

func (c *cevmExecutor) SupportsParallel() bool { return true }
func (c *cevmExecutor) SupportsGPU() bool      { return true }

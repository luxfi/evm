// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !parallel

package parallel

import (
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// sequentialExecutor is the default no-op executor.
// It returns (nil, nil) to signal the caller to use the existing sequential path.
type sequentialExecutor struct{}

func (sequentialExecutor) ExecuteBlock(
	_ *ethparams.ChainConfig,
	_ *types.Header,
	_ types.Transactions,
	_ *state.StateDB,
	_ vm.Config,
) ([]*types.Receipt, error) {
	return nil, nil
}

// noGPU is the default no-op GPU accelerator.
type noGPU struct{}

func (noGPU) Available() bool { return false }

func (noGPU) BatchEcrecover(_ []*types.Transaction) (map[common.Hash]common.Address, error) {
	return nil, nil
}

func (noGPU) BatchKeccak(_ [][]byte) ([]common.Hash, error) {
	return nil, nil
}

// DefaultExecutor returns the block executor for this build.
// Without the "parallel" build tag, this is a no-op that always falls
// through to sequential execution.
func DefaultExecutor() BlockExecutor { return sequentialExecutor{} }

// DefaultGPU returns the GPU accelerator for this build.
// Without the "parallel" build tag, GPU is never available.
func DefaultGPU() GPUAccelerator { return noGPU{} }

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build parallel

// When built with -tags parallel, this file provides the real Block-STM
// executor and GPU accelerator by linking against github.com/luxfi/evmgpu.
//
// The evmgpu package must implement:
//   - parallel.RegisterExecutor(BlockExecutor)
//   - parallel.RegisterGPU(GPUAccelerator)
//
// via init() in its own bridge package, or this file can construct them
// directly. For now we use a registration pattern so evmgpu controls its
// own initialization.

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
)

// RegisterExecutor sets the parallel block executor.
// Called by evmgpu's init() when linked with -tags parallel.
func RegisterExecutor(e BlockExecutor) {
	mu.Lock()
	defer mu.Unlock()
	executor = e
}

// RegisterGPU sets the GPU accelerator.
// Called by evmgpu's init() when linked with -tags parallel,gpu.
func RegisterGPU(g GPUAccelerator) {
	mu.Lock()
	defer mu.Unlock()
	accelerator = g
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

// fallbackExecutor is used when -tags parallel is set but no executor
// was registered (e.g., evmgpu not linked). Falls through to sequential.
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

// fallbackGPU is used when -tags parallel is set but no GPU was registered.
type fallbackGPU struct{}

func (fallbackGPU) Available() bool { return false }

func (fallbackGPU) BatchEcrecover(_ []*types.Transaction) (map[common.Hash]common.Address, error) {
	return nil, nil
}

func (fallbackGPU) BatchKeccak(_ [][]byte) ([]common.Hash, error) {
	return nil, nil
}

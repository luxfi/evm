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
)

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

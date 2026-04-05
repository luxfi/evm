// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
)

// TestDefaultExecutorReturnsNil verifies the default executor signals
// "not handled" by returning (nil, nil), regardless of build tags.
func TestDefaultExecutorReturnsNil(t *testing.T) {
	exec := DefaultExecutor()
	receipts, err := exec.ExecuteBlock(nil, nil, nil, nil, vm.Config{})
	if err != nil {
		t.Fatalf("DefaultExecutor returned error: %v", err)
	}
	if receipts != nil {
		t.Fatalf("DefaultExecutor returned non-nil receipts: %v", receipts)
	}
}

// TestDefaultGPUNotAvailable verifies the default GPU reports unavailable.
func TestDefaultGPUNotAvailable(t *testing.T) {
	gpu := DefaultGPU()
	if gpu.Available() {
		t.Fatal("DefaultGPU should not be available")
	}
}

// TestDefaultGPUBatchEcrecoverNoop verifies batch ecrecover returns nil.
func TestDefaultGPUBatchEcrecoverNoop(t *testing.T) {
	gpu := DefaultGPU()
	result, err := gpu.BatchEcrecover([]*types.Transaction{types.NewTx(&types.LegacyTx{})})
	if err != nil {
		t.Fatalf("BatchEcrecover returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("BatchEcrecover returned non-nil result: %v", result)
	}
}

// TestDefaultGPUBatchKeccakNoop verifies batch keccak returns nil.
func TestDefaultGPUBatchKeccakNoop(t *testing.T) {
	gpu := DefaultGPU()
	result, err := gpu.BatchKeccak([][]byte{{0x01, 0x02}})
	if err != nil {
		t.Fatalf("BatchKeccak returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("BatchKeccak returned non-nil result: %v", result)
	}
}

// TestInterfaceCompliance verifies the default types satisfy the interfaces
// at runtime (works regardless of build tags).
func TestInterfaceCompliance(t *testing.T) {
	var _ BlockExecutor = DefaultExecutor()
	var _ GPUAccelerator = DefaultGPU()
}

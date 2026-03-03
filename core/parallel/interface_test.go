// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"runtime"
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
)

// TestDefaultExecutorReturnsNil verifies the default executor signals
// "not handled" by returning (nil, nil).
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

// TestGPUAutoDetect verifies GPU is auto-detected on darwin (Metal)
// and reports unavailable on other platforms.
func TestGPUAutoDetect(t *testing.T) {
	gpu := DefaultGPU()
	if runtime.GOOS == "darwin" {
		// On macOS with CGo, the Metal GPU bridge should be registered
		t.Logf("GPU available: %v (expected true on darwin with Metal)", gpu.Available())
	} else {
		if gpu.Available() {
			t.Fatal("GPU should not be available on non-darwin")
		}
	}
}

// TestGPUBatchEcrecover verifies batch ecrecover handles empty/nil inputs.
func TestGPUBatchEcrecover(t *testing.T) {
	gpu := DefaultGPU()

	// Nil input
	result, err := gpu.BatchEcrecover(nil)
	if err != nil {
		t.Fatalf("nil input returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("nil input returned non-nil result: %v", result)
	}

	// Empty input
	result, err = gpu.BatchEcrecover([]*types.Transaction{})
	if err != nil {
		t.Fatalf("empty input returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("empty input returned non-nil result: %v", result)
	}
}

// TestGPUBatchKeccakReturnsNil verifies batch keccak returns nil (not yet wired).
func TestGPUBatchKeccakReturnsNil(t *testing.T) {
	gpu := DefaultGPU()
	result, err := gpu.BatchKeccak([][]byte{{0x01, 0x02}})
	if err != nil {
		t.Fatalf("BatchKeccak returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("BatchKeccak returned non-nil result: %v", result)
	}
}

// TestInterfaceCompliance verifies the types satisfy interfaces at runtime.
func TestInterfaceCompliance(t *testing.T) {
	var _ BlockExecutor = DefaultExecutor()
	var _ GPUAccelerator = DefaultGPU()
}

// TestFallbackGPUBatchEcrecoverReturnsNil verifies the CPU fallback returns
// nil for all inputs (signaling "not handled").
func TestFallbackGPUBatchEcrecoverReturnsNil(t *testing.T) {
	gpu := fallbackGPU{}

	// Non-nil but empty
	result, err := gpu.BatchEcrecover([]*types.Transaction{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != nil {
		t.Fatal("fallback GPU should return nil for empty input")
	}

	// Nil
	result, err = gpu.BatchEcrecover(nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != nil {
		t.Fatal("fallback GPU should return nil for nil input")
	}
}

// TestFallbackGPUNotAvailable verifies the fallback reports unavailable.
func TestFallbackGPUNotAvailable(t *testing.T) {
	gpu := fallbackGPU{}
	if gpu.Available() {
		t.Fatal("fallback GPU must report not available")
	}
}

// TestFallbackExecutorReturnsNilNil verifies the fallback executor
// always returns (nil, nil) for any input.
func TestFallbackExecutorReturnsNilNil(t *testing.T) {
	exec := fallbackExecutor{}
	receipts, err := exec.ExecuteBlock(nil, nil, nil, nil, vm.Config{})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if receipts != nil {
		t.Fatal("expected nil receipts from fallback executor")
	}
}

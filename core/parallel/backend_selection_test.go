// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"testing"

	"github.com/luxfi/geth/core/types"
)

// TestCheckTxResultFailsClosed locks the fix for the silent-fall-through bug
// (LP-108): when a non-Go EVM backend is active, the state processor must NOT
// silently drop to the Go EVM when the backend declines or errors — it must fail
// the block loudly. This is a consensus-observability property: an operator who
// selected revm/cevm must never unknowingly run a different EVM.
//
// It is the per-tx complement to the parallel==serial state-root gate proven by
// blockstm_dex_determinism_test.go (Test9999_DisjointFillsParallelRootEqualsSequential
// and Test9999_ConflictingFillsReExecToSequentialRoot). No backend may become the
// default execution path until such a determinism test passes for it.
func TestCheckTxResultFailsClosed(t *testing.T) {
	receipt := &types.Receipt{GasUsed: 21000}

	tests := []struct {
		name    string
		backend EVMBackend
		receipt *types.Receipt
		execErr error
		wantErr bool
	}{
		{
			// The exact silent-degradation bug: a selected backend returns
			// (nil, nil) ("decline"). Must be a loud error, NOT a fall-back.
			name:    "active backend declines (nil,nil) is an error",
			backend: RustEVM,
			receipt: nil,
			execErr: nil,
			wantErr: true,
		},
		{
			// A backend that errors must surface the error (the old code only
			// inspected execErr when receipt != nil, so a (nil, err) was dropped).
			name:    "active backend error is surfaced",
			backend: CppEVM,
			receipt: nil,
			execErr: errStub,
			wantErr: true,
		},
		{
			// The happy path: a real receipt is consumed with no error.
			name:    "receipt is consumed",
			backend: RustEVM,
			receipt: receipt,
			execErr: nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckTxResult(tt.backend, tt.receipt, tt.execErr)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CheckTxResult(%s, %v, %v) err = %v, wantErr = %v",
					tt.backend, tt.receipt, tt.execErr, err, tt.wantErr)
			}
		})
	}
}

// TestSetBackendFailsClosed verifies backend selection is explicit and validated:
// selecting a modular backend that is not registered in this build returns an
// error and leaves the active backend unchanged (rather than pointing it at a
// name with no executor, which would silently run the Go EVM). GoEVM and AutoEVM
// always succeed. This closes the selection-time half of the silent-degradation
// bug (the execution-time half is TestCheckTxResultFailsClosed).
func TestSetBackendFailsClosed(t *testing.T) {
	// Restore the package default so we do not pollute other tests in this package.
	t.Cleanup(func() { _ = SetBackend(GoEVM) })

	// GoEVM always succeeds and is the default.
	if err := SetBackend(GoEVM); err != nil {
		t.Fatalf("SetBackend(GoEVM) = %v, want nil", err)
	}
	if got := ActiveBackend(); got != GoEVM {
		t.Fatalf("ActiveBackend() = %s, want %s", got, GoEVM)
	}

	// Auto with nothing registered falls back to GoEVM, no error.
	if err := SetBackend(AutoEVM); err != nil {
		t.Fatalf("SetBackend(AutoEVM) = %v, want nil", err)
	}
	if got := ActiveBackend(); got != GoEVM {
		t.Fatalf("ActiveBackend() after auto = %s, want %s", got, GoEVM)
	}

	// A modular backend that is not wired in this (pure-Go) build must be
	// rejected, and the active backend must be left unchanged.
	if err := SetBackend(RustEVM); err == nil {
		t.Fatal("SetBackend(RustEVM) = nil, want error (revm not registered in a pure-Go build)")
	}
	if got := ActiveBackend(); got != GoEVM {
		t.Fatalf("ActiveBackend() after rejected SetBackend = %s, want unchanged %s", got, GoEVM)
	}
	if err := SetBackend(CppEVM); err == nil {
		t.Fatal("SetBackend(CppEVM) = nil, want error (cevm not registered in a pure-Go build)")
	}
}

// errStub is a sentinel execution error for the fail-closed table above.
var errStub = stubError("backend exec failed")

type stubError string

func (e stubError) Error() string { return string(e) }

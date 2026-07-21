// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !cevm

package cevmbridge

// Enabled reports whether the real C++ EVM bridge is linked. It is false in the
// default build; build with -tags cevm to link the cevm libs and get the real
// process_block path.
const Enabled = false

// ABIVersion returns 0 when the bridge is not linked.
func ABIVersion() uint32 { return 0 }

// ProcessBlock is a no-op in the non-cevm build: it always declines with
// ErrDisabled so callers fall through to the Go EVM.
func ProcessBlock(_ []Account, _ []Storage, _ []Tx, _ BlockCtx) (Result, error) {
	return Result{}, ErrDisabled
}

// --- Resident (stateful) handle API — no-op stubs ------------------------
// Handle 0 is the "no resident state" sentinel; the applier treats a zero
// handle as never-created and simply declines every block to the Go EVM.

// StateCreate returns 0 (no resident StateDB in the non-cevm build).
func StateCreate() uint64 { return 0 }

// StateSeed is a no-op in the non-cevm build.
func StateSeed(_ uint64, _ []Account, _ []Storage) {}

// StateApplyBlock always declines with ErrDisabled in the non-cevm build.
func StateApplyBlock(_ uint64, _ []Tx, _ BlockCtx) (Result, error) {
	return Result{}, ErrDisabled
}

// StateRoot returns the zero root in the non-cevm build.
func StateRoot(_ uint64) [32]byte { return [32]byte{} }

// StateFree is a no-op in the non-cevm build.
func StateFree(_ uint64) {}

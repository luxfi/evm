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

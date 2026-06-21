// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
)

// precompile_statedb_subbalance_test.go pins the fix for a LATENT bug: the
// precompile registry's stateDBBridge.SubBalance type-asserts the internal
// contract.StateDB for the EXTERNAL SubBalance signature
//
//	SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) uint256.Int
//
// and previously fell back to a NO-OP when the assertion failed. The concrete
// adapter the EVM hands precompiles is *stateDBAdapter; if it does not implement
// that EXACT signature, a precompile that debits native value — e.g. the DEX 0x9999
// custody vault, whose withdraw debits the vault before releasing to the caller —
// would silently MINT (caller credited, vault never debited). The fix added
// SubBalance to *stateDBAdapter so the bridge's assertion succeeds and the fallback
// is unreachable; the fallback itself now FAILS CLOSED (panics → reverted call)
// rather than returning a zero "previous balance" without debiting.

// subBalancer is the EXACT interface precompile/registry/bridge.go type-asserts.
// Keep it byte-identical so this test fails if the bridge's expected signature or
// the adapter's method ever drifts.
type subBalancer interface {
	SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) uint256.Int
}

// TestStateDBAdapter_SatisfiesSubBalancer is the regression guard: *stateDBAdapter
// MUST satisfy the subBalancer interface the registry bridge type-asserts. If this
// fails to compile or asserts false, the bridge would hit its mint fallback for
// every precompile native-value debit (the DEX withdraw vault-release).
func TestStateDBAdapter_SatisfiesSubBalancer(t *testing.T) {
	// Compile-time assertion (the primary guard against signature drift).
	var _ subBalancer = (*stateDBAdapter)(nil)

	// Runtime assertion mirroring exactly what the bridge does:
	//   if sb, ok := s.internal.(subBalancer); ok { return sb.SubBalance(...) }
	var adapter contractStateDBSurface = (*stateDBAdapter)(nil)
	if _, ok := interface{}(adapter).(subBalancer); !ok {
		t.Fatal("*stateDBAdapter does NOT satisfy subBalancer — the registry bridge would hit the mint fallback for native-value debits")
	}
	// A non-nil *uint256.Int sanity reference (keeps the import meaningful and the
	// signature exercised in source).
	_ = uint256.NewInt(0)
}

// contractStateDBSurface is the minimal surface we hold the adapter as for the
// runtime assertion — it is satisfied by *stateDBAdapter (which implements the
// full internal contract.StateDB). Held as an interface value so the type
// assertion to subBalancer is a genuine runtime check, not a compile-time tautology.
type contractStateDBSurface interface {
	GetBalance(common.Address) *uint256.Int
	AddBalance(common.Address, *uint256.Int)
}

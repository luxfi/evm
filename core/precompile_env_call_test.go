// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"testing"

	"github.com/luxfi/geth/core/vm"
)

// precompile_env_call_test.go pins the PRODUCER side of the ERC-20 settlement env
// wiring: the EVM hands a precompile its AccessibleState as *accessibleStateAdapter,
// which captures the geth vm.PrecompileEnvironment (the only object with a Call
// surface). The registry bridge (precompile/registry/bridge.go) type-asserts the
// concrete envProvider accessor below to forward that Call to the external DEX 0x9999
// precompile so its ERC-20 leg (transferFrom / transfer / balanceOf on the token
// contract) executes on-chain. If *accessibleStateAdapter ever stops exposing the env,
// GetPrecompileEnv() reverts to nil and ERC-20 settlement silently refuses with
// ErrERC20VaultUnavailable — so this guard fails loudly on that drift.

// envProvider is the EXACT accessor precompile/registry/bridge.go type-asserts. Keep
// it byte-identical so this test fails if either side's signature drifts.
type envProvider interface {
	GetPrecompileEnv() vm.PrecompileEnvironment
}

// TestAccessibleStateAdapter_ExposesPrecompileEnv is the regression guard: the
// concrete *accessibleStateAdapter the EVM gives precompiles MUST satisfy envProvider
// and MUST return the same non-nil geth env it was constructed with. If it returns nil
// or fails the assertion, the registry bridge cannot forward Call and ERC-20
// settlement is dead on-chain.
func TestAccessibleStateAdapter_ExposesPrecompileEnv(t *testing.T) {
	// Compile-time assertion (primary guard against signature drift).
	var _ envProvider = (*accessibleStateAdapter)(nil)

	// Runtime: construct the adapter exactly as RunStateful does and confirm it hands
	// back the SAME env. A nil sentinel env is sufficient — the load-bearing fact is
	// the accessor returns what it captured, not nil.
	env := &sentinelPrecompileEnv{}
	a := &accessibleStateAdapter{env: env}

	var asProvider envProvider = a
	got := asProvider.GetPrecompileEnv()
	if got == nil {
		t.Fatal("GetPrecompileEnv() returned nil — registry bridge cannot forward Call; ERC-20 settlement would refuse")
	}
	if got != vm.PrecompileEnvironment(env) {
		t.Fatal("GetPrecompileEnv() did not return the captured env")
	}
}

// sentinelPrecompileEnv is a do-nothing vm.PrecompileEnvironment used only to prove
// the adapter returns the env it was given. Its methods are never invoked by this
// test (identity check only), so they panic if ever called — keeping the sentinel
// honest about being a marker, not a behavioural mock.
type sentinelPrecompileEnv struct {
	vm.PrecompileEnvironment // embed the interface; all methods panic via the nil embedding if called
}

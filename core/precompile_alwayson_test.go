// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/precompile/modules"
	// registry side-effect: bridges the external DEX settlement module (0x9999)
	// into the EVM's internal registry so it is visible as AlwaysOn here.
	_ "github.com/luxfi/evm/precompile/registry"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/stretchr/testify/require"
)

// precompile_alwayson_test.go pins the activation of the DEX settlement money path
// 0x9999. It is a SYSTEM precompile activated by a single canonical dated fork —
// extras.DexSettleActivationTime (Dec 25 2025) — with NO per-net config (no
// dexSettleConfig genesis precompile, no per-net precompileUpgrades timestamp). The
// activation is timestamp-gated, and has TWO coupled effects, both pinned here:
//
//   - DISPATCH: at/after the fork, 0x9999 is in the enabled precompile set so the EVM
//     runs it; before the fork it is absent (so replaying pre-activation history sees a
//     plain account and stays byte-identical to canonical state).
//   - MARKER: on the block transition that crosses the fork, the precompile-activation
//     marker (nonce=1 + non-empty code) is written into 0x9999, installed FORWARD — never
//     in historical genesis (which would change the genesis hash and fork pre-activation
//     sync). A fresh net whose genesis ts >= the fork gets the marker at block 0.
//
// If activation regresses to unconditional, replay of a pre-Dec-25 value transfer to
// 0x9999 would dispatch the precompile instead of crediting a plain account — diverging
// from canonical history. If it regresses to dispatch-only (no marker), a typed Solidity
// call would fail the compiler's contract-existence guard (EXTCODESIZE==0).

var settleAddr9999 = common.HexToAddress("0x0000000000000000000000000000000000009999")

// A timestamp safely before the Dec 25 2025 fork, and one safely at/after it.
const (
	tsPreActivation  uint64 = extras.DexSettleActivationTime - 1
	tsAtActivation   uint64 = extras.DexSettleActivationTime
	tsPostActivation uint64 = extras.DexSettleActivationTime + 86_400
)

// TestAlwaysOn_9999_Registered asserts the DEX settlement precompile bridges into the
// EVM registry as AlwaysOn and is enumerated by AlwaysOnModules().
func TestAlwaysOn_9999_Registered(t *testing.T) {
	m, ok := modules.GetPrecompileModuleByAddress(settleAddr9999)
	require.True(t, ok, "0x9999 must be registered in the EVM precompile registry")
	require.True(t, m.AlwaysOn, "0x9999 (%s) must be AlwaysOn — it is the system money-path precompile", m.ConfigKey)

	found := false
	for _, am := range modules.AlwaysOnModules() {
		if am.Address == settleAddr9999 {
			found = true
		}
	}
	require.True(t, found, "0x9999 must be enumerated by modules.AlwaysOnModules()")
}

// TestDexSettle_DispatchGate_TimestampGated asserts 0x9999 is in the active precompile
// set ONLY at/after the Dec 25 2025 fork, on a chain whose config carries NO precompile
// activation whatsoever (no genesis precompiles, no upgrades). This is the dispatch gate
// PrecompileOverride consults. Pre-fork: absent (replay safety). At/after: present.
func TestDexSettle_DispatchGate_TimestampGated(t *testing.T) {
	// A bare chain config — nothing activates 0x9999 via config; only the built-in fork can.
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	// Pre-activation: 0x9999 must NOT be dispatch-enabled — a pre-Dec-25 tx-to-0x9999
	// must hit a plain account, so RLP replay of pre-activation history stays canonical.
	rulesPre := params.GetExtrasRules(params.Rules{}, cfg, tsPreActivation)
	_, okPre := rulesPre.Precompiles[settleAddr9999]
	require.False(t, okPre,
		"0x9999 must NOT be in the enabled set before the Dec 25 2025 fork — replay safety")

	// At the fork timestamp: enabled.
	rulesAt := params.GetExtrasRules(params.Rules{}, cfg, tsAtActivation)
	_, okAt := rulesAt.Precompiles[settleAddr9999]
	require.True(t, okAt, "0x9999 must be in the enabled set at the Dec 25 2025 fork timestamp")

	// After the fork: still enabled (forks are monotonic).
	rulesPost := params.GetExtrasRules(params.Rules{}, cfg, tsPostActivation)
	_, okPost := rulesPost.Precompiles[settleAddr9999]
	require.True(t, okPost, "0x9999 must stay in the enabled set at every timestamp at/after the fork")
}

// TestDexSettle_NoGenesisMarker_ExistingNet asserts that for an EXISTING network (whose
// genesis timestamp is before the Dec 25 2025 fork), ApplyPrecompileActivations at
// genesis writes NO marker to 0x9999 — the genesis state root is consensus-critical and
// must be byte-identical to the committed genesis so pre-activation history replays
// canonically. (parent==nil, blockTs < fork ⇒ no transition.)
func TestDexSettle_NoGenesisMarker_ExistingNet(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	require.NoError(t, err)

	cfg := &params.ChainConfig{}
	// Existing-net genesis: timestamp well before the fork (block-0 ts 0).
	blockCtx := NewBlockContext(big.NewInt(0), 0)
	require.NoError(t, ApplyPrecompileActivations(cfg, nil, blockCtx, statedb))

	require.Zero(t, statedb.GetNonce(settleAddr9999),
		"0x9999 must NOT get a genesis nonce on an existing net — genesis hash is immutable")
	require.Empty(t, statedb.GetCode(settleAddr9999),
		"0x9999 must NOT get genesis code on an existing net — genesis hash is immutable")
}

// TestDexSettle_MarkerInstalledAtForkTransition asserts the marker (nonce=1 + non-empty
// code) IS written when a block transition crosses the Dec 25 2025 fork. This is the
// forward state upgrade: EXTCODESIZE>0 / eth_getCode!=0x / Solidity contract-existence
// guard passes from the activation block onward.
func TestDexSettle_MarkerInstalledAtForkTransition(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	require.NoError(t, err)

	cfg := &params.ChainConfig{}
	parent := tsPreActivation
	// Block at the fork: parentTs < fork <= blockTs ⇒ transition fires, marker installed.
	blockCtx := NewBlockContext(big.NewInt(1), tsAtActivation)
	require.NoError(t, ApplyPrecompileActivations(cfg, &parent, blockCtx, statedb))

	require.Equal(t, uint64(1), statedb.GetNonce(settleAddr9999),
		"0x9999 must get the activation nonce at the fork transition")
	require.NotEmpty(t, statedb.GetCode(settleAddr9999),
		"0x9999 must get non-empty marker code at the fork transition (EXTCODESIZE>0)")
	require.Greater(t, statedb.GetCodeSize(settleAddr9999), 0,
		"EXTCODESIZE(0x9999) must be > 0 after activation — Solidity contract-existence guard")
}

// TestDexSettle_ForkTransitionFiresOnce asserts the transition fires exactly once: at the
// boundary, at a fresh-net genesis whose ts >= the fork, and NOT on later blocks nor at
// an existing-net pre-fork genesis.
func TestDexSettle_ForkTransitionFiresOnce(t *testing.T) {
	require.True(t, params.IsDexSettleForkTransition(ptr(tsPreActivation), tsAtActivation),
		"the 0x9999 fork transition must fire on the block that crosses the boundary")
	require.False(t, params.IsDexSettleForkTransition(ptr(tsAtActivation), tsPostActivation),
		"the 0x9999 fork transition must fire exactly once — not on later blocks")
	require.True(t, params.IsDexSettleForkTransition(nil, tsAtActivation),
		"a fresh net whose genesis ts >= the fork must install the marker at block 0")
	require.False(t, params.IsDexSettleForkTransition(nil, tsPreActivation),
		"an existing net whose genesis ts < the fork must NOT install the marker at genesis")
}

func ptr(v uint64) *uint64 { return &v }

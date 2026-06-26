// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"sync"
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
// 0x9999. It is a FIRST-RUN, no-legacy system precompile: ACTIVE FROM GENESIS on every
// Lux chain, with NO dated fork, NO activation timestamp, and NO per-net config (no
// dexSettleConfig genesis precompile, no precompileUpgrades entry). There is no
// pre-activation history to protect, so there is no "before" case and no plain-account
// branch. The activation has TWO coupled effects, both pinned here:
//
//   - DISPATCH: 0x9999 is in the enabled precompile set at EVERY timestamp (genesis
//     included), so the EVM runs it from block 0. It is injected unconditionally from
//     modules.AlwaysOnModules() — never gated on a timestamp.
//   - MARKER: the precompile-activation marker (nonce=1 + non-empty code) is written into
//     0x9999 exactly ONCE, at genesis (the parent==nil transition in
//     ApplyPrecompileActivations), so it lands in the committed genesis state root and
//     EXTCODESIZE>0 / eth_getCode!=0x / Solidity's contract-existence guard pass from
//     block 0. Every later block (parent!=nil) leaves it untouched.
//
// If dispatch regresses to timestamp-gated, a genesis-era call to 0x9999 would miss the
// settlement precompile. If the marker regresses to a forward (post-genesis) install, a
// typed Solidity call at genesis would fail the compiler's contract-existence guard
// (EXTCODESIZE==0).

var settleAddr9999 = common.HexToAddress("0x0000000000000000000000000000000000009999")

// A representative spread of block timestamps: genesis (0), small, a realistic
// wall-clock value, and the max. 0x9999 must be enabled at ALL of them.
var dispatchTimestamps = []uint64{0, 1, 1_700_000_000, ^uint64(0)}

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

// TestDexSettle_DispatchEnabledFromGenesis asserts 0x9999 is in the active precompile set
// at EVERY timestamp — genesis (ts=0) included — on a chain whose config carries NO
// precompile activation whatsoever (no genesis precompiles, no upgrades). This is the
// dispatch set PrecompileOverride consults. There is no "before" case: AlwaysOn means
// present from block 0, unconditionally.
func TestDexSettle_DispatchEnabledFromGenesis(t *testing.T) {
	// A bare chain config — nothing activates 0x9999 via config; the AlwaysOn mechanism does.
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	for _, ts := range dispatchTimestamps {
		rules := params.GetExtrasRules(params.Rules{}, cfg, ts)
		_, ok := rules.Precompiles[settleAddr9999]
		require.Truef(t, ok, "0x9999 must be in the enabled set at ts=%d — AlwaysOn from genesis", ts)
	}
}

// TestDexSettle_MarkerInstalledAtGenesis asserts the marker (nonce=1 + non-empty code) IS
// written at genesis (parent==nil) — the exact call core/genesis.go makes via
// ApplyPrecompileActivations(g.Config, nil, ...) before committing the genesis root. So
// EXTCODESIZE>0 / eth_getCode!=0x / Solidity contract-existence guard pass from block 0.
func TestDexSettle_MarkerInstalledAtGenesis(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	require.NoError(t, err)

	cfg := &params.ChainConfig{}
	// Genesis: parent==nil, mirroring core/genesis.go.
	blockCtx := NewBlockContext(big.NewInt(0), 0)
	require.NoError(t, ApplyPrecompileActivations(cfg, nil, blockCtx, statedb))

	require.Equal(t, uint64(1), statedb.GetNonce(settleAddr9999),
		"0x9999 must carry the activation nonce from genesis (parent==nil)")
	require.NotEmpty(t, statedb.GetCode(settleAddr9999),
		"0x9999 must carry non-empty marker code from genesis (EXTCODESIZE>0)")
	require.Greater(t, statedb.GetCodeSize(settleAddr9999), 0,
		"EXTCODESIZE(0x9999) must be > 0 from genesis — Solidity contract-existence guard")
}

// TestDexSettle_MarkerOnlyAtGenesis asserts the marker is installed EXACTLY ONCE, at
// genesis: a non-genesis block (parent!=nil) must NOT write it. In production the marker
// is already in the committed genesis state; every later block leaves 0x9999 untouched.
// (If this regressed to writing every block, 0x9999 would be needlessly re-dirtied in the
// journal on every block.)
func TestDexSettle_MarkerOnlyAtGenesis(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	require.NoError(t, err)

	cfg := &params.ChainConfig{}
	// A non-genesis block: parent != nil ⇒ no marker write on this fresh state.
	parent := uint64(0)
	blockCtx := NewBlockContext(big.NewInt(1), 1)
	require.NoError(t, ApplyPrecompileActivations(cfg, &parent, blockCtx, statedb))

	require.Zero(t, statedb.GetNonce(settleAddr9999),
		"0x9999 marker must be written only at genesis (parent==nil), never on a later block")
	require.Empty(t, statedb.GetCode(settleAddr9999),
		"0x9999 marker code must be written only at genesis, never on a later block")
}

// TestDexSettle_GenesisBuilders_Skip9999 is the AUTHORITATIVE INFO1 regression: with the
// registry bridge imported (so 0x9999 is a live AlwaysOn module in the EVM registry), the
// genesis-config builders must NOT emit a timestamp-0 genesis config for 0x9999. Writing
// one would create a SECOND, conflicting activation path (the module's Configurator would
// run for a system precompile that has no activating config) alongside the AlwaysOn
// mechanism. One and only one activation path. (The extras-package guard test runs without
// the registry import, so 0x9999 is invisible there; THIS test sees it.)
func TestDexSettle_GenesisBuilders_Skip9999(t *testing.T) {
	// Precondition: 0x9999 really is registered as AlwaysOn in this package's module view.
	m, ok := modules.GetPrecompileModuleByAddress(settleAddr9999)
	require.True(t, ok)
	require.True(t, m.AlwaysOn)

	fromFunc := extras.AllGenesisPrecompiles()
	_, in := fromFunc[m.ConfigKey]
	require.Falsef(t, in,
		"AllGenesisPrecompiles must SKIP the AlwaysOn 0x9999 module (%s) — it is activated by the "+
			"AlwaysOn mechanism (unconditional dispatch + genesis marker), not a genesis config", m.ConfigKey)

	var cfg extras.ChainConfig
	cfg.SetAllGenesisPrecompiles()
	_, inMethod := cfg.GenesisPrecompiles[m.ConfigKey]
	require.Falsef(t, inMethod,
		"SetAllGenesisPrecompiles must SKIP the AlwaysOn 0x9999 module (%s)", m.ConfigKey)
}

// TestDexSettle_PrecompileOverride_PresentUnconditional asserts the dispatch gate
// (LuxPrecompileOverrider.PrecompileOverride) returns the wrapped 0x9999 settlement
// contract at EVERY timestamp — genesis included — and is immune to the process-global
// lastRulesContext by construction (0x9999 is AlwaysOn, so its presence cannot depend on
// any timestamp). We POISON the global with an arbitrary timestamp first; the override
// must still answer PRESENT from its own field at every timestamp.
func TestDexSettle_PrecompileOverride_PresentUnconditional(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	// Poison the global — a correct AlwaysOn override ignores it.
	params.SetRulesContext(&params.Rules{}, cfg, 1_700_000_000)

	for _, ts := range dispatchTimestamps {
		o := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: ts}
		c, ok := o.PrecompileOverride(settleAddr9999)
		require.Truef(t, ok, "0x9999 must dispatch at ts=%d — AlwaysOn from genesis", ts)
		require.NotNilf(t, c, "0x9999 override must return the wrapped settlement contract at ts=%d", ts)
	}
}

// TestDexSettle_PrecompileOverride_NoGlobalRace is the -race regression for the AlwaysOn
// model. It reproduces the relaunch scenario: a verify goroutine replaying a block
// concurrently with a post-genesis eth_call/estimateGas/worker goroutine that rewrites the
// last-writer-wins global timestamp. Because 0x9999 is AlwaysOn, BOTH overriders MUST
// deterministically see it PRESENT on every iteration — the timestamp gate that once made
// this racy is gone, so presence cannot depend on who wrote the global last.
//
// Run with: go test -race -run NoGlobalRace ./core/
func TestDexSettle_PrecompileOverride_NoGlobalRace(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	a := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: 0}              // genesis replay
	b := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: 1_700_000_000} // later block / eth_call

	const iters = 2000
	done := make(chan struct{})

	// Adversary: continuously clobber the global with alternating timestamps, mimicking
	// concurrent eth_call (wall-clock) and any block-replay rules construction. This is
	// precisely what SetRulesContext does on every Rules()/RulesAt() across goroutines.
	go func() {
		r := &params.Rules{}
		for {
			select {
			case <-done:
				return
			default:
				params.SetRulesContext(r, cfg, 1_700_000_000)
				params.SetRulesContext(r, cfg, 0)
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)
	for _, o := range []*LuxPrecompileOverrider{a, b} {
		o := o
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				if _, ok := o.PrecompileOverride(settleAddr9999); !ok {
					t.Errorf("0x9999 override saw ABSENT at iter %d — AlwaysOn must be present on "+
						"every replay regardless of the concurrent global timestamp", i)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(done)
}

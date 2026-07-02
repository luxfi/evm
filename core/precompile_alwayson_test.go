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
	"github.com/luxfi/evm/precompile/registry"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/stretchr/testify/require"
)

// precompile_alwayson_test.go pins the activation contract of the DEX settlement money
// path 0x9999. It is a SYSTEM precompile activated by the Lux PROTOCOL on every chain
// with NO per-net config (no dexSettleConfig genesis precompile, no precompileUpgrades
// entry) — but NOT from genesis. It turns on at the protocol constant
// registry.DexSettleActivationTime (2025-12-25), exactly like a dated fork, so a chain
// whose genesis predates it reproduces its ORIGINAL genesis (see
// genesis_dexsettle_reproduce_test.go). The activation couples two effects, both pinned
// here and both keyed on the SAME timestamp so they can never disagree:
//
//   - DISPATCH: 0x9999 enters the enabled precompile set at timestamps >=
//     DexSettleActivationTime, and is ABSENT before it. This is the set
//     PrecompileOverride consults; the decision is a pure function of the passed-in
//     timestamp (no process-global read), so it is race-safe and identical on every
//     replay of a given block.
//   - MARKER: the EXTCODESIZE marker (nonce=1 + non-empty code) is written into 0x9999
//     exactly ONCE, on the block transition that crosses DexSettleActivationTime (which
//     is genesis iff the genesis timestamp already reaches it). So EXTCODESIZE>0 /
//     eth_getCode!=0x / Solidity's contract-existence guard pass from the activation
//     block forward, and NOT before.

var settleAddr9999 = common.HexToAddress("0x0000000000000000000000000000000000009999")

// addr9999 is the shared handle used by the genesis-reproduction regression in the
// sibling file genesis_dexsettle_reproduce_test.go.
var addr9999 = settleAddr9999

// activation is the protocol timestamp at which 0x9999 turns on (marker + dispatch).
var activation = registry.DexSettleActivationTime

// TestAlwaysOn_9999_Registered asserts the DEX settlement precompile bridges into the
// EVM registry as AlwaysOn, is enumerated by AlwaysOnModules(), and carries the protocol
// ActivationTime (Dec 2025) — the value the whole activation contract keys on.
func TestAlwaysOn_9999_Registered(t *testing.T) {
	m, ok := modules.GetPrecompileModuleByAddress(settleAddr9999)
	require.True(t, ok, "0x9999 must be registered in the EVM precompile registry")
	require.True(t, m.AlwaysOn, "0x9999 (%s) must be AlwaysOn — it is the system money-path precompile", m.ConfigKey)
	require.Equal(t, activation, m.ActivationTime,
		"0x9999 must carry the DEX settlement protocol ActivationTime, not activate at genesis")
	require.NotZero(t, activation, "the DEX settlement activation must be a real (post-genesis) timestamp")

	found := false
	for _, am := range modules.AlwaysOnModules() {
		if am.Address == settleAddr9999 {
			found = true
		}
	}
	require.True(t, found, "0x9999 must be enumerated by modules.AlwaysOnModules()")
}

// TestDexSettle_DispatchGatedByActivation asserts 0x9999 is ABSENT from the enabled
// precompile set before its activation timestamp and PRESENT at/after it, on a chain
// whose config carries NO precompile activation whatsoever (the AlwaysOn mechanism, not
// a genesis config, drives this).
func TestDexSettle_DispatchGatedByActivation(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	absentAt := []uint64{0, 1, activation - 1}
	for _, ts := range absentAt {
		rules := params.GetExtrasRules(params.Rules{}, cfg, ts)
		_, ok := rules.Precompiles[settleAddr9999]
		require.Falsef(t, ok, "0x9999 must be ABSENT at ts=%d (< activation %d)", ts, activation)
	}
	presentAt := []uint64{activation, activation + 1, ^uint64(0)}
	for _, ts := range presentAt {
		rules := params.GetExtrasRules(params.Rules{}, cfg, ts)
		_, ok := rules.Precompiles[settleAddr9999]
		require.Truef(t, ok, "0x9999 must be PRESENT at ts=%d (>= activation %d)", ts, activation)
	}
}

// TestDexSettle_MarkerAtActivationCrossing asserts the EXTCODESIZE marker is installed
// exactly on the transition that crosses the activation timestamp, and nowhere else.
func TestDexSettle_MarkerAtActivationCrossing(t *testing.T) {
	newState := func() *state.StateDB {
		db := rawdb.NewMemoryDatabase()
		sdb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
		require.NoError(t, err)
		return sdb
	}
	cfg := &params.ChainConfig{}
	hasMarker := func(sdb *state.StateDB) bool {
		return sdb.GetNonce(settleAddr9999) == 1 && len(sdb.GetCode(settleAddr9999)) > 0
	}

	// Pre-activation genesis (parent==nil, ts < activation): NO marker — preserves the
	// original genesis hash of every pre-2025 chain.
	sdb := newState()
	require.NoError(t, ApplyPrecompileActivations(cfg, nil, NewBlockContext(big.NewInt(0), activation-1), sdb))
	require.False(t, hasMarker(sdb), "no marker at a pre-activation genesis")

	// Post-activation genesis (parent==nil, ts >= activation): marker present (a fresh
	// chain that wants 0x9999 at genesis).
	sdb = newState()
	require.NoError(t, ApplyPrecompileActivations(cfg, nil, NewBlockContext(big.NewInt(0), activation), sdb))
	require.True(t, hasMarker(sdb), "marker at a post-activation genesis")

	// Pre-activation block during replay (parent < block < activation): still no marker.
	sdb = newState()
	parent := activation - 100
	require.NoError(t, ApplyPrecompileActivations(cfg, &parent, NewBlockContext(big.NewInt(10), activation-1), sdb))
	require.False(t, hasMarker(sdb), "no marker before the crossing block")

	// The crossing block (parent < activation <= block): marker installed here.
	sdb = newState()
	parent = activation - 1
	require.NoError(t, ApplyPrecompileActivations(cfg, &parent, NewBlockContext(big.NewInt(11), activation), sdb))
	require.True(t, hasMarker(sdb), "marker installed on the activation-crossing block")

	// A later block wholly after activation (parent >= activation): this transition does
	// NOT re-write the marker (in production it is already in committed state).
	sdb = newState()
	parent = activation
	require.NoError(t, ApplyPrecompileActivations(cfg, &parent, NewBlockContext(big.NewInt(12), activation+100), sdb))
	require.False(t, hasMarker(sdb), "the marker is written only on the crossing transition, not re-written after")
}

// TestDexSettle_GenesisBuilders_Skip9999 is the AUTHORITATIVE INFO1 regression: with the
// registry bridge imported (so 0x9999 is a live AlwaysOn module), the genesis-config
// builders must NOT emit a genesis config for 0x9999. Its activation is the AlwaysOn
// protocol mechanism (timestamp-gated dispatch + marker), never a genesis config — one
// and only one activation path.
func TestDexSettle_GenesisBuilders_Skip9999(t *testing.T) {
	m, ok := modules.GetPrecompileModuleByAddress(settleAddr9999)
	require.True(t, ok)
	require.True(t, m.AlwaysOn)

	fromFunc := extras.AllGenesisPrecompiles()
	_, in := fromFunc[m.ConfigKey]
	require.Falsef(t, in,
		"AllGenesisPrecompiles must SKIP the AlwaysOn 0x9999 module (%s)", m.ConfigKey)

	var cfg extras.ChainConfig
	cfg.SetAllGenesisPrecompiles()
	_, inMethod := cfg.GenesisPrecompiles[m.ConfigKey]
	require.Falsef(t, inMethod,
		"SetAllGenesisPrecompiles must SKIP the AlwaysOn 0x9999 module (%s)", m.ConfigKey)
}

// TestDexSettle_PrecompileOverride_GatedByActivation asserts the dispatch gate
// (LuxPrecompileOverrider.PrecompileOverride) returns the wrapped 0x9999 settlement
// contract at/after activation and ABSENT before it, and is immune to the process-global
// lastRulesContext by construction (the decision is bound to the overrider's OWN
// timestamp field). We POISON the global first; the override still answers from its field.
func TestDexSettle_PrecompileOverride_GatedByActivation(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})
	// Poison the global with a post-activation timestamp — a pre-activation override must
	// still answer ABSENT from its own field.
	params.SetRulesContext(&params.Rules{}, cfg, activation+1_000_000)

	for _, ts := range []uint64{0, 1, activation - 1} {
		o := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: ts}
		_, ok := o.PrecompileOverride(settleAddr9999)
		require.Falsef(t, ok, "0x9999 must NOT dispatch at ts=%d (< activation)", ts)
	}
	// Poison with a pre-activation timestamp — a post-activation override must still
	// answer PRESENT from its own field.
	params.SetRulesContext(&params.Rules{}, cfg, 0)
	for _, ts := range []uint64{activation, activation + 1, ^uint64(0)} {
		o := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: ts}
		c, ok := o.PrecompileOverride(settleAddr9999)
		require.Truef(t, ok, "0x9999 must dispatch at ts=%d (>= activation)", ts)
		require.NotNilf(t, c, "0x9999 override must return the wrapped settlement contract at ts=%d", ts)
	}
}

// TestDexSettle_PrecompileOverride_NoGlobalRace is the -race regression for the
// activation model. It reproduces the relaunch scenario: a verify goroutine replaying a
// post-activation block concurrently with an eth_call/estimateGas/worker goroutine that
// rewrites the last-writer-wins global timestamp. Both overriders are pinned to their OWN
// post-activation timestamps, so both MUST deterministically see 0x9999 PRESENT on every
// iteration — presence cannot depend on who wrote the global last.
//
// Run with: go test -race -run NoGlobalRace ./core/
func TestDexSettle_PrecompileOverride_NoGlobalRace(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	a := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: activation}          // activation-block replay
	b := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: activation + 1 << 30} // later block / eth_call

	const iters = 2000
	done := make(chan struct{})

	go func() {
		r := &params.Rules{}
		for {
			select {
			case <-done:
				return
			default:
				params.SetRulesContext(r, cfg, activation+1)
				params.SetRulesContext(r, cfg, 0) // pre-activation clobber
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
					t.Errorf("0x9999 override saw ABSENT at iter %d — a post-activation override must be "+
						"present on every replay regardless of the concurrent global timestamp", i)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(done)
}

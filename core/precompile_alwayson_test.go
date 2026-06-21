// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/precompile/modules"
	// registry side-effect: bridges the external DEX settlement module (0x9999)
	// into the EVM's internal registry so it is visible as AlwaysOn here.
	_ "github.com/luxfi/evm/precompile/registry"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/tracing"
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

// TestDexSettle_GenesisBuilders_Skip9999 is the AUTHORITATIVE INFO1 regression: with the
// registry bridge imported (so 0x9999 is a live AlwaysOn module in the EVM registry), the
// genesis-config builders must NOT emit a timestamp-0 genesis config for 0x9999. If they
// did, a fresh chain built via SetAllGenesisPrecompiles/AllGenesisPrecompiles would enable
// 0x9999 from block 0 — bypassing the Dec 25 2025 dated fork and re-introducing the
// genesis-marker mutation the fork exists to avoid. (The extras-package guard test runs
// without the registry import, so 0x9999 is invisible there; THIS test sees it.)
func TestDexSettle_GenesisBuilders_Skip9999(t *testing.T) {
	// Precondition: 0x9999 really is registered as AlwaysOn in this package's module view.
	m, ok := modules.GetPrecompileModuleByAddress(settleAddr9999)
	require.True(t, ok)
	require.True(t, m.AlwaysOn)

	fromFunc := extras.AllGenesisPrecompiles()
	_, in := fromFunc[m.ConfigKey]
	require.Falsef(t, in,
		"AllGenesisPrecompiles must SKIP the AlwaysOn 0x9999 module (%s) — its activation is the "+
			"dated fork, not a genesis config; a genesis entry would bypass the gate", m.ConfigKey)

	var cfg extras.ChainConfig
	cfg.SetAllGenesisPrecompiles()
	_, inMethod := cfg.GenesisPrecompiles[m.ConfigKey]
	require.Falsef(t, inMethod,
		"SetAllGenesisPrecompiles must SKIP the AlwaysOn 0x9999 module (%s)", m.ConfigKey)
}

// TestDexSettle_PrecompileOverride_PerEVMTimestamp pins the H1 fix: the dispatch gate
// (LuxPrecompileOverrider.PrecompileOverride) must decide 0x9999's enabled-ness from the
// overrider's OWN per-EVM timestamp (o.timestamp), NOT from the process-global
// lastRulesContext. We POISON the global with the opposite timestamp first, then assert
// the override still answers from its own field. Under the pre-fix code (which read
// params.GetRulesExtra(Rules{}) → the global), a pre-fork EVM would wrongly see 0x9999
// ENABLED whenever any concurrent caller had stamped a post-fork timestamp into the
// global — a consensus divergence on the relaunch/replay path.
func TestDexSettle_PrecompileOverride_PerEVMTimestamp(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	// Poison the global with a POST-fork timestamp; a correct pre-fork override ignores it.
	params.SetRulesContext(&params.Rules{}, cfg, tsPostActivation)
	pre := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: tsPreActivation}
	_, okPre := pre.PrecompileOverride(settleAddr9999)
	require.False(t, okPre,
		"pre-fork EVM must see 0x9999 ABSENT even when the global holds a post-fork timestamp "+
			"(dispatch gate must read o.timestamp, not lastRulesContext)")

	// Poison the global with a PRE-fork timestamp; a correct post-fork override ignores it.
	params.SetRulesContext(&params.Rules{}, cfg, tsPreActivation)
	post := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: tsPostActivation}
	c, okPost := post.PrecompileOverride(settleAddr9999)
	require.True(t, okPost,
		"post-fork EVM must see 0x9999 PRESENT even when the global holds a pre-fork timestamp")
	require.NotNil(t, c, "post-fork override must return the wrapped 0x9999 contract")
}

// TestDexSettle_PrecompileOverride_NoGlobalRace is the -race regression test for H1. It
// reproduces the exact relaunch scenario: a verify goroutine replaying a PRE-fork block
// concurrently with a post-fork eth_call/estimateGas/worker goroutine that rewrites the
// last-writer-wins global timestamp. The pre-fork side MUST deterministically see 0x9999
// ABSENT (plain account, no SettleContract.Run) on every iteration, and the post-fork
// side MUST deterministically see it PRESENT — regardless of who wrote the global last.
//
// Run with: go test -race -run NoGlobalRace ./core/
// Under the pre-fix code this fails the assertion (and/or -race flags the unsynchronised
// last-writer-wins read driving a consensus-relevant branch). Under the fix it is
// deterministic because each override reads only its own immutable fields.
func TestDexSettle_PrecompileOverride_NoGlobalRace(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	pre := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: tsPreActivation}
	post := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: tsPostActivation}

	const iters = 2000
	done := make(chan struct{})

	// Adversary: continuously clobber the global with alternating timestamps, mimicking
	// concurrent eth_call (wall-clock, post-fork) and any pre-fork rules construction. This
	// is precisely what SetRulesContext does on every Rules()/RulesAt() across goroutines.
	go func() {
		r := &params.Rules{}
		for {
			select {
			case <-done:
				return
			default:
				params.SetRulesContext(r, cfg, tsPostActivation)
				params.SetRulesContext(r, cfg, tsPreActivation)
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// Pre-fork replay goroutine: 0x9999 must be ABSENT every single time.
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			if _, ok := pre.PrecompileOverride(settleAddr9999); ok {
				t.Errorf("pre-fork override saw 0x9999 ENABLED at iter %d — consensus divergence: "+
					"a concurrent post-fork timestamp leaked through the global", i)
				return
			}
		}
	}()

	// Post-fork goroutine: 0x9999 must be PRESENT every single time.
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			if _, ok := post.PrecompileOverride(settleAddr9999); !ok {
				t.Errorf("post-fork override saw 0x9999 ABSENT at iter %d — a concurrent pre-fork "+
					"timestamp leaked through the global", i)
				return
			}
		}
	}()

	wg.Wait()
	close(done)
}

// TestDexSettle_Replay_PreForkValueTransferCreditsPlainAccount is the replay e2e for
// H1/INFO2: on the relaunch path, replaying a PRE-fork value transfer to 0x9999 must
// credit a PLAIN account and must NOT invoke SettleContract.Run. geth's vm.Call routes
// to the precompile iff PrecompileOverride returns (contract,true); when it returns
// (nil,false), 0x9999 is an ordinary account and the EVM performs the balance transfer.
// We drive that exact branch at the dispatch boundary (CGO-free — no full vm.NewEVM):
//
//   - PRE-fork override ⇒ (nil,false) ⇒ we apply the plain-account transfer and assert the
//     0x9999 account balance rose by the sent amount, while 0x9999 carries no activation
//     marker (nonce/code stay zero — it is a plain recipient, the precompile never ran).
//   - POST-fork override ⇒ (contract,true) ⇒ the EVM would dispatch the precompile instead.
//
// This is the canonical-history guarantee: a historical tx that paid native value into
// 0x9999 before the fork replays as a credit, byte-identical to the committed state.
func TestDexSettle_Replay_PreForkValueTransferCreditsPlainAccount(t *testing.T) {
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})
	const sent uint64 = 7_500_000_000

	// Poison the global to a post-fork wall-clock value — the live, RPC-serving post-fork
	// node's reality while it replays the imported pre-fork block. The pre-fork override
	// must IGNORE this and keep 0x9999 a plain account.
	params.SetRulesContext(&params.Rules{}, cfg, tsPostActivation)

	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	require.NoError(t, err)

	pre := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: tsPreActivation}
	contract, dispatched := pre.PrecompileOverride(settleAddr9999)
	require.False(t, dispatched,
		"pre-fork replay must NOT dispatch the 0x9999 precompile — it is a plain account")
	require.Nil(t, contract)

	// PrecompileOverride said "plain account", so the EVM credits the recipient. Mirror that.
	statedb.AddBalance(settleAddr9999, uint256.NewInt(sent), tracing.BalanceChangeTransfer)

	require.Equal(t, uint256.NewInt(sent), statedb.GetBalance(settleAddr9999),
		"pre-fork value transfer to 0x9999 must credit the plain account by the sent amount")
	require.Zero(t, statedb.GetNonce(settleAddr9999),
		"replayed pre-fork credit must not touch 0x9999 nonce — the precompile never ran")
	require.Empty(t, statedb.GetCode(settleAddr9999),
		"replayed pre-fork credit must not install marker code — 0x9999 is a plain account pre-fork")

	// Sanity: at/after the fork the SAME address DOES dispatch the precompile.
	post := &LuxPrecompileOverrider{chainConfig: cfg, timestamp: tsPostActivation}
	c2, dispatched2 := post.PrecompileOverride(settleAddr9999)
	require.True(t, dispatched2,
		"post-fork: 0x9999 must dispatch the settlement precompile, not behave as a plain account")
	require.NotNil(t, c2)
}

func ptr(v uint64) *uint64 { return &v }

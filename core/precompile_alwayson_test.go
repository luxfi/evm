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

// precompile_alwayson_test.go pins the always-on activation of the DEX settlement
// money path 0x9999. It is ACTIVE on every chain from genesis with NO config entry
// (no dexSettleConfig genesis precompile, no precompileUpgrades timestamp). Activation
// is DISPATCH-only by design: the precompile is in the enabled set so the EVM runs it
// for a tx-to-0x9999, a low-level CALL/STATICCALL, and the 0x9010 V4 forward. It
// deliberately gets NO genesis state write — the genesis root is consensus-critical
// and immutable for existing networks, so always-on must never change the genesis hash.
//
// If always-on regresses, 0x9999 would only be callable on chains carrying a
// dexSettleConfig entry — exactly the per-net config this design kills.

var settleAddr9999 = common.HexToAddress("0x0000000000000000000000000000000000009999")

// TestAlwaysOn_9999_Registered asserts the DEX settlement precompile bridges into the
// EVM registry as AlwaysOn and is enumerated by AlwaysOnModules().
func TestAlwaysOn_9999_Registered(t *testing.T) {
	m, ok := modules.GetPrecompileModuleByAddress(settleAddr9999)
	require.True(t, ok, "0x9999 must be registered in the EVM precompile registry")
	require.True(t, m.AlwaysOn, "0x9999 (%s) must be AlwaysOn — it is the always-on money path", m.ConfigKey)

	found := false
	for _, am := range modules.AlwaysOnModules() {
		if am.Address == settleAddr9999 {
			found = true
		}
	}
	require.True(t, found, "0x9999 must be enumerated by modules.AlwaysOnModules()")
}

// TestAlwaysOn_9999_DispatchEnabledWithNoConfig asserts 0x9999 is in the active
// precompile set on a chain whose config carries NO precompile activation whatsoever
// (no genesis precompiles, no upgrades). This is the dispatch gate the EVM consults in
// PrecompileOverride: if it is enabled, the precompile runs. Always-on guarantees it is
// enabled on every chain at every timestamp with zero config.
func TestAlwaysOn_9999_DispatchEnabledWithNoConfig(t *testing.T) {
	// A bare chain config — nothing activates 0x9999 via config; only always-on can.
	cfg := params.WithExtra(&params.ChainConfig{}, &extras.ChainConfig{})

	// GetExtrasRules is the pure function that builds the per-block enabled precompile
	// set (PrecompileOverride consults its IsPrecompileEnabled). Always-on injects 0x9999
	// here unconditionally. Assert it is present in the enabled set with NO config, at
	// genesis and at a far-future timestamp (always-on is timestamp-independent).
	rules0 := params.GetExtrasRules(params.Rules{}, cfg, 0)
	_, ok0 := rules0.Precompiles[settleAddr9999]
	require.True(t, ok0,
		"0x9999 must be in the enabled precompile set at genesis with NO config — the always-on money path")

	rulesLater := params.GetExtrasRules(params.Rules{}, cfg, 2_000_000_000)
	_, okLater := rulesLater.Precompiles[settleAddr9999]
	require.True(t, okLater, "0x9999 must stay in the enabled set at every timestamp")
}

// TestAlwaysOn_NoGenesisStateWrite asserts always-on does NOT mutate genesis state:
// ApplyPrecompileActivations at genesis with a bare config leaves 0x9999 with no code
// and no nonce. This guards the consensus-critical invariant that always-on never
// changes the genesis hash (which would fork existing networks).
func TestAlwaysOn_NoGenesisStateWrite(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	require.NoError(t, err)

	cfg := &params.ChainConfig{}
	blockCtx := NewBlockContext(big.NewInt(0), 0)
	require.NoError(t, ApplyPrecompileActivations(cfg, nil, blockCtx, statedb))

	require.Zero(t, statedb.GetNonce(settleAddr9999),
		"0x9999 must NOT get a genesis nonce — always-on writes no genesis state (genesis hash is immutable)")
	require.Empty(t, statedb.GetCode(settleAddr9999),
		"0x9999 must NOT get genesis code — always-on writes no genesis state (genesis hash is immutable)")
}

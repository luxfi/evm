// Copyright (C) 2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"testing"

	"github.com/luxfi/evm/utils"
	"github.com/stretchr/testify/require"
)

// strict_pq_activation_test.go proves Red wiring task 2 deliverable (b):
// the StrictPQ gate is no longer inert. NetworkUpgrades.IsStrictPQ is the
// single predicate contract.RefuseUnderStrictPQ consults (via the chain
// config's StrictPQReporter); these tests prove it returns true on a chain
// whose StrictPQTimestamp is set, and that the production wiring (vm.config.
// PQ -> StrictPQTimestamp=0) makes it active from genesis.

const quasarStrictPQTS uint64 = 1766708400 // Dec 25 2025 16:20 PST — mainnet

// TestIsStrictPQ_NilTimestamp_NeverActive confirms the default (nil) is
// classical-permissive: IsStrictPQ is false at every timestamp. This is
// exactly the inert state Red found on chains that never set the field.
func TestIsStrictPQ_NilTimestamp_NeverActive(t *testing.T) {
	n := NetworkUpgrades{} // StrictPQTimestamp nil
	require.False(t, n.IsStrictPQ(0))
	require.False(t, n.IsStrictPQ(quasarStrictPQTS))
	require.False(t, n.IsStrictPQ(^uint64(0)))
}

// TestIsStrictPQ_GenesisActivation proves a prod strict chain wired via
// vm.config.PQ (which sets StrictPQTimestamp = &0) reports strict-PQ at
// every timestamp ≥ 0 — i.e. from genesis. This is the posture a node
// rebuilding state from scratch gets.
func TestIsStrictPQ_GenesisActivation(t *testing.T) {
	zero := uint64(0)
	n := NetworkUpgrades{StrictPQTimestamp: &zero}
	require.True(t, n.IsStrictPQ(0), "StrictPQTimestamp=0 ⇒ active from genesis")
	require.True(t, n.IsStrictPQ(quasarStrictPQTS))
}

// TestIsStrictPQ_TimestampActivation proves the running-chain rollout: the
// canonical mainnet upgrade.json sets strictPQTimestamp = the Quasar
// timestamp. IsStrictPQ must be false strictly before it and true at/after.
func TestIsStrictPQ_TimestampActivation(t *testing.T) {
	ts := quasarStrictPQTS
	n := NetworkUpgrades{StrictPQTimestamp: &ts}
	require.False(t, n.IsStrictPQ(ts-1), "must be classical-permissive strictly before activation")
	require.True(t, n.IsStrictPQ(ts), "must be strict-PQ at the activation timestamp")
	require.True(t, n.IsStrictPQ(ts+1))
}

// TestIsStrictPQ_OverridePropagates proves NetworkUpgrades.Override carries
// StrictPQTimestamp through — the path upgrade.json's networkUpgradeOverrides
// takes to reach the live config. Without this the upgrade.json activation
// would be silently dropped (the inert failure mode).
func TestIsStrictPQ_OverridePropagates(t *testing.T) {
	base := NetworkUpgrades{} // nil — inert
	require.False(t, base.IsStrictPQ(quasarStrictPQTS))

	ts := quasarStrictPQTS
	base.Override(&NetworkUpgrades{StrictPQTimestamp: &ts})
	require.True(t, base.IsStrictPQ(quasarStrictPQTS),
		"Override must carry StrictPQTimestamp so upgrade.json's override actually activates the gate")
}

// TestSetDefaults_PreservesExplicitStrictPQ proves SetDefaults does not
// clobber an explicitly-set StrictPQTimestamp (it has no default, so a set
// value must survive default-filling).
func TestSetDefaults_PreservesExplicitStrictPQ(t *testing.T) {
	n := NetworkUpgrades{
		EVMTimestamp:      utils.NewUint64(0),
		StrictPQTimestamp: utils.NewUint64(0),
	}
	n.SetDefaults(getTestMainnetConfig())
	require.NotNil(t, n.StrictPQTimestamp)
	require.Equal(t, uint64(0), *n.StrictPQTimestamp)
	require.True(t, n.IsStrictPQ(0))
}

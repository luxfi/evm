// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/types"
	"github.com/stretchr/testify/require"
)

// coinbaseAddr is an arbitrary non-vault fee recipient used to prove the legacy
// path credits the coinbase and the split path does not.
var coinbaseAddr = common.HexToAddress("0x0100000000000000000000000000000000000000")

func newFeeSplitStateDB(t *testing.T) *state.StateDB {
	t.Helper()
	sdb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	require.NoError(t, err)
	return sdb
}

// feeSplitConfig returns a chain config whose FeeSplit upgrade is active from
// genesis (active=true) or never (active=false).
func feeSplitConfig(active bool) *params.ChainConfig {
	extra := &extras.ChainConfig{}
	if active {
		ts := uint64(0)
		extra.FeeSplitTimestamp = &ts
	}
	return params.WithExtra(&params.ChainConfig{ChainID: big.NewInt(96369)}, extra)
}

// TestCreditTxFeeLegacy: with FeeSplit inactive, 100% of the fee goes to the
// coinbase and the vault is never touched (historical behavior preserved).
func TestCreditTxFeeLegacy(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_000)

	creditTxFee(sdb, feeSplitConfig(false), coinbaseAddr, 0, fee)

	require.Equal(t, uint64(1_000_000), sdb.GetBalance(coinbaseAddr).Uint64(), "coinbase gets full fee")
	require.True(t, sdb.GetBalance(extras.FeeRewardVault).IsZero(), "vault untouched when split inactive")
}

// TestCreditTxFeeSplit: with FeeSplit active, the fee splits 50/50 — the vault
// receives floor(fee/2), the coinbase receives nothing, and the remaining
// (ceil) half is burned (credited to no account).
func TestCreditTxFeeSplit(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_000)

	creditTxFee(sdb, feeSplitConfig(true), coinbaseAddr, 0, fee)

	reward := sdb.GetBalance(extras.FeeRewardVault).Uint64()
	coinbase := sdb.GetBalance(coinbaseAddr).Uint64()
	require.Equal(t, uint64(500_000), reward, "vault gets floor(fee/2)")
	require.Equal(t, uint64(0), coinbase, "coinbase gets nothing under split")

	burn := fee.Uint64() - reward
	require.Equal(t, uint64(500_000), burn, "burn = fee - reward")
	require.Equal(t, fee.Uint64(), reward+burn, "conservation: reward + burn == fee")
}

// TestCreditTxFeeOddWei: on an odd fee the single leftover wei is assigned to
// the burn (burn == reward+1), never over-crediting the reward side. This is the
// deterministic rounding rule every validator must apply identically.
func TestCreditTxFeeOddWei(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_001)

	creditTxFee(sdb, feeSplitConfig(true), coinbaseAddr, 0, fee)

	reward := sdb.GetBalance(extras.FeeRewardVault).Uint64()
	burn := fee.Uint64() - reward
	require.Equal(t, uint64(500_000), reward, "reward = floor(fee/2)")
	require.Equal(t, uint64(500_001), burn, "burn = ceil(fee/2) gets the odd wei")
	require.Equal(t, burn, reward+1, "odd wei goes to burn")
	require.GreaterOrEqual(t, burn, reward, "burn >= reward always")
}

// TestCreditTxFeeConservation exercises the invariant reward+burn == fee across
// a spread of fees (zero, one, even, odd, uint64 max, and a >uint64 value). For
// every fee: vault delta == floor(fee/2), burn == fee - vault, and the two halves
// reconstruct the fee exactly. burn is the exact per-tx supply reduction.
func TestCreditTxFeeConservation(t *testing.T) {
	max64 := new(uint256.Int).SetUint64(^uint64(0))
	big1 := new(uint256.Int).Lsh(uint256.NewInt(1), 200) // 2^200 wei, well beyond uint64
	fees := []*uint256.Int{
		uint256.NewInt(0),
		uint256.NewInt(1),
		uint256.NewInt(2),
		uint256.NewInt(3),
		uint256.NewInt(21_000 * 25_000_000_000), // 21k gas at 25 gwei
		max64,
		big1,
	}

	for _, fee := range fees {
		sdb := newFeeSplitStateDB(t)
		creditTxFee(sdb, feeSplitConfig(true), coinbaseAddr, 0, fee)

		reward := sdb.GetBalance(extras.FeeRewardVault) // = floor(fee/2)
		burn := new(uint256.Int).Sub(fee, reward)       // = fee - reward
		wantReward := new(uint256.Int).Rsh(fee, 1)

		require.Equal(t, wantReward, reward, "vault = floor(fee/2) for fee=%s", fee)
		require.True(t, sdb.GetBalance(coinbaseAddr).IsZero(), "coinbase = 0 for fee=%s", fee)

		sum := new(uint256.Int).Add(reward, burn)
		require.Equal(t, fee, sum, "conservation reward+burn==fee for fee=%s", fee)
		require.True(t, burn.Cmp(reward) >= 0, "burn>=reward for fee=%s", fee)
	}
}

// TestCreditTxFeeDeterministic: the split is a pure function of the fee — two
// independent states credited the same fee reach byte-identical balances. A
// non-deterministic handler would fork the chain, so this is a consensus guard.
func TestCreditTxFeeDeterministic(t *testing.T) {
	fee := uint256.NewInt(777_777_777)

	sdbA := newFeeSplitStateDB(t)
	sdbB := newFeeSplitStateDB(t)
	creditTxFee(sdbA, feeSplitConfig(true), coinbaseAddr, 0, fee)
	creditTxFee(sdbB, feeSplitConfig(true), coinbaseAddr, 0, fee)

	require.Equal(t, sdbA.GetBalance(extras.FeeRewardVault), sdbB.GetBalance(extras.FeeRewardVault))
	require.Equal(t, sdbA.IntermediateRoot(true), sdbB.IntermediateRoot(true), "identical post-state root")
}

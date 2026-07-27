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

// rewardSink stands in for whatever address the RewardManager precompile
// currently points at — the value BlockChain.GetCoinbaseAt resolves and hands to
// creditTxFee. Deliberately NOT the blackhole and NOT any protocol constant: the
// destination of the kept half is governed state, so the fee logic must work for
// an arbitrary address it has never heard of.
var rewardSink = common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D")

// blackholeAddr is the address GetCoinbaseAt falls back to when RewardManager is
// NOT enabled. Fees sent here are stranded (keyless account), not burned.
var blackholeAddr = common.HexToAddress("0x0100000000000000000000000000000000000000")

// legacyFeeRewardVault is the compiled-in address the split used to credit before
// the destination was decomplected from the ratio. Nothing may ever credit it
// again: it is keyless, so anything landing there is stranded forever.
var legacyFeeRewardVault = common.HexToAddress("0x0100000000000000000000000000000000000002")

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
// configured coinbase (historical behavior preserved).
func TestCreditTxFeeLegacy(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_000)

	creditTxFee(sdb, feeSplitConfig(false), rewardSink, 0, fee)

	require.Equal(t, uint64(1_000_000), sdb.GetBalance(rewardSink).Uint64(), "coinbase gets full fee")
	require.True(t, sdb.GetBalance(legacyFeeRewardVault).IsZero(), "no compiled-in vault is ever credited")
}

// TestCreditTxFeeSplit: with FeeSplit active, the fee splits 50/50 — the
// CONFIGURED COINBASE receives floor(fee/2) and the remaining (ceil) half is
// burned (credited to no account). The split changes how much is kept, never
// where the kept part goes.
func TestCreditTxFeeSplit(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_000)

	creditTxFee(sdb, feeSplitConfig(true), rewardSink, 0, fee)

	reward := sdb.GetBalance(rewardSink).Uint64()
	require.Equal(t, uint64(500_000), reward, "configured coinbase gets floor(fee/2)")
	require.True(t, sdb.GetBalance(legacyFeeRewardVault).IsZero(), "no compiled-in vault is ever credited")

	burn := fee.Uint64() - reward
	require.Equal(t, uint64(500_000), burn, "burn = fee - reward")
	require.Equal(t, fee.Uint64(), reward+burn, "conservation: reward + burn == fee")
}

// TestCreditTxFeeDestinationIsGoverned is the whole point of the decomplection:
// the same fee, the same config, two different configured coinbases -> the kept
// half follows the configured address. Nothing about the destination is
// compiled in, so a RewardManager admin transaction (which is what moves that
// address) is sufficient to redirect the reward stream.
func TestCreditTxFeeDestinationIsGoverned(t *testing.T) {
	fee := uint256.NewInt(1_000_000)
	newTarget := common.HexToAddress("0x229599f227231d8C90fcF1a78589F5DC4b7A6962")

	before := newFeeSplitStateDB(t)
	creditTxFee(before, feeSplitConfig(true), rewardSink, 0, fee)

	after := newFeeSplitStateDB(t)
	creditTxFee(after, feeSplitConfig(true), newTarget, 0, fee)

	require.Equal(t, uint64(500_000), before.GetBalance(rewardSink).Uint64())
	require.True(t, before.GetBalance(newTarget).IsZero())

	require.Equal(t, uint64(500_000), after.GetBalance(newTarget).Uint64())
	require.True(t, after.GetBalance(rewardSink).IsZero(), "old target stops accruing the moment the configured coinbase moves")

	// The burn is identical either way: the ratio is protocol, the address is policy.
	require.Equal(t,
		fee.Uint64()-before.GetBalance(rewardSink).Uint64(),
		fee.Uint64()-after.GetBalance(newTarget).Uint64(),
		"burn is invariant under a destination change",
	)
}

// TestCreditTxFeeUngovernedDestinationStrands documents the honest failure mode:
// if RewardManager is NOT enabled, GetCoinbaseAt returns the blackhole, so the
// kept half strands at a keyless account exactly as the full fee does today.
// The split alone does not create governance — activating RewardManager does.
func TestCreditTxFeeUngovernedDestinationStrands(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_000)

	creditTxFee(sdb, feeSplitConfig(true), blackholeAddr, 0, fee)

	require.Equal(t, uint64(500_000), sdb.GetBalance(blackholeAddr).Uint64(),
		"with RewardManager off the kept half accrues at the keyless blackhole (stranded, NOT burned)")
}

// TestCreditTxFeeOddWei: on an odd fee the single leftover wei is assigned to
// the burn (burn == reward+1), never over-crediting the reward side. This is the
// deterministic rounding rule every validator must apply identically.
func TestCreditTxFeeOddWei(t *testing.T) {
	sdb := newFeeSplitStateDB(t)
	fee := uint256.NewInt(1_000_001)

	creditTxFee(sdb, feeSplitConfig(true), rewardSink, 0, fee)

	reward := sdb.GetBalance(rewardSink).Uint64()
	burn := fee.Uint64() - reward
	require.Equal(t, uint64(500_000), reward, "reward = floor(fee/2)")
	require.Equal(t, uint64(500_001), burn, "burn = ceil(fee/2) gets the odd wei")
	require.Equal(t, burn, reward+1, "odd wei goes to burn")
	require.GreaterOrEqual(t, burn, reward, "burn >= reward always")
}

// TestCreditTxFeeConservation exercises the invariant reward+burn == fee across
// a spread of fees (zero, one, even, odd, uint64 max, and a >uint64 value). For
// every fee: coinbase delta == floor(fee/2), burn == fee - coinbase delta, and
// the two halves reconstruct the fee exactly. burn is the exact per-tx supply
// reduction.
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
		creditTxFee(sdb, feeSplitConfig(true), rewardSink, 0, fee)

		reward := sdb.GetBalance(rewardSink)      // = floor(fee/2)
		burn := new(uint256.Int).Sub(fee, reward) // = fee - reward
		wantReward := new(uint256.Int).Rsh(fee, 1)

		require.Equal(t, wantReward, reward, "coinbase = floor(fee/2) for fee=%s", fee)
		require.True(t, sdb.GetBalance(legacyFeeRewardVault).IsZero(), "vault untouched for fee=%s", fee)

		sum := new(uint256.Int).Add(reward, burn)
		require.Equal(t, fee, sum, "conservation reward+burn==fee for fee=%s", fee)
		require.True(t, burn.Cmp(reward) >= 0, "burn>=reward for fee=%s", fee)
	}
}

// TestCreditTxFeeDeterministic: the split is a pure function of (fee, coinbase)
// — two independent states credited the same fee reach byte-identical state
// roots. A non-deterministic handler would fork the chain, so this is a
// consensus guard.
func TestCreditTxFeeDeterministic(t *testing.T) {
	fee := uint256.NewInt(777_777_777)

	sdbA := newFeeSplitStateDB(t)
	sdbB := newFeeSplitStateDB(t)
	creditTxFee(sdbA, feeSplitConfig(true), rewardSink, 0, fee)
	creditTxFee(sdbB, feeSplitConfig(true), rewardSink, 0, fee)

	require.Equal(t, sdbA.GetBalance(rewardSink), sdbB.GetBalance(rewardSink))
	require.Equal(t, sdbA.IntermediateRoot(true), sdbB.IntermediateRoot(true), "identical post-state root")
}

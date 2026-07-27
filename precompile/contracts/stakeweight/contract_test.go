// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// runGetVerifiedStakeWeight drives Run() against a block that already carries a
// predicate verdict, exactly as EVM execution would. Nothing here consults the
// P-chain: Run() is a pure function of data committed in the block.
func runGetVerifiedStakeWeight(t *testing.T, slot []byte, slotExists bool, failedIndexes []int, gas uint64) ([]byte, uint64, error) {
	t.Helper()
	ctrl := gomock.NewController(t)
	txHash := common.HexToHash("0xdecafbad")

	failed := set.NewBits()
	for _, i := range failedIndexes {
		failed.Add(i)
	}

	stateDB := contract.NewMockStateDB(ctrl)
	stateDB.EXPECT().GetPredicateStorageSlots(ContractAddress, 0).Return(slot, slotExists).AnyTimes()
	stateDB.EXPECT().GetTxHash().Return(txHash).AnyTimes()

	blockCtx := contract.NewMockBlockContext(ctrl)
	blockCtx.EXPECT().GetPredicateResults(txHash, ContractAddress).Return(failed.Bytes()).AnyTimes()

	accessibleState := contract.NewMockAccessibleState(ctrl)
	accessibleState.EXPECT().GetStateDB().Return(stateDB).AnyTimes()
	accessibleState.EXPECT().GetBlockContext().Return(blockCtx).AnyTimes()

	input, err := PackGetVerifiedStakeWeight(0)
	require.NoError(t, err)
	return StakeWeightPrecompile.Run(accessibleState, common.HexToAddress("0x1"), ContractAddress, input, gas, true)
}

func testBallot() *Ballot {
	b := &Ballot{
		NodeID:       ids.GenerateTestNodeID(),
		Voter:        voterAddr,
		AuthEpoch:    3,
		PChainHeight: snapshotHeight,
		Weight:       mainnetVdrWeight,
		TotalWeight:  mainnetTotalWeight,
	}
	return b
}

func TestGetVerifiedStakeWeight(t *testing.T) {
	require := require.New(t)
	ballot := testBallot()

	ret, remaining, err := runGetVerifiedStakeWeight(t, predicateBytes(ballot), true, nil, GetVerifiedStakeWeightGasCost)
	require.NoError(err)
	require.Zero(remaining)

	out, err := UnpackGetVerifiedStakeWeightOutput(ret)
	require.NoError(err)
	require.True(out.Valid)
	require.Equal([20]byte(ballot.NodeID), out.StakeWeight.NodeID)
	require.Equal(ballot.Voter, out.StakeWeight.Voter)
	require.Equal(ballot.AuthEpoch, out.StakeWeight.AuthEpoch)
	require.Equal(ballot.PChainHeight, out.StakeWeight.PChainHeight)
	require.Equal(new(big.Int).SetUint64(ballot.Weight), out.StakeWeight.Weight)
	require.Equal(new(big.Int).SetUint64(ballot.TotalWeight), out.StakeWeight.TotalWeight)
}

// The claims of a ballot the block marked invalid must never reach Solidity as
// a number: a failed predicate returns valid=false and a zeroed struct, so a
// contract that ignores `valid` still counts zero weight rather than the
// attacker's claim.
func TestGetVerifiedStakeWeightInvalid(t *testing.T) {
	require := require.New(t)
	ballot := testBallot()

	for name, tc := range map[string]struct {
		exists bool
		failed []int
	}{
		"predicate verification failed": {exists: true, failed: []int{0}},
		"no ballot at that index":       {exists: false},
	} {
		t.Run(name, func(t *testing.T) {
			ret, _, err := runGetVerifiedStakeWeight(t, predicateBytes(ballot), tc.exists, tc.failed, GetVerifiedStakeWeightGasCost)
			require.NoError(err)

			out, err := UnpackGetVerifiedStakeWeightOutput(ret)
			require.NoError(err)
			require.False(out.Valid)
			require.Zero(out.StakeWeight.Weight.Sign())
			require.Zero(out.StakeWeight.TotalWeight.Sign())
			require.Equal([20]byte{}, out.StakeWeight.NodeID)
			require.Equal(common.Address{}, out.StakeWeight.Voter)
		})
	}
}

// A bit set at another index must not taint this one.
func TestGetVerifiedStakeWeightOtherIndexFailed(t *testing.T) {
	require := require.New(t)
	ret, _, err := runGetVerifiedStakeWeight(t, predicateBytes(testBallot()), true, []int{1, 2}, GetVerifiedStakeWeightGasCost)
	require.NoError(err)

	out, err := UnpackGetVerifiedStakeWeightOutput(ret)
	require.NoError(err)
	require.True(out.Valid)
}

func TestGetVerifiedStakeWeightOutOfGas(t *testing.T) {
	_, _, err := runGetVerifiedStakeWeight(t, predicateBytes(testBallot()), true, nil, GetVerifiedStakeWeightGasCost-1)
	require.ErrorIs(t, err, vm.ErrOutOfGas)
}

func TestModuleRegistered(t *testing.T) {
	require := require.New(t)
	require.Equal(common.HexToAddress("0x0200000000000000000000000000000000000006"), ContractAddress)
	require.Equal("stakeWeightConfig", ConfigKey)
	// Registration happens in init(); a duplicate address or key would have
	// panicked before this test ran.
	require.Equal(ContractAddress, Module.Address)
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/evm/predicate"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	validators "github.com/luxfi/validators"
	"github.com/luxfi/vm/chain"
	"github.com/stretchr/testify/require"
)

// The whole path a real vote takes, with no shortcuts between the stages:
//
//	ballot -> transaction access list -> predicate arguments -> VerifyPredicate
//	       -> results bitset -> what Solidity reads
//
// This is the test that would catch a mistake the unit tests cannot: an access
// list that does not round-trip, a bitset indexed the wrong way, or a slot the
// EVM would not find.
func TestEndToEndBallotThroughAccessList(t *testing.T) {
	require := require.New(t)

	chainID := ids.GenerateTestID()
	operator, err := localsigner.New()
	require.NoError(err)
	nodeID := ids.GenerateTestNodeID()
	forged := ids.GenerateTestNodeID()

	set5 := forkedMainnetSet(t)
	opPK := bls.PublicKeyToCompressedBytes(operator.PublicKey())
	set5[nodeID] = &validators.GetValidatorOutput{NodeID: nodeID, PublicKey: opPK, Weight: mainnetVdrWeight}
	set5[forged] = &validators.GetValidatorOutput{NodeID: forged, PublicKey: opPK, Weight: mainnetVdrWeight}
	total, err := totalWeight(set5)
	require.NoError(err)

	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{mainnetPChainHeight: set5})

	mk := func(node ids.NodeID, weight uint64) []byte {
		b := Ballot{
			NodeID:       node,
			Voter:        voterAddr,
			AuthEpoch:    1,
			PChainHeight: mainnetPChainHeight,
			Weight:       weight,
			TotalWeight:  total,
		}
		sig, sErr := operator.Sign(b.SigningBytes(chainID))
		require.NoError(sErr)
		copy(b.Signature[:], bls.SignatureToBytes(sig))
		return b.Bytes()
	}

	good := mk(nodeID, mainnetVdrWeight)
	// Index 1 overstates: same signer, but more weight than the set holds.
	bad := mk(forged, mainnetVdrWeight+1)

	// One transaction carrying two ballots for this precompile.
	tx := predicate.NewPredicateTx(
		big.NewInt(96369), 0, &common.Address{}, 2_000_000,
		big.NewInt(1), big.NewInt(1), big.NewInt(0), nil,
		types.AccessList{{
			Address:     ContractAddress,
			StorageKeys: hashSlice(predicate.PackPredicate(good)),
		}},
		ContractAddress, bad,
	)

	cfg := NewConfig(new(uint64))
	rules := &extras.Rules{Predicaters: map[common.Address]precompileconfig.Predicater{ContractAddress: cfg}}
	args := predicate.PreparePredicateStorageSlots(rules, tx.AccessList())
	require.Len(args[ContractAddress], 2, "both ballots must survive the access-list round trip")

	predicateCtx := &precompileconfig.PredicateContext{
		ConsensusCtx:       ctx,
		ProposerVMBlockCtx: &chain.Context{PChainHeight: mainnetPChainHeight},
	}

	// Reproduce core.CheckPredicates' bitset construction: a SET bit is a FAILURE.
	failed := set.NewBits()
	for i, p := range args[ContractAddress] {
		if err := cfg.VerifyPredicate(predicateCtx, p); err != nil {
			failed.Add(i)
		}
	}
	require.False(failed.Contains(0), "the honest ballot must pass")
	require.True(failed.Contains(1), "the overstated ballot must fail")

	// And what Solidity sees at each index.
	ret, _, err := runGetVerifiedStakeWeight(t, args[ContractAddress][0], true, bitsOf(failed, 2), GetVerifiedStakeWeightGasCost)
	require.NoError(err)
	out, err := UnpackGetVerifiedStakeWeightOutput(ret)
	require.NoError(err)
	require.True(out.Valid)
	require.Equal(mainnetVdrWeight, out.StakeWeight.Weight.Uint64())
	require.Equal(total, out.StakeWeight.TotalWeight.Uint64())
	require.Equal([20]byte(nodeID), out.StakeWeight.NodeID)
}

func hashSlice(b []byte) []common.Hash {
	out := make([]common.Hash, 0, len(b)/common.HashLength)
	for i := 0; i < len(b); i += common.HashLength {
		out = append(out, common.BytesToHash(b[i:i+common.HashLength]))
	}
	return out
}

func bitsOf(s set.Bits, n int) []int {
	var out []int
	for i := 0; i < n; i++ {
		if s.Contains(i) {
			out = append(out, i)
		}
	}
	return out
}

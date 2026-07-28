// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"encoding/hex"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/ids"
	validators "github.com/luxfi/validators"
	"github.com/luxfi/vm/chain"
	"github.com/stretchr/testify/require"
)

// Read-only fork of Lux mainnet (chain ID 96369). Captured from
// platform.getCurrentValidators on https://api.lux.network/v1/bc/P while
// C-chain eth_blockNumber was 0x10c1cf. Nothing here writes to mainnet; the
// production validator set is used purely as fixture state.
//
// Every entry carries the validator's REAL registered BLS key (the P-chain
// `signer.publicKey`, 48-byte compressed) — the same key the precompile checks
// authorisations against.
var mainnetValidators = []struct {
	nodeID string
	pk     string
	weight uint64
}{
	{"NodeID-DwsrqSkPoE3pXWrUt9nkJ5yBycwRQ246X", "8533063955b58b05a188baea620dfd35894657424e92ca378cc6b51f1fba4ded475265d59272f949e0a097f923211b1c", 500_000_000_000_000_000},
	{"NodeID-8mY2fhUehN27v3LCU84BnnKEoeRfd2weC", "85e48ceb16986dc86796fced851749c303441d9ae7b2b639b4b5eea6dd94b21193981e518863d961210e57ec7e1e7d6c", 500_000_000_000_000_000},
	{"NodeID-Ld9VFBQ9zGbd79z2vzaAqkQ3jHuqbtRpo", "865e60e9d5973af6b0c6d003afa104d337e8bf9ac8b8a711e260626781886073ce79b3efc3a8fd1534a4069c90b88182", 500_000_000_000_000_000},
	{"NodeID-2TwSZ2oyeBK2mv7JiseEQ8m74rotDj4QR", "84e20183f945fea4e08eb126338763a0508b15409f5fbb7e525013d5134069b1e1037226dc6bc295ab18d9c7605368cb", 500_000_000_000_000_000},
	{"NodeID-Mf3JfSY91oDwfBqf7rCLmhg4NDtDghw1f", "a9503da5b7b1fc48f6f699677ee5ba577b8cae2b6a98faa03fc1c8e33cafb075dd3ea93321fc3c13f8e22524bc25b17b", 500_000_000_000_000_000},
}

// mainnetPChainHeight is what platform.getHeight answers on 96369 today: the
// P-chain has produced no block beyond genesis, so every C-chain block is
// verified against P-chain height 0 and the validator set has never moved.
const mainnetPChainHeight uint64 = 0

func forkedMainnetSet(t testing.TB) vdrSet {
	t.Helper()
	set := make(vdrSet, len(mainnetValidators))
	for _, v := range mainnetValidators {
		nodeID, err := ids.NodeIDFromString(v.nodeID)
		require.NoError(t, err)
		pk, err := hex.DecodeString(v.pk)
		require.NoError(t, err)
		require.Len(t, pk, bls.PublicKeyLen)
		// Every registered key must actually decode; a validator whose key does
		// not is one that can never authorise a voter.
		_, err = bls.PublicKeyFromCompressedBytes(pk)
		require.NoError(t, err, "mainnet validator %s has an undecodable registered key", v.nodeID)
		set[nodeID] = &validators.GetValidatorOutput{NodeID: nodeID, PublicKey: pk, Weight: v.weight}
	}
	require.Len(t, set, len(mainnetValidators), "duplicate nodeID in the fork fixture")
	return set
}

// The electorate and the tally denominator come straight from the forked set.
func TestForkMainnetElectorate(t *testing.T) {
	require := require.New(t)
	set := forkedMainnetSet(t)

	total, err := totalWeight(set)
	require.NoError(err)
	require.Equal(uint64(2_500_000_000_000_000_000), total)
	require.Equal(5, len(set))

	// One map entry per nodeID is what makes double-counting a delegation
	// impossible: a delegator's stake is folded into its validator's single
	// weight, never listed separately. Mainnet has no delegations today
	// (delegatorWeight "0" on all five), so weight == own stake here.
	for _, vdr := range set {
		require.Equal(uint64(500_000_000_000_000_000), vdr.Weight)
	}
}

// Nobody can vote a real mainnet validator's weight without that validator's
// registered secret key — including whoever can write arbitrary ballots.
func TestForkMainnetStakeIsNotVotableWithoutTheRegisteredKey(t *testing.T) {
	require := require.New(t)
	chainID := ids.GenerateTestID()
	set := forkedMainnetSet(t)
	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{mainnetPChainHeight: set})

	attacker, err := localsigner.New()
	require.NoError(err)

	for _, v := range mainnetValidators {
		nodeID, err := ids.NodeIDFromString(v.nodeID)
		require.NoError(err)

		ballot := Ballot{
			NodeID:       nodeID,
			Voter:        voterAddr,
			AuthEpoch:    1,
			PChainHeight: mainnetPChainHeight,
			Weight:       v.weight,
			TotalWeight:  2_500_000_000_000_000_000,
		}
		sig, err := attacker.Sign(ballot.SigningBytes(chainID))
		require.NoError(err)
		copy(ballot.Signature[:], bls.SignatureToBytes(sig))

		err = NewConfig(utils.NewUint64(0)).VerifyPredicate(&precompileconfig.PredicateContext{
			ConsensusCtx:       ctx,
			ProposerVMBlockCtx: &chain.Context{PChainHeight: mainnetPChainHeight},
		}, predicateBytes(&ballot))
		require.ErrorIs(err, errUnauthorizedVoter, "forged ballot accepted for %s", v.nodeID)
	}
}

// End-to-end against the forked set: one validator authorises an EVM address,
// and that address votes the validator's real mainnet weight out of the real
// mainnet total. The authorising key stands in for the operator's — the secret
// halves of the production keys are, correctly, not available here.
func TestForkMainnetAuthorizedVoteCarriesRealWeight(t *testing.T) {
	require := require.New(t)
	chainID := ids.GenerateTestID()
	set := forkedMainnetSet(t)

	nodeID, err := ids.NodeIDFromString(mainnetValidators[0].nodeID)
	require.NoError(err)
	operator, err := localsigner.New()
	require.NoError(err)
	set[nodeID].PublicKey = bls.PublicKeyToCompressedBytes(operator.PublicKey())

	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{mainnetPChainHeight: set})
	cfg := NewConfig(utils.NewUint64(0))
	predicateCtx := &precompileconfig.PredicateContext{
		ConsensusCtx:       ctx,
		ProposerVMBlockCtx: &chain.Context{PChainHeight: mainnetPChainHeight},
	}

	ballot := Ballot{
		NodeID:       nodeID,
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: mainnetPChainHeight,
		Weight:       mainnetValidators[0].weight,
		TotalWeight:  2_500_000_000_000_000_000,
	}
	sig, err := operator.Sign(ballot.SigningBytes(chainID))
	require.NoError(err)
	copy(ballot.Signature[:], bls.SignatureToBytes(sig))

	require.NoError(cfg.VerifyPredicate(predicateCtx, predicateBytes(&ballot)))

	// The same authorisation is reusable for every later proposal — no
	// re-signing, no registry write, nobody to ask.
	second := ballot
	second.Weight = 1
	require.NoError(cfg.VerifyPredicate(predicateCtx, predicateBytes(&second)))

	// 5e17 of 2.5e18 is exactly one fifth: a single mainnet validator is 20% of
	// the electorate, so any threshold above 20% needs a second one.
	require.Equal(uint64(5), ballot.TotalWeight/ballot.Weight)

	// And Run() hands Solidity those exact numbers once the block records the pass.
	ret, _, err := runGetVerifiedStakeWeight(t, predicateBytes(&ballot), true, nil, GetVerifiedStakeWeightGasCost)
	require.NoError(err)
	out, err := UnpackGetVerifiedStakeWeightOutput(ret)
	require.NoError(err)
	require.True(out.Valid)
	require.Equal(ballot.Weight, out.StakeWeight.Weight.Uint64())
	require.Equal(ballot.TotalWeight, out.StakeWeight.TotalWeight.Uint64())
}

// P-chain height 0 is the only height mainnet can be read at today, and the
// bound still holds: a ballot cannot name a height the block has not reached.
func TestForkMainnetHeightZeroBound(t *testing.T) {
	require := require.New(t)
	chainID := ids.GenerateTestID()
	set := forkedMainnetSet(t)
	nodeID, err := ids.NodeIDFromString(mainnetValidators[0].nodeID)
	require.NoError(err)

	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{mainnetPChainHeight: set})
	ballot := Ballot{
		NodeID:       nodeID,
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: 1, // one past what the P-chain has finalized
		Weight:       mainnetValidators[0].weight,
		TotalWeight:  2_500_000_000_000_000_000,
	}
	err = NewConfig(utils.NewUint64(0)).VerifyPredicate(&precompileconfig.PredicateContext{
		ConsensusCtx:       ctx,
		ProposerVMBlockCtx: &chain.Context{PChainHeight: mainnetPChainHeight},
	}, predicateBytes(&ballot))
	require.ErrorIs(err, errSnapshotInFuture)
}

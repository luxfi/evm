// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"context"
	"testing"

	"github.com/luxfi/constants"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/evm/precompile/precompiletest"
	"github.com/luxfi/evm/predicate"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/evm/utils/utilstest"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
	consensuscontext "github.com/luxfi/runtime"
	validators "github.com/luxfi/validators"
	"github.com/luxfi/validators/validatorstest"
	"github.com/luxfi/vm/chain"
	"github.com/stretchr/testify/require"
)

// Heights used throughout: the snapshot a proposal pins, and a later height at
// which ballots land.
const (
	snapshotHeight uint64 = 100
	blockHeight    uint64 = 120
)

var (
	// Mainnet-shaped numbers, measured 2026-07 from
	// platform.getCurrentValidators on https://api.lux.network/ext/bc/P:
	// 5 validators, each weight 5e17 nLUX, total 2.5e18.
	mainnetVdrWeight   uint64 = 500_000_000_000_000_000
	mainnetTotalWeight uint64 = 2_500_000_000_000_000_000

	voterAddr = common.HexToAddress("0x9011E888251AB053B7bD1cdB598Db4f9DEd94714")
)

type testValidator struct {
	nodeID  ids.NodeID
	signer  *localsigner.LocalSigner
	pkBytes []byte
}

func newTestValidator(t testing.TB) *testValidator {
	t.Helper()
	sk, err := localsigner.New()
	require.NoError(t, err)
	return &testValidator{
		nodeID:  ids.GenerateTestNodeID(),
		signer:  sk,
		pkBytes: bls.PublicKeyToCompressedBytes(sk.PublicKey()),
	}
}

func (v *testValidator) output(weight uint64) *validators.GetValidatorOutput {
	return &validators.GetValidatorOutput{
		NodeID:    v.nodeID,
		PublicKey: v.pkBytes,
		Weight:    weight,
	}
}

type vdrSet map[ids.NodeID]*validators.GetValidatorOutput

// setsByHeight backs a validator state that answers differently per height —
// the whole point of snapshot semantics.
func newConsensusCtx(t testing.TB, chainID ids.ID, setsByHeight map[uint64]vdrSet) context.Context {
	t.Helper()
	base := &validatorstest.State{
		GetValidatorSetF: func(_ context.Context, height uint64, netID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			require.Equal(t, constants.PrimaryNetworkID, netID, "stake weight must read the PRIMARY NETWORK set")
			set, ok := setsByHeight[height]
			if !ok {
				return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
			}
			return set, nil
		},
	}
	rt := utilstest.NewTestRuntime(t, chainID)
	rt.ValidatorState = utilstest.NewTestValidatorStateFromBase(base)
	return consensuscontext.WithContext(context.Background(), rt)
}

// signedBallot returns a ballot whose authorisation signature is valid for
// (chainID, nodeID, voter, authEpoch).
func signedBallot(t testing.TB, v *testValidator, chainID ids.ID, b Ballot) *Ballot {
	t.Helper()
	b.NodeID = v.nodeID
	sig, err := v.signer.Sign(b.SigningBytes(chainID))
	require.NoError(t, err)
	copy(b.Signature[:], bls.SignatureToBytes(sig))
	return &b
}

func predicateBytes(b *Ballot) []byte {
	return predicate.PackPredicate(b.Bytes())
}

func newTest(ctx context.Context, b *Ballot, expectedErr error) precompiletest.PredicateTest {
	return precompiletest.PredicateTest{
		Config: NewConfig(utils.NewUint64(0)),
		PredicateContext: &precompileconfig.PredicateContext{
			ConsensusCtx:       ctx,
			ProposerVMBlockCtx: &chain.Context{PChainHeight: blockHeight},
		},
		PredicateBytes: predicateBytes(b),
		Gas:            PredicateGasCost,
		ExpectedErr:    expectedErr,
	}
}

func TestVerifyPredicate(t *testing.T) {
	chainID := ids.GenerateTestID()
	vdr := newTestValidator(t)
	other := newTestValidator(t)

	// The primary-network set at both heights. Every validator's weight ALREADY
	// includes the LUX delegated to it — the P-chain folds each delegator's
	// weight into the validator's single entry (node
	// vms/platformvm/state/state_validators.go: AddStaker(own stake) followed by
	// AddWeight(delegator weight) under the same nodeID). There is exactly one
	// map entry per nodeID, so a delegation cannot be counted twice.
	steady := vdrSet{
		vdr.nodeID:   vdr.output(mainnetVdrWeight),
		other.nodeID: other.output(mainnetTotalWeight - mainnetVdrWeight),
	}
	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{
		snapshotHeight: steady,
		blockHeight:    steady,
	})

	valid := Ballot{
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: snapshotHeight,
		Weight:       mainnetVdrWeight,
		TotalWeight:  mainnetTotalWeight,
	}

	tests := map[string]precompiletest.PredicateTest{
		"valid ballot": newTest(ctx, signedBallot(t, vdr, chainID, valid), nil),

		"under-claiming is allowed": newTest(ctx, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.Weight = 1
			return b
		}()), nil),

		"claiming more than the snapshot weight": newTest(ctx, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.Weight = mainnetVdrWeight + 1
			return b
		}()), errWeightOverstated),

		"zero weight": newTest(ctx, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.Weight = 0
			return b
		}()), errZeroWeight),

		"understated total inflates the share": newTest(ctx, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.TotalWeight = mainnetVdrWeight
			return b
		}()), errTotalWeightMismatch),

		"snapshot height in the future": newTest(ctx, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.PChainHeight = blockHeight + 1
			return b
		}()), errSnapshotInFuture),

		"nodeID absent at the snapshot height": newTest(ctx, signedBallot(t, newTestValidator(t), chainID, valid), errNotAValidator),

		// Signature bound to a different voter address: an attacker who copies a
		// ballot cannot re-point it at itself.
		"signature does not cover this voter": newTest(ctx, func() *Ballot {
			b := signedBallot(t, vdr, chainID, valid)
			b.Voter = common.HexToAddress("0xdead")
			return b
		}(), errUnauthorizedVoter),

		"signature does not cover this epoch": newTest(ctx, func() *Ballot {
			b := signedBallot(t, vdr, chainID, valid)
			b.AuthEpoch = 2
			return b
		}(), errUnauthorizedVoter),

		"signature minted for another chain": newTest(ctx, signedBallot(t, vdr, ids.GenerateTestID(), valid), errUnauthorizedVoter),

		"signature by a different validator's key": newTest(ctx, func() *Ballot {
			b := signedBallot(t, other, chainID, valid)
			b.NodeID = vdr.nodeID // claim vdr's weight with other's signature
			return b
		}(), errUnauthorizedVoter),

		"garbage signature": newTest(ctx, func() *Ballot {
			b := signedBallot(t, vdr, chainID, valid)
			b.Signature = [bls.SignatureLen]byte{}
			return b
		}(), errInvalidSignature),
	}
	precompiletest.RunPredicateTests(t, tests)
}

// A validator that has left the set, or whose weight shrank because a
// delegation expired, cannot vote the weight it had at the snapshot.
func TestVerifyPredicateLiveness(t *testing.T) {
	chainID := ids.GenerateTestID()
	vdr := newTestValidator(t)
	other := newTestValidator(t)

	snapshot := vdrSet{
		vdr.nodeID:   vdr.output(mainnetVdrWeight),
		other.nodeID: other.output(mainnetTotalWeight - mainnetVdrWeight),
	}

	valid := Ballot{
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: snapshotHeight,
		Weight:       mainnetVdrWeight,
		TotalWeight:  mainnetTotalWeight,
	}

	exited := newConsensusCtx(t, chainID, map[uint64]vdrSet{
		snapshotHeight: snapshot,
		blockHeight:    {other.nodeID: other.output(mainnetTotalWeight - mainnetVdrWeight)},
	})
	// endTime passed / stake withdrawn between the snapshot and the ballot.
	shrunk := newConsensusCtx(t, chainID, map[uint64]vdrSet{
		snapshotHeight: snapshot,
		blockHeight: {
			vdr.nodeID:   vdr.output(mainnetVdrWeight / 5), // delegations expired
			other.nodeID: other.output(mainnetTotalWeight - mainnetVdrWeight),
		},
	})
	// Stake ADDED after the snapshot must not count: the snapshot bound applies.
	flash := newConsensusCtx(t, chainID, map[uint64]vdrSet{
		snapshotHeight: snapshot,
		blockHeight: {
			vdr.nodeID:   vdr.output(mainnetVdrWeight * 4),
			other.nodeID: other.output(mainnetTotalWeight - mainnetVdrWeight),
		},
	})

	tests := map[string]precompiletest.PredicateTest{
		"validator exited before the ballot landed": newTest(exited, signedBallot(t, vdr, chainID, valid), errValidatorNotLive),

		"weight shrank after the snapshot": newTest(shrunk, signedBallot(t, vdr, chainID, valid), errWeightOverstated),

		"shrunk validator may still vote its live weight": newTest(shrunk, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.Weight = mainnetVdrWeight / 5
			return b
		}()), nil),

		"flash stake cannot exceed the snapshot": newTest(flash, signedBallot(t, vdr, chainID, func() Ballot {
			b := valid
			b.Weight = mainnetVdrWeight * 4
			return b
		}()), errWeightOverstated),

		"flash staker still votes only its snapshot weight": newTest(flash, signedBallot(t, vdr, chainID, valid), nil),
	}
	precompiletest.RunPredicateTests(t, tests)
}

// A ballot whose snapshot height equals the block height must take the
// single-lookup path and reach the same verdict.
func TestVerifyPredicateSameHeight(t *testing.T) {
	chainID := ids.GenerateTestID()
	vdr := newTestValidator(t)
	set := vdrSet{vdr.nodeID: vdr.output(mainnetVdrWeight)}
	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{blockHeight: set})

	b := signedBallot(t, vdr, chainID, Ballot{
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: blockHeight,
		Weight:       mainnetVdrWeight,
		TotalWeight:  mainnetVdrWeight,
	})
	newTest(ctx, b, nil).Run(t)
}

func TestVerifyPredicateContextFailures(t *testing.T) {
	require := require.New(t)
	chainID := ids.GenerateTestID()
	vdr := newTestValidator(t)
	set := vdrSet{vdr.nodeID: vdr.output(mainnetVdrWeight)}
	b := signedBallot(t, vdr, chainID, Ballot{
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: snapshotHeight,
		Weight:       mainnetVdrWeight,
		TotalWeight:  mainnetVdrWeight,
	})
	cfg := NewConfig(utils.NewUint64(0))

	// No ProposerVM context => no agreed P-chain height => refuse.
	err := cfg.VerifyPredicate(&precompileconfig.PredicateContext{
		ConsensusCtx: newConsensusCtx(t, chainID, map[uint64]vdrSet{snapshotHeight: set, blockHeight: set}),
	}, predicateBytes(b))
	require.ErrorIs(err, errMissingProposerVMCtx)

	// No validator state in the consensus context => refuse.
	rt := utilstest.NewTestRuntime(t, chainID)
	rt.ValidatorState = nil
	err = cfg.VerifyPredicate(&precompileconfig.PredicateContext{
		ConsensusCtx:       consensuscontext.WithContext(context.Background(), rt),
		ProposerVMBlockCtx: &chain.Context{PChainHeight: blockHeight},
	}, predicateBytes(b))
	require.ErrorIs(err, errNoValidatorState)

	// Unknown chain ID => the signing domain is unknown => fail closed rather
	// than verify against a degenerate domain another chain could also produce.
	err = cfg.VerifyPredicate(&precompileconfig.PredicateContext{
		ConsensusCtx:       newConsensusCtx(t, ids.Empty, map[uint64]vdrSet{snapshotHeight: set, blockHeight: set}),
		ProposerVMBlockCtx: &chain.Context{PChainHeight: blockHeight},
	}, predicateBytes(b))
	require.ErrorIs(err, errUnknownChainID)
}

func TestPredicateGas(t *testing.T) {
	require := require.New(t)
	cfg := NewConfig(utils.NewUint64(0))

	gas, err := cfg.PredicateGas(predicate.PackPredicate(make([]byte, BallotLen)))
	require.NoError(err)
	require.Equal(PredicateGasCost, gas)
	require.Equal(uint64(400_000), PredicateGasCost)

	_, err = cfg.PredicateGas(predicate.PackPredicate(make([]byte, BallotLen-1)))
	require.ErrorIs(err, errInvalidBallotLen)

	_, err = cfg.PredicateGas(make([]byte, 32)) // all zero: no end delimiter
	require.ErrorIs(err, errInvalidPredicateBytes)
}

// A validator with no registered BLS key cannot authorise anyone: there is no
// key to check against, and inventing a fallback would be a back door.
func TestValidatorWithoutPublicKey(t *testing.T) {
	chainID := ids.GenerateTestID()
	vdr := newTestValidator(t)
	out := vdr.output(mainnetVdrWeight)
	out.PublicKey = nil
	ctx := newConsensusCtx(t, chainID, map[uint64]vdrSet{
		snapshotHeight: {vdr.nodeID: out},
		blockHeight:    {vdr.nodeID: out},
	})
	b := signedBallot(t, vdr, chainID, Ballot{
		Voter:        voterAddr,
		AuthEpoch:    1,
		PChainHeight: snapshotHeight,
		Weight:       mainnetVdrWeight,
		TotalWeight:  mainnetVdrWeight,
	})
	newTest(ctx, b, errNoRegisteredPublicKey).Run(t)
}

// The total is summed with overflow checking: the P-chain itself cannot
// represent a larger total, so wrapping would silently mint voting power.
func TestTotalWeightOverflow(t *testing.T) {
	a, b := newTestValidator(t), newTestValidator(t)
	_, err := totalWeight(vdrSet{
		a.nodeID: a.output(1 << 63),
		b.nodeID: b.output(1 << 63),
	})
	require.ErrorIs(t, err, errTotalWeightOverflow)
}

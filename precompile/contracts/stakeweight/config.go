// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package stakeweight makes P-chain stake — a validator's own stake plus every
// LUX delegated to it — readable as a vote weight inside the C-chain EVM.
//
// # Why a predicate and not a plain precompile call
//
// A stateful precompile's Run() may only read values every node already agrees
// on. Its BlockContext (precompile/contract/interfaces.go) exposes exactly
// Number(), Timestamp() and GetPredicateResults() — no P-chain height. Reading
// the validator set from Run() would therefore have to use the node's CURRENT
// P-chain height, which comes from the last block that node happens to have
// accepted on the P-chain (vms/platformvm/validators/manager.go getCurrentHeight
// -> state.GetLastAccepted). Two honest nodes executing the same C-chain block a
// second apart would read different weights, produce different state roots and
// fork the chain. So the naive "validator-set precompile" is not merely
// imprecise, it is consensus-fatal.
//
// The predicate path is the one place where a P-chain height IS agreed:
// Block.VerifyWithContext receives the ProposerVM block context and passes
// ProposerVMBlockCtx.PChainHeight to VerifyPredicate (plugin/evm/block.go). Every
// node re-verifying that block uses the same height, historical validator sets
// are reconstructible from P-chain weight diffs, and the pass/fail bitset is
// written into the header and re-checked on replay (miner/worker.go ->
// plugin/evm/block.go errInvalidHeaderPredicateResults). A verified ballot is
// therefore a consensus-committed fact, not an oracle reading.
//
// # What the predicate proves
//
// The ballot CLAIMS a weight; the predicate checks the claim against P-chain
// state at two heights and refuses any overstatement:
//
//	weight <= weightAt(ballot.pChainHeight)     — no flash-stake: you cannot claim
//	                                              stake you did not have at the
//	                                              snapshot the proposal pinned.
//	weight <= weightAt(block.pChainHeight)      — no exited/shrunken validator: you
//	                                              cannot claim stake you no longer
//	                                              have when the ballot lands. A
//	                                              validator absent from the live set
//	                                              has weight 0 and cannot vote at all.
//	totalWeight == totalWeightAt(snapshot)      — the tally denominator is exact, so
//	                                              nobody can shrink it to inflate a share.
//
// Under-claiming is always allowed and only ever hurts the claimer, so a voter
// never has to predict which block their transaction lands in.
package stakeweight

import (
	"errors"
	"fmt"

	"github.com/luxfi/constants"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/evm/predicate"
	"github.com/luxfi/ids"
	"github.com/luxfi/math"
	consensuscontext "github.com/luxfi/runtime"
	validators "github.com/luxfi/validators"
)

var (
	_ precompileconfig.Config     = (*Config)(nil)
	_ precompileconfig.Predicater = (*Config)(nil)
)

const (
	// BallotVerifyGasCost covers one BLS signature verification. Same figure the
	// warp precompile charges for the same operation
	// (warp.GasCostPerSignatureVerification).
	BallotVerifyGasCost uint64 = 200_000
	// ValidatorSetLookupGasCost covers ONE validator-set materialisation. A
	// ballot performs at most two (snapshot height and block height), and the
	// second is skipped when they coincide — but gas must not depend on that,
	// or it would depend on the block's P-chain height, which the sender cannot
	// know when it signs. Charged unconditionally: gas is a property of the
	// ballot alone.
	ValidatorSetLookupGasCost uint64 = 100_000
	// PredicateGasCost is the whole, fixed price of verifying one ballot.
	PredicateGasCost = BallotVerifyGasCost + 2*ValidatorSetLookupGasCost
)

var (
	errInvalidPredicateBytes  = errors.New("cannot unpack stake-weight predicate bytes")
	errMissingProposerVMCtx   = errors.New("stake-weight predicate requires the ProposerVM block context")
	errNoValidatorState       = errors.New("validator state not found in consensus context")
	errUnknownChainID         = errors.New("chain ID absent from consensus context")
	errSnapshotInFuture       = errors.New("ballot pChainHeight exceeds the block's P-chain height")
	errZeroWeight             = errors.New("ballot claims zero weight")
	errCannotRetrieveSnapshot = errors.New("cannot retrieve validator set at snapshot height")
	errCannotRetrieveLiveSet  = errors.New("cannot retrieve validator set at block height")
	errNotAValidator          = errors.New("nodeID is not a primary-network validator at the snapshot height")
	errValidatorNotLive       = errors.New("nodeID is no longer a primary-network validator")
	errWeightOverstated       = errors.New("ballot claims more weight than the nodeID holds")
	errTotalWeightMismatch    = errors.New("ballot total weight does not match the primary-network total")
	errTotalWeightOverflow    = errors.New("primary-network total weight overflows uint64")
	errNoRegisteredPublicKey  = errors.New("validator has no registered BLS public key")
	errInvalidPublicKey       = errors.New("cannot parse the validator's registered BLS public key")
	errInvalidSignature       = errors.New("cannot parse the ballot signature")
	errUnauthorizedVoter      = errors.New("ballot signature does not authorize this voter for this nodeID")
)

// Config activates the stake-weight precompile at a block timestamp. It carries
// no parameters: there is nothing to tune, nobody to name, and no key to hold.
// Anyone who is a validator can vote its weight; nobody can grant or revoke that.
type Config struct {
	precompileconfig.Upgrade
}

// NewConfig returns a config enabling stakeweight at [blockTimestamp].
func NewConfig(blockTimestamp *uint64) *Config {
	return &Config{Upgrade: precompileconfig.Upgrade{BlockTimestamp: blockTimestamp}}
}

// NewDisableConfig returns a config disabling stakeweight at [blockTimestamp].
func NewDisableConfig(blockTimestamp *uint64) *Config {
	return &Config{Upgrade: precompileconfig.Upgrade{BlockTimestamp: blockTimestamp, Disable: true}}
}

// Key returns the upgrade.json key for this precompile.
func (*Config) Key() string { return ConfigKey }

// Verify is called at startup; there is no configuration to reject.
func (*Config) Verify(precompileconfig.ChainConfig) error { return nil }

// Equal returns true if [other] configures this precompile identically.
func (c *Config) Equal(other precompileconfig.Config) bool {
	otherCfg, ok := other.(*Config)
	if !ok {
		return false
	}
	return c.Upgrade.Equal(&otherCfg.Upgrade)
}

// PredicateGas charges a fixed price: a ballot is fixed-width, so its
// verification cost is a constant and cannot be gamed by padding.
func (*Config) PredicateGas(predicateBytes []byte) (uint64, error) {
	unpacked, err := predicate.UnpackPredicate(predicateBytes)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", errInvalidPredicateBytes, err)
	}
	if len(unpacked) != BallotLen {
		return 0, fmt.Errorf("%w: got %d, want %d", errInvalidBallotLen, len(unpacked), BallotLen)
	}
	return PredicateGasCost, nil
}

// VerifyPredicate returns nil iff the ballot's claims hold against P-chain state.
// A non-nil error marks the ballot invalid for this transaction; it never
// invalidates the block (core/predicate_check.go records failures in a bitset).
func (*Config) VerifyPredicate(predicateContext *precompileconfig.PredicateContext, predicateBytes []byte) error {
	unpacked, err := predicate.UnpackPredicate(predicateBytes)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidPredicateBytes, err)
	}
	ballot, err := ParseBallot(unpacked)
	if err != nil {
		return err
	}
	if predicateContext.ProposerVMBlockCtx == nil {
		return errMissingProposerVMCtx
	}
	blockPChainHeight := predicateContext.ProposerVMBlockCtx.PChainHeight

	// A ballot may look backwards at any finalized P-chain height, never forwards:
	// a future height is not yet agreed and GetValidatorSet would answer
	// differently on different nodes.
	if ballot.PChainHeight > blockPChainHeight {
		return fmt.Errorf("%w: ballot %d > block %d", errSnapshotInFuture, ballot.PChainHeight, blockPChainHeight)
	}
	if ballot.Weight == 0 {
		return errZeroWeight
	}

	ctx := predicateContext.ConsensusCtx
	validatorState := consensuscontext.GetValidatorState(ctx)
	if validatorState == nil {
		return errNoValidatorState
	}

	snapshot, err := validatorState.GetValidatorSet(ctx, ballot.PChainHeight, constants.PrimaryNetworkID)
	if err != nil {
		return fmt.Errorf("%w: %w", errCannotRetrieveSnapshot, err)
	}
	vdr, ok := snapshot[ballot.NodeID]
	if !ok {
		return fmt.Errorf("%w: %s at height %d", errNotAValidator, ballot.NodeID, ballot.PChainHeight)
	}
	if ballot.Weight > vdr.Weight {
		return fmt.Errorf("%w: claimed %d > snapshot %d", errWeightOverstated, ballot.Weight, vdr.Weight)
	}

	// The denominator every tally divides by must be exact, or a voter could
	// understate it and inflate its own share.
	total, err := totalWeight(snapshot)
	if err != nil {
		return err
	}
	if ballot.TotalWeight != total {
		return fmt.Errorf("%w: claimed %d, actual %d", errTotalWeightMismatch, ballot.TotalWeight, total)
	}

	// Liveness: stake that has left the validator set cannot vote, and stake that
	// shrank between the snapshot and this block votes only at its current size.
	liveSet := snapshot
	if blockPChainHeight != ballot.PChainHeight {
		liveSet, err = validatorState.GetValidatorSet(ctx, blockPChainHeight, constants.PrimaryNetworkID)
		if err != nil {
			return fmt.Errorf("%w: %w", errCannotRetrieveLiveSet, err)
		}
	}
	liveVdr, ok := liveSet[ballot.NodeID]
	if !ok {
		return fmt.Errorf("%w: %s at height %d", errValidatorNotLive, ballot.NodeID, blockPChainHeight)
	}
	if ballot.Weight > liveVdr.Weight {
		return fmt.Errorf("%w: claimed %d > live %d", errWeightOverstated, ballot.Weight, liveVdr.Weight)
	}

	// Authorisation. The key is the one the P-chain registered for this nodeID at
	// the snapshot height — no separate registry exists, so none can be captured.
	chainID := consensuscontext.GetChainID(ctx)
	if chainID == ids.Empty {
		// Fail closed: verifying a signature under an unknown domain would let an
		// authorisation minted for another chain pass here.
		return errUnknownChainID
	}
	return verifyAuthorization(vdr, ballot, chainID)
}

func verifyAuthorization(vdr *validators.GetValidatorOutput, ballot *Ballot, chainID ids.ID) error {
	if len(vdr.PublicKey) == 0 {
		return fmt.Errorf("%w: %s", errNoRegisteredPublicKey, ballot.NodeID)
	}
	pk, err := parsePublicKey(vdr.PublicKey)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidPublicKey, err)
	}
	sig, err := bls.SignatureFromBytes(ballot.Signature[:])
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidSignature, err)
	}
	if !bls.Verify(pk, sig, ballot.SigningBytes(chainID)) {
		return fmt.Errorf("%w: nodeID %s voter %s epoch %d", errUnauthorizedVoter, ballot.NodeID, ballot.Voter, ballot.AuthEpoch)
	}
	return nil
}

// parsePublicKey accepts either encoding the validator manager may hold. The
// P-chain stores compressed keys (bls.PublicKeyLen), but AddStaker is called
// with PublicKeyToUncompressedBytes, which is a distinct 96-byte encoding on
// the cgo build. Accepting both keeps this correct on either build rather than
// silently rejecting every validator on one of them.
func parsePublicKey(b []byte) (*bls.PublicKey, error) {
	if len(b) == bls.PublicKeyLen {
		return bls.PublicKeyFromCompressedBytes(b)
	}
	if pk := bls.PublicKeyFromValidUncompressedBytes(b); pk != nil {
		return pk, nil
	}
	return nil, fmt.Errorf("unsupported public key encoding (%d bytes)", len(b))
}

// totalWeight sums a validator set with overflow checking. The P-chain itself
// represents total stake as a uint64 (validators.Manager.TotalWeight), so an
// overflow here is an impossible state on the P-chain and must fail identically
// on every node rather than wrap.
func totalWeight(set map[ids.NodeID]*validators.GetValidatorOutput) (uint64, error) {
	var (
		total    uint64
		overflow bool
	)
	for _, vdr := range set {
		total, overflow = math.SafeAdd(total, vdr.Weight)
		if overflow {
			return 0, errTotalWeightOverflow
		}
	}
	return total, nil
}

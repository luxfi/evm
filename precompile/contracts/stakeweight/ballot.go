// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
)

// SigningDomain separates a stake-weight authorization from every other BLS
// signature a validator's registered key ever produces (warp Beam signatures,
// proof-of-possession, Pulsar shares). A validator's P-chain `signer` key is
// reused here on purpose — it is the ONE identity the P-chain already binds to
// a nodeID, so authorising an EVM address needs no new registry, no admin and
// no key ceremony.
const SigningDomain = "lux-stake-weight-authorization-v1"

// Wire layout of a Ballot. Fixed width: an attestation has no variable-length
// field, so parsing is total and gas is a constant.
//
//	[  0: 20)  nodeID        ids.NodeID
//	[ 20: 40)  voter         common.Address  — the EVM address authorised to speak for nodeID
//	[ 40: 48)  authEpoch     uint64 BE       — monotone authorisation sequence number
//	[ 48: 56)  pChainHeight  uint64 BE       — P-chain height the weight is claimed AT
//	[ 56: 64)  weight        uint64 BE       — claimed vote weight
//	[ 64: 72)  totalWeight   uint64 BE       — claimed total primary-network weight at pChainHeight
//	[ 72:168)  signature     96 bytes        — BLS over SigningBytes(chainID)
const (
	nodeIDOffset       = 0
	voterOffset        = nodeIDOffset + ids.NodeIDLen
	authEpochOffset    = voterOffset + common.AddressLength
	pChainHeightOffset = authEpochOffset + 8
	weightOffset       = pChainHeightOffset + 8
	totalWeightOffset  = weightOffset + 8
	signatureOffset    = totalWeightOffset + 8
	// BallotLen is the exact, only accepted length of an unpacked predicate.
	BallotLen = signatureOffset + bls.SignatureLen
)

var errInvalidBallotLen = errors.New("invalid stake-weight ballot length")

// Ballot is a claim that `NodeID` had `Weight` of a `TotalWeight`
// primary-network stake at P-chain height `PChainHeight`, cast by `Voter`,
// who holds a BLS authorisation from nodeID's registered signer key.
//
// Nothing in a Ballot is trusted. VerifyPredicate checks every field against
// P-chain state at a consensus-agreed height; a ballot that overstates any of
// them is marked invalid, not merely ignored.
type Ballot struct {
	NodeID       ids.NodeID
	Voter        common.Address
	AuthEpoch    uint64
	PChainHeight uint64
	Weight       uint64
	TotalWeight  uint64
	Signature    [bls.SignatureLen]byte
}

// ParseBallot decodes b. Length is the only structural rule.
func ParseBallot(b []byte) (*Ballot, error) {
	if len(b) != BallotLen {
		return nil, fmt.Errorf("%w: got %d, want %d", errInvalidBallotLen, len(b), BallotLen)
	}
	ballot := &Ballot{
		AuthEpoch:    binary.BigEndian.Uint64(b[authEpochOffset:pChainHeightOffset]),
		PChainHeight: binary.BigEndian.Uint64(b[pChainHeightOffset:weightOffset]),
		Weight:       binary.BigEndian.Uint64(b[weightOffset:totalWeightOffset]),
		TotalWeight:  binary.BigEndian.Uint64(b[totalWeightOffset:signatureOffset]),
	}
	copy(ballot.NodeID[:], b[nodeIDOffset:voterOffset])
	copy(ballot.Voter[:], b[voterOffset:authEpochOffset])
	copy(ballot.Signature[:], b[signatureOffset:])
	return ballot, nil
}

// Bytes encodes the ballot in the wire layout above.
func (b *Ballot) Bytes() []byte {
	out := make([]byte, BallotLen)
	copy(out[nodeIDOffset:], b.NodeID[:])
	copy(out[voterOffset:], b.Voter[:])
	binary.BigEndian.PutUint64(out[authEpochOffset:], b.AuthEpoch)
	binary.BigEndian.PutUint64(out[pChainHeightOffset:], b.PChainHeight)
	binary.BigEndian.PutUint64(out[weightOffset:], b.Weight)
	binary.BigEndian.PutUint64(out[totalWeightOffset:], b.TotalWeight)
	copy(out[signatureOffset:], b.Signature[:])
	return out
}

// SigningBytes returns the message the validator's BLS key signs.
//
// It deliberately covers ONLY (domain, chainID, nodeID, voter, authEpoch) — the
// standing authorisation "this EVM address speaks for me". It does NOT cover
// pChainHeight, weight or totalWeight, because those are not the validator's to
// assert: they are read from P-chain state during predicate verification. One
// signature therefore authorises an address once and is reusable for every
// later proposal, and rotating authority is a strictly greater authEpoch — a
// sequence number the consuming contract enforces, with no revocation registry
// and nobody able to revoke on the validator's behalf.
//
// chainID is the blockchainID of the EVM chain the ballot is cast on, so an
// authorisation minted for one Lux EVM chain cannot be replayed on another.
func (b *Ballot) SigningBytes(chainID ids.ID) []byte {
	msg := make([]byte, 0, len(SigningDomain)+ids.IDLen+ids.NodeIDLen+common.AddressLength+8)
	msg = append(msg, SigningDomain...)
	msg = append(msg, chainID[:]...)
	msg = append(msg, b.NodeID[:]...)
	msg = append(msg, b.Voter[:]...)
	msg = binary.BigEndian.AppendUint64(msg, b.AuthEpoch)
	return msg
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestBallotRoundTrip(t *testing.T) {
	require := require.New(t)

	in := &Ballot{
		NodeID:       ids.GenerateTestNodeID(),
		Voter:        common.HexToAddress("0x9011E888251AB053B7bD1cdB598Db4f9DEd94714"),
		AuthEpoch:    7,
		PChainHeight: 1_098_191,
		Weight:       500_000_000_000_000_000,
		TotalWeight:  2_500_000_000_000_000_000,
	}
	for i := range in.Signature {
		in.Signature[i] = byte(i)
	}

	encoded := in.Bytes()
	require.Len(encoded, BallotLen)
	require.Equal(168, BallotLen, "wire layout must not drift silently")

	out, err := ParseBallot(encoded)
	require.NoError(err)
	require.Equal(in, out)
}

func TestParseBallotRejectsWrongLength(t *testing.T) {
	for _, n := range []int{0, BallotLen - 1, BallotLen + 1, bls.SignatureLen} {
		_, err := ParseBallot(make([]byte, n))
		require.ErrorIs(t, err, errInvalidBallotLen, "length %d", n)
	}
}

// The signed statement must separate authorisations by chain, by node, by voter
// and by epoch — otherwise one signature could be replayed onto another chain,
// another node's identity, another address, or a revoked epoch.
func TestSigningBytesSeparation(t *testing.T) {
	require := require.New(t)

	chainA := ids.GenerateTestID()
	chainB := ids.GenerateTestID()
	base := &Ballot{
		NodeID:    ids.GenerateTestNodeID(),
		Voter:     common.HexToAddress("0x01"),
		AuthEpoch: 1,
	}
	ref := base.SigningBytes(chainA)

	otherChain := base.SigningBytes(chainB)
	require.NotEqual(ref, otherChain)

	otherNode := *base
	otherNode.NodeID = ids.GenerateTestNodeID()
	require.NotEqual(ref, otherNode.SigningBytes(chainA))

	otherVoter := *base
	otherVoter.Voter = common.HexToAddress("0x02")
	require.NotEqual(ref, otherVoter.SigningBytes(chainA))

	otherEpoch := *base
	otherEpoch.AuthEpoch = 2
	require.NotEqual(ref, otherEpoch.SigningBytes(chainA))

	// The height and weight fields are NOT signed: one authorisation is reusable
	// for every later snapshot, and the numbers are checked against P-chain state
	// rather than asserted by the signer.
	otherHeight := *base
	otherHeight.PChainHeight = 999
	otherHeight.Weight = 999
	otherHeight.TotalWeight = 999
	require.Equal(ref, otherHeight.SigningBytes(chainA))

	require.Equal([]byte(SigningDomain), ref[:len(SigningDomain)])
}

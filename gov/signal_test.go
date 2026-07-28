// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import (
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/stretchr/testify/require"
)

// TestSignalRoundTrip pins the wire format. It is consensus-critical: a node
// that encodes or decodes these eight bytes differently from its peers counts a
// different election.
func TestSignalRoundTrip(t *testing.T) {
	for _, s := range []Signal{
		{ParamGasLimit, 12_000_000},         // the value mainnet runs today
		{ParamGasLimit, 100_000_000},        // upper bound
		{ParamGasLimit, 5_000_000},          // lower bound
		{ParamMinBaseFee, 25_000_000_000},   // the value mainnet runs today
		{ParamMinBaseFee, 500_000_000_000},  // upper bound, largest legal value
		{ParamMinBlockGasCost, 0},           // zero IS a legal value here
		{ParamTargetBlockRate, 2},           //
		{ParamBaseFeeChangeDenominator, 36}, //
		{ParamTargetGas, 15_000_000},        //
		{ParamBlockGasCostStep, 200_000},    //
		{ParamMaxBlockGasCost, 10_000_000},  //
	} {
		n, ok := s.Encode()
		require.Truef(t, ok, "%+v must encode", s)
		got, ok := DecodeSignal(n)
		require.Truef(t, ok, "%+v must decode", s)
		require.Equal(t, s, got)
	}
}

// TestZeroNonceIsAbstention is the measurement that makes this field usable.
// Every mainnet 96369 header sampled from genesis to 1098193 carries
// nonce 0x0000000000000000; if that decoded as anything but "no opinion",
// switching governance on would count a million phantom votes.
func TestZeroNonceIsAbstention(t *testing.T) {
	_, ok := DecodeSignal(types.BlockNonce{})
	require.False(t, ok, "the zero nonce every existing block carries must be an abstention")

	// And the same through the header path a node actually uses.
	_, ok = SignalFromHeader(&types.Header{})
	require.False(t, ok)
	_, ok = SignalFromHeader(nil)
	require.False(t, ok)
}

// TestOutOfBoundsIsNotASignal is the constitution enforced at the door: a value
// outside its compiled-in bound is not a losing vote, it is not a vote. A
// unanimous validator set cannot set a gas limit of zero because no encoding of
// that opinion is ever counted.
func TestOutOfBoundsIsNotASignal(t *testing.T) {
	tests := []struct {
		name string
		s    Signal
	}{
		{"gasLimit zero would halt the chain", Signal{ParamGasLimit, 0}},
		{"gasLimit one under the floor", Signal{ParamGasLimit, 4_999_999}},
		{"gasLimit one over the ceiling", Signal{ParamGasLimit, 100_000_001}},
		{"free transactions", Signal{ParamMinBaseFee, 0}},
		{"targetBlockRate zero divides by zero", Signal{ParamTargetBlockRate, 0}},
		{"baseFeeChangeDenominator zero divides by zero", Signal{ParamBaseFeeChangeDenominator, 0}},
		{"unknown parameter id", Signal{ParamID(200), 1}},
		{"parameter id zero", Signal{ParamID(0), 1}},
		{"value beyond the 48-bit field", Signal{ParamGasLimit, MaxValue + 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ok := tt.s.Encode()
			require.False(t, ok, "must refuse to encode")

			// And it is refused on the way in too, so a hand-crafted nonce that
			// skipped Encode cannot smuggle the value into a tally.
			forged := forge(tt.s)
			_, ok = DecodeSignal(forged)
			require.False(t, ok, "must refuse to decode a hand-crafted nonce")
		})
	}
}

// TestForeignNonceIsAbstention: bytes that are not ours are not ours. Only the
// magic byte admits a signal, so any other use of the field reads as silence.
func TestForeignNonceIsAbstention(t *testing.T) {
	legal, ok := Signal{ParamGasLimit, 12_000_000}.Encode()
	require.True(t, ok)

	for b := 0; b < 256; b++ {
		if byte(b) == signalMagic {
			continue
		}
		n := legal
		n[0] = byte(b)
		_, ok := DecodeSignal(n)
		require.Falsef(t, ok, "magic byte %#x must not be accepted", b)
	}
	// Positive control: with the magic byte restored, the same bytes ARE a signal.
	s, ok := DecodeSignal(legal)
	require.True(t, ok)
	require.Equal(t, Signal{ParamGasLimit, 12_000_000}, s)
}

// forge builds a nonce without going through Encode's checks, so tests can put
// an illegal opinion on the wire the way a hostile validator would.
func forge(s Signal) types.BlockNonce {
	var n types.BlockNonce
	n[0] = signalMagic
	n[1] = byte(s.ParamID)
	v := s.Value
	for i := 7; i >= 2; i-- {
		n[i] = byte(v)
		v >>= 8
	}
	return n
}

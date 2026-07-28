// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// window builds an EpochLength-long window in which [n] blocks carry [s] and
// the rest are silent, which is how an epoch actually looks.
func window(s Signal, n uint64) []Signal {
	w := make([]Signal, EpochLength)
	for i := uint64(0); i < n && i < EpochLength; i++ {
		w[i] = s
	}
	return w
}

// TestThresholdIsExact pins the boundary. 3/4 of 2016 is 1512 exactly; one
// block short must fail, because a threshold that is approximately enforced is
// not a threshold.
func TestThresholdIsExact(t *testing.T) {
	require.Equal(t, uint64(1512), Required(EpochLength))

	s := Signal{ParamGasLimit, 20_000_000}

	require.Empty(t, Tally(window(s, 1511)), "1511 of 2016 must not pass")

	got := Tally(window(s, 1512))
	require.Len(t, got, 1, "1512 of 2016 must pass")
	require.Equal(t, Decision{ParamGasLimit, 20_000_000, 1512}, got[0])
}

// TestAbstentionCountsAgainst is the property that makes it safe to leave this
// mechanism switched on forever. A near-empty epoch in which every block that
// spoke agreed still changes nothing: the denominator is the window, not the
// turnout. Without this, one validator producing the only three signalling
// blocks in a quiet hour would carry a unanimous 3-of-3 "election".
func TestAbstentionCountsAgainst(t *testing.T) {
	s := Signal{ParamGasLimit, 20_000_000}
	w := make([]Signal, EpochLength)
	for i := 0; i < 3; i++ {
		w[i] = s
	}
	require.Empty(t, Tally(w), "3 signals out of 2016 blocks is not a supermajority")

	// Positive control: the same three signals in a three-block window WOULD
	// pass, proving the emptiness above comes from the denominator and not from
	// a broken counter.
	require.Len(t, Tally([]Signal{s, s, s}), 1)
}

// TestSplitVoteFails: a plurality is not a supermajority, and disagreement does
// not average. Two camps at 1000 and 1016 blocks both want a bigger gas limit
// and neither gets one.
func TestSplitVoteFails(t *testing.T) {
	w := make([]Signal, EpochLength)
	for i := 0; i < 1000; i++ {
		w[i] = Signal{ParamGasLimit, 20_000_000}
	}
	for i := 1000; i < 2016; i++ {
		w[i] = Signal{ParamGasLimit, 30_000_000}
	}
	require.Empty(t, Tally(w), "1016 of 2016 is a plurality, not 3/4")
}

// TestOutOfBoundsNeverCounted: defence in depth. Even if a signal for an
// illegal value reached the tally — it cannot, DecodeSignal drops it first —
// counting it is refused here too. A cartel cannot widen a bound by agreeing.
func TestOutOfBoundsNeverCounted(t *testing.T) {
	w := make([]Signal, EpochLength)
	for i := range w {
		w[i] = Signal{ParamGasLimit, 0} // unanimous vote to halt the chain
	}
	require.Empty(t, Tally(w), "a unanimous illegal value must decide nothing")

	for i := range w {
		w[i] = Signal{ParamID(200), 1} // unanimous vote on a nonexistent parameter
	}
	require.Empty(t, Tally(w))
}

// TestAtMostOneChangePerEpoch is a structural consequence worth stating out
// loud, because it bounds how fast this system can move at all: a block carries
// exactly one signal, so two parameters would need 150% of an epoch's blocks to
// both clear 3/4. Governance is therefore serialized — one referendum per
// epoch, roughly one per hour on the C-Chain — no matter how many parameters a
// coalition wants to change or how large it is.
func TestAtMostOneChangePerEpoch(t *testing.T) {
	w := make([]Signal, EpochLength)
	for i := 0; i < 1008; i++ {
		w[i] = Signal{ParamGasLimit, 20_000_000}
	}
	for i := 1008; i < 2016; i++ {
		w[i] = Signal{ParamMinBaseFee, 50_000_000_000}
	}
	require.Empty(t, Tally(w), "a unanimous set split across two parameters passes neither")

	// Each half wins when it gets a window to itself.
	require.Len(t, Tally(window(Signal{ParamGasLimit, 20_000_000}, 1512)), 1)
	require.Len(t, Tally(window(Signal{ParamMinBaseFee, 50_000_000_000}, 1512)), 1)

	// Exhaustive: no window of any size can produce two decisions, because the
	// two counts would have to sum to more than the window.
	for size := 1; size <= 64; size++ {
		for split := 0; split <= size; split++ {
			w := make([]Signal, size)
			for i := 0; i < split; i++ {
				w[i] = Signal{ParamGasLimit, 20_000_000}
			}
			for i := split; i < size; i++ {
				w[i] = Signal{ParamMinBaseFee, 50_000_000_000}
			}
			require.LessOrEqualf(t, len(Tally(w)), 1,
				"size=%d split=%d produced more than one decision", size, split)
		}
	}
}

// TestTieCanNeverPass: the tie-break in Tally is provably unreachable under any
// threshold above one half — two values tied at c blocks need 2c <= window, so
// c <= window/2 < 3*window/4. It stays in the code so the function is total and
// its output order is defined, but no draw has ever decided anything and none
// can. There is no coin flip anywhere in this mechanism.
func TestTieCanNeverPass(t *testing.T) {
	for size := 2; size <= 200; size += 2 {
		w := make([]Signal, size)
		for i := 0; i < size/2; i++ {
			w[i] = Signal{ParamGasLimit, 30_000_000}
		}
		for i := size / 2; i < size; i++ {
			w[i] = Signal{ParamGasLimit, 20_000_000}
		}
		require.Emptyf(t, Tally(w), "a perfect draw at size %d must decide nothing", size)
	}
	// Positive control: break the draw by one block, past the threshold, and it
	// decides — so the emptiness above is the tie, not a dead counter.
	w := make([]Signal, 4)
	for i := range w {
		w[i] = Signal{ParamGasLimit, 20_000_000}
	}
	require.Len(t, Tally(w), 1)
}

// TestDeterministicOrder: the tally's output drives state writes, so it must
// not vary with Go's randomized map iteration. Same window, same bytes, every
// time.
func TestDeterministicOrder(t *testing.T) {
	w := make([]Signal, 16)
	for i := 0; i < 12; i++ { // 12 of 16 = exactly 3/4
		w[i] = Signal{ParamGasLimit, 20_000_000}
	}
	for i := 12; i < 16; i++ {
		w[i] = Signal{ParamGasLimit, 30_000_000}
	}
	first := Tally(w)
	require.Len(t, first, 1)
	for i := 0; i < 500; i++ {
		require.Equal(t, first, Tally(w))
	}
}

// TestRequiredRoundsUp: 3/4 of 10 is 7.5, and half a block does not exist. It
// must round toward more agreement, never less.
func TestRequiredRoundsUp(t *testing.T) {
	require.Equal(t, uint64(8), Required(10))
	require.Equal(t, uint64(3), Required(4))
	require.Equal(t, uint64(1), Required(1))
	require.Equal(t, uint64(1512), Required(2016))
}

// TestEmptyWindow decides nothing rather than panicking or deciding everything.
func TestEmptyWindow(t *testing.T) {
	require.Empty(t, Tally(nil))
	require.Empty(t, Tally([]Signal{}))
}

// TestEpochBoundaries: genesis closes no epoch, and the boundary is where the
// arithmetic says it is.
func TestEpochBoundaries(t *testing.T) {
	require.False(t, IsEpochEnd(0), "genesis closes nothing")
	require.False(t, IsEpochEnd(1))
	require.False(t, IsEpochEnd(EpochLength-1))
	require.True(t, IsEpochEnd(EpochLength))
	require.False(t, IsEpochEnd(EpochLength+1))
	require.True(t, IsEpochEnd(2*EpochLength))
	require.Equal(t, uint64(1), EpochOf(EpochLength))
	require.Equal(t, uint64(2), EpochOf(2*EpochLength))
}

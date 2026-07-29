// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import "sort"

// Decision is a parameter change that carried an epoch.
type Decision struct {
	ParamID ParamID
	Value   uint64
	Count   uint64 // blocks that signalled it, out of EpochLength
}

// Tally counts one epoch and returns what passed.
//
// It is a pure function of the window: the same blocks produce the same
// decisions on every node, in the same order, forever. That is the only
// property that matters, because the result is written into the state root.
//
// The rules, in full:
//
//   - [window] is the whole epoch, EpochLength blocks. A block that signals
//     nothing still occupies a slot, so a quiet epoch decides nothing. This is
//     BIP-9's rule and it is what makes the mechanism safe to leave switched on:
//     doing nothing is a vote against.
//   - Signals for a parameter outside the constitution, or for a value outside
//     its bound, are already gone — DecodeSignal dropped them. A cartel cannot
//     widen a bound by agreeing to.
//   - Within a parameter, the value with the most blocks wins, and needs
//     ThresholdNum/ThresholdDen of the WHOLE window to pass. Splitting a vote
//     across two values therefore fails; it does not average, and it does not
//     let a plurality through.
//   - Ties are broken by the lower value. The rule exists so the function is
//     total and its output order defined; it is also unreachable, since two
//     values tied at c blocks need 2c <= window and therefore c < 3*window/4.
//     No draw has ever decided anything here and none can — there is no coin
//     flip in this mechanism, and no casting vote for anyone to hold.
//   - Parameters are counted independently, but a block carries exactly one
//     signal, so two of them clearing 3/4 would need 150% of an epoch's blocks.
//     Governance is serialized by construction: at most one parameter changes
//     per epoch, however large or determined the coalition.
func Tally(window []Signal) []Decision {
	if len(window) == 0 {
		return nil
	}
	// (param, value) -> blocks
	counts := make(map[ParamID]map[uint64]uint64, len(Bounds))
	for _, s := range window {
		if !Governable(s.ParamID, s.Value) {
			continue // defence in depth; DecodeSignal already refused these
		}
		byValue, ok := counts[s.ParamID]
		if !ok {
			byValue = make(map[uint64]uint64, 2)
			counts[s.ParamID] = byValue
		}
		byValue[s.Value]++
	}

	need := required(uint64(len(window)))
	out := make([]Decision, 0, len(counts))
	for id, byValue := range counts {
		bestValue, bestCount := uint64(0), uint64(0)
		for v, c := range byValue {
			if c > bestCount || (c == bestCount && v < bestValue) {
				bestValue, bestCount = v, c
			}
		}
		if bestCount >= need {
			out = append(out, Decision{ParamID: id, Value: bestValue, Count: bestCount})
		}
	}
	// Map iteration is random; state writes are not allowed to be.
	sort.Slice(out, func(i, j int) bool { return out[i].ParamID < out[j].ParamID })
	return out
}

// required is the number of blocks a value needs to carry a window of [size].
// Ceiling division, so 3/4 of 2016 is 1512 and 3/4 of 10 is 8 — never a
// fraction of a block rounded in a cartel's favour.
func required(size uint64) uint64 {
	return (size*ThresholdNum + ThresholdDen - 1) / ThresholdDen
}

// Required exposes the threshold for a window of [size], for callers that want
// to state it rather than rediscover it.
func Required(size uint64) uint64 { return required(size) }

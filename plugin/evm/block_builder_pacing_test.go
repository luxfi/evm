// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"testing"
	"time"
)

// TestNextBuildTime locks in the block-build pacing decision (the fix for the
// network-wide block-production stall: building before parent.Time+TargetBlockRate
// raises the block fee above what a small-tip tx can cover, so verifyBlockFee
// rejects the block and the chain never advances).
//
// nextBuildTime returns the later of:
//   - earliestBuild (parent.Time + TargetBlockRate), and
//   - lastBuildTime + minBlockBuildingRetryDelay (retry floor, only when a block
//     was already built this session).
func TestNextBuildTime(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)

	tests := []struct {
		name          string
		earliestBuild time.Time
		lastBuildTime time.Time
		want          time.Time
	}{
		{
			// First build of the session (lastBuildTime zero): pace purely to the
			// target rate. This is the path every frozen chain takes on its first
			// post-bootstrap block — it MUST honor earliestBuild, never fire early.
			name:          "first build paces to earliestBuild",
			earliestBuild: base.Add(2 * time.Second),
			lastBuildTime: time.Time{},
			want:          base.Add(2 * time.Second),
		},
		{
			// earliestBuild already in the past + no prior build: build now
			// (returned time is in the past, so time.Until <= 0). This is the
			// healthy catch-up case (a chain idle longer than the block rate).
			name:          "first build with past earliestBuild is immediate",
			earliestBuild: base.Add(-time.Hour),
			lastBuildTime: time.Time{},
			want:          base.Add(-time.Hour),
		},
		{
			// Target rate dominates the retry floor: a freshly built block must
			// still wait the full target rate, not just the retry floor.
			name:          "target rate dominates retry floor",
			earliestBuild: base.Add(2 * time.Second),
			lastBuildTime: base.Add(-50 * time.Millisecond),
			want:          base.Add(2 * time.Second),
		},
		{
			// earliestBuild already passed but we JUST built: the retry floor
			// prevents a hot-loop. Returns lastBuildTime + retry delay.
			name:          "retry floor prevents hot-loop after earliestBuild passed",
			earliestBuild: base.Add(-time.Second),
			lastBuildTime: base,
			want:          base.Add(minBlockBuildingRetryDelay),
		},
		{
			// Exact boundary: retry floor equal to earliestBuild is NOT "after",
			// so earliestBuild is returned (deterministic tie-break, no drift).
			name:          "retry floor equal to earliestBuild yields earliestBuild",
			earliestBuild: base.Add(minBlockBuildingRetryDelay),
			lastBuildTime: base,
			want:          base.Add(minBlockBuildingRetryDelay),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBuildTime(tt.earliestBuild, tt.lastBuildTime)
			if !got.Equal(tt.want) {
				t.Fatalf("nextBuildTime(%v, %v) = %v, want %v",
					tt.earliestBuild, tt.lastBuildTime, got, tt.want)
			}
		})
	}
}

// TestNextBuildTime_NeverEarlierThanTargetRate is the property the stall fix
// depends on: regardless of the prior build time, the next build is NEVER
// scheduled before earliestBuild (the target-rate floor). If this ever returns
// a time before earliestBuild, a block could be built too soon, its fee would
// exceed a small-tip tx's budget, and the chain would stall again.
func TestNextBuildTime_NeverEarlierThanTargetRate(t *testing.T) {
	earliest := time.Unix(1_700_000_000, 0)
	for _, deltaMs := range []int{-10_000, -100, -1, 0, 1, 100, 10_000} {
		last := earliest.Add(time.Duration(deltaMs) * time.Millisecond)
		got := nextBuildTime(earliest, last)
		if got.Before(earliest) {
			t.Fatalf("nextBuildTime scheduled %v BEFORE earliestBuild %v (lastBuildTime=%v) — would re-introduce the fee stall",
				got, earliest, last)
		}
	}
	// And the zero-value (first build) case must equal earliestBuild exactly.
	if got := nextBuildTime(earliest, time.Time{}); !got.Equal(earliest) {
		t.Fatalf("first-build nextBuildTime = %v, want earliestBuild %v", got, earliest)
	}
}

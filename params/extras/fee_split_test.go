// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func u64(v uint64) *uint64 { return &v }

// TestIsFeeSplit verifies the activation gate: nil = never, 0 = genesis, and a
// concrete timestamp turns on at/after that time (never before). This is the
// deterministic predicate the state-transition seam reads to decide whether to
// split fees, so it must be exact at the boundary.
func TestIsFeeSplit(t *testing.T) {
	tests := []struct {
		name string
		ts   *uint64
		time uint64
		want bool
	}{
		{"nil never active at 0", nil, 0, false},
		{"nil never active later", nil, 1_000_000, false},
		{"genesis active at 0", u64(0), 0, true},
		{"genesis active later", u64(0), 5, true},
		{"scheduled inactive before", u64(100), 99, false},
		{"scheduled active at boundary", u64(100), 100, true},
		{"scheduled active after", u64(100), 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ChainConfig{FeeSplitTimestamp: tt.ts}
			require.Equal(t, tt.want, c.IsFeeSplit(tt.time))
		})
	}
}

// TestFeeSplitCompatibility guards against an operator silently changing the
// activation time: once the fork is (or would be) active at the head, its
// timestamp is frozen — a mismatch must be rejected, or nodes fork on fee
// disbursement. A still-future fork may be rescheduled, and equal values are
// always compatible.
func TestFeeSplitCompatibility(t *testing.T) {
	base := func(ts *uint64) *ChainConfig {
		return &ChainConfig{
			NetworkUpgrades:   GetDefaultNetworkUpgrades(),
			FeeSplitTimestamp: ts,
		}
	}
	tests := []struct {
		name      string
		stored    *uint64
		updated   *uint64
		head      uint64
		wantError bool
	}{
		{"unset stays unset", nil, nil, 500, false},
		{"equal active timestamps", u64(100), u64(100), 500, false},
		{"reschedule while both future", u64(100), u64(150), 50, false},
		{"change after activation rejected", u64(100), u64(150), 200, true},
		{"disable after activation rejected", u64(100), nil, 200, true},
		{"enable a still-future fork", nil, u64(300), 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := base(tt.stored).checkConfigCompatible(base(tt.updated), new(big.Int), tt.head)
			if tt.wantError {
				require.NotNil(t, err, "expected incompatibility error")
			} else {
				require.Nil(t, err, "expected compatible")
			}
		})
	}
}

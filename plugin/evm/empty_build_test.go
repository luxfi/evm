// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import "testing"

// An empty assembled block is TWO conditions wearing one shape, and the whole point of
// this rule is to tell them apart.
//
// Discarding an empty build is correct when nobody asked for anything — that is the
// demand-driven property, and without it an idle chain grows forever. It is a halt when
// the pool HAS work: no block means no nonce advances, so the pending transaction is
// never mined, so the pool never drains, so the condition that would allow a block can
// never return. The chain is then dead with a queue in front of it and every proposer
// logging errEmptyBlock, which is exactly how lux-devnet spent 41 hours on 2026-08-11.
//
// The gate used to read the assembled block, which cannot distinguish the two. It reads
// the pool now, which can.
func TestProposeEmptyBuild(t *testing.T) {
	for _, tc := range []struct {
		name       string
		pending    int
		automining bool
		want       bool
	}{
		{"idle chain: nothing pending, nothing to build", 0, false, false},
		{"one executable transaction the assembler missed", 1, false, true},
		{"a queue of them", 7100, false, true},
		{"automining builds regardless — that is what it is for", 0, true, true},
		{"automining with demand", 3, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := proposeEmptyBuild(tc.pending, tc.automining); got != tc.want {
				t.Fatalf("proposeEmptyBuild(pending=%d, automining=%v) = %v, want %v",
					tc.pending, tc.automining, got, tc.want)
			}
		})
	}
}

// The regression, stated as the property that was violated: a chain holding work must
// never refuse to build. Any pending count above zero has to propose, or the deadlock
// above is reachable again.
func TestPendingWorkAlwaysBuilds(t *testing.T) {
	for _, pending := range []int{1, 2, 10, 1_000, 7_100} {
		if !proposeEmptyBuild(pending, false) {
			t.Fatalf("refused to build with %d transactions pending — that is the 41-hour halt", pending)
		}
	}
}

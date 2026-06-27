// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"sync/atomic"

	"github.com/luxfi/geth/common"
)

// This file holds the address-level conflict-detection kernel and the package
// metrics. It is the shared primitive used by the DEX fill-settlement path
// (settlement.go) where fills touch coarse, disjoint account sets and an
// address-level edge set is exact. The EVM block path does NOT use this
// approximation; it uses the multi-version executor (executor.go) which records
// the ACTUAL per-location read/write sets observed during execution.

// txReadWriteSet records the address-level slots read and written by a unit of
// work (a DEX fill). Keyed by the hash of the address. Slot-level granularity is
// captured by the EVM executor's MVMemory, not here.
type txReadWriteSet struct {
	reads  map[common.Hash]common.Hash // slot -> value read
	writes map[common.Hash]common.Hash // slot -> value written
}

// detectConflicts is the conflict-detection kernel shared by the DEX-settlement
// path. It implements the exact predicate the GPU conflict_detect kernel
// enforces (gpu-kernels ops/cevm/{cuda,metal,wgsl}/conflict_detect, KAT
// byte-equal to this CPU oracle):
//
//	tx[i] conflicts with an earlier tx[j<i] iff
//	  W_j ∩ R_i ≠ ∅   OR   W_j ∩ W_i ≠ ∅   OR   R_j ∩ W_i ≠ ∅
//
// The single map (slot -> highest writer index) gives the same edge set as the
// O(N²) upper-triangle GPU sweep in O(Σ|rwSet|), and selecting "any earlier
// writer" yields the deterministic, transaction-order re-execution mask. failed
// marks speculative failures that must be retried regardless of conflict.
//
// Returns the re-execution mask and the conflict count.
func detectConflicts(rwSets []txReadWriteSet, failed []bool) ([]bool, int) {
	n := len(rwSets)
	reExec := make([]bool, n)

	// committedWrites maps slot -> index of the latest tx that wrote it.
	committedWrites := make(map[common.Hash]int)

	conflicts := 0
	for i := 0; i < n; i++ {
		if failed != nil && failed[i] {
			// Speculative execution failed -- must retry sequentially.
			reExec[i] = true
			conflicts++
		} else {
			for slot := range rwSets[i].reads {
				if writerIdx, ok := committedWrites[slot]; ok && writerIdx < i {
					reExec[i] = true
					conflicts++
					break
				}
			}
			if !reExec[i] {
				// A later tx must also re-execute if it writes a slot an earlier
				// tx wrote (W∩W). The read scan above covers W∩R and R∩W; this
				// closes W∩W.
				for slot := range rwSets[i].writes {
					if writerIdx, ok := committedWrites[slot]; ok && writerIdx < i {
						reExec[i] = true
						conflicts++
						break
					}
				}
			}
		}
		// Register writes so later txs that touch them are flagged.
		for slot := range rwSets[i].writes {
			committedWrites[slot] = i
		}
	}

	return reExec, conflicts
}

// Metrics exposes parallel-execution statistics for observability.
type Metrics struct {
	BlocksProcessed atomic.Int64
	TxsProcessed    atomic.Int64
	TxsReExecuted   atomic.Int64
	// VerifiedBlocks counts blocks committed through ExecuteVerified — i.e. blocks
	// where the parallel engine produced a state root byte-identical to the local
	// sequential reference and the result was committed. It is the observable proof
	// that the gated parallel path actually engaged (vs falling closed to
	// sequential), and the signal a Process-level test asserts on.
	VerifiedBlocks atomic.Int64
}

// DefaultMetrics is the global parallel-execution metrics instance.
var DefaultMetrics Metrics

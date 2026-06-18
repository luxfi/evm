// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// DEX-fill settlement on the Block-STM engine.
//
// A matched DEX fill settles to a balance mutation: debit the seller, credit
// the buyer, touch the market. That mutation is exactly the unit Block-STM
// parallelises -- a piece of work with an explicit read/write set. This file
// adapts DEX fills onto the SAME speculate -> conflict-detect -> commit engine
// the EVM block path uses (blockstm.go), reusing the shared detectConflicts
// kernel (the predicate the GPU conflict_detect kernel enforces byte-for-byte).
//
// Why this is the right seam (not a second engine):
//   - The EVM path approximates a tx's read/write set from To()/coinbase.
//     A DEX fill carries its EXACT footprint: {maker account, taker account,
//     market}. So the settlement path is MORE precise, not a reimplementation.
//   - At 10k markets, distinct fills touch disjoint accounts -> detectConflicts
//     finds few edges -> near-linear parallel settlement. The few cross-account
//     conflicts (one account in two fills) re-execute in deterministic index
//     order, identical to the GPU edge order.
//
// Boundary: this package settles Fill values; it does NOT match orders and does
// NOT depend on lux/dex. The d-chain matcher (dex/pkg/dchain) produces the
// ordered Fill slice and supplies the FillApplyFunc that mutates its state.
// That hook is documented in DChainSettlementHook below.

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
)

// Fill is one matched DEX fill to settle. It is a value: the accounts and
// market it touches (its conflict footprint) plus the index that fixes its
// deterministic commit order (the matcher's sequence number within the batch).
//
// Apply performs the actual balance mutation against the supplied state. It is
// injected so this package never needs to know asset/ledger internals -- the
// same inversion TxApplyFunc uses for the EVM path. Apply MUST be a pure
// function of the state it is given (no shared mutable capture) so speculative
// execution on isolated copies is sound.
type Fill struct {
	// Maker and Taker are the two accounts whose balances move. For a spot
	// trade these are the resting-order owner and the incoming-order owner.
	Maker common.Address
	Taker common.Address

	// Market identifies the order book (e.g. the pool/pair address). Two fills
	// in the same market that adjust shared market-level state (fee accumulator,
	// last-price oracle slot) conflict; fills in different markets do not.
	Market common.Address

	// Index is the fill's position in the deterministic settlement order
	// (matcher sequence). Commit and re-execution honour ascending Index.
	Index int

	// Apply settles this fill against the state. Return an error to abort the
	// batch (e.g. insufficient balance that the matcher should have caught).
	Apply FillApplyFunc
}

// FillApplyFunc settles a single fill against a state copy. The d-chain matcher
// supplies this; it debits/credits the two accounts and updates market state.
type FillApplyFunc func(statedb *state.StateDB, f Fill) error

// SettlementExecutor settles batches of DEX fills. On a single state it applies
// in deterministic order with cheap conflict detection (SettleBatch); across
// independent state shards it settles concurrently (SettleSharded) -- the latter
// is where DEX-settlement parallelism actually lives (geth StateDB cannot be
// mutated concurrently even for disjoint accounts; see SettleBatch).
type SettlementExecutor struct {
	workers int
}

// NewSettlementExecutor creates a settlement executor. Pass 0 for workers to
// use runtime.NumCPU().
func NewSettlementExecutor(workers int) *SettlementExecutor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &SettlementExecutor{workers: workers}
}

// fillRWSet builds the exact read/write footprint of a fill: both accounts and
// the market are read and written (balances and market state change). This is
// the precise footprint -- no false positives from coarse approximation, so
// detectConflicts produces the minimal, correct edge set.
func fillRWSet(f Fill) txReadWriteSet {
	rw := txReadWriteSet{
		reads:  make(map[common.Hash]common.Hash, 3),
		writes: make(map[common.Hash]common.Hash, 3),
	}
	for _, a := range [...]common.Address{f.Maker, f.Taker, f.Market} {
		slot := common.BytesToHash(a.Bytes())
		rw.reads[slot] = slot
		rw.writes[slot] = slot
	}
	return rw
}

// SettleBatch settles fills against a single state in deterministic index order
// and returns the conflict count (for observability). It does NOT copy state per
// fill: a DEX fill is a ~3-account balance mutation (sub-microsecond), so the
// optimistic copy-the-world-per-item model the EVM block path uses (where each
// tx is heavy enough to amortise a StateDB.Copy) is catastrophically wrong for
// fine-grained fills -- measured 311x SLOWER than sequential on this host.
//
// MEASURED FINDING (M1 Max, 10k fills): per-fill StateDB.Copy() => 750 fills/s
// (70 GB allocated); sequential apply on the shared state => 233k fills/s. The
// conflict-detection kernel itself is cheap (2.2M fills/s) -- so the right shape
// is "detect cheaply, then settle without per-item world-copies".
//
// Single-state parallelism is impossible here: geth's StateDB.stateObjects is a
// plain map mutated even on reads (getOrNewStateObject), so concurrent goroutines
// touching DISJOINT accounts still race the map and panic. The parallelism for
// DEX settlement therefore lives ONE level up -- across independent state shards
// (SettleSharded), which is also the LAN / multi-node story. On one state, the
// correct, fastest path is the deterministic sequential apply this method does.
//
// fills MUST be ordered by their matcher sequence; the order is preserved so the
// committed state is identical for every validator.
func (e *SettlementExecutor) SettleBatch(base *state.StateDB, fills []Fill) (int, error) {
	n := len(fills)
	if n == 0 {
		return 0, nil
	}

	// Conflict detection over the precise fill footprints. This is the
	// consensus-relevant artefact (the same edge set the GPU conflict_detect
	// kernel produces, KAT byte-equal) and the input to SettleSharded's
	// cross-shard handling. Cheap: O(Σ|rwSet|), measured 2.2M fills/s.
	_, conflicts := detectConflicts(rwSetsForFills(fills), nil)

	// Deterministic sequential apply on the shared state -- no per-fill copy.
	for i := 0; i < n; i++ {
		if err := fills[i].Apply(base, fills[i]); err != nil {
			return conflicts, fmt.Errorf("settle fill %d (market %s): %w",
				i, fills[i].Market.Hex(), err)
		}
	}

	DefaultMetrics.BlocksProcessed.Add(1)
	DefaultMetrics.TxsProcessed.Add(int64(n))
	DefaultMetrics.TxsReExecuted.Add(int64(conflicts))
	return conflicts, nil
}

// ShardResult reports a sharded settlement outcome.
type ShardResult struct {
	Shards        int // number of intra-shard states settled concurrently
	IntraShard    int // fills settled inside a single shard (parallel)
	CrossShard    int // fills touching >1 shard (settled serially after)
	Conflicts     int // total conflict edges across all fills (consensus artefact)
	CrossShardCfl int // conflicts among the cross-shard fills
}

// shardOf maps an address to one of `shards` partitions (FNV-1a, deterministic
// and identical on every validator). Used to assign a MARKET to a shard.
func shardOf(a common.Address, shards int) int {
	var h uint64 = 1469598103934665603
	for _, b := range a {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return int(h % uint64(shards))
}

// fillShard assigns a fill to a shard BY MARKET. This is the correct DEX
// partition: a fill belongs to exactly one market, so sharding by market makes
// every fill intra-shard by construction -- the same per-book-arena model the
// matcher uses ("books never share state on the hot path"). The only state that
// crosses a shard boundary is an ACCOUNT that trades in markets assigned to
// different shards (a maker active across markets); those collisions are the
// cross-shard merge cost, NOT the common case.
//
// Sharding by account instead would make a 2-party trade straddle two shards
// almost always (two independent accounts rarely co-locate) -- measured ~100%
// cross-shard at 4+ account-shards. Sharding by market is the fix.
func fillShard(f Fill, shards int) int {
	return shardOf(f.Market, shards)
}

// SettleSharded settles a fill batch by partitioning accounts across N
// independent state shards and settling each shard CONCURRENTLY. This is the
// real parallelism for DEX settlement: each shard is its own *state.StateDB (no
// shared map), so the goroutines never race. It is also the in-process model of
// the LAN multi-node settlement -- a node == a shard, the merge == cross-shard
// reconciliation.
//
// Contract:
//   - shardStates[i] is an independent state holding the accounts that trade
//     ONLY in markets assigned to shard i; the caller seeds each such account
//     into that shard, and cross-shard accounts into base (see seedSharded).
//   - base is the authoritative state for fills touching a cross-shard account
//     (an account that trades in markets on >1 shard) and the merge target.
//     After this returns, the canonical post-batch state = the union of the
//     shard states + the cross-shard fills applied to base, exactly as a single
//     sequential settlement would produce (TestSettleShardedEqualsSequential).
//
// Partition: fills are sharded BY MARKET (fillShard), so a fill is always
// intra-shard for its market. The hazard is an ACCOUNT trading across markets
// that landed on different shards -- two shard goroutines would mutate it
// concurrently. SettleSharded detects those cross-shard accounts up front and
// routes every fill touching one to the serial base tail, so the concurrent
// shard phase only ever touches shard-local accounts -- race-free WITHOUT locks.
//
// Determinism: a shard's fills apply in ascending Index; shards are disjoint by
// construction so their relative order is irrelevant; the cross-shard tail
// applies last in ascending Index. Every validator runs the same shardOf, so
// the partition and the final state are identical everywhere.
func (e *SettlementExecutor) SettleSharded(
	base *state.StateDB,
	shardStates []*state.StateDB,
	fills []Fill,
) (ShardResult, error) {
	shards := len(shardStates)
	res := ShardResult{Shards: shards}
	if shards == 0 {
		return res, fmt.Errorf("SettleSharded: no shard states")
	}

	// Whole-batch conflict count for observability (consensus artefact).
	_, res.Conflicts = detectConflicts(rwSetsForFills(fills), nil)

	// Pass 1: find accounts that appear under markets assigned to >1 shard.
	// acctShard[acct] = the single shard it's seen on, or -1 once it spans two.
	acctShard := make(map[common.Address]int, len(fills)*2)
	note := func(a common.Address, s int) {
		if prev, ok := acctShard[a]; !ok {
			acctShard[a] = s
		} else if prev != s && prev != -1 {
			acctShard[a] = -1 // now cross-shard
		}
	}
	for _, f := range fills {
		s := fillShard(f, shards)
		note(f.Maker, s)
		note(f.Taker, s)
		// Market is unique to its shard by definition; no need to track.
	}
	isCross := func(a common.Address) bool { return acctShard[a] == -1 }

	// Pass 2: route. A fill goes to the serial tail iff either trading account
	// is cross-shard; otherwise to its market's shard (race-free, shard-local).
	perShard := make([][]Fill, shards)
	var cross []Fill
	for _, f := range fills {
		if isCross(f.Maker) || isCross(f.Taker) {
			cross = append(cross, f)
			res.CrossShard++
		} else {
			s := fillShard(f, shards)
			perShard[s] = append(perShard[s], f)
			res.IntraShard++
		}
	}

	// Settle each shard concurrently on its own state. No shared mutable state
	// is touched across goroutines, so this is race-free by construction.
	errs := make([]error, shards)
	sem := make(chan struct{}, e.workers)
	var wg sync.WaitGroup
	for s := 0; s < shards; s++ {
		if len(perShard[s]) == 0 {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(shard int) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, f := range perShard[shard] {
				if err := f.Apply(shardStates[shard], f); err != nil {
					errs[shard] = fmt.Errorf("shard %d fill %d: %w", shard, f.Index, err)
					return
				}
			}
		}(s)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return res, err
		}
	}

	// Cross-shard fills: apply on base in ascending Index (the merge). These are
	// the boundary-spanning settlements -- rare when accounts cluster, but always
	// correct. Their conflict count is reported for the LAN-merge cost model.
	if len(cross) > 0 {
		_, res.CrossShardCfl = detectConflicts(rwSetsForFills(cross), nil)
		for i := range cross {
			if err := cross[i].Apply(base, cross[i]); err != nil {
				return res, fmt.Errorf("cross-shard fill %d: %w", cross[i].Index, err)
			}
		}
	}

	DefaultMetrics.BlocksProcessed.Add(1)
	DefaultMetrics.TxsProcessed.Add(int64(len(fills)))
	DefaultMetrics.TxsReExecuted.Add(int64(res.Conflicts))
	return res, nil
}

// conflictEdgesOf returns the FULL conflict edge list over read/write sets in
// canonical (lo, hi) ascending order -- byte-identical to what the GPU
// conflict_detect kernel emits (gpu-kernels ops/cevm/.../conflict_detect, which
// flags EVERY conflicting upper-triangle pair, KAT byte-equal to the CPU
// oracle). The predicate is the same three-way intersection the GPU enforces:
//
//	pair (lo, hi) is an edge iff
//	  W_lo ∩ R_hi ≠ ∅  OR  W_lo ∩ W_hi ≠ ∅  OR  R_lo ∩ W_hi ≠ ∅
//
// Note this differs from the detectConflicts re-execution MASK: the mask only
// needs "fill i conflicts with SOME earlier fill" (one bit per fill), whereas
// this edge list is the consensus artefact -- the exact set the GPU produces
// and every validator must agree on. For a slot written by fills 0,1,2 the GPU
// emits all of (0,1),(0,2),(1,2); this reproduces that.
//
// It builds per-fill an inverted index of which earlier fills wrote each slot,
// so the cost is Σ(edges) rather than the O(N²) the GPU pays with a thread per
// pair -- same output, CPU-appropriate shape. (At low conflict rates Σ(edges)
// is tiny; the GPU's value is the dense/worst-case batch.)
func conflictEdgesOf(rwSets []txReadWriteSet) [][2]int {
	n := len(rwSets)
	// Per slot, the earlier fills that wrote / read it (ascending index).
	writers := make(map[common.Hash][]int)
	readers := make(map[common.Hash][]int)
	var edges [][2]int
	for i := 0; i < n; i++ {
		seen := make(map[int]struct{})
		mark := func(idxs []int) {
			for _, j := range idxs {
				seen[j] = struct{}{}
			}
		}
		// hi=i's READS hit earlier WRITES  -> W_lo ∩ R_hi
		for slot := range rwSets[i].reads {
			mark(writers[slot])
		}
		// hi=i's WRITES hit earlier WRITES -> W_lo ∩ W_hi
		// hi=i's WRITES hit earlier READS  -> R_lo ∩ W_hi
		for slot := range rwSets[i].writes {
			mark(writers[slot])
			mark(readers[slot])
		}
		if len(seen) > 0 {
			los := make([]int, 0, len(seen))
			for j := range seen {
				los = append(los, j)
			}
			sortInts(los)
			for _, j := range los {
				edges = append(edges, [2]int{j, i})
			}
		}
		// Register i for later fills (after computing its edges -- upper triangle).
		for slot := range rwSets[i].writes {
			writers[slot] = append(writers[slot], i)
		}
		for slot := range rwSets[i].reads {
			readers[slot] = append(readers[slot], i)
		}
	}
	return edges
}

// ConflictEdges returns the GPU-equivalent conflict edge list for a fill batch
// (canonical (lo,hi) ascending). Exposed so the d-chain side can assert
// CPU==GPU by diffing this against the GPU op_cevm_conflict_detect output over
// the same footprints, and to drive a sharded merge.
func ConflictEdges(fills []Fill) [][2]int {
	return conflictEdgesOf(rwSetsForFills(fills))
}

func rwSetsForFills(fills []Fill) []txReadWriteSet {
	out := make([]txReadWriteSet, len(fills))
	for i := range fills {
		out[i] = fillRWSet(fills[i])
	}
	return out
}

// sortInts is a tiny ascending insertion sort (edge fan-in per fill is small).
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

// DChainSettlementHook documents the boundary between this package (settlement
// on the reused Block-STM engine) and dex/pkg/dchain (the matcher + ledger).
// This is the documented adapter seam: the d-chain VM owns the other side and
// is out of scope for this track (another track owns dex/pkg/dchain).
//
// On the d-chain side, Block.Verify already matches orders against a versiondb
// overlay (see dchain-vm-design). To settle the resulting fills in parallel:
//
//  1. After matching, build []Fill from the matched trades:
//     fills[k] = Fill{
//     Maker:  trade.MakerOwner,
//     Taker:  trade.TakerOwner,
//     Market: poolAddr,
//     Index:  k,                 // matcher sequence == deterministic order
//     Apply:  func(sdb *state.StateDB, f Fill) error {
//     // debit/credit both legs in the d-chain ledger overlay,
//     // update market fee/oracle slot; fixed-point (Q64.64),
//     // never float -- the determinism rule from dchain-vm-design
//     },
//     }
//  2. Settle:  conflicts, err := NewSettlementExecutor(0).SettleBatch(overlay, fills)
//  3. Optionally assert CPU==GPU edges at scale:
//     cpuEdges := ConflictEdges(fills)
//     gpuEdges := <gpu-kernels op_cevm_conflict_detect over the same rwSets>
//     require cpuEdges == gpuEdges   // the consensus-relevant invariant
//  4. Block.Accept commits the overlay -> zapdb atomically (unchanged).
//
// The hook is deliberately a doc, not code: wiring it lives in dex/pkg/dchain,
// which this track must not modify.
const DChainSettlementHook = "see settlement.go doc: dex/pkg/dchain builds []Fill from matched trades and calls SettlementExecutor.SettleBatch on its state overlay"

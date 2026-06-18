// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"fmt"
	"math/rand"
	"sync/atomic"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
)

// newTestState builds an empty in-memory StateDB for settlement tests/benches.
func newTestState(t testing.TB) *state.StateDB {
	t.Helper()
	db := state.NewDatabase(rawdb.NewMemoryDatabase())
	sdb, err := state.New(types.EmptyRootHash, db, nil)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return sdb
}

// addr builds a deterministic address from an integer id.
func addr(id uint64) common.Address {
	var a common.Address
	a[0] = byte(id >> 8)
	a[1] = byte(id)
	a[19] = 0xAA
	return a
}

// transferApply returns a FillApplyFunc that moves `amount` from Maker to Taker
// and credits 1 wei to the Market (fee), the canonical balance mutation a spot
// fill settles to. It reads both balances (forcing a real state read) and
// writes them back.
func transferApply(amount uint64) FillApplyFunc {
	return func(sdb *state.StateDB, f Fill) error {
		amt := uint256.NewInt(amount)
		bal := sdb.GetBalance(f.Maker)
		if bal.Cmp(amt) < 0 {
			return fmt.Errorf("maker %s insufficient: have %s want %s",
				f.Maker.Hex(), bal, amt)
		}
		sdb.SubBalance(f.Maker, amt, tracing.BalanceChangeTransfer)
		sdb.AddBalance(f.Taker, amt, tracing.BalanceChangeTransfer)
		sdb.AddBalance(f.Market, uint256.NewInt(1), tracing.BalanceChangeTransfer)
		return nil
	}
}

// fund credits an account so it can be a maker.
func fund(sdb *state.StateDB, a common.Address, amount uint64) {
	sdb.AddBalance(a, uint256.NewInt(amount), tracing.BalanceChangeTransfer)
}

// ---------------------------------------------------------------------------
// Correctness: the conflict predicate over fills (the consensus artefact).
// ---------------------------------------------------------------------------

func TestSettlementDisjointFillsNoConflict(t *testing.T) {
	// 1000 fills, every fill touches a unique maker/taker/market -> 0 conflicts.
	const n = 1000
	fills := make([]Fill, n)
	for i := 0; i < n; i++ {
		fills[i] = Fill{
			Maker:  addr(uint64(i*3 + 1)),
			Taker:  addr(uint64(i*3 + 2)),
			Market: addr(uint64(i*3 + 3)),
			Index:  i,
			Apply:  transferApply(10),
		}
	}
	_, conflicts := detectConflicts(rwSetsForFills(fills), nil)
	if conflicts != 0 {
		t.Fatalf("disjoint fills: expected 0 conflicts, got %d", conflicts)
	}
	if edges := ConflictEdges(fills); len(edges) != 0 {
		t.Fatalf("disjoint fills: expected 0 edges, got %d: %v", len(edges), edges)
	}
}

func TestSettlementSharedAccountConflict(t *testing.T) {
	// fill 0 and fill 2 share a maker -> exactly one edge (0,2).
	shared := addr(9999)
	fills := []Fill{
		{Maker: shared, Taker: addr(1), Market: addr(100), Index: 0, Apply: transferApply(10)},
		{Maker: addr(2), Taker: addr(3), Market: addr(101), Index: 1, Apply: transferApply(10)},
		{Maker: shared, Taker: addr(4), Market: addr(102), Index: 2, Apply: transferApply(10)},
	}
	edges := ConflictEdges(fills)
	want := [][2]int{{0, 2}}
	if len(edges) != 1 || edges[0] != want[0] {
		t.Fatalf("shared-maker: expected edges %v, got %v", want, edges)
	}
	_, conflicts := detectConflicts(rwSetsForFills(fills), nil)
	if conflicts != 1 {
		t.Fatalf("shared-maker: expected 1 conflict, got %d", conflicts)
	}
}

func TestSettlementSharedMarketConflict(t *testing.T) {
	// Two fills in the SAME market (disjoint accounts) conflict on market state.
	mkt := addr(500)
	fills := []Fill{
		{Maker: addr(1), Taker: addr(2), Market: mkt, Index: 0, Apply: transferApply(10)},
		{Maker: addr(3), Taker: addr(4), Market: mkt, Index: 1, Apply: transferApply(10)},
	}
	edges := ConflictEdges(fills)
	if len(edges) != 1 || edges[0] != [2]int{0, 1} {
		t.Fatalf("shared-market: expected edge (0,1), got %v", edges)
	}
}

// TestConflictEdgesMatchGPUKAT replays the EXACT scenarios the GPU
// conflict_detect KAT uses (gpu-kernels conflict_detect_vectors.hpp) as raw
// read/write sets and asserts conflictEdgesOf produces the same canonical
// (lo,hi) edge list the GPU kernel emits. This is the CPU side of the CPU==GPU
// commit-order guarantee; the GPU side is proven (this host, M1 Max) by
// webgpu_cevm_conflict_detect_kat which dispatches the real WGSL kernel and
// matches the same expected_edges byte-for-byte.
//
// These use independent read-only and write keys (unlike DEX fills, where every
// account is read+write), so they exercise all three predicate branches
// (W∩R, W∩W, R∩W) exactly as the KAT vectors do.
func TestConflictEdgesMatchGPUKAT(t *testing.T) {
	key := func(tag byte, idx uint32) common.Hash {
		var h common.Hash
		h[0] = tag
		h[1] = byte(idx)
		h[2] = byte(idx >> 8)
		h[3] = byte(idx >> 16)
		h[4] = byte(idx >> 24)
		return h
	}
	rw := func(reads, writes []common.Hash) txReadWriteSet {
		s := txReadWriteSet{reads: map[common.Hash]common.Hash{}, writes: map[common.Hash]common.Hash{}}
		for _, k := range reads {
			s.reads[k] = k
		}
		for _, k := range writes {
			s.writes[k] = k
		}
		return s
	}

	cases := []struct {
		name string
		sets []txReadWriteSet
		want [][2]int
	}{
		{
			name: "two_tx_no_conflict",
			sets: []txReadWriteSet{
				rw([]common.Hash{key(0x01, 0)}, []common.Hash{key(0x02, 0)}),
				rw([]common.Hash{key(0x03, 0)}, []common.Hash{key(0x04, 0)}),
			},
			want: nil,
		},
		{
			name: "two_tx_w_w",
			sets: []txReadWriteSet{
				rw(nil, []common.Hash{key(0x10, 0)}),
				rw(nil, []common.Hash{key(0x10, 0)}),
			},
			want: [][2]int{{0, 1}},
		},
		{
			name: "two_tx_r_w", // tx0 writes K0, tx1 reads K0 -> W_lo ∩ R_hi
			sets: []txReadWriteSet{
				rw(nil, []common.Hash{key(0x20, 0)}),
				rw([]common.Hash{key(0x20, 0)}, nil),
			},
			want: [][2]int{{0, 1}},
		},
		{
			name: "three_tx_star", // tx0 writes K0; tx1,tx2 read K0; disjoint else
			sets: []txReadWriteSet{
				rw(nil, []common.Hash{key(0x30, 0)}),
				rw([]common.Hash{key(0x30, 0)}, []common.Hash{key(0x30, 1)}),
				rw([]common.Hash{key(0x30, 0)}, []common.Hash{key(0x30, 2)}),
			},
			want: [][2]int{{0, 1}, {0, 2}}, // no (1,2): both only READ K0
		},
		{
			name: "r_w_then_w", // R_lo ∩ W_hi: tx0 reads K, tx1 writes K
			sets: []txReadWriteSet{
				rw([]common.Hash{key(0x40, 0)}, nil),
				rw(nil, []common.Hash{key(0x40, 0)}),
			},
			want: [][2]int{{0, 1}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := conflictEdgesOf(c.sets)
			if !edgesEqual(got, c.want) {
				t.Fatalf("%s: GPU-KAT edges %v, got %v", c.name, c.want, got)
			}
		})
	}
}

// TestSettlementFillStarIsWriteChain documents the DEX-fill semantics: because a
// fill writes every account it touches (balances move), a single account shared
// by three fills produces the full upper-triangle (0,1),(0,2),(1,2) -- a write
// chain that serialises in index order. This is the correct, GPU-equivalent
// edge set for the fill model (distinct from the EVM-tx star above, where the
// shared key is read-only).
func TestSettlementFillStarIsWriteChain(t *testing.T) {
	shared := addr(0x3000)
	fills := []Fill{
		{Maker: shared, Taker: addr(0x3001), Market: addr(0x3101), Index: 0, Apply: transferApply(10)},
		{Maker: shared, Taker: addr(0x3002), Market: addr(0x3102), Index: 1, Apply: transferApply(10)},
		{Maker: shared, Taker: addr(0x3003), Market: addr(0x3103), Index: 2, Apply: transferApply(10)},
	}
	got := ConflictEdges(fills)
	want := [][2]int{{0, 1}, {0, 2}, {1, 2}}
	if !edgesEqual(got, want) {
		t.Fatalf("fill write-chain: expected %v, got %v", want, got)
	}
}

// TestSettlementParallelEqualsSequential proves the engine's core guarantee:
// SettleBatch yields byte-identical final balances to a plain sequential
// settlement, with and without conflicts (the deterministic-order guarantee).
func TestSettlementBatchEqualsSequential(t *testing.T) {
	for _, conflictRate := range []float64{0.0, 0.05, 0.5} {
		t.Run(fmt.Sprintf("conflict_%.0f%%", conflictRate*100), func(t *testing.T) {
			const n = 2000
			fills := makeFills(n, conflictRate, 1)

			// Sequential reference.
			seq := newTestState(t)
			fundMakers(seq, fills)
			for i := range fills {
				if err := fills[i].Apply(seq, fills[i]); err != nil {
					t.Fatalf("seq fill %d: %v", i, err)
				}
			}
			seqRoot := seq.IntermediateRoot(false)

			par := newTestState(t)
			fundMakers(par, fills)
			conflicts, err := NewSettlementExecutor(0).SettleBatch(par, fills)
			if err != nil {
				t.Fatalf("SettleBatch: %v", err)
			}
			parRoot := par.IntermediateRoot(false)

			if seqRoot != parRoot {
				t.Fatalf("SettleBatch root %s != sequential %s (conflicts=%d)",
					parRoot.Hex(), seqRoot.Hex(), conflicts)
			}
			t.Logf("n=%d conflictRate=%.0f%% -> conflicts=%d, roots match",
				n, conflictRate*100, conflicts)
		})
	}
}

// TestSettleShardedEqualsSequential proves the REAL parallel path: settling the
// same fills across N concurrently-settled state shards (+ a cross-shard tail)
// produces the same aggregate balances as a single sequential settlement. The
// aggregate is checked per-account because state lives across multiple shards.
func TestSettleShardedEqualsSequential(t *testing.T) {
	for _, shards := range []int{1, 3, 4, 10} {
		t.Run(fmt.Sprintf("shards_%d", shards), func(t *testing.T) {
			const n = 5000
			fills := makeFills(n, 0.02, 1)

			// Sequential reference balances.
			seq := newTestState(t)
			fundMakers(seq, fills)
			for i := range fills {
				if err := fills[i].Apply(seq, fills[i]); err != nil {
					t.Fatalf("seq fill %d: %v", i, err)
				}
			}

			// Sharded: one state per shard + base for cross-shard fills.
			base := newTestState(t)
			shardStates := make([]*state.StateDB, shards)
			for s := range shardStates {
				shardStates[s] = newTestState(t)
			}
			// Seed each maker's funds into the state that account maps to (its
			// shard, or base if the maker straddles -- makers always map to a
			// shard via shardOf, but a fill straddles if taker/market differ).
			seedSharded(base, shardStates, fills, shards)

			res, err := NewSettlementExecutor(0).SettleSharded(base, shardStates, fills)
			if err != nil {
				t.Fatalf("SettleSharded: %v", err)
			}

			// Compare per-account: an account's authoritative balance lives in
			// its shard (intra-shard fills) AND base (cross-shard fills). Sum.
			mismatch := 0
			for _, a := range allAccounts(fills) {
				want := seq.GetBalance(a)
				got := shardBalance(base, shardStates, a, shards)
				if want.Cmp(got) != 0 {
					mismatch++
					if mismatch <= 3 {
						t.Errorf("account %s: sharded %s != sequential %s",
							a.Hex(), got, want)
					}
				}
			}
			if mismatch != 0 {
				t.Fatalf("%d account balance mismatches (shards=%d)", mismatch, shards)
			}
			t.Logf("shards=%d n=%d -> intra=%d cross=%d conflicts=%d crossCfl=%d, balances match",
				shards, n, res.IntraShard, res.CrossShard, res.Conflicts, res.CrossShardCfl)
		})
	}
}

// ---------------------------------------------------------------------------
// Measurement: parallel speedup + conflict overhead of the reused engine.
// ---------------------------------------------------------------------------

// BenchmarkSettlementSerial measures plain sequential fill settlement.
func BenchmarkSettlementSerial(b *testing.B) {
	const n = 10000
	fills := makeFills(n, 0.01, 1)
	b.ResetTimer()
	for iter := 0; iter < b.N; iter++ {
		b.StopTimer()
		sdb := newTestState(b)
		fundMakers(sdb, fills)
		b.StartTimer()
		for i := range fills {
			if err := fills[i].Apply(sdb, fills[i]); err != nil {
				b.Fatalf("fill %d: %v", i, err)
			}
		}
	}
	b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "fills/s")
}

// BenchmarkSettleBatch measures the single-state path: conflict detection +
// deterministic sequential apply over 10k fills (1% conflict). This is the
// honest single-shard settlement rate (no per-fill world copy).
func BenchmarkSettleBatch(b *testing.B) {
	const n = 10000
	fills := makeFills(n, 0.01, 1)
	exec := NewSettlementExecutor(0)
	b.ResetTimer()
	for iter := 0; iter < b.N; iter++ {
		b.StopTimer()
		sdb := newTestState(b)
		fundMakers(sdb, fills)
		b.StartTimer()
		if _, err := exec.SettleBatch(sdb, fills); err != nil {
			b.Fatalf("SettleBatch: %v", err)
		}
	}
	b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "fills/s")
}

// BenchmarkConflictDetect isolates the conflict-detection kernel (the CPU twin
// of the GPU conflict_detect op) over a 10k-fill batch -- the edge-build cost.
func BenchmarkConflictDetect(b *testing.B) {
	for _, rate := range []float64{0.0, 0.01, 0.1} {
		b.Run(fmt.Sprintf("rate_%.0f%%", rate*100), func(b *testing.B) {
			const n = 10000
			fills := makeFills(n, rate, 1)
			rwSets := rwSetsForFills(fills)
			b.ResetTimer()
			var conflicts int
			for iter := 0; iter < b.N; iter++ {
				_, conflicts = detectConflicts(rwSets, nil)
			}
			b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "fills/s")
			b.ReportMetric(float64(conflicts), "conflicts")
		})
	}
}

// BenchmarkSettleSharded sweeps shard count to expose the REAL parallel speedup:
// independent state shards settled concurrently (the multi-core / multi-node
// model). 10k fills, 1% conflict rate; report aggregate fills/s.
func BenchmarkSettleSharded(b *testing.B) {
	for _, shards := range []int{1, 2, 4, 8, 10} {
		b.Run(fmt.Sprintf("shards_%d", shards), func(b *testing.B) {
			const n = 10000
			fills := makeFills(n, 0.01, 1)
			exec := NewSettlementExecutor(shards)
			var cross int64
			b.ResetTimer()
			for iter := 0; iter < b.N; iter++ {
				b.StopTimer()
				base := newTestState(b)
				shardStates := make([]*state.StateDB, shards)
				for s := range shardStates {
					shardStates[s] = newTestState(b)
				}
				seedSharded(base, shardStates, fills, shards)
				b.StartTimer()
				res, err := exec.SettleSharded(base, shardStates, fills)
				if err != nil {
					b.Fatalf("SettleSharded: %v", err)
				}
				atomic.AddInt64(&cross, int64(res.CrossShard))
			}
			b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "fills/s")
			b.ReportMetric(float64(cross)/float64(b.N), "cross/batch")
		})
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// makeFills builds n fills. Each fill gets unique maker/taker/market EXCEPT a
// `conflictRate` fraction that reuse an earlier fill's maker (forcing a W∩W or
// W∩R edge) -- modelling the cross-account collisions at 10k markets.
func makeFills(n int, conflictRate float64, amount uint64) []Fill {
	rng := rand.New(rand.NewSource(0xDEC1))
	fills := make([]Fill, n)
	for i := 0; i < n; i++ {
		maker := addr(uint64(i)*4 + 1)
		if i > 0 && rng.Float64() < conflictRate {
			// Collide with an earlier fill's maker.
			maker = addr(uint64(rng.Intn(i))*4 + 1)
		}
		fills[i] = Fill{
			Maker:  maker,
			Taker:  addr(uint64(i)*4 + 2),
			Market: addr(uint64(i)*4 + 3),
			Index:  i,
			Apply:  transferApply(amount),
		}
	}
	return fills
}

// fundMakers credits every distinct maker enough to cover its fills.
func fundMakers(sdb *state.StateDB, fills []Fill) {
	need := make(map[common.Address]uint64)
	for _, f := range fills {
		need[f.Maker] += 1_000_000
	}
	for a, amt := range need {
		fund(sdb, a, amt)
	}
}

// crossShardAccounts mirrors SettleSharded's routing: an account trading under
// markets on >1 shard is cross-shard. The test must agree with the production
// routing so funds land where the debits happen.
func crossShardAccounts(fills []Fill, shards int) map[common.Address]bool {
	seen := make(map[common.Address]int)
	note := func(a common.Address, s int) {
		if prev, ok := seen[a]; !ok {
			seen[a] = s
		} else if prev != s && prev != -1 {
			seen[a] = -1
		}
	}
	for _, f := range fills {
		s := fillShard(f, shards)
		note(f.Maker, s)
		note(f.Taker, s)
	}
	out := make(map[common.Address]bool)
	for a, s := range seen {
		if s == -1 {
			out[a] = true
		}
	}
	return out
}

// seedSharded funds each maker in the SAME state where its fill will debit it:
// the market's shard for shard-local fills, or base for fills routed to the
// cross-shard tail. This makes the across-states sum equal the single-state
// sequential result.
func seedSharded(base *state.StateDB, shardStates []*state.StateDB, fills []Fill, shards int) {
	cross := crossShardAccounts(fills, shards)
	for _, f := range fills {
		if cross[f.Maker] || cross[f.Taker] {
			fund(base, f.Maker, 1_000_000)
		} else {
			fund(shardStates[fillShard(f, shards)], f.Maker, 1_000_000)
		}
	}
}

// shardBalance returns an account's total balance across base + all shards
// (state lives wherever the fills touching the account were settled).
func shardBalance(base *state.StateDB, shardStates []*state.StateDB, a common.Address, shards int) *uint256.Int {
	sum := new(uint256.Int).Set(base.GetBalance(a))
	for s := range shardStates {
		sum.Add(sum, shardStates[s].GetBalance(a))
	}
	return sum
}

// allAccounts returns every distinct account referenced by the fills.
func allAccounts(fills []Fill) []common.Address {
	seen := make(map[common.Address]struct{})
	var out []common.Address
	add := func(a common.Address) {
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	for _, f := range fills {
		add(f.Maker)
		add(f.Taker)
		add(f.Market)
	}
	return out
}

func edgesEqual(a, b [][2]int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

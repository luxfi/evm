// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
)

// blockstm_dex_determinism_test.go is the DETERMINISM SLICE of the unified Block-STM
// executor on the fast SettleBatch seam — the proof that parallel-eligible settlement
// reproduces the canonical sequential state root, the anti-fork property the locked
// execution model rests on. It reuses the REAL kernels (detectConflicts via ConflictEdges,
// SettleBatch, the geth state root) — no second engine, no mock conflict detector.
//
// Two cases, the two halves of the invariant:
//
//   - DISJOINT fills (different markets, different accounts) -> detectConflicts finds ZERO
//     edges -> settling them yields byte-identical state to applying them sequentially. This
//     is the parallel-safe case: no edge means the scheduler may run them concurrently and
//     the committed root is identical everywhere.
//   - CONFLICTING fills (a HOT market two fills both touch, sharing an account) ->
//     detectConflicts MUST return the edge -> the fills are serialized in deterministic
//     ASCENDING-INDEX order -> the committed root equals the ascending-index sequential root
//     AND differs from the WRONG (swapped) order. The conflict edge is the consensus artifact
//     (the same the GPU conflict_detect kernel emits) that makes the serialization mandatory;
//     the root binding proves it fired and that the canonical order — not an arbitrary one —
//     is what every validator commits.
//
// The owner's locked-model tests TestBlockSTM_DifferentMarketsParallel and
// TestHotMarketBatchDeterministicRoot are satisfied by the SAME bodies (thin wrappers below),
// so this slice IS the owner's determinism gate, not a parallel one.

// hotMarketSlot is a fixed storage slot on a fill's Market account standing in for the
// ORDER-SENSITIVE per-market state a real fill mutates (the fee accumulator / last-price
// oracle / last-fill stamp named in settlement.go). It is what makes a HOT-market batch's
// root order-dependent: pure balance moves are commutative (a+x-y == a-y+x), so a balance-
// only fill could not distinguish settle order — the determinism claim would be vacuous. A
// market slot folded as keccak(prev || taker) is NON-commutative, so the committed root
// genuinely depends on the order the conflicting fills are applied, which is exactly what
// "deterministic hot-market root" must pin.
var hotMarketSlot = common.BytesToHash([]byte("lux.dex.hotmarket.lastfill"))

// hotMarketApply returns a FillApplyFunc that (1) moves `amount` from Maker to Taker (the
// real value leg, funded so it never underflows — kept commutative so it is NOT the source of
// order-dependence) and (2) folds the taker into the market's hot slot as
// keccak(prevSlot || taker) — the ORDER-SENSITIVE market mutation. So two fills on the SAME
// market commit a market slot whose final value depends on their apply order, isolating the
// determinism proof to the (consensus-relevant) ordering of conflicting fills.
func hotMarketApply(amount uint64) FillApplyFunc {
	return func(sdb *state.StateDB, f Fill) error {
		amt := uint256.NewInt(amount)
		// Value leg (commutative — funded so order does not change the final balances).
		if sdb.GetBalance(f.Maker).Cmp(amt) >= 0 {
			sdb.SubBalance(f.Maker, amt, tracing.BalanceChangeTransfer)
			sdb.AddBalance(f.Taker, amt, tracing.BalanceChangeTransfer)
		}
		// Hot-market leg (order-sensitive): fold the taker into the market's last-fill slot.
		// crypto.Keccak256 returns []byte (the crypto module's common.Hash differs from geth's,
		// so we convert through geth/common.BytesToHash to land the right slot type).
		prev := sdb.GetState(f.Market, hotMarketSlot)
		next := common.BytesToHash(crypto.Keccak256(prev.Bytes(), f.Taker.Bytes()))
		sdb.SetState(f.Market, hotMarketSlot, next)
		return nil
	}
}

// applySequential applies fills to a fresh state in the given order and returns the resulting
// intermediate root — the reference a parallel/serialized settlement must reproduce.
func applySequential(t testing.TB, fund func(*state.StateDB), order []Fill, apply func(uint64) FillApplyFunc, amount uint64) common.Hash {
	t.Helper()
	sdb := newTestState(t)
	fund(sdb)
	for i := range order {
		f := order[i]
		f.Apply = apply(amount)
		if err := f.Apply(sdb, f); err != nil {
			t.Fatalf("sequential apply fill %d: %v", i, err)
		}
	}
	return sdb.IntermediateRoot(false)
}

// ---------------------------------------------------------------------------
// Case 1 — disjoint fills (different markets) settle == sequential, 0 conflicts.
// ---------------------------------------------------------------------------

func runDisjointFillsParallelRootEqualsSequential(t *testing.T) {
	a := addrOf(0xA1)
	b := addrOf(0xB1)
	c := addrOf(0xC1)
	d := addrOf(0xD1)
	m1 := addrOf(0x101)
	m2 := addrOf(0x102)

	// Fill A on market M1 between accounts (a,b); fill B on market M2 between (c,d). Every
	// account and both markets are distinct -> the footprints are disjoint.
	mkFills := func() []Fill {
		return []Fill{
			{Maker: a, Taker: b, Market: m1, Index: 0, Apply: hotMarketApply(10)},
			{Maker: c, Taker: d, Market: m2, Index: 1, Apply: hotMarketApply(10)},
		}
	}
	fund := func(sdb *state.StateDB) {
		for _, acct := range []common.Address{a, c} { // makers need funds.
			sdb.AddBalance(acct, uint256.NewInt(1000), tracing.BalanceChangeTransfer)
		}
	}

	// DISJOINT => detectConflicts finds ZERO edges. This is the parallel-safe certificate.
	if edges := ConflictEdges(mkFills()); len(edges) != 0 {
		t.Fatalf("disjoint fills (different markets) must produce 0 conflict edges, got %v", edges)
	}

	// SettleBatch root == ascending-index sequential root.
	par := newTestState(t)
	fund(par)
	conflicts, err := NewSettlementExecutor(0).SettleBatch(par, mkFills())
	if err != nil {
		t.Fatalf("SettleBatch: %v", err)
	}
	if conflicts != 0 {
		t.Fatalf("disjoint settle reported %d conflicts, want 0", conflicts)
	}
	parRoot := par.IntermediateRoot(false)

	seqRoot := applySequential(t, fund, mkFills(), hotMarketApply, 10)
	if parRoot != seqRoot {
		t.Fatalf("disjoint SettleBatch root %s != sequential %s", parRoot.Hex(), seqRoot.Hex())
	}

	// FAIL-WITHOUT (in-test): the 0-edge result above is MEANINGFUL only if the detector
	// would have found an edge had the fills actually conflicted. Collapse the two markets
	// onto the SAME market and assert an edge NOW appears — proving the disjointness check is
	// real, not vacuously empty.
	collide := []Fill{
		{Maker: a, Taker: b, Market: m1, Index: 0, Apply: hotMarketApply(10)},
		{Maker: c, Taker: d, Market: m1, Index: 1, Apply: hotMarketApply(10)}, // SAME market m1.
	}
	if edges := ConflictEdges(collide); len(edges) == 0 {
		t.Fatal("fail-without broken: two fills on the SAME market produced NO edge — the " +
			"conflict detector is not actually keying on the market")
	}
	t.Logf("disjoint: 0 edges, SettleBatch root == sequential; same-market control yields an edge")
}

// Test9999_DisjointFillsParallelRootEqualsSequential proves disjoint (different-market) fills
// settle to the sequential root with zero conflicts — the parallel-safe half of the
// anti-fork invariant.
func Test9999_DisjointFillsParallelRootEqualsSequential(t *testing.T) {
	runDisjointFillsParallelRootEqualsSequential(t)
}

// TestBlockSTM_DifferentMarketsParallel is the owner's locked-model name for the same
// guarantee: fills on different markets are conflict-free and settle deterministically.
func TestBlockSTM_DifferentMarketsParallel(t *testing.T) {
	runDisjointFillsParallelRootEqualsSequential(t)
}

// ---------------------------------------------------------------------------
// Case 2 — conflicting (hot-market) fills serialize to the ascending-index root.
// ---------------------------------------------------------------------------

func runConflictingFillsReExecToSequentialRoot(t *testing.T) {
	a := addrOf(0xAA)  // the SHARED account: taker in C, maker in D.
	m1 := addrOf(0x51) // maker of C.
	t2 := addrOf(0x52) // taker of D.
	mkt := addrOf(0x500)

	// Fill C: Maker=m1, Taker=a (a is the taker). Fill D: Maker=a, Taker=t2 (a is the maker).
	// Both on the SAME hot market `mkt`. They share account a AND the market -> a true
	// conflict (W∩W on both a's slot and the market slot).
	fillC := Fill{Maker: m1, Taker: a, Market: mkt, Index: 0, Apply: hotMarketApply(10)}
	fillD := Fill{Maker: a, Taker: t2, Market: mkt, Index: 1, Apply: hotMarketApply(10)}
	fills := []Fill{fillC, fillD}

	fund := func(sdb *state.StateDB) {
		// Fund every maker generously so the balance legs never underflow and are commutative
		// — isolating the ONLY order-dependence to the hot-market slot (the consensus point).
		for _, acct := range []common.Address{m1, a, t2} {
			sdb.AddBalance(acct, uint256.NewInt(1_000_000), tracing.BalanceChangeTransfer)
		}
	}

	// detectConflicts MUST return the edge (0,1): the fills genuinely conflict, so a
	// deterministic serialization is mandatory for a fork-free root.
	edges := ConflictEdges(fills)
	if len(edges) != 1 || edges[0] != [2]int{0, 1} {
		t.Fatalf("conflicting hot-market fills must yield exactly edge (0,1), got %v", edges)
	}

	// The two possible serial orders produce DIFFERENT roots (the hot-market slot is order-
	// sensitive). This is the guard that the test is MEANINGFUL: if these were equal the
	// ordering claim would be vacuous (a commutative Apply). C is index 0, D is index 1, so
	// ascending-index order is C-then-D.
	rootCthenD := applySequential(t, fund, []Fill{fillC, fillD}, hotMarketApply, 10)
	rootDthenC := applySequential(t, fund, []Fill{fillD, fillC}, hotMarketApply, 10)
	if rootCthenD == rootDthenC {
		t.Fatal("hot-market Apply is order-INSENSITIVE — the determinism test would be vacuous; " +
			"the market slot must make the root depend on apply order")
	}

	// SettleBatch serializes in ascending INDEX order -> its root MUST equal C-then-D AND
	// MUST NOT equal the swapped D-then-C order. This proves the deterministic ordering fired
	// (the conflicting fills did not commit in an arbitrary/speculative order).
	par := newTestState(t)
	fund(par)
	conflicts, err := NewSettlementExecutor(0).SettleBatch(par, fills)
	if err != nil {
		t.Fatalf("SettleBatch: %v", err)
	}
	if conflicts != 1 {
		t.Fatalf("hot-market settle reported %d conflicts, want 1", conflicts)
	}
	parRoot := par.IntermediateRoot(false)

	if parRoot != rootCthenD {
		t.Fatalf("SettleBatch root %s != ascending-index (C-then-D) sequential %s — deterministic order NOT enforced",
			parRoot.Hex(), rootCthenD.Hex())
	}
	if parRoot == rootDthenC {
		t.Fatalf("SettleBatch root equals the SWAPPED (D-then-C) order %s — re-serialization did not fire",
			rootDthenC.Hex())
	}
	t.Logf("hot-market: edge (0,1) detected; SettleBatch root == C-then-D != D-then-C (deterministic order enforced)")
}

// Test9999_ConflictingFillsReExecToSequentialRoot proves conflicting hot-market fills
// serialize to the canonical ascending-index root (and not the swapped order) — the
// conflict-serialization half of the anti-fork invariant.
func Test9999_ConflictingFillsReExecToSequentialRoot(t *testing.T) {
	runConflictingFillsReExecToSequentialRoot(t)
}

// TestHotMarketBatchDeterministicRoot is the owner's locked-model name for the same
// guarantee: a batch of fills contending on one hot market commits a deterministic root.
func TestHotMarketBatchDeterministicRoot(t *testing.T) {
	runConflictingFillsReExecToSequentialRoot(t)
}

// addrOf builds a deterministic, collision-free address from an id (distinct from the
// settlement_test.go `addr` helper's layout so these tests' accounts never alias those).
func addrOf(id uint64) common.Address {
	var a common.Address
	a[0] = 0x9D
	a[18] = byte(id >> 8)
	a[19] = byte(id)
	return a
}

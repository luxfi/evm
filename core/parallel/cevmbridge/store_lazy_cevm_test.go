// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

// Bounded-RAM load proof for the in-luxd apply path. Seed + apply blocks against a
// resident cevm StateDB, StatePersist to a REAL zapdb, then load the SAME persisted
// state two ways from disk: StateLoadFrom (full reseed — materializes everything)
// and StateLoadFromLazy (store-backed — nothing materialized up front). Applying the
// SAME next block to both must yield a byte-identical FULL state root: the lazy
// handle faults only the touched accounts/slots in from zapdb, yet commits the whole
// state root. This is what removes "declined-too-large" for a real node.

import (
	"testing"

	"github.com/luxfi/database/zapdb"
	"github.com/luxfi/metric"
)

func TestStateLoadLazyMatchesFullReseed(t *testing.T) {
	const n = 100
	seedAccts, txsA, ctx := goldenTransferBlock(n)

	// Build resident state: seed once, apply block A then block B.
	h := StateCreate()
	if h == 0 {
		t.Fatal("StateCreate returned 0")
	}
	StateSeed(h, seedAccts, nil)

	if resA, err := StateApplyBlock(h, txsA, ctx); err != nil || !resA.OK {
		t.Fatalf("apply A: err=%v ok=%v", err, resA.OK)
	}
	rcptsA := make([]([20]byte), n)
	for i := uint32(0); i < n; i++ {
		rcptsA[i] = rcpt(i)
	}
	txsB, rcptsB := blockOnward(rcptsA, 15)
	ctxB := ctx
	ctxB.BlockNumber = 2
	if resB, err := StateApplyBlock(h, txsB, ctxB); err != nil || !resB.OK {
		t.Fatalf("apply B: err=%v ok=%v", err, resB.OK)
	}
	preRoot := StateRoot(h)

	// Persist to a real on-disk zapdb, then simulate a restart.
	dir := t.TempDir()
	db, err := zapdb.New(dir, nil, "cevm-lazy-test", metric.NewRegistry())
	if err != nil {
		t.Fatalf("zapdb.New: %v", err)
	}
	if _, err := StatePersist(h, db); err != nil {
		t.Fatalf("StatePersist: %v", err)
	}
	StateFree(h)
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	// Reopen the persisted zapdb from disk.
	db2, err := zapdb.New(dir, nil, "cevm-lazy-test", metric.NewRegistry())
	if err != nil {
		t.Fatalf("zapdb.New reopen: %v", err)
	}
	defer db2.Close()

	// Load the SAME state two ways.
	hFull, err := StateLoadFrom(db2)
	if err != nil || hFull == 0 {
		t.Fatalf("StateLoadFrom: err=%v h=%d", err, hFull)
	}
	defer StateFree(hFull)
	hLazy, err := StateLoadFromLazy(db2)
	if err != nil || hLazy == 0 {
		t.Fatalf("StateLoadFromLazy: err=%v h=%d", err, hLazy)
	}
	defer StateFree(hLazy)

	// Both must recover the committed root (the lazy one from its store-backed ref).
	if rf, rl := StateRoot(hFull), StateRoot(hLazy); rf != preRoot || rl != preRoot {
		t.Fatalf("recovered root mismatch\n full=0x%x\n lazy=0x%x\n want=0x%x", rf, rl, preRoot)
	}
	t.Logf("both loads recovered committed root 0x%x", preRoot[:])

	// Apply the SAME next block to both. The lazy handle faults touched accounts/
	// slots in from zapdb on demand; both must commit the identical FULL root.
	txsC, _ := blockOnward(rcptsB, 14)
	ctxC := ctx
	ctxC.BlockNumber = 3

	resFull, err := StateApplyBlock(hFull, txsC, ctxC)
	if err != nil || !resFull.OK {
		t.Fatalf("apply C (full): err=%v ok=%v", err, resFull.OK)
	}
	resLazy, err := StateApplyBlock(hLazy, txsC, ctxC)
	if err != nil || !resLazy.OK {
		t.Fatalf("apply C (lazy): err=%v ok=%v", err, resLazy.OK)
	}

	if resFull.StateRoot != resLazy.StateRoot {
		t.Fatalf("paged apply != full-reseed apply (BOUNDED-RAM BUG)\n full=0x%x\n lazy=0x%x",
			resFull.StateRoot, resLazy.StateRoot)
	}
	t.Logf("PASS: paged (store-backed, bounded-RAM) apply root 0x%x == full-reseed apply — lazy load applies blocks byte-exactly without materializing the whole state",
		resLazy.StateRoot[:])
}

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

// Restart-survival proof for cevm's durable resident state. Seed + apply a few
// blocks against a RESIDENT cevm StateDB, StatePersist it to a REAL zapdb, then
// simulate a full process restart — free the in-memory handle AND close the
// zapdb — reopen the zapdb from disk, StateLoadFrom into a NEW handle, and assert
// the reloaded MPT root byte-matches the pre-restart root. Then apply the NEXT
// block on the reloaded handle and assert it still matches the golden-equivalent
// (== luxfi/geth) stateless path.
//
// This exercises the IN-PROCESS callback backing (the durable node path): cevm
// (C++/cgo) calls db.Get/Put back through THIS Go runtime via the registered
// trampolines — no second Go runtime is embedded.

import (
	"testing"

	"github.com/luxfi/database/zapdb"
	"github.com/luxfi/metric"
)

// blockOnward builds a block where each of the N `from` addresses sends value=1
// to a fresh recipient derived by setting byte `mark` of its own address. Zero
// gas price (golden convention), so a sender needs only its 1-wei balance.
func blockOnward(from []([20]byte), mark int) ([]Tx, []([20]byte)) {
	txs := make([]Tx, len(from))
	to := make([]([20]byte), len(from))
	for i := range from {
		dst := from[i]
		dst[mark] = byte(0xA0 + mark) // distinct, deterministic next hop
		var value [32]byte
		value[31] = 1
		txs[i] = Tx{Sender: from[i], Recipient: dst, Value: value, GasLimit: 21000, Nonce: 0}
		to[i] = dst
	}
	return txs, to
}

func TestStatePersistReloadSurvivesRestart(t *testing.T) {
	if ABIVersion() != 2 {
		t.Fatalf("unexpected ABI version %d (want 2)", ABIVersion())
	}

	const n = 100
	seedAccts, txsA, ctx := goldenTransferBlock(n)

	// --- Build resident state: seed once, apply block A, then block B. ---
	h := StateCreate()
	if h == 0 {
		t.Fatal("StateCreate returned 0 (no resident handle)")
	}
	StateSeed(h, seedAccts, nil)

	resA, err := StateApplyBlock(h, txsA, ctx)
	if err != nil || !resA.OK {
		t.Fatalf("apply A: err=%v ok=%v", err, resA.OK)
	}

	// Block B: block-A recipients (rcpt(i)) each send onward — depends on the
	// block-A post-state (recipient balances, sender nonces).
	rcptsA := make([]([20]byte), n)
	for i := uint32(0); i < n; i++ {
		rcptsA[i] = rcpt(i)
	}
	txsB, rcptsB := blockOnward(rcptsA, 15)
	ctxB := ctx
	ctxB.BlockNumber = 2
	resB, err := StateApplyBlock(h, txsB, ctxB)
	if err != nil || !resB.OK {
		t.Fatalf("apply B: err=%v ok=%v", err, resB.OK)
	}

	preRestartRoot := StateRoot(h)
	var zero [32]byte
	if preRestartRoot == zero {
		t.Fatal("pre-restart root is zero — resident state not established")
	}

	// --- Persist to a REAL zapdb on disk. ---
	dir := t.TempDir()
	db, err := zapdb.New(dir, nil, "cevm-persist-test", metric.NewRegistry())
	if err != nil {
		t.Fatalf("zapdb.New: %v", err)
	}
	nAcc, err := StatePersist(h, db)
	if err != nil {
		t.Fatalf("StatePersist: %v", err)
	}
	if nAcc <= 0 {
		t.Fatalf("StatePersist reported %d accounts, want > 0", nAcc)
	}

	// --- Full restart: destroy the in-memory handle AND close the zapdb. ---
	StateFree(h)
	if err := db.Close(); err != nil {
		t.Fatalf("db.Close: %v", err)
	}

	// --- Reopen the SAME on-disk zapdb (proves the bytes are durable). ---
	db2, err := zapdb.New(dir, nil, "cevm-persist-test", metric.NewRegistry())
	if err != nil {
		t.Fatalf("zapdb.New reopen: %v", err)
	}
	defer db2.Close()

	// --- Load into a NEW handle; assert the reloaded root byte-matches. ---
	h2, err := StateLoadFrom(db2)
	if err != nil {
		t.Fatalf("StateLoadFrom: %v", err)
	}
	if h2 == 0 {
		t.Fatal("StateLoadFrom returned 0")
	}
	defer StateFree(h2)

	reloadedRoot := StateRoot(h2)
	if reloadedRoot != preRestartRoot {
		t.Fatalf("reloaded root != pre-restart root (DURABILITY BUG)\n reloaded   =0x%x\n pre-restart=0x%x",
			reloadedRoot, preRestartRoot)
	}
	t.Logf("persisted %d accounts to zapdb, reloaded, root 0x%x == pre-restart root", nAcc, reloadedRoot[:])

	// --- Next block after reload still matches Go (the golden-equivalent path). ---
	// Block C: block-B recipients send onward again, applied on the RELOADED
	// handle. Reference: the stateless cevm path (byte-identical to luxfi/geth via
	// the golden gate) seeded from the reconstructed block-B post-state.
	txsC, _ := blockOnward(rcptsB, 14)
	ctxC := ctx
	ctxC.BlockNumber = 3

	resC, err := StateApplyBlock(h2, txsC, ctxC) // on the reloaded handle
	if err != nil || !resC.OK {
		t.Fatalf("apply C on reloaded handle: err=%v ok=%v", err, resC.OK)
	}

	// Reconstruct block-B post-state as an explicit stateless seed (seed0 -> A -> B).
	acctsA, storageA := applyDeltaToSeed(seedAccts, nil, resA)
	acctsB, storageB := applyDeltaToSeed(acctsA, storageA, resB)
	refC, err := ProcessBlock(acctsB, storageB, txsC, ctxC)
	if err != nil || !refC.OK {
		t.Fatalf("stateless ref C: err=%v ok=%v", err, refC.OK)
	}

	if resC.StateRoot != refC.StateRoot {
		t.Fatalf("next-block-after-reload root != Go stateless reference\n reloaded-applied=0x%x\n stateless-ref   =0x%x",
			resC.StateRoot, refC.StateRoot)
	}
	t.Logf("PASS: next block after reload root 0x%x == Go stateless (geth-golden) reference — reloaded resident state applies forward byte-exactly",
		resC.StateRoot[:])
}

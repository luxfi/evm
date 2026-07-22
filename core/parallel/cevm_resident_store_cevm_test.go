// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package parallel

// Disk-backed resident proof: with a store registered, the cevm resident StateDB
// CHECKPOINTS after a verified apply and, after a simulated restart (handles freed,
// store retained), LAZY-LOADS that checkpoint via getResident instead of a Go-state
// dump — then applies the next block to a byte-identical FULL root vs a full-reseed
// reference. This is the "own disk-backed state" path removing the post-restart
// declined-too-large dump, exercised through the real resident lifecycle.

import (
	"math/big"
	"testing"

	"github.com/luxfi/database/memdb"
	"github.com/luxfi/evm/core/parallel/cevmbridge"
	ethparams "github.com/luxfi/geth/params"
)

func whaleAccts(n int) []cevmbridge.Account {
	accts := make([]cevmbridge.Account, n)
	for i := range accts {
		accts[i].Address[0] = byte(i)
		accts[i].Address[1] = byte(i >> 8)
		accts[i].Balance[0] = 1_000_000 // 1e6 wei each; index 0 is the tx sender
	}
	accts[0].Balance[0] = 1_000_000_000_000 // whale sender
	return accts
}

// one 1-wei transfer from the whale (accts[0]) to a fresh recipient at `nonce`.
func transferBlock(nonce uint64, mark byte) []cevmbridge.Tx {
	var tx cevmbridge.Tx
	tx.Sender[0] = 0
	tx.Recipient[19] = mark // distinct new recipient per block
	tx.Value[31] = 1
	tx.GasLimit = 21000
	tx.Nonce = nonce
	return []cevmbridge.Tx{tx}
}

func ctxAt(num int64) cevmbridge.BlockCtx {
	return cevmbridge.BlockCtx{
		BlockNumber:   num,
		BlockTime:     1_700_000_000 + num,
		BlockGasLimit: 30_000_000,
		ChainID:       [32]byte{31: 1},
		Revision:      cevmbridge.RevShanghai,
	}
}

func TestResidentCheckpointLazyReload(t *testing.T) {
	cfg := &ethparams.ChainConfig{ChainID: big.NewInt(1)}

	db := memdb.New()
	SetCevmResidentStore(db)
	SetCevmCheckpointInterval(1) // checkpoint every block for the test
	defer func() {
		SetCevmResidentStore(nil)
		SetCevmCheckpointInterval(4096)
		resetResidents()
	}()

	accts := whaleAccts(50)

	// --- Establish resident state: fresh handle (no checkpoint yet), seed, apply A. ---
	resetResidents()
	rs := getResident(cfg)
	if rs.haveHandle == false {
		t.Fatal("getResident did not allocate a handle")
	}
	cevmbridge.StateSeed(rs.handle, accts, nil)
	if resA, err := cevmbridge.StateApplyBlock(rs.handle, transferBlock(0, 0xA1), ctxAt(1)); err != nil || !resA.OK {
		t.Fatalf("apply A: err=%v ok=%v", err, resA.OK)
	}
	rs.synced = true
	rs.height = 1
	persistCheckpoint(rs) // writes StatePersist records + ckpt height (1 % 1 == 0)
	preRoot := cevmbridge.StateRoot(rs.handle)

	// --- Simulate restart: free the in-memory handle + clear the registry. The
	// store (db) persists, holding the checkpoint. ---
	resetResidents()

	// --- getResident must now LAZY-LOAD the checkpoint (no Go dump). ---
	rs2 := getResident(cfg)
	if !rs2.haveHandle {
		t.Fatal("getResident (post-restart) did not establish a handle")
	}
	if !rs2.synced || rs2.height != 1 {
		t.Fatalf("post-restart resident not lazy-loaded: synced=%v height=%d (want synced height 1)", rs2.synced, rs2.height)
	}
	if got := cevmbridge.StateRoot(rs2.handle); got != preRoot {
		t.Fatalf("lazy-loaded root != checkpoint root\n got=0x%x\n want=0x%x", got, preRoot)
	}
	t.Logf("post-restart: lazy-loaded checkpoint at height 1, root 0x%x (no Go dump)", preRoot[:])

	// --- Apply the next block on the LAZY handle and on a FULL-RESEED reference;
	// the paged apply must produce the byte-identical FULL root. ---
	resLazy, err := cevmbridge.StateApplyBlock(rs2.handle, transferBlock(1, 0xB2), ctxAt(2))
	if err != nil || !resLazy.OK {
		t.Fatalf("apply B (lazy): err=%v ok=%v", err, resLazy.OK)
	}

	ref := cevmbridge.StateCreate()
	defer cevmbridge.StateFree(ref)
	cevmbridge.StateSeed(ref, accts, nil)
	if r, err := cevmbridge.StateApplyBlock(ref, transferBlock(0, 0xA1), ctxAt(1)); err != nil || !r.OK {
		t.Fatalf("ref apply A: err=%v ok=%v", err, r.OK)
	}
	if r := cevmbridge.StateRoot(ref); r != preRoot {
		t.Fatalf("reference post-A root != checkpoint root (seed mismatch)\n ref=0x%x\n ckpt=0x%x", r, preRoot)
	}
	resFull, err := cevmbridge.StateApplyBlock(ref, transferBlock(1, 0xB2), ctxAt(2))
	if err != nil || !resFull.OK {
		t.Fatalf("ref apply B: err=%v ok=%v", err, resFull.OK)
	}

	if resLazy.StateRoot != resFull.StateRoot {
		t.Fatalf("lazy-reloaded apply != full-reseed apply (BOUNDED-RAM RESIDENT BUG)\n lazy=0x%x\n full=0x%x",
			resLazy.StateRoot, resFull.StateRoot)
	}
	t.Logf("PASS: block after lazy-reload root 0x%x == full-reseed — the disk-backed resident applies forward byte-exactly without a Go dump",
		resLazy.StateRoot[:])
}

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

import (
	"encoding/hex"
	"testing"
)

// sndr mirrors test/cabi_block_parity.c: bytes[19]=i, [18]=i>>8, [17]=i>>16.
func sndr(i uint32) [20]byte {
	var a [20]byte
	a[19] = byte(i)
	a[18] = byte(i >> 8)
	a[17] = byte(i >> 16)
	return a
}

// rcpt(i) = sndr(i) with bytes[16]=0x01.
func rcpt(i uint32) [20]byte {
	a := sndr(i)
	a[16] = 0x01
	return a
}

// goldenTransferBlock builds the exact golden N-transfer block the C driver
// (cevm-cabi-block-parity) proves byte-identical to luxfi/geth: N funded senders
// each doing one value=1 transfer to a distinct fresh recipient, 21000 gas, zero
// gas price, Shanghai. Shared by the stateless and resident golden tests so both
// drive byte-identical inputs.
func goldenTransferBlock(n uint32) ([]Account, []Tx, BlockCtx) {
	accts := make([]Account, n)
	txs := make([]Tx, n)
	for i := uint32(0); i < n; i++ {
		accts[i] = Account{Address: sndr(i), Nonce: 0, Balance: [4]uint64{1000000, 0, 0, 0}}
		var value [32]byte
		value[31] = 1 // big-endian value = 1
		txs[i] = Tx{
			Sender:    sndr(i),
			Recipient: rcpt(i),
			Value:     value,
			// GasPrice all-zero
			GasLimit: 21000,
			Nonce:    0,
		}
	}
	var ctx BlockCtx
	ctx.Coinbase[19] = 0xFF
	ctx.BlockNumber = 1
	ctx.BlockTime = 1700000000
	ctx.BlockGasLimit = 30000000
	ctx.ChainID[31] = 1
	ctx.Revision = RevShanghai
	return accts, txs, ctx
}

// goldenRoots are the geth-golden state roots for goldenTransferBlock(n). ABI v2
// adds the post-state delta export; the roots are unchanged across v1→v2.
var goldenRoots = []struct {
	n      uint32
	golden string
}{
	{1, "b9d849f90993abb25f57172eb086e4b6ac15369954b0c556ae2280a1559805e9"},
	{100, "2028713fd7a12c1c73fb90f7b9de08be87c1d23b76acb9d2a4cfe3788577cf0f"},
	{1000, "57b7250733ad7f7a1be82d7f1e8d385496b00d2bb93a553b6730dd83dd020099"},
}

// TestProcessBlockGoldenRoots drives the golden N-transfer blocks through the GO
// marshalling in this package via the STATELESS cevm_process_block. A match here
// proves the Go structs serialize into the C ABI identically to the reference C
// driver (cevm-cabi-block-parity), which is byte-identical to luxfi/geth.
func TestProcessBlockGoldenRoots(t *testing.T) {
	if ABIVersion() != 2 {
		t.Fatalf("unexpected ABI version %d (want 2)", ABIVersion())
	}

	for _, c := range goldenRoots {
		accts, txs, ctx := goldenTransferBlock(c.n)

		res, err := ProcessBlock(accts, nil, txs, ctx)
		if err != nil {
			t.Fatalf("N=%d: ProcessBlock error: %v", c.n, err)
		}
		if !res.OK {
			t.Fatalf("N=%d: ProcessBlock ok=false", c.n)
		}
		got := hex.EncodeToString(res.StateRoot[:])
		if got != c.golden {
			t.Fatalf("N=%d: root mismatch\n got=0x%s\nwant=0x%s", c.n, got, c.golden)
		}
		t.Logf("N=%-4d cevm(stateless)=0x%s == geth PASS", c.n, got)

		if len(res.TxResults) != int(c.n) {
			t.Fatalf("N=%d: got %d tx results, want %d", c.n, len(res.TxResults), c.n)
		}
		for i, tr := range res.TxResults {
			if tr.Status != 1 || tr.GasUsed != 21000 || tr.NLogs != 0 || tr.Rejected {
				t.Fatalf("N=%d tx[%d] unexpected: status=%d gas=%d nlogs=%d rejected=%v",
					c.n, i, tr.Status, tr.GasUsed, tr.NLogs, tr.Rejected)
			}
		}
	}
}

// TestStateApplyBlockGoldenRoots drives the SAME golden blocks through the
// RESIDENT handle path (StateCreate → StateSeed → StateApplyBlock): seed the
// accounts once, apply the block against the resident StateDB, and assert cevm's
// resident MPT root byte-matches the geth golden root — and that StateRoot(h)
// reports the same root. This proves the stateful C-ABI produces the identical
// root to the stateless cevm_process_block (and thus luxfi/geth), so the resident
// applier is a byte-exact substitute for the per-block-dump path.
func TestStateApplyBlockGoldenRoots(t *testing.T) {
	if ABIVersion() != 2 {
		t.Fatalf("unexpected ABI version %d (want 2)", ABIVersion())
	}

	for _, c := range goldenRoots {
		accts, txs, ctx := goldenTransferBlock(c.n)

		h := StateCreate()
		if h == 0 {
			t.Fatalf("N=%d: StateCreate returned 0 (no resident handle)", c.n)
		}
		StateSeed(h, accts, nil)

		res, err := StateApplyBlock(h, txs, ctx)
		if err != nil {
			t.Fatalf("N=%d: StateApplyBlock error: %v", c.n, err)
		}
		if !res.OK {
			t.Fatalf("N=%d: StateApplyBlock ok=false", c.n)
		}
		got := hex.EncodeToString(res.StateRoot[:])
		if got != c.golden {
			t.Fatalf("N=%d: resident root mismatch\n got=0x%s\nwant=0x%s", c.n, got, c.golden)
		}

		// The resident root accessor must report the same post-apply root.
		rootAfter := StateRoot(h)
		if got2 := hex.EncodeToString(rootAfter[:]); got2 != c.golden {
			t.Fatalf("N=%d: StateRoot(h) after apply = 0x%s, want 0x%s", c.n, got2, c.golden)
		}

		if len(res.TxResults) != int(c.n) {
			t.Fatalf("N=%d: got %d tx results, want %d", c.n, len(res.TxResults), c.n)
		}
		for i, tr := range res.TxResults {
			if tr.Status != 1 || tr.GasUsed != 21000 || tr.NLogs != 0 || tr.Rejected {
				t.Fatalf("N=%d tx[%d] unexpected: status=%d gas=%d nlogs=%d rejected=%v",
					c.n, i, tr.Status, tr.GasUsed, tr.NLogs, tr.Rejected)
			}
		}
		StateFree(h)
		t.Logf("N=%-4d cevm(resident)=0x%s == geth PASS", c.n, got)
	}
}

// TestStateResidentAcrossBlocksMatchesStateless proves the KEY resident property
// at the C-ABI: applying a second block against the RESIDENT StateDB (no reseed)
// yields the byte-identical root to a STATELESS process_block seeded explicitly
// with block-1's post-state. i.e. the resident tries carry the post-state across
// blocks exactly, so no per-block dump is needed for correctness.
func TestStateResidentAcrossBlocksMatchesStateless(t *testing.T) {
	const n = 100
	accts, txsA, ctx := goldenTransferBlock(n)

	// Block B: each of the N recipients now sends 1 wei onward to a 2nd-hop
	// address, so block B touches a disjoint-ish set and depends on block-1
	// post-state (recipient balances, sender nonces). Recipients from block A are
	// rcpt(i); their nonce after receiving is 0, so a fresh send has nonce 0.
	txsB := make([]Tx, n)
	for i := uint32(0); i < n; i++ {
		hop2 := rcpt(i)
		hop2[15] = 0x02 // distinct 2nd-hop recipient
		var value [32]byte
		value[31] = 1
		txsB[i] = Tx{Sender: rcpt(i), Recipient: hop2, Value: value, GasLimit: 21000, Nonce: 0}
	}
	ctxB := ctx
	ctxB.BlockNumber = 2

	// --- Resident: seed once, apply A then B (no reseed between) ---
	h := StateCreate()
	if h == 0 {
		t.Fatal("StateCreate returned 0")
	}
	defer StateFree(h)
	StateSeed(h, accts, nil)
	resA, err := StateApplyBlock(h, txsA, ctx)
	if err != nil || !resA.OK {
		t.Fatalf("resident apply A: err=%v ok=%v", err, resA.OK)
	}
	resB, err := StateApplyBlock(h, txsB, ctxB)
	if err != nil || !resB.OK {
		t.Fatalf("resident apply B: err=%v ok=%v", err, resB.OK)
	}
	residentRootB := hex.EncodeToString(resB.StateRoot[:])

	// --- Stateless reference: reconstruct block-A post-state as an explicit
	// seed, then process_block(B) fresh. This is exactly what the OLD
	// dump-every-block path did — the resident path must match it byte-for-byte.
	accts1, storage1 := applyDeltaToSeed(accts, nil, resA)
	refB, err := ProcessBlock(accts1, storage1, txsB, ctxB)
	if err != nil || !refB.OK {
		t.Fatalf("stateless ref B: err=%v ok=%v", err, refB.OK)
	}
	statelessRootB := hex.EncodeToString(refB.StateRoot[:])

	if residentRootB != statelessRootB {
		t.Fatalf("resident-across-blocks root != stateless-with-reseed root\n resident =0x%s\n stateless=0x%s",
			residentRootB, statelessRootB)
	}
	t.Logf("PASS: resident block-2 root 0x%s == stateless(seed=block-1-poststate) root — resident tries carry post-state across blocks byte-exactly",
		residentRootB)
}

// applyDeltaToSeed reconstructs the post-state of a block as a fresh
// (accts, storage) seed pair: seed0 transformed by the result's absolute
// post-state delta (PostAccounts / PostStorage). Mirrors the Go applier's
// writePostState, but into the bridge seed structs.
func applyDeltaToSeed(seed []Account, seedStorage []Storage, res Result) ([]Account, []Storage) {
	accts := map[[20]byte]Account{}
	for _, a := range seed {
		accts[a.Address] = a
	}
	storage := map[[20]byte]map[[32]byte][32]byte{}
	for _, s := range seedStorage {
		if storage[s.Address] == nil {
			storage[s.Address] = map[[32]byte][32]byte{}
		}
		storage[s.Address][s.Key] = s.Value
	}

	for _, pa := range res.PostAccounts {
		if pa.Deleted {
			delete(accts, pa.Address)
			delete(storage, pa.Address)
			continue
		}
		a := accts[pa.Address]
		a.Address = pa.Address
		a.Nonce = pa.Nonce
		a.Balance = pa.Balance
		if pa.CodeChanged {
			a.Code = append([]byte(nil), pa.Code...)
		}
		accts[pa.Address] = a
	}
	for _, ps := range res.PostStorage {
		var zero [32]byte
		if ps.Value == zero {
			if storage[ps.Address] != nil {
				delete(storage[ps.Address], ps.Key)
			}
			continue
		}
		if storage[ps.Address] == nil {
			storage[ps.Address] = map[[32]byte][32]byte{}
		}
		storage[ps.Address][ps.Key] = ps.Value
	}

	outAccts := make([]Account, 0, len(accts))
	for _, a := range accts {
		outAccts = append(outAccts, a)
	}
	var outStorage []Storage
	for addr, slots := range storage {
		for k, v := range slots {
			outStorage = append(outStorage, Storage{Address: addr, Key: k, Value: v})
		}
	}
	return outAccts, outStorage
}

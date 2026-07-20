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

// TestProcessBlockGoldenRoots drives the exact golden N-transfer blocks the C
// driver (cevm-cabi-block-parity) proves byte-identical to luxfi/geth, but
// through the GO marshalling in this package. A match here proves the Go structs
// serialize into the C ABI identically to the reference C driver.
func TestProcessBlockGoldenRoots(t *testing.T) {
	cases := []struct {
		n      uint32
		golden string
	}{
		{1, "b9d849f90993abb25f57172eb086e4b6ac15369954b0c556ae2280a1559805e9"},
		{100, "2028713fd7a12c1c73fb90f7b9de08be87c1d23b76acb9d2a4cfe3788577cf0f"},
		{1000, "57b7250733ad7f7a1be82d7f1e8d385496b00d2bb93a553b6730dd83dd020099"},
	}

	// ABI v2 adds the post-state delta export (out_accts / out_storage) used by
	// the applier; the golden roots below are unchanged across v1→v2.
	if ABIVersion() != 2 {
		t.Fatalf("unexpected ABI version %d (want 2)", ABIVersion())
	}

	for _, c := range cases {
		accts := make([]Account, c.n)
		txs := make([]Tx, c.n)
		for i := uint32(0); i < c.n; i++ {
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
		t.Logf("N=%-4d cevm=0x%s == geth PASS", c.n, got)

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

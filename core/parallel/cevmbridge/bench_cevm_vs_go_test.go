// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

// Apples-to-apples block/state-layer benchmark: the cevm (C++ EVM) cgo
// ProcessBlock path vs the Go EVM (luxfi/geth core/state) raw state transition,
// on the GOLDEN block of N value transfers that both engines prove to the same
// keccak-MPT root 0x57b7250733ad7f7a1be82d7f1e8d385496b00d2bb93a553b6730dd83dd020099.
//
// The golden block (matches block_root_parity.cpp / bridge_cevm_test.go):
//   sender(i)    : bytes[19..17]=i, funded 1_000_000, nonce 0
//   recipient(i) : sender(i) with bytes[16]=0x01
//   tx           : value 1, gasPrice 0, gasLimit 21000, nonce 0
// gasPrice 0 isolates the transition to {nonce++, sender-=1, recipient+=1}, so
// the same post-state (and therefore the same MPT root) is reachable by both a
// full block pipeline (cevm) and a bare state mutation (Go).
//
// TIMED-REGION HONESTY (what each figure includes):
//   BenchmarkCevmProcessBlock  : the WHOLE cgo ProcessBlock — marshal accts+txs
//       across cgo, seed a fresh C++ StateDB, run the real evmc VM with full
//       nonce/gas/balance validation + journaling, commit the REAL keccak MPT,
//       and marshal tx-results + post-state deltas back. cevm does the full
//       consensus block-processing pipeline.
//   BenchmarkGoStateApply      : Go state-transition ONLY — 1000x
//       {SubBalance,AddBalance,SetNonce} + IntermediateRoot(true). Seeding the
//       1000 funded senders is EXCLUDED (StopTimer). NO EVM interpreter, NO tx
//       validation, NO gas accounting, NO cgo. This is the narrowest possible
//       Go baseline: cevm does strictly MORE work than this region.
//   BenchmarkGoStateSeedApply  : Go seed(1000)+apply(1000)+IntermediateRoot(true)
//       — the pure-function analogue of ProcessBlock (both start from raw
//       account snapshots and end at the root), still with no EVM/validation/gas.
//
// TestBenchRootParity is the VALIDITY GATE: if either engine fails to produce the
// golden root, the benchmark comparison is meaningless and the test fails.

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/geth/common"
	gethstate "github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
)

// benchSizes are the block sizes measured. N=1000 is the golden block whose
// root is precomputed and geth-proven; N=10000 has no precomputed golden, so it
// is validated by cevm==Go mutual agreement (each already matching geth at
// N=1/100/1000 is strong evidence the shared transition is correct).
var benchSizes = []uint32{1000, 10000}

// goldenRootHex is the keccak-MPT root of the N=1000 golden block, proven
// byte-identical between cevm process_block and luxfi/geth StateDB.
const goldenRootHex = "0x57b7250733ad7f7a1be82d7f1e8d385496b00d2bb93a553b6730dd83dd020099"

// goldenBlock builds the cevm-side inputs (accts + txs + ctx) for N transfers.
// sndr/rcpt are shared with bridge_cevm_test.go (same package + build tag).
func goldenBlock(n uint32) ([]Account, []Tx, BlockCtx) {
	accts := make([]Account, n)
	txs := make([]Tx, n)
	for i := uint32(0); i < n; i++ {
		accts[i] = Account{Address: sndr(i), Nonce: 0, Balance: [4]uint64{1_000_000, 0, 0, 0}}
		var value [32]byte
		value[31] = 1 // big-endian value = 1
		txs[i] = Tx{
			Sender:    sndr(i),
			Recipient: rcpt(i),
			Value:     value, // gasPrice all-zero
			GasLimit:  21000,
			Nonce:     0,
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

// seedGoState returns a fresh geth StateDB with the N senders funded to
// 1_000_000 (nonce 0) — the untimed pre-block snapshot, matching the C++
// setup_state + commit in block_root_parity.cpp.
func seedGoState(tb testing.TB, n uint32) *gethstate.StateDB {
	sdb, err := gethstate.New(types.EmptyRootHash, gethstate.NewDatabaseForTesting())
	if err != nil {
		tb.Fatalf("state.New: %v", err)
	}
	fund := uint256.NewInt(1_000_000)
	for i := uint32(0); i < n; i++ {
		sdb.AddBalance(common.Address(sndr(i)), fund, tracing.BalanceChangeUnspecified)
	}
	return sdb
}

// applyGoTransfers applies the N golden transfers as the raw state transition:
// {sender-=1, recipient+=1, sender.nonce=1}. gasPrice 0 => no fee, no coinbase
// credit. Returns the post-block MPT root.
func applyGoTransfers(sdb *gethstate.StateDB, n uint32) common.Hash {
	one := uint256.NewInt(1)
	for i := uint32(0); i < n; i++ {
		s := common.Address(sndr(i))
		sdb.SubBalance(s, one, tracing.BalanceChangeTransfer)
		sdb.AddBalance(common.Address(rcpt(i)), one, tracing.BalanceChangeTransfer)
		sdb.SetNonce(s, 1, tracing.NonceChangeEoACall)
	}
	return sdb.IntermediateRoot(true)
}

// cevmRoot runs cevm ProcessBlock for N transfers and returns the MPT root hex.
func cevmRoot(tb testing.TB, n uint32) string {
	accts, txs, ctx := goldenBlock(n)
	res, err := ProcessBlock(accts, nil, txs, ctx)
	if err != nil || !res.OK {
		tb.Fatalf("cevm ProcessBlock N=%d: err=%v ok=%v", n, err, res.OK)
	}
	return common.BytesToHash(res.StateRoot[:]).Hex()
}

// goRoot runs the Go geth state layer (seed + apply) for N transfers.
func goRoot(tb testing.TB, n uint32) string {
	return applyGoTransfers(seedGoState(tb, n), n).Hex()
}

// TestBenchRootParity is the validity gate: for every measured N, cevm and the
// Go state layer must agree — and at N=1000 both must equal the geth-proven
// golden root. Without this, the benchmark numbers are not a like-for-like
// comparison and the test fails.
func TestBenchRootParity(t *testing.T) {
	for _, n := range benchSizes {
		c := cevmRoot(t, n)
		g := goRoot(t, n)
		if c != g {
			t.Fatalf("N=%d: cevm(%s) != go(%s)", n, c, g)
		}
		if n == 1000 && c != goldenRootHex {
			t.Fatalf("N=1000: root %s != geth golden %s", c, goldenRootHex)
		}
		note := "cevm==go (mutual)"
		if n == 1000 {
			note = "cevm==go==geth golden"
		}
		t.Logf("ROOT PARITY OK  N=%-5d %s  root=%s", n, note, c)
	}
}

// reportRates attaches tx/s and nominal Mgas/s (21000 gas/tx imputed — the Go
// raw path does NO gas accounting, so its Mgas/s is a nominal comparison figure)
// for a run that processed n txs per op.
func reportRates(b *testing.B, n uint32) {
	secs := b.Elapsed().Seconds()
	if secs <= 0 {
		return
	}
	txs := float64(n) * float64(b.N)
	b.ReportMetric(txs/secs, "tx/s")
	b.ReportMetric(txs*21000/secs/1e6, "Mgas/s")
}

// BenchmarkCevmProcessBlock times the whole cgo ProcessBlock (seed C++ state +
// full validated EVM block + real keccak-MPT commit + result marshalling).
func BenchmarkCevmProcessBlock(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(sizeName(n), func(b *testing.B) {
			accts, txs, ctx := goldenBlock(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				res, err := ProcessBlock(accts, nil, txs, ctx)
				if err != nil || !res.OK {
					b.Fatalf("ProcessBlock: err=%v ok=%v", err, res.OK)
				}
			}
			b.StopTimer()
			reportRates(b, n)
		})
	}
}

// BenchmarkGoStateApply times ONLY the Go state transition + root
// (Nx Sub/Add/SetNonce + IntermediateRoot(true)); seeding is excluded.
func BenchmarkGoStateApply(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(sizeName(n), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				sdb := seedGoState(b, n)
				b.StartTimer()
				_ = applyGoTransfers(sdb, n)
			}
			b.StopTimer()
			reportRates(b, n)
		})
	}
}

// BenchmarkGoStateSeedApply times seed(N)+apply(N)+root — the pure-function
// analogue of ProcessBlock (raw snapshot -> root), still with no EVM
// interpreter / tx validation / gas accounting.
func BenchmarkGoStateSeedApply(b *testing.B) {
	for _, n := range benchSizes {
		b.Run(sizeName(n), func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sdb := seedGoState(b, n)
				_ = applyGoTransfers(sdb, n)
			}
			b.StopTimer()
			reportRates(b, n)
		})
	}
}

func sizeName(n uint32) string {
	if n >= 1000 && n%1000 == 0 {
		return "N=" + itoa(n/1000) + "k"
	}
	return "N=" + itoa(n)
}

func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var b [10]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

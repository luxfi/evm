// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// Measures the EXISTING BlockSTMExecutor (blockstm.go) parallel-exec speedup vs
// serial over a synthetic mostly-disjoint workload. The engine deep-copies the
// StateDB per tx in speculateAll, so the parallel win only materialises when the
// per-tx work is heavy enough to amortise that copy -- these benches make that
// crossover explicit (the same lesson the DEX-settlement path encodes: fine-
// grained items must NOT use copy-the-world speculation; see settlement.go).

// blockStateWith builds an empty in-mem StateDB; the per-tx work populates it,
// so each speculative StateDB.Copy() clones a progressively larger state -- which
// is exactly the cost these benches expose.
func blockStateWith(b *testing.B) *state.StateDB {
	b.Helper()
	db := state.NewDatabase(rawdb.NewMemoryDatabase())
	sdb, err := state.New(types.EmptyRootHash, db, nil)
	if err != nil {
		b.Fatalf("state.New: %v", err)
	}
	return sdb
}

// workApplyFn returns a TxApplyFunc that does `spin` units of arithmetic work
// per tx (modelling EVM interpreter cost) on top of a deterministic receipt.
func workApplyFn(spin int) TxApplyFunc {
	return func(
		_ *ethparams.ChainConfig,
		_ *types.Header,
		tx *types.Transaction,
		statedb *state.StateDB,
		_ vm.Config,
		txIndex int,
	) (*types.Receipt, error) {
		statedb.SetTxContext(tx.Hash(), txIndex)
		// Busy-work proportional to spin (keeps the result observable so the
		// compiler cannot elide it).
		acc := uint64(txIndex + 1)
		for i := 0; i < spin; i++ {
			acc = acc*6364136223846793005 + 1442695040888963407
		}
		return &types.Receipt{
			Type:    tx.Type(),
			Status:  types.ReceiptStatusSuccessful,
			TxHash:  tx.Hash(),
			GasUsed: 21000 + acc&0x1,
		}, nil
	}
}

// disjointTxs builds n txs each to a unique recipient (mostly-disjoint: only the
// shared coinbase write collides, which buildRWSet records as a write-only slot
// that no tx reads -> no conflict, per TestNoConflictDifferentAddresses).
func disjointTxs(n int) types.Transactions {
	txs := make(types.Transactions, n)
	for i := 0; i < n; i++ {
		var to common.Address
		to[0] = byte(i >> 16)
		to[1] = byte(i >> 8)
		to[2] = byte(i)
		to[19] = 0xBB
		txs[i] = types.NewTransaction(uint64(i), to, big.NewInt(1), 21000, big.NewInt(1e9), nil)
	}
	return txs
}

// benchBlockSTM runs the engine (serial when workers==1 is forced via a direct
// loop, parallel via ExecuteBlock) and reports tx/s.
func benchBlockSTM(b *testing.B, n, spin int) {
	header := testHeader()
	txs := disjointTxs(n)

	b.Run("serial", func(b *testing.B) {
		apply := workApplyFn(spin)
		for iter := 0; iter < b.N; iter++ {
			b.StopTimer()
			sdb := blockStateWith(b)
			b.StartTimer()
			for i := 0; i < n; i++ {
				if _, err := apply(nil, header, txs[i], sdb, vm.Config{}, i); err != nil {
					b.Fatal(err)
				}
			}
		}
		b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "tx/s")
	})

	b.Run("blockstm", func(b *testing.B) {
		exec := NewBlockSTMExecutor(0, workApplyFn(spin))
		for iter := 0; iter < b.N; iter++ {
			b.StopTimer()
			sdb := blockStateWith(b)
			b.StartTimer()
			if _, err := exec.ExecuteBlock(nil, header, txs, sdb, vm.Config{}); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "tx/s")
	})
}

// BenchmarkBlockSTMLightTx: cheap per-tx work -> the per-tx StateDB.Copy in
// speculateAll dominates, so Block-STM is SLOWER than serial. Documents the
// amortization floor (why fine-grained fills must not use this path).
func BenchmarkBlockSTMLightTx(b *testing.B) { benchBlockSTM(b, 2000, 0) }

// BenchmarkBlockSTMHeavyTx: heavy per-tx work (10k spins ~ a non-trivial EVM
// tx) -> parallel speculation amortises the copy and Block-STM beats serial,
// showing the engine's core scaling.
func BenchmarkBlockSTMHeavyTx(b *testing.B) { benchBlockSTM(b, 2000, 10000) }

// BenchmarkBlockSTMWorkers sweeps worker counts on the heavy-tx workload to
// expose core scaling of the speculative phase directly.
func BenchmarkBlockSTMWorkers(b *testing.B) {
	header := testHeader()
	const n = 2000
	txs := disjointTxs(n)
	for _, w := range []int{1, 2, 4, 8, 10} {
		b.Run(fmt.Sprintf("workers_%d", w), func(b *testing.B) {
			exec := NewBlockSTMExecutor(w, workApplyFn(10000))
			for iter := 0; iter < b.N; iter++ {
				b.StopTimer()
				sdb := blockStateWith(b)
				b.StartTimer()
				if _, err := exec.ExecuteBlock(nil, header, txs, sdb, vm.Config{}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(n*b.N)/b.Elapsed().Seconds(), "tx/s")
		})
	}
}

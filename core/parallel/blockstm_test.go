// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// mockApplyFn returns a TxApplyFunc that produces deterministic receipts.
// Each receipt uses gasUsed gas and succeeds.
func mockApplyFn(gasUsed uint64) TxApplyFunc {
	return func(
		config *ethparams.ChainConfig,
		header *types.Header,
		tx *types.Transaction,
		statedb *state.StateDB,
		vmCfg vm.Config,
		txIndex int,
	) (*types.Receipt, error) {
		statedb.SetTxContext(tx.Hash(), txIndex)
		return &types.Receipt{
			Type:    tx.Type(),
			Status:  types.ReceiptStatusSuccessful,
			TxHash:  tx.Hash(),
			GasUsed: gasUsed,
		}, nil
	}
}

// testHeader returns a minimal block header for testing.
func testHeader() *types.Header {
	return &types.Header{
		Number:   big.NewInt(100),
		GasLimit: 30_000_000,
		Time:     1700000000,
		Coinbase: common.HexToAddress("0x1111111111111111111111111111111111111111"),
		BaseFee:  big.NewInt(1000000000),
	}
}

// testTx creates a simple legacy transaction for testing.
// Uses different nonces and recipients to control conflict behavior.
func testTx(nonce uint64, to common.Address) *types.Transaction {
	return types.NewTransaction(
		nonce,
		to,
		big.NewInt(1000),
		21000,
		big.NewInt(1000000000),
		nil,
	)
}

func TestNewBlockSTMExecutor(t *testing.T) {
	e := NewBlockSTMExecutor(0, mockApplyFn(21000))
	if e.workers <= 0 {
		t.Fatalf("expected workers > 0, got %d", e.workers)
	}
	if e.applyFn == nil {
		t.Fatal("expected non-nil applyFn")
	}

	e2 := NewBlockSTMExecutor(4, mockApplyFn(21000))
	if e2.workers != 4 {
		t.Fatalf("expected 4 workers, got %d", e2.workers)
	}
}

func TestExecuteBlockNilApplyFn(t *testing.T) {
	e := &BlockSTMExecutor{workers: 2, applyFn: nil}
	receipts, err := e.ExecuteBlock(nil, testHeader(), make(types.Transactions, 5), nil, vm.Config{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if receipts != nil {
		t.Fatal("expected nil receipts when applyFn is nil")
	}
}

func TestExecuteBlockSingleTxFallthrough(t *testing.T) {
	e := NewBlockSTMExecutor(2, mockApplyFn(21000))
	header := testHeader()
	txs := types.Transactions{testTx(0, common.HexToAddress("0xaaaa"))}

	receipts, err := e.ExecuteBlock(nil, header, txs, nil, vm.Config{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if receipts != nil {
		t.Fatal("expected nil receipts for single tx (fall through)")
	}
}

func TestExecuteBlockEmptyFallthrough(t *testing.T) {
	e := NewBlockSTMExecutor(2, mockApplyFn(21000))
	receipts, err := e.ExecuteBlock(nil, testHeader(), nil, nil, vm.Config{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if receipts != nil {
		t.Fatal("expected nil receipts for empty tx list")
	}
}

func TestBuildRWSetWithRecipient(t *testing.T) {
	to := common.HexToAddress("0xbbbb")
	tx := testTx(0, to)
	header := testHeader()

	rwSet := buildRWSet(tx, header)

	addrSlot := common.BytesToHash(to.Bytes())
	if _, ok := rwSet.reads[addrSlot]; !ok {
		t.Fatal("expected recipient in reads")
	}
	if _, ok := rwSet.writes[addrSlot]; !ok {
		t.Fatal("expected recipient in writes")
	}

	// Coinbase should be in writes.
	coinbaseSlot := common.BytesToHash(header.Coinbase.Bytes())
	if _, ok := rwSet.writes[coinbaseSlot]; !ok {
		t.Fatal("expected coinbase in writes")
	}
}

func TestBuildRWSetContractCreation(t *testing.T) {
	// nil To = contract creation
	tx := types.NewContractCreation(0, big.NewInt(0), 100000, big.NewInt(1000000000), []byte{0x60, 0x00})
	header := testHeader()

	rwSet := buildRWSet(tx, header)

	// Contract creation should write the tx hash as sentinel.
	if _, ok := rwSet.writes[tx.Hash()]; !ok {
		t.Fatal("expected tx hash sentinel in writes for contract creation")
	}

	// No recipient read for contract creation.
	if len(rwSet.reads) != 0 {
		t.Fatalf("expected 0 reads for contract creation, got %d", len(rwSet.reads))
	}
}

func TestConflictDetection(t *testing.T) {
	// Two txs to the same address should produce a conflict.
	sharedAddr := common.HexToAddress("0xcccc")
	tx0 := testTx(0, sharedAddr)
	tx1 := testTx(1, sharedAddr)
	header := testHeader()

	rw0 := buildRWSet(tx0, header)
	rw1 := buildRWSet(tx1, header)

	// tx0 writes to sharedAddr slot, tx1 reads it -> conflict.
	committedWrites := make(map[common.Hash]int)
	for slot := range rw0.writes {
		committedWrites[slot] = 0
	}

	conflict := false
	for slot := range rw1.reads {
		if writerIdx, ok := committedWrites[slot]; ok && writerIdx < 1 {
			conflict = true
			break
		}
	}

	if !conflict {
		t.Fatal("expected conflict for two txs to same address")
	}
}

func TestNoConflictDifferentAddresses(t *testing.T) {
	tx0 := testTx(0, common.HexToAddress("0xdddd"))
	tx1 := testTx(1, common.HexToAddress("0xeeee"))
	header := testHeader()

	rw0 := buildRWSet(tx0, header)
	rw1 := buildRWSet(tx1, header)

	committedWrites := make(map[common.Hash]int)
	for slot := range rw0.writes {
		committedWrites[slot] = 0
	}

	conflict := false
	for slot := range rw1.reads {
		if writerIdx, ok := committedWrites[slot]; ok && writerIdx < 1 {
			conflict = true
			break
		}
	}

	// Coinbase is a shared write but tx1 does not READ coinbase, only writes.
	if conflict {
		t.Fatal("expected no conflict for txs to different addresses")
	}
}

func TestCommitOrdered(t *testing.T) {
	e := NewBlockSTMExecutor(2, mockApplyFn(21000))

	txs := types.Transactions{
		testTx(0, common.HexToAddress("0xaaaa")),
		testTx(1, common.HexToAddress("0xbbbb")),
		testTx(2, common.HexToAddress("0xcccc")),
	}

	results := []txResult{
		{receipt: &types.Receipt{GasUsed: 21000, TxHash: txs[0].Hash()}},
		{receipt: &types.Receipt{GasUsed: 42000, TxHash: txs[1].Hash()}},
		{receipt: &types.Receipt{GasUsed: 63000, TxHash: txs[2].Hash()}},
	}

	receipts, err := e.commitOrdered(txs, results)
	if err != nil {
		t.Fatalf("commitOrdered failed: %v", err)
	}
	if len(receipts) != 3 {
		t.Fatalf("expected 3 receipts, got %d", len(receipts))
	}

	// Verify cumulative gas is monotonically increasing.
	if receipts[0].CumulativeGasUsed != 21000 {
		t.Fatalf("receipt[0] cumulative gas: want 21000, got %d", receipts[0].CumulativeGasUsed)
	}
	if receipts[1].CumulativeGasUsed != 63000 {
		t.Fatalf("receipt[1] cumulative gas: want 63000, got %d", receipts[1].CumulativeGasUsed)
	}
	if receipts[2].CumulativeGasUsed != 126000 {
		t.Fatalf("receipt[2] cumulative gas: want 126000, got %d", receipts[2].CumulativeGasUsed)
	}
}

func TestCommitOrderedErrorPropagation(t *testing.T) {
	e := NewBlockSTMExecutor(2, mockApplyFn(21000))

	txs := types.Transactions{
		testTx(0, common.HexToAddress("0xaaaa")),
		testTx(1, common.HexToAddress("0xbbbb")),
	}

	results := []txResult{
		{receipt: &types.Receipt{GasUsed: 21000, TxHash: txs[0].Hash()}},
		{err: fmt.Errorf("execution failed")},
	}

	_, err := e.commitOrdered(txs, results)
	if err == nil {
		t.Fatal("expected error from commitOrdered when result has error")
	}
}

func TestMetrics(t *testing.T) {
	// Reset
	DefaultMetrics.BlocksProcessed.Store(0)
	DefaultMetrics.TxsProcessed.Store(0)
	DefaultMetrics.TxsReExecuted.Store(0)

	DefaultMetrics.BlocksProcessed.Add(1)
	DefaultMetrics.TxsProcessed.Add(100)
	DefaultMetrics.TxsReExecuted.Add(5)

	if DefaultMetrics.BlocksProcessed.Load() != 1 {
		t.Fatalf("expected 1, got %d", DefaultMetrics.BlocksProcessed.Load())
	}
	if DefaultMetrics.TxsProcessed.Load() != 100 {
		t.Fatalf("expected 100, got %d", DefaultMetrics.TxsProcessed.Load())
	}
	if DefaultMetrics.TxsReExecuted.Load() != 5 {
		t.Fatalf("expected 5, got %d", DefaultMetrics.TxsReExecuted.Load())
	}
}

func TestBlockSTMImplementsBlockExecutor(t *testing.T) {
	var _ BlockExecutor = (*BlockSTMExecutor)(nil)
}

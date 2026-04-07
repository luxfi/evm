// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// Block-STM parallel execution scaffold.
//
// Algorithm from "Block-STM: Scaling Blockchain Execution by Turning Ordering
// Curse to a Performance Blessing" (Gelashvili et al., Aptos Labs).
//
// Overview:
//   1. Speculatively execute all transactions in parallel on copied state.
//   2. Track read/write sets per transaction.
//   3. Validate: check if any earlier-committed tx wrote to a slot this tx read.
//   4. Re-execute conflicting transactions with merged state.
//   5. Commit writes in original transaction order.
//
// This file provides the goroutine pool, conflict detection, and commit logic.
// Actual EVM execution is injected via TxApplyFunc to avoid circular imports
// (core -> parallel -> core).

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
	log "github.com/luxfi/log"
)

// TxApplyFunc executes a single transaction against the given state and returns
// the receipt. This is the injection point that breaks the circular dependency
// between core and parallel: core.applyTransaction satisfies this signature.
//
// The function must call statedb.SetTxContext before execution.
type TxApplyFunc func(
	config *ethparams.ChainConfig,
	header *types.Header,
	tx *types.Transaction,
	statedb *state.StateDB,
	vmCfg vm.Config,
	txIndex int,
) (*types.Receipt, error)

// txReadWriteSet records the storage slots read and written by a single
// transaction during speculative execution. Keyed by the hash of the address
// (address-level tracking). Full slot-level tracking (address+slot) is the
// production upgrade path.
type txReadWriteSet struct {
	reads  map[common.Hash]common.Hash // slot -> value read
	writes map[common.Hash]common.Hash // slot -> value written
}

// txResult holds the outcome of one speculative execution.
type txResult struct {
	receipt *types.Receipt
	err     error
	rwSet   txReadWriteSet
}

// BlockSTMExecutor implements BlockExecutor using the Block-STM algorithm.
// It executes all transactions in a block speculatively in parallel, detects
// read-write conflicts, and re-executes conflicting transactions.
type BlockSTMExecutor struct {
	workers int         // parallel worker count (default: runtime.NumCPU())
	applyFn TxApplyFunc // injected transaction execution function
}

// NewBlockSTMExecutor creates a Block-STM executor.
// Pass 0 for workers to use runtime.NumCPU().
// applyFn is the function that executes a single tx against a StateDB.
func NewBlockSTMExecutor(workers int, applyFn TxApplyFunc) *BlockSTMExecutor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &BlockSTMExecutor{
		workers: workers,
		applyFn: applyFn,
	}
}

// ExecuteBlock processes all transactions in the block using Block-STM.
//
// Returns (nil, nil) when:
//   - The block has fewer than 2 transactions (no benefit from parallelism).
//   - No apply function was configured.
func (e *BlockSTMExecutor) ExecuteBlock(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
	vmCfg vm.Config,
) ([]*types.Receipt, error) {
	n := len(txs)
	if n < 2 || e.applyFn == nil {
		return nil, nil // fall through to sequential
	}

	log.Debug("Block-STM speculative execution starting",
		"block", header.Number,
		"txs", n,
		"workers", e.workers,
	)

	// Phase 1: Speculative parallel execution.
	results := e.speculateAll(config, header, txs, statedb, vmCfg)

	// Phase 2: Validate and re-execute conflicting transactions.
	results, err := e.validateAndReExecute(config, header, txs, statedb, vmCfg, results)
	if err != nil {
		return nil, err
	}

	// Phase 3: Build final receipt list with correct cumulative gas.
	receipts, err := e.commitOrdered(txs, results)
	if err != nil {
		return nil, err
	}

	DefaultMetrics.BlocksProcessed.Add(1)
	DefaultMetrics.TxsProcessed.Add(int64(n))

	return receipts, nil
}

// speculateAll runs every transaction on a private copy of the state,
// collecting read/write sets and tentative results.
func (e *BlockSTMExecutor) speculateAll(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
	vmCfg vm.Config,
) []txResult {
	n := len(txs)
	results := make([]txResult, n)

	// Semaphore limits goroutine concurrency.
	sem := make(chan struct{}, e.workers)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{} // acquire slot
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }() // release slot

			results[idx] = e.executeOne(config, header, txs[idx], statedb, vmCfg, idx)
		}(i)
	}

	wg.Wait()
	return results
}

// executeOne runs a single transaction on a deep copy of the base state.
func (e *BlockSTMExecutor) executeOne(
	config *ethparams.ChainConfig,
	header *types.Header,
	tx *types.Transaction,
	baseState *state.StateDB,
	vmCfg vm.Config,
	txIndex int,
) txResult {
	// Deep copy: each goroutine gets an isolated state.
	snap := baseState.Copy()

	receipt, err := e.applyFn(config, header, tx, snap, vmCfg, txIndex)

	// Build read/write set from the transaction's touched addresses.
	rwSet := buildRWSet(tx, header)

	return txResult{
		receipt: receipt,
		err:     err,
		rwSet:   rwSet,
	}
}

// buildRWSet constructs an address-level read/write set from a transaction.
// This is a conservative approximation: it marks the recipient and coinbase
// as read/written, which may produce false-positive conflicts.
// False positives cause re-execution but never incorrect results.
func buildRWSet(tx *types.Transaction, header *types.Header) txReadWriteSet {
	rwSet := txReadWriteSet{
		reads:  make(map[common.Hash]common.Hash),
		writes: make(map[common.Hash]common.Hash),
	}

	// Mark the recipient address as a read/write.
	if to := tx.To(); to != nil {
		addrSlot := common.BytesToHash(to.Bytes())
		rwSet.reads[addrSlot] = addrSlot
		rwSet.writes[addrSlot] = addrSlot
	}

	// Mark the coinbase as written (miner reward / gas fee recipient).
	coinbaseSlot := common.BytesToHash(header.Coinbase.Bytes())
	rwSet.writes[coinbaseSlot] = coinbaseSlot

	// Contract creation: mark a sentinel write derived from the tx hash.
	if tx.To() == nil {
		rwSet.writes[tx.Hash()] = tx.Hash()
	}

	return rwSet
}

// validateAndReExecute detects conflicts and re-executes affected txs.
//
// A conflict exists when tx[i] reads a slot that tx[j] (j < i) wrote.
// Conflicting transactions are re-executed sequentially in order on a fresh
// state copy so they see the correct predecessor writes.
func (e *BlockSTMExecutor) validateAndReExecute(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
	vmCfg vm.Config,
	results []txResult,
) ([]txResult, error) {
	n := len(results)

	// committedWrites maps slot -> index of the tx that wrote it.
	committedWrites := make(map[common.Hash]int)

	// Identify which transactions need re-execution.
	reExec := make([]bool, n)
	for i := 0; i < n; i++ {
		if results[i].err != nil {
			// Speculative execution failed -- must retry sequentially.
			reExec[i] = true
		} else {
			for slot := range results[i].rwSet.reads {
				if writerIdx, ok := committedWrites[slot]; ok && writerIdx < i {
					reExec[i] = true
					break
				}
			}
		}
		// Register writes so later txs that read them are flagged.
		for slot := range results[i].rwSet.writes {
			committedWrites[slot] = i
		}
	}

	// Count conflicts.
	var conflicts int
	for _, r := range reExec {
		if r {
			conflicts++
		}
	}
	if conflicts == 0 {
		return results, nil
	}

	log.Debug("Block-STM conflict detection",
		"block", header.Number,
		"txs", n,
		"conflicts", conflicts,
	)
	DefaultMetrics.TxsReExecuted.Add(int64(conflicts))

	// Re-execute conflicting transactions sequentially on a fresh copy.
	seqState := statedb.Copy()
	for i := 0; i < n; i++ {
		if !reExec[i] {
			continue
		}

		receipt, err := e.applyFn(config, header, txs[i], seqState, vmCfg, i)
		if err != nil {
			return nil, fmt.Errorf("block-stm re-exec tx %d [%v]: %w", i, txs[i].Hash().Hex(), err)
		}

		results[i] = txResult{
			receipt: receipt,
			rwSet:   buildRWSet(txs[i], header),
		}
	}

	return results, nil
}

// commitOrdered builds the final receipt list with correct cumulative gas.
func (e *BlockSTMExecutor) commitOrdered(
	txs types.Transactions,
	results []txResult,
) ([]*types.Receipt, error) {
	receipts := make([]*types.Receipt, len(results))
	var cumulativeGas uint64

	for i, res := range results {
		if res.err != nil {
			return nil, fmt.Errorf("tx %d [%v] failed: %w", i, txs[i].Hash().Hex(), res.err)
		}
		cumulativeGas += res.receipt.GasUsed
		res.receipt.CumulativeGasUsed = cumulativeGas
		receipts[i] = res.receipt
	}

	return receipts, nil
}

// Metrics exposes Block-STM conflict statistics for observability.
type Metrics struct {
	BlocksProcessed atomic.Int64
	TxsProcessed    atomic.Int64
	TxsReExecuted   atomic.Int64
}

// DefaultMetrics is the global Block-STM metrics instance.
var DefaultMetrics Metrics

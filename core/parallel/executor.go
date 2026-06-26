// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	ethstate "github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
)

// Enabled gates parallel block execution. It is OFF by default and nothing in
// production flips it: sequential execution is the live consensus path until
// red-team and scientist review have signed off on byte-identical state roots
// across the full conformance corpus. The flag exists so the wiring is a single,
// auditable switch — never an implicit default.
var Enabled atomic.Bool

// ApplyFunc executes one transaction against the supplied vm.StateDB and returns
// its receipt. It is injected (rather than imported) so this package does not
// depend on core, which depends on it. The caller builds the EVM with the given
// state — the engine controls only the state the EVM reads and writes.
type ApplyFunc func(vmsdb vm.StateDB, txIndex int) (*types.Receipt, error)

// Executor is a deterministic, consensus-safe Block-STM block executor
// (Gelashvili et al., Aptos Labs). It executes a block's transactions
// optimistically in parallel against a shared multi-version memory, validates
// each transaction's recorded reads against the latest versions, re-executes the
// ones whose inputs changed, and iterates to a fixpoint. The fixpoint is the
// unique sequential result, so the committed state root is byte-identical to
// sequential execution — the single invariant that keeps the chain from forking.
type Executor struct {
	db          state.Database
	root        common.Hash
	txs         types.Transactions
	deleteEmpty bool
	workers     int
	apply       ApplyFunc

	mv      *mvMemory
	results []txOutcome
}

// txOutcome is the latest execution result for one transaction.
type txOutcome struct {
	receipt     *types.Receipt
	rs          *readSet
	ws          *writeSet
	err         error
	incarnation int
}

// NewExecutor builds an executor over a block's transactions. `root` is the
// pre-state root (the parent block's state root); `deleteEmpty` is the EIP-158
// rule for the block; `workers` ≤ 0 means runtime.NumCPU().
func NewExecutor(db state.Database, root common.Hash, txs types.Transactions, deleteEmpty bool, workers int, apply ApplyFunc) *Executor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &Executor{
		db:          db,
		root:        root,
		txs:         txs,
		deleteEmpty: deleteEmpty,
		workers:     workers,
		apply:       apply,
	}
}

// Execute runs the block to a fixpoint and materializes the result into
// `canonical`, returning the receipts and the resulting state root. The returned
// root MUST equal sequential execution's root; callers that require the
// consensus guarantee use ExecuteVerified instead, which enforces it.
func (e *Executor) Execute(canonical *state.StateDB) ([]*types.Receipt, common.Hash, error) {
	n := len(e.txs)
	e.mv = newMVMemory()
	e.results = make([]txOutcome, n)
	if n == 0 {
		canonical.Finalise(e.deleteEmpty)
		return nil, canonical.IntermediateRoot(e.deleteEmpty), nil
	}

	pending := make([]int, n)
	for i := range pending {
		pending[i] = i
	}

	// A transaction is SETTLED once it executes without error and every read it
	// recorded still resolves to the same value (err == nil && consistent). The
	// rest are pending and re-execute next round.
	//
	// Convergence + genuine-failure detection rest on one invariant: the lowest
	// unsettled index L has a fully settled, FROZEN prefix (all j < L are settled,
	// hence not re-executed, hence their multi-version writes are fixed). So L
	// re-executes against final inputs. For a valid block L therefore settles and
	// the lowest unsettled index strictly increases — at most n rounds. If a round
	// fails to advance it, L cannot settle against a final prefix: a genuine
	// failure (e.g. an invalid nonce or insufficient balance). The engine then
	// fails secure and the caller runs sequential, which surfaces the real error.
	prevLowest := -1
	maxRounds := n + 2
	for round := 0; round < maxRounds; round++ {
		e.executeParallel(pending)
		consistent := e.validateParallel(n)

		pending = pending[:0]
		for i := 0; i < n; i++ {
			if e.results[i].err != nil || !consistent[i] {
				pending = append(pending, i) // built in index order ⇒ pending[0] is the minimum
			}
		}
		if len(pending) == 0 {
			return e.commit(canonical)
		}
		if lowest := pending[0]; lowest <= prevLowest {
			return nil, common.Hash{}, fmt.Errorf("block-stm: transaction %d cannot settle against a final prefix: %w", lowest, e.settleError(lowest))
		} else {
			prevLowest = lowest
		}
		DefaultMetrics.TxsReExecuted.Add(int64(len(pending)))
	}
	return nil, common.Hash{}, fmt.Errorf("block-stm: did not converge in %d rounds (%d txs)", maxRounds, n)
}

// settleError reports why transaction i failed to settle, for the fail-secure
// bail path.
func (e *Executor) settleError(i int) error {
	if e.results[i].err != nil {
		return e.results[i].err
	}
	return fmt.Errorf("read set remained inconsistent against a final prefix")
}

// ExecuteVerified is the fail-secure entry point. It runs the engine on a COPY of
// the pre-state and only mutates `pre` (returning the receipts) if the parallel
// state root byte-equals referenceRoot — the authoritative sequential/consensus
// root. On disabled, error, or ANY root divergence it returns ok=false and leaves
// `pre` untouched so the caller runs sequential. A parallel result that does not
// reproduce the consensus root is never accepted.
func (e *Executor) ExecuteVerified(pre *state.StateDB, referenceRoot common.Hash) (receipts []*types.Receipt, ok bool) {
	if !Enabled.Load() {
		return nil, false
	}
	probe := pre.Copy()
	receipts, root, err := e.Execute(probe)
	if err != nil || root != referenceRoot {
		return nil, false
	}
	// Byte-identical: replay the same writes onto the real pre-state.
	e.materialize(pre)
	pre.Finalise(e.deleteEmpty)
	return receipts, true
}

// executeParallel runs the given transaction indices, each on its own
// speculative StateDB reading through the multi-version layer.
func (e *Executor) executeParallel(indices []int) {
	sem := make(chan struct{}, e.workers)
	var wg sync.WaitGroup
	for _, i := range indices {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			e.executeOne(i)
		}(i)
	}
	wg.Wait()
}

// executeOne speculatively executes transaction i against a fresh StateDB whose
// reads route through the multi-version layer and whose writes are captured.
func (e *Executor) executeOne(i int) {
	prior := e.results[i]
	inc := 0
	if prior.rs != nil {
		inc = prior.incarnation + 1
	}

	baseReader, err := e.db.Reader(e.root)
	if err != nil {
		e.results[i] = txOutcome{err: err, incarnation: inc}
		return
	}
	rs := newReadSet()
	mvr := &mvReader{tx: i, base: baseReader, mv: e.mv, rs: rs}
	inner, err := ethstate.NewWithReader(e.root, e.db, mvr)
	if err != nil {
		e.results[i] = txOutcome{err: err, incarnation: inc}
		return
	}
	inner.SetTxContext(e.txs[i].Hash(), i)

	wc := newWriteCapture()
	hooked := ethstate.NewHookedState(inner, wc.hooks())

	receipt, execErr := e.apply(hooked, i)
	ws := buildWriteSet(inner, wc)
	e.mv.record(i, inc, ws)

	e.results[i] = txOutcome{receipt: receipt, rs: rs, ws: ws, err: execErr, incarnation: inc}
}

// validateParallel re-checks every transaction's recorded reads against the
// current multi-version layer, returning per-index consistency.
func (e *Executor) validateParallel(n int) []bool {
	consistent := make([]bool, n)
	sem := make(chan struct{}, e.workers)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			consistent[i] = e.readsConsistent(i)
		}(i)
	}
	wg.Wait()
	return consistent
}

// readsConsistent reports whether every read transaction i recorded still
// resolves to the same value against the current multi-version layer. The base
// pre-state is immutable, so a base-sourced read is consistent iff no lower
// transaction has since written that location.
func (e *Executor) readsConsistent(i int) bool {
	rs := e.results[i].rs
	if rs == nil {
		return false
	}
	for _, o := range rs.obs {
		switch o.key.kind {
		case accountKey:
			mvVal, ok := e.mv.readAccount(o.key.addr, i)
			if o.fromMV {
				if !ok || !mvVal.equal(o.resolved) {
					return false
				}
			} else if ok {
				return false
			}
		case storageKey:
			val, wiped, ok := e.mv.readStorage(o.key.addr, o.key.slot, i)
			nowControlled := ok || wiped
			nowVal := common.Hash{}
			if ok && !wiped {
				nowVal = val
			}
			if o.fromMV {
				if !nowControlled || nowVal != o.val {
					return false
				}
			} else if nowControlled {
				return false
			}
		}
	}
	return true
}

// commit materializes the fixpoint into canonical state and assembles receipts
// with correct cumulative gas.
func (e *Executor) commit(canonical *state.StateDB) ([]*types.Receipt, common.Hash, error) {
	e.materialize(canonical)
	canonical.Finalise(e.deleteEmpty)
	root := canonical.IntermediateRoot(e.deleteEmpty)

	receipts := make([]*types.Receipt, len(e.txs))
	var cumulative uint64
	for i := range e.results {
		r := e.results[i].receipt
		if r == nil {
			return nil, common.Hash{}, fmt.Errorf("block-stm tx %d: nil receipt", i)
		}
		cumulative += r.GasUsed
		r.CumulativeGasUsed = cumulative
		receipts[i] = r
	}
	DefaultMetrics.BlocksProcessed.Add(1)
	DefaultMetrics.TxsProcessed.Add(int64(len(e.txs)))
	return receipts, root, nil
}

// materialize replays the fixpoint write sets onto canonical state in
// transaction order. Because each location's final value is published by its
// highest writer and replayed last, the canonical trie ends byte-identical to
// sequential execution.
func (e *Executor) materialize(canonical *state.StateDB) {
	for i := range e.results {
		ws := e.results[i].ws
		if ws == nil {
			continue
		}
		destructed := make(map[common.Address]bool)
		for _, aw := range ws.accounts {
			if !aw.val.exists {
				canonical.SelfDestruct(aw.addr)
				destructed[aw.addr] = true
				continue
			}
			bal := aw.val.balance
			canonical.SetBalance(aw.addr, &bal, tracing.BalanceChangeUnspecified)
			canonical.SetNonce(aw.addr, aw.val.nonce, tracing.NonceChangeUnspecified)
			if aw.val.codeHash != types.EmptyCodeHash && aw.val.codeHash != (common.Hash{}) {
				if canonical.GetCodeHash(aw.addr) != aw.val.codeHash {
					canonical.SetCode(aw.addr, aw.val.code, tracing.CodeChangeUnspecified)
				}
			}
		}
		for _, sw := range ws.storage {
			if destructed[sw.addr] {
				continue
			}
			canonical.SetState(sw.addr, sw.slot, sw.val)
		}
	}
}

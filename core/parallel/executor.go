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
	gasLimit    uint64
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
// pre-state root (the parent block's state root); `gasLimit` is the block gas
// limit, enforced as a faithful GasPool reservation in the sequential reference
// so an over-limit block is rejected exactly as the live path rejects it;
// `deleteEmpty` is the EIP-158 rule for the block; `workers` ≤ 0 means
// runtime.NumCPU().
func NewExecutor(db state.Database, root common.Hash, txs types.Transactions, gasLimit uint64, deleteEmpty bool, workers int, apply ApplyFunc) *Executor {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &Executor{
		db:          db,
		root:        root,
		txs:         txs,
		gasLimit:    gasLimit,
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

// ExecuteVerified is the fail-secure entry point. The parity reference is the
// LOCALLY COMPUTED sequential root — never the caller-supplied `referenceRoot` —
// and is recomputed here every block (no speedup). This pins the safety contract
// in code: a future caller cannot wire referenceRoot = header.Root to skip
// sequential and have a divergent parallel result silently accepted. The engine
// is not trusted to be ≡ sequential until independently proven across the full
// conformance corpus; until then this gate re-derives sequential and refuses any
// parallel result that does not byte-equal it.
//
// `referenceRoot` is retained as a mandatory cross-check: it MUST equal the
// locally computed sequential root, so a caller that passes a forged or merely
// claimed root (e.g. an untrusted header.Root) is rejected rather than trusted.
// Because DummyEngine.Finalize mutates no state, the block's header.Root IS the
// post-transaction state root, so the live caller passes block.Root() here and a
// forged header is caught in this gate rather than only downstream.
//
// `pre` is mutated (and receipts returned) only when sequential, the caller's
// reference, and the parallel result all agree byte-for-byte; otherwise ok=false
// and `pre` is left untouched so the caller runs sequential itself.
//
// The RETURNED receipts are the canonical sequential ones, NOT the parallel
// engine's: each parallel transaction runs on its own speculative StateDB whose
// per-block log index counter restarts at zero, so the parallel receipts carry
// tx-local log indices and would derive a wrong receipt-trie root. The sequential
// reference accumulates every transaction's logs on one StateDB, so its receipts
// are block-correct. The committed STATE comes from materializing the parallel
// write sets (proven byte-identical to sequential by the determinism corpus); the
// committed RECEIPTS come from the sequential reference. Both halves are therefore
// exactly what a sequential validator commits.
func (e *Executor) ExecuteVerified(pre *state.StateDB, referenceRoot common.Hash) (receipts []*types.Receipt, ok bool) {
	if !Enabled.Load() {
		return nil, false
	}
	// The authoritative reference: recompute sequential locally, capturing the
	// canonical receipts and root. This is the only root the parallel result is
	// ever checked against, and the only receipts ever committed.
	seqReceipts, seqRoot, seqErr := e.executeSequential(pre.Copy())
	if seqErr != nil {
		return nil, false
	}
	// The caller's claimed root must match local truth. header.Root can therefore
	// never serve as the parity reference: if it diverges from local sequential we
	// fail secure here, before the parallel engine's output is ever consulted.
	if referenceRoot != seqRoot {
		return nil, false
	}
	// Parallel cross-check on an independent copy. Its receipts are intentionally
	// discarded (tx-local log indices); only its ROOT is consensus-comparable.
	probe := pre.Copy()
	if _, parRoot, err := e.Execute(probe); err != nil || parRoot != seqRoot {
		return nil, false
	}
	// Byte-identical to the locally computed sequential root: replay the parallel
	// write sets onto pre. materialize reproduces the per-transaction Finalise
	// cadence, leaving pre fully finalised at seqRoot.
	e.materialize(pre)
	DefaultMetrics.VerifiedBlocks.Add(1)
	return seqReceipts, true
}

// executeSequential runs every transaction in order on `sdb`, Finalising after
// each (the post-Byzantium cadence), and returns the canonical receipts and the
// resulting state root. It is the authoritative sequential reference for
// ExecuteVerified. It is intentionally NOT a fast path: it provides no
// parallelism and exists solely so the parity reference — root AND receipts — is
// always locally computed, never a caller-supplied value.
//
// It also reproduces the live path's block gas-limit enforcement. geth's GasPool
// reserves each transaction's gas limit up front (SubGas) and refunds the unused
// remainder, so after transaction j the pool holds blockGasLimit − Σ gasUsed and
// transaction i is admitted iff Σ_{j<i} gasUsed_j + gasLimit_i ≤ blockGasLimit.
// An over-limit block therefore fails the sequential reference here, so
// ExecuteVerified returns ok=false and the caller's own sequential loop surfaces
// the identical ErrGasLimitReached — a flag-on validator can never accept a block
// (e.g. one whose later transaction cannot reserve its gas) that a flag-off
// validator rejects. The pure-root parallel cross-check does not model the pool;
// pinning the gate to this sequential reference is what closes that fork class.
func (e *Executor) executeSequential(sdb *state.StateDB) ([]*types.Receipt, common.Hash, error) {
	receipts := make([]*types.Receipt, len(e.txs))
	var cumulative uint64 // Σ gasUsed; invariant: cumulative ≤ e.gasLimit
	for i := range e.txs {
		if e.txs[i].Gas() > e.gasLimit-cumulative { // faithful SubGas(gasLimit) reservation
			return nil, common.Hash{}, fmt.Errorf("block-stm: gas limit reached at tx %d (block limit %d, used %d, tx wants %d)", i, e.gasLimit, cumulative, e.txs[i].Gas())
		}
		sdb.SetTxContext(e.txs[i].Hash(), i)
		r, err := e.apply(sdb, i)
		if err != nil {
			return nil, common.Hash{}, err
		}
		sdb.Finalise(e.deleteEmpty)
		cumulative += r.GasUsed
		r.CumulativeGasUsed = cumulative
		receipts[i] = r
	}
	return receipts, sdb.IntermediateRoot(e.deleteEmpty), nil
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
	ws := buildWriteSet(inner, wc, rs, e.deleteEmpty)
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
	e.materialize(canonical) // finalises per transaction; the journal is already clear
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

// materialize replays the fixpoint write sets onto canonical state, reproducing
// sequential execution's per-transaction Finalise cadence EXACTLY: each
// transaction's net write set is applied and then Finalise(deleteEmpty) is run,
// just as sequential does between transactions. This is what preserves the
// delete-then-recreate boundary of a resurrection and the EIP-158/161
// empty-account deletion — a single trailing Finalise collapses those into the
// wrong order and forks (an account self-destructed by tx A then revived by tx B
// would be deleted, or an empty account touched by a tx would survive). Because
// each write set already carries the post-Finalise truth of its transaction
// (buildWriteSet ran the same Finalise to compute it), replaying it under the
// same cadence reconstructs sequential's intermediate states one for one, so the
// canonical trie ends byte-identical to sequential execution.
func (e *Executor) materialize(canonical *state.StateDB) {
	for i := range e.results {
		if ws := e.results[i].ws; ws != nil {
			applyWriteSet(canonical, ws)
		}
		// Reproduce sequential's per-transaction Finalise. A transaction that
		// produced no writes still finalises (clearing any touched-empty account)
		// so the cadence — and therefore the destruct/empty bookkeeping that drives
		// resurrection — matches sequential one transaction at a time.
		canonical.Finalise(e.deleteEmpty)
	}
}

// applyWriteSet applies one transaction's net write set onto canonical state. An
// account marked absent is self-destructed (its storage is wiped, so its storage
// writes are skipped); an account marked present has its balance, nonce and code
// set. Code is replayed only when its hash actually changed. The subsequent
// Finalise (in materialize) realizes destructs/empties before the next
// transaction's writes are applied, so a later revival sees a deleted account and
// recreates it fresh — exactly as the live EVM does.
func applyWriteSet(canonical *state.StateDB, ws *writeSet) {
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

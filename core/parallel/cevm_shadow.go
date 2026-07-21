// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// cevm_shadow.go wires the Lux C++ EVM (cevm) into the block executor as a
// consensus-SAFE, apply-then-verify GATED APPLIER (a strict superset of the
// original byte-exact shadow verifier).
//
// The map proved two facts that lock this design:
//   1. parallel.RegisterExecutor(e) makes core/state_processor.go route EVERY
//      block through e.ExecuteBlock (no SetBackend needed).
//   2. The CONSENSUS state root is taken from the GO statedb.IntermediateRoot()
//      (block_validator.go), NOT from cevm's returned root. So for cevm to be
//      the APPLIER it must write its post-execution state into the Go statedb,
//      after which block_validator recomputes IntermediateRoot over that
//      written-back state — which must equal cevm's root (== header.Root).
//
// So this executor: (a) keeps a RESIDENT cevm StateDB per chain (held C-side
// behind an opaque handle) and dumps + seeds the pre-block Go state into it only
// ONCE per sync — the first block for a chain, or a resync after a drift — NOT
// every block; (b) applies each block's transactions against that resident state
// (no dump on the in-sync path) through cevm's real process_block, whose
// incremental commit() re-hashes only the O(changed) paths, to get cevm's
// independent keccak-MPT root + per-tx results + logs + a POST-STATE DELTA (ABI
// v2); (c) cross-checks cevm's root against header.Root (the post-state root the
// Go EVM produced when the block was built), tallying
// agree/disagree/finalize-gap/declined counters.
//
// The resident state is what kills the per-block full-state dump (the
// declined-too-large decline at scale): across N blocks cevm dumps the full
// state ~ONCE, not N times. Correctness is unchanged — the resident root is
// verified against the header at every height, and on any mismatch the resident
// is re-seeded from the (correct) Go state and the block is declined to the Go
// EVM, so a drifted resident can never commit wrong state.
//
// When the roots AGREE, it PROMOTES to applier under a four-way safety gate:
//   - write cevm's post-state delta into an ISOLATED COPY of the Go statedb;
//   - compute that copy's IntermediateRoot(IsEIP158) and rebuild the receipts;
//   - keep the writeback ONLY IF all four consensus quantities byte-match the
//     header: state root, aggregate bloom, receipt-trie hash, and cumulative
//     gas. Then apply the identical delta to the LIVE statedb and return the
//     receipts — cevm has now EXECUTED the block and the Go EVM will NOT re-run
//     it. If ANY of the four mismatches (or a tx was rejected, or a blob tx is
//     present), the live statedb is NEVER touched and it returns (nil, nil) so
//     the Go EVM applies. A wrong or incomplete delta can therefore never be
//     committed — the gate is consensus-safe BY CONSTRUCTION.
//
// The trial writeback runs on statedb.Copy() (not the live statedb) precisely
// because IntermediateRoot -> Finalise -> clearJournalAndRefund invalidates the
// journal, which would make a post-hoc RevertToSnapshot unreliable. Verifying on
// an isolated copy and only replaying onto the live statedb after a full match
// sidesteps that entirely: on any mismatch there is nothing to revert.
//
// The proven safe subset (pure value transfers + non-reverting calls into
// existing code) yields a byte-exact match and is APPLIED by cevm; everything
// else (top-level CREATE, REVERT/OOG, EIP-158 empty-account divergence, EIP-2930
// access lists, EIP-3529 refunds, withdrawals, or a state too large to snapshot)
// is tallied as a decline/disagree/reverted-mismatch and the Go EVM applies it
// unchallenged. Declined blocks fall through with the live state untouched, so
// the suite stays correct.
//
// This file is intentionally NOT behind //go:build cevm: it always compiles and
// always registers. When built without -tags cevm, cevmbridge.Enabled is false
// and ExecuteBlock returns (nil, nil) immediately — a behavioural no-op
// identical to the fallback executor, so the default build is unchanged.

import (
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/holiman/uint256"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/core/parallel/cevmbridge"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
	"github.com/luxfi/geth/trie"
)

// Seed caps. The resident path dumps the full pre-state only on a SEED/RESYNC
// (not every block), so these bound that one dump: a chain whose state exceeds
// them cannot be seeded and is declined (declined-too-large) rather than risk an
// unbounded dump. Because seeds are rare (~once per chain), this cap no longer
// fires per block the way the old dump-every-block shadow did — that is the
// declined-too-large fix. As the SOLE mainnet engine cevm would drop the cap
// entirely and own disk-backed state; here it stays a bounded correctness harness.
const (
	maxShadowAccounts = 200_000
	maxShadowStorage  = 1_000_000
)

// Package-level shadow counters (atomic). Read via CevmShadowStats.
var (
	cevmProcessed          uint64 // blocks cevm actually ran and returned a root for
	cevmAgree              uint64 // cevm root == header.Root (byte-exact)
	cevmDisagree           uint64 // cevm root != header.Root, no known-alignment excuse
	cevmFinalizeGap        uint64 // mismatch attributable to withdrawals (cevm runs no engine.Finalize)
	cevmDeclinedTooLarge   uint64 // pre-state exceeded the snapshot caps
	cevmDeclinedNoPreimage uint64 // an account address was unrecoverable (preimages off) — cannot seed cevm
	cevmDeclinedTx         uint64 // a tx sender could not be recovered
	cevmErrored            uint64 // cevm process_block failed / panicked
	cevmApplied            uint64 // cevm APPLIED the block: four-way gate passed, receipts returned
	cevmRevertedMismatch   uint64 // agreed on root but the gated writeback/gate failed → declined to Go

	// Resident-applier counters. The applier holds a PERSISTENT cevm StateDB per
	// chain and applies each block against it (no per-block dump). These measure
	// the declined-too-large-killer: full-state seeds should be ~1 (genesis + rare
	// resyncs), NOT N.
	cevmResidentApplied uint64 // blocks applied via the resident handle (subset of Applied)
	cevmResidentSeeds   uint64 // full-state dumps+seeds into the resident StateDB (initial + resyncs)
	cevmResidentReseeds uint64 // subset of seeds that were drift RESYNCS (not the first seed)
)

// ShadowStats is a snapshot of the cevm shadow-verifier / applier counters.
// Invariant: Applied + RevertedMismatch == Agree — every agreeing block is put
// through the four-way apply gate, which either applies it or declines to Go.
type ShadowStats struct {
	Processed          uint64
	Agree              uint64
	Disagree           uint64
	FinalizeGap        uint64
	DeclinedTooLarge   uint64
	DeclinedNoPreimage uint64
	DeclinedTx         uint64
	Errored            uint64
	Applied            uint64
	RevertedMismatch   uint64
	// Resident-applier metrics.
	ResidentApplied uint64
	ResidentSeeds   uint64 // full-state seeds (the declined-too-large-killer number)
	ResidentReseeds uint64
}

// CevmShadowStats returns the current cevm shadow-verifier counters.
func CevmShadowStats() ShadowStats {
	return ShadowStats{
		Processed:          atomic.LoadUint64(&cevmProcessed),
		Agree:              atomic.LoadUint64(&cevmAgree),
		Disagree:           atomic.LoadUint64(&cevmDisagree),
		FinalizeGap:        atomic.LoadUint64(&cevmFinalizeGap),
		DeclinedTooLarge:   atomic.LoadUint64(&cevmDeclinedTooLarge),
		DeclinedNoPreimage: atomic.LoadUint64(&cevmDeclinedNoPreimage),
		DeclinedTx:         atomic.LoadUint64(&cevmDeclinedTx),
		Errored:            atomic.LoadUint64(&cevmErrored),
		Applied:            atomic.LoadUint64(&cevmApplied),
		RevertedMismatch:   atomic.LoadUint64(&cevmRevertedMismatch),
		ResidentApplied:    atomic.LoadUint64(&cevmResidentApplied),
		ResidentSeeds:      atomic.LoadUint64(&cevmResidentSeeds),
		ResidentReseeds:    atomic.LoadUint64(&cevmResidentReseeds),
	}
}

// ResetCevmShadowStats zeroes the counters and frees any resident cevm StateDB
// handles (test helper). Clearing the resident registry makes each test start
// from a clean "first block seeds once" state, independent of prior tests.
func ResetCevmShadowStats() {
	for _, p := range []*uint64{
		&cevmProcessed, &cevmAgree, &cevmDisagree, &cevmFinalizeGap,
		&cevmDeclinedTooLarge, &cevmDeclinedNoPreimage, &cevmDeclinedTx, &cevmErrored,
		&cevmApplied, &cevmRevertedMismatch,
		&cevmResidentApplied, &cevmResidentSeeds, &cevmResidentReseeds,
	} {
		atomic.StoreUint64(p, 0)
	}
	resetResidents()
}

// CevmShadowEnabled reports whether the real cevm bridge is linked (-tags cevm).
func CevmShadowEnabled() bool { return cevmbridge.Enabled }

func init() {
	// Register only when the real C++ EVM is linked (-tags cevm). In the default
	// and -tags parallel builds the bridge is a no-op stub, so registering would
	// only risk clobbering another package's real BlockExecutor (e.g. a Block-STM
	// engine) with a verifier that always declines. Gating on Enabled keeps those
	// builds byte-for-byte unchanged while activating the shadow under -tags cevm.
	if cevmbridge.Enabled {
		RegisterExecutor(cevmShadowExecutor{})
	}
}

// cevmShadowExecutor is a BlockExecutor that verifies cevm's independent
// keccak-MPT root against the header and, when they byte-match under the
// four-way gate, APPLIES cevm's post-state into the live statedb and returns its
// receipts; on any mismatch or decline reason it returns (nil, nil) so the Go
// EVM applies the block, leaving the live statedb untouched.
type cevmShadowExecutor struct{}

// ExecuteBlock runs the cevm cross-check and, on a four-way byte-match, APPLIES
// cevm's execution by writing its post-state delta into the live statedb and
// returning the reconstructed receipts. On any mismatch, decline reason, or
// internal failure it returns (nil, nil) with the live statedb untouched, so the
// Go EVM applies the block — cevm can therefore never destabilise consensus.
func (cevmShadowExecutor) ExecuteBlock(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
	_ vm.Config,
) (receipts []*types.Receipt, err error) {
	if !cevmbridge.Enabled || config == nil || header == nil || statedb == nil || len(txs) == 0 {
		return nil, nil
	}
	// A panic anywhere in the snapshot, bridge, or gate must NEVER destabilise
	// consensus: swallow it, count it, and decline. All risky work (bridge cgo,
	// trial writeback, root/receipt hashing) runs BEFORE the single live
	// writeback, so a panic always leaves the live statedb pristine.
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&cevmErrored, 1)
			receipts, err = nil, nil
		}
	}()
	return shadowApply(config, header, txs, statedb), nil
}

// residentState tracks the PERSISTENT cevm StateDB for one chain: its opaque
// handle and whether that resident state is known-good at a given block height.
// The handle is created once and reused; the resident state advances one block
// at a time as the applier applies blocks, and is re-seeded (a full-state dump)
// only on the first block or after a drift/decline knocks it out of sync.
type residentState struct {
	handle     uint64
	haveHandle bool   // handle allocated (StateCreate called)
	everSeeded bool   // at least one full-state seed has happened (first seed vs resync)
	synced     bool   // resident state is verified-good at `height`
	height     uint64 // block number the resident state currently holds (post-block)
}

var (
	residentMu sync.Mutex
	residents  = map[string]*residentState{}
)

// getResident returns the per-chain resident state (keyed by chain ID), creating
// the entry and lazily its cevm handle on first use. Block execution is serial
// per chain, so the returned *residentState is mutated without a lock by the
// single in-flight ExecuteBlock; only the registry map is guarded here.
func getResident(config *ethparams.ChainConfig) *residentState {
	key := "?"
	if config != nil && config.ChainID != nil {
		key = config.ChainID.String()
	}
	residentMu.Lock()
	defer residentMu.Unlock()
	rs := residents[key]
	if rs == nil {
		rs = &residentState{}
		residents[key] = rs
	}
	if !rs.haveHandle {
		rs.handle = cevmbridge.StateCreate()
		rs.haveHandle = true
	}
	return rs
}

// resetResidents frees all resident cevm StateDB handles and clears the registry
// (test helper, called by ResetCevmShadowStats).
func resetResidents() {
	residentMu.Lock()
	defer residentMu.Unlock()
	for _, rs := range residents {
		if rs.haveHandle {
			cevmbridge.StateFree(rs.handle)
		}
	}
	residents = map[string]*residentState{}
}

// shadowApply cross-checks cevm's independent keccak-MPT root against the header
// and, on a byte-match under the four-way gate, APPLIES cevm's post-state into
// the live Go statedb. It runs cevm against a RESIDENT StateDB held C-side per
// chain, so state lives ACROSS blocks: the pre-block Go state is dumped and
// seeded into cevm only ONCE per sync (the first block for a chain, or a resync
// after a drift/decline), NOT every block. On the common in-sync path it applies
// the block against the resident tries with NO dump — cevm's incremental commit()
// then costs O(changed), not O(state). Every outcome updates exactly one verifier
// counter; a wrong resident root is caught by the root check (disagree) and the
// four-way apply gate, so a drifted resident can never commit wrong state — it
// only forces a resync on the next block.
func shadowApply(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
) []*types.Receipt {
	rs := getResident(config)

	// Executed blocks have Number >= 1, so `parent` is the height cevm's resident
	// state must already hold to apply this block without a full reseed.
	var number, parent uint64
	if header.Number != nil {
		number = header.Number.Uint64()
		if number > 0 {
			parent = number - 1
		}
	}

	// (a) (Re)seed the resident StateDB when it is not already at the parent
	// height. This is the ONLY full-state dump — once per sync, not per block.
	// DumpToCollector iterates the trie at statedb's original (parent) root, which
	// is exactly the pre-block state the block was applied to.
	if !rs.synced || rs.height != parent {
		snap := &shadowSnapshot{maxAccts: maxShadowAccounts, maxStorage: maxShadowStorage}
		statedb.DumpToCollector(snap, &state.DumpConfig{})
		switch {
		case snap.tooLarge:
			atomic.AddUint64(&cevmDeclinedTooLarge, 1)
			rs.synced = false
			return nil
		case snap.missingAddr || snap.badBalance:
			// Addresses come from trie-key preimages; without them cevm cannot be
			// seeded (its leaf key is keccak256(addr)). Decline honestly.
			atomic.AddUint64(&cevmDeclinedNoPreimage, 1)
			rs.synced = false
			return nil
		}
		cevmbridge.StateSeed(rs.handle, snap.accts, snap.storage)
		atomic.AddUint64(&cevmResidentSeeds, 1)
		if rs.everSeeded {
			atomic.AddUint64(&cevmResidentReseeds, 1)
		}
		rs.everSeeded = true
		rs.synced = true
		rs.height = parent // resident now holds the parent state
	}

	// (b) Build the cevm tx array, retaining each recovered sender for
	// ContractAddress reconstruction on CREATE receipts.
	ctxs, froms, ok := buildCevmTxs(config, header, txs)
	if !ok {
		atomic.AddUint64(&cevmDeclinedTx, 1)
		rs.synced = false // Go will apply this block; resident falls behind → resync next
		return nil
	}

	// Pessimistically flag the resident out-of-sync BEFORE the apply: any error,
	// panic, or root mismatch below then forces a resync next block. It is
	// re-marked synced only after cevm's root is verified against the header.
	rs.synced = false

	// (c) Apply the block against the RESIDENT StateDB (no reseed): cevm's new
	// root + per-tx results + logs + post-state delta (from the commit change-set).
	res, err := cevmbridge.StateApplyBlock(rs.handle, ctxs, blockCtx(config, header))
	if err != nil || !res.OK {
		atomic.AddUint64(&cevmErrored, 1)
		return nil
	}
	atomic.AddUint64(&cevmProcessed, 1)

	// (d) Cross-check cevm's resident root against the root the Go EVM committed.
	if common.BytesToHash(res.StateRoot[:]) != header.Root {
		// Known-alignment gap: cevm credits the coinbase per-tx (gas*gasPrice,
		// which matches evm's full-fee-to-coinbase state_transition) but does NOT
		// run engine.Finalize. On Lux that Finalize is a state no-op EXCEPT for
		// EIP-4895 withdrawals, which move header.Root. Attribute such mismatches
		// to the finalize gap rather than blaming cevm.
		if header.WithdrawalsHash != nil && *header.WithdrawalsHash != types.EmptyWithdrawalsHash {
			atomic.AddUint64(&cevmFinalizeGap, 1)
		} else {
			atomic.AddUint64(&cevmDisagree, 1)
		}
		// cevm's resident state advanced to a NON-matching state → stays unsynced,
		// so the next block re-seeds from the (correct) Go state.
		return nil
	}
	atomic.AddUint64(&cevmAgree, 1)

	// Roots agree: promote to APPLIER under the four-way safety gate. On any
	// mismatch the gate declines with the live statedb untouched (Go EVM applies).
	receipts := applyAgreedBlock(config, header, txs, froms, statedb, res)
	if receipts == nil {
		atomic.AddUint64(&cevmRevertedMismatch, 1)
		// The root agreed, so cevm's resident state IS correct at this height; the
		// decline was only a receipt-reconstruction gap and the Go EVM reaches the
		// identical state, so the resident stays in sync.
		rs.synced = true
		rs.height = number
		return nil
	}
	atomic.AddUint64(&cevmApplied, 1)
	atomic.AddUint64(&cevmResidentApplied, 1)
	rs.synced = true
	rs.height = number // resident verified at height N
	return receipts
}

// buildCevmTxs converts the block's transactions into the cevm bridge tx array,
// returning the recovered senders (for CREATE-receipt ContractAddress
// reconstruction) and ok=false if any sender is unrecoverable.
func buildCevmTxs(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
) (ctxs []cevmbridge.Tx, froms []common.Address, ok bool) {
	signer := types.MakeSigner(config, header.Number, header.Time)
	ctxs = make([]cevmbridge.Tx, len(txs))
	froms = make([]common.Address, len(txs))
	for i, tx := range txs {
		from, err := types.Sender(signer, tx)
		if err != nil {
			return nil, nil, false
		}
		froms[i] = from
		var t cevmbridge.Tx
		copy(t.Sender[:], from[:])
		if to := tx.To(); to != nil {
			copy(t.Recipient[:], to[:])
		} else {
			t.IsCreate = true
		}
		fillBE32(&t.Value, tx.Value())
		fillBE32(&t.GasPrice, effectiveGasPrice(tx, header.BaseFee))
		t.GasLimit = tx.Gas()
		t.Nonce = tx.Nonce()
		if d := tx.Data(); len(d) > 0 {
			t.Data = d
		}
		// EIP-2930 access list (nil for legacy txs). Carrying it lets cevm charge
		// the intrinsic surcharge (2400/addr + 1900/key) and pre-warm the entries,
		// so its gas_used — and thus the coinbase credit and keccak-MPT root —
		// matches the Go EVM on an access-list-bearing fee block.
		if al := tx.AccessList(); len(al) > 0 {
			t.AccessList = make([]cevmbridge.AccessTuple, len(al))
			for j := range al {
				copy(t.AccessList[j].Address[:], al[j].Address[:])
				if len(al[j].StorageKeys) > 0 {
					t.AccessList[j].StorageKeys = make([][32]byte, len(al[j].StorageKeys))
					for k := range al[j].StorageKeys {
						copy(t.AccessList[j].StorageKeys[k][:], al[j].StorageKeys[k][:])
					}
				}
			}
		}
		ctxs[i] = t
	}
	return ctxs, froms, true
}

// applyAgreedBlock is entered ONLY when cevm's root already byte-matches
// header.Root. It promotes cevm from verifier to APPLIER under the four-way
// consensus gate — the identical quantities block_validator.ValidateState
// checks: state root, aggregate bloom, receipt-trie hash, and cumulative gas.
//
// It reconstructs the receipts, replays cevm's post-state delta onto an ISOLATED
// COPY of the statedb, and keeps the writeback ONLY IF all four byte-match the
// header. On a full match it replays the identical delta onto the LIVE statedb
// and returns the receipts (cevm has executed the block); on ANY mismatch — or an
// unrepresentable block (blob tx, cevm-rejected tx, tx-result count skew) — it
// returns nil WITHOUT touching the live statedb, so the Go EVM applies.
//
// The trial runs on statedb.Copy() rather than a Snapshot()/RevertToSnapshot()
// pair because IntermediateRoot → Finalise → clearJournalAndRefund invalidates
// the journal, making a post-IntermediateRoot revert unreliable. Verifying on a
// deep copy leaves the live statedb pristine on every decline (nothing to
// revert) and is consensus-safe by construction: a wrong or incomplete delta can
// never reach the live state.
func applyAgreedBlock(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	froms []common.Address,
	statedb *state.StateDB,
	res cevmbridge.Result,
) []*types.Receipt {
	n := len(txs)
	// Unrepresentable-block pre-checks: decline before any state work.
	if len(res.TxResults) != n {
		return nil
	}
	for i := range txs {
		if txs[i].Type() == types.BlobTxType {
			return nil // blob receipts carry BlobGasUsed/Price we do not reconstruct
		}
		if res.TxResults[i].Rejected {
			return nil // block includes a tx cevm deems non-includable
		}
	}

	receipts := buildCevmReceipts(header, txs, froms, res)

	// Trial writeback + verification on an isolated deep copy — the live statedb
	// is NOT touched here.
	trial := statedb.Copy()
	writePostState(trial, res)
	root := trial.IntermediateRoot(config.IsEIP158(header.Number))

	// FOUR-WAY consensus gate. Any mismatch ⇒ decline; live statedb untouched.
	if root != header.Root {
		return nil
	}
	var rbloom types.Bloom
	for _, r := range receipts {
		b := types.CreateBloom(r)
		for k := range rbloom {
			rbloom[k] |= b[k]
		}
	}
	if rbloom != header.Bloom {
		return nil
	}
	if types.DeriveSha(types.Receipts(receipts), trie.NewStackTrie(nil)) != header.ReceiptHash {
		return nil
	}
	if receipts[n-1].CumulativeGasUsed != header.GasUsed {
		return nil
	}

	// All four byte-match: cevm's execution reproduces the header exactly. Replay
	// the identical delta onto the LIVE statedb and hand back cevm's receipts —
	// state_processor will NOT re-run the Go EVM for this block.
	writePostState(statedb, res)
	return receipts
}

// buildCevmReceipts reconstructs the block's receipts from cevm's per-tx results
// + emitted logs, mirroring core/state_processor.go applyTransaction (post-
// Byzantium: PostState=nil, Status from the per-tx flag, ContractAddress via
// crypto.CreateAddress for CREATE, logs attached by tx index with a block-wide
// running index, per-receipt Bloom). The result is gate-checked against the
// header before any receipt is returned.
func buildCevmReceipts(
	header *types.Header,
	txs types.Transactions,
	froms []common.Address,
	res cevmbridge.Result,
) []*types.Receipt {
	blockHash := header.Hash()
	blockNum := header.Number
	receipts := make([]*types.Receipt, len(txs))
	var logIndex uint
	for i, tx := range txs {
		tr := res.TxResults[i]
		r := &types.Receipt{
			Type:              tx.Type(),
			Status:            uint64(tr.Status),
			CumulativeGasUsed: tr.CumulativeGasUsed,
			TxHash:            tx.Hash(),
			GasUsed:           tr.GasUsed,
			BlockHash:         blockHash,
			BlockNumber:       blockNum,
			TransactionIndex:  uint(i),
		}
		if tx.To() == nil {
			var from crypto.Address
			copy(from[:], froms[i][:])
			created := crypto.CreateAddress(from, tx.Nonce())
			r.ContractAddress = common.BytesToAddress(created[:])
		}
		var txLogs []*types.Log
		for k := range res.Logs {
			lg := &res.Logs[k]
			if lg.TxIndex != uint32(i) {
				continue
			}
			l := &types.Log{
				Address:        common.BytesToAddress(lg.Address[:]),
				Topics:         make([]common.Hash, len(lg.Topics)),
				Data:           append([]byte(nil), lg.Data...),
				BlockNumber:    blockNum.Uint64(),
				TxHash:         tx.Hash(),
				TxIndex:        uint(i),
				BlockHash:      blockHash,
				BlockTimestamp: header.Time,
				Index:          logIndex,
			}
			for t := range lg.Topics {
				l.Topics[t] = common.BytesToHash(lg.Topics[t][:])
			}
			txLogs = append(txLogs, l)
			logIndex++
		}
		r.Logs = txLogs
		r.Bloom = types.CreateBloom(r)
		receipts[i] = r
	}
	return receipts
}

// writePostState replays cevm's post-state DELTA onto sdb using ABSOLUTE setters.
// It is the ONE writeback, applied first to an isolated Copy for verification
// and, only after the four-way gate passes, to the live statedb — so the copy
// and the live state receive byte-identical mutations. cevm emits ABSOLUTE post
// values (SetNonce/SetBalance/SetCode/SetState), a Deleted flag for
// selfdestructed accounts (Finalise drops the object and its storage under the
// promoted deletion), and omits storage rows for deleted accounts.
func writePostState(sdb *state.StateDB, res cevmbridge.Result) {
	for i := range res.PostAccounts {
		pa := &res.PostAccounts[i]
		addr := common.BytesToAddress(pa.Address[:])
		if pa.Deleted {
			sdb.SelfDestruct(addr)
			continue
		}
		sdb.SetNonce(addr, pa.Nonce, tracing.NonceChangeUnspecified)
		sdb.SetBalance(addr, &uint256.Int{pa.Balance[0], pa.Balance[1], pa.Balance[2], pa.Balance[3]}, tracing.BalanceChangeUnspecified)
		if pa.CodeChanged {
			sdb.SetCode(addr, pa.Code, tracing.CodeChangeUnspecified)
		}
	}
	for i := range res.PostStorage {
		ps := &res.PostStorage[i]
		sdb.SetState(common.BytesToAddress(ps.Address[:]), common.BytesToHash(ps.Key[:]), common.BytesToHash(ps.Value[:]))
	}
}

// effectiveGasPrice mirrors evm/core/state_transition.go: the price used for the
// sender debit and the full coinbase fee credit. For a legacy tx this is the
// gas price; for a dynamic-fee tx, min(gasFeeCap, gasTipCap+baseFee).
func effectiveGasPrice(tx *types.Transaction, baseFee *big.Int) *big.Int {
	if baseFee == nil {
		return tx.GasPrice()
	}
	sum := new(big.Int).Add(tx.GasTipCap(), baseFee)
	if sum.Cmp(tx.GasFeeCap()) > 0 {
		return new(big.Int).Set(tx.GasFeeCap())
	}
	return sum
}

// blockCtx builds the cevm block context from the header. The 256-bit fields are
// big-endian; PrevRandao is the post-merge MixDigest. Revision is derived from
// the chain config (immaterial to the root for the proven transfer subset).
func blockCtx(config *ethparams.ChainConfig, header *types.Header) cevmbridge.BlockCtx {
	var c cevmbridge.BlockCtx
	copy(c.Coinbase[:], header.Coinbase[:])
	if header.Number != nil {
		c.BlockNumber = header.Number.Int64()
	}
	c.BlockTime = int64(header.Time)
	c.BlockGasLimit = int64(header.GasLimit)
	if config.ChainID != nil {
		fillBE32(&c.ChainID, config.ChainID)
	}
	if header.BaseFee != nil {
		fillBE32(&c.BaseFee, header.BaseFee)
	}
	copy(c.PrevRandao[:], header.MixDigest[:])
	c.Revision = revisionOf(config, header)
	return c
}

func revisionOf(config *ethparams.ChainConfig, header *types.Header) uint8 {
	num, t := header.Number, header.Time
	switch {
	case config.IsCancun(num, t):
		return cevmbridge.RevCancun
	case config.IsShanghai(num, t):
		return cevmbridge.RevShanghai
	case config.IsLondon(num):
		return cevmbridge.RevLondon
	case config.IsBerlin(num):
		return cevmbridge.RevBerlin
	case config.IsIstanbul(num):
		return cevmbridge.RevIstanbul
	default:
		return cevmbridge.RevIstanbul
	}
}

// fillBE32 writes v as a 32-byte big-endian value (right-aligned, zero-padded).
func fillBE32(dst *[32]byte, v *big.Int) {
	*dst = [32]byte{}
	if v == nil || v.Sign() == 0 {
		return
	}
	b := v.Bytes() // big-endian, minimal
	if len(b) >= 32 {
		copy(dst[:], b[len(b)-32:])
		return
	}
	copy(dst[32-len(b):], b)
}

// shadowSnapshot is a DumpCollector that materializes the pre-block accounts +
// storage into cevmbridge structs, bounded by the snapshot caps.
type shadowSnapshot struct {
	maxAccts, maxStorage int
	accts                []cevmbridge.Account
	storage              []cevmbridge.Storage
	missingAddr          bool
	badBalance           bool
	tooLarge             bool
}

func (s *shadowSnapshot) OnRoot(common.Hash) {}

func (s *shadowSnapshot) OnAccount(addr *common.Address, acc state.DumpAccount) {
	if s.tooLarge || s.missingAddr || s.badBalance {
		return
	}
	if addr == nil {
		// No preimage for this trie key: cannot recover the 20-byte address.
		s.missingAddr = true
		return
	}
	if len(s.accts) >= s.maxAccts {
		s.tooLarge = true
		return
	}
	var a cevmbridge.Account
	copy(a.Address[:], addr[:])
	a.Nonce = acc.Nonce
	bal, err := uint256.FromDecimal(acc.Balance)
	if err != nil {
		s.badBalance = true
		return
	}
	a.Balance = [4]uint64{bal[0], bal[1], bal[2], bal[3]}
	if len(acc.Code) > 0 {
		a.Code = append([]byte(nil), acc.Code...)
	}
	s.accts = append(s.accts, a)

	for k, vhex := range acc.Storage {
		if len(s.storage) >= s.maxStorage {
			s.tooLarge = true
			return
		}
		var st cevmbridge.Storage
		copy(st.Address[:], addr[:])
		copy(st.Key[:], k[:])
		vb := common.Hex2Bytes(vhex) // dump value is hex without 0x, big-endian minimal
		if len(vb) >= 32 {
			copy(st.Value[:], vb[len(vb)-32:])
		} else {
			copy(st.Value[32-len(vb):], vb)
		}
		s.storage = append(s.storage, st)
	}
}

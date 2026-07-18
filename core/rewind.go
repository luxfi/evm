// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"fmt"

	"github.com/luxfi/evm/plugin/evm/customrawdb"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	log "github.com/luxfi/log"
)

// FinalizeRewind persists and cleans up an accepted-tip rewind AFTER the chain
// has been (re)constructed at [target].
//
// It is the OFFLINE recovery for a proposervm/EVM height inconsistency. When the
// inner EVM ran ahead of consensus finality — the proposervm never created the
// outer wrappers for the heights above its finality tip — the wrapping proposervm
// refuses to boot (vms/proposervm/vm.go: heightBehind is a fail-closed fatal,
// never a silent finality reset). Rolling the EVM accepted tip back DOWN to the
// consensus finalized height [target] makes the two indices agree, so the
// proposervm reconciles on the next boot as heightMatch — or, for a node whose
// proposervm sits above target, as heightAhead, which self-heals by rolling ITS
// pointer back to the inner tip. No re-genesis: all balances at height <= target
// are preserved.
//
// The head-rewind itself is performed by coreth's own boot path: constructing the
// chain with NewBlockChain(..., lastAcceptedHash = target's hash, ...) runs
// loadLastState, which sets the finalized floor to target FIRST and then reorgs
// the head down to it — deleting the canonical hash->number mappings for every
// discarded run-ahead block — and re-derives target's state (reprocessState). We
// deliberately reuse that exact, tested path rather than a bespoke reorg (a
// direct SetPreference below the current accepted tip is correctly refused:
// "cannot orphan finalized block"). This method finalizes that reconstruction on
// disk:
//
//   - SetLastAcceptedBlockDirect(target): persist head + last-accepted + the
//     ACCEPTOR TIP to target. The reorg leaves the on-disk acceptor tip at the
//     discarded run-ahead tip, from which reprocessState re-seeds its walk-back on
//     the next boot — a deep rewind then fails "required historical state
//     unavailable". This realigns acceptorTip == head == lastAccepted == target.
//   - ForceCommitState(target): persist target's state root. Under the hash
//     scheme Stop does not journal recent roots, so an accepted-but-uncommitted
//     root would be lost on the next restart — the exact failure this recovery
//     must avoid. Production runs with pruning, so target's root is usually not
//     otherwise committed.
//   - INVALIDATE the flat snapshot (delete its markers) so coreth rebuilds it from
//     target's trie on the next boot. Repointing the markers to target would be
//     WRONG: production boots SkipVerify, so matching-but-mislabeled markers make
//     coreth trust the discarded tip's flat data and serve wrong balances at target.
//   - CleanBlockRootsAboveLastAccepted(): drop any orphaned processing roots.
//
// It asserts the reconstruction actually landed at target with state present, so
// a wrong target / failed reprocess is caught here — before the caller commits
// the VM-level last-accepted pointer — rather than on the live node's boot.
//
// The caller MUST also roll the VM-level last-accepted pointer (plugin/evm
// acceptedBlockDB[lastAcceptedKey]) to target's hash, so readLastAccepted returns
// target on the next boot. The two writes together are what the node reads.
func (bc *BlockChain) FinalizeRewind(target uint64) (*types.Block, error) {
	head := bc.CurrentBlock()
	if head.Number.Uint64() != target {
		return nil, fmt.Errorf(
			"chain is at height %d, not the rewind target %d: construct the chain with lastAcceptedHash = target's hash (loadLastState performs the head reorg) before FinalizeRewind",
			head.Number.Uint64(), target)
	}
	tb := bc.GetBlockByNumber(target)
	if tb == nil {
		return nil, fmt.Errorf("canonical block at target %d not found", target)
	}
	if tb.Hash() != head.Hash() {
		return nil, fmt.Errorf("canonical block at %d (%s) does not match head (%s)", target, tb.Hash(), head.Hash())
	}
	if la := bc.LastAcceptedBlock(); la == nil || la.NumberU64() != target || la.Hash() != tb.Hash() {
		return nil, fmt.Errorf("last-accepted is not target %d (%s)", target, tb.Hash())
	}

	log.Info("FinalizeRewind: begin", "target", target, "hash", tb.Hash(), "root", tb.Root())

	// Persist head + last-accepted + ACCEPTOR TIP to target on disk. The
	// reconstruct reorg moved the in-memory markers and the on-disk head hash, but
	// NOT the on-disk acceptor tip — which still points at the discarded run-ahead
	// tip. reprocessState re-seeds its walk-back from the acceptor tip on the next
	// boot, so a stale one makes a deep rewind fail "required historical state
	// unavailable (reexec=...)". SetLastAcceptedBlockDirect writes the acceptor tip
	// (via batchBlockAcceptedIndices) together with the head pointers, so
	// acceptorTip == head == lastAccepted == target.
	if err := bc.SetLastAcceptedBlockDirect(tb); err != nil {
		return nil, fmt.Errorf("persist head/last-accepted/acceptor-tip at %d (%s): %w", target, tb.Hash(), err)
	}

	// Persist target's state root so it survives the next restart under pruning.
	if err := bc.ForceCommitState(tb); err != nil {
		return nil, fmt.Errorf("commit state at %d (%s): %w", target, tb.Hash(), err)
	}
	if !bc.HasState(tb.Root()) {
		return nil, fmt.Errorf("state root %s for height %d not present after commit", tb.Root(), target)
	}

	// INVALIDATE the flat snapshot — do NOT repoint it to target. After a rewind the
	// on-disk snapshot disk-layer still holds the DISCARDED tip's account/storage
	// data. Production boots with SnapshotVerify=false (SkipVerify), so writing
	// target's markers would make loadSnapshot find MATCHING markers and TRUST the
	// mislabeled layer — serving the discarded tip's balances as target's state,
	// fleet-wide and undetectable by a trie-only check. Deleting the markers makes
	// loadSnapshot fail on the next boot, which triggers a clean Rebuild from
	// target's committed trie (async; state reads fall back to the trie meanwhile).
	batch := bc.db.NewBatch()
	rawdb.DeleteSnapshotRoot(batch)
	customrawdb.DeleteSnapshotBlockHash(batch)
	if err := batch.Write(); err != nil {
		return nil, fmt.Errorf("invalidate snapshot at %d: %w", target, err)
	}

	// Drop now-orphaned processing roots above target.
	if err := bc.CleanBlockRootsAboveLastAccepted(); err != nil {
		return nil, fmt.Errorf("clean block roots above %d: %w", target, err)
	}

	// The construction reorg must have removed canonical above target.
	if next := bc.GetBlockByNumber(target + 1); next != nil {
		return nil, fmt.Errorf("canonical block at %d still present after rewind (%s)", target+1, next.Hash())
	}

	log.Info("FinalizeRewind: done", "height", target, "hash", tb.Hash(), "root", tb.Root())
	return tb, nil
}

// HighestCommittedStateAtOrBelow returns the highest canonical height h with
// h <= max whose state root is committed on disk, scanning down at most [window]
// heights from max. ok is false if no committed state is found in that window.
//
// It is used to choose a rewind target that needs ZERO re-execution (and hence
// carries zero re-execution / chain-identity risk): reconstructing at a height
// whose state is already committed makes loadLastState's reprocessState a no-op,
// so no finalized block is re-executed offline. A window of 0 scans to genesis.
func (bc *BlockChain) HighestCommittedStateAtOrBelow(max uint64, window uint64) (uint64, common.Hash, bool) {
	var floor uint64
	if window != 0 && max > window {
		floor = max - window
	}
	for h := max; ; h-- {
		if header := bc.GetHeaderByNumber(h); header != nil && bc.HasState(header.Root) {
			return h, header.Hash(), true
		}
		if h == 0 || h == floor {
			break
		}
	}
	return 0, common.Hash{}, false
}

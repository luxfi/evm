// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// Disk-backed cevm resident state — the "own disk-backed state" evolution of the
// resident applier (see cevm_shadow.go's seed-cap comment). When the VM registers a
// durable store, the cevm resident StateDB CHECKPOINTS to it after verified applies
// and LAZY-LOADS a checkpoint at startup instead of dumping the full Go state:
// StateLoadFromLazy pages the account trie from disk, so RAM is bounded to the
// touched working set, not the whole state — removing the declined-too-large seed
// dump on the post-restart path.
//
// Triple-gated + safe by construction: it does nothing unless (1) built with -tags
// cevm (cevmbridge.Enabled), (2) the VM has registered a store, and (3) a checkpoint
// exists. And the applier still verifies EVERY cevm root against the header before
// applying, so a stale/wrong checkpoint can only force a resync (a Go-dump seed on
// the next block), never a wrong commit. nil store (the default) = the in-memory,
// seed-from-Go behavior, byte-for-byte unchanged.

import (
	"encoding/binary"
	"sync"

	"github.com/luxfi/database"
	"github.com/luxfi/evm/core/parallel/cevmbridge"
)

// cevmCkptHeightKey records the block height of the persisted checkpoint (8-byte
// big-endian), alongside the flat records + trie nodes StatePersist writes.
const cevmCkptHeightKey = "cevm-ckpt-height"

var (
	cevmStoreMu      sync.Mutex
	cevmStateStore   database.Database // registered by the VM; nil => disk-backed residency off
	cevmCkptInterval uint64 = 4096     // checkpoint every N blocks (0 disables checkpointing)
)

// SetCevmResidentStore registers the durable KV the cevm resident StateDB persists
// its checkpoints to and lazy-loads at startup (bounded RAM). Passing nil (the
// default) disables disk-backed residency — the resident stays in-memory and is
// seeded from the Go state dump exactly as before. The VM calls this once at
// C-Chain init when the operator opts into cevm-owned state.
func SetCevmResidentStore(db database.Database) {
	cevmStoreMu.Lock()
	cevmStateStore = db
	cevmStoreMu.Unlock()
}

// SetCevmCheckpointInterval overrides the block cadence at which the resident state
// is checkpointed to the store (0 disables checkpoint writes). For tests + tuning.
func SetCevmCheckpointInterval(n uint64) {
	cevmStoreMu.Lock()
	cevmCkptInterval = n
	cevmStoreMu.Unlock()
}

func cevmStore() (database.Database, uint64) {
	cevmStoreMu.Lock()
	defer cevmStoreMu.Unlock()
	return cevmStateStore, cevmCkptInterval
}

// tryLazyLoadCheckpoint establishes rs from a persisted checkpoint WITHOUT a Go-state
// dump: StateLoadFromLazy pages the account trie from disk, bounding RAM to the
// touched set. On success it sets rs to the (verified-good) checkpoint height and
// returns true; on any miss it returns false and leaves rs untouched, so the caller
// falls back to StateCreate + the Go-dump seed. Called under residentMu by
// getResident on a chain's first use (e.g. just after a node restart).
func tryLazyLoadCheckpoint(rs *residentState) bool {
	db, _ := cevmStore()
	if db == nil || !cevmbridge.Enabled {
		return false
	}
	hb, err := db.Get([]byte(cevmCkptHeightKey))
	if err != nil || len(hb) != 8 {
		return false // no checkpoint recorded
	}
	h, err := cevmbridge.StateLoadFromLazy(db)
	if err != nil || h == 0 {
		return false // no / corrupt persisted state
	}
	rs.handle = h
	rs.haveHandle = true
	rs.everSeeded = true
	rs.synced = true
	rs.height = binary.BigEndian.Uint64(hb)
	return true
}

// persistCheckpoint writes the resident StateDB to the store + records its height at
// the configured cadence, so a restart can lazy-load it. Best-effort: a store error
// is non-fatal — the next restart simply falls back to a Go-dump seed. Called by the
// applier only after a block's cevm root has been verified against the header.
func persistCheckpoint(rs *residentState) {
	db, interval := cevmStore()
	if db == nil || !cevmbridge.Enabled || !rs.synced || interval == 0 {
		return
	}
	if rs.height%interval != 0 {
		return // only at the cadence
	}
	if _, err := cevmbridge.StatePersist(rs.handle, db); err != nil {
		return
	}
	var hb [8]byte
	binary.BigEndian.PutUint64(hb[:], rs.height)
	_ = db.Put([]byte(cevmCkptHeightKey), hb[:])
}

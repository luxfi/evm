// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"sync"

	"github.com/holiman/uint256"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
)

// mvMemory is the multi-version data structure at the heart of Block-STM
// (Gelashvili et al., Aptos Labs). For every state location it remembers, per
// writing transaction index, the value that transaction produced. A read by
// transaction j returns the value written by the HIGHEST transaction index
// i < j — exactly the value sequential execution would have given j. Validation
// re-checks that a recorded read still returns the same value; on mismatch the
// reader is re-executed. This is what makes optimistic parallel execution
// converge to the unique sequential result.
//
// Locations are addressed at the granularity the EVM StateDB reads them:
//   - accountKey: the account metadata tuple (existence, nonce, balance, code)
//     loaded by a single StateDB.reader.Account call.
//   - storageKey: one storage slot loaded by StateDB.reader.Storage.
//
// This granularity is identical to the StateDB's own read granularity, so the
// version sets carry no false sharing beyond what sequential execution has — and
// critically it CAPTURES THE SENDER: the EVM reads the sender account (nonce +
// balance) on every transaction, so same-sender transactions are recorded as
// genuine reader/writer pairs rather than (incorrectly) treated as disjoint.

type accessKind uint8

const (
	accountKey accessKind = iota
	storageKey
)

// mvKey identifies one versioned location.
type mvKey struct {
	addr common.Address
	slot common.Hash // zero for accountKey
	kind accessKind
}

// accountVal is the multi-version snapshot of an account's EVM-visible metadata.
// It is a value type: copies are independent and safe to compare.
type accountVal struct {
	exists   bool        // false => account absent (never existed or self-destructed)
	nonce    uint64      //
	balance  uint256.Int //
	codeHash common.Hash //
	code     []byte      // contract code (nil for EOAs / empty code)
}

// equal reports whether two account snapshots are EVM-indistinguishable. Code is
// compared by hash (codeHash uniquely determines code), so the byte slice need
// not be examined.
func (a accountVal) equal(b accountVal) bool {
	if a.exists != b.exists {
		return false
	}
	if !a.exists {
		return true // both absent
	}
	return a.nonce == b.nonce && a.balance == b.balance && a.codeHash == b.codeHash
}

// cell is one version of one location: the value transaction tx produced in the
// given incarnation. Exactly one of acct/val is meaningful, per the key kind.
type cell struct {
	tx          int
	incarnation int
	acct        accountVal  // accountKey
	val         common.Hash // storageKey
}

// mvMemory is concurrency-safe: parallel workers Record their outputs and Read
// each other's versions during a round. A single mutex is sufficient and keeps
// the structure obviously correct; finer-grained per-key locking is a perf
// upgrade, not a correctness requirement (validation re-checks every read).
type mvMemory struct {
	mu sync.RWMutex
	// data[key][tx] = cell. Per-key version maps are small (one entry per writer
	// of that location).
	data map[mvKey]map[int]cell
	// written[tx] lists the keys tx most recently published, so a re-execution
	// can atomically retract the prior incarnation's writes before publishing
	// the new ones (write sets may shrink across incarnations).
	written map[int][]mvKey
}

func newMVMemory() *mvMemory {
	return &mvMemory{
		data:    make(map[mvKey]map[int]cell),
		written: make(map[int][]mvKey),
	}
}

// record publishes transaction tx's write set for the given incarnation,
// retracting any writes from a previous incarnation first.
func (m *mvMemory) record(tx, incarnation int, ws *writeSet) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Retract the previous incarnation's writes.
	for _, k := range m.written[tx] {
		if versions := m.data[k]; versions != nil {
			delete(versions, tx)
			if len(versions) == 0 {
				delete(m.data, k)
			}
		}
	}

	keys := make([]mvKey, 0, len(ws.accounts)+len(ws.storage))
	put := func(k mvKey, c cell) {
		versions := m.data[k]
		if versions == nil {
			versions = make(map[int]cell)
			m.data[k] = versions
		}
		versions[tx] = c
		keys = append(keys, k)
	}
	for _, aw := range ws.accounts {
		put(mvKey{addr: aw.addr, kind: accountKey},
			cell{tx: tx, incarnation: incarnation, acct: aw.val})
	}
	for _, sw := range ws.storage {
		put(mvKey{addr: sw.addr, slot: sw.slot, kind: storageKey},
			cell{tx: tx, incarnation: incarnation, val: sw.val})
	}
	m.written[tx] = keys
}

// readCell returns the cell written by the highest transaction index strictly
// below beforeTx, if any. This is the value visible to transaction beforeTx.
func (m *mvMemory) readCell(k mvKey, beforeTx int) (cell, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	versions := m.data[k]
	if versions == nil {
		return cell{}, false
	}
	best := -1
	for tx := range versions {
		if tx < beforeTx && tx > best {
			best = tx
		}
	}
	if best < 0 {
		return cell{}, false
	}
	return versions[best], true
}

// readAccount resolves the account a tx at index `at` should observe from the
// multi-version layer alone (base fallback is the reader's job). ok=false means
// no lower transaction wrote the account, so the base value applies.
func (m *mvMemory) readAccount(addr common.Address, at int) (accountVal, bool) {
	c, ok := m.readCell(mvKey{addr: addr, kind: accountKey}, at)
	if !ok {
		return accountVal{}, false
	}
	return c.acct, true
}

// readStorage resolves the storage slot a tx at index `at` should observe from
// the multi-version layer alone. The second return reports whether the slot's
// owning account was self-destructed by a lower transaction (storage wiped);
// the third reports whether any lower transaction wrote this slot.
func (m *mvMemory) readStorage(addr common.Address, slot common.Hash, at int) (val common.Hash, wiped bool, ok bool) {
	// A lower self-destruct of the account clears all of its storage, and that
	// must shadow any base value (and any slot write older than the destruct).
	acctCell, hasAcct := m.readCell(mvKey{addr: addr, kind: accountKey}, at)
	slotCell, hasSlot := m.readCell(mvKey{addr: addr, slot: slot, kind: storageKey}, at)
	if hasAcct && !acctCell.acct.exists {
		if !hasSlot || slotCell.tx < acctCell.tx {
			return common.Hash{}, true, false
		}
	}
	if hasSlot {
		return slotCell.val, false, true
	}
	return common.Hash{}, false, false
}

// toStateAccount converts an account snapshot to the StateDB reader's return
// type. The storage Root is reported empty: storage reads route independently
// through Reader.Storage (verified against state_object.go), so the Root field
// is never consulted for value reads on the speculative StateDB.
func (a accountVal) toStateAccount() *types.StateAccount {
	if !a.exists {
		return nil
	}
	bal := a.balance
	codeHash := a.codeHash
	if codeHash == (common.Hash{}) {
		codeHash = types.EmptyCodeHash
	}
	return &types.StateAccount{
		Nonce:    a.nonce,
		Balance:  &bal,
		Root:     types.EmptyRootHash,
		CodeHash: codeHash.Bytes(),
	}
}

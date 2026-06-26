// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

import (
	"math/big"
	"sync"

	"github.com/holiman/uint256"
	"github.com/luxfi/geth/common"
	ethstate "github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
)

// mvReader is a geth state.Reader that serves every leaf read of one speculative
// transaction from the multi-version layer (mvMemory) first and the immutable
// pre-state (base) second, recording each observation so the read can later be
// validated. Because geth's stateObject routes ALL account, storage and code
// reads through this Reader (verified against state_object.go: reader.Account at
// load, reader.Storage in GetCommittedState, reader.Code/CodeSize for code), the
// speculative StateDB built on it inherits every correct EVM semantic — journal,
// revert, refund, transient storage, access lists, EIP-158 — while reading a
// consistent multi-version snapshot.
type mvReader struct {
	tx   int
	base ethstate.Reader
	mv   *mvMemory
	rs   *readSet
}

// readObservation is one recorded leaf read and the value the reader returned,
// sufficient to re-validate it against the current multi-version layer without
// touching the (immutable) base state again.
type readObservation struct {
	key      mvKey
	resolved accountVal  // accountKey: the account returned
	val      common.Hash // storageKey: the slot value returned
	fromMV   bool        // observation was determined by the multi-version layer
}

// readSet accumulates the observations of one speculative execution. Reads of a
// given key after the first are served identically (StateDB caches them), so
// only the first observation per key is recorded.
type readSet struct {
	mu   sync.Mutex
	obs  []readObservation
	seen map[mvKey]struct{}
}

func newReadSet() *readSet {
	return &readSet{seen: make(map[mvKey]struct{})}
}

func (rs *readSet) add(o readObservation) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if _, ok := rs.seen[o.key]; ok {
		return
	}
	rs.seen[o.key] = struct{}{}
	rs.obs = append(rs.obs, o)
}

// lookupAccount resolves the account visible to this transaction without
// recording it. fromMV reports whether the multi-version layer supplied it.
func (r *mvReader) lookupAccount(addr common.Address) (av accountVal, fromMV bool, err error) {
	if mvVal, ok := r.mv.readAccount(addr, r.tx); ok {
		return mvVal, true, nil
	}
	base, err := r.base.Account(addr)
	if err != nil {
		return accountVal{}, false, err
	}
	if base == nil {
		return accountVal{exists: false}, false, nil
	}
	bal := uint256.Int{}
	if base.Balance != nil {
		bal = *base.Balance
	}
	return accountVal{
		exists:   true,
		nonce:    base.Nonce,
		balance:  bal,
		codeHash: common.BytesToHash(base.CodeHash),
	}, false, nil
}

// Account implements state.StateReader.
func (r *mvReader) Account(addr common.Address) (*types.StateAccount, error) {
	av, fromMV, err := r.lookupAccount(addr)
	if err != nil {
		return nil, err
	}
	r.rs.add(readObservation{key: mvKey{addr: addr, kind: accountKey}, resolved: av, fromMV: fromMV})
	if !fromMV && av.exists {
		// Return the base account verbatim to preserve its real storage Root for
		// any path that consults it; values still read through Storage().
		return r.base.Account(addr)
	}
	return av.toStateAccount(), nil
}

// Storage implements state.StateReader.
func (r *mvReader) Storage(addr common.Address, slot common.Hash) (common.Hash, error) {
	val, wiped, ok := r.mv.readStorage(addr, slot, r.tx)
	if wiped {
		r.rs.add(readObservation{key: mvKey{addr: addr, slot: slot, kind: storageKey}, val: common.Hash{}, fromMV: true})
		return common.Hash{}, nil
	}
	if ok {
		r.rs.add(readObservation{key: mvKey{addr: addr, slot: slot, kind: storageKey}, val: val, fromMV: true})
		return val, nil
	}
	base, err := r.base.Storage(addr, slot)
	if err != nil {
		return common.Hash{}, err
	}
	r.rs.add(readObservation{key: mvKey{addr: addr, slot: slot, kind: storageKey}, val: base, fromMV: false})
	return base, nil
}

// Code implements state.ContractCodeReader. Code identity is pinned by codeHash,
// which is part of the account snapshot already recorded by Account(); no extra
// observation is needed.
func (r *mvReader) Code(addr common.Address, codeHash common.Hash) ([]byte, error) {
	av, fromMV, err := r.lookupAccount(addr)
	if err != nil {
		return nil, err
	}
	if fromMV {
		if !av.exists {
			return nil, nil
		}
		return av.code, nil
	}
	return r.base.Code(addr, codeHash)
}

// CodeSize implements state.ContractCodeReader.
func (r *mvReader) CodeSize(addr common.Address, codeHash common.Hash) (int, error) {
	av, fromMV, err := r.lookupAccount(addr)
	if err != nil {
		return 0, err
	}
	if fromMV {
		if !av.exists {
			return 0, nil
		}
		return len(av.code), nil
	}
	return r.base.CodeSize(addr, codeHash)
}

// Has implements state.ContractCodeReader.
func (r *mvReader) Has(addr common.Address, codeHash common.Hash) bool {
	av, fromMV, err := r.lookupAccount(addr)
	if err != nil {
		return false
	}
	if fromMV {
		return av.exists && len(av.code) > 0
	}
	return r.base.Has(addr, codeHash)
}

// writeCapture records WHICH locations a speculative execution touched. The
// final, post-revert value of each touched location is read back from the
// authoritative inner StateDB at the end of execution (buildWriteSet), so a
// location that was written and then rolled back contributes its restored value
// — never the transient one. The hooks therefore only need the location, not
// the value.
type writeCapture struct {
	addrs map[common.Address]struct{}
	slots map[slotID]struct{}
}

type slotID struct {
	addr common.Address
	slot common.Hash
}

func newWriteCapture() *writeCapture {
	return &writeCapture{
		addrs: make(map[common.Address]struct{}),
		slots: make(map[slotID]struct{}),
	}
}

func (w *writeCapture) hooks() *tracing.Hooks {
	return &tracing.Hooks{
		OnBalanceChange: func(addr common.Address, _, _ *big.Int, _ tracing.BalanceChangeReason) {
			w.addrs[addr] = struct{}{}
		},
		OnNonceChange: func(addr common.Address, _, _ uint64) {
			w.addrs[addr] = struct{}{}
		},
		OnCodeChange: func(addr common.Address, _ common.Hash, _ []byte, _ common.Hash, _ []byte) {
			w.addrs[addr] = struct{}{}
		},
		OnStorageChange: func(addr common.Address, slot common.Hash, _, _ common.Hash) {
			w.addrs[addr] = struct{}{}
			w.slots[slotID{addr: addr, slot: slot}] = struct{}{}
		},
	}
}

// accountWrite / storageWrite / writeSet are the published outputs of one
// transaction: the net effect that materialization replays onto canonical state
// in transaction order.
type accountWrite struct {
	addr common.Address
	val  accountVal
}

type storageWrite struct {
	addr common.Address
	slot common.Hash
	val  common.Hash
}

type writeSet struct {
	accounts []accountWrite
	storage  []storageWrite
}

// buildWriteSet reads the FINAL (post-execution, post-revert) value of every
// touched location from the authoritative inner StateDB. This is what makes the
// capture revert-safe: hooks may have fired for values later rolled back, but
// the inner StateDB holds only the committed truth.
func buildWriteSet(inner *ethstate.StateDB, wc *writeCapture) *writeSet {
	ws := &writeSet{
		accounts: make([]accountWrite, 0, len(wc.addrs)),
		storage:  make([]storageWrite, 0, len(wc.slots)),
	}
	destructed := make(map[common.Address]bool, len(wc.addrs))
	for addr := range wc.addrs {
		if inner.HasSelfDestructed(addr) || !inner.Exist(addr) {
			destructed[addr] = true
			ws.accounts = append(ws.accounts, accountWrite{addr: addr, val: accountVal{exists: false}})
			continue
		}
		av := accountVal{
			exists:   true,
			nonce:    inner.GetNonce(addr),
			balance:  *inner.GetBalance(addr),
			codeHash: inner.GetCodeHash(addr),
		}
		if av.codeHash != types.EmptyCodeHash && av.codeHash != (common.Hash{}) {
			av.code = inner.GetCode(addr)
		}
		ws.accounts = append(ws.accounts, accountWrite{addr: addr, val: av})
	}
	for sl := range wc.slots {
		if destructed[sl.addr] {
			continue // storage of a destructed account is wiped, not written
		}
		ws.storage = append(ws.storage, storageWrite{addr: sl.addr, slot: sl.slot, val: inner.GetState(sl.addr, sl.slot)})
	}
	return ws
}

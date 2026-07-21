// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

// Durability wrappers: persist the RESIDENT cevm StateDB behind a handle to a
// geth-style key/value store, and load it back into a fresh handle with a
// byte-identical MPT root — so a cevm StateDB survives a process restart.
//
// The backing store is luxd's OWN zapdb (or any database.Database). cevm (C++)
// calls back into THIS Go runtime through the cevm_kv_store function pointers
// built by cevmbridge_make_store below — no second Go runtime is embedded (the
// two-runtime constraint). The //export trampolines those pointers target live
// in store_callbacks_cevm.go (kept separate because //export forbids C
// definitions in its file's preamble).

/*
#cgo CFLAGS: -I/Users/z/work/luxcpp/cevm/lib/evm
#include <stdint.h>
#include "state/go_process_block.h"

// Forward declarations of the Go trampolines (defined via //export in
// store_callbacks_cevm.go). Signatures match cgo's generated C prototypes
// (non-const pointers); the cast to the const-qualified cevm_kv_store field
// types below is exact and warning-free.
int cevmbridge_store_get(void* ctx, uint8_t* key, int klen, uint8_t* out, int outcap);
int cevmbridge_store_put(void* ctx, uint8_t* key, int klen, uint8_t* val, int vlen);

// Build a cevm_kv_store whose callbacks are the Go trampolines and whose ctx is
// an opaque cgo.Handle token. The uintptr -> void* cast happens HERE in C, so no
// uintptr<->unsafe.Pointer conversion (which go vet flags) is needed in Go.
static cevm_kv_store cevmbridge_make_store(uintptr_t ctx) {
    cevm_kv_store s;
    s.get = (int (*)(void*, const uint8_t*, int, uint8_t*, int))cevmbridge_store_get;
    s.put = (int (*)(void*, const uint8_t*, int, const uint8_t*, int))cevmbridge_store_put;
    s.ctx = (void*)ctx;
    return s;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime/cgo"

	"github.com/luxfi/database"
)

// ErrNilStore is returned when a persist/load call is handed a nil store.
var ErrNilStore = errors.New("cevmbridge: nil store")

// StatePersist serializes the resident cevm StateDB behind handle h into db (a
// geth-style KV — in production luxd's own zapdb) and returns the number of
// accounts persisted. The C++ side calls db.Get/Put back through this SAME Go
// runtime via registered trampolines, so no second runtime is embedded.
//
// MVP scope: persists account state (accounts + storage), NOT trie nodes — the
// resident MPT is rebuilt from that state on StateLoadFrom. That is durability
// (survives restart, byte-identical root), not yet bounded-RAM load-on-demand.
func StatePersist(h uint64, db database.Database) (int, error) {
	if db == nil {
		return 0, ErrNilStore
	}
	st := &kvStore{db: db}
	handle := cgo.NewHandle(st)
	defer handle.Delete()

	cs := C.cevmbridge_make_store(C.uintptr_t(handle))
	n := C.cevm_state_persist(C.uint64_t(h), &cs)
	if n < 0 {
		return 0, fmt.Errorf("cevmbridge: cevm_state_persist failed for handle %d (rc=%d)", h, int(n))
	}
	return int(n), nil
}

// StateLoadFrom creates a NEW resident cevm StateDB, reads every persisted
// account + slot from db, and commits to rebuild the resident tries + root,
// returning the new non-zero handle. The C++ side self-verifies the rebuilt root
// against the persisted meta root and returns 0 on ANY mismatch — a wrong
// reloaded root is a durability bug, never silently accepted.
func StateLoadFrom(db database.Database) (uint64, error) {
	if db == nil {
		return 0, ErrNilStore
	}
	st := &kvStore{db: db}
	handle := cgo.NewHandle(st)
	defer handle.Delete()

	cs := C.cevmbridge_make_store(C.uintptr_t(handle))
	h := C.cevm_state_load(&cs)
	if h == 0 {
		return 0, errors.New("cevmbridge: cevm_state_load failed (no/corrupt persisted state, or reloaded root != persisted root)")
	}
	return uint64(h), nil
}

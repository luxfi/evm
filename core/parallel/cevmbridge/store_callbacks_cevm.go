// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

// The Go trampolines the C++ resident-state persistence calls through the
// cevm_kv_store function pointers (see state_store.h). They resolve the opaque
// ctx token back to a *kvStore via cgo.Handle and forward get/put to the backing
// geth-style database.Database (in production, luxd's OWN zapdb).
//
// CRITICAL (two-Go-runtime constraint): these run on luxd's SINGLE Go runtime —
// the C++/cgo cevm calls BACK into this same runtime through the registered
// pointers. cevm never embeds a second Go runtime (that would mean two GCs).
//
// //export requires this file's cgo preamble to hold DECLARATIONS ONLY, so the
// cevm_kv_store constructor (a C function definition) lives in store_cevm.go.

/*
#include <stdint.h>
*/
import "C"

import (
	"runtime/cgo"
	"unsafe"
)

// kvStore adapts a geth-style KV (database.Database: Get/Put) to the cevm
// node-store interface. The in-luxd zapdb and any database.Database back it.
type kvStore struct{ db kvBackend }

// kvBackend is the minimal surface cevm persistence needs from a store. It is
// satisfied by *zapdb.Database and every other database.Database.
type kvBackend interface {
	Get(key []byte) ([]byte, error)
	Put(key, value []byte) error
}

//export cevmbridge_store_get
func cevmbridge_store_get(ctx unsafe.Pointer, key *C.uint8_t, klen C.int, out *C.uint8_t, outcap C.int) C.int {
	st, ok := cgo.Handle(uintptr(ctx)).Value().(*kvStore)
	if !ok {
		return -2
	}
	k := C.GoBytes(unsafe.Pointer(key), klen)
	v, err := st.db.Get(k)
	if err != nil {
		return -1 // absent (database.ErrNotFound) or store error => treat as absent
	}
	n := len(v)
	if n > 0 && outcap > 0 {
		c := n
		if c > int(outcap) {
			c = int(outcap)
		}
		dst := unsafe.Slice((*byte)(unsafe.Pointer(out)), c)
		copy(dst, v[:c])
	}
	return C.int(n)
}

//export cevmbridge_store_put
func cevmbridge_store_put(ctx unsafe.Pointer, key *C.uint8_t, klen C.int, val *C.uint8_t, vlen C.int) C.int {
	st, ok := cgo.Handle(uintptr(ctx)).Value().(*kvStore)
	if !ok {
		return -2
	}
	k := C.GoBytes(unsafe.Pointer(key), klen)
	var v []byte
	if vlen > 0 {
		v = C.GoBytes(unsafe.Pointer(val), vlen)
	} else {
		v = []byte{}
	}
	if err := st.db.Put(k, v); err != nil {
		return -1
	}
	return 0
}

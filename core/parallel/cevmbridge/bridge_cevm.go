// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package cevmbridge

// Real cgo bridge into the Lux C++ EVM (cevm) block-batch consensus entry
// point. Links the FRESH build-mpt CPU libs that contain the real keccak
// Merkle-Patricia-Trie commit — NOT the stale /opt/homebrew pre-MPT libs and
// NOT the gpu execute_block placeholder-root path.
//
// The cgo link line below is byte-for-byte the one proven to link and return
// the byte-exact geth empty-MPT root through this exact entry point (see the
// map:linking verification). cgo takes LITERAL absolute paths — no env/${SRCDIR}
// expansion — so the paths are spelled out. The Conan hash dirs
// (lux-c4c40b1a25281d = CPU precompile deps, bls3f47da987ce4d = blst) are the
// authoritative set read from the gate's own link.txt. -Wl,-rpath is MANDATORY:
// libevm.0.19.0.dylib has install name @rpath/libevm.0.19.dylib.
//
// The preamble only includes the C ABI header (pure C, extern "C"); the C++
// symbols it pulls resolve from the linked archives, so -lc++ is required and
// not duplicated (this is a C cgo package — cgo does not auto-add libc++ here).
//
// Two entry families share one marshalling pipeline (the marshal* / unmarshal*
// helpers below):
//   - STATELESS: ProcessBlock -> cevm_process_block (fresh StateDB per call).
//   - RESIDENT:  StateCreate/StateSeed/StateApplyBlock/StateRoot/StateFree ->
//     the cevm_state_* handle ABI (a PERSISTENT StateDB held C-side, so state
//     lives across blocks and no per-block dump crosses the FFI boundary).

/*
#cgo CFLAGS: -I/Users/z/work/luxcpp/cevm/lib/evm
#cgo LDFLAGS: /Users/z/work/luxcpp/cevm/build-mpt/build/Release/lib/evm/libevm-state.a /Users/z/work/luxcpp/cevm/build-mpt/build/Release/lib/libevm.0.19.0.dylib /Users/z/work/luxcpp/cevm/build-mpt/build/Release/lib/cevm_precompiles/libcevm_precompiles.a /Users/z/work/luxcpp/cevm/build-mpt/build/Release/lib/cevm_precompiles/libcevm_bls_kzg_canonical_cpu.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libbls_evm.a /Users/z/.conan2/p/b/bls3f47da987ce4d/p/lib/libblst.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libbn254.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libbn254_cpu.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libmodexp.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libmodexp_cpu.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libsha256.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libsha256_cpu.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libripemd160.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libripemd160_cpu.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libblake2b.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libblake2b_cpu.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libsecp256r1.a /Users/z/.conan2/p/b/lux-c4c40b1a25281d/p/lib/libsecp256r1_cpu.a -Wl,-rpath,/Users/z/work/luxcpp/cevm/build-mpt/build/Release/lib -lc++
#include <stdlib.h>
#include "state/go_process_block.h"
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// Enabled reports that the real C++ EVM bridge is linked.
const Enabled = true

// ABIVersion returns the ABI version compiled into the linked cevm library.
func ABIVersion() uint32 { return uint32(C.cevm_pb_abi_version()) }

// cAlloc accumulates C heap allocations (code/data/access-list payloads that
// cgo forbids inside Go-memory structs) so they can be freed after the call.
type cAlloc struct{ ptrs []unsafe.Pointer }

func (c *cAlloc) free() {
	for _, p := range c.ptrs {
		C.free(p)
	}
}

// bytes copies b into C memory tracked for free-on-return.
func (c *cAlloc) bytes(b []byte) unsafe.Pointer {
	p := C.CBytes(b)
	c.ptrs = append(c.ptrs, p)
	return p
}

// --- Go -> C marshalling (shared by the stateless + resident paths) ------

func marshalAccts(accts []Account, ca *cAlloc) []C.cevm_pb_acct {
	c := make([]C.cevm_pb_acct, len(accts))
	for i := range accts {
		a := &accts[i]
		for j := 0; j < 20; j++ {
			c[i].address[j] = C.uint8_t(a.Address[j])
		}
		c[i].nonce = C.uint64_t(a.Nonce)
		for j := 0; j < 4; j++ {
			c[i].balance[j] = C.uint64_t(a.Balance[j])
		}
		if len(a.Code) > 0 {
			c[i].code = (*C.uint8_t)(ca.bytes(a.Code))
			c[i].code_len = C.uint32_t(len(a.Code))
		}
	}
	return c
}

func marshalStorage(storage []Storage) []C.cevm_pb_storage {
	c := make([]C.cevm_pb_storage, len(storage))
	for i := range storage {
		s := &storage[i]
		for j := 0; j < 20; j++ {
			c[i].address[j] = C.uint8_t(s.Address[j])
		}
		for j := 0; j < 32; j++ {
			c[i].key[j] = C.uint8_t(s.Key[j])
			c[i].value[j] = C.uint8_t(s.Value[j])
		}
	}
	return c
}

func marshalTxs(txs []Tx, ca *cAlloc) []C.cevm_pb_tx {
	c := make([]C.cevm_pb_tx, len(txs))
	for i := range txs {
		t := &txs[i]
		for j := 0; j < 20; j++ {
			c[i].sender[j] = C.uint8_t(t.Sender[j])
			c[i].recipient[j] = C.uint8_t(t.Recipient[j])
		}
		for j := 0; j < 32; j++ {
			c[i].value[j] = C.uint8_t(t.Value[j])
			c[i].gas_price[j] = C.uint8_t(t.GasPrice[j])
		}
		c[i].gas_limit = C.uint64_t(t.GasLimit)
		c[i].nonce = C.uint64_t(t.Nonce)
		if len(t.Data) > 0 {
			c[i].data = (*C.uint8_t)(ca.bytes(t.Data))
			c[i].data_len = C.uint32_t(len(t.Data))
		}
		if t.IsCreate {
			c[i].is_create = 1
		}

		// --- EIP-2930 access list (may be nil) ---------------------------
		// The tuple array and each tuple's flattened key buffer are allocated
		// in C memory: c is Go memory passed to C, and cgo forbids it from
		// containing Go pointers — so access_list / storage_keys must point at
		// C allocations (freed on return alongside the data payloads).
		if nal := len(t.AccessList); nal > 0 {
			tupBytes := C.size_t(nal) * C.size_t(unsafe.Sizeof(C.cevm_pb_access_tuple{}))
			tupMem := C.malloc(tupBytes)
			ca.ptrs = append(ca.ptrs, tupMem)
			tuples := unsafe.Slice((*C.cevm_pb_access_tuple)(tupMem), nal)
			for j := range t.AccessList {
				at := &t.AccessList[j]
				for b := 0; b < 20; b++ {
					tuples[j].address[b] = C.uint8_t(at.Address[b])
				}
				tuples[j].storage_keys = nil
				tuples[j].n_storage_keys = 0
				if nk := len(at.StorageKeys); nk > 0 {
					flat := make([]byte, nk*32)
					for k := range at.StorageKeys {
						copy(flat[k*32:(k+1)*32], at.StorageKeys[k][:])
					}
					tuples[j].storage_keys = (*C.uint8_t)(ca.bytes(flat))
					tuples[j].n_storage_keys = C.uint32_t(nk)
				}
			}
			c[i].access_list = (*C.cevm_pb_access_tuple)(tupMem)
			c[i].n_access_list = C.uint32_t(nal)
		}
	}
	return c
}

func marshalCtx(ctx BlockCtx) C.cevm_pb_ctx {
	var c C.cevm_pb_ctx
	for j := 0; j < 20; j++ {
		c.coinbase[j] = C.uint8_t(ctx.Coinbase[j])
	}
	c.block_number = C.int64_t(ctx.BlockNumber)
	c.block_timestamp = C.int64_t(ctx.BlockTime)
	c.block_gas_limit = C.int64_t(ctx.BlockGasLimit)
	for j := 0; j < 32; j++ {
		c.chain_id[j] = C.uint8_t(ctx.ChainID[j])
		c.block_base_fee[j] = C.uint8_t(ctx.BaseFee[j])
		c.prev_randao[j] = C.uint8_t(ctx.PrevRandao[j])
		c.blob_base_fee[j] = C.uint8_t(ctx.BlobBaseFee[j])
	}
	c.revision = C.uint8_t(ctx.Revision)
	return c
}

// --- C -> Go unmarshalling (shared) --------------------------------------
// Reads the result's heap arrays; the CALLER owns freeing r via C.cevm_pb_free.

func unmarshalResult(r C.cevm_pb_result) (Result, error) {
	var res Result
	res.ABIVersion = uint32(r.abi_version)
	res.OK = r.ok != 0
	res.TotalGas = uint64(r.total_gas)
	for j := 0; j < 32; j++ {
		res.StateRoot[j] = byte(r.state_root[j])
	}
	if r.ok == 0 {
		return res, fmt.Errorf("cevmbridge: cevm returned ok=0")
	}
	if n := int(r.n_tx_results); n > 0 && r.tx_results != nil {
		cres := unsafe.Slice((*C.cevm_pb_txresult)(unsafe.Pointer(r.tx_results)), n)
		res.TxResults = make([]TxResult, n)
		for i := 0; i < n; i++ {
			res.TxResults[i] = TxResult{
				EVMCStatus:        int32(cres[i].evmc_status),
				Status:            uint8(cres[i].status),
				Rejected:          cres[i].rejected != 0,
				GasUsed:           uint64(cres[i].gas_used),
				CumulativeGasUsed: uint64(cres[i].cumulative_gas_used),
				NLogs:             uint32(cres[i].n_logs),
			}
		}
	}

	// --- logs (ABI v2) ---------------------------------------------------
	// Marshal the emitted logs so the applier can rebuild receipt bloom +
	// receipt-trie hash (statedb.GetLogs is empty when cevm — not the Go EVM —
	// executed). Topics are flat n_topics*32 big-endian.
	if n := int(r.n_logs); n > 0 && r.logs != nil {
		clogs := unsafe.Slice((*C.cevm_pb_log)(unsafe.Pointer(r.logs)), n)
		res.Logs = make([]Log, n)
		for i := 0; i < n; i++ {
			lg := Log{TxIndex: uint32(clogs[i].tx_index)}
			for j := 0; j < 20; j++ {
				lg.Address[j] = byte(clogs[i].address[j])
			}
			if nt := int(clogs[i].n_topics); nt > 0 && clogs[i].topics != nil {
				tb := unsafe.Slice((*C.uint8_t)(unsafe.Pointer(clogs[i].topics)), nt*32)
				lg.Topics = make([][32]byte, nt)
				for t := 0; t < nt; t++ {
					for b := 0; b < 32; b++ {
						lg.Topics[t][b] = byte(tb[t*32+b])
					}
				}
			}
			if dl := int(clogs[i].data_len); dl > 0 && clogs[i].data != nil {
				db := unsafe.Slice((*C.uint8_t)(unsafe.Pointer(clogs[i].data)), dl)
				lg.Data = make([]byte, dl)
				for b := 0; b < dl; b++ {
					lg.Data[b] = byte(db[b])
				}
			}
			res.Logs[i] = lg
		}
	}

	// --- post-state account deltas (ABI v2) ------------------------------
	if n := int(r.n_out_accts); n > 0 && r.out_accts != nil {
		ca := unsafe.Slice((*C.cevm_pb_out_account)(unsafe.Pointer(r.out_accts)), n)
		res.PostAccounts = make([]PostAccount, n)
		for i := 0; i < n; i++ {
			pa := PostAccount{
				Nonce:       uint64(ca[i].nonce),
				Deleted:     ca[i].deleted != 0,
				CodeChanged: ca[i].code_changed != 0,
			}
			for j := 0; j < 20; j++ {
				pa.Address[j] = byte(ca[i].address[j])
			}
			for j := 0; j < 4; j++ {
				pa.Balance[j] = uint64(ca[i].balance[j])
			}
			for j := 0; j < 32; j++ {
				pa.CodeHash[j] = byte(ca[i].code_hash[j])
			}
			if cl := int(ca[i].code_len); cl > 0 && ca[i].code != nil {
				cb := unsafe.Slice((*C.uint8_t)(unsafe.Pointer(ca[i].code)), cl)
				pa.Code = make([]byte, cl)
				for b := 0; b < cl; b++ {
					pa.Code[b] = byte(cb[b])
				}
			}
			res.PostAccounts[i] = pa
		}
	}

	// --- post-state storage deltas (ABI v2) ------------------------------
	if n := int(r.n_out_storage); n > 0 && r.out_storage != nil {
		cs := unsafe.Slice((*C.cevm_pb_out_storage)(unsafe.Pointer(r.out_storage)), n)
		res.PostStorage = make([]PostStorage, n)
		for i := 0; i < n; i++ {
			var ps PostStorage
			for j := 0; j < 20; j++ {
				ps.Address[j] = byte(cs[i].address[j])
			}
			for j := 0; j < 32; j++ {
				ps.Key[j] = byte(cs[i].key[j])
				ps.Value[j] = byte(cs[i].value[j])
			}
			res.PostStorage[i] = ps
		}
	}
	return res, nil
}

// ProcessBlock (STATELESS) seeds a FRESH cevm StateDB from accts+storage, replays
// txs through a real evmc_create_cevm() VM, and returns the real MPT state_root +
// per-tx status/gas + logs + post-state delta. It is a pure function: it does not
// touch any Go state, and rebuilds the whole StateDB from the seed each call.
func ProcessBlock(accts []Account, storage []Storage, txs []Tx, ctx BlockCtx) (Result, error) {
	var ca cAlloc
	defer ca.free()

	cAccts := marshalAccts(accts, &ca)
	cStor := marshalStorage(storage)
	cTxs := marshalTxs(txs, &ca)
	cCtx := marshalCtx(ctx)

	var (
		txPtr   *C.cevm_pb_tx
		acctPtr *C.cevm_pb_acct
		storPtr *C.cevm_pb_storage
	)
	if len(cTxs) > 0 {
		txPtr = &cTxs[0]
	}
	if len(cAccts) > 0 {
		acctPtr = &cAccts[0]
	}
	if len(cStor) > 0 {
		storPtr = &cStor[0]
	}

	r := C.cevm_process_block(
		txPtr, C.uint32_t(len(cTxs)),
		acctPtr, C.uint32_t(len(cAccts)),
		storPtr, C.uint32_t(len(cStor)),
		&cCtx,
	)
	defer C.cevm_pb_free(&r)
	return unmarshalResult(r)
}

// --- Resident (stateful) handle API --------------------------------------

// StateCreate allocates a resident cevm StateDB and returns its opaque non-zero
// handle. Free it with StateFree when the chain is torn down.
func StateCreate() uint64 { return uint64(C.cevm_state_create()) }

// StateSeed (re)seeds the resident StateDB behind handle h from accts+storage and
// commits to establish its base root. A reseed FULLY RESETS the handle's state —
// this is the one full-state dump per sync (genesis or a drift resync).
func StateSeed(h uint64, accts []Account, storage []Storage) {
	var ca cAlloc
	defer ca.free()

	cAccts := marshalAccts(accts, &ca)
	cStor := marshalStorage(storage)

	var (
		acctPtr *C.cevm_pb_acct
		storPtr *C.cevm_pb_storage
	)
	if len(cAccts) > 0 {
		acctPtr = &cAccts[0]
	}
	if len(cStor) > 0 {
		storPtr = &cStor[0]
	}
	C.cevm_state_seed(C.uint64_t(h),
		acctPtr, C.uint32_t(len(cAccts)),
		storPtr, C.uint32_t(len(cStor)))
}

// StateApplyBlock replays txs against the RESIDENT StateDB behind h (NO reseed),
// returning the new MPT root + per-tx + logs + post-state delta. The resident
// tries make commit() incremental, so this is O(changed), not O(state), and no
// full-state dump crosses the FFI boundary.
func StateApplyBlock(h uint64, txs []Tx, ctx BlockCtx) (Result, error) {
	var ca cAlloc
	defer ca.free()

	cTxs := marshalTxs(txs, &ca)
	cCtx := marshalCtx(ctx)

	var txPtr *C.cevm_pb_tx
	if len(cTxs) > 0 {
		txPtr = &cTxs[0]
	}
	r := C.cevm_state_apply_block(C.uint64_t(h), txPtr, C.uint32_t(len(cTxs)), &cCtx)
	defer C.cevm_pb_free(&r)
	return unmarshalResult(r)
}

// StateRoot returns the current resident MPT root behind h (all-zero on an
// unknown handle). Used for sync checks.
func StateRoot(h uint64) [32]byte {
	var buf [32]C.uint8_t
	C.cevm_state_root(C.uint64_t(h), &buf[0])
	var root [32]byte
	for j := 0; j < 32; j++ {
		root[j] = byte(buf[j])
	}
	return root
}

// StateFree releases the resident StateDB behind h (no-op on an unknown handle).
func StateFree(h uint64) { C.cevm_state_free(C.uint64_t(h)) }

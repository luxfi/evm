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

// ProcessBlock seeds a cevm StateDB from accts+storage, replays txs through a
// real evmc_create_cevm() VM, and returns the real MPT state_root + per-tx
// status/gas. It is a pure function: it does not touch any Go state.
func ProcessBlock(accts []Account, storage []Storage, txs []Tx, ctx BlockCtx) (Result, error) {
	// C buffers allocated for code/data payloads; freed on return.
	var frees []unsafe.Pointer
	defer func() {
		for _, p := range frees {
			C.free(p)
		}
	}()

	// --- accounts --------------------------------------------------------
	cAccts := make([]C.cevm_pb_acct, len(accts))
	for i := range accts {
		a := &accts[i]
		for j := 0; j < 20; j++ {
			cAccts[i].address[j] = C.uint8_t(a.Address[j])
		}
		cAccts[i].nonce = C.uint64_t(a.Nonce)
		for j := 0; j < 4; j++ {
			cAccts[i].balance[j] = C.uint64_t(a.Balance[j])
		}
		if len(a.Code) > 0 {
			p := C.CBytes(a.Code)
			frees = append(frees, p)
			cAccts[i].code = (*C.uint8_t)(p)
			cAccts[i].code_len = C.uint32_t(len(a.Code))
		}
	}

	// --- storage ---------------------------------------------------------
	cStor := make([]C.cevm_pb_storage, len(storage))
	for i := range storage {
		s := &storage[i]
		for j := 0; j < 20; j++ {
			cStor[i].address[j] = C.uint8_t(s.Address[j])
		}
		for j := 0; j < 32; j++ {
			cStor[i].key[j] = C.uint8_t(s.Key[j])
			cStor[i].value[j] = C.uint8_t(s.Value[j])
		}
	}

	// --- transactions ----------------------------------------------------
	cTxs := make([]C.cevm_pb_tx, len(txs))
	for i := range txs {
		t := &txs[i]
		for j := 0; j < 20; j++ {
			cTxs[i].sender[j] = C.uint8_t(t.Sender[j])
			cTxs[i].recipient[j] = C.uint8_t(t.Recipient[j])
		}
		for j := 0; j < 32; j++ {
			cTxs[i].value[j] = C.uint8_t(t.Value[j])
			cTxs[i].gas_price[j] = C.uint8_t(t.GasPrice[j])
		}
		cTxs[i].gas_limit = C.uint64_t(t.GasLimit)
		cTxs[i].nonce = C.uint64_t(t.Nonce)
		if len(t.Data) > 0 {
			p := C.CBytes(t.Data)
			frees = append(frees, p)
			cTxs[i].data = (*C.uint8_t)(p)
			cTxs[i].data_len = C.uint32_t(len(t.Data))
		}
		if t.IsCreate {
			cTxs[i].is_create = 1
		}
	}

	// --- context ---------------------------------------------------------
	var cCtx C.cevm_pb_ctx
	for j := 0; j < 20; j++ {
		cCtx.coinbase[j] = C.uint8_t(ctx.Coinbase[j])
	}
	cCtx.block_number = C.int64_t(ctx.BlockNumber)
	cCtx.block_timestamp = C.int64_t(ctx.BlockTime)
	cCtx.block_gas_limit = C.int64_t(ctx.BlockGasLimit)
	for j := 0; j < 32; j++ {
		cCtx.chain_id[j] = C.uint8_t(ctx.ChainID[j])
		cCtx.block_base_fee[j] = C.uint8_t(ctx.BaseFee[j])
		cCtx.prev_randao[j] = C.uint8_t(ctx.PrevRandao[j])
		cCtx.blob_base_fee[j] = C.uint8_t(ctx.BlobBaseFee[j])
	}
	cCtx.revision = C.uint8_t(ctx.Revision)

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

	var res Result
	res.ABIVersion = uint32(r.abi_version)
	res.OK = r.ok != 0
	res.TotalGas = uint64(r.total_gas)
	for j := 0; j < 32; j++ {
		res.StateRoot[j] = byte(r.state_root[j])
	}
	if r.ok == 0 {
		return res, fmt.Errorf("cevmbridge: cevm_process_block returned ok=0")
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
	return res, nil
}

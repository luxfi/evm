// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package cevmbridge is the cgo seam from Go into the Lux C++ EVM (cevm)
// block-batch consensus entry point evm::state::process_block, exposed through
// the extern "C" ABI declared in luxcpp/cevm/lib/evm/state/go_process_block.h.
//
// ProcessBlock seeds a fresh C++ StateDB from a caller-supplied read-only
// snapshot of the pre-block accounts + storage, replays the block's
// transactions through a real evmc_create_cevm() VM, and returns the REAL
// keccak Merkle-Patricia-Trie state root (db.commit()) plus per-tx status and
// gas. It NEVER mutates the caller's Go state — it is a pure function of its
// inputs, which is exactly what a byte-exact shadow verifier needs.
//
// Two build variants share this one API:
//
//   - //go:build cevm : bridge_cevm.go links the fresh build-mpt CPU libs and
//     calls the real cevm_process_block. Enabled == true.
//   - //go:build !cevm : bridge_stub.go is pure Go, ProcessBlock returns
//     ErrDisabled. Enabled == false. This keeps `go build ./...` green on any
//     machine (no hardcoded C++ lib paths, no cgo link) while the real path is
//     opt-in via -tags cevm, mirroring the existing backend_cevm.go convention.
//
// All multi-byte scalar encodings match go_process_block.h exactly:
//   - Account.Balance : 4 x uint64 little-endian limbs, [0] = low 64 bits.
//   - Tx.Value / Tx.GasPrice, Storage.Key / Storage.Value, and the BlockCtx
//     256-bit fields : 32-byte BIG-endian.
package cevmbridge

import "errors"

// ErrDisabled is returned by ProcessBlock when the package was built without
// the `cevm` tag (the C++ EVM is not linked).
var ErrDisabled = errors.New("cevmbridge: built without -tags cevm (C++ EVM not linked)")

// Account is a read-only snapshot of one pre-block account, materialized into
// the cevm StateDB before the block runs.
type Account struct {
	Address [20]byte
	Nonce   uint64
	Balance [4]uint64 // little-endian limbs, [0] = low 64 bits
	Code    []byte    // nil/empty => EOA
}

// Storage is one pre-existing contract storage slot.
type Storage struct {
	Address [20]byte // owning account
	Key     [32]byte // big-endian slot key
	Value   [32]byte // big-endian slot value
}

// Tx is one transaction to replay. Value and GasPrice are 32-byte big-endian.
type Tx struct {
	Sender    [20]byte
	Recipient [20]byte // ignored when IsCreate
	Value     [32]byte // big-endian
	GasPrice  [32]byte // big-endian
	GasLimit  uint64
	Nonce     uint64
	Data      []byte // calldata or init code
	IsCreate  bool
}

// BlockCtx is the block / transaction context. The 256-bit fields are 32-byte
// big-endian; Revision is an evmc_revision value (see the CEVM_PB_REV_* consts
// mirrored below).
type BlockCtx struct {
	Coinbase      [20]byte
	BlockNumber   int64
	BlockTime     int64
	BlockGasLimit int64
	ChainID       [32]byte // big-endian
	BaseFee       [32]byte // big-endian
	PrevRandao    [32]byte // big-endian
	BlobBaseFee   [32]byte // big-endian
	Revision      uint8
}

// TxResult is the per-tx outcome cevm surfaced (index-aligned to the input txs).
type TxResult struct {
	EVMCStatus        int32 // raw evmc_status_code (SUCCESS=0, FAILURE=1, REVERT=2, ...)
	Status            uint8 // Go receipt status: 1 iff EVMC_SUCCESS
	Rejected          bool  // true iff EVMC_REJECTED (nonce/intrinsic/balance) — not block-includable
	GasUsed           uint64
	CumulativeGasUsed uint64
	NLogs             uint32 // logs this tx emitted (0 => within the proven safe subset)
}

// Log is one EVM log (LOG0..LOG4) cevm emitted. The three fields are the
// consensus fields (Address, Topics, Data); TxIndex identifies the emitting tx
// so the applier can attach the log to the right receipt. Logs arrive in tx
// order (the flat index is the block-wide log index).
type Log struct {
	Address [20]byte
	Topics  [][32]byte // each 32-byte big-endian topic
	Data    []byte
	TxIndex uint32
}

// PostAccount is one post-state account delta (DIFF final-vs-seed): an account
// the block CREATED or whose nonce/balance/code CHANGED, or (Deleted) a seed
// account the block REMOVED (selfdestruct). Values are ABSOLUTE post values —
// the applier writes them with SetNonce/SetBalance/SetCode. Code is present
// only when CodeChanged; otherwise the applier keeps the seed code.
type PostAccount struct {
	Address     [20]byte
	Nonce       uint64
	Balance     [4]uint64 // little-endian limbs, [0] = low 64 bits (absolute)
	CodeHash    [32]byte  // final keccak256(code)
	Code        []byte    // non-nil iff CodeChanged
	Deleted     bool      // true => remove the account (selfdestruct)
	CodeChanged bool      // true => Code differs from seed; apply SetCode
}

// PostStorage is one post-state storage delta: a slot whose POST value differs
// from the seed. Value all-zero encodes a CLEARED slot (SetState(key,0)). Slots
// of a selfdestructed account are covered by its PostAccount.Deleted and are not
// listed here.
type PostStorage struct {
	Address [20]byte
	Key     [32]byte // big-endian slot key
	Value   [32]byte // big-endian post value (all-zero => cleared)
}

// Result is the whole-block outcome. StateRoot + TxResults + TotalGas are what a
// shadow VERIFIER needs; Logs + PostAccounts + PostStorage are the additional
// ABI-v2 export an APPLIER needs to write cevm's post-state into the Go StateDB
// and rebuild the receipts.
type Result struct {
	StateRoot    [32]byte      // REAL keccak Merkle-Patricia-Trie root
	TxResults    []TxResult    // length == len(txs) on success
	Logs         []Log         // all txs' logs, in tx order (nil if none)
	PostAccounts []PostAccount // post-state account deltas (nil if none)
	PostStorage  []PostStorage // post-state storage deltas (nil if none)
	TotalGas     uint64
	OK           bool
	ABIVersion   uint32
}

// evmc_revision values (mirror go_process_block.h CEVM_PB_REV_*).
const (
	RevFrontier       uint8 = 0
	RevHomestead      uint8 = 1
	RevTangerine      uint8 = 2
	RevSpurious       uint8 = 3
	RevByzantium      uint8 = 4
	RevConstantinople uint8 = 5
	RevPetersburg     uint8 = 6
	RevIstanbul       uint8 = 7
	RevBerlin         uint8 = 8
	RevLondon         uint8 = 9
	RevParis          uint8 = 10
	RevShanghai       uint8 = 11
	RevCancun         uint8 = 12
)

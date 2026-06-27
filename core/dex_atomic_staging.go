// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"encoding/binary"
	"errors"

	"github.com/luxfi/database"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/ids"
	"github.com/luxfi/vm/chains/atomic"
)

// dex_atomic_staging.go is the HOST side of the LP-311 stateless-conduit atomic seam: the
// revert-safe StageExport / ConsumeImport the 0x9999 conduit type-asserts (contract.
// AtomicStager) so it can move cross-chain value objects WITHOUT writing any durable storage
// to its OWN (empty, reapable) account.
//
// ── WHERE THE STAGING LIVES (the reaping dissolution) ───────────────────────────────
// The staged cross-domain ops live in DexAtomicStagingAccount — a NON-EMPTY host system
// account, NOT the conduit's 0x9999 account and NOT a precompile. The host keeps it non-empty
// (a nonce marker on THIS account), so EIP-158 Finalise(deleteEmptyObjects=true) never reaps
// the staged records. The conduit (0x9999) writes ZERO own storage, so reaping the conduit is
// a no-op. Relocating the marker onto a genuinely-stateful host component is LP-311 §7's
// sanctioned "non-empty account" option — NOT the rejected workaround of marking the
// stateless conduit to defend a ledger it should not own.
//
// ── WHY EVM STATE (not a host-memory buffer) ────────────────────────────────────────
// The staging MUST be (a) revert-safe at inner-call granularity, (b) consensus-deterministic,
// and (c) durable to block Accept. The ONLY primitive a host gives a precompile that is all
// three is journaled EVM state (SetState): geth exposes no external custom-journal entry, and
// transient storage (EIP-1153) is cleared at tx end. A naive host-memory buffer gated on
// tx-receipt-status would MINT on a partial inner revert (a caught CALL revert that rolls back
// the escrow but not the host buffer). So the staging is revert-safe EVM state at the host
// account; a reverted frame discards the staged op atomically with its rolled-back escrow.
//
// ── DETERMINISTIC FLUSH WINDOW (the platformvm acceptor pattern) ────────────────────
// A monotonic per-account seq (part of the state root) brackets each block's staged ops; the
// host flushes the [parentSeq, currentSeq) window to shared memory in the SAME database batch
// as the accepted EVM state commit (FlushHostStagedAtomic, called from Block.Accept). Every
// validator replaying the ordered block derives the identical request set.

// DexAtomicStagingAccount is the host's NON-EMPTY atomic-staging backend account (LP-9994 in
// the DEX system-address band). It is the conduit's transport buffer — never 0x9999.
var DexAtomicStagingAccount = common.HexToAddress("0x0000000000000000000000000000000000009994")

// exportedObjectSize is the fixed cross-chain object width: rail(1)|owner(20)|asset(32)|
// amount(8)|spent(8) — byte-identical with precompile/dex native_wire.go and chains/dexvm.
const exportedObjectSize = 1 + 20 + 32 + 8 + 8

const (
	hostStageKindPut    byte = 1
	hostStageKindRemove byte = 2
)

var (
	// ErrHostStageBadObject rejects a malformed cross-chain object (wrong width) at stage time.
	ErrHostStageBadObject = errors.New("evm/dex: host atomic stage object is malformed (width)")
	// ErrHostStageNoSharedMemory is the fail-closed refusal when ConsumeImport is called on a
	// host that wired no shared memory (single-chain dev) — the conduit reverts, never mints.
	ErrHostStageNoSharedMemory = errors.New("evm/dex: host wired no shared memory for atomic consume")
	// ErrHostStageMalformed fails block accept on a malformed staged record (never skipped).
	ErrHostStageMalformed = errors.New("evm/dex: host staged atomic op malformed (version/length) — fatal")
	// ErrHostStageSeqRegression fails block accept on a staged-seq regression (corruption/reorg).
	ErrHostStageSeqRegression = errors.New("evm/dex: host staged atomic seq regressed — fatal")
)

const hostStageNS = "lux.dex.hoststage.v1."

func hostStageSlot(field string, seq uint64, word int) common.Hash {
	var id [16]byte
	binary.BigEndian.PutUint64(id[0:8], seq)
	binary.BigEndian.PutUint64(id[8:16], uint64(word))
	return common.Keccak256Hash([]byte(hostStageNS+field), id[:])
}

var hostStageSeqKey = common.Keccak256Hash([]byte(hostStageNS + "seq"))

// hostConsumedKey marks a D→C object (src,key) as consumed this block. It closes the
// within-block DOUBLE-CONSUME hole: a Remove staged by ConsumeImport only lands at Accept,
// so without this marker a second ConsumeImport(same key) in the same block would Get the
// still-present object and release escrow TWICE (a mint). The marker lives in the (non-empty)
// host account, is revert-safe (a reverted tx discards it with its escrow), and is durable
// (across blocks the object is gone from shared memory anyway, so the marker is harmless).
func hostConsumedKey(src, key ids.ID) common.Hash {
	return common.Keccak256Hash([]byte(hostStageNS+"consumed"), src[:], key[:])
}

// hostStageReader is the read-only slot accessor both the live StateDB and a committed
// *state.StateDB satisfy (GetState(addr,key)). The flush is read-only over the accepted state.
type hostStageReader interface {
	GetState(addr common.Address, key common.Hash) common.Hash
}

func readHostStageSeqFrom(r hostStageReader) uint64 {
	w := r.GetState(DexAtomicStagingAccount, hostStageSeqKey)
	return binary.BigEndian.Uint64(w[24:32])
}

// stagedAtomicState is the host-side StateDB surface StageExport/ConsumeImport write through:
// the geth vm.StateDB the precompile env exposes (SetState/GetState/Set+GetNonce). It is the
// SAME journaled, revert-safe, durable EVM state the escrow legs use, so a reverted tx
// discards the staged op atomically with the rolled-back escrow.
type stagedAtomicState interface {
	GetState(common.Address, common.Hash) common.Hash
	SetState(common.Address, common.Hash, common.Hash) common.Hash
	GetNonce(common.Address) uint64
	SetNonce(common.Address, uint64, tracing.NonceChangeReason)
}

// ensureHostStagingNonEmpty keeps the HOST staging account non-empty (nonce>=1) so the staged
// records survive Finalise(true). Idempotent and revert-safe (a journaled nonce change). The
// marker is on the HOST account, never the conduit 0x9999.
func ensureHostStagingNonEmpty(sdb stagedAtomicState) {
	if sdb.GetNonce(DexAtomicStagingAccount) == 0 {
		sdb.SetNonce(DexAtomicStagingAccount, 1, tracing.NonceChangeUnspecified)
	}
}

func writeHostStageWord(sdb stagedAtomicState, field string, seq uint64, word int, v common.Hash) {
	sdb.SetState(DexAtomicStagingAccount, hostStageSlot(field, seq, word), v)
}

func stageHostAtomic(sdb stagedAtomicState, kind byte, dst, key ids.ID, object []byte) {
	ensureHostStagingNonEmpty(sdb)
	seq := readHostStageSeqFrom(sdb)
	var meta common.Hash
	meta[31] = kind
	writeHostStageWord(sdb, "meta", seq, 0, meta)
	writeHostStageWord(sdb, "dst", seq, 0, common.BytesToHash(dst[:]))
	writeHostStageWord(sdb, "key", seq, 0, common.BytesToHash(key[:]))
	for i := 0; i*32 < len(object); i++ {
		var w common.Hash
		end := (i + 1) * 32
		if end > len(object) {
			end = len(object)
		}
		copy(w[:], object[i*32:end])
		writeHostStageWord(sdb, "obj", seq, i, w)
	}
	var seqWord common.Hash
	binary.BigEndian.PutUint64(seqWord[24:32], seq+1)
	sdb.SetState(DexAtomicStagingAccount, hostStageSeqKey, seqWord)
}

// CollectHostStagedRange reads the staged host atomic ops in the half-open window
// [fromSeq, toSeq) from a (committed) state view, in deterministic seq order, and returns them
// as an atomic.Requests map ready for sm.Apply. It FAILS (fatal block accept) on a malformed
// record — never silently skips one. fromSeq/toSeq come from consensus state (parent and
// current seq), so every validator derives the identical request set.
func CollectHostStagedRange(r hostStageReader, fromSeq, toSeq uint64) (map[ids.ID]*atomic.Requests, error) {
	if toSeq <= fromSeq {
		return nil, nil
	}
	out := make(map[ids.ID]*atomic.Requests)
	forChain := func(id ids.ID) *atomic.Requests {
		req, ok := out[id]
		if !ok {
			req = &atomic.Requests{}
			out[id] = req
		}
		return req
	}
	for seq := fromSeq; seq < toSeq; seq++ {
		meta := r.GetState(DexAtomicStagingAccount, hostStageSlot("meta", seq, 0))
		var dst, key ids.ID
		copy(dst[:], r.GetState(DexAtomicStagingAccount, hostStageSlot("dst", seq, 0)).Bytes())
		copy(key[:], r.GetState(DexAtomicStagingAccount, hostStageSlot("key", seq, 0)).Bytes())
		switch meta[31] {
		case hostStageKindPut:
			object := make([]byte, exportedObjectSize)
			for i := 0; i*32 < exportedObjectSize; i++ {
				w := r.GetState(DexAtomicStagingAccount, hostStageSlot("obj", seq, i))
				end := (i + 1) * 32
				if end > exportedObjectSize {
					end = exportedObjectSize
				}
				copy(object[i*32:end], w[:end-i*32])
			}
			// The owner Trait is bytes [1:21] of the object (past the rail byte), the same Trait
			// the peer chain's import side indexes by — so the destination finds it by recipient.
			var owner [20]byte
			copy(owner[:], object[1:21])
			req := forChain(dst)
			req.PutRequests = append(req.PutRequests, &atomic.Element{
				Key:    append([]byte(nil), key[:]...),
				Value:  object,
				Traits: [][]byte{append([]byte(nil), owner[:]...)},
			})
		case hostStageKindRemove:
			req := forChain(dst)
			req.RemoveRequests = append(req.RemoveRequests, append([]byte(nil), key[:]...))
		default:
			return nil, ErrHostStageMalformed
		}
	}
	return out, nil
}

// FlushHostStagedAtomic is the host's block-accept cross-domain commit for the conduit's
// staged ops. It derives the flush window from CONSENSUS STATE (parent and accepted staged
// seq), collects that window, and applies it to shared memory in the SAME database batch as
// the accepted EVM state commit (the platformvm acceptor pattern; never EVM-first-then-SM).
// It rejects a seq regression as fatal. sm/batch may be nil in single-chain dev (no-op).
func FlushHostStagedAtomic(parentState, currentState hostStageReader, sm atomic.SharedMemory, batch database.Batch) error {
	from := uint64(0)
	if parentState != nil {
		from = readHostStageSeqFrom(parentState)
	}
	to := readHostStageSeqFrom(currentState)
	if to < from {
		return ErrHostStageSeqRegression
	}
	reqs, err := CollectHostStagedRange(currentState, from, to)
	if err != nil {
		return err
	}
	if len(reqs) == 0 || sm == nil {
		return nil
	}
	if batch != nil {
		return sm.Apply(reqs, batch)
	}
	return sm.Apply(reqs)
}

// ── the accessibleStateAdapter host implementation of contract.AtomicStager ──────────

// StageExport records a revert-safe C→D atomic Put in the host staging account. The conduit
// (0x9999) calls this instead of writing its own storage. Validated object width; the owner
// Trait is read from the object at flush.
func (a *accessibleStateAdapter) StageExport(dst ids.ID, key ids.ID, object []byte) error {
	if len(object) != exportedObjectSize {
		return ErrHostStageBadObject
	}
	sdb, ok := a.env.StateDB().(stagedAtomicState)
	if !ok {
		return ErrHostStageNoSharedMemory
	}
	stageHostAtomic(sdb, hostStageKindPut, dst, key, object)
	return nil
}

// ConsumeImport reads a D→C atomic object from shared memory by (src, key), and on a hit
// records a revert-safe Remove staged for the next Accept. Returns the object so the conduit
// can value-bind and release escrow in the same tx. Fail-closed (no shared memory → error).
func (a *accessibleStateAdapter) ConsumeImport(src ids.ID, key ids.ID) ([]byte, bool, error) {
	sm := a.AtomicMemory()
	if sm == nil {
		return nil, false, ErrHostStageNoSharedMemory
	}
	sdb, ok := a.env.StateDB().(stagedAtomicState)
	if !ok {
		return nil, false, ErrHostStageNoSharedMemory
	}
	// EXACTLY-ONCE (within-block): a Remove staged below only lands at Accept, so reject a
	// re-consume of the same (src,key) in this block — else the conduit would release escrow
	// twice against one still-present object (a mint).
	if sdb.GetState(DexAtomicStagingAccount, hostConsumedKey(src, key)) != (common.Hash{}) {
		return nil, false, nil
	}
	vals, err := sm.Get(src, [][]byte{key[:]})
	if err != nil || len(vals) == 0 || vals[0] == nil {
		return nil, false, nil
	}
	ensureHostStagingNonEmpty(sdb)
	sdb.SetState(DexAtomicStagingAccount, hostConsumedKey(src, key), common.Hash{31: 1})
	stageHostAtomic(sdb, hostStageKindRemove, src, key, nil)
	return vals[0], true, nil
}

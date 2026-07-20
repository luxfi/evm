// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// cevm_shadow.go wires the Lux C++ EVM (cevm) into the block executor as a
// byte-exact SHADOW VERIFIER, not an applier.
//
// The map proved two facts that lock this design:
//   1. parallel.RegisterExecutor(e) makes core/state_processor.go route EVERY
//      block through e.ExecuteBlock (no SetBackend needed).
//   2. The CONSENSUS state root is taken from the GO statedb.IntermediateRoot()
//      (block_validator.go), NOT from cevm's returned root. So cevm as the
//      APPLIER would have to mutate the Go statedb via host callbacks (heavy,
//      deferred). As a VERIFIER it needs none of that.
//
// So this executor: (a) snapshots the pre-block Go state read-only, (b) replays
// the block's transactions through cevm's real process_block over that snapshot
// to get cevm's independent keccak-MPT root, (c) cross-checks that root against
// header.Root (the post-state root the Go EVM produced when the block was
// built), tallying agree/disagree/finalize-gap/declined counters, and then
// ALWAYS returns (nil, nil) so the Go sequential path is the one that actually
// applies the block. cevm never fabricates receipts and never touches the live
// state — the suite stays correct while cevm's real MPT is checked on every
// block it can represent.
//
// Root-only verification sidesteps BOTH known cevm gaps: it needs no logs (the
// receipts gap) and no state write-back (the applier gap). The proven safe
// subset (pure value transfers + non-reverting calls into existing code) yields
// a byte-exact match; everything else (top-level CREATE, REVERT/OOG, EIP-2930
// access lists, EIP-3529 refunds, or a state too large to snapshot) is tallied
// as a decline/disagree and the Go EVM's root stands unchallenged.
//
// This file is intentionally NOT behind //go:build cevm: it always compiles and
// always registers. When built without -tags cevm, cevmbridge.Enabled is false
// and ExecuteBlock returns (nil, nil) immediately — a behavioural no-op
// identical to the fallback executor, so the default build is unchanged.

import (
	"math/big"
	"sync/atomic"

	"github.com/holiman/uint256"

	"github.com/luxfi/evm/core/parallel/cevmbridge"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// Snapshot caps. A block whose pre-state exceeds these is declined (counted as
// declined-too-large) rather than risk an unbounded dump on live chains — the
// shadow verifier is a correctness harness, not a mainnet path.
const (
	maxShadowAccounts = 200_000
	maxShadowStorage  = 1_000_000
)

// Package-level shadow counters (atomic). Read via CevmShadowStats.
var (
	cevmProcessed          uint64 // blocks cevm actually ran and returned a root for
	cevmAgree              uint64 // cevm root == header.Root (byte-exact)
	cevmDisagree           uint64 // cevm root != header.Root, no known-alignment excuse
	cevmFinalizeGap        uint64 // mismatch attributable to withdrawals (cevm runs no engine.Finalize)
	cevmDeclinedTooLarge   uint64 // pre-state exceeded the snapshot caps
	cevmDeclinedNoPreimage uint64 // an account address was unrecoverable (preimages off) — cannot seed cevm
	cevmDeclinedTx         uint64 // a tx sender could not be recovered
	cevmErrored            uint64 // cevm process_block failed / panicked
)

// ShadowStats is a snapshot of the cevm shadow-verifier counters.
type ShadowStats struct {
	Processed          uint64
	Agree              uint64
	Disagree           uint64
	FinalizeGap        uint64
	DeclinedTooLarge   uint64
	DeclinedNoPreimage uint64
	DeclinedTx         uint64
	Errored            uint64
}

// CevmShadowStats returns the current cevm shadow-verifier counters.
func CevmShadowStats() ShadowStats {
	return ShadowStats{
		Processed:          atomic.LoadUint64(&cevmProcessed),
		Agree:              atomic.LoadUint64(&cevmAgree),
		Disagree:           atomic.LoadUint64(&cevmDisagree),
		FinalizeGap:        atomic.LoadUint64(&cevmFinalizeGap),
		DeclinedTooLarge:   atomic.LoadUint64(&cevmDeclinedTooLarge),
		DeclinedNoPreimage: atomic.LoadUint64(&cevmDeclinedNoPreimage),
		DeclinedTx:         atomic.LoadUint64(&cevmDeclinedTx),
		Errored:            atomic.LoadUint64(&cevmErrored),
	}
}

// ResetCevmShadowStats zeroes the counters (test helper).
func ResetCevmShadowStats() {
	for _, p := range []*uint64{
		&cevmProcessed, &cevmAgree, &cevmDisagree, &cevmFinalizeGap,
		&cevmDeclinedTooLarge, &cevmDeclinedNoPreimage, &cevmDeclinedTx, &cevmErrored,
	} {
		atomic.StoreUint64(p, 0)
	}
}

// CevmShadowEnabled reports whether the real cevm bridge is linked (-tags cevm).
func CevmShadowEnabled() bool { return cevmbridge.Enabled }

func init() {
	// Register only when the real C++ EVM is linked (-tags cevm). In the default
	// and -tags parallel builds the bridge is a no-op stub, so registering would
	// only risk clobbering another package's real BlockExecutor (e.g. a Block-STM
	// engine) with a verifier that always declines. Gating on Enabled keeps those
	// builds byte-for-byte unchanged while activating the shadow under -tags cevm.
	if cevmbridge.Enabled {
		RegisterExecutor(cevmShadowExecutor{})
	}
}

// cevmShadowExecutor is a BlockExecutor that only ever verifies: it returns
// (nil, nil) unconditionally so the Go sequential path applies the block.
type cevmShadowExecutor struct{}

// ExecuteBlock runs the cevm shadow cross-check and ALWAYS declines. It never
// returns receipts or an error, so it can never break block processing — a
// mismatch or an internal failure only moves a counter.
func (cevmShadowExecutor) ExecuteBlock(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
	_ vm.Config,
) ([]*types.Receipt, error) {
	if !cevmbridge.Enabled || config == nil || header == nil || statedb == nil || len(txs) == 0 {
		return nil, nil
	}
	// The verifier must NEVER destabilise consensus: any panic in the snapshot
	// or bridge is swallowed and counted, and we still decline.
	defer func() {
		if r := recover(); r != nil {
			atomic.AddUint64(&cevmErrored, 1)
		}
	}()
	shadowVerify(config, header, txs, statedb)
	return nil, nil
}

// shadowVerify performs the read-only snapshot, cevm replay, and root
// cross-check, updating exactly one counter.
func shadowVerify(
	config *ethparams.ChainConfig,
	header *types.Header,
	txs types.Transactions,
	statedb *state.StateDB,
) {
	// (a) Snapshot the pre-block state read-only. DumpToCollector iterates the
	// trie at statedb's original (parent) root, which is exactly the pre-block
	// state the block was applied to.
	snap := &shadowSnapshot{maxAccts: maxShadowAccounts, maxStorage: maxShadowStorage}
	statedb.DumpToCollector(snap, &state.DumpConfig{})
	switch {
	case snap.tooLarge:
		atomic.AddUint64(&cevmDeclinedTooLarge, 1)
		return
	case snap.missingAddr || snap.badBalance:
		// Addresses come from trie-key preimages; without them cevm cannot be
		// seeded (its leaf key is keccak256(addr)). Decline honestly.
		atomic.AddUint64(&cevmDeclinedNoPreimage, 1)
		return
	}

	// (b) Build the cevm tx array from the block's transactions.
	signer := types.MakeSigner(config, header.Number, header.Time)
	ctxs := make([]cevmbridge.Tx, len(txs))
	for i, tx := range txs {
		from, err := types.Sender(signer, tx)
		if err != nil {
			atomic.AddUint64(&cevmDeclinedTx, 1)
			return
		}
		var t cevmbridge.Tx
		copy(t.Sender[:], from[:])
		if to := tx.To(); to != nil {
			copy(t.Recipient[:], to[:])
		} else {
			t.IsCreate = true
		}
		fillBE32(&t.Value, tx.Value())
		fillBE32(&t.GasPrice, effectiveGasPrice(tx, header.BaseFee))
		t.GasLimit = tx.Gas()
		t.Nonce = tx.Nonce()
		if d := tx.Data(); len(d) > 0 {
			t.Data = d
		}
		ctxs[i] = t
	}

	// (c) cevm's independent root over the same pre-state + txs.
	res, err := cevmbridge.ProcessBlock(snap.accts, snap.storage, ctxs, blockCtx(config, header))
	if err != nil || !res.OK {
		atomic.AddUint64(&cevmErrored, 1)
		return
	}
	atomic.AddUint64(&cevmProcessed, 1)

	// (d) Cross-check against the root the Go EVM committed for this block.
	if common.BytesToHash(res.StateRoot[:]) == header.Root {
		atomic.AddUint64(&cevmAgree, 1)
		return
	}
	// Known-alignment gap: cevm credits the coinbase per-tx (gas*gasPrice, which
	// matches evm's full-fee-to-coinbase state_transition) but does NOT run
	// engine.Finalize. On Lux that Finalize is a state no-op EXCEPT for
	// EIP-4895 withdrawals, which move header.Root. Attribute such mismatches to
	// the finalize gap rather than blaming cevm.
	if header.WithdrawalsHash != nil && *header.WithdrawalsHash != types.EmptyWithdrawalsHash {
		atomic.AddUint64(&cevmFinalizeGap, 1)
		return
	}
	atomic.AddUint64(&cevmDisagree, 1)
}

// effectiveGasPrice mirrors evm/core/state_transition.go: the price used for the
// sender debit and the full coinbase fee credit. For a legacy tx this is the
// gas price; for a dynamic-fee tx, min(gasFeeCap, gasTipCap+baseFee).
func effectiveGasPrice(tx *types.Transaction, baseFee *big.Int) *big.Int {
	if baseFee == nil {
		return tx.GasPrice()
	}
	sum := new(big.Int).Add(tx.GasTipCap(), baseFee)
	if sum.Cmp(tx.GasFeeCap()) > 0 {
		return new(big.Int).Set(tx.GasFeeCap())
	}
	return sum
}

// blockCtx builds the cevm block context from the header. The 256-bit fields are
// big-endian; PrevRandao is the post-merge MixDigest. Revision is derived from
// the chain config (immaterial to the root for the proven transfer subset).
func blockCtx(config *ethparams.ChainConfig, header *types.Header) cevmbridge.BlockCtx {
	var c cevmbridge.BlockCtx
	copy(c.Coinbase[:], header.Coinbase[:])
	if header.Number != nil {
		c.BlockNumber = header.Number.Int64()
	}
	c.BlockTime = int64(header.Time)
	c.BlockGasLimit = int64(header.GasLimit)
	if config.ChainID != nil {
		fillBE32(&c.ChainID, config.ChainID)
	}
	if header.BaseFee != nil {
		fillBE32(&c.BaseFee, header.BaseFee)
	}
	copy(c.PrevRandao[:], header.MixDigest[:])
	c.Revision = revisionOf(config, header)
	return c
}

func revisionOf(config *ethparams.ChainConfig, header *types.Header) uint8 {
	num, t := header.Number, header.Time
	switch {
	case config.IsCancun(num, t):
		return cevmbridge.RevCancun
	case config.IsShanghai(num, t):
		return cevmbridge.RevShanghai
	case config.IsLondon(num):
		return cevmbridge.RevLondon
	case config.IsBerlin(num):
		return cevmbridge.RevBerlin
	case config.IsIstanbul(num):
		return cevmbridge.RevIstanbul
	default:
		return cevmbridge.RevIstanbul
	}
}

// fillBE32 writes v as a 32-byte big-endian value (right-aligned, zero-padded).
func fillBE32(dst *[32]byte, v *big.Int) {
	*dst = [32]byte{}
	if v == nil || v.Sign() == 0 {
		return
	}
	b := v.Bytes() // big-endian, minimal
	if len(b) >= 32 {
		copy(dst[:], b[len(b)-32:])
		return
	}
	copy(dst[32-len(b):], b)
}

// shadowSnapshot is a DumpCollector that materializes the pre-block accounts +
// storage into cevmbridge structs, bounded by the snapshot caps.
type shadowSnapshot struct {
	maxAccts, maxStorage int
	accts                []cevmbridge.Account
	storage              []cevmbridge.Storage
	missingAddr          bool
	badBalance           bool
	tooLarge             bool
}

func (s *shadowSnapshot) OnRoot(common.Hash) {}

func (s *shadowSnapshot) OnAccount(addr *common.Address, acc state.DumpAccount) {
	if s.tooLarge || s.missingAddr || s.badBalance {
		return
	}
	if addr == nil {
		// No preimage for this trie key: cannot recover the 20-byte address.
		s.missingAddr = true
		return
	}
	if len(s.accts) >= s.maxAccts {
		s.tooLarge = true
		return
	}
	var a cevmbridge.Account
	copy(a.Address[:], addr[:])
	a.Nonce = acc.Nonce
	bal, err := uint256.FromDecimal(acc.Balance)
	if err != nil {
		s.badBalance = true
		return
	}
	a.Balance = [4]uint64{bal[0], bal[1], bal[2], bal[3]}
	if len(acc.Code) > 0 {
		a.Code = append([]byte(nil), acc.Code...)
	}
	s.accts = append(s.accts, a)

	for k, vhex := range acc.Storage {
		if len(s.storage) >= s.maxStorage {
			s.tooLarge = true
			return
		}
		var st cevmbridge.Storage
		copy(st.Address[:], addr[:])
		copy(st.Key[:], k[:])
		vb := common.Hex2Bytes(vhex) // dump value is hex without 0x, big-endian minimal
		if len(vb) >= 32 {
			copy(st.Value[:], vb[len(vb)-32:])
		} else {
			copy(st.Value[32-len(vb):], vb)
		}
		s.storage = append(s.storage, st)
	}
}

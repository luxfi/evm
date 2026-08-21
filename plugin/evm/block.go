// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/luxfi/geth/rlp"
	log "github.com/luxfi/log"

	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/plugin/evm/customtypes"
	"github.com/luxfi/evm/plugin/evm/header"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/evm/predicate"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"

	"github.com/luxfi/consensus/core/choices"
	"github.com/luxfi/ids"
	block "github.com/luxfi/vm/chain"
)

var (
	// Block implements the chain.Block interface
	_ block.Block = (*Block)(nil)
)

// Block implements the chain.Block interface
type Block struct {
	id       ids.ID
	ethBlock *types.Block
	vm       *VM
}

// newBlock returns a new Block wrapping the ethBlock type and implementing the chain.Block interface
func (vm *VM) newBlock(ethBlock *types.Block) *Block {
	return &Block{
		id:       ids.ID(ethBlock.Hash()),
		ethBlock: ethBlock,
		vm:       vm,
	}
}

// ID implements the chain.Block interface
func (b *Block) ID() ids.ID { return b.id }

// alreadyAccepted reports whether this exact execution block is on the VM's
// durable accepted canonical chain. A proposer wrapper may legitimately change
// while carrying the same inner EVM block (for example, catch-up adopts a
// quorum-certified wrapper after an older node accepted a locally proposed
// wrapper). Re-executing an already accepted historical block is both unnecessary
// and, after pruning its parent state, impossible.
//
// The height bound is essential. GetCanonicalHash also contains preferred blocks
// above the accepted floor, so hash equality alone would let a merely processing
// block skip Verify/Accept. At or below LastConsensusAcceptedBlock, however, the
// canonical hash is the VM's own durable execution decision. Exact hash equality
// is therefore the safe idempotency key for both the tip and its history.
func (b *Block) alreadyAccepted() bool {
	if b == nil || b.vm == nil || b.ethBlock == nil {
		return false
	}
	// Keep the cheap tip check usable by small/embedder VMs that only provide the
	// chain.State wrapper (and by the focused unit test below).
	if b.vm.State != nil && b.vm.State.LastAcceptedBlockInternal().ID() == b.ID() {
		return true
	}
	if b.vm.blockChain == nil {
		return false
	}
	lastAccepted := b.vm.blockChain.LastConsensusAcceptedBlock()
	if lastAccepted == nil || b.ethBlock.NumberU64() > lastAccepted.NumberU64() {
		return false
	}
	return b.vm.blockChain.GetCanonicalHash(b.ethBlock.NumberU64()) == b.ethBlock.Hash()
}

// Accept implements the chain.Block interface
func (b *Block) Accept(context.Context) error {
	vm := b.vm
	if b.alreadyAccepted() {
		// The execution state, accepted pointer, receipts, precompile effects and
		// atomic markers for this exact hash are already durable. This includes a
		// historical canonical block below the current tip. Replaying Accept would
		// duplicate those side effects; the outer proposer wrapper is the only layer
		// that still needs to advance.
		return nil
	}

	// Although returning an error from Accept is considered fatal, it is good
	// practice to cleanup the batch we were modifying in the case of an error.
	defer vm.versiondb.Abort()

	blkID := b.ID()
	log.Debug("accepting block",
		"hash", blkID.Hex(),
		"id", blkID,
		"height", b.Height(),
	)

	// Call Accept for relevant precompile logs. Note we do this prior to
	// calling Accept on the blockChain so any side effects (eg warp signatures)
	// take place before the accepted log is emitted to subscribers.
	rules := b.vm.rules(b.ethBlock.Number(), b.ethBlock.Time())
	if err := b.handlePrecompileAccept(rules); err != nil {
		return err
	}
	if err := vm.blockChain.Accept(b.ethBlock); err != nil {
		return fmt.Errorf("chain could not accept %s: %w", blkID, err)
	}

	// Tell the builder which head is now accepted. It waits for the transaction pool
	// to reset onto this same hash before building again — without that it spins,
	// assembling against a pool that still lists what this block just mined.
	vm.builderLock.Lock()
	if vm.builder != nil {
		vm.builder.setChainHeadHash(b.ethBlock.Hash())
	}
	vm.builderLock.Unlock()

	// DEX native C<->D atomic seam: stage this block's atomic ops (0x9999
	// SubmitSwapIntent C->D Puts / ImportSettlement D->C Removes) for flush to shared
	// memory. The flush window is derived from CONSENSUS STATE (parent staged-seq ->
	// accepted staged-seq), so every validator applies the identical request set; the
	// staged ops are append-only EVM state, so a reverted tx's staging was already
	// discarded. stageDexAtomic advances the node-local last-applied marker INTO the
	// versiondb in-memory layer and returns the requests + shared memory; the marker
	// and the shared-memory Apply commit together below (one atomic write), which is
	// what makes the flush exactly-once and crash-safe. See native_staging.go.
	atomicReqs, atomicSM, err := vm.stageDexAtomic(b)
	if err != nil {
		return fmt.Errorf("dex atomic flush failed for %s: %w", blkID, err)
	}

	if err := vm.acceptedBlockDB.Put(lastAcceptedKey, blkID[:]); err != nil {
		return fmt.Errorf("failed to put %s as the last accepted block: %w", blkID, err)
	}

	// COMMIT THE STATE, THEN APPLY. The versiondb commit (carrying the advanced
	// dexAtomicSeqKey marker AND lastAcceptedKey) lands FIRST; the shared-memory
	// mutation follows as its own write.
	//
	// WHY NOT ONE ATOMIC WRITE. Handing our batch to sm.Apply is the in-process
	// platformvm acceptor pattern, and it is simply unavailable here: the C-Chain EVM
	// is an out-of-process plugin, its SharedMemory is the ZAP client, and a database
	// batch cannot cross a process boundary — atomiczap.Apply refuses one outright
	// (ErrBatchUnsupported). Passing it unconditionally, as this code did, turns EVERY
	// block that stages an atomic op into a FATAL Accept: the seam would halt the chain
	// on the first swap it ever settled.
	//
	// WHY THIS ORDER AND NOT THE REVERSE. Without a shared batch, exactly-once is
	// unreachable; the choice is at-most-once (commit first: an op can be SKIPPED on a
	// crash in the window between the two writes) or at-least-once (apply first: an op
	// can be REPLAYED). Replay is not survivable here and is not made survivable by any
	// property of the transport: the shared-memory layer's SetValue returns
	// errDuplicatePut for a key already present, which is a FATAL Accept — the chain
	// halt this seam has already caused — and, worse, a Put replayed after the peer has
	// consumed the object RE-CREATES it, letting the peer be funded twice. A skip has no
	// such edge:
	//
	//   - a skipped C->D Put   : the taker's principal stays locked on C and D never
	//                            sees the intent; reclaimIntent refunds it once the
	//                            deadline passes (every intent carries a finite one by
	//                            construction — defaultedReclaimDeadline). Bounded
	//                            liveness loss, no value created or destroyed.
	//   - a skipped D->C Remove: the object lingers in shared memory, but C's replay
	//                            guard for a consumed object is EVM STATE
	//                            (isSettlementConsumed), committed above — not the
	//                            Remove. It can never be consumed twice. Inert garbage.
	//
	// So the failure mode of this ordering is recoverable and the failure mode of the
	// other is a halt or a double-spend. The window itself is the microseconds between
	// two writes on the accept path.
	//
	// STATED HONESTLY, THE RESIDUAL: a skipped Put leaves THIS node's shared memory
	// short one object that its peers hold, so its D-Chain cannot verify a block that
	// imports that intent and will stall on it until an operator re-applies. That is a
	// single-node wedge, not a network fork and not a value leak. Closing the window
	// entirely needs exactly-once across the process boundary, which needs a typed,
	// exported duplicate-operation error on atomic.SharedMemory that survives the ZAP
	// wire — then the flush could apply first and tolerate replays. Matching an
	// unexported error by string across an RPC boundary ON THE MONEY PATH is not a
	// trade worth making, so the simple ordering stands until that error exists.
	//
	// An Apply that ERRORS (as opposed to a crash) stays FATAL deliberately: a node
	// that cannot mutate shared memory must stop rather than run on with a divergent
	// view of it.
	if err := vm.versiondb.Commit(); err != nil {
		return fmt.Errorf("failed to commit accepted state for %s: %w", blkID, err)
	}
	if len(atomicReqs) > 0 && atomicSM != nil {
		if aerr := atomicSM.Apply(atomicReqs); aerr != nil {
			return fmt.Errorf("dex atomic shared-memory apply for %s: %w", blkID, aerr)
		}
	}
	return nil
}

// handlePrecompileAccept calls Accept on any logs generated with an active precompile address that implements
// contract.Accepter
func (b *Block) handlePrecompileAccept(rules extras.Rules) error {
	// Short circuit early if there are no precompile accepters to execute
	if len(rules.AccepterPrecompiles) == 0 {
		return nil
	}

	// Read receipts from disk
	receipts := rawdb.ReadReceipts(b.vm.chaindb, b.ethBlock.Hash(), b.ethBlock.NumberU64(), b.ethBlock.Time(), b.vm.chainConfig)
	// If there are no receipts, ReadReceipts may be nil, so we check the length and confirm the ReceiptHash
	// is empty to ensure that missing receipts results in an error on accept.
	if len(receipts) == 0 && b.ethBlock.ReceiptHash() != types.EmptyRootHash {
		return fmt.Errorf("failed to fetch receipts for accepted block with non-empty root hash (%s) (Block: %s, Height: %d)", b.ethBlock.ReceiptHash(), b.ethBlock.Hash(), b.ethBlock.NumberU64())
	}
	acceptCtx := &precompileconfig.AcceptContext{
		ConsensusCtx: context.Background(),
		Warp:         b.vm.warpBackend,
	}
	for _, receipt := range receipts {
		for logIdx, log := range receipt.Logs {
			accepter, ok := rules.AccepterPrecompiles[log.Address]
			if !ok {
				continue
			}
			if err := accepter.Accept(acceptCtx, log.BlockHash, log.BlockNumber, log.TxHash, logIdx, log.Topics, log.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

// Reject implements the chain.Block interface
func (b *Block) Reject(context.Context) error {
	blkID := b.ID()
	log.Debug("rejecting block",
		"hash", blkID.Hex(),
		"id", blkID,
		"height", b.Height(),
	)
	return b.vm.blockChain.Reject(b.ethBlock)
}

// Parent implements the chain.Block interface
func (b *Block) Parent() ids.ID {
	return ids.ID(b.ethBlock.ParentHash())
}

// ParentID implements the chain.Block interface (same as Parent)
func (b *Block) ParentID() ids.ID {
	return ids.ID(b.ethBlock.ParentHash())
}

// Height implements the chain.Block interface
func (b *Block) Height() uint64 {
	return b.ethBlock.NumberU64()
}

// Timestamp implements the chain.Block interface
func (b *Block) Timestamp() time.Time {
	return time.Unix(int64(b.ethBlock.Time()), 0)
}

// syntacticVerify verifies that a *Block is well-formed.
// ensureBlockGasCost populates this block's derived BlockGasCost from its PARENT header when it is
// not already set. BlockGasCost is NOT part of the RLP-encoded block — it is computed from the
// parent (block-gas economics), so it can only be filled once the parent header is available.
//
// THE PARSE/VERIFY DECOMPLECTION: parse (ParseBlock/UnmarshalBlock) must NOT require the parent — a
// bootstrap descent parses ancestry blocks AHEAD of the accepted height, whose parents are not yet
// present. Best-effort population at parse (parent present ⇒ fill it) keeps the live path fast, and
// leaving it nil for an ahead-of-accepted block lets the block still parse (its id/height/parent —
// all the descent's content-addressed walk needs — come from the decoded header). Verify/Accept then
// runs with the parent ACCEPTED and calls this to fill BlockGasCost authoritatively, after which the
// consensus/dummy header verification enforces its correctness against the recomputed expected value.
// SetHeaderExtra is keyed by header hash (a global map), so a value filled here persists even for a
// block object first parsed (and cached) while its parent was absent. no-op when already set, at
// genesis (always 0), or when the parent is still absent.
func (b *Block) ensureBlockGasCost() {
	if b == nil || b.ethBlock == nil {
		return
	}
	ethHeader := b.ethBlock.Header()
	if ethHeader.Number == nil {
		return
	}
	if ethHeader.Number.Uint64() == 0 {
		if customtypes.GetHeaderExtra(ethHeader).BlockGasCost == nil {
			extra := customtypes.GetHeaderExtra(ethHeader)
			extra.BlockGasCost = big.NewInt(0)
			customtypes.SetHeaderExtra(ethHeader, extra)
		}
		return
	}
	if customtypes.GetHeaderExtra(ethHeader).BlockGasCost != nil {
		return
	}
	parent := b.vm.blockChain.GetHeaderByHash(ethHeader.ParentHash)
	if parent == nil {
		return // parent not yet present (ahead-of-accepted parse) — Verify fills it with the parent present
	}
	config := params.GetExtra(b.vm.chainConfig)
	feeConfig, _, err := b.vm.blockChain.GetFeeConfigAt(parent)
	if err != nil || !config.IsEVM(ethHeader.Time) {
		return
	}
	extra := customtypes.GetHeaderExtra(ethHeader)
	extra.BlockGasCost = header.BlockGasCost(config, feeConfig, parent, ethHeader.Time)
	customtypes.SetHeaderExtra(ethHeader, extra)
}

func (b *Block) syntacticVerify() error {
	if b == nil || b.ethBlock == nil {
		return errInvalidBlock
	}

	header := b.ethBlock.Header()
	rules := b.vm.chainConfig.Rules(header.Number, params.IsMergeTODO, header.Time)
	return b.vm.syntacticBlockValidator.SyntacticVerify(b, rules)
}

// Verify implements the chain.Block interface
func (b *Block) Verify(context.Context) error {
	return b.verify(&precompileconfig.PredicateContext{
		ConsensusCtx:       context.Background(),
		ProposerVMBlockCtx: nil,
	}, true)
}

// ShouldVerifyWithContext implements the block.WithVerifyContext interface
func (b *Block) ShouldVerifyWithContext(context.Context) (bool, error) {
	rules := b.vm.rules(b.ethBlock.Number(), b.ethBlock.Time())
	predicates := rules.Predicaters
	// Short circuit early if there are no predicates to verify
	if len(predicates) == 0 {
		return false, nil
	}

	// Check if any of the transactions in the block specify a precompile that enforces a predicate, which requires
	// the ProposerVMBlockCtx.
	for _, tx := range b.ethBlock.Transactions() {
		for _, accessTuple := range tx.AccessList() {
			if _, ok := predicates[accessTuple.Address]; ok {
				log.Debug("Block verification requires proposerVM context", "block", b.ID(), "height", b.Height())
				return true, nil
			}
		}
	}

	log.Debug("Block verification does not require proposerVM context", "block", b.ID(), "height", b.Height())
	return false, nil
}

// VerifyWithContext implements the block.WithVerifyContext interface
func (b *Block) VerifyWithContext(ctx context.Context, proposerVMBlockCtx *block.Context) error {
	// Convert from node's block.Context to consensus's block.Context
	var consensusBlockCtx *block.Context
	if proposerVMBlockCtx != nil {
		consensusBlockCtx = &block.Context{
			PChainHeight: proposerVMBlockCtx.PChainHeight,
		}
	}

	return b.verify(&precompileconfig.PredicateContext{
		ConsensusCtx:       b.vm.ctx,
		ProposerVMBlockCtx: consensusBlockCtx,
	}, true)
}

// Verify the block is valid.
// Enforces that the predicates are valid within [predicateContext].
// Writes the block details to disk and the state to the trie manager iff writes=true.
func (b *Block) verify(predicateContext *precompileconfig.PredicateContext, writes bool) error {
	if b.alreadyAccepted() {
		// Catch-up can receive a different, quorum-certified proposer wrapper around
		// an exact EVM block this node already accepted. Its parent state may have
		// been pruned, so InsertBlockManual cannot re-execute it. Canonical hash
		// equality at or below the durable accepted tip proves this execution result
		// was already verified and committed; only the outer wrapper remains.
		return nil
	}
	if predicateContext.ProposerVMBlockCtx != nil {
		log.Debug("Verifying block with context", "block", b.ID(), "height", b.Height())
	} else {
		log.Debug("Verifying block without context", "block", b.ID(), "height", b.Height())
	}
	// Populate the parent-derived BlockGasCost now that the parent is ACCEPTED (Verify runs after
	// the parent). A block first parsed while its parent was absent (a bootstrap descent parsing
	// ancestry ahead of the accepted height) carries a nil BlockGasCost; fill it here so the
	// consensus/dummy header verification below (InsertBlockManual) enforces its correctness. This
	// is the VERIFY half of the parse/verify decomplection — parse no longer requires the parent.
	b.ensureBlockGasCost()
	if err := b.syntacticVerify(); err != nil {
		return fmt.Errorf("syntactic block verification failed: %w", err)
	}

	// Only enforce predicates if the chain has already bootstrapped.
	// If the chain is still bootstrapping, we can assume that all blocks we are verifying have
	// been accepted by the network (so the predicate was validated by the network when the
	// block was originally verified).
	if b.vm.bootstrapped.Get() {
		if err := b.verifyPredicates(predicateContext); err != nil {
			return fmt.Errorf("failed to verify predicates: %w", err)
		}
	}

	// The engine may call VerifyWithContext multiple times on the same block with different contexts.
	// Since the engine will only call Accept/Reject once, we should only call InsertBlockManual once.
	// Additionally, if a block is already in processing, then it has already passed verification and
	// at this point we have checked the predicates are still valid in the different context so we
	// can return nil.
	if b.vm.IsProcessing(b.id) {
		return nil
	}

	return b.vm.blockChain.InsertBlockManual(b.ethBlock, writes)
}

// verifyPredicates verifies the predicates in the block are valid according to predicateContext.
func (b *Block) verifyPredicates(predicateContext *precompileconfig.PredicateContext) error {
	// Use RulesAt to properly set up the RulesExtra context for precompile checks
	rules := params.RulesAt(b.vm.chainConfig, b.ethBlock.Number(), params.IsMergeTODO, b.ethBlock.Time())
	rulesExtra := params.GetRulesExtra(rules)

	switch {
	case !rulesExtra.IsDurango && rulesExtra.PredicatersExist:
		return errors.New("cannot enable predicates before Durango activation")
	case !rulesExtra.IsDurango:
		return nil
	}

	predicateResults := predicate.NewResults()
	for _, tx := range b.ethBlock.Transactions() {
		results, err := core.CheckPredicates(rules, predicateContext, tx)
		if err != nil {
			return err
		}
		predicateResults.SetTxResults(tx.Hash(), results)
	}
	// NOTE: document required gas constraints to ensure marshalling predicate results does not error
	predicateResultsBytes, err := predicateResults.Bytes()
	if err != nil {
		return fmt.Errorf("failed to marshal predicate results: %w", err)
	}
	extraData := b.ethBlock.Extra()
	headerPredicateResultsBytes := header.PredicateBytesFromExtra(extraData)
	if !bytes.Equal(headerPredicateResultsBytes, predicateResultsBytes) {
		return fmt.Errorf("%w (remote: %x local: %x)", errInvalidHeaderPredicateResults, headerPredicateResultsBytes, predicateResultsBytes)
	}
	return nil
}

// Bytes implements the chain.Block interface
func (b *Block) Bytes() []byte {
	res, err := rlp.EncodeToBytes(b.ethBlock)
	if err != nil {
		panic(err)
	}
	return res
}

func (b *Block) String() string { return fmt.Sprintf("EVM block, ID = %s", b.ID()) }

// Status implements the chain.Block interface
func (b *Block) Status() uint8 {
	// Return a simple status based on if the block is in the blockchain
	// 0 = unknown, 1 = processing, 2 = accepted, 3 = rejected
	// For simplicity, we'll return 2 (accepted) for now since this is mainly used in consensus
	return 2
}

// SetStatus implements the chain.Block interface
// This is required for chain.Block but not used in our implementation
func (b *Block) SetStatus(status choices.Status) {
	// No-op: EVM blocks manage their status internally through the blockchain
}

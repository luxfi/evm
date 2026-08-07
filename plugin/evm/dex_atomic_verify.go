// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"fmt"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/precompile/dex"
)

// dex_atomic_verify.go is where THE SHIP RULE is proven for the 0x9999 C<->D seam:
// a C balance can be credited by a cross-chain settlement ONLY by consuming a real
// atomic object the peer chain exported.
//
// WHY IT LIVES HERE AND NOT IN EXECUTION. The rule used to be enforced inside EVM
// execution, by reading the object out of shared memory while the matching Remove
// landed at block accept. Those two facts cannot both hold: a node re-executing that
// block afterwards — bootstrapping, state-syncing, re-tracing an archive call —
// found the object already consumed, computed a reverted receipt where the network
// had computed a successful one, and died with `invalid receipt root hash`. The seam
// settled live and wedged every node that tried to sync past it.
//
// So execution now binds value to the object bytes the TRANSACTION carried (a pure
// function of the block, replayable forever) and DECLARES the consumption in a log.
// This hook proves each declaration against shared memory at VERIFY. A forged object
// makes the BLOCK invalid instead of diverging a receipt: the producer verifies its
// own block before proposing it (buildBlock -> blk.verify), so a forgery is refused
// before it is ever seen by anyone else, and every other validator refuses it too.
//
// This is the primary network's own import discipline — object in the transaction,
// shared memory consulted at Verify, consumption applied at Accept — expressed for a
// precompile seam instead of a first-class atomic transaction.
//
// THE BOOTSTRAP GATE IS REQUIRED, NOT AN OPTIMIZATION. A node replaying history has
// not necessarily applied the exporting chain's block yet: C and D bootstrap
// independently, so at C's block N the D->C object may legitimately be absent (not
// yet exported) or already gone (already consumed at C's own accept of N). The
// shared-memory layer states this constraint explicitly on RemoveValue — "chains
// interacting with shared memory must be able to generate their chain state without
// actually performing the read of shared memory ... It is up to the chain to ensure
// this based on the current engine state." Blocks below the bootstrap frontier were
// already validated by the network under ⅔-stake finality; that acceptance is the
// authority for them, exactly as it is for predicates (see Block.verify).
//
// KNOWN, INHERENT LIVENESS PROPERTY (shared with every shared-memory import model,
// including coreth's ImportTx): a validator that is caught up on C but LAGGING on D
// will not yet see D's exported object and will reject an otherwise-valid C block.
// It does not halt — the block is rejected, not fatal, and re-verification succeeds
// once D catches up. The operational rule that keeps this off the critical path is
// that the settlement transaction is only issuable AFTER the object is observable in
// shared memory, which means D's exporting block is already accepted network-wide.

// installDexAtomicImportVerifier wires the block-level authentication onto the chain.
// Called once at initializeChain, after the runtime (and therefore shared memory) is
// bound. It is unconditional: the hook itself is fail-open ONLY where no cross-chain
// capability exists at all, in which case no settlement can execute either.
func (vm *VM) installDexAtomicImportVerifier() {
	vm.blockChain.SetAtomicImportVerifier(vm.verifyDexAtomicImports)
}

// verifyDexAtomicImports authenticates every cross-chain consumption the block
// declared. Each declaration names (sourceChainID, outputID) and commits to the
// object's bytes; we read the object shared memory actually holds and require the
// commitment to match.
func (vm *VM) verifyDexAtomicImports(blk *types.Block, logs []*types.Log) error {
	imports := dex.DecodeSettleImports(logs)
	if len(imports) == 0 {
		return nil
	}

	// Engine-state gate. Below the bootstrap frontier the network's acceptance is the
	// authority and the peer chain's export may not be locally applied yet.
	if !vm.bootstrapped.Get() {
		return nil
	}

	if vm.runtime == nil {
		return nil
	}
	sm := vm.runtime.GetSharedMemory()
	if sm == nil {
		// No cross-chain capability wired: a settlement could not have executed
		// (the precompile refuses without it), so there is nothing to authenticate.
		return nil
	}

	for _, imp := range imports {
		vals, err := sm.Get(imp.SourceChainID, [][]byte{imp.OutputID[:]})
		if err != nil || len(vals) != 1 || len(vals[0]) == 0 {
			return fmt.Errorf(
				"dex atomic import unbacked in block %s (height %d): object %s from chain %s is not in shared memory: %w",
				blk.Hash(), blk.NumberU64(), imp.OutputID, imp.SourceChainID, err,
			)
		}
		if got := dex.SettleObjectHash(vals[0]); got != imp.ObjectHash {
			return fmt.Errorf(
				"dex atomic import forged in block %s (height %d): object %s from chain %s declared %s but shared memory holds %s",
				blk.Hash(), blk.NumberU64(), imp.OutputID, imp.SourceChainID, imp.ObjectHash, got,
			)
		}
	}
	return nil
}

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"math/big"
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/ids"
	"github.com/luxfi/runtime"
	"github.com/stretchr/testify/require"
)

// precompile_reprocess_divergence_test.go is a RED adversarial probe (review-only;
// no production code touched). It pins the consensus-determinism hazard introduced
// by sourcing the 0x9999 value-path identity from bc.consensusCtx, which is bound
// (plugin/evm/vm.go SetConsensusContext) AFTER core.NewBlockChain runs inside
// eth.New. NewBlockChain's loadLastState -> reprocessState -> reprocessBlock ->
// processor.Process re-executes accepted-but-uncommitted blocks (unclean shutdown)
// and populateMissingTries re-executes a configured range (archive backfill) — both
// at construction, with bc.consensusCtx still nil. The identity a precompile reads
// during that re-execution therefore differs from the identity the same block saw
// when it was built/accepted (runtime bound). This test demonstrates the divergence
// at the exact seam (NewEVMBlockContext) and characterizes it as fail-closed.
//
// Whether this is fail-closed-identical (every node reverts -> same state root ->
// safe, just a stuck node) or admit-vs-reject (FORK) hinges on one fact: at
// re-execution time bc.consensusCtx is nil on EVERY node (it is never bound before
// NewBlockChain returns), so the re-execution verdict is (networkID 0 / Empty) on
// every node uniformly. That makes it a NODE-LOCAL liveness failure (the recomputed
// receipts/state-root will not match the committed block -> ValidateState error ->
// the node cannot start), not a chain split. This test proves the (0, Empty)
// re-execution identity is what the construction-time path yields.

type probeIdentity struct {
	networkID uint32
	cChainID  ids.ID
}

func probeHeader() *types.Header {
	return &types.Header{
		Number:     big.NewInt(1),
		Time:       1,
		Difficulty: big.NewInt(0),
		GasLimit:   8_000_000,
		BaseFee:    big.NewInt(0),
	}
}

// identityFromBlockContext drives the SAME production accessibleStateAdapter the EVM
// uses, over an EVM whose block context is the given one, and reads the identity the
// 0x9999 path cross-checks. This reuses newEVMWithConsensusCtx + adapterFor from
// precompile_consensus_identity_test.go so the probe reads the real seam.
func identityFromBlockContext(t *testing.T, bc vm.BlockContext) probeIdentity {
	t.Helper()
	evm := newEVMWithConsensusCtx(t, bc.ConsensusContext)
	a := adapterFor(evm)
	return probeIdentity{networkID: a.NetworkID(), cChainID: a.CChainID()}
}

// TestReprocessWindowIdentityDivergence shows that the SAME header, threaded through
// NewEVMBlockContext, yields the real chain identity when bc.consensusCtx is bound
// (the miner build path and the normal post-Initialize verify path) but the
// fail-closed (networkID 0 / C-Chain Empty) identity when bc.consensusCtx is nil
// (the reprocessState/populateMissingTries construction-time path). Any 0x9999 swap
// that SUCCEEDED at build time therefore REVERTS when re-executed in that window.
func TestReprocessWindowIdentityDivergence(t *testing.T) {
	header := probeHeader()

	// (A) Build/verify path: chain has its runtime-bearing consensus context bound
	// (vm.go SetConsensusContext ran). This is what the miner used to PRODUCE the
	// block and what a normal Process() verify (post-Initialize) uses.
	bound := runtime.WithContext(context.Background(), testRuntime())
	builtCtx := NewEVMBlockContext(header, &stubChainContext{consensusCtx: bound}, nil)
	builtID := identityFromBlockContext(t, builtCtx)

	// (B) Reprocess-at-construction path: NewBlockChain (inside eth.New) re-executes
	// this block BEFORE SetConsensusContext, so the chain's consensus context is the
	// zero value (nil). NewEVMBlockContext leaves ConsensusContext nil.
	reprocessCtx := NewEVMBlockContext(header, &stubChainContext{consensusCtx: nil}, nil)
	reprocessID := identityFromBlockContext(t, reprocessCtx)

	// The build path sees the real identity.
	require.Equal(t, testNetworkID, builtID.networkID,
		"build/verify path (runtime bound) must see the real networkID")
	require.Equal(t, testCChainID, builtID.cChainID,
		"build/verify path (runtime bound) must see the real C-Chain id")

	// The reprocess path sees the fail-closed zero identity — the EXACT (0, Empty)
	// the original real-chain bug observed, now reachable on unclean-shutdown
	// recovery / archive backfill because the binding happens after NewBlockChain.
	require.Equal(t, uint32(0), reprocessID.networkID,
		"reprocess-at-construction path (runtime NOT yet bound) sees networkID 0")
	require.Equal(t, ids.Empty, reprocessID.cChainID,
		"reprocess-at-construction path (runtime NOT yet bound) sees C-Chain Empty")

	// The divergence: the same block computes a DIFFERENT precompile identity at
	// build time vs reprocess time. For an identity-gated 0x9999 swap this is the
	// difference between SUCCESS (committed) and REVERT (recomputed) -> the
	// recomputed state root cannot match the committed one.
	require.NotEqual(t, builtID.networkID, reprocessID.networkID,
		"DETERMINISM HAZARD: build-time identity must not differ from reprocess-time "+
			"identity for the same block; it does because bc.consensusCtx binds after "+
			"NewBlockChain's reprocess re-execution")
}

// TestReprocessWindowIsUniformAcrossNodes pins WHY this is fail-closed liveness and
// not a fork: at reprocess time the consensus context is nil REGARDLESS of the
// node's configured runtime, because NewBlockChain has no access to the runtime
// (eth.New's signature carries no Runtime; SetConsensusContext is a later call). So
// two validators with DIFFERENT real runtimes still both reprocess with (0, Empty)
// — uniformly fail-closed. The recomputed block is rejected on every node the same
// way (a stuck node), never admitted on one and rejected on another (no split).
func TestReprocessWindowIsUniformAcrossNodes(t *testing.T) {
	header := probeHeader()

	// Two nodes reprocessing at construction: both see nil consensus context.
	nodeAReprocess := NewEVMBlockContext(header, &stubChainContext{consensusCtx: nil}, nil)
	nodeBReprocess := NewEVMBlockContext(header, &stubChainContext{consensusCtx: nil}, nil)

	idA := identityFromBlockContext(t, nodeAReprocess)
	idB := identityFromBlockContext(t, nodeBReprocess)

	require.Equal(t, idA.networkID, idB.networkID,
		"reprocess identity must be uniform (0) across nodes — the property that "+
			"keeps the hazard fail-closed-liveness rather than a consensus split")
	require.Equal(t, uint32(0), idA.networkID)
}

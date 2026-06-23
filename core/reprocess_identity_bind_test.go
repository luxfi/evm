// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/plugin/evm/upgrade/legacy"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/runtime"
	"github.com/stretchr/testify/require"
)

// TestNewBlockChain_BindsChainRuntimeBeforeReprocess is the regression guard for the
// unclean-restart node-brick: core.NewBlockChain re-executes accepted blocks DURING
// construction (loadLastState recovery / populateMissingTries backfill), so the chain's
// consensus identity must be bound BEFORE that runs. Previously it was bound only afterward
// (vm.go SetConsensusContext, post eth.New), so an unclean restart after a 0x9999 swap
// re-executed that block with networkID 0 / C-Chain Empty, reverted the committed swap,
// failed ValidateState, and the node could not boot.
//
// The fix threads the *runtime.Runtime the VM already holds into NewBlockChain. We can't
// observe the in-construction reprocess directly, but binding-before-reprocess is equivalent
// to "the identity is readable on the chain the instant construction returns" — if it were
// bound afterward (the bug), it would be nil here. We assert exactly that, plus the nil
// default for chains with no identity-gated precompiles.
func TestNewBlockChain_BindsChainRuntimeBeforeReprocess(t *testing.T) {
	gspec := &Genesis{
		BaseFee: big.NewInt(legacy.BaseFee),
		Config:  params.TestChainConfig,
	}

	t.Run("runtime bound at construction", func(t *testing.T) {
		bc, err := NewBlockChain(
			rawdb.NewMemoryDatabase(), DefaultCacheConfig, gspec, dummy.NewCoinbaseFaker(),
			vm.Config{}, common.Hash{}, false, testRuntime(),
		)
		require.NoError(t, err)
		defer bc.Stop()

		// The consensus context is bound the moment construction returns — i.e. it was
		// present for any reprocess that ran inside NewBlockChain (the bug bound it later).
		ctx := bc.ConsensusContext()
		require.NotNil(t, ctx, "consensus context must be bound at construction, not after")
		rt := runtime.FromContext(ctx)
		require.NotNil(t, rt, "chain Runtime must be recoverable from the bound context")
		require.Equal(t, testNetworkID, rt.NetworkID, "networkID must match the chain's identity")
		require.Equal(t, testCChainID, rt.CChainID, "C-Chain id must match the chain's identity")
	})

	t.Run("nil runtime stays nil (no identity-gated precompiles)", func(t *testing.T) {
		bc, err := NewBlockChain(
			rawdb.NewMemoryDatabase(), DefaultCacheConfig, gspec, dummy.NewCoinbaseFaker(),
			vm.Config{}, common.Hash{}, false, nil,
		)
		require.NoError(t, err)
		defer bc.Stop()
		require.Nil(t, bc.ConsensusContext(), "a chain with no runtime keeps a nil consensus context")
	})
}

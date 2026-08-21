// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/vm/components/chain"
)

// TestAcceptedBlockVerifyAndAcceptAreIdempotent is the catch-up regression:
// consensus may adopt a different certified proposer wrapper around the exact
// EVM block already committed locally. The parent state can already be pruned,
// so neither Verify nor Accept may re-execute or replay side effects for that
// inner hash.
func TestAcceptedBlockVerifyAndAcceptAreIdempotent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	vm := &VM{}
	accepted := vm.newBlock(types.NewBlockWithHeader(&types.Header{
		Number: big.NewInt(5615),
	}))
	vm.State = chain.NewState(&chain.Config{
		DecidedCacheSize:    1 << 20,
		MissingCacheSize:    1,
		UnverifiedCacheSize: 1 << 20,
		BytesToIDCacheSize:  1 << 20,
		LastAcceptedBlock:   accepted,
	})

	// blockChain/versiondb are deliberately nil. Any attempt to re-execute or
	// replay Accept side effects would panic instead of passing this regression.
	require.True(accepted.alreadyAccepted())
	require.NoError(accepted.Verify(ctx))
	require.NoError(accepted.Accept(ctx))
}

func TestDifferentBlockIsNotAlreadyAccepted(t *testing.T) {
	require := require.New(t)

	vm := &VM{}
	accepted := vm.newBlock(types.NewBlockWithHeader(&types.Header{Number: big.NewInt(7)}))
	other := vm.newBlock(types.NewBlockWithHeader(&types.Header{Number: big.NewInt(8)}))
	vm.State = chain.NewState(&chain.Config{
		DecidedCacheSize:    1 << 20,
		MissingCacheSize:    1,
		UnverifiedCacheSize: 1 << 20,
		BytesToIDCacheSize:  1 << 20,
		LastAcceptedBlock:   accepted,
	})

	require.False(other.alreadyAccepted())
}

// TestHistoricalCanonicalBlockVerifyAndAcceptAreIdempotent is the restart
// recovery regression. The inner EVM may already be thousands of blocks ahead
// of a damaged proposer-wrapper index. Replaying the quorum-certified wrappers
// must advance only that outer index; an exact historical block on the EVM's own
// accepted canonical chain must not be executed or accepted a second time.
func TestHistoricalCanonicalBlockVerifyAndAcceptAreIdempotent(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()

	chain, _ := buildAcceptedTestChain(t, 8)
	tip := chain.GetBlockByNumber(8)
	vm := &VM{blockChain: chain}
	vm.State = chainState(vm.newBlock(tip))

	historical := vm.newBlock(chain.GetBlockByNumber(5))
	require.True(historical.alreadyAccepted())
	require.NoError(historical.Verify(ctx))
	require.NoError(historical.Accept(ctx))
	require.Equal(tip.Hash(), chain.LastConsensusAcceptedBlock().Hash(),
		"historical replay must not move the EVM accepted tip")

	// Same height is not enough. A fork hash at an accepted height is not the
	// canonical execution result and must take the normal verification path.
	fork := vm.newBlock(types.NewBlockWithHeader(&types.Header{
		Number:     new(big.Int).Set(historical.ethBlock.Number()),
		ParentHash: historical.ethBlock.ParentHash(),
	}))
	require.NotEqual(historical.ID(), fork.ID())
	require.False(fork.alreadyAccepted())

	// A canonical preferred block above the accepted floor is still processing,
	// never already accepted. This height bound keeps the shortcut fail-closed.
	above := vm.newBlock(types.NewBlockWithHeader(&types.Header{
		Number:     new(big.Int).Add(tip.Number(), big.NewInt(1)),
		ParentHash: tip.Hash(),
	}))
	require.False(above.alreadyAccepted())
}

func chainState(lastAccepted *Block) *chain.State {
	return chain.NewState(&chain.Config{
		DecidedCacheSize:    1 << 20,
		MissingCacheSize:    1,
		UnverifiedCacheSize: 1 << 20,
		BytesToIDCacheSize:  1 << 20,
		LastAcceptedBlock:   lastAccepted,
	})
}

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

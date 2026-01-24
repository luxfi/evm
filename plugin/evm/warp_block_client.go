// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"

	nodeblock "github.com/luxfi/vm/chain"
	"github.com/luxfi/database"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
)

// warpBlockClient wraps VM to provide the warp.BlockClient interface
type warpBlockClient struct {
	vm *VM
}

// GetAcceptedBlock returns an accepted block.
func (w *warpBlockClient) GetAcceptedBlock(ctx context.Context, blkID ids.ID) (nodeblock.Block, error) {
	// First verify the block is accepted
	ethBlock := w.vm.blockChain.GetBlockByHash(common.BytesToHash(blkID[:]))
	if ethBlock == nil {
		return nil, database.ErrNotFound
	}

	// Check if this block is accepted by comparing with canonical chain
	acceptedHash := w.vm.blockChain.GetCanonicalHash(ethBlock.NumberU64())
	if acceptedHash != ethBlock.Hash() {
		return nil, database.ErrNotFound
	}

	return w.vm.newBlock(ethBlock), nil
}

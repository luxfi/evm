// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"

	"github.com/luxfi/database"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
	nodeblock "github.com/luxfi/vm/chain"
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

// LastQuasarHeight returns the accept-tip-CLAMPED EXPORT-FINAL (Quasar, ⅔-by-stake) height — the
// export-tier gate for warp block signatures (a cross-chain export must sit at export finality, not
// the reorgable local Accept tip). It reads the SAME shared clamp (eth.ClampedQuasarHeight) the RPC
// `finalized`/`safe` resolver uses, so the warp gate is structurally belt-protected — it can never
// admit a block above the served accept tip even during the transient rewind window (before
// reconcileQuasarWithAccept clears a stale key), independent of reconcile ordering.
func (w *warpBlockClient) LastQuasarHeight() uint64 {
	if w.vm.eth == nil {
		return 0
	}
	return w.vm.eth.ClampedQuasarHeight()
}

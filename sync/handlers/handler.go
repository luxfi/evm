// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package handlers

import (
	"github.com/luxfi/evm/v2/core/state/snapshot"
	"github.com/luxfi/evm/v2/core/types"
	"github.com/luxfi/geth/common"
)

type BlockProvider interface {
	GetBlock(common.Hash, uint64) *types.Block
}

type SnapshotProvider interface {
	Snapshots() *snapshot.Tree
}

type SyncDataProvider interface {
	BlockProvider
	SnapshotProvider
}
// (c) 2021-2022, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package handlers

import (
	"github.com/luxdefi/subnet-evm/core/state/snapshot"
	"github.com/luxdefi/subnet-evm/core/types"
	"github.com/ethereum/go-ethereum/common"
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

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"time"

	nodeblock "github.com/luxfi/consensus/engine/chain/block"
	consensusblock "github.com/luxfi/consensus/protocol/chain"
	"github.com/luxfi/ids"
)

// BlockAdapter adapts consensus Block to node Block interface
type BlockAdapter struct {
	consensus consensusblock.Block
}

// NewBlockAdapter creates a new block adapter
func NewBlockAdapter(consensusBlock consensusblock.Block) nodeblock.Block {
	return &BlockAdapter{consensus: consensusBlock}
}

// ID returns the block ID
func (b *BlockAdapter) ID() ids.ID {
	return b.consensus.ID()
}

// Parent returns the parent block ID (alias for ParentID)
func (b *BlockAdapter) Parent() ids.ID {
	return b.consensus.ParentID()
}

// ParentID returns the parent block ID
func (b *BlockAdapter) ParentID() ids.ID {
	return b.consensus.ParentID()
}

// Height returns the block height
func (b *BlockAdapter) Height() uint64 {
	return b.consensus.Height()
}

// Timestamp returns the block timestamp
func (b *BlockAdapter) Timestamp() time.Time {
	return b.consensus.Timestamp()
}

// Status returns the block status
func (b *BlockAdapter) Status() uint8 {
	return uint8(b.consensus.Status())
}

// Verify verifies the block
func (b *BlockAdapter) Verify(ctx context.Context) error {
	return b.consensus.Verify(ctx)
}

// Accept accepts the block
func (b *BlockAdapter) Accept(ctx context.Context) error {
	return b.consensus.Accept(ctx)
}

// Reject rejects the block
func (b *BlockAdapter) Reject(ctx context.Context) error {
	return b.consensus.Reject(ctx)
}

// Bytes returns the block bytes
func (b *BlockAdapter) Bytes() []byte {
	return b.consensus.Bytes()
}

// Unwrap returns the underlying consensus block.
// This is useful for tests that need access to the internal *Block type.
func (b *BlockAdapter) Unwrap() consensusblock.Block {
	return b.consensus
}

// ShouldVerifyWithContext implements the block.WithVerifyContext interface
// by delegating to the underlying block if it supports the interface
func (b *BlockAdapter) ShouldVerifyWithContext(ctx context.Context) (bool, error) {
	// Check if the underlying block implements WithVerifyContext
	if verifiable, ok := b.consensus.(nodeblock.WithVerifyContext); ok {
		return verifiable.ShouldVerifyWithContext(ctx)
	}
	// Default to false if not supported
	return false, nil
}

// VerifyWithContext implements the block.WithVerifyContext interface
// by delegating to the underlying block if it supports the interface
func (b *BlockAdapter) VerifyWithContext(ctx context.Context, blockCtx *nodeblock.Context) error {
	// Check if the underlying block implements WithVerifyContext
	if verifiable, ok := b.consensus.(nodeblock.WithVerifyContext); ok {
		return verifiable.VerifyWithContext(ctx, blockCtx)
	}
	// Fall back to regular verification
	return b.consensus.Verify(ctx)
}

// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package atomic

import (
	"context"
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/evm/v2/v2/plugin/evm/message"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/crypto"
)

// Summary provides the information necessary to sync a node starting
// at the given block.
type Summary struct {
	BlockNumber uint64      `serialize:"true"`
	BlockHash   common.Hash `serialize:"true"`
	BlockRoot   common.Hash `serialize:"true"`
	AtomicRoot  common.Hash `serialize:"true"`

	summaryID  ids.ID
	bytes      []byte
	acceptImpl func(*Summary) error
}

func NewSummary(blockHash common.Hash, blockNumber uint64, blockRoot common.Hash, atomicRoot common.Hash) (*Summary, error) {
	// We intentionally do not use the acceptImpl here and leave it for the parser to set.
	summary := Summary{
		BlockNumber: blockNumber,
		BlockHash:   blockHash,
		BlockRoot:   blockRoot,
		AtomicRoot:  atomicRoot,
	}
	bytes, err := message.Codec.Marshal(message.Version, &summary)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal syncable summary: %w", err)
	}

	summary.bytes = bytes
	summaryID, err := ids.ToID(crypto.Keccak256(bytes))
	if err != nil {
		return nil, fmt.Errorf("failed to compute summary ID: %w", err)
	}
	summary.summaryID = summaryID

	return &summary, nil
}

func (a *Summary) Bytes() []byte {
	return a.bytes
}

func (a *Summary) ID() ids.ID {
	return a.summaryID
}

func (a *Summary) String() string {
	return fmt.Sprintf("Summary(BlockHash=%s, BlockNumber=%d, BlockRoot=%s, AtomicRoot=%s)", a.BlockHash, a.BlockNumber, a.BlockRoot, a.AtomicRoot)
}

func (a *Summary) Accept(context.Context) error {
	if a.acceptImpl == nil {
		return fmt.Errorf("accept implementation not specified for summary: %s", a)
	}
	return a.acceptImpl(a)
}
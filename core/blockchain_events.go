// Copyright 2025 Lux Industries, Inc.
// This file contains event types for the blockchain.

package core

import (
	"github.com/luxfi/geth/core/types"
)

// ChainSideEvent is posted when a side chain is detected.
type ChainSideEvent struct {
	Block *types.Block
}

// BadBlockReason represents the reason a block was marked as bad.
type BadBlockReason struct {
	ChainConfig interface{} `json:"chainConfig"`
	Receipts    types.Receipts `json:"receipts"`
	Number      uint64      `json:"number"`
	Hash        string      `json:"hash"`
	Error       string      `json:"error"`
}
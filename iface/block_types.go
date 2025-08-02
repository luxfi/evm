// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package iface

import (
	"math/big"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
)

// Header represents a block header in the blockchain.
// We use a type alias to avoid import cycles while maintaining compatibility.
type Header = types.Header

// ETHBlock represents an Ethereum block in the blockchain.
// We use a type alias to avoid import cycles while maintaining compatibility.
type ETHBlock = types.Block

// Transaction represents a transaction.
// We use a type alias to avoid import cycles while maintaining compatibility.
type Transaction = types.Transaction

// Receipt represents a transaction receipt.
// We use a type alias to avoid import cycles while maintaining compatibility.
type Receipt = types.Receipt

// Log represents a contract log event.
// We use a type alias to avoid import cycles while maintaining compatibility.
type Log = types.Log

// AccessList represents EIP-2930 access list.
// We use a type alias to avoid import cycles while maintaining compatibility.
type AccessList = types.AccessList

// Bloom represents a 2048 bit bloom filter.
type Bloom = types.Bloom

// TxData is the interface for transaction data.
type TxData = types.TxData

// DerivableList is the interface for deriving the hash of a list.
type DerivableList = types.DerivableList

// NewBlockWithHeader creates a new block with the given header.
func NewBlockWithHeader(header *Header) *ETHBlock {
	return types.NewBlockWithHeader(header)
}

// NewBlock creates a new block.
func NewBlock(header *Header, body *Body, uncles []*Header, receipts []*Receipt, hasher types.TrieHasher) *ETHBlock {
	return types.NewBlock(header, body, receipts, hasher)
}

// Body represents the body of a block.
type Body = types.Body

// ETHSigner encapsulates transaction signature handling.
type ETHSigner interface {
	// Sender returns the sender address of the transaction.
	Sender(tx *Transaction) (common.Address, error)
	// SignatureValues returns the raw R, S, V values corresponding to the given signature.
	SignatureValues(tx *Transaction, sig []byte) (r, s, v *big.Int, err error)
	// ChainID returns the chain ID.
	ChainID() *big.Int
	// Hash returns the hash to be signed.
	Hash(tx *Transaction) common.Hash
	// Equal returns true if the given signer is the same as the receiver.
	Equal(ETHSigner) bool
}
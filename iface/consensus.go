// Package interfaces provides common interfaces to break import cycles
package interfaces

import (
	"math/big"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/evm/core/types"
	"github.com/luxfi/evm/iface"
)

// ChainHeaderReader is an alias to iface.ChainHeaderReader
type ChainHeaderReader = iface.ChainHeaderReader

// ChainReader is an alias to iface.ChainReader
type ChainReader = iface.ChainReader

// Engine is an algorithm agnostic consensus engine.
type Engine interface {
	// Author retrieves the Ethereum address of the account that minted the given block.
	Author(header *types.Header) (common.Address, error)

	// VerifyHeader checks whether a header conforms to the consensus rules of a given engine.
	VerifyHeader(chain ChainHeaderReader, header *types.Header, seal bool) error

	// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers concurrently.
	VerifyHeaders(chain ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error)

	// VerifyUncles verifies that the given block's uncles conform to the consensus rules.
	VerifyUncles(chain ChainReader, block *types.Block) error

	// Prepare initializes the consensus fields of a block header according to the rules.
	Prepare(chain ChainHeaderReader, header *types.Header) error

	// Finalize runs any post-transaction state modifications and assembles the final block.
	Finalize(chain ChainHeaderReader, header *types.Header, state StateDB, txs []*types.Transaction,
		uncles []*types.Header) (*types.Block, error)

	// FinalizeAndAssemble runs any post-transaction state modifications and assembles the final block.
	FinalizeAndAssemble(chain ChainHeaderReader, header *types.Header, state StateDB, txs []*types.Transaction,
		uncles []*types.Header, receipts []*types.Receipt) (*types.Block, error)

	// Seal generates a new sealing request for the given input block and pushes it to the sealer.
	Seal(chain ChainHeaderReader, block *types.Block, results chan<- *types.Block, stop <-chan struct{}) error

	// SealHash returns the hash of a block prior to it being sealed.
	SealHash(header *types.Header) common.Hash

	// CalcDifficulty is the difficulty adjustment algorithm.
	CalcDifficulty(chain ChainHeaderReader, time uint64, parent *types.Header) *big.Int

	// Close terminates any background threads maintained by the consensus engine.
	Close() error
}

// ChainConfig is an alias to iface.ChainConfig for interface compatibility
type ChainConfig = iface.ChainConfig

// FeeConfig represents the fee configuration
type FeeConfig interface {
	// Basic getters for fee configuration
	GetGasLimit() *big.Int
	GetTargetBlockRate() uint64
	GetMinBaseFee() *big.Int
	GetTargetGas() *big.Int
	GetBaseFeeChangeDenominator() *big.Int
	GetMinBlockGasCost() *big.Int
	GetMaxBlockGasCost() *big.Int
	GetBlockGasCostStep() *big.Int
}
// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"

	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/luxfi/evm/v2/v2/iface"
	"github.com/luxfi/geth/common"
)

// ChainHeaderReaderAdapter adapts a BlockChain to the ChainHeaderReader interface
// by converting between EVM types and iface types
type ChainHeaderReaderAdapter struct {
	chain *BlockChain
}

// NewChainHeaderReaderAdapter creates a new adapter
func NewChainHeaderReaderAdapter(chain *BlockChain) iface.ChainHeaderReader {
	return &ChainHeaderReaderAdapter{chain: chain}
}

// Config retrieves the blockchain's chain configuration
func (a *ChainHeaderReaderAdapter) Config() iface.ChainConfig {
	return a.chain.Config()
}

// CurrentHeader retrieves the current header from the local chain
func (a *ChainHeaderReaderAdapter) CurrentHeader() *iface.Header {
	evmHeader := a.chain.CurrentHeader()
	return types.ConvertHeaderFromEVM(evmHeader)
}

// GetHeader retrieves a block header from the database by hash and number
func (a *ChainHeaderReaderAdapter) GetHeader(hash common.Hash, number uint64) *iface.Header {
	evmHeader := a.chain.GetHeader(hash, number)
	return types.ConvertHeaderFromEVM(evmHeader)
}

// GetHeaderByNumber retrieves a block header from the database by number
func (a *ChainHeaderReaderAdapter) GetHeaderByNumber(number uint64) *iface.Header {
	evmHeader := a.chain.GetHeaderByNumber(number)
	return types.ConvertHeaderFromEVM(evmHeader)
}

// GetHeaderByHash retrieves a block header from the database by its hash
func (a *ChainHeaderReaderAdapter) GetHeaderByHash(hash common.Hash) *iface.Header {
	evmHeader := a.chain.GetHeaderByHash(hash)
	return types.ConvertHeaderFromEVM(evmHeader)
}

// GetTd retrieves the total difficulty from the database by hash and number
func (a *ChainHeaderReaderAdapter) GetTd(hash common.Hash, number uint64) *big.Int {
	return a.chain.GetTd(hash, number)
}

// GetCoinbaseAt returns the configured coinbase address at the given timestamp
func (a *ChainHeaderReaderAdapter) GetCoinbaseAt(timestamp uint64) common.Address {
	return a.chain.GetCoinbaseAt(timestamp)
}

// GetFeeConfigAt returns the fee configuration at the given timestamp
func (a *ChainHeaderReaderAdapter) GetFeeConfigAt(timestamp uint64) (iface.FeeConfig, error) {
	return a.chain.GetFeeConfigAt(timestamp)
}

// ChainReaderAdapter adapts a BlockChain to the ChainReader interface
type ChainReaderAdapter struct {
	ChainHeaderReaderAdapter
}

// NewChainReaderAdapter creates a new adapter
func NewChainReaderAdapter(chain *BlockChain) iface.ChainReader {
	return &ChainReaderAdapter{
		ChainHeaderReaderAdapter: ChainHeaderReaderAdapter{chain: chain},
	}
}

// GetBlock retrieves a block from the database by hash and number
func (a *ChainReaderAdapter) GetBlock(hash common.Hash, number uint64) *iface.ETHBlock {
	evmBlock := a.chain.GetBlock(hash, number)
	return types.ConvertBlockFromEVM(evmBlock)
}
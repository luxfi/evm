// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

import (
	"math/big"
	
	"github.com/luxfi/geth/common"
)

// Implement interfaces.ChainConfig interface

func (c *ChainConfig) GetEIP150Block() *big.Int {
	return c.ChainConfig.EIP150Block
}

func (c *ChainConfig) GetEIP150Hash() common.Hash {
	return common.Hash{} // We don't use this old-fork constant
}

func (c *ChainConfig) GetEIP155Block() *big.Int {
	return c.ChainConfig.EIP155Block
}

func (c *ChainConfig) GetEIP158Block() *big.Int {
	return c.ChainConfig.EIP158Block
}

func (c *ChainConfig) GetByzantiumBlock() *big.Int {
	return c.ChainConfig.ByzantiumBlock
}

func (c *ChainConfig) GetConstantinopleBlock() *big.Int {
	return c.ChainConfig.ConstantinopleBlock
}

func (c *ChainConfig) GetPetersburgBlock() *big.Int {
	return c.ChainConfig.PetersburgBlock
}

func (c *ChainConfig) GetIstanbulBlock() *big.Int {
	return c.ChainConfig.IstanbulBlock
}

func (c *ChainConfig) GetMuirGlacierBlock() *big.Int {
	return c.ChainConfig.MuirGlacierBlock
}

func (c *ChainConfig) GetBerlinBlock() *big.Int {
	return c.ChainConfig.BerlinBlock
}

func (c *ChainConfig) GetLondonBlock() *big.Int {
	return c.ChainConfig.LondonBlock
}
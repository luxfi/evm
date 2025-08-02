// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ifaceimpl

import (
	"math/big"

	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/params"
)

// ChainConfigAdapter wraps params.ChainConfig to implement iface.ChainConfig
type ChainConfigAdapter struct {
	*params.ChainConfig
}

// NewChainConfigAdapter creates a new adapter
func NewChainConfigAdapter(config *params.ChainConfig) iface.ChainConfig {
	return &ChainConfigAdapter{ChainConfig: config}
}

// GetChainID returns the chain ID
func (c *ChainConfigAdapter) GetChainID() *big.Int {
	return c.ChainID
}

// IsHomestead returns whether num is either equal to the homestead block or greater.
func (c *ChainConfigAdapter) IsHomestead(num *big.Int) bool {
	return c.ChainConfig.IsHomestead(num)
}

// IsEIP150 returns whether num is either equal to the EIP150 fork block or greater.
func (c *ChainConfigAdapter) IsEIP150(num *big.Int) bool {
	return c.ChainConfig.IsEIP150(num)
}

// IsEIP155 returns whether num is either equal to the EIP155 fork block or greater.
func (c *ChainConfigAdapter) IsEIP155(num *big.Int) bool {
	return c.ChainConfig.IsEIP155(num)
}

// IsEIP158 returns whether num is either equal to the EIP158 fork block or greater.
func (c *ChainConfigAdapter) IsEIP158(num *big.Int) bool {
	return c.ChainConfig.IsEIP158(num)
}

// IsByzantium returns whether num is either equal to the Byzantium fork block or greater.
func (c *ChainConfigAdapter) IsByzantium(num *big.Int) bool {
	return c.ChainConfig.IsByzantium(num)
}

// IsConstantinople returns whether num is either equal to the Constantinople fork block or greater.
func (c *ChainConfigAdapter) IsConstantinople(num *big.Int) bool {
	return c.ChainConfig.IsConstantinople(num)
}

// IsPetersburg returns whether num is either equal to the Petersburg fork block or greater.
func (c *ChainConfigAdapter) IsPetersburg(num *big.Int) bool {
	return c.ChainConfig.IsPetersburg(num)
}

// IsIstanbul returns whether num is either equal to the Istanbul fork block or greater.
func (c *ChainConfigAdapter) IsIstanbul(num *big.Int) bool {
	return c.ChainConfig.IsIstanbul(num)
}

// IsBerlin returns whether num is either equal to the Berlin fork block or greater.
func (c *ChainConfigAdapter) IsBerlin(num *big.Int) bool {
	return c.ChainConfig.IsBerlin(num)
}

// IsLondon returns whether num is either equal to the London fork block or greater.
func (c *ChainConfigAdapter) IsLondon(num *big.Int) bool {
	return c.ChainConfig.IsLondon(num)
}

// IsShanghai returns whether time is either equal to the Shanghai fork time or greater.
func (c *ChainConfigAdapter) IsShanghai(num *big.Int, time uint64) bool {
	return c.ChainConfig.IsShanghai(num, time)
}

// IsCancun returns whether time is either equal to the Cancun fork time or greater.
func (c *ChainConfigAdapter) IsCancun(time uint64) bool {
	return c.ChainConfig.IsCancun(time)
}

// IsGenesis returns whether all network upgrades are active at genesis.
func (c *ChainConfigAdapter) IsGenesis(time uint64) bool {
	return c.ChainConfig.IsGenesis(time)
}

// AllowedFeeRecipients returns whether fee recipients are allowed
func (c *ChainConfigAdapter) AllowedFeeRecipients() bool {
	extra := params.GetExtra(c.ChainConfig)
	if extra == nil {
		return false
	}
	return extra.AllowFeeRecipients
}

// GenesisRules returns the Genesis modified rules to support Genesis network upgrades
func (c *ChainConfigAdapter) GenesisRules(blockNum *big.Int, timestamp uint64) iface.GenesisRules {
	// Directly return the iface.GenesisRules from the underlying ChainConfig
	return c.ChainConfig.GenesisRules(blockNum, timestamp)
}

// AsGeth returns the underlying geth ChainConfig for compatibility
func (c *ChainConfigAdapter) AsGeth() interface{} {
	return c.ChainConfig.ChainConfig
}
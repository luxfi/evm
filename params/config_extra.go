// (c) 2025 Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

import (
	"math/big"
	"sync"

	"github.com/luxfi/evm/v2/params/extras"
	"github.com/luxfi/evm/v2/utils"
	ethparams "github.com/luxfi/geth/params"
	"github.com/luxfi/node/upgrade"
)

const (
	// TODO: Value to pass to geth's Rules by default where the appropriate
	// context is not available in the lux code. (similar to context.TODO())
	IsMergeTODO = true
)

var (
	DefaultChainID = big.NewInt(43214)
	
	// Simple payloads system to store extra data per ChainConfig
	payloads = &chainConfigPayloads{
		extras: make(map[*ChainConfig]*extras.ChainConfig),
	}
)

type chainConfigPayloads struct {
	mu     sync.RWMutex
	extras map[*ChainConfig]*extras.ChainConfig
}

func (p *chainConfigPayloads) Get(c *ChainConfig) *extras.ChainConfig {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.extras[c]
}

func (p *chainConfigPayloads) Set(c *ChainConfig, extra *extras.ChainConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.extras[c] = extra
}

// SetEthUpgrades enables Ethereum network upgrades using the same time as
// the Lux network upgrade that enables them.
// For v2.0.0, all upgrades are active at genesis (timestamp 0).
func SetEthUpgrades(c *ChainConfig, luxUpgrades extras.NetworkUpgrades) {
	if c.BerlinBlock == nil {
		c.BerlinBlock = big.NewInt(0)
	}
	if c.LondonBlock == nil {
		c.LondonBlock = big.NewInt(0)
	}
	// For v2.0.0, all upgrades are active at genesis
	if luxUpgrades.GenesisTimestamp != nil {
		c.ShanghaiTime = utils.NewUint64(*luxUpgrades.GenesisTimestamp)
		c.CancunTime = utils.NewUint64(*luxUpgrades.GenesisTimestamp)
	}
}

func GetExtra(c *ChainConfig) *extras.ChainConfig {
	ex := payloads.Get(c)
	if ex == nil {
		ex = &extras.ChainConfig{}
		payloads.Set(c, ex)
	}
	return ex
}

func Copy(c *ChainConfig) ChainConfig {
	cpy := *c
	extraCpy := *GetExtra(c)
	return *WithExtra(&cpy, &extraCpy)
}

// WithExtra sets the extra payload on `c` and returns the modified argument.
func WithExtra(c *ChainConfig, extra *extras.ChainConfig) *ChainConfig {
	payloads.Set(c, extra)
	return c
}

func SetNetworkUpgradeDefaults(c *ChainConfig) {
	if c.HomesteadBlock == nil {
		c.HomesteadBlock = big.NewInt(0)
	}
	if c.EIP150Block == nil {
		c.EIP150Block = big.NewInt(0)
	}
	if c.EIP155Block == nil {
		c.EIP155Block = big.NewInt(0)
	}
	if c.EIP158Block == nil {
		c.EIP158Block = big.NewInt(0)
	}
	if c.ByzantiumBlock == nil {
		c.ByzantiumBlock = big.NewInt(0)
	}
	if c.ConstantinopleBlock == nil {
		c.ConstantinopleBlock = big.NewInt(0)
	}
	if c.PetersburgBlock == nil {
		c.PetersburgBlock = big.NewInt(0)
	}
	if c.IstanbulBlock == nil {
		c.IstanbulBlock = big.NewInt(0)
	}
	if c.MuirGlacierBlock == nil {
		c.MuirGlacierBlock = big.NewInt(0)
	}

	if upgrades, ok := GetExtra(c).ConsensusCtx.NetworkUpgrades.(*upgrade.Config); ok {
		GetExtra(c).NetworkUpgrades.SetDefaults(*upgrades)
	}
}

// GetRulesExtra returns the Lux-specific rules for the given Ethereum rules.
func GetRulesExtra(rules Rules) *extras.Rules {
	// Create a ChainConfig from the Rules to get the extra data
	chainID := rules.ChainID
	if chainID == nil {
		chainID = DefaultChainID
	}
	
	// Create a minimal ChainConfig to get extras
	// This is a workaround since Rules doesn't have direct access to ChainConfig
	tempConfig := &ChainConfig{
		ChainConfig: &ethparams.ChainConfig{
			ChainID: chainID,
		},
	}
	extra := GetExtra(tempConfig)
	
	// Create rules based on the Lux upgrades
	return &extras.Rules{
		GenesisRules: extra.GetRules(0), // Using 0 as we don't have timestamp in Rules
		Precompiles:    rules.ActivePrecompiles,
		Predicaters:    rules.Predicaters,
		AccepterPrecompiles: rules.AccepterPrecompiles,
	}
}

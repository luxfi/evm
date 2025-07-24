// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

import (
	"math/big"

	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
)

// LuxChainConfig is the simplified chain configuration for Lux mainnet
// All legacy upgrades are considered activated from genesis
type LuxChainConfig struct {
	ChainID *big.Int `json:"chainID"`

	// FeeConfig is the dynamic fee configuration
	FeeConfig commontype.FeeConfig `json:"feeConfig"`

	// AllowFeeRecipients allows block builders to claim fees
	AllowFeeRecipients bool `json:"allowFeeRecipients,omitempty"`

	// Precompile upgrade configuration
	UpgradeConfig *extras.UpgradeConfig `json:"upgrades,omitempty"`
}

// SimplifiedRules contains the rules for the Lux chain
// All legacy upgrades are always true
type SimplifiedRules struct {
	ChainID *big.Int

	// All upgrades are active by default in Lux
	IsHomestead, IsEIP150, IsEIP155, IsEIP158               bool
	IsByzantium, IsConstantinople, IsPetersburg, IsIstanbul bool
	IsBerlin, IsLondon                                      bool
	IsShanghai, IsCancun                                    bool

	// ActivePrecompiles maps addresses to stateful precompiled contracts
	ActivePrecompiles map[common.Address]precompileconfig.Config
	// Predicaters maps addresses to stateful precompile Predicaters
	Predicaters map[common.Address]precompileconfig.Predicater
	// AccepterPrecompiles maps addresses to stateful precompile accepter functions
	AccepterPrecompiles map[common.Address]precompileconfig.Accepter
}

// GetRules returns the simplified rules - all upgrades active
func (c *LuxChainConfig) GetRules() SimplifiedRules {
	return SimplifiedRules{
		ChainID:          c.ChainID,
		IsHomestead:      true,
		IsEIP150:         true,
		IsEIP155:         true,
		IsEIP158:         true,
		IsByzantium:      true,
		IsConstantinople: true,
		IsPetersburg:     true,
		IsIstanbul:       true,
		IsBerlin:         true,
		IsLondon:         true,
		IsShanghai:       true,
		IsCancun:         true,
		// Precompiles will be populated by the caller
		ActivePrecompiles:   make(map[common.Address]precompileconfig.Config),
		Predicaters:         make(map[common.Address]precompileconfig.Predicater),
		AccepterPrecompiles: make(map[common.Address]precompileconfig.Accepter),
	}
}

// IsPrecompileEnabled returns true if the precompile at [addr] is enabled
func (r *SimplifiedRules) IsPrecompileEnabled(addr common.Address) bool {
	_, ok := r.ActivePrecompiles[addr]
	return ok
}

// GetFeeConfig returns the fee configuration
func (c *LuxChainConfig) GetFeeConfig() commontype.FeeConfig {
	return c.FeeConfig
}

// AllowedFeeRecipients returns whether fee recipients are allowed
func (c *LuxChainConfig) AllowedFeeRecipients() bool {
	return c.AllowFeeRecipients
}

var (
	// LuxMainnetChainConfig is the chain config for Lux mainnet v1
	// 
	// Historical Note: Lux Network mainnet launched in 2025 with all Avalanche 
	// upgrades (Apricot, Banff, Cortina, Durango, Etna) pre-activated. This was
	// done to simplify the codebase for developers who don't need to be familiarized
	// with 5 years of network upgrades from the Avalanche side.
	//
	// Our previous subnet EVM chain lacked C-Chain precompiles, so we upgraded our
	// core implementation and removed all Avalanche-specific upgrade flags to 
	// accommodate our own upgrade path moving forward.
	LuxMainnetChainConfig = &LuxChainConfig{
		ChainID: big.NewInt(96369), // Lux mainnet chain ID
		FeeConfig: commontype.FeeConfig{
			// Dynamic fee config (Octane/ACP-176 style)
			GasLimit:                 big.NewInt(100_000_000), // 100M gas limit
			TargetGas:                big.NewInt(50_000_000),  // 50M gas target (50% of limit)
			BaseFeeChangeDenominator: big.NewInt(36),
			MinBaseFee:               big.NewInt(25_000_000_000), // 25 gwei
			TargetBlockRate:          2,                          // 2 second blocks
			MinBlockGasCost:          big.NewInt(0),
			MaxBlockGasCost:          big.NewInt(1_000_000),
			BlockGasCostStep:         big.NewInt(50_000),
		},
		AllowFeeRecipients: true,
	}
)
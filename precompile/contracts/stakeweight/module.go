// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	"fmt"

	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/evm/precompile/modules"
	"github.com/luxfi/evm/precompile/precompileconfig"

	"github.com/luxfi/geth/common"
)

var _ contract.Configurator = (*configurator)(nil)

// ConfigKey is the upgrade.json key that activates this precompile.
const ConfigKey = "stakeWeightConfig"

// ContractAddress is the next free slot in the legacy coreth-compatible
// precompile range, immediately after warp at ...0005.
var ContractAddress = common.HexToAddress("0x0200000000000000000000000000000000000006")

// Module registers the precompile.
var Module = modules.Module{
	ConfigKey:    ConfigKey,
	Address:      ContractAddress,
	Contract:     StakeWeightPrecompile,
	Configurator: &configurator{},
}

type configurator struct{}

func init() {
	if err := modules.RegisterModule(Module); err != nil {
		panic(err)
	}
}

// MakeConfig returns an empty config for Marshal/Unmarshal.
func (*configurator) MakeConfig() precompileconfig.Config { return new(Config) }

// MakeGenesisConfig returns a config activated at genesis.
func (*configurator) MakeGenesisConfig() precompileconfig.Config {
	var zero uint64
	return NewConfig(&zero)
}

// Configure stores nothing: the precompile holds no state and no privileged
// address, so there is nothing to seed and nobody to seed it for.
func (*configurator) Configure(_ precompileconfig.ChainConfig, cfg precompileconfig.Config, _ contract.StateDB, _ contract.ConfigurationBlockContext) error {
	if _, ok := cfg.(*Config); !ok {
		return fmt.Errorf("expected config type %T, got %T: %v", &Config{}, cfg, cfg)
	}
	return nil
}

// (c) 2019-2020, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package txallowlist

import (
	"fmt"

	"github.com/luxdefi/subnet-evm/precompile/contract"
	"github.com/luxdefi/subnet-evm/precompile/modules"
	"github.com/luxdefi/subnet-evm/precompile/precompileconfig"
	"github.com/ethereum/go-ethereum/common"
)

var _ contract.Configurator = &configurator{}

// ConfigKey is the key used in json config files to specify this precompile config.
// must be unique across all precompiles.
const ConfigKey = "txAllowListConfig"

var ContractAddress = common.HexToAddress("0x0200000000000000000000000000000000000002")

var Module = modules.Module{
	ConfigKey:    ConfigKey,
	Address:      ContractAddress,
	Contract:     TxAllowListPrecompile,
	Configurator: &configurator{},
}

type configurator struct{}

func init() {
	if err := modules.RegisterModule(Module); err != nil {
		panic(err)
	}
}

func (*configurator) MakeConfig() precompileconfig.Config {
	return new(Config)
}

// Configure configures [state] with the initial state for the precompile.
func (*configurator) Configure(chainConfig precompileconfig.ChainConfig, cfg precompileconfig.Config, state contract.StateDB, blockContext contract.ConfigurationBlockContext) error {
	config, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("expected config type %T, got %T: %v", &Config{}, cfg, cfg)
	}
	return config.AllowListConfig.Configure(chainConfig, ContractAddress, state, blockContext)
}

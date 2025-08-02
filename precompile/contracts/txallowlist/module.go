// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package txallowlist

import (
	"fmt"
	"github.com/luxfi/evm/v2/precompile/contract"
	"github.com/luxfi/evm/v2/precompile/registry"
	"github.com/luxfi/evm/v2/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
)

var _ contract.Configurator = &configurator{}

// ConfigKey is the key used in json config files to specify this precompile config.
// must be unique across all precompiles.
const ConfigKey = "txAllowListConfig"

var ContractAddress = common.HexToAddress("0x0200000000000000000000000000000000000002")

var Module = registry.NewModule(
	ConfigKey,
	ContractAddress,
	TxAllowListPrecompile,
	&configurator{},
)

type configurator struct{}

func init() {
	if err := registry.RegisterModule(Module); err != nil {
		panic(err)
	}
}

// MakeConfig returns a new precompile config instance.
// This is required to Marshal/Unmarshal the precompile config.
func (*configurator) MakeConfig() precompileconfig.Config {
	return new(Config)
}

// Configure configures [state] with the given [cfg] precompileconfig.
// This function is called by the EVM once per precompile contract activation.
func (*configurator) Configure(chainConfig precompileconfig.ChainConfig, cfg precompileconfig.Config, state contract.StateDB, blockContext contract.ConfigurationBlockContext) error {
	config, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("expected config type %T, got %T: %v", &Config{}, cfg, cfg)
	}
	return config.AllowListConfig.Configure(chainConfig, ContractAddress, state, blockContext)
}

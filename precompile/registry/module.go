// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"bytes"
	"github.com/luxfi/evm/v2/v2/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
)

// Module wraps a precompile contract with metadata
type Module struct {
	// configKey is the key used in json config files to specify this precompile config.
	configKey string
	// address is the address where the stateful precompile is accessible.
	address common.Address
	// contract is a thread-safe singleton that can be used as the StatefulPrecompiledContract when
	// this config is enabled.
	contract interface{} // Will be contract.StatefulPrecompiledContract
	// configurator is used to configure the stateful precompile when the config is enabled.
	configurator interface{} // Will be contract.Configurator
}

// NewModule creates a new Module instance
func NewModule(configKey string, address common.Address, contract interface{}, configurator interface{}) Module {
	return Module{
		configKey:    configKey,
		address:      address,
		contract:     contract,
		configurator: configurator,
	}
}

// GetAddress returns the module's address (for backward compatibility)
func (m Module) GetAddress() common.Address {
	return m.address
}

// GetContract returns the module's contract (for backward compatibility)
func (m Module) GetContract() interface{} {
	return m.contract
}

// GetConfigurator returns the module's configurator (for backward compatibility)
func (m Module) GetConfigurator() interface{} {
	return m.configurator
}

// GetConfigKey returns the module's config key (for backward compatibility)
func (m Module) GetConfigKey() string {
	return m.configKey
}

// ConfigKey returns the module's config key (interface method)
func (m Module) ConfigKey() string {
	return m.configKey
}

// Interface methods to satisfy precompile.PrecompileModule
func (m Module) Address() common.Address {
	return m.address
}

func (m Module) Contract() interface{} {
	return m.contract
}

func (m Module) Configurator() interface{} {
	return m.configurator
}

func (m Module) DefaultConfig() interface{} {
	// This would need to be implemented based on the configurator
	return nil
}

func (m Module) MakeConfig() interface{} {
	// Use the configurator to make the config
	if m.configurator != nil {
		// Try the precompileconfig.Config signature first
		if conf, ok := m.configurator.(interface{ MakeConfig() precompileconfig.Config }); ok {
			return conf.MakeConfig()
		}
		// Fall back to generic interface{} signature
		if conf, ok := m.configurator.(interface{ MakeConfig() interface{} }); ok {
			return conf.MakeConfig()
		}
	}
	return nil
}

type moduleArray []Module

func (u moduleArray) Len() int {
	return len(u)
}

func (u moduleArray) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

func (m moduleArray) Less(i, j int) bool {
	return bytes.Compare(m[i].address.Bytes(), m[j].address.Bytes()) < 0
}
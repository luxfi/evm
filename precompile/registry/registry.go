// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Module to facilitate the registration of precompiles and their configuration.
package registry

import (
	"fmt"
	"sort"
	
	"github.com/luxfi/evm/constants"
	"github.com/luxfi/evm/precompile"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/geth/common"
)

var (
	// registeredModules is a list of Module to preserve order
	// for deterministic iteration
	registeredModules = make([]Module, 0)

	reservedRanges = []utils.AddressRange{
		{
			Start: common.HexToAddress("0x0100000000000000000000000000000000000000"),
			End:   common.HexToAddress("0x01000000000000000000000000000000000000ff"),
		},
		{
			Start: common.HexToAddress("0x0200000000000000000000000000000000000000"),
			End:   common.HexToAddress("0x02000000000000000000000000000000000000ff"),
		},
		{
			Start: common.HexToAddress("0x0300000000000000000000000000000000000000"),
			End:   common.HexToAddress("0x03000000000000000000000000000000000000ff"),
		},
	}
)

// globalRegistry is the singleton registry instance
type precompileRegistry struct{}

// Ensure precompileRegistry implements precompile.PrecompileRegistry
var _ precompile.PrecompileRegistry = (*precompileRegistry)(nil)

// GetPrecompileModule returns a precompile module by key
func (r *precompileRegistry) GetPrecompileModule(key string) (precompile.PrecompileModule, bool) {
	for _, stm := range registeredModules {
		if stm.configKey == key {
			return stm, true
		}
	}
	return nil, false
}

// GetPrecompileModuleByAddress returns a precompile module by address
func (r *precompileRegistry) GetPrecompileModuleByAddress(address common.Address) (precompile.PrecompileModule, bool) {
	for _, stm := range registeredModules {
		if stm.address == address {
			return stm, true
		}
	}
	return nil, false
}

// RegisteredModules returns all registered modules
func (r *precompileRegistry) RegisteredModules() []precompile.PrecompileModule {
	result := make([]precompile.PrecompileModule, len(registeredModules))
	for i, m := range registeredModules {
		result[i] = m
	}
	return result
}

// GetRegistry returns the global registry instance
func GetRegistry() precompile.PrecompileRegistry {
	return &precompileRegistry{}
}

// ReservedAddress returns true if [addr] is in a reserved range for custom precompiles
func ReservedAddress(addr common.Address) bool {
	for _, reservedRange := range reservedRanges {
		if reservedRange.Contains(addr) {
			return true
		}
	}

	return false
}

// RegisterModule registers a stateful precompile module
func RegisterModule(stm Module) error {
	address := stm.address
	key := stm.configKey

	if address == constants.BlackholeAddr {
		return fmt.Errorf("address %s overlaps with blackhole address", address)
	}
	if !ReservedAddress(address) {
		return fmt.Errorf("address %s not in a reserved range", address)
	}

	for _, registeredModule := range registeredModules {
		if registeredModule.configKey == key {
			return fmt.Errorf("name %s already used by a stateful precompile", key)
		}
		if registeredModule.address == address {
			return fmt.Errorf("address %s already used by a stateful precompile", address)
		}
	}
	// sort by address to ensure deterministic iteration
	registeredModules = insertSortedByAddress(registeredModules, stm)
	return nil
}

// Legacy functions for compatibility
func GetPrecompileModuleByAddress(address common.Address) (Module, bool) {
	for _, stm := range registeredModules {
		if stm.address == address {
			return stm, true
		}
	}
	return Module{}, false
}

func GetPrecompileModule(key string) (Module, bool) {
	for _, stm := range registeredModules {
		if stm.configKey == key {
			return stm, true
		}
	}
	return Module{}, false
}

func RegisteredModules() []Module {
	return registeredModules
}

func insertSortedByAddress(data []Module, stm Module) []Module {
	data = append(data, stm)
	sort.Sort(moduleArray(data))
	return data
}

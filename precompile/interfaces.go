// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package precompile

import (
	"github.com/luxfi/geth/common"
)

// PrecompileRegistry manages precompile modules
type PrecompileRegistry interface {
	// GetPrecompileModule returns a precompile module by key
	GetPrecompileModule(key string) (PrecompileModule, bool)
	
	// GetPrecompileModuleByAddress returns a precompile module by address
	GetPrecompileModuleByAddress(address common.Address) (PrecompileModule, bool)
	
	// RegisteredModules returns all registered modules
	RegisteredModules() []PrecompileModule
}

// PrecompileModule represents a precompile module
type PrecompileModule interface {
	// Address returns the address of the precompile
	Address() common.Address
	
	// Contract returns the precompile contract
	Contract() interface{}
	
	// Configurator returns the configurator for this precompile
	Configurator() interface{}
	
	// DefaultConfig returns the default config for this precompile
	DefaultConfig() interface{}
	
	// MakeConfig creates a new config instance
	MakeConfig() interface{}
	
	// ConfigKey returns the configuration key for this module
	ConfigKey() string
}
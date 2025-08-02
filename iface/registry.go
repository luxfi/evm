// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package iface

import (
	"github.com/luxfi/evm/v2/precompile"
	"github.com/luxfi/geth/common"
)

// Global registry instance that will be set by the application
var (
	precompileRegistry precompile.PrecompileRegistry
)

// SetPrecompileRegistry sets the global precompile registry
// This should be called during application initialization
func SetPrecompileRegistry(registry precompile.PrecompileRegistry) {
	precompileRegistry = registry
}

// GetPrecompileRegistry returns the global precompile registry
func GetPrecompileRegistry() precompile.PrecompileRegistry {
	return precompileRegistry
}

// GetPrecompileModule is a convenience function that uses the global registry
func GetPrecompileModule(key string) (precompile.PrecompileModule, bool) {
	if precompileRegistry == nil {
		return nil, false
	}
	return precompileRegistry.GetPrecompileModule(key)
}

// GetPrecompileModuleByAddress is a convenience function that uses the global registry
func GetPrecompileModuleByAddress(address common.Address) (precompile.PrecompileModule, bool) {
	if precompileRegistry == nil {
		return nil, false
	}
	return precompileRegistry.GetPrecompileModuleByAddress(address)
}
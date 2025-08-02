// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package modules provides backward compatibility aliases for the registry package.
// This package is deprecated - use github.com/luxfi/evm/precompile/registry instead.
package modules

import (
	"github.com/luxfi/evm/precompile/registry"
	"github.com/luxfi/geth/common"
)

// Type aliases for backward compatibility
type Module = registry.Module

// Deprecated: Use registry.ReservedAddress
func ReservedAddress(addr common.Address) bool {
	return registry.ReservedAddress(addr)
}

// Deprecated: Use registry.RegisterModule
func RegisterModule(stm Module) error {
	return registry.RegisterModule(stm)
}

// Deprecated: Use registry.GetPrecompileModuleByAddress
func GetPrecompileModuleByAddress(address common.Address) (Module, bool) {
	return registry.GetPrecompileModuleByAddress(address)
}

// Deprecated: Use registry.GetPrecompileModule
func GetPrecompileModule(key string) (Module, bool) {
	return registry.GetPrecompileModule(key)
}

// Deprecated: Use registry.RegisteredModules
func RegisteredModules() []Module {
	return registry.RegisteredModules()
}
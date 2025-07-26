// (c) 2025 Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
)

type Rules struct {
	// All historic upgrades activated at Lux Genesis
	GenesisRules

	// Precompiles maps addresses to stateful precompiled contracts that are enabled
	// for this rule set.
	// Note: none of these addresses should conflict with the address space used by
	// any existing precompiles.
	Precompiles map[common.Address]precompileconfig.Config
	// Predicaters maps addresses to stateful precompile Predicaters
	// that are enabled for this rule set.
	Predicaters map[common.Address]precompileconfig.Predicater
	// AccepterPrecompiles map addresses to stateful precompile accepter functions
	// that are enabled for this rule set.
	AccepterPrecompiles map[common.Address]precompileconfig.Accepter
}

func (r *Rules) PredicatersExist() bool {
	return len(r.Predicaters) > 0
}

func (r *Rules) PredicaterExists(addr common.Address) bool {
	_, ok := r.Predicaters[addr]
	return ok
}

// IsPrecompileEnabled returns true if the precompile at `addr` is enabled for this rule set.
func (r *Rules) IsPrecompileEnabled(addr common.Address) bool {
	_, ok := r.Precompiles[addr]
	return ok
}

// IsDurango returns true (always enabled in v2.0.0)
func (r *Rules) IsDurango() bool {
	return true
}

// IsEVM returns true (always enabled in v2.0.0)
func (r *Rules) IsEVM() bool {
	return true
}

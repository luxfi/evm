// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package modules

import (
	"bytes"

	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/geth/common"
)

type Module struct {
	// ConfigKey is the key used in json config files to specify this precompile config.
	ConfigKey string
	// Address returns the address where the stateful precompile is accessible.
	Address common.Address
	// Contract returns a thread-safe singleton that can be used as the StatefulPrecompiledContract when
	// this config is enabled.
	Contract contract.StatefulPrecompiledContract
	// Configurator is used to configure the stateful precompile when the config is enabled.
	contract.Configurator
	// AlwaysOn marks a precompile that is ACTIVE on every chain from genesis with NO
	// config entry (neither a genesis-inlined precompile nor a precompileUpgrades
	// timestamp). It is the one-way activation for system precompiles that take no
	// per-network parameters and resolve everything from the runtime (consensus
	// context / atomic state). The DEX settlement precompile (0x9999) is the canonical
	// case. An always-on module's Configurator is never invoked (no activating config);
	// the host gives it the EXTCODESIZE marker at genesis and dispatches Run on every
	// block. Bridged from github.com/luxfi/precompile/modules.Module.AlwaysOn.
	AlwaysOn bool
}

type moduleArray []Module

func (u moduleArray) Len() int {
	return len(u)
}

func (u moduleArray) Swap(i, j int) {
	u[i], u[j] = u[j], u[i]
}

func (m moduleArray) Less(i, j int) bool {
	return bytes.Compare(m[i].Address.Bytes(), m[j].Address.Bytes()) < 0
}

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
	// AlwaysOn marks a precompile that is a SYSTEM precompile activated by the Lux
	// protocol on every chain with NO per-network config entry (neither a genesis-inlined
	// precompile nor a precompileUpgrades timestamp). It takes no per-network parameters
	// and resolves everything from the runtime (consensus context / atomic state). The DEX
	// settlement precompile (0x9999) is the canonical case. An always-on module's
	// Configurator is never invoked (no activating config); the host installs its
	// EXTCODESIZE marker and dispatches Run. Bridged from
	// github.com/luxfi/precompile/modules.Module.AlwaysOn.
	AlwaysOn bool
	// ActivationTime is the protocol timestamp at which an AlwaysOn system precompile
	// becomes present (EXTCODESIZE marker installed + Run dispatched) on every Lux chain.
	// It is NOT per-network config — it is a protocol constant, the analog of a fork
	// timestamp, applied uniformly. The zero value means "present from genesis" (the
	// historical always-on behavior). A non-zero value means the precompile is ABSENT
	// before that timestamp and activates at the block transition crossing it — exactly
	// like a dated fork — so a chain whose genesis predates ActivationTime does NOT carry
	// the precompile in its genesis state (preserving that chain's original genesis hash),
	// and the precompile activates later during history replay at the crossing block.
	// Ignored unless AlwaysOn is set. Set by the registry bridge, not the JSON config.
	ActivationTime uint64
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

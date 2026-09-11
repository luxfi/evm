// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Module to facilitate the registration of precompiles and their configuration.
package registry

// Force imports of each precompile to ensure each precompile's init function runs and registers itself
// with the registry.
import (
	// The chain-integrated precompiles. These hold chain state and configure the
	// chain itself, so they live here rather than in the standalone suite.
	_ "github.com/luxfi/evm/precompile/contracts/deployerallowlist"
	_ "github.com/luxfi/evm/precompile/contracts/feemanager"
	_ "github.com/luxfi/evm/precompile/contracts/nativeminter"
	_ "github.com/luxfi/evm/precompile/contracts/rewardmanager"
	_ "github.com/luxfi/evm/precompile/contracts/stakeweight"
	_ "github.com/luxfi/evm/precompile/contracts/txallowlist"
	_ "github.com/luxfi/evm/precompile/contracts/warp"

	// Every precompile in the Lux suite, by importing the suite's OWN registry
	// rather than listing its packages here.
	//
	// A precompile is only dispatchable if its init() has run, and luxd rejects
	// an upgrade.json activation for a configKey it has never seen with "unknown
	// precompile config". This file used to name 41 suite packages one by one to
	// make that happen, which made it a second record of what the suite contains
	// — and it had drifted: aivmbridge, swap and v3 were registered in the suite
	// and absent here, so no Lux chain could activate them however its genesis
	// was written.
	//
	// The suite's registry derives its set from the packages that actually
	// register, so importing it cannot drift from what the suite holds. One
	// record, in the repository that owns the thing being recorded.
	_ "github.com/luxfi/precompile/registry"
)

// LP-ALIGNED ADDRESSING (LP-9015):
// DEX precompiles use trailing LP number format: 0x0000...00LPNUM
// The LP number IS the address suffix - maximum simplicity.
//
// Address format: 0x0000000000000000000000000000000000LPNUM
//
// DEX Precompiles (LP-9xxx - QuantumSwap Native DEX):
//   POOL_MANAGER   = 0x0000...9010  // LP-9010 - Singleton pool manager
//   ORACLE_HUB     = 0x0000...9011  // LP-9011 - Multi-source price aggregation
//   SWAP_ROUTER    = 0x0000...9012  // LP-9012 - Swap routing
//   HOOKS_REGISTRY = 0x0000...9013  // LP-9013 - Hook contract registry
//   FLASH_LOAN     = 0x0000...9014  // LP-9014 - Flash loan facility
//   CLOB           = 0x0000...9020  // LP-9020 - Central limit order book
//   VAULT          = 0x0000...9030  // LP-9030 - DeFi vault operations
//   PRICE_FEED     = 0x0000...9040  // LP-9040 - Price feed aggregator
//
// Bridge Precompiles (LP-6xxx):
//   TELEPORT       = 0x0000...6010  // LP-6010 - Cross-chain teleportation
//
// Same addresses work across ALL Lux EVM chains (C-Chain, Zoo, Hanzo, SPC)
// See github.com/luxfi/precompile/registry for canonical Go addresses

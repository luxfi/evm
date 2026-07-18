// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

// Register EVERY stateful-precompile module the C-Chain's stored chain config can
// reference (warp + the full lux BLS/ZK/AI/bridge/DEX set), so ReadChainConfig
// deserializes the mainnet upgrade config instead of failing "unknown precompile
// config: warpConfig" → "genesis has no chain configuration".
//
// This is the SAME aggregator plugin/evm imports (plugin/evm/vm.go:75), so the
// offline rewind binds byte-identical precompile identity to what the live node
// used when it accepted these blocks — any identity-gated (0x9999) re-execution
// validates the same way. Blank import: registration happens in each module's
// init(); duplicate blank imports across files are idempotent in Go.
import _ "github.com/luxfi/evm/precompile/registry"

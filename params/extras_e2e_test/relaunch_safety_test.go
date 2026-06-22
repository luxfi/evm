// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Relaunch-safety gate for the canonical primary-network C-Chain
// upgrade.json files.
//
// Threat addressed (Red HIGH, pre-existing): a mainnet/testnet relaunch
// from the canonical genesis must NOT re-trigger an activation that was
// already applied at block 0 on the running chain. A precompile that is
// live at genesis is a GENESIS property; only strictly-future activations
// belong in upgrade.json's precompileUpgrades schedule. Putting an
// already-live precompile at a post-genesis timestamp reschedules an
// already-applied activation, which on relaunch either (a) makes the
// relaunched node treat the precompile as INACTIVE from block 0 —
// diverging from the canonical chain where it is active at block 0
// (a consensus fork) — or (b) trips checkPrecompileCompatible
// ("missing"/"cannot retroactively enable") and refuses boot.
//
// This test drives the node's REAL fork-application primitive
// (extras.IsForkTransition, the same predicate GetActivatingPrecompileConfigs
// uses) against the embedded canonical fixtures and asserts the invariant:
//
//	For the genesis -> first-block transition (parent = nil), the set of
//	precompiles that activate AT genesis (blockTimestamp <= genesisTime)
//	is EXACTLY the set of precompiles already live at block 0 on the
//	running chain, and every other scheduled activation is strictly in
//	the future (blockTimestamp > genesisTime). No entry sits in the
//	(0, genesisTime] window, which is the only window that could
//	retroactively re-fire an already-applied activation on relaunch.
package extras_e2e_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/evm/params/extras"
)

// cChainGenesisTime is the C-Chain (chainId 96369 mainnet / 96368 testnet)
// genesis BLOCK timestamp baked into the frozen canonical genesis. It is
// NOT the P-chain network startTime of a relaunch; the EVM genesis block
// keeps this timestamp across relaunches, so it is the boundary that
// IsForkTransition(fork, nil, genesisTime) uses to decide which precompiles
// are active at block 0.
//
// Source of truth:
//
//	luxfi/genesis configs/mainnet/cchain.json  -> "timestamp": "0x672485c2"
//	luxfi/genesis configs/testnet/cchain.json  -> "timestamp": "0x67259912"
const (
	mainnetCChainGenesisTime uint64 = 0x672485c2 // 1730446786 = 2024-11-01 07:39:46 UTC
	testnetCChainGenesisTime uint64 = 0x67259912 // 1730517266 = 2024-11-02 03:14:26 UTC
)

// liveAtBlockZero is the exact set of precompile config keys that the
// running mainnet AND testnet C-Chains have active at block 0. Source of
// truth: the UPGRADE_JSON heredoc in luxfi/universe
// k8s/lux-{mainnet,testnet}/luxd-startup.yaml (both carry these 17 at
// blockTimestamp:0). warpConfig is NOT in this set: it is declared in the
// genesis chainConfig itself (cchain.json config.warpConfig), not in
// upgrade.json's precompileUpgrades, so it is handled by the genesis path
// rather than the upgrade schedule.
var liveAtBlockZero = map[string]bool{
	"aiMiningConfig": true,
	"blake3Config":   true,
	"cggmp21Verify":  true,
	"deadZeroConfig": true,
	"deadConfig":     true,
	"deadFullConfig": true,
	"routerConfig":   true,
	"fheConfig":      true,
	"frostVerify":    true,
	"graphConfig":    true,
	"hpkeConfig":     true,
	"mldsaVerify":    true,
	"mlkemConfig":    true,
	"pqcryptoConfig": true,
	"ringConfig":     true,
	"slhdsaVerify":   true,
	"zkConfig":       true,
}

func TestMainnetUpgradeJSON_RelaunchSafe(t *testing.T) {
	assertRelaunchSafe(t, readCanonicalMainnetUpgradeJSONRaw(t), mainnetCChainGenesisTime)
}

func TestTestnetUpgradeJSON_RelaunchSafe(t *testing.T) {
	assertRelaunchSafe(t, readCanonicalTestnetUpgradeJSONRaw(t), testnetCChainGenesisTime)
}

// assertRelaunchSafe parses the canonical upgrade.json and proves the
// relaunch-safety invariant using the node's own IsForkTransition predicate.
func assertRelaunchSafe(t *testing.T, raw []byte, genesisTime uint64) {
	t.Helper()

	var cfg extras.UpgradeConfig
	require.NoError(t, json.Unmarshal(raw, &cfg),
		"canonical upgrade.json failed to parse — see TestMainnetUpgradeJSON_UnmarshalsAgainstRegistry")

	activeAtGenesis := map[string]bool{}

	for i, upg := range cfg.PrecompileUpgrades {
		key := upg.Key()
		ts := upg.Timestamp()
		require.NotNilf(t, ts, "precompileUpgrades[%d] key=%q has nil blockTimestamp", i, key)

		// The node decides "active at the genesis block" via
		// IsForkTransition(fork, parent=nil, current=genesisTime). With a
		// nil parent this reduces to (fork <= genesisTime). This is exactly
		// what GetActivatingPrecompileConfigs(addr, nil, genesisTime, ...)
		// evaluates when a relaunched node builds its first rule set.
		activatesAtGenesis := extras.IsForkTransition(ts, nil, genesisTime)

		if activatesAtGenesis {
			// An entry that activates at/under genesis MUST be one of the
			// already-live precompiles, AND it MUST be pinned to exactly 0.
			// Any value in (0, genesisTime] is a reschedule of an
			// already-applied activation into the retroactive window — the
			// precise fork/boot-refusal hazard Red flagged.
			require.Truef(t, liveAtBlockZero[key],
				"RELAUNCH FORK RISK: precompileUpgrades[%d] key=%q activates at genesis "+
					"(blockTimestamp=%d <= genesisTime=%d) but is NOT in the set of precompiles "+
					"live at block 0 on the running chain. Either remove it (if it is genuinely "+
					"future, give it blockTimestamp > %d) or, if it really is live at genesis, add "+
					"it to liveAtBlockZero and pin it to 0.",
				i, key, *ts, genesisTime, genesisTime)

			require.Equalf(t, uint64(0), *ts,
				"RELAUNCH FORK RISK: live-at-genesis precompile %q is scheduled at blockTimestamp=%d "+
					"instead of 0. A non-zero timestamp <= genesisTime reschedules an already-applied "+
					"activation; on relaunch the node would treat it as inactive before %d and diverge "+
					"from the canonical chain (or checkPrecompileCompatible refuses boot). Pin it to 0.",
				key, *ts, *ts)

			activeAtGenesis[key] = true
		} else {
			// Strictly-future activation: must be > genesisTime. (Guaranteed
			// by !activatesAtGenesis, but assert explicitly so the failure
			// message is unambiguous for an operator reading the log.)
			require.Greaterf(t, *ts, genesisTime,
				"precompileUpgrades[%d] key=%q is treated as future but blockTimestamp=%d <= genesisTime=%d",
				i, key, *ts, genesisTime)
		}
	}

	// Completeness: every precompile known to be live at block 0 on the
	// running chain MUST appear (pinned to 0) in the relaunch config.
	// A missing one would make the relaunched chain LOSE a block-0
	// precompile relative to the canonical chain — also a divergence.
	for key := range liveAtBlockZero {
		require.Truef(t, activeAtGenesis[key],
			"RELAUNCH DIVERGENCE: precompile %q is live at block 0 on the running chain but is "+
				"absent from (or not pinned to 0 in) the canonical relaunch upgrade.json. A relaunch "+
				"would drop it from genesis state.", key)
	}
}

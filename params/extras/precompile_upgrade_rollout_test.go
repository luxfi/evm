// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// External test package so the rollout regression can depend on the
// luxfi/evm/params/extras public API without dragging in
// luxfi/evm/precompile/registry (which would create an import cycle
// via the modules.Register side-effects).
//
// The tests parse raw JSON only — they intentionally avoid the typed
// PrecompileUpgrade UnmarshalJSON path so they do NOT need every
// precompile module's init() to have run. The contract these tests
// enforce is the textual JSON shape (key + blockTimestamp) which is
// what luxd's checkPrecompileCompatible compares; the per-key Config
// struct identity is verified inside the existing module-level tests.
package extras_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// luxMainnetLiveUpgradeJSON is the upgrade.json currently inlined in the
// lux-mainnet StatefulSet startup script
// (~/work/lux/universe/k8s/lux-mainnet/luxd-startup.yaml line 182). It is
// the source-of-truth list of precompile activations active on every C-
// Chain pod today. Every entry here MUST stay present (at the same
// blockTimestamp) in the canonical upgrade.json shipped by luxfi/genesis,
// otherwise luxd's checkPrecompileCompatible refuses to boot.
//
// If this constant changes, the lux-mainnet StatefulSet manifest changed
// out-of-band — sync it back to luxfi/genesis BEFORE shipping the new
// luxd image, never the other way around.
const luxMainnetLiveUpgradeJSON = `{
  "networkUpgradeOverrides": {
    "durangoTimestamp": 0,
    "quasarTimestamp": 0,
    "fortunaTimestamp": 0,
    "graniteTimestamp": 0
  },
  "precompileUpgrades": [
    {"aiMiningConfig":   {"blockTimestamp": 0}},
    {"blake3Config":     {"blockTimestamp": 0}},
    {"cggmp21Verify":    {"blockTimestamp": 0}},
    {"deadZeroConfig":   {"blockTimestamp": 0}},
    {"deadConfig":       {"blockTimestamp": 0}},
    {"deadFullConfig":   {"blockTimestamp": 0}},
    {"routerConfig":     {"blockTimestamp": 0}},
    {"fheConfig":        {"blockTimestamp": 0}},
    {"frostVerify":      {"blockTimestamp": 0}},
    {"graphConfig":      {"blockTimestamp": 0}},
    {"hpkeConfig":       {"blockTimestamp": 0}},
    {"mldsaVerify":      {"blockTimestamp": 0}},
    {"mlkemConfig":      {"blockTimestamp": 0}},
    {"pqcryptoConfig":   {"blockTimestamp": 0}},
    {"ringConfig":       {"blockTimestamp": 0}},
    {"slhdsaVerify":     {"blockTimestamp": 0}},
    {"zkConfig":         {"blockTimestamp": 0}}
  ]
}`

// TestMainnetUpgradeJSON_IsForwardCompatibleWithLiveActivations is the
// regression gate from red-review finding #7. It asserts that every
// precompile activation currently live on lux-mainnet is preserved at the
// same blockTimestamp in the canonical upgrade.json so that
// checkPrecompileCompatible returns nil at boot.
//
// The failure mode this prevents:
//
//	luxd boots, reads the new upgrade.json, and runs
//	checkPrecompileCompatible against the active configs (the live
//	activations that already shipped). For each live entry it walks the
//	new list looking for the same key at the same timestamp. If it's
//	missing -> "missing PrecompileUpgrade[i]". If it's at a different
//	timestamp -> "mismatching PrecompileUpgrade[i]". Either way, BOOT
//	FAILS. This is the exact wedge that bricked the cluster on the
//	original rollout attempt.
func TestMainnetUpgradeJSON_IsForwardCompatibleWithLiveActivations(t *testing.T) {
	canonical := readPrecompileUpgradeTimestamps(t, readCanonicalMainnetUpgradeJSONRaw(t))
	live := readPrecompileUpgradeTimestamps(t, []byte(luxMainnetLiveUpgradeJSON))

	for key, liveTs := range live {
		canonicalTs, ok := canonical[key]
		require.Truef(t, ok,
			"precompile %q is active on lux-mainnet (blockTimestamp=%d) but is missing from canonical upgrade.json — luxd will refuse to boot (checkPrecompileCompatible: missing PrecompileUpgrade)",
			key, liveTs,
		)
		require.Equalf(t, liveTs, canonicalTs,
			"precompile %q is active on lux-mainnet at blockTimestamp=%d but canonical upgrade.json schedules it at blockTimestamp=%d — luxd will refuse to boot (checkPrecompileCompatible: mismatching PrecompileUpgrade). RESCHEDULING AN ALREADY-LIVE PRECOMPILE IS NEVER VALID.",
			key, liveTs, canonicalTs,
		)
	}
}

// TestMainnetUpgradeJSON_PrecompileTimestampsAreMonotonic enforces the
// validation rule from extras.ChainConfig.verifyPrecompileUpgrades —
// precompile timestamps must be monotonically increasing across the
// list (the verify call refuses a config that decreases). If we get this
// wrong in the rollout config, every luxd boot rejects the JSON before
// it even runs checkPrecompileCompatible.
func TestMainnetUpgradeJSON_PrecompileTimestampsAreMonotonic(t *testing.T) {
	raw := readCanonicalMainnetUpgradeJSONRaw(t)
	var doc struct {
		PrecompileUpgrades []map[string]json.RawMessage `json:"precompileUpgrades"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	var prev uint64
	for i, entry := range doc.PrecompileUpgrades {
		require.Lenf(t, entry, 1, "precompileUpgrades[%d] must have exactly one key", i)
		for key, rawVal := range entry {
			var v struct {
				BlockTimestamp uint64 `json:"blockTimestamp"`
			}
			require.NoErrorf(t, json.Unmarshal(rawVal, &v), "precompileUpgrades[%d][%q] must have blockTimestamp", i, key)
			require.GreaterOrEqualf(t, v.BlockTimestamp, prev,
				"precompileUpgrades[%d][%q] blockTimestamp=%d is < previous %d — verifyPrecompileUpgrades refuses non-monotonic timestamps",
				i, key, v.BlockTimestamp, prev,
			)
			prev = v.BlockTimestamp
		}
	}
}

// TestMainnetUpgradeJSON_WarpRequiresPrimaryNetworkSigners enforces the
// Warp policy from red-review finding #8: the canonical upgrade.json must
// schedule warp so that, once the strict-PQ fork lands, every cross-chain
// warp message is signed by primary-network validators (not just a subnet
// quorum).
//
// Warp lives in the genesis chainConfig (cchain.json config.warpConfig at
// the genesis timestamp, with requirePrimaryNetworkSigners=false during the
// classical era). The upgrade schedule then carries a two-step toggle at the
// strict-PQ fork: disable@strictPQ-1, then re-enable@strictPQ with the
// PQ-hardened policy. Both toggles are strictly AFTER genesis time, so this
// is a genuine future upgrade — NOT a reschedule of the genesis warp (that
// would be the relaunch fork hazard; see relaunch_safety_test.go). The
// policy assertion therefore targets the RE-ENABLE entry (the one that
// carries quorumNumerator + requirePrimaryNetworkSigners); the disable entry
// carries only {blockTimestamp, disable:true}.
func TestMainnetUpgradeJSON_WarpRequiresPrimaryNetworkSigners(t *testing.T) {
	const strictPQ uint64 = 1766708400

	raw := readCanonicalMainnetUpgradeJSONRaw(t)

	var doc struct {
		PrecompileUpgrades []map[string]json.RawMessage `json:"precompileUpgrades"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	var (
		foundDisable bool
		foundReEnable bool
		prevWarpTs   *uint64
	)
	for i, entry := range doc.PrecompileUpgrades {
		warpRaw, ok := entry["warpConfig"]
		if !ok {
			continue
		}
		var warp struct {
			BlockTimestamp               uint64 `json:"blockTimestamp"`
			Disable                      bool   `json:"disable"`
			QuorumNumerator              uint64 `json:"quorumNumerator"`
			RequirePrimaryNetworkSigners bool   `json:"requirePrimaryNetworkSigners"`
		}
		require.NoError(t, json.Unmarshal(warpRaw, &warp))

		// Every warp upgrade entry must be strictly after genesis time —
		// rescheduling the genesis warp into the (0, genesisTime] window is
		// the relaunch fork hazard.
		require.Greaterf(t, warp.BlockTimestamp, uint64(0),
			"warpConfig upgrade entry at index %d has blockTimestamp 0 — warp is declared in the genesis chainConfig; an upgrade entry pinned to 0 would collide with the genesis warp and fail verifyPrecompileUpgrades",
			i,
		)
		// Same-key strict increase (mirrors verifyPrecompileUpgrades).
		if prevWarpTs != nil {
			require.Greaterf(t, warp.BlockTimestamp, *prevWarpTs,
				"warpConfig entries must strictly increase in blockTimestamp (got %d after %d)",
				warp.BlockTimestamp, *prevWarpTs,
			)
		}
		ts := warp.BlockTimestamp
		prevWarpTs = &ts

		if warp.Disable {
			foundDisable = true
			require.Equalf(t, strictPQ-1, warp.BlockTimestamp,
				"warpConfig disable must fire one second before the strict-PQ fork (%d), got %d",
				strictPQ-1, warp.BlockTimestamp,
			)
			continue
		}

		// Re-enable entry: carries the PQ-hardened policy.
		foundReEnable = true
		require.Equalf(t, strictPQ, warp.BlockTimestamp,
			"warpConfig re-enable must fire at the strict-PQ fork (%d), got %d", strictPQ, warp.BlockTimestamp,
		)
		require.Equal(t, uint64(67), warp.QuorumNumerator, "warpConfig quorumNumerator must be 67")
		require.Truef(t, warp.RequirePrimaryNetworkSigners,
			"warpConfig.requirePrimaryNetworkSigners must be true on lux-mainnet — every cross-chain warp message MUST be signed by primary-network validators (red-review finding #8)",
		)
	}
	require.True(t, foundDisable, "warpConfig disable entry must be present in canonical upgrade.json")
	require.True(t, foundReEnable, "warpConfig re-enable entry (with PQ signer policy) must be present in canonical upgrade.json")
}

// TestMainnetUpgradeJSON_HasStrictPQActivation enforces that the
// canonical upgrade.json activates the strict-PQ profile so that
// contract.RefuseUnderStrictPQ short-circuits every classical precompile
// at the activation timestamp. Without this, classical primitives
// (bls12-381 modules, sr25519/x25519, babyjubjub, pedersen, pasta,
// frost/cggmp21) keep executing alongside the PQ stack — directly
// contradicting the v1.0 "100% safe" floor of the Quasar Edition rollout.
//
// strictPQTimestamp is a NetworkUpgrades field on the EVM extras config;
// it activates when the chain's current block timestamp >= the value.
// When active, classical-pairing-and-discrete-log precompiles return
// contract.ErrClassicalForbiddenInPQ instead of running their Run()
// bodies (per ~/work/lux/precompile/contract/strict_pq.go).
func TestMainnetUpgradeJSON_HasStrictPQActivation(t *testing.T) {
	const quasarTS uint64 = 1766708400

	raw := readCanonicalMainnetUpgradeJSONRaw(t)
	var doc struct {
		NetworkUpgradeOverrides *struct {
			StrictPQTimestamp *uint64 `json:"strictPQTimestamp"`
		} `json:"networkUpgradeOverrides"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	require.NotNilf(t, doc.NetworkUpgradeOverrides,
		"canonical upgrade.json must include networkUpgradeOverrides with strictPQTimestamp",
	)
	require.NotNilf(t, doc.NetworkUpgradeOverrides.StrictPQTimestamp,
		"canonical upgrade.json must set networkUpgradeOverrides.strictPQTimestamp — without it RefuseUnderStrictPQ never fires and classical primitives keep executing alongside PQ",
	)
	require.Equalf(t, quasarTS, *doc.NetworkUpgradeOverrides.StrictPQTimestamp,
		"strictPQTimestamp must equal the Quasar activation timestamp %d (Dec 25 2025 16:20 PST) — task constraint",
		quasarTS,
	)
}

// TestMainnetChainConfig_HasStrictPQTrue enforces that the EVM-plugin
// chain config for lux-mainnet sets `pq: true`. That flag wires the
// strict-PQ posture into both the geth-layer std precompile registry
// (vm.chainConfig.PQ = gethvm.AllForbidden()) AND the Lux extras
// StrictPQTimestamp = &0 — together they make every classical primitive
// refuse from the moment the EVM plugin boots. The upgrade.json side
// activates RefuseUnderStrictPQ at the Quasar timestamp on RUNNING
// chains; this chain-config flag pins the same posture for nodes that
// rebuild state from scratch.
func TestMainnetChainConfig_HasStrictPQTrue(t *testing.T) {
	raw := readCanonicalMainnetChainConfigRaw(t)
	var doc struct {
		PQ bool `json:"pq"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))
	require.Truef(t, doc.PQ,
		"~/work/lux/state/chain-configs/lux-mainnet/config.json must set \"pq\": true — without it the EVM plugin boots without classical-precompile refusal and the chain runs in classical-permissive mode, contradicting the strict-PQ rollout",
	)
}

// readPrecompileUpgradeTimestamps decodes the precompileUpgrades array
// into a {key: blockTimestamp} map, skipping warpConfig (covered by its
// own test) and feeManagerConfig (not a precompile activation per se).
func readPrecompileUpgradeTimestamps(t *testing.T, raw []byte) map[string]uint64 {
	t.Helper()
	var doc struct {
		PrecompileUpgrades []map[string]json.RawMessage `json:"precompileUpgrades"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	out := make(map[string]uint64, len(doc.PrecompileUpgrades))
	for i, entry := range doc.PrecompileUpgrades {
		require.Lenf(t, entry, 1, "precompileUpgrades[%d] must have exactly one key", i)
		for key, rawVal := range entry {
			if key == "warpConfig" || key == "feeManagerConfig" {
				continue
			}
			var v struct {
				BlockTimestamp uint64 `json:"blockTimestamp"`
			}
			require.NoErrorf(t, json.Unmarshal(rawVal, &v), "precompileUpgrades[%d][%q] must have blockTimestamp", i, key)
			_, dup := out[key]
			require.Falsef(t, dup, "precompileUpgrades has duplicate key %q at index %d", key, i)
			out[key] = v.BlockTimestamp
		}
	}
	return out
}

// readCanonicalMainnetUpgradeJSONRaw returns the canonical upgrade.json
// bytes. The test resolves the file via the relative path from
// evm/params/extras up to genesis/configs/mainnet — both luxfi/evm and
// luxfi/genesis live in the same `~/work/lux` worktree on CI runners.
func readCanonicalMainnetUpgradeJSONRaw(t *testing.T) []byte {
	t.Helper()
	candidates := []string{
		// luxfi/evm running standalone, sibling luxfi/genesis checkout.
		"../../../../genesis/configs/mainnet/upgrade.json",
		// monorepo layout (e.g. when both repos are inside the same root).
		"../../../genesis/configs/mainnet/upgrade.json",
	}
	for _, candidate := range candidates {
		if data, err := os.ReadFile(candidate); err == nil {
			return data
		}
	}
	t.Skipf("canonical mainnet upgrade.json not reachable from cwd — looked in %v; run from a worktree that contains luxfi/genesis alongside luxfi/evm", candidates)
	return nil
}

// readCanonicalMainnetChainConfigRaw returns the canonical mainnet
// chain-config bytes (the EVM plugin config.json mounted at
// /data/configs/chains/<CID>/config.json on production luxd pods).
func readCanonicalMainnetChainConfigRaw(t *testing.T) []byte {
	t.Helper()
	candidates := []string{
		// luxfi/evm running standalone, sibling lux/state checkout.
		"../../../../state/chain-configs/lux-mainnet/config.json",
		// monorepo layout.
		"../../../state/chain-configs/lux-mainnet/config.json",
	}
	for _, candidate := range candidates {
		if data, err := os.ReadFile(candidate); err == nil {
			return data
		}
	}
	t.Skipf("canonical mainnet chain config not reachable from cwd — looked in %v; run from a worktree that contains lux/state alongside luxfi/evm", candidates)
	return nil
}

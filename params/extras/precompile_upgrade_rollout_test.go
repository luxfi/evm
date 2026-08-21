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

// luxMainnetLiveUpgradeJSON mirrors the precompile activations in the
// lux-mainnet StatefulSet startup script
// (~/work/lux/universe/k8s/lux-mainnet/luxd-startup.yaml). Every entry here
// MUST stay present at the SAME blockTimestamp in the canonical upgrade.json
// shipped by luxfi/genesis, or luxd's checkPrecompileCompatible refuses to boot.
//
// These are forward-dated to the strict-PQ fork (1766708400 = Dec 25 2025
// 16:20 PST), NOT blockTimestamp 0. Activating a precompile at 0 runs it
// inside core.ApplyPrecompileActivations during Genesis.toBlock, mutates the
// block-0 state root away from 0x2d1ceda…, and breaks RLP import (block-0 would
// no longer hash to 0x3f4fa2a0…). The regenesis preserves block-0 by keeping
// genesis clean (2-alloc) and forward-dating every precompile PAST the RLP tip
// — so no replayed block ever sees them and history reproduces exactly. The
// prior `blockTimestamp: 0` form of this fixture was stale (pre-forward-dating,
// 2026-06-27) and did not match the deployed manifest. See state/CLAUDE.md
// "RLP ↔ Genesis ↔ Upgrade" and genesis/configs/mainnet/upgrade.json.
//
// If this drifts from the deployed manifest, re-sync it — but NEVER by moving
// an activation onto blockTimestamp 0; that is the import-wedge bug itself.
const luxMainnetLiveUpgradeJSON = `{
  "networkUpgradeOverrides": {
    "durangoTimestamp": 0,
    "quasarTimestamp": 0,
    "fortunaTimestamp": 0,
    "graniteTimestamp": 0
  },
  "precompileUpgrades": [
    {"aiMiningConfig":   {"blockTimestamp": 1766708400}},
    {"blake3Config":     {"blockTimestamp": 1766708400}},
    {"cggmp21Verify":    {"blockTimestamp": 1766708400}},
    {"deadZeroConfig":   {"blockTimestamp": 1766708400}},
    {"deadConfig":       {"blockTimestamp": 1766708400}},
    {"deadFullConfig":   {"blockTimestamp": 1766708400}},
    {"routerConfig":     {"blockTimestamp": 1766708400}},
    {"fheConfig":        {"blockTimestamp": 1766708400}},
    {"frostVerify":      {"blockTimestamp": 1766708400}},
    {"graphConfig":      {"blockTimestamp": 1766708400}},
    {"hpkeConfig":       {"blockTimestamp": 1766708400}},
    {"mldsaVerify":      {"blockTimestamp": 1766708400}},
    {"mlkemConfig":      {"blockTimestamp": 1766708400}},
    {"ringConfig":       {"blockTimestamp": 1766708400}},
    {"slhdsaVerify":     {"blockTimestamp": 1766708400}},
    {"zkConfig":         {"blockTimestamp": 1766708400}}
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
// Warp lives in the genesis chainConfig with the classical-era policy. The
// upgrade schedule contains one replacement at the strict-PQ fork with the
// hardened signer policy. Keeping a single entry avoids two competing ways
// to describe the same activation.
func TestMainnetUpgradeJSON_WarpRequiresPrimaryNetworkSigners(t *testing.T) {
	const strictPQ uint64 = 1766708400

	raw := readCanonicalMainnetUpgradeJSONRaw(t)

	var doc struct {
		PrecompileUpgrades []map[string]json.RawMessage `json:"precompileUpgrades"`
	}
	require.NoError(t, json.Unmarshal(raw, &doc))

	var (
		foundWarp bool
		warpCount int
	)
	for i, entry := range doc.PrecompileUpgrades {
		warpRaw, ok := entry["warpConfig"]
		if !ok {
			continue
		}
		warpCount++
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
		foundWarp = true
		require.False(t, warp.Disable, "the canonical warp activation must not be disabled")
		require.Equalf(t, strictPQ, warp.BlockTimestamp,
			"warpConfig must activate at the strict-PQ fork (%d), got %d", strictPQ, warp.BlockTimestamp,
		)
		require.Equal(t, uint64(67), warp.QuorumNumerator, "warpConfig quorumNumerator must be 67")
		require.Truef(t, warp.RequirePrimaryNetworkSigners,
			"warpConfig.requirePrimaryNetworkSigners must be true on lux-mainnet — every cross-chain warp message MUST be signed by primary-network validators (red-review finding #8)",
		)
	}
	require.True(t, foundWarp, "warpConfig entry with the PQ signer policy must be present")
	require.Equal(t, 1, warpCount, "canonical upgrade.json must contain exactly one warpConfig entry")
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

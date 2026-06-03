// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package extras_e2e_test is a sibling-of-extras test package whose sole
// reason for existing is to drive extras.UpgradeConfig.UnmarshalJSON
// end-to-end against the canonical lux-mainnet upgrade.json after the
// precompile registry side-effects have run.
//
// The companion package extras_test (in params/extras) intentionally
// parses raw JSON only and skips the typed Unmarshal path, because
// pulling in luxfi/evm/precompile/registry there would force every
// existing test (including ones that mutate a fresh registry) to run
// against the same module map. Moving the e2e check into a sibling
// package lets us blank-import the registry exclusively here.
//
// The contract enforced by this test:
//
//	Every key in the canonical upgrade.json must round-trip through
//	extras.UpgradeConfig.UnmarshalJSON without an "unknown precompile
//	config" error AND yield a non-nil Timestamp() on the resulting
//	PrecompileUpgrade.Config. If any key is unregistered (e.g.
//	kzg4844Config, secp256r1Config, ed25519Config under pinned
//	luxfi/precompile v0.5.27), this test fails immediately.
//
// This is the regression gate from Red's MEDIUM (vector 9) finding —
// the JSON-only rollout tests in extras_test would have missed the
// vector-8 CRITICAL because they never exercise the per-key
// modules.GetPrecompileModule lookup.
package extras_e2e_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/evm/params/extras"
	// Side-effect import: every precompile module's init() registers
	// its ConfigKey + factory with modules.RegisteredModules. Without
	// this import the lookup in PrecompileUpgrade.UnmarshalJSON would
	// fail for every key.
	_ "github.com/luxfi/evm/precompile/registry"
)

// canonicalMainnetUpgradeV46 is the byte-for-byte vendored copy of
// luxfi/genesis configs/mainnet/upgrade.json at the v46 precompile-set
// freeze (warpConfig + 18 live-at-block-0 + 27 forward-dated to the
// Quasar Edition activation timestamp). Vendoring it removes runtime
// CWD-walking and makes this regression-proof hermetic in CI runners
// that clone only luxfi/evm.
//
// Sync contract: if luxfi/genesis configs/mainnet/upgrade.json changes,
// regenerate this file:
//
//	cp ../../../../genesis/configs/mainnet/upgrade.json mainnet_upgrade_v46.json
//
// The TestVendoredFixtureMatchesCanonical guard (build-tag: requires the
// sibling genesis checkout) detects drift before merge.
//
//go:embed mainnet_upgrade_v46.json
var canonicalMainnetUpgradeV46 []byte

// TestMainnetUpgradeJSON_UnmarshalsAgainstRegistry is the end-to-end
// regression gate from Red's MEDIUM (vector 9) finding. It enforces
// the strongest possible contract on the canonical upgrade.json:
// every entry round-trips through the same Unmarshal path luxd uses
// at boot. Any unregistered ConfigKey fails the test immediately,
// surfacing the brick-on-boot footgun BEFORE the JSON is shipped to
// production.
func TestMainnetUpgradeJSON_UnmarshalsAgainstRegistry(t *testing.T) {
	raw := readCanonicalMainnetUpgradeJSONRaw(t)

	var cfg extras.UpgradeConfig
	err := json.Unmarshal(raw, &cfg)
	require.NoErrorf(t, err,
		"extras.UpgradeConfig.UnmarshalJSON refused the canonical mainnet "+
			"upgrade.json — luxd would refuse to start the C-Chain VM. "+
			"This is the exact failure mode from Red's vector-8 CRITICAL: "+
			"a key in precompileUpgrades is not registered with "+
			"modules.RegisteredModules. Most likely cause: the canonical "+
			"file added a precompile whose module init() doesn't call "+
			"RegisterModule under the pinned luxfi/precompile version "+
			"(see ~/work/lux/evm/go.mod for the pin). Underlying error: %v",
		err,
	)

	require.NotEmptyf(t, cfg.PrecompileUpgrades,
		"canonical upgrade.json must contain at least one entry in precompileUpgrades",
	)

	// Per-entry contract: every PrecompileUpgrade must yield a non-nil
	// Timestamp(). The parser already enforces this (precompile_upgrade.go
	// line 125), but we re-assert here to make the failure mode explicit
	// for future readers — every activation must have a concrete
	// blockTimestamp; a nil timestamp would cause verifyPrecompileUpgrades
	// to reject the config before luxd even hits checkPrecompileCompatible.
	for i, upg := range cfg.PrecompileUpgrades {
		key := upg.Key()
		ts := upg.Timestamp()
		require.NotNilf(t, ts,
			"precompileUpgrades[%d] key=%q has nil Timestamp() — activations must specify blockTimestamp",
			i, key,
		)
	}
}

// TestMainnetUpgradeJSON_RegistryRejectsUnregisteredKey is the
// negative-control gate. It synthesizes probe upgrade.json fragments
// carrying each known-unregistered key from Red's vector 8 (the
// three EIP precompiles not declared with RegisterModule in pinned
// luxfi/precompile v0.5.27) and asserts the parser refuses each with
// the same "unknown precompile config" error class that luxd would
// emit at boot.
//
// This double-duty as both the negative control AND the regression
// proof requested in Red's MEDIUM finding: if a future re-add of
// these three keys (after a luxfi/precompile bump) ever lands in the
// canonical without the corresponding RegisterModule landing first,
// TestMainnetUpgradeJSON_UnmarshalsAgainstRegistry will trip — and
// THIS test demonstrates the exact "fails when unregistered key
// present" path so the failure mode is self-documenting.
func TestMainnetUpgradeJSON_RegistryRejectsUnregisteredKey(t *testing.T) {
	// One entry per known-unregistered key. Adding a key here is the
	// canonical way to extend coverage if a future luxfi/precompile
	// version ships another module without RegisterModule.
	probes := []struct {
		key  string
		json string
	}{
		{
			key: "kzg4844Config",
			json: `{
			  "networkUpgradeOverrides": {"strictPQTimestamp": 1766708400},
			  "precompileUpgrades": [
			    {"kzg4844Config": {"blockTimestamp": 1782864000}}
			  ]
			}`,
		},
		{
			key: "secp256r1Config",
			json: `{
			  "networkUpgradeOverrides": {"strictPQTimestamp": 1766708400},
			  "precompileUpgrades": [
			    {"secp256r1Config": {"blockTimestamp": 1782864000}}
			  ]
			}`,
		},
		{
			key: "ed25519Config",
			json: `{
			  "networkUpgradeOverrides": {"strictPQTimestamp": 1766708400},
			  "precompileUpgrades": [
			    {"ed25519Config": {"blockTimestamp": 1782864000}}
			  ]
			}`,
		},
	}

	for _, p := range probes {
		t.Run(p.key, func(t *testing.T) {
			var cfg extras.UpgradeConfig
			err := json.Unmarshal([]byte(p.json), &cfg)
			require.Errorf(t, err,
				"parser accepted unregistered key %q — the brick-on-boot guard is broken. "+
					"This negative-control gate exists to ensure that if luxfi/precompile "+
					"ships a key without RegisterModule, the canonical upgrade.json "+
					"that references it will fail UnmarshalJSON, NOT silently boot the "+
					"VM into an unknown state.",
				p.key,
			)
			require.Containsf(t, err.Error(), "unknown precompile config",
				"expected the canonical 'unknown precompile config' error class for %q, got: %v", p.key, err,
			)
			require.Containsf(t, err.Error(), p.key,
				"error must name the offending key so operators can grep the canonical file: %v", err,
			)
		})
	}
}

// TestRegressionProof_SimulatedFortyNineEntryCanonicalFails is the
// explicit regression proof requested in Red's MEDIUM (vector 9)
// remediation. It simulates the pre-patch 49-entry canonical by
// extending the current 46-entry canonical with the three unregistered
// EIP keys and asserts the parser refuses the result.
//
// Concretely: if a future regression re-introduces any of
// {kzg4844Config, secp256r1Config, ed25519Config} into the canonical
// upgrade.json without first bumping luxfi/precompile to a version
// that calls RegisterModule, the boot-time UnmarshalJSON path will
// reject the file with "unknown precompile config: <key>" — and luxd
// will refuse to start the C-Chain VM. This test makes that contract
// machine-checked at PR-review time.
func TestRegressionProof_SimulatedFortyNineEntryCanonicalFails(t *testing.T) {
	raw := readCanonicalMainnetUpgradeJSONRaw(t)

	// Sanity-check: the post-patch canonical accepted as-is.
	var ok extras.UpgradeConfig
	require.NoError(t, json.Unmarshal(raw, &ok),
		"post-patch canonical (46 entries) must parse cleanly — see TestMainnetUpgradeJSON_UnmarshalsAgainstRegistry",
	)
	require.Lenf(t, ok.PrecompileUpgrades, 46,
		"canonical entry count drifted: this regression-proof test was authored against 46 entries (warpConfig + 18 live + 27 forward-dated). If the canonical count legitimately changed, update this assertion alongside.",
	)

	// Build a "pre-patch" 49-entry probe by injecting the three
	// unregistered keys at the same forward-date as the original
	// vector-8 CRITICAL.
	var asObj map[string]any
	require.NoError(t, json.Unmarshal(raw, &asObj))
	upgrades, _ := asObj["precompileUpgrades"].([]any)
	for _, key := range []string{"kzg4844Config", "secp256r1Config", "ed25519Config"} {
		upgrades = append(upgrades, map[string]any{
			key: map[string]any{"blockTimestamp": 1782864000},
		})
	}
	asObj["precompileUpgrades"] = upgrades
	probe, err := json.Marshal(asObj)
	require.NoError(t, err)

	// The pre-patch 49-entry shape MUST fail the parser.
	var bad extras.UpgradeConfig
	err = json.Unmarshal(probe, &bad)
	require.Errorf(t, err,
		"simulated 49-entry canonical (the pre-patch shape Red flagged in vector 8) was accepted by the parser — the regression guard is broken. "+
			"Expected one of {kzg4844Config, secp256r1Config, ed25519Config} to be rejected as 'unknown precompile config'.",
	)
	require.Containsf(t, err.Error(), "unknown precompile config",
		"expected the canonical 'unknown precompile config' error class, got: %v", err,
	)
}

// readCanonicalMainnetUpgradeJSONRaw returns the vendored canonical
// mainnet upgrade.json. The fixture is embedded at build time via
// //go:embed (see canonicalMainnetUpgradeV46) so the test is hermetic
// in CI runners that clone only luxfi/evm — closing Red round-2 vector
// V12 (silent t.Skip on relative-path miss).
//
// The bytes returned MUST be byte-for-byte equal to
// ~/work/lux/genesis/configs/mainnet/upgrade.json. The sync rule lives
// next to the embed directive.
func readCanonicalMainnetUpgradeJSONRaw(t *testing.T) []byte {
	t.Helper()
	if len(canonicalMainnetUpgradeV46) == 0 {
		// The embed directive guarantees the file is present at build
		// time; a zero-length read here would be a build-system bug.
		t.Fatal("embedded canonical mainnet upgrade.json is empty — //go:embed mainnet_upgrade_v46.json failed at build time")
	}
	return canonicalMainnetUpgradeV46
}

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
//	PrecompileUpgrade.Config. If any key is unregistered, this test
//	fails immediately. (Historically kzg4844Config, secp256r1Config and
//	ed25519Config were the unregistered offenders under pinned
//	luxfi/precompile v0.5.27; v0.5.38 registers them — the negative
//	control now uses synthetic sentinel keys, see
//	TestMainnetUpgradeJSON_RegistryRejectsUnregisteredKey.)
//
// This is the regression gate from Red's MEDIUM (vector 9) finding —
// the JSON-only rollout tests in extras_test would have missed the
// vector-8 CRITICAL because they never exercise the per-key
// modules.GetPrecompileModule lookup.
package extras_e2e_test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/evm/params/extras"
	// Side-effect import: every precompile module's init() registers
	// its ConfigKey + factory with modules.RegisteredModules. Without
	// this import the lookup in PrecompileUpgrade.UnmarshalJSON would
	// fail for every key.
	_ "github.com/luxfi/evm/precompile/registry"
)

// canonicalMainnetUpgrade is the byte-for-byte vendored copy of
// luxfi/genesis configs/mainnet/upgrade.json. Vendoring it removes runtime
// CWD-walking and makes this regression proof hermetic in CI runners that
// clone only luxfi/evm. A workspace drift test keeps the copy synchronized.
//
// Sync contract: if luxfi/genesis configs/mainnet/upgrade.json changes,
// regenerate this file:
//
//	cp ../../../../genesis/configs/mainnet/upgrade.json mainnet_upgrade.json
//
// The TestVendoredFixtureMatchesCanonical guard (build-tag: requires the
// sibling genesis checkout) detects drift before merge.
//
//go:embed mainnet_upgrade.json
var canonicalMainnetUpgrade []byte

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
// carrying keys that are NOT registered with modules.RegisteredModules
// and asserts the parser refuses each with the same "unknown precompile
// config" error class that luxd would emit at boot.
//
// History: the original probes were the three EIP precompiles
// (kzg4844Config, secp256r1Config, ed25519Config) that pinned
// luxfi/precompile v0.5.27 shipped WITHOUT RegisterModule — Red's
// vector-8 CRITICAL. As of luxfi/precompile v0.5.38 those three modules
// now call modules.RegisterModule in their init() (verify:
// `grep -rn RegisterModule $(go list -m -f '{{.Dir}}' github.com/luxfi/precompile)/{kzg4844,secp256r1,ed25519}`),
// so they are registered and could no longer serve as the negative
// control. The probes below are therefore synthetic sentinel keys that
// no precompile module will ever register; they exercise the identical
// modules.GetPrecompileModule miss → "unknown precompile config"
// rejection path while being immune to dependency bumps that add real
// modules. This decouples the negative-control mechanism from the churn
// of which real precompiles happen to be (un)registered at a given pin.
func TestMainnetUpgradeJSON_RegistryRejectsUnregisteredKey(t *testing.T) {
	// One entry per synthetic unregistered key. Adding a key here is the
	// canonical way to extend coverage; keep them obviously non-real so a
	// future luxfi/precompile bump can never accidentally register one.
	probes := []struct {
		key  string
		json string
	}{
		{
			key: "definitelyNotARealPrecompileConfig",
			json: `{
			  "networkUpgradeOverrides": {"strictPQTimestamp": 1766708400},
			  "precompileUpgrades": [
			    {"definitelyNotARealPrecompileConfig": {"blockTimestamp": 1782864000}}
			  ]
			}`,
		},
		{
			key: "luxNonexistentSentinelConfig",
			json: `{
			  "networkUpgradeOverrides": {"strictPQTimestamp": 1766708400},
			  "precompileUpgrades": [
			    {"luxNonexistentSentinelConfig": {"blockTimestamp": 1782864000}}
			  ]
			}`,
		},
		{
			key: "unregisteredProbeConfig",
			json: `{
			  "networkUpgradeOverrides": {"strictPQTimestamp": 1766708400},
			  "precompileUpgrades": [
			    {"unregisteredProbeConfig": {"blockTimestamp": 1782864000}}
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

// TestRegressionProof_SimulatedUnregisteredEntriesFail extends the canonical
// schedule with unregistered keys and asserts the parser refuses it.
//
// Concretely: if a future regression introduces any unregistered
// precompile config key into the canonical upgrade.json (a module
// referenced before its init() calls RegisterModule, or a typo'd key),
// the boot-time UnmarshalJSON path will reject the file with
// "unknown precompile config: <key>" — and luxd will refuse to start
// the C-Chain VM. This test makes that contract machine-checked at
// PR-review time.
//
// The injected keys are synthetic sentinels rather than the original
// vector-8 EIP keys (kzg4844Config, secp256r1Config, ed25519Config),
// which luxfi/precompile v0.5.38 now registers — see the history note on
// TestMainnetUpgradeJSON_RegistryRejectsUnregisteredKey. Sentinels keep
// the rejection contract decoupled from real-module registration churn.
func TestRegressionProof_SimulatedUnregisteredEntriesFail(t *testing.T) {
	raw := readCanonicalMainnetUpgradeJSONRaw(t)

	// Sanity-check: the post-patch canonical accepted as-is.
	var ok extras.UpgradeConfig
	require.NoError(t, json.Unmarshal(raw, &ok),
		"canonical upgrade schedule must parse cleanly — see TestMainnetUpgradeJSON_UnmarshalsAgainstRegistry",
	)
	require.Lenf(t, ok.PrecompileUpgrades, 49,
		"canonical entry count drifted; synchronize the fixture with luxfi/genesis and review the change",
	)

	// Inject three guaranteed-unregistered sentinel keys into the canonical
	// schedule. The parser must reject the first unknown key.
	var asObj map[string]any
	require.NoError(t, json.Unmarshal(raw, &asObj))
	upgrades, _ := asObj["precompileUpgrades"].([]any)
	for _, key := range []string{"definitelyNotARealPrecompileConfig", "luxNonexistentSentinelConfig", "unregisteredProbeConfig"} {
		upgrades = append(upgrades, map[string]any{
			key: map[string]any{"blockTimestamp": 1782864000},
		})
	}
	asObj["precompileUpgrades"] = upgrades
	probe, err := json.Marshal(asObj)
	require.NoError(t, err)

	// The augmented schedule MUST fail the parser.
	var bad extras.UpgradeConfig
	err = json.Unmarshal(probe, &bad)
	require.Errorf(t, err,
		"canonical schedule with injected unregistered entries was accepted — the regression guard is broken. "+
			"Expected one of the injected keys to be rejected as 'unknown precompile config'.",
	)
	require.Containsf(t, err.Error(), "unknown precompile config",
		"expected the canonical 'unknown precompile config' error class, got: %v", err,
	)
}

// readCanonicalMainnetUpgradeJSONRaw returns the vendored canonical
// mainnet upgrade.json. The fixture is embedded at build time via
// //go:embed (see canonicalMainnetUpgrade) so the test is hermetic
// in CI runners that clone only luxfi/evm — closing Red round-2 vector
// V12 (silent t.Skip on relative-path miss).
//
// The bytes returned MUST be byte-for-byte equal to
// ~/work/lux/genesis/configs/mainnet/upgrade.json. The sync rule lives
// next to the embed directive.
func readCanonicalMainnetUpgradeJSONRaw(t *testing.T) []byte {
	t.Helper()
	if len(canonicalMainnetUpgrade) == 0 {
		// The embed directive guarantees the file is present at build
		// time; a zero-length read here would be a build-system bug.
		t.Fatal("embedded canonical mainnet upgrade.json is empty — //go:embed mainnet_upgrade.json failed at build time")
	}
	return canonicalMainnetUpgrade
}

// TestVendoredFixturesMatchCanonicalWorkspace catches stale embedded fixtures
// whenever evm and genesis are checked out as siblings. Standalone module CI
// still exercises the embedded files through the parsing tests above.
func TestVendoredFixturesMatchCanonicalWorkspace(t *testing.T) {
	tests := []struct {
		name    string
		fixture []byte
		path    string
	}{
		{"mainnet", canonicalMainnetUpgrade, "../../../genesis/configs/mainnet/upgrade.json"},
		{"testnet", canonicalTestnetUpgrade, "../../../genesis/configs/testnet/upgrade.json"},
	}

	found := false
	for _, test := range tests {
		canonical, err := os.ReadFile(test.path)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		found = true
		require.Truef(t, bytes.Equal(test.fixture, canonical),
			"%s embedded upgrade fixture drifted from %s", test.name, test.path)
	}
	if !found {
		t.Skip("sibling luxfi/genesis checkout not present")
	}
}

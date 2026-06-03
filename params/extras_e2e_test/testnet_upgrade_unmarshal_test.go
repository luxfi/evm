// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Testnet-side regression gate. Mirrors mainnet_upgrade_v46.json's
// upgrade_unmarshal_test.go (task #99) for the Lux primary testnet
// C-Chain upgrade schedule. The mainnet test wouldn't catch a testnet
// drift because the canonical fixture vendored there is only mainnet;
// this test pins the testnet shape so future precompile bumps can't
// silently brick a testnet validator that pulled the JSON before the
// matching evm-plugin rebuild reached the cluster.

package extras_e2e_test

import (
	_ "embed"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/evm/params/extras"
	// Side-effect import already brought in by upgrade_unmarshal_test.go,
	// but we re-declare it here to make the file self-contained for
	// readers who diff this file in isolation.
	_ "github.com/luxfi/evm/precompile/registry"
)

// canonicalTestnetUpgradeV47 is the byte-for-byte vendored copy of
// luxfi/genesis configs/testnet/upgrade.json at the v47 precompile-set
// freeze (warpConfig + 19 forward-dated 1766708400 net-new + 27 more
// safe-subset cryptography precompiles also at 1766708400). The
// testnet schedule front-loads all activations to the strict-PQ
// timestamp because there is no live testnet validator state to keep
// backwards-compatible against — it's the experimentation lane.
//
// Sync contract: if luxfi/genesis configs/testnet/upgrade.json changes,
// regenerate this file:
//
//	cp ../../../../genesis/configs/testnet/upgrade.json testnet_upgrade_v47.json
//
//go:embed testnet_upgrade_v47.json
var canonicalTestnetUpgradeV47 []byte

// TestTestnetUpgradeJSON_UnmarshalsAgainstRegistry mirrors the mainnet
// regression gate. Every key in the canonical testnet upgrade.json
// must round-trip through extras.UpgradeConfig.UnmarshalJSON without
// an "unknown precompile config" error. Any unregistered ConfigKey
// fails the test immediately — surfacing the brick-on-boot footgun
// BEFORE the JSON is shipped to the lux-testnet StatefulSet.
func TestTestnetUpgradeJSON_UnmarshalsAgainstRegistry(t *testing.T) {
	raw := readCanonicalTestnetUpgradeJSONRaw(t)

	var cfg extras.UpgradeConfig
	err := json.Unmarshal(raw, &cfg)
	require.NoErrorf(t, err,
		"extras.UpgradeConfig.UnmarshalJSON refused the canonical testnet "+
			"upgrade.json — luxd would refuse to start the testnet C-Chain VM. "+
			"Most likely cause: the canonical added a precompile whose module "+
			"init() doesn't call RegisterModule under the pinned "+
			"luxfi/precompile version (see ~/work/lux/evm/go.mod). "+
			"Underlying error: %v",
		err,
	)

	require.NotEmptyf(t, cfg.PrecompileUpgrades,
		"canonical testnet upgrade.json must contain at least one entry in precompileUpgrades",
	)

	for i, upg := range cfg.PrecompileUpgrades {
		key := upg.Key()
		ts := upg.Timestamp()
		require.NotNilf(t, ts,
			"precompileUpgrades[%d] key=%q has nil Timestamp() — activations must specify blockTimestamp",
			i, key,
		)
	}
}

// readCanonicalTestnetUpgradeJSONRaw returns the vendored canonical
// testnet upgrade.json. Mirrors the mainnet helper.
func readCanonicalTestnetUpgradeJSONRaw(t *testing.T) []byte {
	t.Helper()
	if len(canonicalTestnetUpgradeV47) == 0 {
		t.Fatal("embedded canonical testnet upgrade.json is empty — //go:embed testnet_upgrade_v47.json failed at build time")
	}
	return canonicalTestnetUpgradeV47
}

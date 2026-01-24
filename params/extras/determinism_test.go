// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"encoding/json"
	"testing"

	"github.com/luxfi/evm/precompile/modules"
	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/require"
)

// TestAllGenesisPrecompilesDeterminism verifies that AllGenesisPrecompiles
// returns consistent results across multiple calls.
func TestAllGenesisPrecompilesDeterminism(t *testing.T) {
	// Call multiple times and verify results are identical
	result1 := AllGenesisPrecompiles()
	result2 := AllGenesisPrecompiles()

	require.Equal(t, len(result1), len(result2), "AllGenesisPrecompiles should return same number of precompiles")

	for key, config1 := range result1 {
		config2, ok := result2[key]
		require.True(t, ok, "key %s should exist in both results", key)
		require.Equal(t, config1.Key(), config2.Key(), "config keys should match")
		// All genesis configs should have timestamp = 0
		require.NotNil(t, config1.Timestamp(), "genesis config timestamp should not be nil")
		require.Equal(t, uint64(0), *config1.Timestamp(), "genesis config timestamp should be 0")
	}

	// Verify all registered modules are present
	for _, module := range modules.RegisteredModules() {
		_, ok := result1[module.ConfigKey]
		require.True(t, ok, "module %s should be in AllGenesisPrecompiles", module.ConfigKey)
	}
}

// TestPrecompilesMarshalDeterministic verifies that MarshalJSONDeterministic
// produces consistent output regardless of map iteration order.
func TestPrecompilesMarshalDeterministic(t *testing.T) {
	precompiles := AllGenesisPrecompiles()

	// Marshal multiple times and verify output is identical
	json1, err := precompiles.MarshalJSONDeterministic()
	require.NoError(t, err)

	json2, err := precompiles.MarshalJSONDeterministic()
	require.NoError(t, err)

	require.Equal(t, string(json1), string(json2), "deterministic marshal should produce identical output")

	// Verify it's valid JSON
	var parsed map[string]json.RawMessage
	err = json.Unmarshal(json1, &parsed)
	require.NoError(t, err)
	require.Equal(t, len(precompiles), len(parsed), "parsed JSON should have same number of keys")
}

// TestAddressBookResolution verifies that address resolution works correctly
// with addressBook overrides.
func TestAddressBookResolution(t *testing.T) {
	// Skip if no modules are registered (shouldn't happen in normal build)
	registeredMods := modules.RegisteredModules()
	if len(registeredMods) == 0 {
		t.Skip("no precompile modules registered")
	}

	// Use the first registered module for testing
	testModule := registeredMods[0]
	testKey := testModule.ConfigKey
	defaultAddr := testModule.Address

	// Create a different address for override
	overrideAddr := common.HexToAddress("0x1234567890123456789012345678901234567890")

	config := &ChainConfig{
		AddressBook: map[string]common.Address{
			testKey: overrideAddr,
		},
	}

	// With addressBook override, should return override address
	addr := config.GetPrecompileAddress(testKey)
	require.Equal(t, overrideAddr, addr, "addressBook should override module address")

	// Without addressBook override, should return module default
	configNoOverride := &ChainConfig{}
	addrDefault := configNoOverride.GetPrecompileAddress(testKey)
	require.Equal(t, defaultAddr, addrDefault, "without addressBook, should use module address")

	// Test LegacyWarpAddress constant is valid
	require.NotEqual(t, common.Address{}, LegacyWarpAddress, "LegacyWarpAddress should be set")
}

// TestEmptyPrecompilesMarshal verifies that empty Precompiles marshal correctly.
func TestEmptyPrecompilesMarshal(t *testing.T) {
	empty := Precompiles{}
	json, err := empty.MarshalJSONDeterministic()
	require.NoError(t, err)
	require.Equal(t, "{}", string(json), "empty Precompiles should marshal to {}")
}

// TestChainConfigGenesisPrecompilesRoundTrip tests that GenesisPrecompiles
// survive JSON round-trip through custom marshal/unmarshal.
func TestChainConfigGenesisPrecompilesRoundTrip(t *testing.T) {
	original := &ChainConfig{
		GenesisPrecompiles: AllGenesisPrecompiles(),
	}

	// Marshal
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Unmarshal
	var restored ChainConfig
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	// Verify GenesisPrecompiles are restored
	require.Equal(t, len(original.GenesisPrecompiles), len(restored.GenesisPrecompiles),
		"GenesisPrecompiles should have same length after round-trip")

	for key := range original.GenesisPrecompiles {
		_, ok := restored.GenesisPrecompiles[key]
		require.True(t, ok, "key %s should exist after round-trip", key)
	}
}

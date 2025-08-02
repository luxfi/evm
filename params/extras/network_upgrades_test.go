// (c) 2022 Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"testing"

	"github.com/luxfi/evm/utils"
	upgrade "github.com/luxfi/node/upgrade"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkUpgradeIsEVM(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name      string
		upgrades  NetworkUpgrades
		timestamp uint64
		expected  bool
	}{
		{
			name: "genesis activated",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			timestamp: 0,
			expected:  true,
		},
		{
			name: "genesis not yet activated",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(10),
			},
			timestamp: 9,
			expected:  true,  // In v2.0.0, IsGenesis always returns true
		},
		{
			name: "genesis activated in past",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(5),
			},
			timestamp: 10,
			expected:  true,
		},
		{
			name: "nil genesis timestamp",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			timestamp: 0,
			expected:  true,  // In v2.0.0, IsGenesis always returns true
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			result := testcase.upgrades.IsGenesis(testcase.timestamp)
			require.Equal(t, testcase.expected, result)
		})
	}
}

func TestNetworkUpgradesDescription(t *testing.T) {
	// Test with nil genesis timestamp
	{
		upgrades := NetworkUpgrades{
			GenesisTimestamp: nil,
		}
		result := upgrades.Description()
		require.Contains(t, result, "Genesis Timestamp: @nil")
	}

	// Test with zero genesis timestamp
	{
		upgrades := NetworkUpgrades{
			GenesisTimestamp: utils.NewUint64(0),
		}
		result := upgrades.Description()
		require.Contains(t, result, "Genesis Timestamp: @0")
	}

	// Test with non-zero genesis timestamp
	{
		upgrades := NetworkUpgrades{
			GenesisTimestamp: utils.NewUint64(100),
		}
		result := upgrades.Description()
		require.Contains(t, result, "Genesis Timestamp: @100")
	}
}

func TestNetworkUpgradesVerify(t *testing.T) {
	testcases := []struct {
		name        string
		upgrades    NetworkUpgrades
		expectError bool
		errorString string
	}{
		{
			name: "valid genesis at 0",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			expectError: false,
		},
		{
			name: "invalid genesis not at 0",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(1),
			},
			expectError: true,
			errorString: "genesis upgrade must be active at timestamp 0",
		},
		{
			name: "invalid nil genesis",
			upgrades: NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			expectError: true,
			errorString: "genesis upgrade must be active at timestamp 0",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			err := testcase.upgrades.verifyNetworkUpgrades(upgrade.Config{})
			if testcase.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), testcase.errorString)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNetworkUpgradesEqual(t *testing.T) {
	testcases := []struct {
		name     string
		upgrade1 *NetworkUpgrades
		upgrade2 *NetworkUpgrades
		expected bool
	}{
		{
			name: "both nil timestamps",
			upgrade1: &NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			upgrade2: &NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			expected: true,
		},
		{
			name: "equal timestamps",
			upgrade1: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			upgrade2: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			expected: true,
		},
		{
			name: "different timestamps",
			upgrade1: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			upgrade2: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(1),
			},
			expected: false,
		},
		{
			name: "one nil one not",
			upgrade1: &NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			upgrade2: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			expected: false,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			result := testcase.upgrade1.Equal(testcase.upgrade2)
			require.Equal(t, testcase.expected, result)
		})
	}
}

func TestCheckNetworkUpgradesCompatible(t *testing.T) {
	// In v2.0.0, checkNetworkUpgradesCompatible always returns nil
	// since all upgrades are active at genesis - no compatibility checks needed
	testcases := []struct {
		name           string
		networkUpgrade *NetworkUpgrades
		newcfg         *NetworkUpgrades
		time           uint64
	}{
		{
			name: "EqualNetworkUpgrades",
			networkUpgrade: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			newcfg: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			time: 0,
		},
		{
			name: "DifferentTimestamps",
			networkUpgrade: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			newcfg: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(1),
			},
			time: 0,
		},
		{
			name: "NilTimestamp",
			networkUpgrade: &NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0),
			},
			newcfg: &NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			time: 0,
		},
		{
			name: "BothNil",
			networkUpgrade: &NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			newcfg: &NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			time: 0,
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			// In v2.0.0, this always returns nil
			err := testcase.networkUpgrade.checkNetworkUpgradesCompatible(testcase.newcfg, testcase.time)
			require.Nil(t, err)
		})
	}
}

// TestCheckNetworkUpgradesCompatibleContext removed - no longer applicable in v2.0.0

func TestActivationTimestamps(t *testing.T) {
	// Test that genesis timestamp must be 0 for v2.0.0
	testcases := []struct {
		name                string
		timestamp           *uint64
		expectedActivated   bool
		expectedDescription string
	}{
		{
			name:                "Genesis Activated",
			timestamp:           utils.NewUint64(0),
			expectedActivated:   true,
			expectedDescription: "Genesis Timestamp: @0",
		},
		{
			name:                "Genesis Not Activated",
			timestamp:           nil,
			expectedActivated:   true,  // In v2.0.0, IsGenesis always returns true
			expectedDescription: "Genesis Timestamp: @nil",
		},
		{
			name:                "Genesis Future Activation",
			timestamp:           utils.NewUint64(10),
			expectedActivated:   true,  // In v2.0.0, IsGenesis always returns true
			expectedDescription: "Genesis Timestamp: @10",
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			upgrades := NetworkUpgrades{
				GenesisTimestamp: testcase.timestamp,
			}
			
			// Test IsGenesis at time 0
			require.Equal(t, testcase.expectedActivated, upgrades.IsGenesis(0))
			
			// Test Description
			desc := upgrades.Description()
			require.Contains(t, desc, testcase.expectedDescription)
		})
	}
}

// TestForkOrder removed - v2.0.0 only has genesis timestamp

func TestSetDefaults(t *testing.T) {
	testcases := []struct {
		name                string
		initial             NetworkUpgrades
		expectedGenesis     *uint64
	}{
		{
			name: "Nil Genesis",
			initial: NetworkUpgrades{
				GenesisTimestamp: nil,
			},
			expectedGenesis: utils.NewUint64(0),
		},
		{
			name: "Existing Genesis",
			initial: NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(10),
			},
			expectedGenesis: utils.NewUint64(10),
		},
	}
	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			testcase.initial.SetDefaults(upgrade.Config{})
			assert.Equal(t, testcase.expectedGenesis, testcase.initial.GenesisTimestamp)
		})
	}
}
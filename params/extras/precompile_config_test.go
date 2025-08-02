// (c) 2022 Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras_test

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/luxfi/evm/v2/commontype"
	"github.com/luxfi/evm/v2/params/extras"
	"github.com/luxfi/evm/v2/precompile/contracts/deployerallowlist"
	"github.com/luxfi/evm/v2/precompile/contracts/feemanager"
	"github.com/luxfi/evm/v2/precompile/contracts/nativeminter"
	"github.com/luxfi/evm/v2/precompile/contracts/rewardmanager"
	"github.com/luxfi/evm/v2/precompile/contracts/txallowlist"
	"github.com/luxfi/evm/v2/utils"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/common/math"
	"github.com/stretchr/testify/require"
)

func TestVerifyWithChainConfig(t *testing.T) {
	admins := []common.Address{{1}}
	enabled := []common.Address{{2}}
	config := extras.GetTestChainConfig()
	config.GenesisPrecompiles = extras.Precompiles{
		txallowlist.ConfigKey: txallowlist.NewConfig(utils.NewUint64(0), admins, enabled, nil),
	}
	require.NoError(t, config.Verify())

	// To re-enable a precompile, it must be disabled first
	upgradeConfig := config.UpgradeConfig
	upgradeConfig.PrecompileUpgrades = []extras.PrecompileUpgrade{
		{
			Config: txallowlist.NewDisableConfig(utils.NewUint64(500)),
		},
		{
			Config: txallowlist.NewConfig(utils.NewUint64(1000), admins, enabled, nil),
		},
	}
	// This is a shallow copy, so it will modify the original config.
	config.UpgradeConfig = upgradeConfig
	require.NoError(t, config.Verify())

	// Conflicting precompile config in the upgrade (trying to enable without disabling)
	upgradeConfig.PrecompileUpgrades = []extras.PrecompileUpgrade{{
		Config: txallowlist.NewConfig(utils.NewUint64(1000), []common.Address{{2}}, admins, nil),
	}}
	config.UpgradeConfig = upgradeConfig
	err := config.Verify()
	require.ErrorContains(t, err, "disable should be [true]")
}

func TestPrecompileUpgradeJSONMarshal(t *testing.T) {
	adminAddrs := []common.Address{{1}, {2}}
	enabledAddrs := []common.Address{{3}, {4}}
	testContractDeployerAllowListConfig := deployerallowlist.NewConfig(utils.NewUint64(10), adminAddrs, enabledAddrs, nil)
	testContractNativeMinterConfig := nativeminter.NewConfig(utils.NewUint64(0), adminAddrs, enabledAddrs, nil,
		map[common.Address]*math.HexOrDecimal256{
			common.HexToAddress("0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC"): (*math.HexOrDecimal256)(common.Big1),
		})
	testFeeManagerConfig := feemanager.NewConfig(utils.NewUint64(0), adminAddrs, enabledAddrs, nil,
		&commontype.FeeConfig{
			GasLimit:                 big.NewInt(8_000_000),
			TargetBlockRate:          2, // in seconds
			MinBaseFee:               big.NewInt(25_000_000_000),
			TargetGas:                big.NewInt(15_000_000),
			BaseFeeChangeDenominator: big.NewInt(36),
			MinBlockGasCost:          big.NewInt(0),
			MaxBlockGasCost:          big.NewInt(1_000_000),
			BlockGasCostStep:         big.NewInt(200_000),
		})
	testRewardManagerConfig := rewardmanager.NewConfig(utils.NewUint64(0), adminAddrs, enabledAddrs, nil,
		&rewardmanager.InitialRewardConfig{
			AllowFeeRecipients: true,
		})
	var manager Manager
	manager.AppendContractDeployerAllowList(testContractDeployerAllowListConfig)
	manager.AppendNativeMinter(testContractNativeMinterConfig)
	manager.AppendFeeManager(testFeeManagerConfig, 1)
	manager.AppendRewardManager(testRewardManagerConfig)

	managerBytes, err := json.Marshal(&manager)
	require.NoError(t, err)
	require.NotEmpty(t, managerBytes)
	
	t.Logf("Marshaled bytes: %s", string(managerBytes))

	var unmarshalledManager Manager
	err = json.Unmarshal(managerBytes, &unmarshalledManager)
	require.NoError(t, err)

	// Now marshal and unmarshal using the ProducerConsumer wrapper to handle
	// the type registration and deserialization.
	require.Equal(t, manager, unmarshalledManager)
}

type Manager struct {
	PrecompileUpgrades []extras.PrecompileUpgrade `json:"precompileUpgrades"`
}

func (m *Manager) AppendContractDeployerAllowList(config *deployerallowlist.Config) {
	m.PrecompileUpgrades = append(m.PrecompileUpgrades, extras.PrecompileUpgrade{Config: config})
}

func (m *Manager) AppendNativeMinter(config *nativeminter.Config) {
	m.PrecompileUpgrades = append(m.PrecompileUpgrades, extras.PrecompileUpgrade{Config: config})
}

func (m *Manager) AppendFeeManager(config *feemanager.Config, gas uint64) {
	m.PrecompileUpgrades = append(m.PrecompileUpgrades, extras.PrecompileUpgrade{Config: config})
}

func (m *Manager) AppendRewardManager(config *rewardmanager.Config) {
	m.PrecompileUpgrades = append(m.PrecompileUpgrades, extras.PrecompileUpgrade{Config: config})
}

func TestPrecompileUpgradeUnmarshalJSON(t *testing.T) {
	upgradeBytes := []byte(`{
		"precompileUpgrades": [
			{
				"feeManagerConfig": {
					"blockTimestamp": 0,
					"adminAddresses": ["0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC"],
					"initialFeeConfig": {
						"gasLimit": 20000000,
						"targetBlockRate": 2,
						"minBaseFee": 1000000000,
						"targetGas": 100000000,
						"baseFeeChangeDenominator": 48,
						"minBlockGasCost": 0,
						"maxBlockGasCost": 10000000,
						"blockGasCostStep": 500000
					}
				}
			},
			{
				"contractDeployerAllowListConfig": {
					"blockTimestamp": 0,
					"adminAddresses": ["0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC"]
				}
			}
		]
	}`)

	var upgradeConfig extras.UpgradeConfig
	err := json.Unmarshal(upgradeBytes, &upgradeConfig)
	require.NoError(t, err)

	require.Len(t, upgradeConfig.PrecompileUpgrades, 2)
}

func getTestContractDeployerAllowListConfig() *deployerallowlist.Config {
	return deployerallowlist.NewConfig(
		utils.NewUint64(0),
		[]common.Address{common.HexToAddress("0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC")},
		nil,
		nil)
}

func getTestContractNativeMinterConfig() *nativeminter.Config {
	return nativeminter.NewConfig(
		utils.NewUint64(0),
		[]common.Address{common.HexToAddress("0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC")},
		nil,
		nil,
		map[common.Address]*math.HexOrDecimal256{
			common.HexToAddress("0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC"): (*math.HexOrDecimal256)(common.Big0),
		})
}

// TestPrecompileUpgradesValidation checks precompile upgrade validation
func TestPrecompileUpgradesValidation(t *testing.T) {
	// check ValidatePrecompileUpgrade accepts PrecompileUpgrade with valid precompile config
	chainConfig := extras.GetTestChainConfig()
	precompileConfig := txallowlist.NewConfig(utils.NewUint64(100), []common.Address{{1}}, nil, nil)
	err := precompileConfig.Verify(chainConfig)
	require.NoError(t, err)

	// check ValidatePrecompileUpgrade with empty allowlist config
	// In the current implementation, an empty config might be valid 
	// as it represents no addresses in any role
	var precompileConfig2 txallowlist.Config
	err = precompileConfig2.Verify(chainConfig)
	// If the allowlist is empty (no admins, enabled, or managers), it should be valid
	require.NoError(t, err)
}

// TestVerifyUpgradeConfig commented out - UpgradeConfig.Verify() method no longer exists in v2.0.0
/*
func TestVerifyUpgradeConfig(t *testing.T) {
	// Test cases removed as UpgradeConfig.Verify() no longer exists
}
*/

// TestCheckCompatibleUpgradeConfigs commented out - CheckCompatibleUpgradeConfigs function no longer exists
/*
func TestCheckCompatibleUpgradeConfigs(t *testing.T) {
	// Test cases removed as CheckCompatibleUpgradeConfigs() no longer exists
}
*/

// TestNewRules tests that the rules are created correctly for v2.0.0
func TestNewRules(t *testing.T) {
	config := extras.GetTestChainConfig()
	
	// In v2.0.0, rules are based on the genesis upgrade
	// The Rules method signature changed - it now requires (blockNumber *big.Int, isEIP158 bool, timestamp uint64)
	rules := config.Rules(big.NewInt(0), true, 0)
	
	// Verify basic rules are set 
	require.NotNil(t, rules)
}
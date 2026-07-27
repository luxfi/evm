// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/precompile/contracts/feemanager"
	"github.com/luxfi/evm/precompile/contracts/rewardmanager"
	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/require"
)

// luxDAOGovSafe is the Lux DAO governance Safe on C-Chain 96369. It is what
// "the DAO controls this" has to mean concretely: an address that appears as
// adminAddresses in the upgrade file and can therefore send the precompile's
// admin transactions.
const luxDAOGovSafe = "0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D"

// feeGovernanceUpgradeJSON is the exact text an operator adds to
// precompileUpgrades in a network's upgrade.json to put both fee precompiles
// under the DAO. Kept verbatim in the test so the answer to "can this be done by
// config alone?" is a compiled, executed fact rather than a claim.
//
//   - rewardManagerConfig owns WHERE fees go. rewardAddress is the initial
//     destination; the admin can move it later with a setRewardAddress
//     transaction, no release and no fork.
//   - feeManagerConfig owns HOW LARGE fees are (the dynamic-fee parameters:
//     gasLimit, minBaseFee, targetGas, baseFeeChangeDenominator, block gas cost
//     curve). The admin can change them with a setFeeConfig transaction.
//
// Neither one can change the burn/reward RATIO. That is not in either ABI.
const feeGovernanceUpgradeJSON = `{
  "precompileUpgrades": [
    {
      "rewardManagerConfig": {
        "blockTimestamp": 1785715200,
        "adminAddresses": ["0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D"],
        "initialRewardConfig": {
          "allowFeeRecipients": false,
          "rewardAddress": "0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D"
        }
      }
    },
    {
      "feeManagerConfig": {
        "blockTimestamp": 1785715200,
        "adminAddresses": ["0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D"]
      }
    }
  ]
}`

// TestFeeGovernanceUpgradeParsesWithDAOAsAdmin proves, through the same
// UnmarshalJSON path luxd uses on upgradeBytes, that both fee precompiles can be
// activated with the DAO Safe as admin by a configuration change alone — no node
// release, no consensus fork, no new code.
//
// It also pins WHERE they live. These are LP-aligned addresses, not the legacy
// Subnet-EVM 0x0200..03 / 0x0200..04 that an eth_getCode probe would naturally
// reach for; probing those two proves nothing about this build.
func TestFeeGovernanceUpgradeParsesWithDAOAsAdmin(t *testing.T) {
	dao := common.HexToAddress(luxDAOGovSafe)
	const activation = uint64(1_785_715_200)

	var upgrades UpgradeConfig
	require.NoError(t, json.Unmarshal([]byte(feeGovernanceUpgradeJSON), &upgrades),
		"the upgrade fragment must parse against the module registry")
	require.Len(t, upgrades.PrecompileUpgrades, 2)

	// Addresses are LP-aligned, and they are what GetCoinbaseAt / GetFeeConfigAt read.
	require.Equal(t, common.HexToAddress("0x10205"), rewardmanager.ContractAddress,
		"RewardManager is at 0x...010205, NOT the legacy 0x0200..04")
	require.Equal(t, common.HexToAddress("0x13F01"), feemanager.ContractAddress,
		"FeeManager is at 0x...013F01, NOT the legacy 0x0200..03")

	cfg := &ChainConfig{
		FeeConfig:       DefaultFeeConfig,
		NetworkUpgrades: GetDefaultNetworkUpgrades(),
		UpgradeConfig:   upgrades,
	}

	// Both are off before activation and on from it — an operator can schedule
	// this without touching a binary.
	require.False(t, cfg.IsPrecompileEnabled(rewardmanager.ContractAddress, activation-1))
	require.False(t, cfg.IsPrecompileEnabled(feemanager.ContractAddress, activation-1))
	require.True(t, cfg.IsPrecompileEnabled(rewardmanager.ContractAddress, activation))
	require.True(t, cfg.IsPrecompileEnabled(feemanager.ContractAddress, activation))

	// The DAO Safe is the admin of both, so both admin surfaces are reachable by
	// a Safe transaction — which is what a lux.vote outcome would execute.
	rmCfg, ok := cfg.GetActivePrecompileConfig(rewardmanager.ContractAddress, activation).(*rewardmanager.Config)
	require.True(t, ok)
	require.Equal(t, []common.Address{dao}, rmCfg.AdminAddresses)
	require.Equal(t, dao, rmCfg.InitialRewardConfig.RewardAddress,
		"fees start flowing to the DAO Safe the moment the upgrade activates")

	fmCfg, ok := cfg.GetActivePrecompileConfig(feemanager.ContractAddress, activation).(*feemanager.Config)
	require.True(t, ok)
	require.Equal(t, []common.Address{dao}, fmCfg.AdminAddresses)

	require.NoError(t, cfg.Verify(), "the fragment must be a config a node will start on")
}

// TestFeeGovernanceUpgradeUnblocksTheSplit states the ordering rule as an
// executed fact: the same chain config that Verify rejects while the split has
// no governed destination is accepted once this upgrade fragment is present.
// This is the whole gate — activate RewardManager first, schedule the split
// second — expressed so it cannot be forgotten.
func TestFeeGovernanceUpgradeUnblocksTheSplit(t *testing.T) {
	const activation = uint64(1_785_715_200)

	withSplitOnly := &ChainConfig{
		FeeConfig:         DefaultFeeConfig,
		NetworkUpgrades:   GetDefaultNetworkUpgrades(),
		FeeSplitTimestamp: u64(activation),
	}
	require.Error(t, withSplitOnly.Verify(), "split alone must be refused")

	var upgrades UpgradeConfig
	require.NoError(t, json.Unmarshal([]byte(feeGovernanceUpgradeJSON), &upgrades))

	withGovernance := &ChainConfig{
		FeeConfig:         DefaultFeeConfig,
		NetworkUpgrades:   GetDefaultNetworkUpgrades(),
		FeeSplitTimestamp: u64(activation),
		UpgradeConfig:     upgrades,
	}
	require.NoError(t, withGovernance.Verify(),
		"split + rewardManagerConfig at the same timestamp is the supported rollout")
}

// TestFeeManagerGovernsSizeNotSplit draws the honest boundary of what activating
// feeManagerConfig buys. Its whole state surface is the dynamic-fee parameter
// set — how big fees are. There is no burn share, no reward share, no ratio
// anywhere in it, so no vote routed through it can move the 50/50.
func TestFeeManagerGovernsSizeNotSplit(t *testing.T) {
	governable := commontype.FeeConfig{
		GasLimit:                 big.NewInt(15_000_000),
		TargetBlockRate:          2,
		MinBaseFee:               big.NewInt(25_000_000_000),
		TargetGas:                big.NewInt(60_000_000),
		BaseFeeChangeDenominator: big.NewInt(36),
		MinBlockGasCost:          big.NewInt(0),
		MaxBlockGasCost:          big.NewInt(1_000_000),
		BlockGasCostStep:         big.NewInt(200_000),
	}
	require.NoError(t, governable.Verify(), "every field a feeManager vote can set")

	// Serialize the full governable surface and assert nothing in it names the split.
	encoded, err := json.Marshal(governable)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	for _, forbidden := range []string{"burn", "burnShare", "rewardShare", "split", "feeSplit", "ratio"} {
		require.NotContains(t, fields, forbidden,
			"feeManager governs fee SIZE only; the burn/reward ratio is not one of its parameters")
	}
	require.Subset(t, keysOf(fields), []string{
		"gasLimit", "targetBlockRate", "minBaseFee", "targetGas",
		"baseFeeChangeDenominator", "minBlockGasCost", "maxBlockGasCost", "blockGasCostStep",
	}, "the governable set is exactly the dynamic-fee parameters")
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

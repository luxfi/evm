// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"fmt"
	"testing"

	"github.com/luxfi/evm/params"
	"github.com/stretchr/testify/require"
)

// feeSplitGenesis builds a C-Chain-shaped genesis whose config carries a
// feeSplitTimestamp. withRewardManager toggles whether the RewardManager
// precompile is active at genesis — the governed destination for the kept half
// of every fee. FeeSplit decides how much is kept; the coinbase, resolved from
// RewardManager, decides where it goes.
func feeSplitGenesis(feeSplit uint64, withRewardManager bool) string {
	rm := ""
	if withRewardManager {
		rm = `,
    "rewardManagerConfig": {
      "blockTimestamp": 0,
      "adminAddresses": ["0x9011E888251AB053B7bD1cdB598Db4f9DEd94714"],
      "initialRewardConfig": {
        "rewardAddress": "0xF66B025b46844AFA5d6df54cf0C00E1583cE1abA"
      }
    }`
	}
	return fmt.Sprintf(`{
  "config": {
    "chainId": 96369,
    "homesteadBlock": 0, "eip150Block": 0, "eip155Block": 0, "eip158Block": 0,
    "byzantiumBlock": 0, "constantinopleBlock": 0, "petersburgBlock": 0,
    "istanbulBlock": 0, "muirGlacierBlock": 0, "londonBlock": 0,
    "shanghaiTime": 0, "cancunTime": 0,
    "evmTimestamp": 0, "durangoTimestamp": 0, "quasarTimestamp": 0,
    "fortunaTimestamp": 0, "graniteTimestamp": 0,
    "feeSplitTimestamp": %d,
    "feeConfig": {
      "gasLimit": 12000000, "targetBlockRate": 2, "minBaseFee": 25000000000,
      "targetGas": 500000000, "baseFeeChangeDenominator": 36,
      "minBlockGasCost": 0, "maxBlockGasCost": 1000000, "blockGasCostStep": 200000
    }%s
  },
  "difficulty": "0x0",
  "gasLimit": "0xb71b00",
  "alloc": {}
}`, feeSplit, rm)
}

// TestParseGenesisReadsFeeSplitTimestamp proves parseGenesis carries
// feeSplitTimestamp from the genesis config into the chain's extra config.
// parseGenesis names the network-upgrade timestamps it copies by hand; before
// this, feeSplitTimestamp was not on that list, so a configured split was
// silently dropped and the fee kept burning whole.
func TestParseGenesisReadsFeeSplitTimestamp(t *testing.T) {
	const feeSplit = uint64(1_789_430_400) // 2026-09-15 00:00 UTC

	g, err := parseGenesis(context.Background(), []byte(feeSplitGenesis(feeSplit, true)), nil, "", "")
	require.NoError(t, err, "split + RewardManager at genesis is a config a node starts on")

	extra := params.GetExtra(g.Config)
	require.NotNil(t, extra.FeeSplitTimestamp, "feeSplitTimestamp must survive parseGenesis")
	require.Equal(t, feeSplit, *extra.FeeSplitTimestamp)
	require.True(t, extra.IsFeeSplit(feeSplit), "the split is active from its timestamp")
	require.False(t, extra.IsFeeSplit(feeSplit-1), "and dormant before it")
}

// TestParseGenesisRefusesSplitWithoutRewardManager states the safety invariant
// as a fact reachable through the real VM parse path: a split with no governed
// destination is refused at parse, so a node never boots into burning half of
// every fee and stranding the other half at a keyless address.
func TestParseGenesisRefusesSplitWithoutRewardManager(t *testing.T) {
	const feeSplit = uint64(1_789_430_400)

	_, err := parseGenesis(context.Background(), []byte(feeSplitGenesis(feeSplit, false)), nil, "", "")
	require.Error(t, err, "split without RewardManager must be refused at parse")
}

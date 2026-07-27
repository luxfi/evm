// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/precompile/contracts/rewardmanager"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/require"
)

func u64(v uint64) *uint64 { return &v }

// TestIsFeeSplit verifies the activation gate: nil = never, 0 = genesis, and a
// concrete timestamp turns on at/after that time (never before). This is the
// deterministic predicate the state-transition seam reads to decide whether to
// split fees, so it must be exact at the boundary.
func TestIsFeeSplit(t *testing.T) {
	tests := []struct {
		name string
		ts   *uint64
		time uint64
		want bool
	}{
		{"nil never active at 0", nil, 0, false},
		{"nil never active later", nil, 1_000_000, false},
		{"genesis active at 0", u64(0), 0, true},
		{"genesis active later", u64(0), 5, true},
		{"scheduled inactive before", u64(100), 99, false},
		{"scheduled active at boundary", u64(100), 100, true},
		{"scheduled active after", u64(100), 101, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &ChainConfig{FeeSplitTimestamp: tt.ts}
			require.Equal(t, tt.want, c.IsFeeSplit(tt.time))
		})
	}
}

// TestFeeSplitCompatibility guards against an operator silently changing the
// activation time: once the fork is (or would be) active at the head, its
// timestamp is frozen — a mismatch must be rejected, or nodes fork on fee
// disbursement. A still-future fork may be rescheduled, and equal values are
// always compatible.
func TestFeeSplitCompatibility(t *testing.T) {
	base := func(ts *uint64) *ChainConfig {
		return &ChainConfig{
			NetworkUpgrades:   GetDefaultNetworkUpgrades(),
			FeeSplitTimestamp: ts,
		}
	}
	tests := []struct {
		name      string
		stored    *uint64
		updated   *uint64
		head      uint64
		wantError bool
	}{
		{"unset stays unset", nil, nil, 500, false},
		{"equal active timestamps", u64(100), u64(100), 500, false},
		{"reschedule while both future", u64(100), u64(150), 50, false},
		{"change after activation rejected", u64(100), u64(150), 200, true},
		{"disable after activation rejected", u64(100), nil, 200, true},
		{"enable a still-future fork", nil, u64(300), 100, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := base(tt.stored).checkConfigCompatible(base(tt.updated), new(big.Int), tt.head)
			if tt.wantError {
				require.NotNil(t, err, "expected incompatibility error")
			} else {
				require.Nil(t, err, "expected compatible")
			}
		})
	}
}

// TestVerifyFeeSplitRequiresGovernedDestination is the ordering gate that keeps
// the burn from becoming a value-destruction bug.
//
// The split only decides how much of a fee is kept; the kept part always goes to
// the configured coinbase, which is the RewardManager precompile's stored
// address — or, with that precompile off, the keyless blackhole. Scheduling the
// split alone would therefore burn half of every fee and strand the other half
// forever. ChainConfig.Verify (called from SetupGenesisBlock and VM init) must
// refuse exactly that, and must accept every configuration where the reward half
// has somewhere governed to land.
func TestVerifyFeeSplitRequiresGovernedDestination(t *testing.T) {
	dao := common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D")
	rewardManagerAt := func(ts uint64) Precompiles {
		return Precompiles{
			rewardmanager.ConfigKey: rewardmanager.NewConfig(
				utils.NewUint64(ts),
				[]common.Address{dao}, nil, nil,
				&rewardmanager.InitialRewardConfig{RewardAddress: dao},
			),
		}
	}

	tests := []struct {
		name        string
		config      *ChainConfig
		wantErr     bool
		errContains string
	}{
		{
			name:   "dormant split needs nothing",
			config: &ChainConfig{},
		},
		{
			name: "split without RewardManager is refused",
			config: &ChainConfig{
				FeeSplitTimestamp: u64(1_785_715_200),
			},
			wantErr:     true,
			errContains: "stranded at the keyless blackhole",
		},
		{
			name: "RewardManager enabled at genesis, split later",
			config: &ChainConfig{
				FeeSplitTimestamp:  u64(1_785_715_200),
				GenesisPrecompiles: rewardManagerAt(0),
			},
		},
		{
			name: "RewardManager enabled exactly at the split",
			config: &ChainConfig{
				FeeSplitTimestamp:  u64(1_785_715_200),
				GenesisPrecompiles: rewardManagerAt(1_785_715_200),
			},
		},
		{
			name: "RewardManager enabled AFTER the split is refused",
			config: &ChainConfig{
				FeeSplitTimestamp:  u64(1_785_715_200),
				GenesisPrecompiles: rewardManagerAt(1_785_715_201),
			},
			wantErr:     true,
			errContains: "is not enabled at that time",
		},
		{
			name: "allowFeeRecipients is an explicit, sufficient destination policy",
			config: &ChainConfig{
				FeeSplitTimestamp:  u64(1_785_715_200),
				AllowFeeRecipients: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.verifyFeeSplit()
			if !tt.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// TestVerifyRejectsShippedMainnetFeeSplitSchedule pins the concrete regression:
// the mainnet C-Chain genesis in ~/work/lux/genesis/configs/mainnet/cchain.json
// carries feeSplitTimestamp 1785715200 (2026-08-03T00:00:00Z) with no
// rewardManagerConfig anywhere in the 49-entry mainnet upgrade.json. Verify must
// reject that shape, so the node refuses to start rather than burning half of
// every fee into a keyless account.
func TestVerifyRejectsShippedMainnetFeeSplitSchedule(t *testing.T) {
	cfg := &ChainConfig{
		FeeConfig:         DefaultFeeConfig,
		NetworkUpgrades:   GetDefaultNetworkUpgrades(),
		FeeSplitTimestamp: u64(1_785_715_200), // as shipped in mainnet/cchain.json
	}
	err := cfg.Verify()
	require.Error(t, err, "a split with no governed destination must not verify")
	require.Contains(t, err.Error(), "invalid fee split")
}

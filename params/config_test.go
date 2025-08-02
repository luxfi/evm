// (c) 2020-2020, Lux Industries, Inc.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import (
	"encoding/json"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/utils"
	ethparams "github.com/luxfi/geth/params"
	"github.com/stretchr/testify/require"
)

func TestCheckCompatible(t *testing.T) {
	type test struct {
		stored, new   *ChainConfig
		headBlock     uint64
		headTimestamp uint64
		wantErr       *ethparams.ConfigCompatError
	}
	tests := []test{
		{stored: TestChainConfig, new: TestChainConfig, headBlock: 0, headTimestamp: 0, wantErr: nil},
		{stored: TestChainConfig, new: TestChainConfig, headBlock: 0, headTimestamp: uint64(time.Now().Unix()), wantErr: nil},
		{stored: TestChainConfig, new: TestChainConfig, headBlock: 100, wantErr: nil},
		{
			stored:        &ChainConfig{ChainConfig: &ethparams.ChainConfig{EIP150Block: big.NewInt(10)}},
			new:           &ChainConfig{ChainConfig: &ethparams.ChainConfig{EIP150Block: big.NewInt(20)}},
			headBlock:     9,
			headTimestamp: 90,
			wantErr:       nil,
		},
		{
			stored:        TestChainConfig,
			new:           &ChainConfig{ChainConfig: &ethparams.ChainConfig{HomesteadBlock: nil}},
			headBlock:     3,
			headTimestamp: 30,
			wantErr: &ethparams.ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      nil,
				RewindToBlock: 0,
			},
		},
		{
			stored:        TestChainConfig,
			new:           &ChainConfig{ChainConfig: &ethparams.ChainConfig{HomesteadBlock: big.NewInt(1)}},
			headBlock:     3,
			headTimestamp: 30,
			wantErr: &ethparams.ConfigCompatError{
				What:          "Homestead fork block",
				StoredBlock:   big.NewInt(0),
				NewBlock:      big.NewInt(1),
				RewindToBlock: 0,
			},
		},
		{
			stored:        &ChainConfig{ChainConfig: &ethparams.ChainConfig{HomesteadBlock: big.NewInt(30), EIP150Block: big.NewInt(10)}},
			new:           &ChainConfig{ChainConfig: &ethparams.ChainConfig{HomesteadBlock: big.NewInt(25), EIP150Block: big.NewInt(20)}},
			headBlock:     25,
			headTimestamp: 250,
			wantErr: &ethparams.ConfigCompatError{
				What:          "EIP150 fork block",
				StoredBlock:   big.NewInt(10),
				NewBlock:      big.NewInt(20),
				RewindToBlock: 9,
			},
		},
		{
			stored:        &ChainConfig{ChainConfig: &ethparams.ChainConfig{ConstantinopleBlock: big.NewInt(30)}},
			new:           &ChainConfig{ChainConfig: &ethparams.ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(30)}},
			headBlock:     40,
			headTimestamp: 400,
			wantErr:       nil,
		},
		{
			stored:        &ChainConfig{ChainConfig: &ethparams.ChainConfig{ConstantinopleBlock: big.NewInt(30)}},
			new:           &ChainConfig{ChainConfig: &ethparams.ChainConfig{ConstantinopleBlock: big.NewInt(30), PetersburgBlock: big.NewInt(31)}},
			headBlock:     40,
			headTimestamp: 400,
			wantErr: &ethparams.ConfigCompatError{
				What:          "Petersburg fork block",
				StoredBlock:   nil,
				NewBlock:      big.NewInt(31),
				RewindToBlock: 30,
			},
		},
		// TODO: Fix these tests once TestPreEVMChainConfig is defined
		// {
		// 	stored:        TestChainConfig,
		// 	new:           TestPreEVMChainConfig,
		// 	headBlock:     0,
		// 	headTimestamp: 0,
		// 	wantErr: &ConfigCompatError{
		// 		What:         "EVM fork block timestamp",
		// 		StoredTime:   utils.NewUint64(0),
		// 		NewTime:      GetExtra(TestPreEVMChainConfig).NetworkUpgrades.EVMTimestamp,
		// 		RewindToTime: 0,
		// 	},
		// },
		// {
		// 	stored:        TestChainConfig,
		// 	new:           TestPreEVMChainConfig,
		// 	headBlock:     10,
		// 	headTimestamp: 100,
		// 	wantErr: &ConfigCompatError{
		// 		What:         "EVM fork block timestamp",
		// 		StoredTime:   utils.NewUint64(0),
		// 		NewTime:      GetExtra(TestPreEVMChainConfig).NetworkUpgrades.EVMTimestamp,
		// 		RewindToTime: 0,
		// 	},
		// },
	}

	for _, test := range tests {
		err := test.stored.CheckCompatible(test.new, test.headBlock, test.headTimestamp)
		if !reflect.DeepEqual(err, test.wantErr) {
			t.Errorf("error mismatch:\nstored: %v\nnew: %v\nblockHeight: %v\nerr: %v\nwant: %v", test.stored, test.new, test.headBlock, err, test.wantErr)
		}
	}
}

func TestConfigRules(t *testing.T) {
	c := WithExtra(
		&ChainConfig{ChainConfig: &ethparams.ChainConfig{}},
		&extras.ChainConfig{
			NetworkUpgrades: extras.NetworkUpgrades{
				GenesisTimestamp: utils.NewUint64(0), // All upgrades active at genesis in v2.0.0
			},
		},
	)

	var stamp uint64
	// In v2.0.0, all upgrades are active from genesis
	if r := c.Rules(big.NewInt(0), stamp); !r.IsEVM() {
		t.Errorf("expected %v to be evm", stamp)
	}
	stamp = 500
	if r := c.Rules(big.NewInt(0), stamp); !r.IsEVM() {
		t.Errorf("expected %v to be evm", stamp)
	}
	stamp = math.MaxInt64
	if r := c.Rules(big.NewInt(0), stamp); !r.IsEVM() {
		t.Errorf("expected %v to be evm", stamp)
	}
}


func TestChainConfigMarshalWithUpgrades(t *testing.T) {
	config := ChainConfigWithUpgradesJSON{
		ChainConfig: *WithExtra(
			&ChainConfig{
				ChainConfig: &ethparams.ChainConfig{
					ChainID:             big.NewInt(1),
					HomesteadBlock:      big.NewInt(0),
					EIP150Block:         big.NewInt(0),
					EIP155Block:         big.NewInt(0),
					EIP158Block:         big.NewInt(0),
					ByzantiumBlock:      big.NewInt(0),
					ConstantinopleBlock: big.NewInt(0),
					PetersburgBlock:     big.NewInt(0),
					IstanbulBlock:       big.NewInt(0),
					MuirGlacierBlock:    big.NewInt(0),
				},
			},
			&extras.ChainConfig{
				FeeConfig:          DefaultFeeConfig,
				AllowFeeRecipients: false,
				NetworkUpgrades: extras.NetworkUpgrades{
					GenesisTimestamp: utils.NewUint64(0),
				},
				GenesisPrecompiles: extras.Precompiles{},
			},
		),
		UpgradeConfig: UpgradeConfig{
			PrecompileUpgrades: []extras.PrecompileUpgrade{},
		},
	}
	result, err := json.Marshal(&config)
	require.NoError(t, err)
	expectedJSON := `{
		"chainId": 1,
		"feeConfig": {
			"gasLimit": 8000000,
			"targetBlockRate": 2,
			"minBaseFee": 25000000000,
			"targetGas": 15000000,
			"baseFeeChangeDenominator": 36,
			"minBlockGasCost": 0,
			"maxBlockGasCost": 1000000,
			"blockGasCostStep": 200000
		},
		"homesteadBlock": 0,
		"eip150Block": 0,
		"eip155Block": 0,
		"eip158Block": 0,
		"byzantiumBlock": 0,
		"constantinopleBlock": 0,
		"petersburgBlock": 0,
		"istanbulBlock": 0,
		"muirGlacierBlock": 0,
		"genesisTimestamp": 0,
		"upgrades": {
			"precompileUpgrades": []
		}
	}`
	require.JSONEq(t, expectedJSON, string(result))

	var unmarshalled ChainConfigWithUpgradesJSON
	err = json.Unmarshal(result, &unmarshalled)
	require.NoError(t, err)
	require.Equal(t, config, unmarshalled)
}

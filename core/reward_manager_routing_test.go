// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/precompile/contracts/rewardmanager"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// TestRewardManagerRoutesFeesToDAOSafe proves the owner's tokenomics target
// (100% of C-Chain tx fees accrue to a DAO-governed, on-chain-redirectable
// reward address) using ONLY the existing RewardManager precompile + the
// production GetCoinbaseAt routing:
//
//  1. With RewardManager active and rewardAddress = DAO Safe, GetCoinbaseAt
//     returns the DAO Safe (NOT the blackhole) — so every block's coinbase, and
//     therefore 100% of the fee credited by creditTxFee, lands at the DAO Safe.
//  2. The DAO admin can redirect on-chain: a real setRewardAddress tx from the
//     admin changes the routing to a new address, observed through GetCoinbaseAt.
//     This is the "continually governed / redirectable" requirement.
//
// FeeSplit is left OFF (dormant), so creditTxFee credits the full fee to the
// coinbase — the legacy path — which is exactly the 100%-to-reward-address
// behavior the DAO plan wants.
func TestRewardManagerRoutesFeesToDAOSafe(t *testing.T) {
	adminKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	adminCrypto := crypto.PubkeyToAddress(adminKey.PublicKey)
	admin := common.BytesToAddress(adminCrypto[:])

	daoSafe := common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D")  // Lux DAO Gov Safe
	redirect := common.HexToAddress("0x229599f227231d8C90fcF1a78589F5DC4b7A6962") // stand-in new target
	blackhole := common.HexToAddress("0x0100000000000000000000000000000000000000")

	// EVM-active base config + RewardManager active at genesis, reward = DAO Safe,
	// admin = the admin address (on mainnet this is the DAO Safe itself).
	config := params.Copy(params.TestEVMChainConfig)
	params.GetExtra(config).GenesisPrecompiles = extras.Precompiles{
		rewardmanager.ConfigKey: rewardmanager.NewConfig(
			utils.NewUint64(0),
			[]common.Address{admin}, // admins (DAO)
			nil,                     // enableds
			nil,                     // managers
			&rewardmanager.InitialRewardConfig{RewardAddress: daoSafe},
		),
	}

	gspec := &Genesis{
		Config:   config,
		Alloc:    types.GenesisAlloc{admin: {Balance: new(big.Int).Mul(big.NewInt(10), big.NewInt(params.Ether))}},
		BaseFee:  big.NewInt(25_000_000_000),
		GasLimit: params.GetExtra(config).FeeConfig.GasLimit.Uint64(),
	}
	engine := dummy.NewCoinbaseFaker()

	db, blocks, _, err := GenerateChainWithGenesis(gspec, engine, 1, 10, func(i int, b *BlockGen) {
		// The DAO admin redirects the reward address on-chain via the precompile.
		input, perr := rewardmanager.PackSetRewardAddress(redirect)
		require.NoError(t, perr)
		tx, serr := types.SignTx(
			types.NewTransaction(0, rewardmanager.ContractAddress, common.Big0, 200_000, big.NewInt(225_000_000_000), input),
			types.LatestSigner(config), adminKey,
		)
		require.NoError(t, serr)
		b.AddTx(tx)
	})
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	// 1. BEFORE redirect: at genesis the reward routing is the DAO Safe.
	genesisCoinbase, feeRecipients, err := chain.GetCoinbaseAt(chain.CurrentHeader())
	require.NoError(t, err)
	require.Equal(t, daoSafe, genesisCoinbase, "100%% of fees must route to the DAO Safe (not blackhole)")
	require.NotEqual(t, blackhole, genesisCoinbase, "blackhole must NOT be the fee sink")
	require.False(t, feeRecipients, "single reward address mode, not per-block fee recipients")

	// Apply the block carrying the DAO's setRewardAddress tx.
	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)
	for _, blk := range blocks {
		require.NoError(t, chain.Accept(blk))
	}
	chain.DrainAcceptorQueue()

	// 2. AFTER redirect: the same production routing now points at the new address.
	newCoinbase, _, err := chain.GetCoinbaseAt(chain.CurrentHeader())
	require.NoError(t, err)
	require.Equal(t, redirect, newCoinbase, "DAO admin's on-chain setRewardAddress must redirect fee routing live")

	t.Logf("reward routing before redirect = %s (DAO Safe)", genesisCoinbase)
	t.Logf("reward routing after  redirect = %s (new target, set on-chain by admin)", newCoinbase)
}

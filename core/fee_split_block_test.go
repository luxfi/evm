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

// TestFeeSplitBlockConservation drives real transactions through real block
// production and the actual state-transition seam (Process -> execute ->
// creditTxFee), then measures the resulting account balances. It proves, on
// executed blocks, every property the rollout must exhibit:
//
//   - the kept half accrues to the CONFIGURED COINBASE — an address supplied by
//     chain state, here a governed reward sink, never a compiled-in constant;
//   - the other ~50% is truly burned: the sum of all balances (supply) drops by
//     exactly that amount — no dead-EOA sink holds it;
//   - the blackhole 0x0100..00 accrues nothing, and neither does the retired
//     compiled-in vault 0x0100..02;
//   - conservation is exact: initialSupply - burn == sender + recipient + sink;
//   - the split is deterministic (integer arithmetic, consensus-agreed address)
//     so all validators compute the identical post-state.
func TestFeeSplitBlockConservation(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	senderCrypto := crypto.PubkeyToAddress(key.PublicKey)
	sender := common.BytesToAddress(senderCrypto[:])
	recipient := common.HexToAddress("0x00000000000000000000000000000000000000AA")

	// EVM mode active (full-fee accounting) + FeeSplit active, both from genesis.
	genesisActive := uint64(0)
	config := params.WithExtra(
		&params.ChainConfig{
			ChainID:             big.NewInt(96369),
			HomesteadBlock:      big.NewInt(0),
			EIP150Block:         big.NewInt(0),
			EIP155Block:         big.NewInt(0),
			EIP158Block:         big.NewInt(0),
			ByzantiumBlock:      big.NewInt(0),
			ConstantinopleBlock: big.NewInt(0),
			PetersburgBlock:     big.NewInt(0),
			IstanbulBlock:       big.NewInt(0),
			MuirGlacierBlock:    big.NewInt(0),
			BerlinBlock:         big.NewInt(0),
			LondonBlock:         big.NewInt(0),
		},
		&extras.ChainConfig{
			FeeConfig:         extras.DefaultFeeConfig, // MinBaseFee = 25 gwei
			NetworkUpgrades:   extras.GetDefaultNetworkUpgrades(),
			FeeSplitTimestamp: &genesisActive,
			// The only shape ChainConfig.Verify accepts: the split is on and the
			// kept half has a governed destination.
			GenesisPrecompiles: extras.Precompiles{
				rewardmanager.ConfigKey: rewardmanager.NewConfig(
					utils.NewUint64(0),
					[]common.Address{rewardSink}, nil, nil,
					&rewardmanager.InitialRewardConfig{RewardAddress: rewardSink},
				),
			},
		},
	)
	require.NoError(t, params.GetExtra(config).Verify(), "the config under test must be one a node would start on")

	initialSupply := new(big.Int).Mul(big.NewInt(10), big.NewInt(params.Ether)) // 10 LUX
	gspec := &Genesis{
		Config:   config,
		Alloc:    types.GenesisAlloc{sender: {Balance: initialSupply}},
		BaseFee:  big.NewInt(25_000_000_000), // 25 gwei
		GasLimit: params.GetExtra(config).FeeConfig.GasLimit.Uint64(),
	}

	const numTxs = 5
	value := big.NewInt(1_000_000_000_000_000) // 0.001 LUX per transfer
	gasPrice := big.NewInt(225_000_000_000)    // 225 gwei > base fee
	signer := types.LatestSigner(config)
	engine := dummy.NewCoinbaseFaker()

	db, blocks, _, err := GenerateChainWithGenesis(gspec, engine, 1, 10, func(i int, b *BlockGen) {
		b.SetCoinbase(rewardSink) // the RewardManager-configured destination
		for n := 0; n < numTxs; n++ {
			tx, err := types.SignTx(
				types.NewTransaction(uint64(n), recipient, value, 21000, gasPrice, nil),
				signer, key,
			)
			require.NoError(t, err)
			b.AddTx(tx)
		}
	})
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)
	for _, blk := range blocks {
		require.NoError(t, chain.Accept(blk))
	}
	chain.DrainAcceptorQueue()

	state, err := chain.StateAt(blocks[len(blocks)-1].Root())
	require.NoError(t, err)

	senderBal := state.GetBalance(sender).ToBig()
	recipientBal := state.GetBalance(recipient).ToBig()
	sinkBal := state.GetBalance(rewardSink).ToBig()
	blackholeBal := state.GetBalance(blackholeAddr).ToBig()
	legacyVaultBal := state.GetBalance(legacyFeeRewardVault).ToBig()

	// supplyAfter is the sum of every account that can hold balance here.
	supplyAfter := new(big.Int).Add(senderBal, recipientBal)
	supplyAfter.Add(supplyAfter, sinkBal)
	supplyAfter.Add(supplyAfter, blackholeBal)
	supplyAfter.Add(supplyAfter, legacyVaultBal)

	burn := new(big.Int).Sub(initialSupply, supplyAfter) // supply actually removed
	totalFee := new(big.Int).Add(burn, sinkBal)          // reward + burn
	totalValue := new(big.Int).Mul(value, big.NewInt(numTxs))
	twoSink := new(big.Int).Mul(sinkBal, big.NewInt(2))
	roundingSlack := new(big.Int).Sub(totalFee, twoSink) // odd-wei count in [0, numTxs]

	t.Logf("initial supply     = %s wei", initialSupply)
	t.Logf("sender  balance    = %s wei", senderBal)
	t.Logf("recipient balance  = %s wei (expected %s)", recipientBal, totalValue)
	t.Logf("reward sink        = %s wei (configured coinbase, 50%%)", sinkBal)
	t.Logf("blackhole balance  = %s wei (expected 0)", blackholeBal)
	t.Logf("legacy vault 0x..02= %s wei (expected 0, address retired)", legacyVaultBal)
	t.Logf("total fee          = %s wei", totalFee)
	t.Logf("BURNED (supply -)  = %s wei", burn)
	t.Logf("supply after       = %s wei (= initial - burn)", supplyAfter)

	// 1. The blackhole and the retired compiled-in vault both stay at zero.
	require.Equal(t, 0, blackholeBal.Sign(), "blackhole must not accumulate fees")
	require.Equal(t, 0, legacyVaultBal.Sign(), "retired vault 0x0100..02 must never be credited again")
	// 2. Value transfers landed.
	require.Equal(t, totalValue, recipientBal, "recipient must receive exactly the transferred value")
	// 3. A real, positive burn occurred (supply decreased).
	require.Equal(t, 1, burn.Sign(), "burn must be positive: supply must decrease")
	// 4. The configured coinbase accrued the reward half (positive).
	require.Equal(t, 1, sinkBal.Sign(), "configured coinbase must accrue the reward half")
	// 5. 50/50 within deterministic rounding: 0 <= totalFee - 2*sink <= numTxs.
	require.GreaterOrEqual(t, roundingSlack.Sign(), 0, "sink must not exceed half the fee")
	require.LessOrEqual(t, roundingSlack.Cmp(big.NewInt(numTxs)), 0, "reward and burn halves differ by at most 1 wei per tx")
	// 6. With even per-tx fees (21000 * 225 gwei), the split is exact: burn == sink.
	require.Equal(t, sinkBal, burn, "exact 50/50: burned half equals rewarded half for even fees")
	// 7. Conservation identity, restated: sender debit == value + fee.
	senderDebit := new(big.Int).Sub(initialSupply, senderBal)
	require.Equal(t, new(big.Int).Add(totalValue, totalFee), senderDebit, "sender paid exactly value + fee")
}

// TestFeeSplitRewardHalfFollowsRewardManagerAcrossRedirect is the governance
// loop, end to end, on executed blocks: FeeSplit ON *and* RewardManager ON at
// the same time.
//
//	block 1 : coinbase = GetCoinbaseAt(genesis) = the DAO Safe.
//	          Five paying transfers + one setRewardAddress(newTarget) sent by the
//	          RewardManager admin.
//	block 2 : coinbase = GetCoinbaseAt(block 1) = newTarget, because block 1's
//	          transaction moved the precompile's stored address.
//	          Five more paying transfers.
//
// Measured afterwards, against the real BlockChain: the DAO Safe holds exactly
// half of block 1's fees and not one wei of block 2's; newTarget holds exactly
// half of block 2's; the other half of both is gone from the supply; and neither
// the blackhole nor the retired 0x0100..02 vault ever receives anything.
//
// This is what "the split ratio is protocol, the destination is governed" means
// operationally: a single admin transaction moves the entire reward stream, and
// the burn is completely unaffected by it.
func TestFeeSplitRewardHalfFollowsRewardManagerAcrossRedirect(t *testing.T) {
	adminKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	adminCrypto := crypto.PubkeyToAddress(adminKey.PublicKey)
	admin := common.BytesToAddress(adminCrypto[:])

	daoSafe := common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D")   // Lux DAO Gov Safe
	newTarget := common.HexToAddress("0x229599f227231d8C90fcF1a78589F5DC4b7A6962") // stand-in successor
	recipient := common.HexToAddress("0x00000000000000000000000000000000000000AA")

	genesisActive := uint64(0)
	config := params.Copy(params.TestEVMChainConfig)
	extra := params.GetExtra(config)
	extra.FeeSplitTimestamp = &genesisActive
	extra.GenesisPrecompiles = extras.Precompiles{
		rewardmanager.ConfigKey: rewardmanager.NewConfig(
			utils.NewUint64(0),
			[]common.Address{admin}, // admins — on mainnet this is the DAO Safe / Timelock
			nil,                     // enableds
			nil,                     // managers
			&rewardmanager.InitialRewardConfig{RewardAddress: daoSafe},
		),
	}

	initialSupply := new(big.Int).Mul(big.NewInt(100), big.NewInt(params.Ether))
	gspec := &Genesis{
		Config:   config,
		Alloc:    types.GenesisAlloc{admin: {Balance: initialSupply}},
		BaseFee:  big.NewInt(25_000_000_000),
		GasLimit: extra.FeeConfig.GasLimit.Uint64(),
	}

	const txsPerBlock = 5
	value := big.NewInt(1_000_000_000_000_000) // 0.001 LUX per transfer
	gasPrice := big.NewInt(225_000_000_000)
	signer := types.LatestSigner(config)
	engine := dummy.NewCoinbaseFaker()

	nonce := uint64(0)
	db, blocks, _, err := GenerateChainWithGenesis(gspec, engine, 2, 10, func(i int, b *BlockGen) {
		switch i {
		case 0:
			b.SetCoinbase(daoSafe)
			for n := 0; n < txsPerBlock; n++ {
				tx, serr := types.SignTx(
					types.NewTransaction(nonce, recipient, value, 21000, gasPrice, nil), signer, adminKey)
				require.NoError(t, serr)
				b.AddTx(tx)
				nonce++
			}
			// The RewardManager admin redirects the reward stream on-chain.
			input, perr := rewardmanager.PackSetRewardAddress(newTarget)
			require.NoError(t, perr)
			tx, serr := types.SignTx(
				types.NewTransaction(nonce, rewardmanager.ContractAddress, common.Big0, 200_000, gasPrice, input),
				signer, adminKey)
			require.NoError(t, serr)
			b.AddTx(tx)
			nonce++
		case 1:
			b.SetCoinbase(newTarget)
			for n := 0; n < txsPerBlock; n++ {
				tx, serr := types.SignTx(
					types.NewTransaction(nonce, recipient, value, 21000, gasPrice, nil), signer, adminKey)
				require.NoError(t, serr)
				b.AddTx(tx)
				nonce++
			}
		}
	})
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)
	for _, blk := range blocks {
		require.NoError(t, chain.Accept(blk))
	}
	chain.DrainAcceptorQueue()

	// The coinbase each block was built with is exactly what the production
	// routing dictates at that block's parent — not an arbitrary choice.
	genesisHeader := chain.GetHeaderByNumber(0)
	require.NotNil(t, genesisHeader)
	cbForBlock1, allowRecipients, err := chain.GetCoinbaseAt(genesisHeader)
	require.NoError(t, err)
	require.False(t, allowRecipients, "single reward address mode, not per-block fee recipients")
	require.Equal(t, daoSafe, cbForBlock1, "block 1's coinbase must be the RewardManager address at genesis")
	require.Equal(t, cbForBlock1, blocks[0].Coinbase())

	cbForBlock2, _, err := chain.GetCoinbaseAt(blocks[0].Header())
	require.NoError(t, err)
	require.Equal(t, newTarget, cbForBlock2, "the admin's setRewardAddress in block 1 must move the routing for block 2")
	require.Equal(t, cbForBlock2, blocks[1].Coinbase())

	// Per-block fees, measured from receipts (never assumed).
	feeOf := func(blk *types.Block) *big.Int {
		receipts := chain.GetReceiptsByHash(blk.Hash())
		require.Len(t, receipts, len(blk.Transactions()))
		total := new(big.Int)
		for _, r := range receipts {
			total.Add(total, new(big.Int).Mul(new(big.Int).SetUint64(r.GasUsed), gasPrice))
		}
		return total
	}
	// expectedReward is sum(floor(txFee/2)) — the split is applied per transaction,
	// so the per-tx floor must be summed, not applied to the block total.
	expectedRewardOf := func(blk *types.Block) *big.Int {
		receipts := chain.GetReceiptsByHash(blk.Hash())
		total := new(big.Int)
		for _, r := range receipts {
			txFee := new(big.Int).Mul(new(big.Int).SetUint64(r.GasUsed), gasPrice)
			total.Add(total, new(big.Int).Rsh(txFee, 1))
		}
		return total
	}

	fee1, fee2 := feeOf(blocks[0]), feeOf(blocks[1])
	want1, want2 := expectedRewardOf(blocks[0]), expectedRewardOf(blocks[1])

	state, err := chain.StateAt(blocks[len(blocks)-1].Root())
	require.NoError(t, err)

	daoBal := state.GetBalance(daoSafe).ToBig()
	newBal := state.GetBalance(newTarget).ToBig()
	adminBal := state.GetBalance(admin).ToBig()
	recipientBal := state.GetBalance(recipient).ToBig()
	blackholeBal := state.GetBalance(blackholeAddr).ToBig()
	legacyVaultBal := state.GetBalance(legacyFeeRewardVault).ToBig()

	supplyAfter := new(big.Int).Add(adminBal, recipientBal)
	supplyAfter.Add(supplyAfter, daoBal)
	supplyAfter.Add(supplyAfter, newBal)
	supplyAfter.Add(supplyAfter, blackholeBal)
	supplyAfter.Add(supplyAfter, legacyVaultBal)
	burn := new(big.Int).Sub(initialSupply, supplyAfter)

	t.Logf("block1 fee       = %s wei -> DAO Safe  %s", fee1, daoBal)
	t.Logf("block2 fee       = %s wei -> newTarget %s", fee2, newBal)
	t.Logf("BURNED (supply-) = %s wei", burn)

	// 1. Each destination received exactly the reward half of its own block.
	require.Equal(t, want1, daoBal, "DAO Safe must hold exactly sum(floor(txFee/2)) of block 1")
	require.Equal(t, want2, newBal, "newTarget must hold exactly sum(floor(txFee/2)) of block 2")
	// 2. The redirect is total: the old destination accrues nothing afterwards.
	require.Equal(t, 1, want2.Sign(), "block 2 must actually have produced fees")
	require.NotEqual(t, new(big.Int).Add(want1, want2), daoBal, "DAO Safe must NOT keep receiving after the redirect")
	// 3. The rest of both blocks' fees left the supply entirely.
	totalFee := new(big.Int).Add(fee1, fee2)
	totalReward := new(big.Int).Add(daoBal, newBal)
	require.Equal(t, new(big.Int).Sub(totalFee, totalReward), burn, "burn == totalFee - totalReward, exactly")
	require.Equal(t, 1, burn.Sign(), "burn must be positive")
	// 4. No keyless sink was ever credited.
	require.Equal(t, 0, blackholeBal.Sign(), "blackhole must stay empty")
	require.Equal(t, 0, legacyVaultBal.Sign(), "retired vault 0x0100..02 must stay empty")
	// 5. Value transfers landed.
	require.Equal(t, new(big.Int).Mul(value, big.NewInt(2*txsPerBlock)), recipientBal)
}

// TestGenesisVerifyRejectsFeeSplitWithoutRewardManager pins the guard at the
// entry point a node actually uses: plugin/evm parses the genesis, then calls
// ChainConfig.Verify, and Genesis.Verify does the same for SetupGenesisBlock.
// A genesis that schedules the split with no governed destination must be
// refused there — before a single block is produced — not discovered later by
// noticing that half the fees vanished into an address nobody holds a key to.
func TestGenesisVerifyRejectsFeeSplitWithoutRewardManager(t *testing.T) {
	scheduled := uint64(1_785_715_200) // 2026-08-03T00:00:00Z, as shipped in mainnet/cchain.json
	base := func(precompiles extras.Precompiles) *Genesis {
		cfg := params.WithExtra(
			&params.ChainConfig{ChainID: big.NewInt(96369)},
			&extras.ChainConfig{
				FeeConfig:          extras.DefaultFeeConfig,
				NetworkUpgrades:    extras.GetDefaultNetworkUpgrades(),
				FeeSplitTimestamp:  &scheduled,
				GenesisPrecompiles: precompiles,
			},
		)
		return &Genesis{Config: cfg, GasLimit: extras.DefaultFeeConfig.GasLimit.Uint64()}
	}

	err := base(nil).Verify()
	require.Error(t, err, "split with no governed destination must not verify")
	require.Contains(t, err.Error(), "invalid fee split")

	governed := extras.Precompiles{
		rewardmanager.ConfigKey: rewardmanager.NewConfig(
			utils.NewUint64(0),
			[]common.Address{common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D")},
			nil, nil,
			&rewardmanager.InitialRewardConfig{
				RewardAddress: common.HexToAddress("0x8E29b816c6C35b13cE1ff68D33E245C2bda8ac3D"),
			},
		),
	}
	require.NoError(t, base(governed).Verify(), "split plus RewardManager is the supported shape")
}

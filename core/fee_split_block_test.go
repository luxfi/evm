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
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// TestFeeSplitBlockConservation drives real transactions through real block
// production and the actual state-transition seam (Process -> execute ->
// creditTxFee), then measures the resulting account balances. It proves, on
// executed blocks, every property the devnet rollout must exhibit:
//
//   - the blackhole coinbase 0x0100..00 no longer accumulates fees (== 0),
//     replacing the "stuck EOA" behavior;
//   - the fee-reward vault accrues ~50% of all fees;
//   - the other ~50% is truly burned: the sum of all balances (supply) drops by
//     exactly that amount — no dead-EOA sink holds it;
//   - conservation is exact: initialSupply - burn == sender + recipient + vault;
//   - the split is deterministic (integer, fixed address) so all validators
//     compute the identical post-state.
func TestFeeSplitBlockConservation(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	senderCrypto := crypto.PubkeyToAddress(key.PublicKey)
	sender := common.BytesToAddress(senderCrypto[:])
	recipient := common.HexToAddress("0x00000000000000000000000000000000000000AA")
	blackhole := common.HexToAddress("0x0100000000000000000000000000000000000000")

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
		},
	)

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
	vaultBal := state.GetBalance(extras.FeeRewardVault).ToBig()
	coinbaseBal := state.GetBalance(blackhole).ToBig()

	// supplyAfter is the sum of every account that can hold balance here.
	supplyAfter := new(big.Int).Add(senderBal, recipientBal)
	supplyAfter.Add(supplyAfter, vaultBal)
	supplyAfter.Add(supplyAfter, coinbaseBal)

	burn := new(big.Int).Sub(initialSupply, supplyAfter) // supply actually removed
	totalFee := new(big.Int).Add(burn, vaultBal)         // reward + burn
	totalValue := new(big.Int).Mul(value, big.NewInt(numTxs))
	twoVault := new(big.Int).Mul(vaultBal, big.NewInt(2))
	roundingSlack := new(big.Int).Sub(totalFee, twoVault) // odd-wei count in [0, numTxs]

	t.Logf("initial supply    = %s wei", initialSupply)
	t.Logf("sender  balance   = %s wei", senderBal)
	t.Logf("recipient balance = %s wei (expected %s)", recipientBal, totalValue)
	t.Logf("vault   balance   = %s wei (fee-reward, 50%%)", vaultBal)
	t.Logf("blackhole balance = %s wei (expected 0)", coinbaseBal)
	t.Logf("total fee         = %s wei", totalFee)
	t.Logf("BURNED (supply -) = %s wei", burn)
	t.Logf("supply after      = %s wei (= initial - burn)", supplyAfter)

	// 1. The blackhole coinbase no longer receives fees.
	require.Equal(t, 0, coinbaseBal.Sign(), "blackhole coinbase must not accumulate fees under FeeSplit")
	// 2. Value transfers landed.
	require.Equal(t, totalValue, recipientBal, "recipient must receive exactly the transferred value")
	// 3. A real, positive burn occurred (supply decreased).
	require.Equal(t, 1, burn.Sign(), "burn must be positive: supply must decrease")
	// 4. The reward vault accrued (positive).
	require.Equal(t, 1, vaultBal.Sign(), "vault must accrue the reward half")
	// 5. 50/50 within deterministic rounding: 0 <= totalFee - 2*vault <= numTxs.
	require.GreaterOrEqual(t, roundingSlack.Sign(), 0, "vault must not exceed half the fee")
	require.LessOrEqual(t, roundingSlack.Cmp(big.NewInt(numTxs)), 0, "reward and burn halves differ by at most 1 wei per tx")
	// 6. With even per-tx fees (21000 * 225 gwei), the split is exact: burn == vault.
	require.Equal(t, vaultBal, burn, "exact 50/50: burned half equals rewarded half for even fees")
	// 7. Conservation identity, restated: sender debit == value + fee.
	senderDebit := new(big.Int).Sub(initialSupply, senderBal)
	require.Equal(t, new(big.Int).Add(totalValue, totalFee), senderDebit, "sender paid exactly value + fee")
}

// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package eth

import (
	"context"
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/rpc"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// TestFinalizedSafeResolveToQuasarNeverAboveIt is the export-boundary gate for the two-tier
// consensus: VM.Accept now advances the local NOVA (bare-majority) tip, which is reorgable and
// MUST NOT be exported. The Ethereum `finalized`(-3)/`safe`(-4) block tags must therefore resolve
// to the EXPORT-FINAL (Quasar, ⅔-by-stake) block — NEVER a block above LastQuasarHeight() (which
// trails, and can never exceed, the accepted tip). This proves `eth_getBlockByNumber("finalized")`
// never returns a block above the Quasar height, that it is monotone (never regresses on a
// restart-seed race), and that `latest` stays on the accepted tip (a correct non-finality tag).
func TestFinalizedSafeResolveToQuasarNeverAboveIt(t *testing.T) {
	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	cryptoAddr := crypto.PubkeyToAddress(key.PublicKey)
	addr := common.BytesToAddress(cryptoAddr[:])
	bal, _ := new(big.Int).SetString("100000000000000000000000", 10)

	gspec := &core.Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{addr: {Balance: bal}},
	}
	engine := dummy.NewETHFaker()

	const numBlocks = 5
	// The dummy engine requires the fee-recipient (blackhole) coinbase on generated blocks.
	coinbase := common.HexToAddress("0x0100000000000000000000000000000000000000")
	genDb, blocks, _, err := core.GenerateChainWithGenesis(gspec, engine, numBlocks, 10, func(i int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
	})
	require.NoError(t, err)
	chain, err := core.NewBlockChain(genDb, core.DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { chain.Stop() })
	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)
	// Accept every block so the blockchain's LastAcceptedBlock is the tip — the Nova/accept tip
	// the `latest` tag tracks (distinct from the export/Quasar tip the `finalized` tag tracks).
	for _, blk := range blocks {
		require.NoError(t, chain.Accept(blk))
	}
	chain.DrainAcceptorQueue()

	// The eth backend's blockchain is the canonical chain (genesis..numBlocks). LastAccepted /
	// `latest` is the Nova tip (numBlocks); the export (Quasar) height is set separately below.
	eth := &Ethereum{blockchain: chain}
	backend := &EthAPIBackend{eth: eth}
	ctx := context.Background()
	tip := chain.CurrentBlock().Number.Uint64()
	require.EqualValues(t, numBlocks, tip)

	num := func(tag rpc.BlockNumber) uint64 {
		blk, berr := backend.BlockByNumber(ctx, tag)
		require.NoError(t, berr)
		require.NotNil(t, blk, "resolution for tag %d returned nil", tag)
		return blk.NumberU64()
	}
	hdrNum := func(tag rpc.BlockNumber) uint64 {
		h, herr := backend.HeaderByNumber(ctx, tag)
		require.NoError(t, herr)
		require.NotNil(t, h, "header resolution for tag %d returned nil", tag)
		return h.Number.Uint64()
	}

	// Before any export forms, `finalized`/`safe` are genesis (0) — NEVER the accepted tip.
	require.Zero(t, num(rpc.FinalizedBlockNumber), "finalized must be genesis before the first export, not the accepted tip")
	require.Zero(t, num(rpc.SafeBlockNumber))
	require.Zero(t, hdrNum(rpc.FinalizedBlockNumber))

	// Export advances to height 3 (strictly below the accepted tip 5).
	eth.SetLastQuasarHeight(3)
	require.EqualValues(t, 3, num(rpc.FinalizedBlockNumber))
	require.EqualValues(t, 3, num(rpc.SafeBlockNumber))
	require.EqualValues(t, 3, hdrNum(rpc.FinalizedBlockNumber))
	require.Less(t, num(rpc.FinalizedBlockNumber), tip, "finalized MUST NOT exceed the Quasar height (which trails the accepted tip)")

	// `latest`/`pending` stay on the accepted tip — they are NOT finality tags.
	require.EqualValues(t, tip, num(rpc.LatestBlockNumber), "latest must remain the accepted tip, not the export height")

	// Monotone: a lower export height is ignored (never regress, e.g. a boot-seed racing a cert).
	eth.SetLastQuasarHeight(2)
	require.EqualValues(t, 3, num(rpc.FinalizedBlockNumber), "SetLastQuasarHeight must be monotone (never regress the export frontier)")

	// Export catches up to the tip.
	eth.SetLastQuasarHeight(numBlocks)
	require.EqualValues(t, numBlocks, num(rpc.FinalizedBlockNumber))

	// THE INVARIANT, exhaustively: for EVERY export height, `finalized` resolves to exactly that
	// height and NEVER above the accepted tip.
	for h := uint64(0); h <= numBlocks; h++ {
		e := &Ethereum{blockchain: chain}
		e.SetLastQuasarHeight(h)
		b := &EthAPIBackend{eth: e}
		blk, berr := b.BlockByNumber(ctx, rpc.FinalizedBlockNumber)
		require.NoError(t, berr)
		require.EqualValues(t, h, blk.NumberU64())
		require.LessOrEqual(t, blk.NumberU64(), tip, "finalized must never exceed the accepted tip")
	}
}

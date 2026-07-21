// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

// End-to-end proof that the cevm applier holds RESIDENT state across blocks: it
// drives core.BlockChain.InsertChain over the SAME 256 real Go-produced blocks
// TestCevmShadowVerifiesRealBlocks uses, but asserts the resident metrics —
// cevm's resident keccak-MPT root byte-matches the Go header.Root at EVERY
// height (Agree == N, applied via the resident handle), while the full pre-state
// is dumped+seeded ~ONCE, not N times. The ResidentSeeds == 1 assertion is the
// declined-too-large-killer: the old shadow dumped the whole pre-state every
// block (capped at maxShadowAccounts, which WAS the declined-too-large decline);
// the resident applier dumps once and then applies incrementally.
//
// Built only under -tags cevm (needs the linked C++ EVM). Lives in the external
// test package so it may import core without an import cycle.
package parallel_test

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/core/parallel"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/plugin/evm/upgrade/legacy"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
)

func TestCevmResidentVerifiesRealBlocks(t *testing.T) {
	if !parallel.CevmShadowEnabled() {
		t.Skip("cevm bridge not linked (build with -tags cevm)")
	}

	key1, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	key2, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	cryptoFrom := crypto.PubkeyToAddress(key1.PublicKey)
	cryptoTo := crypto.PubkeyToAddress(key2.PublicKey)
	from := common.BytesToAddress(cryptoFrom[:])
	to := common.BytesToAddress(cryptoTo[:])
	coinbase := common.HexToAddress("0xCafE00000000000000000000000000000000C0DE")
	funds := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(params.Ether))

	gspec := &core.Genesis{
		Config:  params.TestChainConfig,
		Alloc:   core.GenesisAlloc{from: {Balance: funds}, to: {Balance: funds}},
		BaseFee: big.NewInt(legacy.BaseFee),
	}
	signer := types.LatestSigner(gspec.Config)
	engine := dummy.NewCoinbaseFaker()

	// Commit genesis WITH preimages so the applier can recover 20-byte addresses
	// from the pre-block trie keys when it SEEDS (its leaf key is keccak256(addr)).
	db := rawdb.NewMemoryDatabase()
	cacheConfig := *core.DefaultCacheConfig
	cacheConfig.Preimages = true
	cacheConfig.SnapshotLimit = 0

	chain, err := core.NewBlockChain(db, &cacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	if err != nil {
		t.Fatalf("NewBlockChain: %v", err)
	}
	defer chain.Stop()

	// MANY real blocks of pure value transfers (the proven safe subset). Each
	// block carries several transfers from key1 to deterministically-distinct
	// fresh recipients. gasLimit == 21000 so gasUsed == gasLimit; gasPrice ==
	// baseFee so full fee -> coinbase matches cevm's per-tx credit. The blocks
	// flow through the REAL StateProcessor (InsertChain -> Process ->
	// parallel.DefaultExecutor().ExecuteBlock = the RESIDENT cevm applier).
	const (
		nBlocks = 256
		txper   = 3
	)
	var nonce uint64
	blocks, _, err := core.GenerateChain(gspec.Config, chain.Genesis(), engine, db, nBlocks, 10, func(i int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		for j := 0; j < txper; j++ {
			var rcpt common.Address
			rcpt[0] = 0x11
			rcpt[18] = byte(i)
			rcpt[19] = byte(j)
			tx := types.NewTransaction(nonce, rcpt, big.NewInt(int64(1000+i*txper+j)), 21000, big.NewInt(legacy.BaseFee), nil)
			signed, serr := types.SignTx(tx, signer, key1)
			if serr != nil {
				t.Fatalf("SignTx: %v", serr)
			}
			b.AddTx(signed)
			nonce++
		}
		_ = to
	})
	if err != nil {
		t.Fatalf("GenerateChain: %v", err)
	}

	// Reset (also frees any prior resident handle so this chain seeds fresh).
	parallel.ResetCevmShadowStats()
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("InsertChain: %v", err)
	}
	s := parallel.CevmShadowStats()

	t.Logf("cevm RESIDENT stats over %d real blocks (%d txs): %+v", nBlocks, nBlocks*txper, s)

	// Correctness: cevm's resident root byte-matched the Go header.Root at EVERY
	// height, and every block was APPLIED via the resident handle.
	if s.DeclinedNoPreimage > 0 {
		t.Fatalf("applier declined %d blocks for missing preimages", s.DeclinedNoPreimage)
	}
	if s.Processed != nBlocks {
		t.Fatalf("cevm processed %d blocks, want %d (declinedTooLarge=%d declinedTx=%d errored=%d)",
			s.Processed, nBlocks, s.DeclinedTooLarge, s.DeclinedTx, s.Errored)
	}
	if s.Agree != nBlocks {
		t.Fatalf("cevm agreed on %d/%d blocks (disagree=%d finalizeGap=%d) — resident root mismatch vs Go EVM",
			s.Agree, nBlocks, s.Disagree, s.FinalizeGap)
	}
	if s.Disagree != 0 {
		t.Fatalf("cevm disagreed on %d blocks", s.Disagree)
	}
	if s.ResidentApplied != nBlocks {
		t.Fatalf("cevm resident-applied %d/%d blocks — some block did not go through the resident handle path",
			s.ResidentApplied, nBlocks)
	}

	// THE WIN: across N blocks the full pre-state is dumped+seeded ~ONCE (genesis
	// + rare resyncs), not N times. For this clean transfer suite it is exactly 1
	// seed and 0 resyncs — every subsequent block applied against the resident
	// tries with NO dump. That is the declined-too-large-killer.
	if s.ResidentSeeds != 1 {
		t.Fatalf("cevm seeded the full pre-state %d times over %d blocks — want exactly 1 (the no-per-block-dump win)",
			s.ResidentSeeds, nBlocks)
	}
	if s.ResidentReseeds != 0 {
		t.Fatalf("cevm resynced %d times on a clean transfer suite — want 0", s.ResidentReseeds)
	}

	t.Logf("PASS: cevm RESIDENT root byte-matched the Go header.Root on ALL %d/%d blocks (%d transfers), "+
		"applied via the resident handle, with %d full-state seed (not %d) — declined-too-large killed: dumps %d vs blocks %d",
		s.Agree, s.ResidentApplied, nBlocks*txper, s.ResidentSeeds, nBlocks, s.ResidentSeeds, nBlocks)
}

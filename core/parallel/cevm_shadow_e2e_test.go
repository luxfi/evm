// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

// End-to-end proof that the cevm shadow verifier runs over REAL Go-produced
// blocks: it drives core.BlockChain.InsertChain (which calls
// StateProcessor.Process -> parallel.DefaultExecutor().ExecuteBlock on every
// block), and asserts that cevm's independent keccak-MPT root byte-matches the
// header.Root the Go EVM committed. This is the honest deliverable: cevm's real
// process_block is exercised against genuine blocks, not a synthetic fixture.
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

func TestCevmShadowVerifiesRealBlocks(t *testing.T) {
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
	funds := new(big.Int).Mul(big.NewInt(1000), big.NewInt(params.Ether))

	gspec := &core.Genesis{
		Config:  params.TestChainConfig,
		Alloc:   core.GenesisAlloc{from: {Balance: funds}, to: {Balance: funds}},
		BaseFee: big.NewInt(legacy.BaseFee),
	}
	signer := types.LatestSigner(gspec.Config)
	engine := dummy.NewCoinbaseFaker()

	// Commit genesis WITH preimages so the shadow verifier can recover the
	// 20-byte addresses from the pre-block trie keys (its leaf key is
	// keccak256(addr)). Preimages off => the verifier declines honestly.
	db := rawdb.NewMemoryDatabase()
	cacheConfig := *core.DefaultCacheConfig
	cacheConfig.Preimages = true
	cacheConfig.SnapshotLimit = 0

	chain, err := core.NewBlockChain(db, &cacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	if err != nil {
		t.Fatalf("NewBlockChain: %v", err)
	}
	defer chain.Stop()

	// Generate real blocks of pure value transfers (the proven safe subset).
	// gasLimit == 21000 so gasUsed == gasLimit (no refund ambiguity); gasPrice
	// == baseFee so the effective price is exactly baseFee; full fee -> coinbase
	// matches cevm's per-tx coinbase credit.
	const nBlocks = 3
	blocks, _, err := core.GenerateChain(gspec.Config, chain.Genesis(), engine, db, nBlocks, 10, func(i int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		tx := types.NewTransaction(uint64(i), to, big.NewInt(int64(1000+i)), 21000, big.NewInt(legacy.BaseFee), nil)
		signed, serr := types.SignTx(tx, signer, key1)
		if serr != nil {
			t.Fatalf("SignTx: %v", serr)
		}
		b.AddTx(signed)
	})
	if err != nil {
		t.Fatalf("GenerateChain: %v", err)
	}

	parallel.ResetCevmShadowStats()
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatalf("InsertChain: %v", err)
	}
	s := parallel.CevmShadowStats()

	t.Logf("cevm shadow stats over %d real blocks: %+v", nBlocks, s)

	// Every block carried exactly one pure transfer over a fully-snapshottable
	// pre-state with preimages on, so cevm must have PROCESSED every block and
	// AGREED byte-exactly with the Go EVM root on each.
	if s.DeclinedNoPreimage > 0 {
		t.Fatalf("verifier declined %d blocks for missing preimages — genesis/state preimages not available", s.DeclinedNoPreimage)
	}
	if s.Processed != nBlocks {
		t.Fatalf("cevm processed %d blocks, want %d (declinedTooLarge=%d declinedTx=%d errored=%d)",
			s.Processed, nBlocks, s.DeclinedTooLarge, s.DeclinedTx, s.Errored)
	}
	if s.Agree != nBlocks {
		t.Fatalf("cevm agreed on %d/%d blocks (disagree=%d finalizeGap=%d) — root mismatch vs Go EVM",
			s.Agree, nBlocks, s.Disagree, s.FinalizeGap)
	}
	if s.Disagree != 0 {
		t.Fatalf("cevm disagreed on %d blocks", s.Disagree)
	}
	t.Logf("PASS: cevm real-MPT root byte-matched the Go EVM header.Root on all %d blocks", s.Agree)
}

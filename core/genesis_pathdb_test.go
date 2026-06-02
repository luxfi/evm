// Copyright (C) 2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/triedb"
	gethpathdb "github.com/luxfi/geth/triedb/pathdb"
)

// TestGenesisCommit_PathDB_FreshDB documents the contract that pathdb must
// satisfy for genesis-commit to work on a brand-new database.
//
// In production on lux-mainnet 2026-06-02, every newly-created L2 EVM chain
// (hanzo, zoo, spc, pars) panicked at chain create with:
//
//	panic in eth.New: unable to commit genesis block to statedb:
//	  triedb parent [0x56e81f17…] layer missing
//
// 0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421 is
// types.EmptyRootHash — the parentRoot that core.Genesis.toBlock + statedb.Commit
// always supply when committing the very first state on a fresh DB.
//
// Contract: a fresh pathdb-backed triedb.Database must have EmptyRootHash
// registered as a disk layer in its layer tree, so that statedb.Commit(0, …)
// can call db.Update(newRoot, parentRoot=EmptyRootHash, …) without
// layertree.add returning "triedb parent … layer missing".
//
// This test asserts the in-memory pathdb path works. The production wedge
// was caused by a separate problem: the EVM plugin lets eth/backend.go
// pick pathdb as the implicit default for empty DBs (because the upstream
// geth rawdb.ParseStateScheme returns "path" when neither scheme nor stored
// state is present) while simultaneously refusing path mode at vm.go's
// preflight check. The combined effect is that path-mode commit runs
// without ever installing the EmptyRootHash disk layer the way that
// production BadgerDB triggers. See plugin/evm/vm.go's StateScheme
// defaulting block for the fix.
//
// If this test ever regresses, every fresh L1/L2 EVM chain fails at boot.
func TestGenesisCommit_PathDB_FreshDB(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	tdb := triedb.NewDatabase(db, &triedb.Config{
		PathDB: gethpathdb.Defaults,
	})
	defer tdb.Close()

	g := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			common.HexToAddress("0x1000000000000000000000000000000000000001"): {
				Balance: big.NewInt(1),
			},
		},
		GasLimit:   params.GetExtra(params.TestChainConfig).FeeConfig.GasLimit.Uint64(),
		Difficulty: big.NewInt(1),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("genesis.Commit panicked on fresh pathdb DB (regression): %v\n"+
				"This is the lux-mainnet 2026-06-02 chain-creation failure mode.\n"+
				"Verify geth/triedb/pathdb.merkleNodeHasher returns types.EmptyRootHash\n"+
				"when ReadAccountTrieNode is empty, AND that newLayerTree(loadLayers())\n"+
				"installs the disk layer at EmptyRootHash before SetupGenesisBlock runs.",
				r)
		}
	}()

	block, err := g.Commit(db, tdb)
	if err != nil {
		t.Fatalf("genesis.Commit returned error on fresh pathdb DB: %v", err)
	}
	if block == nil {
		t.Fatal("genesis.Commit returned nil block")
	}
	if block.NumberU64() != 0 {
		t.Fatalf("genesis block number = %d, want 0", block.NumberU64())
	}
}

// TestGenesisCommit_PathDB_SetupGenesisBlock_FreshDB exercises the full
// SetupGenesisBlock path that NewBlockChain uses, with pathdb. This is the
// exact call sequence executed by eth.New for a newly-created L2 EVM chain.
func TestGenesisCommit_PathDB_SetupGenesisBlock_FreshDB(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	tdb := triedb.NewDatabase(db, &triedb.Config{
		PathDB: gethpathdb.Defaults,
	})
	defer tdb.Close()

	g := &Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			common.HexToAddress("0x1000000000000000000000000000000000000001"): {
				Balance: big.NewInt(1),
			},
		},
		GasLimit:   params.GetExtra(params.TestChainConfig).FeeConfig.GasLimit.Uint64(),
		Difficulty: big.NewInt(1),
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("SetupGenesisBlock panicked on fresh pathdb DB (regression): %v", r)
		}
	}()

	_, hash, err := SetupGenesisBlock(db, tdb, g, common.Hash{}, false)
	if err != nil {
		t.Fatalf("SetupGenesisBlock returned error on fresh pathdb DB: %v", err)
	}
	if (hash == common.Hash{}) {
		t.Fatal("SetupGenesisBlock returned empty hash")
	}
}

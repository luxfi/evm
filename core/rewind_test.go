// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"

	ethparams "github.com/luxfi/geth/params"
)

// TestRewindToHeight reproduces the mainnet C-Chain recovery at unit scale: an
// EVM that has accepted PAST the consensus-finalized height is rolled back down
// to that height, leaving the on-disk state consistent so the chain re-opens
// cleanly (the exact condition the wrapping proposervm requires — heightMatch
// instead of the fail-closed heightBehind) and can build forward again.
//
// It asserts, in order, the three properties the on-cluster proof checks:
//  1. the rewind lands the chain exactly at N with state present + serveable
//     (balances at N preserved, canonical above N cleared);
//  2. a fresh re-open at N (simulating the node reboot that reads the rewound
//     last-accepted pointer) mounts with committed state — no re-genesis;
//  3. the re-opened chain builds and accepts block N+1.
func TestRewindToHeight(t *testing.T) {
	const (
		tip = 30 // EVM ran ahead to here
		n   = 15 // consensus finality floor we roll back to
	)

	key1, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	key2, _ := crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	ca1 := crypto.PubkeyToAddress(key1.PublicKey)
	ca2 := crypto.PubkeyToAddress(key2.PublicKey)
	addr1 := common.BytesToAddress(ca1[:])
	addr2 := common.BytesToAddress(ca2[:])

	const perBlock = int64(10000) // wei addr1 -> addr2 in every block
	chainDB := rawdb.NewMemoryDatabase()
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{addr1: {Balance: big.NewInt(1e18)}},
	}

	bc, err := createBlockChain(chainDB, pruningConfig, gspec, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}

	signer := types.LatestSigner(params.TestChainConfig)
	_, blocks, _, err := GenerateChainWithGenesis(gspec, bc.engine, tip, 10, func(i int, gen *BlockGen) {
		tx, _ := types.SignTx(
			types.NewTransaction(gen.TxNonce(addr1), addr2, big.NewInt(perBlock), ethparams.TxGas, big.NewInt(225000000000), nil),
			signer, key1)
		gen.AddTx(tx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if err := bc.Accept(b); err != nil {
			t.Fatal(err)
		}
	}
	bc.DrainAcceptorQueue()

	if got := bc.LastAcceptedBlock().NumberU64(); got != tip {
		t.Fatalf("setup: last-accepted = %d, want tip %d", got, tip)
	}

	// blocks is 0-indexed: height h is blocks[h-1].
	blockN := blocks[n-1]
	wantAddr2AtN := big.NewInt(perBlock * int64(n)) // cumulative transfers through height n

	// Mirror the tool: commit state@N to DISK from the tip chain first.
	// MaterializeState regenerates it from the nearest committed ancestor if pruned
	// (no-op if present); ForceCommitState flushes it out of the dirty cache (which
	// the hash scheme drops on Stop), so the reconstruct-AT-N boot can mount it.
	if err := bc.MaterializeState(blockN); err != nil {
		t.Fatalf("materialize state@%d: %v", n, err)
	}
	if err := bc.ForceCommitState(blockN); err != nil {
		t.Fatalf("commit state@%d: %v", n, err)
	}
	bc.Stop() // release the db; the rewind is applied by re-opening AT N

	// ---- (1) rewind: reconstruct AT N (loadLastState reorgs the run-ahead tip
	// away + reprocesses state@N — coreth's own boot path), then FinalizeRewind. --
	atN, err := createBlockChain(chainDB, pruningConfig, gspec, blockN.Hash())
	if err != nil {
		t.Fatalf("reconstruct at N=%d failed: %v", n, err)
	}
	tb, err := atN.FinalizeRewind(n)
	if err != nil {
		t.Fatalf("FinalizeRewind(%d): %v", n, err)
	}
	if tb.Hash() != blockN.Hash() {
		t.Fatalf("rewind returned %s, want block %d %s", tb.Hash(), n, blockN.Hash())
	}
	if h := atN.CurrentBlock(); h.Number.Uint64() != n || h.Hash() != blockN.Hash() {
		t.Fatalf("head = %d (%s), want %d (%s)", h.Number.Uint64(), h.Hash(), n, blockN.Hash())
	}
	if la := atN.LastAcceptedBlock(); la.NumberU64() != n || la.Hash() != blockN.Hash() {
		t.Fatalf("last-accepted = %d (%s), want %d", la.NumberU64(), la.Hash(), n)
	}
	if !atN.HasState(blockN.Root()) {
		t.Fatalf("state root %s at height %d not present after rewind", blockN.Root(), n)
	}
	if next := atN.GetBlockByNumber(n + 1); next != nil {
		t.Fatalf("canonical block at %d still present after rewind (%s)", n+1, next.Hash())
	}
	assertBalance(t, atN, blockN.Root(), addr2, wantAddr2AtN, "after rewind")

	// FinalizeRewind must refuse to "finalize" a target the chain is not at.
	if _, err := atN.FinalizeRewind(tip); err == nil {
		t.Fatalf("expected FinalizeRewind(%d) to fail when head is %d", tip, n)
	}
	atN.Stop()

	// ---- (2) fresh re-open at N mounts with committed state (the reboot) ----
	// This mirrors the node reading the rewound last-accepted pointer on boot:
	// loadLastState re-derives head + reprocesses the committed state from disk.
	reopened, err := createBlockChain(chainDB, pruningConfig, gspec, blockN.Hash())
	if err != nil {
		t.Fatalf("re-open at N failed (would be a boot failure): %v", err)
	}
	defer reopened.Stop()
	if h := reopened.CurrentBlock(); h.Number.Uint64() != n || h.Hash() != blockN.Hash() {
		t.Fatalf("reopened head = %d (%s), want %d", h.Number.Uint64(), h.Hash(), n)
	}
	if !reopened.HasState(blockN.Root()) {
		t.Fatalf("reopened chain missing state at N=%d", n)
	}
	assertBalance(t, reopened, blockN.Root(), addr2, wantAddr2AtN, "after reboot")

	// ---- (3) the re-opened chain builds + accepts N+1 ----
	nextBlock := blocks[n] // height n+1, parent = blockN (still canonical)
	if _, err := reopened.InsertChain([]*types.Block{nextBlock}); err != nil {
		t.Fatalf("failed to build past N (insert %d): %v", n+1, err)
	}
	if err := reopened.Accept(nextBlock); err != nil {
		t.Fatalf("failed to accept %d: %v", n+1, err)
	}
	reopened.DrainAcceptorQueue()
	if h := reopened.CurrentBlock(); h.Number.Uint64() != n+1 {
		t.Fatalf("after building forward head = %d, want %d", h.Number.Uint64(), n+1)
	}
	if la := reopened.LastAcceptedBlock(); la.NumberU64() != n+1 {
		t.Fatalf("after accept head last-accepted = %d, want %d", la.NumberU64(), n+1)
	}
	assertBalance(t, reopened, nextBlock.Root(), addr2, big.NewInt(perBlock*int64(n+1)), "after building N+1")
}

// TestRewindToHeight_NoState verifies HighestCommittedStateAtOrBelow reports the
// zero-re-execution target correctly and that RewindToHeight is idempotent when
// already at the target.
func TestRewindIdempotentAtTarget(t *testing.T) {
	key1, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	ca1 := crypto.PubkeyToAddress(key1.PublicKey)
	addr1 := common.BytesToAddress(ca1[:])
	chainDB := rawdb.NewMemoryDatabase()
	gspec := &Genesis{Config: params.TestChainConfig, Alloc: types.GenesisAlloc{addr1: {Balance: big.NewInt(1e18)}}}

	bc, err := createBlockChain(chainDB, archiveConfig, gspec, common.Hash{})
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Stop()

	_, blocks, _, err := GenerateChainWithGenesis(gspec, bc.engine, 5, 10, func(i int, gen *BlockGen) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bc.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	for _, b := range blocks {
		if err := bc.Accept(b); err != nil {
			t.Fatal(err)
		}
	}
	bc.DrainAcceptorQueue()

	// Archive mode commits every block, so the highest committed state at/below
	// the tip is the tip itself.
	h, _, ok := bc.HighestCommittedStateAtOrBelow(5, 0)
	if !ok || h != 5 {
		t.Fatalf("HighestCommittedStateAtOrBelow(5) = (%d,%v), want (5,true)", h, ok)
	}

	// Finalizing at the current tip is a no-op rewind that still lands consistently
	// (commits state, cleans roots) and asserts head == target.
	if _, err := bc.FinalizeRewind(5); err != nil {
		t.Fatalf("idempotent finalize at tip: %v", err)
	}
	if got := bc.CurrentBlock().Number.Uint64(); got != 5 {
		t.Fatalf("head after no-op finalize = %d, want 5", got)
	}
}

func assertBalance(t *testing.T, bc *BlockChain, root common.Hash, addr common.Address, want *big.Int, ctx string) {
	t.Helper()
	st, err := bc.StateAt(root)
	if err != nil {
		t.Fatalf("%s: StateAt(%s): %v", ctx, root, err)
	}
	got := st.GetBalance(addr).ToBig()
	if got.Cmp(want) != 0 {
		t.Fatalf("%s: balance(%s) = %s, want %s", ctx, addr, got, want)
	}
}

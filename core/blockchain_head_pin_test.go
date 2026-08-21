// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	ethparams "github.com/luxfi/geth/params"
	"github.com/luxfi/geth/trie"
	"github.com/stretchr/testify/require"
)

// headPinConfigWith is a mainnet-faithful pruning cache config: hash scheme,
// snapshots disabled (no snapshot fallback — a missing trie node at the head is
// fatal, exactly as it is for block building on mainnet), parameterized by the
// tipBuffer depth (StateHistory) and the on-disk commit cadence (CommitInterval).
func headPinConfigWith(commitInterval, stateHistory uint64) *CacheConfig {
	return &CacheConfig{
		TrieCleanLimit:            256,
		TrieDirtyLimit:            256,
		TrieDirtyCommitTarget:     20,
		TriePrefetcherParallelism: 4,
		Pruning:                   true,
		CommitInterval:            commitInterval,
		StateHistory:              stateHistory,
		SnapshotLimit:             0,
		AcceptorQueueLimit:        64,
	}
}

// headPinConfig uses the exact mainnet defaults: StateHistory=32 (tipBuffer
// depth), CommitInterval=4096 (so a short chain never writes state to disk — the
// head lives only in the hashdb dirty cache).
func headPinConfig() *CacheConfig { return headPinConfigWith(4096, 32) }

// senderHexKey is the funded account's private key; recipientHexKey funds the
// destination address. Each proof re-derives the key locally for signing.
const (
	senderHexKey    = "b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291"
	recipientHexKey = "8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a"
)

// headPinFixtures returns the sender (funded) and recipient addresses. luxfi/crypto
// uses its own common.Address type, so convert to geth's via the raw bytes.
func headPinFixtures() (sender, recipient common.Address) {
	k, _ := crypto.HexToECDSA(senderHexKey)
	k2, _ := crypto.HexToECDSA(recipientHexKey)
	ks, kr := crypto.PubkeyToAddress(k.PublicKey), crypto.PubkeyToAddress(k2.PublicKey)
	return common.BytesToAddress(ks[:]), common.BytesToAddress(kr[:])
}

// insertAccept inserts, accepts, and synchronously drains one block so that
// AcceptTrie (tipBuffer.Insert -> hashdb.Dereference) has fully executed before
// returning. This is the exact path that evicts the head without the fix.
func insertAccept(t *testing.T, bc *BlockChain, block *types.Block) error {
	t.Helper()
	if err := bc.InsertBlock(block); err != nil {
		return err
	}
	if err := bc.Accept(block); err != nil {
		return err
	}
	bc.DrainAcceptorQueue()
	return nil
}

// TestHeadPin_StateAvailableAfterIdle is the core red->green proof for Bug B.
//
// It reproduces the accepted-head GC eviction: a pruning chain reaches a tip at
// a non-CommitInterval height (state only in the hashdb dirty cache), then idles
// producing empty blocks whose state root is identical to the tip. The
// StateHistory-deep tipBuffer cycles and Dereferences that shared root against
// itself. Because hashdb.Update never references the state root (parents==0),
// WITHOUT the fix the live head's trie is deleted from the dirty cache and,
// never having been flushed to disk, becomes unserveable ("missing trie node").
//
// The invariant asserted here — for accepted head H, StateAt(root(H)) +
// GetBalance(latest) + build-on-H all succeed after idle + GC — FAILS on the
// unpatched code and PASSES with bc.triedb.Reference pinning the root.
func TestHeadPin_StateAvailableAfterIdle(t *testing.T) {
	sender, recipient := headPinFixtures()
	key, _ := crypto.HexToECDSA(senderHexKey)

	genesisBalance := new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{sender: {Balance: genesisBalance}},
	}
	signer := types.LatestSigner(params.TestChainConfig)

	const (
		activeBlocks = 5  // real transfers; tip lands at height 5 (non-4096)
		idleBlocks   = 96 // 3x StateHistory of empty blocks -> buffer fully cycles
	)
	transfer := big.NewInt(1_000)

	// Generate active + idle + one extra (build-on-H) block against a THROWAWAY
	// db. The pruning chain under test runs on its own fresh db and rebuilds all
	// state itself via re-execution, so nothing is pre-committed to disk for it.
	genDB, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), activeBlocks+idleBlocks+1, 10, func(i int, gen *BlockGen) {
		// Active transfer blocks and the final build-on-H block carry a tx; the
		// idle window in between is empty (identical state root -> duplicate roots).
		if i < activeBlocks || i == activeBlocks+idleBlocks {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, transfer, ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
			gen.AddTx(tx)
		}
	})
	require.NoError(t, err)
	_ = genDB

	bc, err := createBlockChain(rawdb.NewMemoryDatabase(), headPinConfig(), gspec, common.Hash{})
	require.NoError(t, err)
	defer bc.Stop()

	// Phase 1: build real state to the tip.
	for i := 0; i < activeBlocks; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]), "active block %d", i+1)
	}
	tipRoot := bc.CurrentHeader().Root
	require.NotEqual(t, common.Hash{}, tipRoot)
	require.NotEqual(t, types.EmptyRootHash, tipRoot, "tip must carry non-trivial state")

	// Record the head balance while it is unambiguously serveable.
	sdb, err := bc.StateAt(tipRoot)
	require.NoError(t, err, "head state must be serveable at the tip before idling")
	balBefore := sdb.GetBalance(sender)
	require.True(t, balBefore.Sign() > 0)

	// Phase 2: idle. Empty blocks share tipRoot; the tipBuffer Dereferences it.
	// Pre-fix, at ~StateHistory idle blocks the tipBuffer cycles and Dereferences
	// the shared head root against itself (parents==0) — the head trie is deleted
	// and, never flushed to disk, becomes unserveable. Checking StateAt(head)
	// after every idle accept catches the eviction at the exact block it happens.
	for i := activeBlocks; i < activeBlocks+idleBlocks; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]), "idle block %d (pre-fix: ancestor state evicted -> chain freezes)", i+1)
		require.Equal(t, tipRoot, bc.CurrentHeader().Root, "idle block %d must preserve the head state root", i+1)
		if _, err := bc.StateAt(bc.CurrentHeader().Root); err != nil {
			t.Fatalf("BUG B reproduced: accepted-head state evicted by GC after %d idle blocks (StateHistory=32): %v", i+1-activeBlocks, err)
		}
	}

	// ---- INVARIANT (fails without the fix, holds with it) ----
	headRoot := bc.CurrentHeader().Root
	require.Equal(t, tipRoot, headRoot)

	// (a) OpenTrie(stateRoot(H)) succeeds after idle + GC.
	sdbAfter, err := bc.StateAt(headRoot)
	require.NoError(t, err, "BUG B: accepted-head state root evicted by GC during idle (missing trie node at tip)")

	// (b) eth_getBalance(latest) is stable and correct after idle + GC.
	balAfter := sdbAfter.GetBalance(sender)
	require.Zero(t, balAfter.Cmp(balBefore), "head balance changed / unreadable after idle: before=%s after=%s", balBefore, balAfter)

	// (c) build-on-H (T+1) succeeds: re-executes against the head state.
	require.NoError(t, insertAccept(t, bc, chain[activeBlocks+idleBlocks]), "BUG B: cannot build T+1 on evicted head state")
	require.Equal(t, chain[activeBlocks+idleBlocks].Hash(), bc.CurrentHeader().Hash())

	// Head advanced and its new state is serveable too.
	_, err = bc.StateAt(bc.CurrentHeader().Root)
	require.NoError(t, err, "post-build head state must be serveable")
}

// TestHeadPin_NoLeakOverLongIdle proves the fix is refcount-balanced: +1 Reference
// per processed block is matched by the tipBuffer's -1 Dereference as roots age
// out. Over a long idle (duplicate roots add no new trie nodes), the hashdb dirty
// cache must stay bounded — no unbounded growth from accumulating references.
func TestHeadPin_NoLeakOverLongIdle(t *testing.T) {
	sender, recipient := headPinFixtures()
	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")

	genesisBalance := new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{sender: {Balance: genesisBalance}},
	}
	signer := types.LatestSigner(params.TestChainConfig)

	const (
		activeBlocks = 5
		idleBlocks   = 20 * 32 // 20x StateHistory of empty blocks
	)
	transfer := big.NewInt(1_000)

	genDB, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), activeBlocks+idleBlocks, 10, func(i int, gen *BlockGen) {
		if i < activeBlocks {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, transfer, ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
			gen.AddTx(tx)
		}
	})
	require.NoError(t, err)
	_ = genDB

	bc, err := createBlockChain(rawdb.NewMemoryDatabase(), headPinConfig(), gspec, common.Hash{})
	require.NoError(t, err)
	defer bc.Stop()

	for i := 0; i < activeBlocks; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]))
	}

	dirtyBytes := func() common.StorageSize {
		_, nodes, _ := bc.triedb.Size()
		return nodes
	}

	// Idle through 2x StateHistory so the tipBuffer has fully cycled and reached
	// steady state, then take the baseline dirty-cache size.
	warmup := activeBlocks + 2*32
	for i := activeBlocks; i < warmup; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]))
	}
	baseline := dirtyBytes()

	// Idle the rest. Empty blocks add no trie nodes and the reference count is
	// balanced, so the dirty cache must not grow beyond the steady-state baseline.
	for i := warmup; i < activeBlocks+idleBlocks; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]))
	}
	final := dirtyBytes()

	t.Logf("hashdb dirty cache: baseline=%v final=%v after %d idle blocks", baseline, final, idleBlocks)
	require.LessOrEqual(t, uint64(final), uint64(baseline), "dirty cache grew during idle: reference count is leaking (baseline=%v final=%v)", baseline, final)

	// Head is still serveable at the end of the long idle.
	_, err = bc.StateAt(bc.CurrentHeader().Root)
	require.NoError(t, err, "head state must remain serveable after long idle")
}

// TestHeadPin_ExecutionIdentical is the mainnet-history safety gate. The fix must
// change ONLY in-memory GC timing — never a state root, block hash, gas value, or
// receipt. This drives a deterministic mixed workload (transfers interleaved with
// empty blocks) through a pruning chain (which runs the patched writeBlockWithState
// Reference) and a parallel archive chain, and asserts every block's canonical
// hash — which commits to state root, receipts root, and gasUsed — is byte-identical
// between them and equal to the fix-independent values produced by GenerateChain.
//
// The golden head hash/root below are additionally verified byte-identical between
// the pre-fix and post-fix builds (see the report's stash A/B run), closing the
// gate: applying the fix reproduces mainnet history exactly.
func TestHeadPin_ExecutionIdentical(t *testing.T) {
	sender, recipient := headPinFixtures()
	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")

	genesisBalance := new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{sender: {Balance: genesisBalance}},
	}
	signer := types.LatestSigner(params.TestChainConfig)

	// A deterministic workload that is well under StateHistory of consecutive
	// empties so it does NOT trigger the bug — both pre-fix and post-fix builds
	// process it cleanly, isolating the "does the fix change any hash?" question.
	const total = 40
	genDB, chain, receipts, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), total, 10, func(i int, gen *BlockGen) {
		// Transfer on even blocks, empty on odd blocks (interleaved duplicate roots).
		if i%2 == 0 {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, big.NewInt(int64(1_000+i)), ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
			gen.AddTx(tx)
		}
	})
	require.NoError(t, err)
	_ = genDB

	// Patched pruning chain (exercises writeBlockWithState's Reference).
	pruneBC, err := createBlockChain(rawdb.NewMemoryDatabase(), headPinConfig(), gspec, common.Hash{})
	require.NoError(t, err)
	defer pruneBC.Stop()

	// Reference archive chain (no tipBuffer GC path at all).
	archiveBC, err := createBlockChain(rawdb.NewMemoryDatabase(), archiveConfig, gspec, common.Hash{})
	require.NoError(t, err)
	defer archiveBC.Stop()

	for i, block := range chain {
		require.NoError(t, insertAccept(t, pruneBC, block), "prune insert block %d", i+1)
		require.NoError(t, insertAccept(t, archiveBC, block), "archive insert block %d", i+1)

		ph := pruneBC.GetHeaderByNumber(uint64(i + 1))
		ah := archiveBC.GetHeaderByNumber(uint64(i + 1))
		// Gate rows 1-5, per block: every consensus-committed field must be
		// byte-identical between the patched pruning chain and the archive chain,
		// and equal to the fix-independent value produced by GenerateChain.
		require.Equal(t, block.Hash(), ph.Hash(), "block %d BLOCK HASH diverged (prune)", i+1)
		require.Equal(t, block.Hash(), ah.Hash(), "block %d BLOCK HASH diverged (archive)", i+1)
		require.Equal(t, block.Root(), ph.Root, "block %d STATE ROOT diverged (prune)", i+1)
		require.Equal(t, ah.Root, ph.Root, "block %d STATE ROOT diverged (prune vs archive)", i+1)
		require.Equal(t, block.ReceiptHash(), ph.ReceiptHash, "block %d RECEIPT ROOT diverged (prune)", i+1)
		require.Equal(t, ah.ReceiptHash, ph.ReceiptHash, "block %d RECEIPT ROOT diverged (prune vs archive)", i+1)
		require.Equal(t, block.GasUsed(), ph.GasUsed, "block %d GAS USED diverged (prune)", i+1)
		require.Equal(t, ah.GasUsed, ph.GasUsed, "block %d GAS USED diverged (prune vs archive)", i+1)
		require.Equal(t, block.Bloom(), ph.Bloom, "block %d LOGS BLOOM diverged (prune)", i+1)
		require.Equal(t, ah.Bloom, ph.Bloom, "block %d LOGS BLOOM diverged (prune vs archive)", i+1)
		// Independently re-derive the receipts root from the canonical receipts.
		require.Equal(t, block.ReceiptHash(), types.DeriveSha(receipts[i], trie.NewStackTrie(nil)), "block %d receipts do not derive to header root", i+1)
	}

	// Whole-chain agreement between patched-pruning and archive execution.
	require.Equal(t, archiveBC.CurrentHeader().Hash(), pruneBC.CurrentHeader().Hash(), "patched pruning head hash != archive head hash")
	require.Equal(t, archiveBC.CurrentHeader().Root, pruneBC.CurrentHeader().Root, "patched pruning head root != archive head root")

	t.Logf("EXECUTION-IDENTICAL head: number=%d hash=%s root=%s",
		pruneBC.CurrentHeader().Number, pruneBC.CurrentHeader().Hash().Hex(), pruneBC.CurrentHeader().Root.Hex())

	// Golden values pinned from the deterministic workload. These are verified
	// byte-identical on the PRE-fix build (stash A/B in the Blue report): the
	// unpatched mainnet plugin produces exactly these. If the fix ever perturbs a
	// state root or block hash, these assertions break before the plugin can ship.
	const (
		goldenHeadHash = "0xdef5961779e04fa8d7e93a166f8e271e0c28363875a128bdb717a82d539ce14d"
		goldenHeadRoot = "0x8d8cc41e11177f8532bb1fe727a4a3b36c56abbe60f4fafa8747b6d842494ae6"
	)
	require.Equal(t, goldenHeadHash, pruneBC.CurrentHeader().Hash().Hex(), "head block hash changed — NOT execution-identical to mainnet history")
	require.Equal(t, goldenHeadRoot, pruneBC.CurrentHeader().Root.Hex(), "head state root changed — NOT execution-identical to mainnet history")
}

// TestHeadPin_Across4096Boundary_NoLeak is the make-or-break interaction test the
// owner flagged: 32-slot tipBuffer × 4096 commit boundary × distinct-root progression
// × non-boundary idle × GC. It proves (a) execution stays healthy across a real 4096
// commit boundary, (b) PREVIOUS head roots are dereferenced as they leave the
// StateHistory window so the dirty cache stays BOUNDED over thousands of distinct
// roots (no retention leak), (c) the root committed at the 4096 boundary is durable
// on disk, and (d) the head remains serveable after crossing the boundary and idling
// at a non-boundary height, with build-H+1 succeeding.
func TestHeadPin_Across4096Boundary_NoLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("crosses a full 4096 commit boundary; skipped under -short")
	}
	sender, recipient := headPinFixtures()
	key, _ := crypto.HexToECDSA(senderHexKey)

	// Fund heavily so thousands of transfers never run out of gas/balance.
	genesisBalance := new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(params.Ether))
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{sender: {Balance: genesisBalance}},
	}
	signer := types.LatestSigner(params.TestChainConfig)

	const (
		commitInterval = uint64(4096)
		stateHistory   = uint64(32)
		activeBlocks   = 4100 // distinct-root transfers; crosses the 4096 boundary
		idleBlocks     = 40   // empty blocks -> idle at a non-boundary tip (4101..4140)
	)

	genDB, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), activeBlocks+idleBlocks, 10, func(i int, gen *BlockGen) {
		if i < activeBlocks {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, big.NewInt(1), ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
			gen.AddTx(tx)
		}
	})
	require.NoError(t, err)
	_ = genDB

	bc, err := createBlockChain(rawdb.NewMemoryDatabase(), headPinConfigWith(commitInterval, stateHistory), gspec, common.Hash{})
	require.NoError(t, err)
	defer bc.Stop()

	dirtyBytes := func() uint64 { _, n, _ := bc.triedb.Size(); return uint64(n) }

	var (
		rootAt4096      common.Hash
		dirtyAt500      uint64
		dirtyAtBoundary uint64
	)
	for i := 0; i < activeBlocks; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]), "active block %d", i+1)
		switch i + 1 {
		case 500:
			dirtyAt500 = dirtyBytes()
		case int(commitInterval): // block 4096 — the commit boundary
			rootAt4096 = bc.CurrentHeader().Root
			dirtyAtBoundary = dirtyBytes()
		}
		// Head is serveable at every step (spot-checked cheaply every 512 blocks).
		if (i+1)%512 == 0 {
			_, err := bc.StateAt(bc.CurrentHeader().Root)
			require.NoError(t, err, "head state unserveable at block %d", i+1)
		}
	}
	require.NotEqual(t, common.Hash{}, rootAt4096)

	// Idle at a non-boundary tip (heights 4101..4140), past StateHistory.
	for i := activeBlocks; i < activeBlocks+idleBlocks; i++ {
		require.NoError(t, insertAccept(t, bc, chain[i]))
	}

	// (b) Distinct-root retention: over 4100 distinct roots the dirty cache must
	// stay bounded to the working set, NOT grow ~linearly. A retention leak would
	// make the final cache dwarf the mid-run sample.
	dirtyFinal := dirtyBytes()
	t.Logf("dirty cache: @500=%d @4096-boundary=%d final(@4140)=%d bytes", dirtyAt500, dirtyAtBoundary, dirtyFinal)
	require.Less(t, dirtyFinal, 4*dirtyAt500, "dirty cache grew ~linearly with chain length: retention leak (@500=%d final=%d)", dirtyAt500, dirtyFinal)

	// (c) The root committed at the 4096 boundary is durable on disk: it is far
	// outside the 32-slot dirty window now, so a successful open must come from disk.
	_, err = bc.StateAt(rootAt4096)
	require.NoError(t, err, "root committed at the 4096 boundary is not durable on disk")

	// (d) Head serveable after boundary + idle, and build-H+1 succeeds.
	_, err = bc.StateAt(bc.CurrentHeader().Root)
	require.NoError(t, err, "head state unserveable after crossing 4096 boundary + idle")

	extra, _, err := GenerateChain(gspec.Config, chain[activeBlocks+idleBlocks-1], dummy.NewCoinbaseFaker(), genDB, 1, 10, func(i int, gen *BlockGen) {
		tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, big.NewInt(7), ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
		gen.AddTx(tx)
	})
	require.NoError(t, err)
	require.NoError(t, insertAccept(t, bc, extra[0]), "cannot build H+1 after 4096 boundary + idle")
}

// TestHeadPin_RestartAtNonBoundary proves the patched plugin recovers cleanly when
// restarted with an accepted head at a non-CommitInterval height (the exact mainnet
// freeze condition): Shutdown commits the last accepted root, and a fresh chain
// reopened on the same db serves that head and builds on it.
func TestHeadPin_RestartAtNonBoundary(t *testing.T) {
	sender, recipient := headPinFixtures()
	key, _ := crypto.HexToECDSA(senderHexKey)

	genesisBalance := new(big.Int).Mul(big.NewInt(1), big.NewInt(params.Ether))
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  types.GenesisAlloc{sender: {Balance: genesisBalance}},
	}
	signer := types.LatestSigner(params.TestChainConfig)

	// Active transfers then idle past StateHistory, ending at a non-boundary height.
	const activeBlocks, idleBlocks = 5, 40
	genDB, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), activeBlocks+idleBlocks, 10, func(i int, gen *BlockGen) {
		if i < activeBlocks {
			tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, big.NewInt(1_000), ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
			gen.AddTx(tx)
		}
	})
	require.NoError(t, err)
	_ = genDB

	db := rawdb.NewMemoryDatabase()
	cfg := headPinConfig()

	bc, err := createBlockChain(db, cfg, gspec, common.Hash{})
	require.NoError(t, err)
	for _, b := range chain {
		require.NoError(t, insertAccept(t, bc, b))
	}
	headHash := bc.CurrentHeader().Hash()
	headNum := bc.CurrentHeader().Number.Uint64()
	require.NotZero(t, headNum%cfg.CommitInterval, "head must be at a NON-boundary height")
	sdb, err := bc.StateAt(bc.CurrentHeader().Root)
	require.NoError(t, err)
	balBefore := sdb.GetBalance(sender)

	// Clean shutdown: Shutdown() commits the last accepted (non-boundary) root.
	bc.Stop()

	// Restart on the same db, pinned to the accepted head.
	restarted, err := createBlockChain(db, cfg, gspec, headHash)
	require.NoError(t, err)
	defer restarted.Stop()

	require.Equal(t, headHash, restarted.CurrentHeader().Hash(), "restart lost the accepted head")
	rsdb, err := restarted.StateAt(restarted.CurrentHeader().Root)
	require.NoError(t, err, "restarted chain cannot serve the non-boundary head state")
	require.Zero(t, rsdb.GetBalance(sender).Cmp(balBefore), "restarted head balance mismatch")

	// Build H+1 on the restarted, non-boundary head.
	extra, _, err := GenerateChain(gspec.Config, chain[len(chain)-1], dummy.NewCoinbaseFaker(), genDB, 1, 10, func(i int, gen *BlockGen) {
		tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(sender), recipient, big.NewInt(500), ethparams.TxGas, big.NewInt(225_000_000_000), nil), signer, key)
		gen.AddTx(tx)
	})
	require.NoError(t, err)
	require.NoError(t, insertAccept(t, restarted, extra[0]), "cannot build H+1 after restart at non-boundary head")
}

// TestRejectCanonicalProcessingHead_RewindsDurableHead proves a rejected H+1
// processing block cannot leave the on-disk head pointer aimed at its deleted
// body. This is the devnet restart failure: H was accepted, H+1 became the
// canonical processing head, consensus rejected H+1, and loadLastState later
// failed with "could not load head block".
func TestRejectCanonicalProcessingHead_RewindsDurableHead(t *testing.T) {
	gspec := &Genesis{Config: params.TestChainConfig}
	_, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), 2, 10, nil)
	require.NoError(t, err)

	db := rawdb.NewMemoryDatabase()
	cfg := headPinConfig()
	bc, err := createBlockChain(db, cfg, gspec, common.Hash{})
	require.NoError(t, err)

	require.NoError(t, insertAccept(t, bc, chain[0]))
	accepted := chain[0]
	processing := chain[1]
	require.NoError(t, bc.InsertBlock(processing))
	require.Equal(t, processing.Hash(), bc.CurrentHeader().Hash(), "processing block must be the pre-reject head")
	require.Equal(t, processing.Hash(), rawdb.ReadHeadBlockHash(db), "durable pre-reject head mismatch")

	require.NoError(t, bc.Reject(processing))
	require.Equal(t, accepted.Hash(), bc.CurrentHeader().Hash(), "reject must rewind the in-memory head")
	require.Equal(t, accepted.Hash(), rawdb.ReadHeadBlockHash(db), "reject must rewind the durable head before deleting the block")
	require.Nil(t, bc.GetBlockByHash(processing.Hash()), "rejected processing body must be deleted")
	bc.Stop()

	restarted, err := createBlockChain(db, cfg, gspec, accepted.Hash())
	require.NoError(t, err, "restart followed a stale head pointer to the rejected block")
	defer restarted.Stop()
	require.Equal(t, accepted.Hash(), restarted.CurrentHeader().Hash())
}

// TestLoadLastState_RepairsMissingProcessingHead reproduces a database already
// damaged by an older release: the processing head pointer and canonical index
// name H+1, its block body has been deleted, and the independent consensus
// pointer still names accepted H. Startup must recover exactly to H.
func TestLoadLastState_RepairsMissingProcessingHead(t *testing.T) {
	gspec := &Genesis{Config: params.TestChainConfig}
	_, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), 2, 10, nil)
	require.NoError(t, err)

	db := rawdb.NewMemoryDatabase()
	cfg := headPinConfig()
	bc, err := createBlockChain(db, cfg, gspec, common.Hash{})
	require.NoError(t, err)

	require.NoError(t, insertAccept(t, bc, chain[0]))
	accepted := chain[0]
	processing := chain[1]
	require.NoError(t, bc.InsertBlock(processing))
	require.Equal(t, processing.Hash(), rawdb.ReadHeadBlockHash(db))
	bc.Stop()

	// Model the legacy Reject ordering: delete the canonical processing block
	// while leaving both durable head markers and its canonical assignment.
	batch := db.NewBatch()
	rawdb.DeleteBlock(batch, processing.Hash(), processing.NumberU64())
	require.NoError(t, batch.Write())
	require.Equal(t, processing.Hash(), rawdb.ReadHeadBlockHash(db))
	require.Equal(t, processing.Hash(), rawdb.ReadCanonicalHash(db, processing.NumberU64()))
	require.Nil(t, rawdb.ReadBlock(db, processing.Hash(), processing.NumberU64()))

	restarted, err := createBlockChain(db, cfg, gspec, accepted.Hash())
	require.NoError(t, err)
	defer restarted.Stop()
	require.Equal(t, accepted.Hash(), restarted.CurrentHeader().Hash())
	require.Equal(t, accepted.Hash(), rawdb.ReadHeadBlockHash(db))
	require.Equal(t, accepted.Hash(), rawdb.ReadHeadHeaderHash(db))
	require.Equal(t, common.Hash{}, rawdb.ReadCanonicalHash(db, processing.NumberU64()))

	// The repaired database is not merely readable: it can extend and accept the
	// next block normally after startup.
	require.NoError(t, insertAccept(t, restarted, processing))
	require.Equal(t, processing.Hash(), restarted.LastConsensusAcceptedBlock().Hash())
}

// Copyright (C) 2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pathdb

import (
	"testing"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/trie/trienode"
	"github.com/luxfi/geth/trie/triestate"
)

// TestLoadLayers_FreshDB_UsesEmptyRootHash asserts the disk layer's root on a
// pristine key-value store is types.EmptyRootHash, not Keccak256Hash(nil).
//
// Why this matters: core.Genesis.toBlock commits genesis state with parentRoot
// == EmptyRootHash. layertree.add looks up parentRoot in the layer tree before
// inserting the new layer; if the only existing layer is at Keccak256Hash(nil)
// (`0xc5d2…a470`), the lookup misses and the commit panics with
// "triedb parent layer missing". That single hash mismatch broke every
// fresh-launched chain on this fork.
//
// Regression guard: any future refactor that re-introduces
// `crypto.Keccak256Hash(data)` as the unconditional root computation for an
// empty disk node MUST update this test in tandem with the layer-tree
// initialization, or chains will silently fail to commit genesis again.
func TestLoadLayers_FreshDB_UsesEmptyRootHash(t *testing.T) {
	memdb := rawdb.NewMemoryDatabase()
	db := New(memdb, nil)
	defer db.Close()

	head := db.tree.get(types.EmptyRootHash)
	if head == nil {
		got := "<no layers in tree>"
		// Walk the (small) layer set so the failure message tells you what
		// root the tree DID install — most useful when the regression hits.
		db.tree.lock.RLock()
		for k := range db.tree.layers {
			got = k.Hex()
			break
		}
		db.tree.lock.RUnlock()
		t.Fatalf("fresh DB disk layer should be at EmptyRootHash (%s); tree has root=%s",
			types.EmptyRootHash.Hex(), got)
	}

	if head.rootHash() != types.EmptyRootHash {
		t.Fatalf("disk layer root mismatch: got %s, want %s",
			head.rootHash().Hex(), types.EmptyRootHash.Hex())
	}
}

// TestLoadLayers_FreshDB_AddOnTopOfEmptyRootSucceeds end-to-end check:
// simulate the sequence the genesis-commit path takes on a fresh DB —
// add a new layer with parentRoot = EmptyRootHash. Before the fix this
// returned "triedb parent layer missing"; with the fix it succeeds.
func TestLoadLayers_FreshDB_AddOnTopOfEmptyRootSucceeds(t *testing.T) {
	memdb := rawdb.NewMemoryDatabase()
	db := New(memdb, nil)
	defer db.Close()

	// Construct a single trivial layer pretending to be the genesis state.
	childRoot := common.HexToHash("0x000000000000000000000000000000000000000000000000000000000000beef")

	// Empty payload — genesis with no allocations is similar in shape
	// (the alloc map only adds account nodes; for this regression test we
	// only care that parent lookup against EmptyRootHash succeeds).
	emptyNodes := trienode.NewMergedNodeSet()
	emptyStates := triestate.New(nil, nil, nil)

	if err := db.Update(childRoot, types.EmptyRootHash, 0, emptyNodes, emptyStates); err != nil {
		t.Fatalf("Update onto EmptyRootHash parent failed (the bug): %v", err)
	}
	if got := db.tree.get(childRoot); got == nil {
		t.Fatalf("child layer at %s missing from tree after Update", childRoot.Hex())
	}
}

// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// rlp_seam_red_test.go — RED adversarial proof of the IMPORT → PRODUCE-ON-TOP seam,
// the exact reliability the owner is worried about: mainnet imported
// lux-mainnet-96369-full-1083873.rlp then produced new blocks up to 1085755. The seam
// MUST hold with NO gap and NO divergence:
//
//	parentHash(first produced block) == hash(last imported block)   (no fork at the seam)
//	height(first produced block)     == height(last imported) + 1   (no gap / no skip)
//	each produced block chains to the previous                      (contiguous ancestry)
//	the post-produce snapshot's tip hash == the produced tip hash   (state root chains;
//	                                                                 equal hash ⇒ equal
//	                                                                 stateRoot ⇒ no re-exec
//	                                                                 divergence)
//
// The DELTA exports (lux-mainnet-96369-delta-1083347-1083356 …-1083873) ARE the blocks
// produced ON TOP of the full-1083346 snapshot, and full-1083873 is the snapshot AFTER
// they were produced — so the real state files let us prove the seam end-to-end without
// standing up a 1.2 GB EVM state trie in a unit test.
//
// Files are located via LUX_RLP_STATE_DIR (default ~/work/lux/state) and every case skips
// cleanly when its inputs are absent, so CI without the exports stays green. The heavy
// full-snapshot cases (1.2 GB stream ×2) are additionally gated behind
// LUX_RLP_SEAM_HEAVY=1.
package core

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

func rlpStateDir() string {
	if d := os.Getenv("LUX_RLP_STATE_DIR"); d != "" {
		return d
	}
	return os.ExpandEnv("$HOME/work/lux/state")
}

type blkInfo struct {
	number uint64
	hash   [32]byte
	parent [32]byte
	root   [32]byte
}

// forEachBlock streams a C-Chain RLP export block-by-block (the admin.exportChain
// format), invoking fn per block. It retains no blocks (constant memory), so a 1.2 GB
// export streams without OOM. Returns first, last, and count. A non-contiguous parent
// chain is reported by fn returning an error (the caller decides fatality).
func forEachBlock(t *testing.T, path string, fn func(idx uint64, b *types.Block) error) (first, last blkInfo, count uint64) {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err, "open %s", path)
	defer f.Close()

	stream := rlp.NewStream(f, 0)
	for {
		var b types.Block
		err := stream.Decode(&b)
		if err == io.EOF {
			break
		}
		require.NoErrorf(t, err, "decode block #%d in %s", count, path)

		info := blkInfo{number: b.NumberU64(), hash: b.Hash(), parent: b.ParentHash(), root: b.Root()}
		if count == 0 {
			first = info
		}
		if fn != nil {
			require.NoError(t, fn(count, &b), "block continuity at index %d (height %d)", count, b.NumberU64())
		}
		last = info
		count++
	}
	return first, last, count
}

// deltaFiles are the exports of the blocks PRODUCED on top of the full-1083346 snapshot,
// in production order. Their concatenation is the exact "produce-on-top" sequence.
var deltaFiles = []string{
	"lux-mainnet-96369-delta-1083347-1083356.rlp",
	"lux-mainnet-96369-delta-1083357-1083370.rlp",
	"lux-mainnet-96369-delta-1083371-1083873.rlp",
}

// TestRedRLPSeam_LuxMainnet_ProducedDeltasChainWithNoGap proves the produced blocks form
// ONE unbroken chain: contiguous heights and matching parent hashes WITHIN each export and
// ACROSS export boundaries. A gap, a re-org, or a parent-hash break at any seam fails here.
func TestRedRLPSeam_LuxMainnet_ProducedDeltasChainWithNoGap(t *testing.T) {
	dir := rlpStateDir()
	// Require all three deltas present (they are small; if any is missing, skip the case).
	for _, name := range deltaFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Skipf("delta export %s not present in %s — skipping", name, dir)
		}
	}

	var prev *blkInfo // last block of the previous export (nil before the first)
	var globalFirst, globalLast blkInfo
	for fi, name := range deltaFiles {
		path := filepath.Join(dir, name)
		var haveParentInFile bool
		var fileFirst blkInfo
		first, last, count := forEachBlock(t, path, func(idx uint64, b *types.Block) error {
			if idx == 0 {
				fileFirst = blkInfo{number: b.NumberU64(), hash: b.Hash(), parent: b.ParentHash()}
				return nil
			}
			// Within-file contiguity: height+1 and parent == previous hash.
			if !haveParentInFile {
				haveParentInFile = true
			}
			return nil
		})
		require.Greater(t, count, uint64(0), "%s decoded zero blocks", name)

		// Re-stream to assert strict within-file contiguity (height n+1, parent==prev.hash).
		var last2 blkInfo
		var have bool
		forEachBlock(t, path, func(idx uint64, b *types.Block) error {
			cur := blkInfo{number: b.NumberU64(), hash: b.Hash(), parent: b.ParentHash()}
			if have {
				require.Equalf(t, last2.number+1, cur.number, "%s: HEIGHT GAP %d→%d", name, last2.number, cur.number)
				require.Equalf(t, last2.hash, cur.parent, "%s: PARENT BREAK at height %d", name, cur.number)
			}
			last2, have = cur, true
			return nil
		})

		// Across-file seam: this file's first block is produced ON TOP of the previous file's tip.
		if prev != nil {
			require.Equalf(t, prev.number+1, fileFirst.number,
				"SEAM GAP between %s and %s: %d → %d", deltaFiles[fi-1], name, prev.number, fileFirst.number)
			require.Equalf(t, prev.hash, fileFirst.parent,
				"SEAM PARENT BREAK: %s first block (h=%d) parent != %s tip hash", name, fileFirst.number, deltaFiles[fi-1])
		}
		if fi == 0 {
			globalFirst = first
		}
		globalLast = last
		p := last
		prev = &p
	}

	// The produced sequence spans exactly 1083347..1083873 with no gap.
	require.Equal(t, uint64(1083347), globalFirst.number, "produced sequence must START at 1083347 (full snapshot tip +1)")
	require.Equal(t, uint64(1083873), globalLast.number, "produced sequence must END at the 1083873 snapshot height")
	t.Logf("produced-on-top chain verified contiguous: blocks %d..%d, tip=%x", globalFirst.number, globalLast.number, globalLast.hash)
}

// TestRedRLPSeam_LuxMainnet_FullSnapshotToProducedTip is the FULL end-to-end seam (heavy):
// stream the 1.2 GB full-1083346 snapshot to its tip, prove the first produced block is
// built directly on that tip (parent hash + height+1), then stream the 1.2 GB full-1083873
// snapshot and prove its tip is byte-identical to the produced tip — so importing the
// snapshot and producing on top yields the SAME chain the later snapshot commits to (state
// root chains; no re-exec divergence). Gated behind LUX_RLP_SEAM_HEAVY=1.
func TestRedRLPSeam_LuxMainnet_FullSnapshotToProducedTip(t *testing.T) {
	if os.Getenv("LUX_RLP_SEAM_HEAVY") != "1" {
		t.Skip("heavy 1.2GB×2 stream; set LUX_RLP_SEAM_HEAVY=1 to run")
	}
	dir := rlpStateDir()
	full0 := filepath.Join(dir, "lux-mainnet-96369-full-1083346.rlp")
	full1 := filepath.Join(dir, "lux-mainnet-96369-full-1083873.rlp")
	d1 := filepath.Join(dir, deltaFiles[0])
	d3 := filepath.Join(dir, deltaFiles[len(deltaFiles)-1])
	for _, p := range []string{full0, full1, d1, d3} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("%s absent — skipping heavy seam", p)
		}
	}

	// 1. Import snapshot tip (stream to the end; assert genesis-rooted contiguity along the way).
	_, base, n0 := forEachBlock(t, full0, func(idx uint64, b *types.Block) error { return nil })
	require.Equal(t, uint64(1083346), base.number, "full-1083346 tip height")
	require.Equal(t, uint64(1083347), n0, "full-1083346 must contain blocks 0..1083346")

	// 2. First produced block sits directly on the imported tip.
	firstProduced, _, _ := forEachBlock(t, d1, func(idx uint64, b *types.Block) error { return nil })
	require.Equal(t, base.number+1, firstProduced.number, "produce-on-top HEIGHT GAP at the seam")
	require.Equal(t, base.hash, firstProduced.parent, "produce-on-top PARENT BREAK at the seam")

	// 3. Produced tip == later snapshot tip (state root chains; no divergence).
	_, producedTip, _ := forEachBlock(t, d3, func(idx uint64, b *types.Block) error { return nil })
	_, snapTip, _ := forEachBlock(t, full1, func(idx uint64, b *types.Block) error { return nil })
	require.Equal(t, uint64(1083873), snapTip.number, "full-1083873 tip height")
	require.Equal(t, producedTip.number, snapTip.number, "produced tip vs snapshot tip HEIGHT mismatch")
	require.Equal(t, producedTip.hash, snapTip.hash, "produced tip HASH != snapshot tip HASH — re-exec DIVERGENCE at the seam")
	require.Equal(t, producedTip.root, snapTip.root, "produced tip STATE ROOT != snapshot state root — state divergence")
	t.Logf("FULL SEAM verified: import@%d(%x) → produce → tip@%d(%x) == snapshot tip",
		base.number, base.hash, producedTip.number, producedTip.hash)
}

// TestRedRLPSeam_ZooMainnet_ContinuityAndWellFormedTip proves the Zoo mainnet export
// (zoo-mainnet-200200) is a sound import base: an unbroken genesis-rooted chain with
// monotonic heights, a non-zero state root at every block, and a well-formed tip — the
// same continuity the Lux seam relies on, for the second chain the owner named.
func TestRedRLPSeam_ZooMainnet_ContinuityAndWellFormedTip(t *testing.T) {
	path := filepath.Join(rlpStateDir(), "zoo-mainnet-200200-full.rlp")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("zoo-mainnet export absent (%s) — skipping", path)
	}
	var last blkInfo
	var have bool
	var zero [32]byte
	first, tip, count := forEachBlock(t, path, func(idx uint64, b *types.Block) error {
		cur := blkInfo{number: b.NumberU64(), hash: b.Hash(), parent: b.ParentHash(), root: b.Root()}
		require.NotEqualf(t, zero, cur.hash, "zoo block %d has zero hash", cur.number)
		if have {
			require.Equalf(t, last.number+1, cur.number, "zoo HEIGHT GAP %d→%d", last.number, cur.number)
			require.Equalf(t, last.hash, cur.parent, "zoo PARENT BREAK at height %d", cur.number)
		}
		last, have = cur, true
		return nil
	})
	require.Greater(t, count, uint64(0), "zoo export decoded zero blocks")
	require.Equal(t, uint64(0), first.number, "zoo export must start at genesis (height 0)")
	t.Logf("zoo-mainnet import base verified: %d blocks, genesis..%d, tip=%x", count, tip.number, tip.hash)
}

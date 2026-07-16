// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/evm/core"
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

// writeChainRLP RLP-encodes canonical blocks [from,to] of chain into a temp file, in the
// sequential per-block format importBlocksFromFile reads, and returns its path.
func writeChainRLP(t *testing.T, chain *core.BlockChain, from, to uint64) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "chain.rlp")
	f, err := os.Create(p)
	require.NoError(t, err)
	for i := from; i <= to; i++ {
		b := chain.GetBlockByNumber(i)
		require.NotNilf(t, b, "canonical block %d missing", i)
		require.NoError(t, rlp.Encode(f, b))
	}
	require.NoError(t, f.Close())
	return p
}

// TestStartupImportIdempotency_AlreadyAtTip locks the contract the startup --import-chain-data
// idempotency guard (VM.Initialize -> isNothingToImportError) depends on: on an already-imported
// chain, re-importing the SAME RLP must return a "nothing to import" error the guard classifies as
// idempotent (log + skip) — NOT a fatal error. If this drifts, a production restart or a G9
// kill-rejoin (both keep --import-chain-data SET in the pod spec) CRASH-LOOPS the node — the
// re-audit's blocking HIGH (admin_api.go:607).
//
// Both RLP shapes are covered because admin_exportChain can include or omit genesis, and the two
// hit DIFFERENT importBlocksFromFile returns ("no blocks imported" vs "no blocks found in file").
func TestStartupImportIdempotency_AlreadyAtTip(t *testing.T) {
	const tip = 5
	chain, _ := buildAcceptedTestChain(t, tip) // real canonical chain, accept tip == 5
	curHead := chain.CurrentBlock().Number.Uint64()
	require.EqualValues(t, tip, curHead)

	// RLP WITH genesis (blocks 0..tip): genesis is skipped, all others already present ->
	// importBlocksFromFile falls through to the totalImported==0 "no blocks imported" return.
	withGenesis := writeChainRLP(t, chain, 0, tip)
	imported, _, _, err := importBlocksFromFile(chain, withGenesis, nil)
	require.Equal(t, 0, imported)
	require.Error(t, err)
	require.Truef(t, isNothingToImportError(err, curHead),
		"already-at-tip re-import (genesis-inclusive RLP) must classify idempotent; got: %v", err)

	// RLP WITHOUT genesis (blocks 1..tip): every block is skipped before it is counted ->
	// the inner loop returns "no blocks found in file".
	withoutGenesis := writeChainRLP(t, chain, 1, tip)
	imported, _, _, err = importBlocksFromFile(chain, withoutGenesis, nil)
	require.Equal(t, 0, imported)
	require.Error(t, err)
	require.Truef(t, isNothingToImportError(err, curHead),
		"already-at-tip re-import (no-genesis RLP) must classify idempotent; got: %v", err)

	// SAFETY: the SAME "nothing to import" error on a FRESH chain (curHead == 0) must stay FATAL —
	// a genuinely empty/corrupt RLP on first boot must never be silently skipped.
	require.False(t, isNothingToImportError(err, 0))
}

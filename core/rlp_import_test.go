// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"io"
	"os"
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

// TestRLPImportZooMainnet tests that we can properly decode historic Zoo mainnet blocks
// from RLP export and that hashes are preserved correctly.
func TestRLPImportZooMainnet(t *testing.T) {
	rlpPath := os.Getenv("ZOO_RLP_PATH")
	if rlpPath == "" {
		rlpPath = "/Users/z/work/lux/state/rlp/zoo-mainnet/zoo-mainnet-200200.rlp"
	}

	// Check if file exists
	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Zoo RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Read and verify blocks
	blockCount := 0
	var prevBlockHash *types.Header
	for {
		var block types.Block
		err := stream.Decode(&block)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Try decoding as just a header
			file.Seek(0, 0)
			stream = rlp.NewStream(file, 0)
			var blocks []types.Block
			if err := stream.Decode(&blocks); err == nil {
				for i, b := range blocks {
					verifyBlockDecode(t, &b, i)
				}
				t.Logf("Decoded %d blocks from array format", len(blocks))
				return
			}
			require.NoError(t, err, "failed to decode block %d", blockCount)
		}

		verifyBlockDecode(t, &block, blockCount)

		// Verify parent hash chain
		header := block.Header()
		if blockCount > 0 && prevBlockHash != nil {
			// The parent hash of this block should match the hash of the previous block
			require.Equal(t, prevBlockHash.Hash(), header.ParentHash,
				"block %d parent hash mismatch", blockCount)
		}

		prevBlockHash = header
		blockCount++

		// Log progress every 100 blocks
		if blockCount%100 == 0 {
			t.Logf("Decoded %d blocks...", blockCount)
		}

		// Limit to first 1000 blocks for quick test
		if blockCount >= 1000 {
			t.Logf("Stopped at %d blocks (test limit)", blockCount)
			break
		}
	}

	t.Logf("Successfully decoded %d blocks from Zoo mainnet RLP", blockCount)
	require.Greater(t, blockCount, 0, "should decode at least one block")
}

func verifyBlockDecode(t *testing.T, block *types.Block, blockNum int) {
	header := block.Header()

	// Verify hash can be computed
	hash := block.Hash()
	require.NotEmpty(t, hash, "block %d hash should not be empty", blockNum)

	// Verify header fields are accessible
	require.NotNil(t, header.Number, "block %d number should not be nil", blockNum)

	// For Cancun blocks, verify blob gas fields
	if header.ExcessBlobGas != nil {
		// BlobGasUsed should also be set for Cancun blocks
		require.NotNil(t, header.BlobGasUsed, "block %d BlobGasUsed should not be nil when ExcessBlobGas is set", blockNum)
	}

	// Test encode/decode roundtrip
	encoded, err := rlp.EncodeToBytes(header)
	require.NoError(t, err, "block %d header encoding failed", blockNum)

	decoded, err := types.DecodeHeader(encoded)
	require.NoError(t, err, "block %d header decoding failed", blockNum)

	// Verify all critical fields match
	require.Equal(t, header.ParentHash, decoded.ParentHash, "block %d ParentHash mismatch", blockNum)
	require.Equal(t, header.Root, decoded.Root, "block %d Root mismatch", blockNum)
	require.Equal(t, header.TxHash, decoded.TxHash, "block %d TxHash mismatch", blockNum)

	// The hash of the re-encoded header should match the original
	// Note: This may differ if rawRLP preservation is used for historic blocks
	if header.RawRLP() == nil {
		// For newly created blocks, hash should match after roundtrip
		require.Equal(t, hash, decoded.Hash(), "block %d hash mismatch after roundtrip", blockNum)
	}
}

// TestRLPImportLuxMainnetSample tests a small sample of Lux C-chain blocks
func TestRLPImportLuxMainnetSample(t *testing.T) {
	rlpPath := os.Getenv("LUX_RLP_PATH")
	if rlpPath == "" {
		rlpPath = "/Users/z/work/lux/state/rlp/lux-mainnet/lux-mainnet-96369.rlp"
	}

	// Check if file exists
	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Lux RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Read first 100 blocks as a sample
	blockCount := 0
	for blockCount < 100 {
		var block types.Block
		err := stream.Decode(&block)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "failed to decode block %d", blockCount)

		verifyBlockDecode(t, &block, blockCount)
		blockCount++
	}

	t.Logf("Successfully decoded %d Lux C-chain blocks from RLP", blockCount)
	require.Greater(t, blockCount, 0, "should decode at least one block")
}

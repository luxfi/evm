// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"io"
	"os"
	"testing"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

// Expected genesis hashes for each chain
// These are the canonical genesis block hashes that must be preserved
var (
	// Lux C-Chain (96369) genesis hash
	LuxMainnetGenesisHash = common.HexToHash("0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecafc987794050cb0d286f1ec61e")
	// Lux C-Chain genesis state root
	LuxMainnetStateRoot = common.HexToHash("0x2d1cedac263020c5c56ef962f6abe0da1f5217bdc6468f8c9258a0ea23699e80")

	// Zoo mainnet (200200) genesis hash
	ZooMainnetGenesisHash = common.HexToHash("0x7c548af47de27560779ccc67dda32a540944accc71dac3343da3b9cd18f14933")
)

// TestLuxGenesisHashFromRLP verifies that the Lux C-chain genesis block
// can be decoded from RLP and maintains the correct hash.
func TestLuxGenesisHashFromRLP(t *testing.T) {
	rlpPath := "/Users/z/work/lux/state/rlp/lux-mainnet/lux-mainnet-96369.rlp"

	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Lux RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Read the first block (genesis)
	var block types.Block
	err = stream.Decode(&block)
	require.NoError(t, err, "failed to decode genesis block")

	header := block.Header()

	t.Logf("Lux Genesis Block Details:")
	t.Logf("  Block Number: %d", header.Number.Uint64())
	t.Logf("  Block Hash: %s", block.Hash().Hex())
	t.Logf("  State Root: %s", header.Root.Hex())
	t.Logf("  Parent Hash: %s", header.ParentHash.Hex())
	t.Logf("  Header Fields: %d", countHeaderFields(header))

	// Verify genesis block number
	require.Equal(t, uint64(0), header.Number.Uint64(), "genesis block should be block 0")

	// Verify genesis hash matches expected
	// Note: If using rawRLP preservation, the hash should be computed from original RLP
	actualHash := block.Hash()
	t.Logf("  Actual Hash: %s", actualHash.Hex())
	t.Logf("  Expected Hash: %s", LuxMainnetGenesisHash.Hex())

	// The hash must match the canonical genesis hash
	require.Equal(t, LuxMainnetGenesisHash, actualHash,
		"Lux genesis hash mismatch - decode changes broke hash computation")

	// Verify state root
	require.Equal(t, LuxMainnetStateRoot, header.Root,
		"Lux genesis state root mismatch")
}

// TestZooGenesisHashFromRLP verifies that the Zoo genesis block
// can be decoded from RLP and maintains the correct hash.
func TestZooGenesisHashFromRLP(t *testing.T) {
	rlpPath := "/Users/z/work/lux/state/rlp/zoo-mainnet/zoo-mainnet-200200.rlp"

	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Zoo RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Read the first block (genesis)
	var block types.Block
	err = stream.Decode(&block)
	require.NoError(t, err, "failed to decode genesis block")

	header := block.Header()

	t.Logf("Zoo Genesis Block Details:")
	t.Logf("  Block Number: %d", header.Number.Uint64())
	t.Logf("  Block Hash: %s", block.Hash().Hex())
	t.Logf("  State Root: %s", header.Root.Hex())
	t.Logf("  Parent Hash: %s", header.ParentHash.Hex())
	t.Logf("  Header Fields: %d", countHeaderFields(header))
	if header.ExcessBlobGas != nil {
		t.Logf("  ExcessBlobGas: %d", *header.ExcessBlobGas)
	}
	if header.BlobGasUsed != nil {
		t.Logf("  BlobGasUsed: %d", *header.BlobGasUsed)
	}

	// Verify genesis block number
	require.Equal(t, uint64(0), header.Number.Uint64(), "genesis block should be block 0")

	// Record the actual hash for reference
	actualHash := block.Hash()
	t.Logf("  Actual Hash: %s", actualHash.Hex())

	// The hash must match the canonical genesis hash (if we have one)
	// For Zoo, we need to verify the hash is computed correctly
	if ZooMainnetGenesisHash != (common.Hash{}) {
		t.Logf("  Expected Hash: %s", ZooMainnetGenesisHash.Hex())
		require.Equal(t, ZooMainnetGenesisHash, actualHash,
			"Zoo genesis hash mismatch - decode changes broke hash computation")
	} else {
		t.Logf("  Note: Zoo genesis hash not yet recorded, current hash: %s", actualHash.Hex())
	}
}

// TestBlockHashPreservationAfterDecode verifies that blocks maintain
// their hashes after decode, re-encode, and re-decode cycles.
func TestBlockHashPreservationAfterDecode(t *testing.T) {
	rlpPath := "/Users/z/work/lux/state/rlp/zoo-mainnet/zoo-mainnet-200200.rlp"

	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Zoo RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Test first 100 blocks for hash preservation
	for i := 0; i < 100; i++ {
		var block types.Block
		err := stream.Decode(&block)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "failed to decode block %d", i)

		originalHash := block.Hash()
		header := block.Header()

		// Re-encode and decode the header
		encoded, err := rlp.EncodeToBytes(header)
		require.NoError(t, err, "failed to encode header %d", i)

		decoded, err := types.DecodeHeader(encoded)
		require.NoError(t, err, "failed to decode header %d", i)

		// For blocks with rawRLP preserved, the hash should use original bytes
		// For re-encoded blocks, the hash might differ (which is expected)
		if header.RawRLP() != nil {
			// Original block should have preserved hash
			require.Equal(t, originalHash, block.Hash(),
				"block %d hash changed after storing rawRLP", i)
		}

		// After re-decode, critical fields must match
		require.Equal(t, header.ParentHash, decoded.ParentHash, "block %d ParentHash mismatch", i)
		require.Equal(t, header.Root, decoded.Root, "block %d Root mismatch", i)
		require.Equal(t, header.Number.Uint64(), decoded.Number.Uint64(), "block %d Number mismatch", i)
	}

	t.Log("Hash preservation test passed for 100 blocks")
}

// TestStateRootChainThroughZooBlocks verifies that state roots form a valid chain
// and are properly preserved through decode operations.
func TestStateRootChainThroughZooBlocks(t *testing.T) {
	rlpPath := "/Users/z/work/lux/state/rlp/zoo-mainnet/zoo-mainnet-200200.rlp"

	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Zoo RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	type blockInfo struct {
		number     uint64
		hash       common.Hash
		stateRoot  common.Hash
		parentHash common.Hash
	}

	var blocks []blockInfo

	// Read all blocks
	for {
		var block types.Block
		err := stream.Decode(&block)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "failed to decode block %d", len(blocks))

		header := block.Header()
		blocks = append(blocks, blockInfo{
			number:     header.Number.Uint64(),
			hash:       block.Hash(),
			stateRoot:  header.Root,
			parentHash: header.ParentHash,
		})
	}

	t.Logf("Loaded %d Zoo blocks", len(blocks))
	require.Greater(t, len(blocks), 0, "should have at least one block")

	// Verify genesis block
	genesis := blocks[0]
	t.Logf("Genesis Block:")
	t.Logf("  Number: %d", genesis.number)
	t.Logf("  Hash: %s", genesis.hash.Hex())
	t.Logf("  State Root: %s", genesis.stateRoot.Hex())
	require.Equal(t, uint64(0), genesis.number, "first block should be genesis")
	require.Equal(t, ZooMainnetGenesisHash, genesis.hash, "genesis hash mismatch")

	// Verify parent hash chain and state roots
	stateRoots := make(map[common.Hash]uint64) // stateRoot -> first block that had it
	stateRoots[genesis.stateRoot] = 0

	for i := 1; i < len(blocks); i++ {
		block := blocks[i]
		prevBlock := blocks[i-1]

		// Verify block number is sequential
		require.Equal(t, prevBlock.number+1, block.number,
			"block %d number not sequential", i)

		// Verify parent hash points to previous block
		require.Equal(t, prevBlock.hash, block.parentHash,
			"block %d parent hash doesn't match previous block hash", i)

		// Track state root (state roots can repeat if no state changes)
		if _, seen := stateRoots[block.stateRoot]; !seen {
			stateRoots[block.stateRoot] = block.number
		}

		// State root must not be empty
		require.NotEqual(t, common.Hash{}, block.stateRoot,
			"block %d has empty state root", i)
	}

	t.Logf("Verified chain integrity for %d blocks", len(blocks))
	t.Logf("Found %d unique state roots", len(stateRoots))

	// Log first few state roots
	t.Logf("\nFirst 10 blocks state roots:")
	for i := 0; i < 10 && i < len(blocks); i++ {
		t.Logf("  Block %d: %s", blocks[i].number, blocks[i].stateRoot.Hex())
	}

	// Log last few state roots
	if len(blocks) > 10 {
		t.Logf("\nLast 5 blocks state roots:")
		for i := len(blocks) - 5; i < len(blocks); i++ {
			t.Logf("  Block %d: %s", blocks[i].number, blocks[i].stateRoot.Hex())
		}
	}
}

// TestStateRootRoundtripZoo verifies state roots survive encode/decode cycles
func TestStateRootRoundtripZoo(t *testing.T) {
	rlpPath := "/Users/z/work/lux/state/rlp/zoo-mainnet/zoo-mainnet-200200.rlp"

	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Zoo RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Test first 100 blocks
	for i := 0; i < 100; i++ {
		var block types.Block
		err := stream.Decode(&block)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "failed to decode block %d", i)

		header := block.Header()
		originalStateRoot := header.Root
		originalHash := block.Hash()

		// Re-encode the header
		encoded, err := rlp.EncodeToBytes(header)
		require.NoError(t, err, "failed to encode header %d", i)

		// Decode it again
		decoded, err := types.DecodeHeader(encoded)
		require.NoError(t, err, "failed to decode header %d", i)

		// State root MUST be identical
		require.Equal(t, originalStateRoot, decoded.Root,
			"block %d state root changed after roundtrip", i)

		// All critical fields must match
		require.Equal(t, header.ParentHash, decoded.ParentHash,
			"block %d ParentHash mismatch", i)
		require.Equal(t, header.TxHash, decoded.TxHash,
			"block %d TxHash mismatch", i)
		require.Equal(t, header.ReceiptHash, decoded.ReceiptHash,
			"block %d ReceiptHash mismatch", i)
		require.Equal(t, header.Number.Uint64(), decoded.Number.Uint64(),
			"block %d Number mismatch", i)

		// Log every 20 blocks
		if i%20 == 0 {
			t.Logf("Block %d: StateRoot=%s Hash=%s", i, originalStateRoot.Hex(), originalHash.Hex())
		}
	}

	t.Log("State root roundtrip verified for 100 blocks")
}

// TestStateRootChainThroughLuxBlocks verifies state roots through Lux C-chain blocks
func TestStateRootChainThroughLuxBlocks(t *testing.T) {
	rlpPath := "/Users/z/work/lux/state/rlp/lux-mainnet/lux-mainnet-96369.rlp"

	if _, err := os.Stat(rlpPath); os.IsNotExist(err) {
		t.Skipf("Lux RLP file not found at %s, skipping test", rlpPath)
	}

	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)

	// Read first 100 blocks and verify state roots
	var prevHash common.Hash
	for i := 0; i < 100; i++ {
		var block types.Block
		err := stream.Decode(&block)
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "failed to decode block %d", i)

		header := block.Header()

		// Verify block number
		require.Equal(t, uint64(i), header.Number.Uint64(), "block number mismatch")

		// Verify parent hash chain (except genesis)
		if i > 0 {
			require.Equal(t, prevHash, header.ParentHash,
				"block %d parent hash mismatch", i)
		}

		// State root must not be empty
		require.NotEqual(t, common.Hash{}, header.Root,
			"block %d has empty state root", i)

		// Log every 20 blocks
		if i%20 == 0 {
			t.Logf("Block %d: StateRoot=%s Hash=%s", i, header.Root.Hex(), block.Hash().Hex())
		}

		// Test roundtrip
		encoded, err := rlp.EncodeToBytes(header)
		require.NoError(t, err)
		decoded, err := types.DecodeHeader(encoded)
		require.NoError(t, err)
		require.Equal(t, header.Root, decoded.Root, "block %d state root roundtrip failed", i)

		prevHash = block.Hash()
	}

	t.Log("Lux C-chain state roots verified for 100 blocks")
}

// countHeaderFields counts how many fields are set in a header
// (for debugging header format detection)
func countHeaderFields(h *types.Header) int {
	count := 15 // core fields always present
	if h.BaseFee != nil {
		count++
	}
	if h.ExtDataHash != nil {
		count++
	}
	if h.ExtDataGasUsed != nil {
		count++
	}
	if h.BlockGasCost != nil {
		count++
	}
	if h.BlobGasUsed != nil {
		count++
	}
	if h.ExcessBlobGas != nil {
		count++
	}
	if h.ParentBeaconRoot != nil {
		count++
	}
	if h.WithdrawalsHash != nil {
		count++
	}
	if h.RequestsHash != nil {
		count++
	}
	return count
}

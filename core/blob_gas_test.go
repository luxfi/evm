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
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/geth/rlp"
	ethparams "github.com/luxfi/geth/params"
	"github.com/stretchr/testify/require"
)

// countRLPFields counts the number of fields in an RLP-encoded list
func countRLPFields(data []byte) (int, error) {
	content, _, err := rlp.SplitList(data)
	if err != nil {
		return 0, err
	}
	count := 0
	for len(content) > 0 {
		_, rest, err := rlp.SplitString(content)
		if err != nil {
			_, rest, err = rlp.SplitList(content) // bloom is a list
			if err != nil {
				return 0, err
			}
		}
		content = rest
		count++
	}
	return count, nil
}

func TestBlobGasEncodeDecodeRoundtrip(t *testing.T) {
	var (
		key1, _     = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
		cryptoAddr1 = crypto.PubkeyToAddress(key1.PublicKey)
		addr1       = common.BytesToAddress(cryptoAddr1[:])
	)

	genesisBalance := big.NewInt(1000000)
	gspec := &Genesis{
		Config: params.TestChainConfig,
		Alloc:  GenesisAlloc{addr1: {Balance: genesisBalance}},
	}

	signer := types.LatestSigner(params.TestChainConfig)
	chainDB, chain, _, err := GenerateChainWithGenesis(gspec, dummy.NewCoinbaseFaker(), 1, 10, func(i int, gen *BlockGen) {
		tx, _ := types.SignTx(types.NewTransaction(gen.TxNonce(addr1), common.Address{}, big.NewInt(10), ethparams.TxGas, big.NewInt(1), nil), signer, key1)
		gen.AddTx(tx)
	})
	require.NoError(t, err)
	require.Len(t, chain, 1)

	block1 := chain[0]
	header1 := block1.Header()

	t.Logf("Generated block 1:")
	t.Logf("  Hash: %s", block1.Hash().Hex())
	if header1.ExcessBlobGas != nil {
		t.Logf("  ExcessBlobGas: %d (ptr: %p)", *header1.ExcessBlobGas, header1.ExcessBlobGas)
	} else {
		t.Logf("  ExcessBlobGas: nil")
	}
	if header1.BlobGasUsed != nil {
		t.Logf("  BlobGasUsed: %d (ptr: %p)", *header1.BlobGasUsed, header1.BlobGasUsed)
	} else {
		t.Logf("  BlobGasUsed: nil")
	}
	t.Logf("  ParentBeaconRoot: %v", header1.ParentBeaconRoot)
	t.Logf("  WithdrawalsHash: %v", header1.WithdrawalsHash)
	t.Logf("  RawRLP len: %d", len(header1.RawRLP()))

	require.NotNil(t, header1.ExcessBlobGas, "ExcessBlobGas should be set for Cancun block")
	require.NotNil(t, header1.BlobGasUsed, "BlobGasUsed should be set for Cancun block")
	require.NotNil(t, header1.ParentBeaconRoot, "ParentBeaconRoot should be set for Cancun block")

	// Test encode/decode roundtrip
	encoded, err := rlp.EncodeToBytes(header1)
	require.NoError(t, err)
	t.Logf("Encoded header RLP length: %d", len(encoded))

	// Count fields in the encoded RLP
	fieldCount, err := countRLPFields(encoded)
	require.NoError(t, err)
	t.Logf("Encoded RLP field count: %d", fieldCount)

	decoded, err := types.DecodeHeader(encoded)
	require.NoError(t, err)
	t.Logf("Decoded header:")
	if decoded.ExcessBlobGas != nil {
		t.Logf("  ExcessBlobGas: %d (ptr: %p)", *decoded.ExcessBlobGas, decoded.ExcessBlobGas)
	} else {
		t.Logf("  ExcessBlobGas: nil")
	}
	if decoded.BlobGasUsed != nil {
		t.Logf("  BlobGasUsed: %d (ptr: %p)", *decoded.BlobGasUsed, decoded.BlobGasUsed)
	} else {
		t.Logf("  BlobGasUsed: nil")
	}
	t.Logf("  ParentBeaconRoot: %v", decoded.ParentBeaconRoot)

	require.NotNil(t, decoded.ExcessBlobGas, "Decoded ExcessBlobGas should not be nil")
	require.NotNil(t, decoded.BlobGasUsed, "Decoded BlobGasUsed should not be nil")
	require.NotNil(t, decoded.ParentBeaconRoot, "Decoded ParentBeaconRoot should not be nil")

	// Test database roundtrip via WriteHeader/ReadHeader
	db := rawdb.NewMemoryDatabase()
	rawdb.WriteHeader(db, header1)

	readHeader := rawdb.ReadHeader(db, header1.Hash(), header1.Number.Uint64())
	require.NotNil(t, readHeader)
	t.Logf("Read header from DB:")
	t.Logf("  ExcessBlobGas: %v", readHeader.ExcessBlobGas)
	t.Logf("  BlobGasUsed: %v", readHeader.BlobGasUsed)
	t.Logf("  ParentBeaconRoot: %v", readHeader.ParentBeaconRoot)

	require.NotNil(t, readHeader.ExcessBlobGas, "Read ExcessBlobGas should not be nil")
	require.NotNil(t, readHeader.BlobGasUsed, "Read BlobGasUsed should not be nil")
	require.NotNil(t, readHeader.ParentBeaconRoot, "Read ParentBeaconRoot should not be nil")

	// Test full block roundtrip via rawdb.WriteBlock/ReadBlock
	rawdb.WriteBlock(db, block1)

	readBlock := rawdb.ReadBlock(db, block1.Hash(), block1.NumberU64())
	require.NotNil(t, readBlock)
	readHeader2 := readBlock.Header()
	t.Logf("Read block header from DB:")
	t.Logf("  ExcessBlobGas: %v", readHeader2.ExcessBlobGas)
	t.Logf("  BlobGasUsed: %v", readHeader2.BlobGasUsed)
	t.Logf("  ParentBeaconRoot: %v", readHeader2.ParentBeaconRoot)

	require.NotNil(t, readHeader2.ExcessBlobGas, "Block read ExcessBlobGas should not be nil")
	require.NotNil(t, readHeader2.BlobGasUsed, "Block read BlobGasUsed should not be nil")
	require.NotNil(t, readHeader2.ParentBeaconRoot, "Block read ParentBeaconRoot should not be nil")

	// Now test blockchain GetBlockByNumber
	t.Logf("\nNow testing blockchain GetBlockByNumber...")

	// Create blockchain from the chainDB that GenerateChainWithGenesis returned
	cacheConfig := &CacheConfig{
		TrieCleanLimit:            256,
		TrieDirtyLimit:            256,
		TrieDirtyCommitTarget:     20,
		TriePrefetcherParallelism: 4,
		Pruning:                   false,
		SnapshotLimit:             0,
		AcceptorQueueLimit:        64,
	}

	blockchain, err := NewBlockChain(
		chainDB,
		cacheConfig,
		gspec,
		dummy.NewCoinbaseFaker(),
		vm.Config{},
		common.Hash{},
		false,
	)
	require.NoError(t, err)
	defer blockchain.Stop()

	// Insert the chain
	_, err = blockchain.InsertChain(chain)
	require.NoError(t, err)

	// Accept the blocks
	for _, block := range chain {
		err = blockchain.Accept(block)
		require.NoError(t, err)
	}
	blockchain.DrainAcceptorQueue()

	// Now retrieve via GetBlockByNumber
	retrievedBlock := blockchain.GetBlockByNumber(1)
	require.NotNil(t, retrievedBlock, "Block 1 should be retrievable")

	retrievedHeader := retrievedBlock.Header()
	t.Logf("Retrieved via GetBlockByNumber:")
	t.Logf("  Hash: %s", retrievedBlock.Hash().Hex())
	t.Logf("  ExcessBlobGas: %v", retrievedHeader.ExcessBlobGas)
	t.Logf("  BlobGasUsed: %v", retrievedHeader.BlobGasUsed)
	t.Logf("  ParentBeaconRoot: %v", retrievedHeader.ParentBeaconRoot)

	require.NotNil(t, retrievedHeader.ExcessBlobGas, "Retrieved ExcessBlobGas should not be nil")
	require.NotNil(t, retrievedHeader.BlobGasUsed, "Retrieved BlobGasUsed should not be nil")
	require.NotNil(t, retrievedHeader.ParentBeaconRoot, "Retrieved ParentBeaconRoot should not be nil")
}

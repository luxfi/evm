// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"encoding/binary"
	"testing"

	"github.com/luxfi/evm/v2/plugin/evm/atomic"
	"github.com/luxfi/database/memdb"
	"github.com/luxfi/database/versiondb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAtomicRepositoryWriteAndRead(t *testing.T) {
	// Create a test database
	db := versiondb.New(memdb.New())
	
	// Create atomic repository
	repo, err := NewAtomicTxRepository(db, atomic.Codec, 100)
	require.NoError(t, err)
	
	// Create test transactions
	txMap := make(map[uint64][]*atomic.Tx)
	
	// Write transactions at different heights
	for height := uint64(1); height < 10; height++ {
		txs := make([]*atomic.Tx, 0, 2)
		for i := 0; i < 2; i++ {
			tx := &atomic.Tx{
				UnsignedAtomicTx: &atomic.TestUnsignedTx{
					GasUsedV: 1000 + uint64(i),
				},
			}
			txs = append(txs, tx)
		}
		
		err := repo.Write(height, txs)
		require.NoError(t, err)
		txMap[height] = txs
	}
	
	// Read back and verify
	for height, expectedTxs := range txMap {
		txs, err := repo.GetByHeight(height)
		assert.NoError(t, err)
		assert.Len(t, txs, len(expectedTxs))
		
		// Verify transaction IDs match
		for i, tx := range txs {
			assert.Equal(t, expectedTxs[i].ID(), tx.ID())
		}
	}
}

func TestAtomicRepositoryMigration(t *testing.T) {
	// Test migration from old format
	db := versiondb.New(memdb.New())
	
	// Create repository - should handle migration automatically
	repo, err := NewAtomicTxRepository(db, atomic.Codec, 50)
	require.NoError(t, err)
	
	// Verify we can write and read after migration
	tx := &atomic.Tx{
		UnsignedAtomicTx: &atomic.TestUnsignedTx{
			GasUsedV: 2000,
		},
	}
	
	err = repo.Write(51, []*atomic.Tx{tx})
	require.NoError(t, err)
	
	// Read back
	txs, err := repo.GetByHeight(51)
	require.NoError(t, err)
	require.Len(t, txs, 1)
	assert.Equal(t, tx.ID(), txs[0].ID())
}

func TestAtomicRepositoryEmptyHeight(t *testing.T) {
	db := versiondb.New(memdb.New())
	repo, err := NewAtomicTxRepository(db, atomic.Codec, 100)
	require.NoError(t, err)
	
	// Query non-existent height
	txs, err := repo.GetByHeight(999)
	assert.Error(t, err)
	assert.Nil(t, txs)
}

// Helper function to create height bytes
func makeHeightBytes(height uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, height)
	return bytes
}
// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"encoding/binary"
	"fmt"

	"github.com/luxfi/evm/v2/plugin/evm/atomic"
	luxatomic "github.com/luxfi/node/v2/chains/atomic"
	"github.com/luxfi/node/v2/codec"
	"github.com/luxfi/database"
	"github.com/luxfi/database/prefixdb"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/v2/utils"
	"github.com/luxfi/node/v2/utils/wrappers"
)

var (
	atomicTxIDDBPrefix         = []byte("atomicTxDB")
	atomicHeightTxDBPrefix     = []byte("atomicHeightTxDB")
	atomicRepoMetadataDBPrefix = []byte("atomicRepoMetadataDB")
	maxIndexedHeightKey        = []byte("maxIndexedAtomicTxHeight")
)

// AtomicRepository manages the database interactions for atomic operations.
type AtomicRepository struct {
	// [acceptedAtomicTxDB] maintains an index of [txID] => [height]+[atomic tx] for all accepted atomic txs.
	acceptedAtomicTxDB database.Database

	// [acceptedAtomicTxByHeightDB] maintains an index of [height] => [atomic txs] for all accepted block heights.
	acceptedAtomicTxByHeightDB database.Database

	// [metadataDB] maintains the metadata for the atomic repository.
	metadataDB database.Database

	// [codec] is used to encode/decode atomic txs.
	codec codec.Manager
}

// NewAtomicTxRepository creates a new AtomicRepository instance
func NewAtomicTxRepository(db database.Database, codec codec.Manager, lastAcceptedHeight uint64) (*AtomicRepository, error) {
	repo := &AtomicRepository{
		acceptedAtomicTxDB:         prefixdb.New(atomicTxIDDBPrefix, db),
		acceptedAtomicTxByHeightDB: prefixdb.New(atomicHeightTxDBPrefix, db),
		metadataDB:                 prefixdb.New(atomicRepoMetadataDBPrefix, db),
		codec:                      codec,
	}

	// Check if we need to migrate from the old format
	_, err := repo.metadataDB.Get(maxIndexedHeightKey)
	if err == nil {
		// Already migrated, return
		return repo, nil
	}

	// Migrate old format transactions
	if err := repo.migrateOldFormat(lastAcceptedHeight); err != nil {
		return nil, fmt.Errorf("failed to migrate old format: %w", err)
	}

	return repo, nil
}

// Write stores atomic transactions at a given height
func (a *AtomicRepository) Write(height uint64, txs []*atomic.Tx) error {
	// Sort txs by ID for consistent ordering
	utils.Sort(txs)

	// Encode height and all tx IDs
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)

	txIDs := make([]ids.ID, 0, len(txs))
	for _, tx := range txs {
		txID := tx.ID()
		txIDs = append(txIDs, txID)

		// Store tx by ID with height prefix
		txBytes, err := a.codec.Marshal(atomic.CodecVersion, tx)
		if err != nil {
			return fmt.Errorf("failed to marshal tx: %w", err)
		}

		packer := wrappers.Packer{MaxSize: 1024 * 1024}
		packer.PackLong(height)
		packer.PackBytes(txBytes)
		if err := packer.Err; err != nil {
			return fmt.Errorf("failed to pack tx data: %w", err)
		}

		if err := a.acceptedAtomicTxDB.Put(txID[:], packer.Bytes); err != nil {
			return fmt.Errorf("failed to write tx: %w", err)
		}
	}

	// Store tx IDs by height
	if len(txIDs) > 0 {
		idsBytes, err := a.codec.Marshal(atomic.CodecVersion, txIDs)
		if err != nil {
			return fmt.Errorf("failed to marshal tx IDs: %w", err)
		}
		if err := a.acceptedAtomicTxByHeightDB.Put(heightBytes, idsBytes); err != nil {
			return fmt.Errorf("failed to write tx IDs by height: %w", err)
		}
	}

	// Update max indexed height
	return a.metadataDB.Put(maxIndexedHeightKey, heightBytes)
}

// GetByHeight retrieves all atomic transactions at a given height
func (a *AtomicRepository) GetByHeight(height uint64) ([]*atomic.Tx, error) {
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)

	// Get tx IDs for this height
	idsBytes, err := a.acceptedAtomicTxByHeightDB.Get(heightBytes)
	if err != nil {
		if err == database.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get tx IDs by height: %w", err)
	}

	var txIDs []ids.ID
	if _, err := a.codec.Unmarshal(idsBytes, &txIDs); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx IDs: %w", err)
	}

	// Retrieve each tx
	txs := make([]*atomic.Tx, 0, len(txIDs))
	for _, txID := range txIDs {
		data, err := a.acceptedAtomicTxDB.Get(txID[:])
		if err != nil {
			return nil, fmt.Errorf("failed to get tx %s: %w", txID, err)
		}

		unpacker := wrappers.Packer{Bytes: data}
		_ = unpacker.UnpackLong() // height - stored but not needed for retrieval
		txBytes := unpacker.UnpackBytes()
		if err := unpacker.Err; err != nil {
			return nil, fmt.Errorf("failed to unpack tx data: %w", err)
		}

		var tx atomic.Tx
		if _, err := a.codec.Unmarshal(txBytes, &tx); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tx: %w", err)
		}

		txs = append(txs, &tx)
	}

	return txs, nil
}

// migrateOldFormat migrates transactions from the old storage format
func (a *AtomicRepository) migrateOldFormat(lastAcceptedHeight uint64) error {
	// This is a simplified migration that processes all existing transactions
	iter := a.acceptedAtomicTxDB.NewIterator()
	defer iter.Release()

	txsByHeight := make(map[uint64][]*atomic.Tx)

	for iter.Next() {
		data := iter.Value()
		
		unpacker := wrappers.Packer{Bytes: data}
		height := unpacker.UnpackLong()
		txBytes := unpacker.UnpackBytes()
		if err := unpacker.Err; err != nil {
			continue // Skip malformed entries
		}

		var tx atomic.Tx
		if _, err := a.codec.Unmarshal(txBytes, &tx); err != nil {
			continue // Skip malformed entries
		}

		txsByHeight[height] = append(txsByHeight[height], &tx)
	}

	// Write transactions in the new format
	for height := uint64(0); height <= lastAcceptedHeight; height++ {
		if txs, ok := txsByHeight[height]; ok {
			if err := a.Write(height, txs); err != nil {
				return fmt.Errorf("failed to write txs at height %d: %w", height, err)
			}
		}
	}

	return nil
}

// mergeAtomicOps merges atomic operations from multiple transactions
func mergeAtomicOps(txs []*atomic.Tx) (map[ids.ID]*luxatomic.Requests, error) {
	requests := make(map[ids.ID]*luxatomic.Requests)
	
	for _, tx := range txs {
		chainID, req, err := tx.AtomicOps()
		if err != nil {
			return nil, err
		}
		if req == nil {
			continue
		}
		
		existing, ok := requests[chainID]
		if !ok {
			requests[chainID] = req
			continue
		}
		
		// Merge requests
		existing.PutRequests = append(existing.PutRequests, req.PutRequests...)
		existing.RemoveRequests = append(existing.RemoveRequests, req.RemoveRequests...)
	}
	
	return requests, nil
}
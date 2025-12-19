// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// export reads blocks from a SubnetEVM pebbledb and exports to RLP
package main

import (
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/cockroachdb/pebble"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
)

// SubnetEVM namespace for Zoo mainnet
var zooNamespace, _ = hex.DecodeString("337fb73f9bcdac8c31a2d5f7b877ab1e8a2b7f2a1e9bf02a0a0e6c6fd164f1d1")

func main() {
	dbPath := flag.String("db", "", "Path to pebbledb")
	output := flag.String("output", "blocks.rlp", "Output RLP file")
	flag.Parse()

	if *dbPath == "" {
		fmt.Println("Usage: export -db /path/to/pebbledb -output blocks.rlp")
		os.Exit(1)
	}

	// Open database in readonly mode
	db, err := pebble.Open(*dbPath, &pebble.Options{ReadOnly: true})
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Database opened: %s\n", *dbPath)

	// Find the tip height
	tipHeight, tipHash, err := findTip(db)
	if err != nil {
		fmt.Printf("Failed to find tip: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Tip block: height=%d, hash=%s\n", tipHeight, hex.EncodeToString(tipHash))

	// Create output file
	outFile, err := os.Create(*output)
	if err != nil {
		fmt.Printf("Failed to create output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	// Export blocks from 0 to tip
	fmt.Printf("Exporting blocks 0 to %d...\n", tipHeight)

	exported := 0
	for height := uint64(0); height <= tipHeight; height++ {
		block, err := getBlockByHeight(db, height)
		if err != nil {
			fmt.Printf("Warning: failed to get block %d: %v\n", height, err)
			continue
		}

		// Encode block to RLP
		blockRLP, err := rlp.EncodeToBytes(block)
		if err != nil {
			fmt.Printf("Warning: failed to encode block %d: %v\n", height, err)
			continue
		}

		// Write length-prefixed RLP
		lengthBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(lengthBuf, uint32(len(blockRLP)))
		outFile.Write(lengthBuf)
		outFile.Write(blockRLP)

		exported++
		if height%100 == 0 || height == tipHeight {
			fmt.Printf("Exported block %d/%d\n", height, tipHeight)
		}
	}

	fmt.Printf("\nExported %d blocks to %s\n", exported, *output)
}

func findTip(db *pebble.DB) (uint64, []byte, error) {
	// Look for AcceptorTipKey
	tipKey := append(zooNamespace, []byte("AcceptorTipKey")...)
	tipHash, closer, err := db.Get(tipKey)
	if err != nil {
		return 0, nil, fmt.Errorf("AcceptorTipKey not found: %v", err)
	}
	tipHashCopy := make([]byte, len(tipHash))
	copy(tipHashCopy, tipHash)
	closer.Close()

	// Get height from hash
	height, err := getHeightByHash(db, tipHashCopy)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get height for tip: %v", err)
	}

	return height, tipHashCopy, nil
}

func getHeightByHash(db *pebble.DB, hash []byte) (uint64, error) {
	// Key: namespace + 'H' + hash
	key := append(zooNamespace, 'H')
	key = append(key, hash...)

	value, closer, err := db.Get(key)
	if err != nil {
		return 0, err
	}
	defer closer.Close()

	if len(value) != 8 {
		return 0, fmt.Errorf("invalid height value length: %d", len(value))
	}

	return binary.BigEndian.Uint64(value), nil
}

func getHashByHeight(db *pebble.DB, height uint64) ([]byte, error) {
	// Iterate to find header with this height
	// Key format: namespace + 'h' + be8(height) + hash
	prefix := append(zooNamespace, 'h')
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)
	prefix = append(prefix, heightBytes...)

	iter, err := db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: append(prefix, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	if iter.First() {
		key := iter.Key()
		// Key is: namespace(32) + 'h'(1) + height(8) + hash(32) = 73 bytes
		if len(key) >= 73 {
			hash := key[41:73]
			hashCopy := make([]byte, 32)
			copy(hashCopy, hash)
			return hashCopy, nil
		}
	}

	return nil, fmt.Errorf("hash not found for height %d", height)
}

func getBlockByHeight(db *pebble.DB, height uint64) (*types.Block, error) {
	hash, err := getHashByHeight(db, height)
	if err != nil {
		return nil, err
	}

	header, err := getHeader(db, height, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get header: %v", err)
	}

	body, err := getBody(db, height, hash)
	if err != nil {
		// Body might be empty for genesis
		body = &types.Body{}
	}

	return types.NewBlockWithHeader(header).WithBody(*body), nil
}

func getHeader(db *pebble.DB, height uint64, hash []byte) (*types.Header, error) {
	// Key: namespace + 'h' + be8(height) + hash
	key := append(zooNamespace, 'h')
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)
	key = append(key, heightBytes...)
	key = append(key, hash...)

	value, closer, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var header types.Header
	if err := rlp.DecodeBytes(value, &header); err != nil {
		return nil, fmt.Errorf("failed to decode header: %v", err)
	}

	return &header, nil
}

func getBody(db *pebble.DB, height uint64, hash []byte) (*types.Body, error) {
	// Key: namespace + 'b' + be8(height) + hash
	key := append(zooNamespace, 'b')
	heightBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(heightBytes, height)
	key = append(key, heightBytes...)
	key = append(key, hash...)

	value, closer, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var body types.Body
	if err := rlp.DecodeBytes(value, &body); err != nil {
		return nil, fmt.Errorf("failed to decode body: %v", err)
	}

	return &body, nil
}

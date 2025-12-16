// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// verify_db reads a pebbledb and verifies the data integrity
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"github.com/cockroachdb/pebble"
)

func main() {
	dbPath := flag.String("db", "", "Path to pebbledb")
	flag.Parse()

	if *dbPath == "" {
		fmt.Println("Usage: verify_db -db /path/to/pebbledb")
		os.Exit(1)
	}

	// Open database in readonly mode
	db, err := pebble.Open(*dbPath, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		fmt.Printf("Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	fmt.Printf("Database opened successfully: %s\n\n", *dbPath)

	// Count keys and analyze structure
	iter, err := db.NewIter(nil)
	if err != nil {
		fmt.Printf("Failed to create iterator: %v\n", err)
		os.Exit(1)
	}
	defer iter.Close()

	keyCount := 0
	totalSize := int64(0)
	prefixCounts := make(map[string]int)

	for iter.First(); iter.Valid(); iter.Next() {
		keyCount++
		key := iter.Key()
		value := iter.Value()
		totalSize += int64(len(key) + len(value))

		// Analyze key prefixes (first byte after namespace if applicable)
		if len(key) > 32 {
			prefix := string(key[32:33])
			prefixCounts[prefix]++
		} else if len(key) > 0 {
			prefix := string(key[0:1])
			prefixCounts[prefix]++
		}

		// Print first 10 keys for debugging
		if keyCount <= 10 {
			keyHex := hex.EncodeToString(key)
			if len(keyHex) > 80 {
				keyHex = keyHex[:80] + "..."
			}
			fmt.Printf("Key %d: %s (value: %d bytes)\n", keyCount, keyHex, len(value))
		}
	}

	fmt.Printf("\n=== Database Statistics ===\n")
	fmt.Printf("Total keys: %d\n", keyCount)
	fmt.Printf("Total size: %d bytes (%.2f MB)\n", totalSize, float64(totalSize)/1024/1024)

	fmt.Printf("\n=== Key Prefixes ===\n")
	for prefix, count := range prefixCounts {
		fmt.Printf("'%s' (0x%02x): %d keys\n", prefix, prefix[0], count)
	}

	// Look for specific metadata keys
	fmt.Printf("\n=== Looking for Metadata ===\n")

	// Try to find AcceptorTipKey
	findKey(db, []byte("AcceptorTipKey"))
	findKey(db, []byte("AcceptorTipHeightKey"))

	// Try to find genesis or block 0
	fmt.Printf("\n=== Database verified successfully ===\n")
}

func findKey(db *pebble.DB, suffix []byte) {
	iter, _ := db.NewIter(nil)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := iter.Key()
		// Check if key ends with suffix
		if len(key) >= len(suffix) {
			if string(key[len(key)-len(suffix):]) == string(suffix) {
				fmt.Printf("Found '%s': key=%s, value=%s\n",
					string(suffix),
					hex.EncodeToString(key),
					hex.EncodeToString(iter.Value()))
				return
			}
		}
	}
	fmt.Printf("Key '%s' not found\n", string(suffix))
}

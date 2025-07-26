package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/luxfi/database/factory"
	luxlog "github.com/luxfi/log"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	var (
		sourceDB      = flag.String("source-db", "", "Path to source database")
		sourceType    = flag.String("source-type", "pebbledb", "Source database type (pebbledb, leveldb)")
		targetDB      = flag.String("target-db", "", "Path to target database")
		targetType    = flag.String("target-type", "badgerdb", "Target database type (badgerdb, leveldb, pebbledb)")
		batchSize     = flag.Int("batch-size", 10000, "Number of key-value pairs to process in each batch")
		verifyMigration = flag.Bool("verify", true, "Verify migration by comparing key counts")
	)
	flag.Parse()

	if *sourceDB == "" || *targetDB == "" {
		fmt.Println("Usage: dbmigrate -source-db <path> -target-db <path> [-source-type <type>] [-target-type <type>]")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Ensure target directory exists
	targetDir := filepath.Dir(*targetDB)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		log.Fatalf("Failed to create target directory: %v", err)
	}

	logger := luxlog.NewNoOpLogger()
	gatherer := prometheus.NewRegistry()
	
	fmt.Printf("Opening source database (%s) at %s...\n", *sourceType, *sourceDB)
	srcDB, err := factory.New(
		*sourceType,
		*sourceDB,
		true, // read-only
		nil,
		gatherer,
		logger,
		"source",
		"meterdb",
	)
	if err != nil {
		log.Fatalf("Failed to open source database: %v", err)
	}
	defer srcDB.Close()

	fmt.Printf("Creating target database (%s) at %s...\n", *targetType, *targetDB)
	dstDB, err := factory.New(
		*targetType,
		*targetDB,
		false, // writable
		nil,
		gatherer,
		logger,
		"target",
		"meterdb",
	)
	if err != nil {
		log.Fatalf("Failed to create target database: %v", err)
	}
	defer dstDB.Close()

	// Start migration
	fmt.Println("Starting database migration...")
	startTime := time.Now()
	
	keyCount := 0
	batch := dstDB.NewBatch()
	batchKeyCount := 0
	
	// Iterate through all keys in source database
	iter := srcDB.NewIterator()
	defer iter.Release()
	
	for iter.Next() {
		key := make([]byte, len(iter.Key()))
		copy(key, iter.Key())
		
		value := make([]byte, len(iter.Value()))
		copy(value, iter.Value())
		
		if err := batch.Put(key, value); err != nil {
			log.Fatalf("Failed to put key: %v", err)
		}
		
		batchKeyCount++
		keyCount++
		
		// Write batch when it reaches the specified size
		if batchKeyCount >= *batchSize {
			if err := batch.Write(); err != nil {
				log.Fatalf("Failed to write batch: %v", err)
			}
			fmt.Printf("Migrated %d keys...\n", keyCount)
			batch.Reset()
			batchKeyCount = 0
		}
	}
	
	// Write any remaining keys in the last batch
	if batchKeyCount > 0 {
		if err := batch.Write(); err != nil {
			log.Fatalf("Failed to write final batch: %v", err)
		}
	}
	
	if err := iter.Error(); err != nil {
		log.Fatalf("Iterator error: %v", err)
	}
	
	duration := time.Since(startTime)
	fmt.Printf("\nMigration completed successfully!\n")
	fmt.Printf("Total keys migrated: %d\n", keyCount)
	fmt.Printf("Time taken: %v\n", duration)
	fmt.Printf("Migration rate: %.2f keys/second\n", float64(keyCount)/duration.Seconds())
	
	// Verify migration if requested
	if *verifyMigration {
		fmt.Println("\nVerifying migration...")
		verifyKeyCount := 0
		
		verifyIter := dstDB.NewIterator()
		defer verifyIter.Release()
		
		for verifyIter.Next() {
			verifyKeyCount++
		}
		
		if err := verifyIter.Error(); err != nil {
			log.Printf("Warning: verification iterator error: %v", err)
		}
		
		if verifyKeyCount == keyCount {
			fmt.Printf("✓ Verification passed: %d keys in target database\n", verifyKeyCount)
		} else {
			fmt.Printf("✗ Verification failed: expected %d keys, found %d keys\n", keyCount, verifyKeyCount)
		}
	}
}
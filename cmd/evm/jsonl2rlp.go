// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build ignore

package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
)

type BlockExport struct {
	Number      uint64 `json:"number"`
	Hash        string `json:"hash"`
	HeaderRLP   string `json:"header_rlp"`
	BodyRLP     string `json:"body_rlp"`
	ReceiptsRLP string `json:"receipts_rlp"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: go run jsonl2rlp.go <input.jsonl> <output.rlp>")
		os.Exit(1)
	}

	inputFile := os.Args[1]
	outputFile := os.Args[2]

	// Read all blocks from JSONL
	file, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("Failed to open input: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	var exports []BlockExport
	scanner := bufio.NewScanner(file)
	// Increase buffer size for large lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		var exp BlockExport
		if err := json.Unmarshal(scanner.Bytes(), &exp); err != nil {
			fmt.Printf("Failed to parse JSON: %v\n", err)
			continue
		}
		exports = append(exports, exp)
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("Scanner error: %v\n", err)
		os.Exit(1)
	}

	// Sort by block number (ascending)
	sort.Slice(exports, func(i, j int) bool {
		return exports[i].Number < exports[j].Number
	})

	fmt.Printf("Read %d blocks\n", len(exports))
	if len(exports) > 0 {
		fmt.Printf("Range: %d to %d\n", exports[0].Number, exports[len(exports)-1].Number)
	}

	// Create output file
	out, err := os.Create(outputFile)
	if err != nil {
		fmt.Printf("Failed to create output: %v\n", err)
		os.Exit(1)
	}
	defer out.Close()

	writer := bufio.NewWriter(out)
	count := 0

	for _, exp := range exports {
		// Decode header
		headerBytes, err := hex.DecodeString(strings.TrimPrefix(exp.HeaderRLP, "0x"))
		if err != nil {
			fmt.Printf("Block %d: invalid header hex: %v\n", exp.Number, err)
			continue
		}

		var header types.Header
		if err := rlp.DecodeBytes(headerBytes, &header); err != nil {
			fmt.Printf("Block %d: failed to decode header: %v\n", exp.Number, err)
			continue
		}

		// Decode body
		var body types.Body
		if exp.BodyRLP != "" {
			bodyBytes, err := hex.DecodeString(strings.TrimPrefix(exp.BodyRLP, "0x"))
			if err != nil {
				fmt.Printf("Block %d: invalid body hex: %v\n", exp.Number, err)
				continue
			}
			if err := rlp.DecodeBytes(bodyBytes, &body); err != nil {
				fmt.Printf("Block %d: failed to decode body: %v\n", exp.Number, err)
				// Empty body is okay
				body = types.Body{}
			}
		}

		// Create block
		block := types.NewBlockWithHeader(&header).WithBody(body)

		// Encode block to RLP
		if err := rlp.Encode(writer, block); err != nil {
			fmt.Printf("Block %d: failed to encode: %v\n", exp.Number, err)
			continue
		}

		count++
		if exp.Number%100 == 0 {
			fmt.Printf("Wrote block %d\n", exp.Number)
		}
	}

	if err := writer.Flush(); err != nil {
		fmt.Printf("Failed to flush: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d blocks to %s\n", count, outputFile)
}

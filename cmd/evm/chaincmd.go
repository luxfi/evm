// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"compress/gzip"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/luxfi/evm/internal/flags"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/urfave/cli/v2"
)

var (
	DataDirFlag = &cli.StringFlag{
		Name:     "datadir",
		Usage:    "Path to pebbledb database",
		Category: flags.VMCategory,
	}
	NamespaceFlag = &cli.StringFlag{
		Name:     "namespace",
		Usage:    "SubnetEVM database namespace (hex)",
		Value:    "337fb73f9bcdac8c31a2d5f7b877ab1e8a2b7f2a1e9bf02a0a0e6c6fd164f1d1", // Zoo mainnet
		Category: flags.VMCategory,
	}

	exportCommand = &cli.Command{
		Name:      "export",
		Usage:     "Export blockchain from SubnetEVM pebbledb to RLP file",
		ArgsUsage: "<filename> [<blockNumFirst> <blockNumLast>]",
		Action:    exportChain,
		Flags: []cli.Flag{
			DataDirFlag,
			NamespaceFlag,
		},
	}

	importCommand = &cli.Command{
		Name:      "import",
		Usage:     "Validate/preview RLP blockchain file",
		ArgsUsage: "<filename>",
		Action:    importChain,
	}
)

func exportChain(ctx *cli.Context) error {
	if ctx.Args().Len() < 1 {
		return fmt.Errorf("output filename required")
	}

	dbPath := ctx.String(DataDirFlag.Name)
	if dbPath == "" {
		return fmt.Errorf("--datadir required")
	}

	outputFile := ctx.Args().Get(0)
	namespace, err := hex.DecodeString(ctx.String(NamespaceFlag.Name))
	if err != nil {
		return fmt.Errorf("invalid namespace: %v", err)
	}

	fmt.Printf("Opening pebbledb: %s\n", dbPath)
	db, err := pebble.Open(dbPath, &pebble.Options{ReadOnly: true})
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}
	defer db.Close()

	tipHeight, tipHash, err := findTip(db, namespace)
	if err != nil {
		return fmt.Errorf("failed to find tip: %v", err)
	}
	fmt.Printf("Found tip: height=%d hash=%s\n", tipHeight, hex.EncodeToString(tipHash))

	var first, last uint64
	if ctx.Args().Len() >= 3 {
		fmt.Sscanf(ctx.Args().Get(1), "%d", &first)
		fmt.Sscanf(ctx.Args().Get(2), "%d", &last)
	} else {
		first, last = 0, tipHeight
	}

	out, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output: %v", err)
	}
	defer out.Close()

	var writer io.Writer = out
	if strings.HasSuffix(outputFile, ".gz") {
		gw := gzip.NewWriter(out)
		defer gw.Close()
		writer = gw
	}

	fmt.Printf("Exporting blocks %d to %d -> %s\n", first, last, outputFile)
	start := time.Now()
	exported := 0

	for h := first; h <= last; h++ {
		block, err := getBlock(db, namespace, h)
		if err != nil {
			fmt.Printf("Skipping block %d: %v\n", h, err)
			continue
		}
		if err := rlp.Encode(writer, block); err != nil {
			return fmt.Errorf("encode error at block %d: %v", h, err)
		}
		exported++
		if h%500 == 0 {
			fmt.Printf("Progress: block %d, exported %d\n", h, exported)
		}
	}

	fmt.Printf("Export complete: %d blocks, file=%s, elapsed=%v\n", exported, outputFile, time.Since(start))
	return nil
}

func importChain(ctx *cli.Context) error {
	if ctx.Args().Len() < 1 {
		return fmt.Errorf("input filename required")
	}

	inputFile := ctx.Args().Get(0)
	in, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(inputFile, ".gz") {
		gr, err := gzip.NewReader(in)
		if err != nil {
			return err
		}
		defer gr.Close()
		reader = gr
	}

	fmt.Printf("Validating RLP file: %s\n", inputFile)
	stream := rlp.NewStream(reader, 0)
	count := 0

	for {
		var block types.Block
		if err := stream.Decode(&block); err == io.EOF {
			break
		} else if err != nil {
			return fmt.Errorf("decode error at block %d: %v", count, err)
		}
		count++
		if count == 1 || count%500 == 0 {
			fmt.Printf("Block %d: number=%d txs=%d\n", count, block.NumberU64(), len(block.Transactions()))
		}
	}

	fmt.Printf("Validation complete: %d blocks\n", count)
	fmt.Printf("Import with: geth import %s\n", inputFile)
	return nil
}

// SubnetEVM pebbledb helpers

func findTip(db *pebble.DB, ns []byte) (uint64, []byte, error) {
	val, closer, err := db.Get(append(ns, []byte("AcceptorTipKey")...))
	if err != nil {
		return 0, nil, err
	}
	hash := make([]byte, len(val))
	copy(hash, val)
	closer.Close()

	heightKey := append(ns, 'H')
	heightKey = append(heightKey, hash...)
	hval, hcloser, err := db.Get(heightKey)
	if err != nil {
		return 0, nil, err
	}
	defer hcloser.Close()
	return binary.BigEndian.Uint64(hval), hash, nil
}

func getBlock(db *pebble.DB, ns []byte, height uint64) (*types.Block, error) {
	hash, err := getHash(db, ns, height)
	if err != nil {
		return nil, err
	}

	hdr, err := getHeader(db, ns, height, hash)
	if err != nil {
		return nil, err
	}

	body, _ := getBody(db, ns, height, hash)
	if body == nil {
		body = &types.Body{}
	}

	return types.NewBlockWithHeader(hdr).WithBody(*body), nil
}

func getHash(db *pebble.DB, ns []byte, height uint64) ([]byte, error) {
	prefix := append(ns, 'h')
	hb := make([]byte, 8)
	binary.BigEndian.PutUint64(hb, height)
	prefix = append(prefix, hb...)

	iter, _ := db.NewIter(&pebble.IterOptions{LowerBound: prefix, UpperBound: append(prefix, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff)})
	defer iter.Close()

	if iter.First() && len(iter.Key()) >= 73 {
		h := make([]byte, 32)
		copy(h, iter.Key()[41:73])
		return h, nil
	}
	return nil, fmt.Errorf("hash not found for height %d", height)
}

func getHeader(db *pebble.DB, ns []byte, height uint64, hash []byte) (*types.Header, error) {
	key := append(ns, 'h')
	hb := make([]byte, 8)
	binary.BigEndian.PutUint64(hb, height)
	key = append(key, hb...)
	key = append(key, hash...)

	val, closer, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var hdr types.Header
	if err := rlp.DecodeBytes(val, &hdr); err != nil {
		return nil, err
	}
	return &hdr, nil
}

func getBody(db *pebble.DB, ns []byte, height uint64, hash []byte) (*types.Body, error) {
	key := append(ns, 'b')
	hb := make([]byte, 8)
	binary.BigEndian.PutUint64(hb, height)
	key = append(key, hb...)
	key = append(key, hash...)

	val, closer, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	defer closer.Close()

	var body types.Body
	if err := rlp.DecodeBytes(val, &body); err != nil {
		return nil, err
	}
	return &body, nil
}

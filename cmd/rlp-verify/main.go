// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// rlp-verify decodes every block of a C-Chain RLP export and verifies:
//   - the genesis hash matches a provided genesis JSON (luxfi/evm Genesis)
//   - each subsequent block's ParentHash matches the previous block's Hash
//   - the stream terminates cleanly at EOF
//
// This is what admin.importChain does (sans state execution), so a clean
// verify here is sufficient to prove the RLP+genesis pair is wireable to
// a luxd boot — the eth state will be rebuilt from genesis state root +
// the transactions in each block during InsertChain.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	evmcore "github.com/luxfi/evm/core"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
)

func main() {
	rlpPath := flag.String("rlp", "", "Path to RLP export (required)")
	genesisPath := flag.String("genesis", "", "Path to luxfi/evm Genesis JSON (required)")
	maxBlocks := flag.Int("max-blocks", 0, "Stop after N blocks (0 = all)")
	flag.Parse()

	if *rlpPath == "" || *genesisPath == "" {
		fmt.Fprintln(os.Stderr, "usage: rlp-verify -rlp <file.rlp> -genesis <file.json> [-max-blocks N]")
		os.Exit(2)
	}

	// 1. Compute expected genesis hash from the provided genesis JSON.
	genRaw, err := os.ReadFile(*genesisPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read genesis:", err)
		os.Exit(1)
	}
	var g evmcore.Genesis
	if err := json.Unmarshal(genRaw, &g); err != nil {
		fmt.Fprintln(os.Stderr, "decode genesis:", err)
		os.Exit(1)
	}
	expectedGenesisHash := g.ToBlock().Hash()

	// 2. Stream the RLP, decode each block, check chain continuity.
	f, err := os.Open(*rlpPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open rlp:", err)
		os.Exit(1)
	}
	defer f.Close()

	stream := rlp.NewStream(f, 0)
	var (
		count      uint64
		lastHash   common.Hash
		firstBlock *types.Block
	)
	t0 := time.Now()
	for {
		var b types.Block
		err := stream.Decode(&b)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "decode block #%d: %v\n", count, err)
			os.Exit(1)
		}
		if count == 0 {
			firstBlock = &b
			if b.Hash() != expectedGenesisHash {
				fmt.Fprintf(os.Stderr, "GENESIS MISMATCH\n  rlp block 0 hash:   %s\n  genesis-computed:   %s\n",
					b.Hash().Hex(), expectedGenesisHash.Hex())
				os.Exit(1)
			}
		} else {
			if b.ParentHash() != lastHash {
				fmt.Fprintf(os.Stderr, "PARENT MISMATCH at height %d\n  block.parent: %s\n  prev.hash:    %s\n",
					b.NumberU64(), b.ParentHash().Hex(), lastHash.Hex())
				os.Exit(1)
			}
		}
		lastHash = b.Hash()
		count++
		if *maxBlocks > 0 && count >= uint64(*maxBlocks) {
			break
		}
		if count%50000 == 0 {
			fmt.Fprintf(os.Stderr, "verified %d blocks (last=%s)\n", count, lastHash.Hex())
		}
	}
	elapsed := time.Since(t0)

	out := map[string]any{
		"genesis_hash_match": true,
		"genesis_hash":       expectedGenesisHash.Hex(),
		"blocks_verified":    count,
		"first_block_hash":   firstBlock.Hash().Hex(),
		"first_block_state":  firstBlock.Root().Hex(),
		"last_block_hash":    lastHash.Hex(),
		"verify_seconds":     elapsed.Seconds(),
	}
	j, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(j))
}

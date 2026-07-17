// Command ghash computes the canonical block-0 hash of one or more C-Chain
// genesis JSON files via luxfi/evm core.Genesis.ToBlock — the production path
// (geth v1.20.1), replacing the legacy coreth-based verifier.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/luxfi/evm/core"
)

func hashOf(path string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = fmt.Sprintf("panic: %v", r)
		}
	}()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("read error: %v", err)
	}
	var g core.Genesis
	if err := json.Unmarshal(data, &g); err != nil {
		return fmt.Sprintf("unmarshal error: %v", err)
	}
	b := g.ToBlock()
	return fmt.Sprintf("block-0 = %s  (allocs=%d ts=0x%x skipPMF=%v)",
		b.Hash().Hex(), len(g.Alloc), g.Timestamp, g.SkipPostMergeFields)
}

func main() {
	for _, path := range os.Args[1:] {
		fmt.Printf("%s\n  %s\n", path, hashOf(path))
	}
}

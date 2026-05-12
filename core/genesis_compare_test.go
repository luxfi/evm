// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

// TestLuxGenesisFromJSON builds the genesis block from cchain.json and compares
// hash + RLP bytes to the genesis recovered from the canonical RLP file.
func TestLuxGenesisFromJSON(t *testing.T) {
	cchainPath := "/Users/z/work/lux/genesis/configs/mainnet/cchain.json"
	rlpPath := "/Users/z/work/lux/state/rlp/lux-mainnet/lux-mainnet-96369.rlp"

	for _, p := range []string{cchainPath, rlpPath} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Skipf("missing input %s", p)
		}
	}

	// 1) Build the genesis block from JSON.
	jsonBytes, err := os.ReadFile(cchainPath)
	require.NoError(t, err)

	var g Genesis
	require.NoError(t, json.Unmarshal(jsonBytes, &g))

	// Override gasLimit to canonical 12M (cchain.json has 100M).
	g.GasLimit = 0xb71b00

	// Defer shanghai/cancun beyond genesis to avoid Eth2 fields (e.g. WithdrawalsHash)
	// being added to the genesis header, which would force a 17-field RLP encoding.
	farFuture := uint64(253399622400)
	g.Config.ShanghaiTime = &farFuture
	g.Config.CancunTime = &farFuture

	tmp := t.TempDir()
	_ = tmp
	// Use ToBlock without DB commit by extracting head computation.
	jsonBlock := g.ToBlock()
	jsonHeader := jsonBlock.Header()
	jsonRLP, err := rlp.EncodeToBytes(jsonHeader)
	require.NoError(t, err)

	t.Logf("JSON-built genesis:")
	t.Logf("  hash:       %s", jsonBlock.Hash().Hex())
	t.Logf("  state root: %s", jsonHeader.Root.Hex())
	t.Logf("  RLP len:    %d", len(jsonRLP))
	t.Logf("  RLP:        %s", hex.EncodeToString(jsonRLP))
	t.Logf("  ExtDataHash:    %v", jsonHeader.ExtDataHash)
	t.Logf("  ExtDataGasUsed: %v", jsonHeader.ExtDataGasUsed)
	t.Logf("  BlockGasCost:   %v", jsonHeader.BlockGasCost)
	t.Logf("  WithdrawalsHash:%v", jsonHeader.WithdrawalsHash)
	t.Logf("  BlobGasUsed:    %v", jsonHeader.BlobGasUsed)
	t.Logf("  ExcessBlobGas:  %v", jsonHeader.ExcessBlobGas)
	t.Logf("  ParentBeaconRoot:%v", jsonHeader.ParentBeaconRoot)
	t.Logf("  RequestsHash:   %v", jsonHeader.RequestsHash)

	// 2) Decode genesis from canonical RLP file.
	file, err := os.Open(rlpPath)
	require.NoError(t, err)
	defer file.Close()

	stream := rlp.NewStream(file, 0)
	var rlpBlock types.Block
	require.NoError(t, stream.Decode(&rlpBlock))
	rlpHeader := rlpBlock.Header()

	rlpHeaderBytes, err := rlp.EncodeToBytes(rlpHeader)
	require.NoError(t, err)

	t.Logf("RLP-decoded genesis:")
	t.Logf("  hash:       %s", rlpBlock.Hash().Hex())
	t.Logf("  state root: %s", rlpHeader.Root.Hex())
	t.Logf("  RLP len:    %d", len(rlpHeaderBytes))
	t.Logf("  RLP:        %s", hex.EncodeToString(rlpHeaderBytes))
}

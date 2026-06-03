// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"encoding/json"
	"testing"

	"github.com/luxfi/geth/common"
)

// TestSkipPostMergeFieldsGenesisHash pins the canonical Lux mainnet
// C-Chain genesis hash 0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecafc987794050cb0d286f1ec61e.
//
// The genesis JSON sets cancunTime=0, so without SkipPostMergeFields
// the genesis block would carry ParentBeaconRoot/ExcessBlobGas/BlobGasUsed
// header fields that did not exist when the historic chain was launched.
// Honoring the JSON flag preserves hash continuity with the historic
// 1.08M-block RLP export at lux-mainnet-96369.rlp (block 0 hash above).
func TestSkipPostMergeFieldsGenesisHash(t *testing.T) {
	const luxMainnetGenesis = `{
  "alloc": {
    "0200000000000000000000000000000000000005": {
      "balance": "0x0",
      "code": "0x01",
      "nonce": "0x1"
    },
    "0x9011E888251AB053B7bD1cdB598Db4f9DEd94714": {
      "balance": "0x193e5939a08ce9dbd480000000"
    }
  },
  "baseFeePerGas": "0x5d21dba00",
  "coinbase": "0x0000000000000000000000000000000000000000",
  "config": {
    "arrowGlacierBlock": 0,
    "berlinBlock": 0,
    "byzantiumBlock": 0,
    "cancunTime": 0,
    "chainId": 96369,
    "constantinopleBlock": 0,
    "eip150Block": 0,
    "eip155Block": 0,
    "eip158Block": 0,
    "evmTimestamp": 0,
    "feeConfig": {
      "baseFeeChangeDenominator": 36,
      "blockGasCostStep": 200000,
      "gasLimit": 12000000,
      "maxBlockGasCost": 1000000,
      "minBaseFee": 25000000000,
      "minBlockGasCost": 0,
      "targetBlockRate": 2,
      "targetGas": 500000000
    },
    "grayGlacierBlock": 0,
    "homesteadBlock": 0,
    "istanbulBlock": 0,
    "londonBlock": 0,
    "mergeNetsplitBlock": 0,
    "muirGlacierBlock": 0,
    "petersburgBlock": 0,
    "shanghaiTime": 0,
    "terminalTotalDifficulty": 0,
    "durangoTimestamp": 0,
    "quasarTimestamp": 0,
    "fortunaTimestamp": 0,
    "graniteTimestamp": 0
  },
  "difficulty": "0x0",
  "extraData": "0x",
  "gasLimit": "0xb71b00",
  "gasUsed": "0x0",
  "mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "nonce": "0x0",
  "number": "0x0",
  "parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "skipPostMergeFields": true,
  "stateRoot": "0x2d1cedac263020c5c56ef962f6abe0da1f5217bdc6468f8c9258a0ea23699e80",
  "timestamp": "0x672485c2"
}`
	const wantHash = "0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecafc987794050cb0d286f1ec61e"

	var g Genesis
	if err := json.Unmarshal([]byte(luxMainnetGenesis), &g); err != nil {
		t.Fatalf("unmarshal genesis: %v", err)
	}
	if !g.SkipPostMergeFields {
		t.Fatalf("SkipPostMergeFields not honored from JSON")
	}
	got := g.ToBlock().Hash()
	if got != common.HexToHash(wantHash) {
		t.Fatalf("genesis hash mismatch:\n  got  %s\n  want %s", got.Hex(), wantHash)
	}
}

// TestPostMergeFieldsActiveWhenFlagFalse ensures that without
// SkipPostMergeFields the genesis hash flips to a different value
// (i.e. the flag is the gate, not a no-op). Uses the same chain spec
// minus the flag.
func TestPostMergeFieldsActiveWhenFlagFalse(t *testing.T) {
	const baseGenesis = `{
  "alloc": {
    "0200000000000000000000000000000000000005": {
      "balance": "0x0",
      "code": "0x01",
      "nonce": "0x1"
    },
    "0x9011E888251AB053B7bD1cdB598Db4f9DEd94714": {
      "balance": "0x193e5939a08ce9dbd480000000"
    }
  },
  "baseFeePerGas": "0x5d21dba00",
  "coinbase": "0x0000000000000000000000000000000000000000",
  "config": {
    "arrowGlacierBlock": 0,
    "berlinBlock": 0,
    "byzantiumBlock": 0,
    "cancunTime": 0,
    "chainId": 96369,
    "constantinopleBlock": 0,
    "eip150Block": 0,
    "eip155Block": 0,
    "eip158Block": 0,
    "evmTimestamp": 0,
    "feeConfig": {
      "baseFeeChangeDenominator": 36,
      "blockGasCostStep": 200000,
      "gasLimit": 12000000,
      "maxBlockGasCost": 1000000,
      "minBaseFee": 25000000000,
      "minBlockGasCost": 0,
      "targetBlockRate": 2,
      "targetGas": 500000000
    },
    "grayGlacierBlock": 0,
    "homesteadBlock": 0,
    "istanbulBlock": 0,
    "londonBlock": 0,
    "mergeNetsplitBlock": 0,
    "muirGlacierBlock": 0,
    "petersburgBlock": 0,
    "shanghaiTime": 0,
    "terminalTotalDifficulty": 0,
    "durangoTimestamp": 0,
    "quasarTimestamp": 0,
    "fortunaTimestamp": 0,
    "graniteTimestamp": 0
  },
  "difficulty": "0x0",
  "extraData": "0x",
  "gasLimit": "0xb71b00",
  "gasUsed": "0x0",
  "mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "nonce": "0x0",
  "number": "0x0",
  "parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "stateRoot": "0x2d1cedac263020c5c56ef962f6abe0da1f5217bdc6468f8c9258a0ea23699e80",
  "timestamp": "0x672485c2"
}`
	var g Genesis
	if err := json.Unmarshal([]byte(baseGenesis), &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if g.SkipPostMergeFields {
		t.Fatalf("SkipPostMergeFields should default false")
	}
	got := g.ToBlock().Hash()
	canonical := common.HexToHash("0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecafc987794050cb0d286f1ec61e")
	if got == canonical {
		t.Fatalf("expected different hash when post-merge fields are active, got canonical %s", canonical.Hex())
	}
}

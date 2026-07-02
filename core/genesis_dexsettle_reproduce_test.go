// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/evm/precompile/registry"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/tracing"
	gethtypes "github.com/luxfi/geth/core/types"
	"github.com/stretchr/testify/require"

	// Register the full precompile set (incl. the AlwaysOn DEX settlement module at
	// 0x9999), exactly as the production node does.
	_ "github.com/luxfi/evm/precompile/registry"
)

// This file is the ship-gate regression for the DEX settlement precompile (0x9999)
// genesis-injection fix. Before the fix, 0x9999's EXTCODESIZE marker was baked into
// EVERY chain's genesis state unconditionally, which mutated the genesis state root and
// changed the genesis hash of every pre-2025 chain — breaking RLP re-import of the real
// archived Lux mainnet, Lux testnet and Zoo mainnet (all re-genesised Nov 2024, before
// the DEX settlement activation, Dec 2025). The fix gates the injection on the
// precompile's protocol ActivationTime: it lands in genesis state ONLY IF
// ActivationTime <= the genesis timestamp, so a chain born before activation reproduces
// its ORIGINAL genesis byte-for-byte, and 0x9999 instead activates later, at the block
// crossing the timestamp.
//
// The three real genesis hashes below are the authoritative block-0 hashes of the RLP
// exports in ~/work/lux/state/rlp/{lux-mainnet,lux-testnet,zoo-mainnet}. They are the
// literal import targets; if a computed hash drifts from them, the archived chains
// cannot be re-imported. addr9999 is defined in precompile_alwayson_test.go.

// canonicalGenesisJSON is the 2-account genesis shared by the Nov-2024 re-genesis of
// Lux mainnet, Lux testnet and Zoo mainnet: the historical warp precompile marker at
// 0x02..05 (code 0x01) + the treasury at 0x9011 funded with 2e30 wei — stateRoot
// 0x2d1cedac…, NO 0x9999. The three chains differ ONLY in chainId and genesis timestamp
// (all other header fields — gasLimit 0xb71b00, baseFee 25e9, zero difficulty/nonce/
// coinbase/mixhash, empty extraData, 16-field pre-withdrawals header — are identical).
// quasarTimestamp is set far in the future so Cancun is inactive at genesis and the
// header stays in the original pre-withdrawals (16-field) format.
const canonicalGenesisJSON = `{
  "alloc": {
    "0200000000000000000000000000000000000005": {"balance":"0x0","code":"0x01","nonce":"0x1"},
    "9011E888251AB053B7bD1cdB598Db4f9DEd94714": {"balance":"0x193e5939a08ce9dbd480000000"}
  },
  "baseFeePerGas": "0x5d21dba00",
  "coinbase": "0x0000000000000000000000000000000000000000",
  "config": {
    "berlinBlock":0,"byzantiumBlock":0,"chainId":%d,"constantinopleBlock":0,
    "durangoTimestamp":0,"eip150Block":0,"eip155Block":0,"eip158Block":0,
    "quasarTimestamp":253399622400,"evmTimestamp":0,
    "fortunaTimestamp":253399622400,"graniteTimestamp":253399622400,
    "homesteadBlock":0,"istanbulBlock":0,"londonBlock":0,"muirGlacierBlock":0,
    "petersburgBlock":0,"terminalTotalDifficulty":0,
    "feeConfig":{"baseFeeChangeDenominator":36,"blockGasCostStep":200000,"gasLimit":12000000,
      "maxBlockGasCost":1000000,"minBaseFee":25000000000,"minBlockGasCost":0,
      "targetBlockRate":2,"targetGas":15000000}
  },
  "difficulty": "0x0",
  "extraData": "0x",
  "gasLimit": "0xb71b00",
  "gasUsed": "0x0",
  "mixHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "nonce": "0x0",
  "number": "0x0",
  "parentHash": "0x0000000000000000000000000000000000000000000000000000000000000000",
  "timestamp": "%s"
}`

func canonicalGenesis(t *testing.T, chainID uint64, timestampHex string) *Genesis {
	t.Helper()
	var g Genesis
	require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(canonicalGenesisJSON, chainID, timestampHex)), &g))
	return &g
}

// TestGenesisReproduce_PreDexActivationChains is the hard ship gate: the fixed build
// reproduces the ORIGINAL block-0 genesis hash of each real archived chain, byte-
// identical, so its RLP re-imports. All three were re-genesised in Nov 2024, before the
// DEX settlement activation (Dec 2025), so NONE carries 0x9999 in genesis state.
func TestGenesisReproduce_PreDexActivationChains(t *testing.T) {
	cases := []struct {
		name         string
		chainID      uint64
		timestampHex string // block-0 header timestamp, from the RLP export
		wantHash     string // authoritative block-0 hash, the RLP import target
	}{
		{"lux-mainnet-96369", 96369, "0x672485c2", "0x3f4fa2a0b0ce089f52bf0ae9199c75ffdd76ecafc987794050cb0d286f1ec61e"},
		{"lux-testnet-96368", 96368, "0x67259912", "0x1c5fe37764b8bc146dc88bc1c2e0259cd8369b07a06439bcfa1782b5d4fb0995"},
		{"zoo-mainnet-200200", 200200, "0x6727e9c3", "0x7c548af47de27560779ccc67dda32a540944accc71dac3343da3b9cd18f14933"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := canonicalGenesis(t, tc.chainID, tc.timestampHex)
			// Genesis timestamp is Nov 2024, strictly before the DEX activation.
			require.Less(t, g.Timestamp, registry.DexSettleActivationTime,
				"precondition: this chain's genesis must predate DEX settlement activation")

			blk := g.ToBlock()
			require.Equal(t, tc.wantHash, blk.Hash().Hex(),
				"%s genesis hash MUST reproduce byte-identically or the archived RLP cannot import", tc.name)

			// 0x9999 must NOT be in the genesis state (this is what preserves the hash).
			sdb := genesisState(t, g)
			require.Empty(t, sdb.GetCode(addr9999),
				"0x9999 must be absent from a pre-activation chain's genesis state")
			require.Zero(t, sdb.GetNonce(addr9999),
				"0x9999 must carry no activation nonce at a pre-activation genesis")
		})
	}
}

// TestGenesisReproduce_FreshChainWantsDexAtGenesis pins condition 4: a genuinely fresh
// chain whose genesis timestamp is at/after the DEX activation DOES get 0x9999 injected
// into its genesis state (EXTCODESIZE marker present from block 0), and its genesis hash
// therefore differs from the same chain built with a pre-activation timestamp.
func TestGenesisReproduce_FreshChainWantsDexAtGenesis(t *testing.T) {
	freshTS := registry.DexSettleActivationTime + 3600 // one hour after activation
	g := canonicalGenesis(t, 424242, fmt.Sprintf("0x%x", freshTS))
	require.GreaterOrEqual(t, g.Timestamp, registry.DexSettleActivationTime)

	sdb := genesisState(t, g)
	require.Equal(t, []byte{0x01}, sdb.GetCode(addr9999),
		"a chain genesised at/after DEX activation MUST carry the 0x9999 marker at genesis")
	require.Equal(t, uint64(1), sdb.GetNonce(addr9999),
		"0x9999 must carry the activation nonce at a post-activation genesis")

	// Its hash must differ from the identical chain built one second before activation
	// (which omits 0x9999) — proving the injection is genuinely timestamp-gated, not inert.
	pre := canonicalGenesis(t, 424242, fmt.Sprintf("0x%x", registry.DexSettleActivationTime-1))
	require.NotEqual(t, pre.ToBlock().Hash(), g.ToBlock().Hash(),
		"genesis hash must change across the activation boundary")
}

// genesisState rebuilds the committed genesis account trie so the test can inspect
// getCode(0x9999). It mirrors (*Genesis).toBlock exactly (ApplyPrecompileActivations at
// parent==nil, then alloc) and its root MUST equal ToBlock's, which it asserts.
func genesisState(t *testing.T, g *Genesis) *state.StateDB {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	statedb, err := state.New(gethtypes.EmptyRootHash, state.NewDatabase(db), nil)
	require.NoError(t, err)
	require.NoError(t, ApplyPrecompileActivations(g.Config, nil, NewBlockContext(big.NewInt(int64(g.Number)), g.Timestamp), statedb))
	for addr, acct := range g.Alloc {
		statedb.SetBalance(addr, uint256.MustFromBig(acct.Balance), tracing.BalanceIncreaseGenesisBalance)
		_ = statedb.SetCode(addr, acct.Code, tracing.CodeChangeGenesis)
		statedb.SetNonce(addr, acct.Nonce, tracing.NonceChangeGenesis)
		for k, v := range acct.Storage {
			statedb.SetState(addr, k, v)
		}
	}
	require.Equal(t, g.ToBlock().Root(), statedb.IntermediateRoot(false),
		"rebuilt genesis state root must equal ToBlock's — otherwise this inspection is not authoritative")
	return statedb
}

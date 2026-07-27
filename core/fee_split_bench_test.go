// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
)

// Fee-disbursement hot-path microbenchmarks. creditTxFee runs once per
// transaction, so the dormant (production) path must add negligible cost over the
// pre-split code. The three benchmarks isolate:
//
//	RawBaseline : the pre-fee-split code — a bare AddBalance, no branch.
//	Legacy      : the shipped dormant path — GetExtra + IsFeeSplit branch, then
//	              the same AddBalance. (Legacy − RawBaseline) == the seam overhead
//	              every mainnet tx pays today.
//	Split       : the active split path — a shift, then the same AddBalance to the
//	              same configured coinbase; the remainder is left uncredited (burn).
func benchFeeStateDB(b *testing.B) *state.StateDB {
	b.Helper()
	sdb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	if err != nil {
		b.Fatal(err)
	}
	return sdb
}

func benchFeeConfig(active bool) *params.ChainConfig {
	extra := &extras.ChainConfig{}
	if active {
		ts := uint64(0)
		extra.FeeSplitTimestamp = &ts
	}
	return params.WithExtra(&params.ChainConfig{ChainID: big.NewInt(96369)}, extra)
}

var benchFee = uint256.NewInt(21000 * 225_000_000_000) // 21k gas @ 225 gwei
var benchCoinbase = common.HexToAddress("0x0100000000000000000000000000000000000000")

func BenchmarkCreditTxFee_RawBaseline(b *testing.B) {
	sdb := benchFeeStateDB(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sdb.AddBalance(benchCoinbase, benchFee, tracing.BalanceIncreaseRewardTransactionFee)
	}
}

func BenchmarkCreditTxFee_Legacy(b *testing.B) {
	sdb := benchFeeStateDB(b)
	cfg := benchFeeConfig(false)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		creditTxFee(sdb, cfg, benchCoinbase, 0, benchFee)
	}
}

func BenchmarkCreditTxFee_Split(b *testing.B) {
	sdb := benchFeeStateDB(b)
	cfg := benchFeeConfig(true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		creditTxFee(sdb, cfg, benchCoinbase, 0, benchFee)
	}
}

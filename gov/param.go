// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import (
	"math/big"
	"sort"

	"github.com/luxfi/evm/commontype"
)

// ParamID names a governable value. IDs are consensus-critical and append-only:
// a released ID keeps its meaning forever, because it is the key under which the
// value is stored and the token validators put in their headers.
type ParamID uint8

// The governable set.
//
// Every member is a field the node already resolves through
// BlockChain.GetFeeConfigAt(parent) — i.e. already read from consensus state,
// already enforced in header validity by dummy.verifyHeaderGasFields. Nothing
// else is in this table, and that is the honest boundary: a value the node does
// not read from state cannot be governed by writing state, however the vote is
// counted.
//
// Explicitly NOT here, and not governable by any vote:
//
//	SupplyCap, MinConsumptionRate, MaxConsumptionRate, MintingPeriod
//	    P-chain emission. Resolved once at VM init (platformvm/vm.go:254
//	    reward.NewCalculator(vm.RewardConfig)) from CLI flags defaulting to
//	    genesis.MainnetParams. The P-chain cannot read C-chain state, so no
//	    write here can reach it. Changing emission is a node release.
//	MinValidatorStake, MaxValidatorStake, MinDelegatorStake,
//	MinStakeDuration, MaxStakeDuration, MinDelegationFee, UptimeRequirement
//	    Same file, same fate.
//	the 50/50 fee split
//	    core/fee_split.go computes Rsh(fee,1). A constant, by design.
const (
	ParamGasLimit                 ParamID = 1
	ParamTargetBlockRate          ParamID = 2
	ParamMinBaseFee               ParamID = 3
	ParamTargetGas                ParamID = 4
	ParamBaseFeeChangeDenominator ParamID = 5
	ParamMinBlockGasCost          ParamID = 6
	ParamMaxBlockGasCost          ParamID = 7
	ParamBlockGasCostStep         ParamID = 8
)

// Bound is the compiled-in range a governed value may move within. It is the
// constitution: no vote can widen it, and a signal outside it is never counted.
type Bound struct {
	Name string
	Min  uint64
	Max  uint64
}

// Bounds is the whole governable universe. A ParamID absent from this map is
// not governable, and a signal naming it is discarded before it is counted.
//
// Ranges are set so that every reachable combination keeps the chain alive:
// blocks can always be produced, fees can never be zero (which would make spam
// free), and the dynamic-fee controller can never be given a denominator of
// zero or a target of zero. The mainnet values measured at height 1098191
// (GasLimit 12000000, MinBaseFee 25 gwei) sit inside every range.
var Bounds = map[ParamID]Bound{
	// Block capacity. Floor keeps the chain usable; ceiling keeps a state-growth
	// attack from being one vote away.
	ParamGasLimit: {"gasLimit", 5_000_000, 100_000_000},
	// Seconds per block the fee controller targets. Zero would divide by zero.
	ParamTargetBlockRate: {"targetBlockRate", 1, 30},
	// Floor on the EIP-1559 base fee, in wei. A nonzero minimum is what makes
	// spam cost something; 1 gwei is the floor of the floor.
	ParamMinBaseFee: {"minBaseFee", 1_000_000_000, 500_000_000_000},
	// Gas the controller aims to see per rolling window.
	ParamTargetGas: {"targetGas", 1_000_000, 1_000_000_000},
	// Larger = stickier base fee. Zero would divide by zero.
	ParamBaseFeeChangeDenominator: {"baseFeeChangeDenominator", 2, 1_000_000},
	// Block-production gas cost band and its per-second step.
	ParamMinBlockGasCost:  {"minBlockGasCost", 0, 1_000_000},
	ParamMaxBlockGasCost:  {"maxBlockGasCost", 0, 10_000_000},
	ParamBlockGasCostStep: {"blockGasCostStep", 0, 5_000_000},
}

// Governable reports whether [id] is in the constitution and [value] is inside
// its bound. Both halves are consensus-critical: every node must agree on which
// signals count, so this is the single place either question is answered.
func Governable(id ParamID, value uint64) bool {
	b, ok := Bounds[id]
	return ok && value >= b.Min && value <= b.Max
}

// SortedParamIDs returns every governable ID in ascending order. Iteration over
// Bounds is random; anything that touches state must use this instead.
func SortedParamIDs() []ParamID {
	ids := make([]ParamID, 0, len(Bounds))
	for id := range Bounds {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Resolve returns the fee schedule the node should actually use: [base] with
// every governed value substituted in, unless the result would be invalid, in
// which case [base] is returned untouched and the second result is false.
//
// The fallback is not defensive decoration, it closes a real liveness hole.
// Parameters are voted one at a time, so an epoch that lowers MaxBlockGasCost
// and an epoch that raises MinBlockGasCost each pass their own bound check yet
// together produce Min > Max, which FeeConfig.Verify rejects. Without the
// fallback GetFeeConfigAt would start returning an error, header verification
// would fail, and the chain would stop — a governance vote must never be able
// to halt the chain, not even by accident, and especially not on purpose. The
// worst a hostile supermajority can achieve here is that its own change is
// ignored.
//
// This is the only way to read governed values into node behaviour: there is
// deliberately no exported path that yields an unverified config.
func Resolve(base commontype.FeeConfig, r Reader) (commontype.FeeConfig, bool) {
	candidate := overlay(base, r)
	if err := candidate.Verify(); err != nil {
		return base, false
	}
	return candidate, true
}

// overlay substitutes every governed value from [r] into [base], field by
// field. Unset params fall through, so a chain that has never voted behaves
// exactly as it does today. [base] is not mutated.
func overlay(base commontype.FeeConfig, r Reader) commontype.FeeConfig {
	out := base
	get := func(id ParamID) (uint64, bool) { return r.Param(id) }

	if v, ok := get(ParamGasLimit); ok {
		out.GasLimit = new(big.Int).SetUint64(v)
	}
	if v, ok := get(ParamTargetBlockRate); ok {
		out.TargetBlockRate = v
	}
	if v, ok := get(ParamMinBaseFee); ok {
		out.MinBaseFee = new(big.Int).SetUint64(v)
	}
	if v, ok := get(ParamTargetGas); ok {
		out.TargetGas = new(big.Int).SetUint64(v)
	}
	if v, ok := get(ParamBaseFeeChangeDenominator); ok {
		out.BaseFeeChangeDenominator = new(big.Int).SetUint64(v)
	}
	if v, ok := get(ParamMinBlockGasCost); ok {
		out.MinBlockGasCost = new(big.Int).SetUint64(v)
	}
	if v, ok := get(ParamMaxBlockGasCost); ok {
		out.MaxBlockGasCost = new(big.Int).SetUint64(v)
	}
	if v, ok := get(ParamBlockGasCostStep); ok {
		out.BlockGasCostStep = new(big.Int).SetUint64(v)
	}
	return out
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package stakeweight

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/luxfi/evm/accounts/abi"
	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/evm/predicate"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/math/set"
)

// GetVerifiedStakeWeightGasCost prices the state read Run() performs. The
// expensive work — two validator-set materialisations and a BLS verification —
// was already paid for in PredicateGas before the block was executed.
const GetVerifiedStakeWeightGasCost uint64 = 2_000

var (
	//go:embed contract.abi
	StakeWeightRawABI string

	StakeWeightABI = contract.ParseABI(StakeWeightRawABI)

	StakeWeightPrecompile = createStakeWeightPrecompile()

	errInvalidIndexInput = errors.New("invalid index to specify stake-weight ballot")

	invalidOutput []byte
)

// StakeWeight is the ABI struct returned to Solidity.
//
// weight and totalWeight are P-chain uint64 values widened to uint256 at the
// boundary: a tally sums many of them and multiplies by a numerator, and
// forcing that arithmetic through uint64 in Solidity would invite an overflow
// at every integration site. Widening is lossless.
type StakeWeight struct {
	NodeID       [20]byte
	Voter        common.Address
	AuthEpoch    uint64
	PChainHeight uint64
	Weight       *big.Int
	TotalWeight  *big.Int
}

func init() {
	res, err := PackGetVerifiedStakeWeightOutput(GetVerifiedStakeWeightOutput{Valid: false})
	if err != nil {
		panic(err)
	}
	invalidOutput = res
}

// GetVerifiedStakeWeightOutput is the full return tuple.
type GetVerifiedStakeWeightOutput struct {
	StakeWeight StakeWeight
	Valid       bool
}

// PackGetVerifiedStakeWeight packs [index] including the 4-byte selector.
func PackGetVerifiedStakeWeight(index uint32) ([]byte, error) {
	return StakeWeightABI.Pack("getVerifiedStakeWeight", index)
}

// UnpackGetVerifiedStakeWeightInput unpacks [input] (selector already stripped).
func UnpackGetVerifiedStakeWeightInput(input []byte) (uint32, error) {
	res, err := StakeWeightABI.UnpackInput("getVerifiedStakeWeight", input, false)
	if err != nil {
		return 0, err
	}
	return *abi.ConvertType(res[0], new(uint32)).(*uint32), nil
}

// PackGetVerifiedStakeWeightOutput packs [out] to conform to the ABI outputs.
func PackGetVerifiedStakeWeightOutput(out GetVerifiedStakeWeightOutput) ([]byte, error) {
	if out.StakeWeight.Weight == nil {
		out.StakeWeight.Weight = new(big.Int)
	}
	if out.StakeWeight.TotalWeight == nil {
		out.StakeWeight.TotalWeight = new(big.Int)
	}
	return StakeWeightABI.PackOutput("getVerifiedStakeWeight", out.StakeWeight, out.Valid)
}

// UnpackGetVerifiedStakeWeightOutput unpacks [output] as the return tuple.
func UnpackGetVerifiedStakeWeightOutput(output []byte) (GetVerifiedStakeWeightOutput, error) {
	out := GetVerifiedStakeWeightOutput{}
	err := StakeWeightABI.UnpackIntoInterface(&out, "getVerifiedStakeWeight", output)
	return out, err
}

// getVerifiedStakeWeight returns the ballot at [index] of the transaction's
// access list for this precompile, together with whether the block's committed
// predicate results attest that its claims held against P-chain state.
//
// Run() re-derives nothing. It reads the ballot the sender committed to and the
// verdict consensus already recorded, so every node computes the same answer
// from data inside the block.
func getVerifiedStakeWeight(accessibleState contract.AccessibleState, _ common.Address, _ common.Address, input []byte, suppliedGas uint64, _ bool) ([]byte, uint64, error) {
	remainingGas, err := contract.DeductGas(suppliedGas, GetVerifiedStakeWeightGasCost)
	if err != nil {
		return nil, remainingGas, err
	}
	index, err := UnpackGetVerifiedStakeWeightInput(input)
	if err != nil {
		return nil, remainingGas, fmt.Errorf("%w: %w", errInvalidIndexInput, err)
	}
	if index > math.MaxInt32 {
		return nil, remainingGas, fmt.Errorf("%w: larger than MaxInt32", errInvalidIndexInput)
	}

	state := accessibleState.GetStateDB()
	predicateBytes, exists := state.GetPredicateStorageSlots(ContractAddress, int(index))
	// A set bit means VerifyPredicate returned an error for that index.
	failed := set.BitsFromBytes(
		accessibleState.GetBlockContext().GetPredicateResults(state.GetTxHash(), ContractAddress),
	).Contains(int(index))
	if !exists || failed {
		return invalidOutput, remainingGas, nil
	}

	unpacked, err := predicate.UnpackPredicate(predicateBytes)
	if err != nil {
		// Unreachable: the same unpack succeeded during predicate verification.
		return nil, remainingGas, fmt.Errorf("%w: %w", errInvalidPredicateBytes, err)
	}
	ballot, err := ParseBallot(unpacked)
	if err != nil {
		return nil, remainingGas, err
	}

	packed, err := PackGetVerifiedStakeWeightOutput(GetVerifiedStakeWeightOutput{
		StakeWeight: StakeWeight{
			NodeID:       ballot.NodeID,
			Voter:        ballot.Voter,
			AuthEpoch:    ballot.AuthEpoch,
			PChainHeight: ballot.PChainHeight,
			Weight:       new(big.Int).SetUint64(ballot.Weight),
			TotalWeight:  new(big.Int).SetUint64(ballot.TotalWeight),
		},
		Valid: true,
	})
	if err != nil {
		return nil, remainingGas, err
	}
	return packed, remainingGas, nil
}

func createStakeWeightPrecompile() contract.StatefulPrecompiledContract {
	method, ok := StakeWeightABI.Methods["getVerifiedStakeWeight"]
	if !ok {
		panic("getVerifiedStakeWeight missing from the stakeweight ABI")
	}
	statefulContract, err := contract.NewStatefulPrecompileContract(nil, []*contract.StatefulPrecompileFunction{
		contract.NewStatefulPrecompileFunction(method.ID, getVerifiedStakeWeight),
	})
	if err != nil {
		panic(err)
	}
	return statefulContract
}

// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package vm

import (
	"errors"
	"fmt"
	"math/big"
	
	"github.com/luxfi/evm/v2/v2/precompile/contract"
	"github.com/luxfi/evm/v2/v2/vmerrs"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
)

// wrappedPrecompiledContract implements StatefulPrecompiledContract by wrapping stateless native precompiled contracts
// in Ethereum.
type wrappedPrecompiledContract struct {
	p PrecompiledContract
}

// newWrappedPrecompiledContract returns a wrapped version of [PrecompiledContract] to be executed according to the StatefulPrecompiledContract
// interface.
func newWrappedPrecompiledContract(p PrecompiledContract) contract.StatefulPrecompiledContract {
	return &wrappedPrecompiledContract{p: p}
}

// Run implements the StatefulPrecompiledContract interface
func (w *wrappedPrecompiledContract) Run(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	return RunPrecompiledContract(w.p, input, suppliedGas)
}

// RunStatefulPrecompiledContract confirms runs [precompile] with the specified parameters.
func RunStatefulPrecompiledContract(precompile contract.StatefulPrecompiledContract, accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	return precompile.Run(accessibleState, caller, addr, input, suppliedGas, readOnly)
}

// deprecatedContract is a placeholder for deprecated precompiled contracts
type deprecatedContract struct{}

func (d *deprecatedContract) Run(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	return nil, suppliedGas, errors.New("deprecated contract")
}

// nativeAssetBalance is the precompiled contract for native asset balance queries
type nativeAssetBalance struct {
	gasCost uint64
}

func (n *nativeAssetBalance) Run(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	// Native asset balance implementation
	// This is deprecated but kept for compatibility
	if suppliedGas < n.gasCost {
		return nil, 0, vmerrs.ErrOutOfGas
	}
	return nil, suppliedGas - n.gasCost, errors.New("deprecated contract")
}

// nativeAssetCall is the precompiled contract for native asset transfers
type nativeAssetCall struct {
	gasCost uint64
}

func (n *nativeAssetCall) Run(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	// Native asset call implementation
	// This is deprecated but kept for compatibility
	if suppliedGas < n.gasCost {
		return nil, 0, vmerrs.ErrOutOfGas
	}
	return nil, suppliedGas - n.gasCost, errors.New("deprecated contract")
}

// PackNativeAssetCallInput packs the input for a native asset call
func PackNativeAssetCallInput(to common.Address, assetID ids.ID, assetAmount *big.Int, callData []byte) []byte {
	input := make([]byte, 0, 84+len(callData))
	input = append(input, to.Bytes()...)
	input = append(input, assetID[:]...)
	amountBytes := common.LeftPadBytes(assetAmount.Bytes(), 32)
	input = append(input, amountBytes...)
	input = append(input, callData...)
	return input
}

// UnpackNativeAssetCallInput unpacks the input for a native asset call
func UnpackNativeAssetCallInput(input []byte) (common.Address, ids.ID, *big.Int, []byte, error) {
	if len(input) < 84 {
		return common.Address{}, ids.ID{}, nil, nil, fmt.Errorf("input too short: %d < 84", len(input))
	}
	to := common.BytesToAddress(input[:20])
	assetID, err := ids.ToID(input[20:52])
	if err != nil {
		return common.Address{}, ids.ID{}, nil, nil, err
	}
	assetAmount := new(big.Int).SetBytes(input[52:84])
	callData := input[84:]
	return to, assetID, assetAmount, callData, nil
}

// PackNativeAssetBalanceInput packs the input for native asset balance query
func PackNativeAssetBalanceInput(address common.Address, assetID ids.ID) []byte {
	input := make([]byte, 52)
	copy(input[:20], address.Bytes())
	copy(input[20:], assetID[:])
	return input
}

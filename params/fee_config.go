// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package params

import (
	"fmt"
	"math/big"
)

// DynamicFeeConfig implements Octane (ACP-176) style dynamic fees
type DynamicFeeConfig struct {
	// TargetGas is the target gas usage per block (50% full)
	TargetGas uint64 `json:"targetGas"`
	
	// BaseFeeChangeDenominator controls fee adjustment rate
	BaseFeeChangeDenominator uint64 `json:"baseFeeChangeDenominator"`
	
	// MinBaseFee is the minimum base fee (25 gwei for Lux)
	MinBaseFee *big.Int `json:"minBaseFee"`
	
	// TargetBlockRate is the target time between blocks (2 seconds)
	TargetBlockRate uint64 `json:"targetBlockRate"`
	
	// BlockGasCostStep for base fee calculation
	BlockGasCostStep *big.Int `json:"blockGasCostStep"`
}

var (
	// DefaultDynamicFeeConfig for Lux mainnet (Octane/ACP-176 style)
	DefaultDynamicFeeConfig = DynamicFeeConfig{
		TargetGas:                50_000_000, // 50M gas target (50% of 100M block gas limit)
		BaseFeeChangeDenominator: 36,         // Smooth fee changes
		MinBaseFee:               big.NewInt(25_000_000_000), // 25 gwei minimum
		TargetBlockRate:          2,                          // 2 second blocks
		BlockGasCostStep:         big.NewInt(50_000),
	}

	// All legacy static fee configs are removed - only dynamic fees supported
)

// Verify validates the fee configuration
func (f *DynamicFeeConfig) Verify() error {
	if f.TargetGas == 0 {
		return fmt.Errorf("targetGas cannot be 0")
	}
	if f.BaseFeeChangeDenominator == 0 {
		return fmt.Errorf("baseFeeChangeDenominator cannot be 0")
	}
	if f.MinBaseFee == nil || f.MinBaseFee.Sign() < 0 {
		return fmt.Errorf("minBaseFee must be positive")
	}
	if f.TargetBlockRate == 0 {
		return fmt.Errorf("targetBlockRate cannot be 0")
	}
	return nil
}

// BaseFeeCalculator is an interface for headers used in base fee calculation
type BaseFeeCalculator interface {
	GetBaseFee() *big.Int
	GetGasUsed() uint64
}

// CalcBaseFee calculates the base fee for the next block based on parent
func CalcBaseFee(config *DynamicFeeConfig, parent BaseFeeCalculator) *big.Int {
	// If parent has no basefee, use initial base fee
	parentFee := parent.GetBaseFee()
	if parentFee == nil {
		return new(big.Int).Set(config.MinBaseFee)
	}

	parentGasUsed := parent.GetGasUsed()
	targetGas := config.TargetGas
	changeDenominator := config.BaseFeeChangeDenominator

	// Calculate new base fee based on gas usage vs target
	var baseFee *big.Int
	if parentGasUsed > targetGas {
		// Increase base fee if over target
		gasUsedDelta := new(big.Int).SetUint64(parentGasUsed - targetGas)
		x := new(big.Int).Mul(parentFee, gasUsedDelta)
		y := x.Div(x, new(big.Int).SetUint64(targetGas))
		baseFeeDelta := y.Div(y, new(big.Int).SetUint64(changeDenominator))

		baseFee = new(big.Int).Add(parentFee, baseFeeDelta)
	} else {
		// Decrease base fee if under target
		gasUsedDelta := new(big.Int).SetUint64(targetGas - parentGasUsed)
		x := new(big.Int).Mul(parentFee, gasUsedDelta)
		y := x.Div(x, new(big.Int).SetUint64(targetGas))
		baseFeeDelta := y.Div(y, new(big.Int).SetUint64(changeDenominator))

		baseFee = new(big.Int).Sub(parentFee, baseFeeDelta)
		if baseFee.Cmp(config.MinBaseFee) < 0 {
			baseFee = new(big.Int).Set(config.MinBaseFee)
		}
	}

	return baseFee
}
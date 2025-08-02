// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package header

import (
	"errors"
	"fmt"

	"github.com/luxfi/evm/v2/commontype"
	"github.com/luxfi/evm/v2/core/types"
	evmparams "github.com/luxfi/evm/v2/params"
	"github.com/luxfi/geth/params"
)

var (
	errInvalidGasUsed  = errors.New("invalid gas used")
	errInvalidGasLimit = errors.New("invalid gas limit")
)

// absDiff returns the absolute difference between two uint64 values
func absDiff(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

type CalculateGasLimitFunc func(parentGasUsed, parentGasLimit, gasFloor, gasCeil uint64) uint64

// GasLimit takes the previous header and the timestamp of its child block and
// calculates the gas limit for the child interfaces.
func GasLimit(
	config *evmparams.ChainConfig,
	feeConfig commontype.FeeConfig,
	parent *types.Header,
	timestamp uint64,
) (uint64, error) {
	switch {
	case config.IsEVM(timestamp):
		return feeConfig.GasLimit.Uint64(), nil
	default:
		// since all chains have activated EVM,
		// this code is not used in production. To avoid a dependency on the
		// `core` package, this code is modified to just return the parent gas
		// limit; which was valid to do prior to EVM.
		return parent.GasLimit, nil
	}
}

// VerifyGasUsed verifies that the gas used is less than or equal to the gas
// limit.
func VerifyGasUsed(
	config *evmparams.ChainConfig,
	feeConfig commontype.FeeConfig,
	parent *types.Header,
	header *types.Header,
) error {
	gasUsed := header.GasUsed
	capacity, err := GasCapacity(config, feeConfig, parent, header.Time)
	if err != nil {
		return fmt.Errorf("calculating gas capacity: %w", err)
	}
	if gasUsed > capacity {
		return fmt.Errorf("%w: have %d, capacity %d",
			errInvalidGasUsed,
			gasUsed,
			capacity,
		)
	}
	return nil
}

// VerifyGasLimit verifies that the gas limit for the header is valid.
func VerifyGasLimit(
	config *evmparams.ChainConfig,
	feeConfig commontype.FeeConfig,
	parent *types.Header,
	header *types.Header,
) error {
	switch {
	case config.IsEVM(header.Time):
		expectedGasLimit := feeConfig.GasLimit.Uint64()
		if header.GasLimit != expectedGasLimit {
			return fmt.Errorf("%w: expected to be %d in EVM, but found %d",
				errInvalidGasLimit,
				expectedGasLimit,
				header.GasLimit,
			)
		}
	default:
		if header.GasLimit < params.MinGasLimit || header.GasLimit > params.MaxGasLimit {
			return fmt.Errorf("%w: %d not in range [%d, %d]",
				errInvalidGasLimit,
				header.GasLimit,
				params.MinGasLimit,
				params.MaxGasLimit,
			)
		}

		// Verify that the gas limit remains within allowed bounds
		diff := absDiff(parent.GasLimit, header.GasLimit)
		limit := parent.GasLimit / params.GasLimitBoundDivisor
		if diff >= limit {
			return fmt.Errorf("%w: have %d, want %d += %d",
				errInvalidGasLimit,
				header.GasLimit,
				parent.GasLimit,
				limit,
			)
		}
	}
	return nil
}

// GasCapacity takes the previous header and the timestamp of its child block
// and calculates the available gas that can be consumed in the child interfaces.
func GasCapacity(
	config *evmparams.ChainConfig,
	feeConfig commontype.FeeConfig,
	parent *types.Header,
	timestamp uint64,
) (uint64, error) {
	return GasLimit(config, feeConfig, parent, timestamp)
}

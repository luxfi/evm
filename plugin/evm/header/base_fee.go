// (c) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package header

import (
	"errors"
	"math/big"

	"github.com/luxfi/evm/v2/v2/commontype"
	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/luxfi/evm/v2/v2/params/extras"
)

var errEstimateBaseFeeWithoutActivation = errors.New("cannot estimate base fee for chain without activation scheduled")

// BaseFee takes the previous header and the timestamp of its child block and
// calculates the expected base fee for the child block.
//
// Prior to EVM, the returned base fee will be nil.
func BaseFee(
	config *extras.ChainConfig,
	feeConfig commontype.FeeConfig,
	parent *types.Header,
	timestamp uint64,
) (*big.Int, error) {
	switch {
	case config.IsEVM(timestamp):
		return baseFeeFromWindow(config, feeConfig, parent, timestamp)
	default:
		// Prior to EVM the expected base fee is nil.
		return nil, nil
	}
}

// EstimateNextBaseFee attempts to estimate the base fee of a block built at
// `timestamp` on top of `parent`.
//
// If timestamp is before parent.Time or the EVM activation time, then timestamp
// is set to the maximum of parent.Time and the EVM activation time.
//
// Warning: This function should only be used in estimation and should not be
// used when calculating the canonical base fee for a block.
func EstimateNextBaseFee(
	config *extras.ChainConfig,
	feeConfig commontype.FeeConfig,
	parent *types.Header,
	timestamp uint64,
) (*big.Int, error) {
	// For v2.0.0, all upgrades are active at genesis (timestamp 0)
	// So we don't need to check EVMTimestamp
	timestamp = max(timestamp, parent.Time, 0)
	return BaseFee(config, feeConfig, parent, timestamp)
}

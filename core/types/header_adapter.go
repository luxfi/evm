// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"math/big"
)

// GetBaseFee returns the base fee of the header
func (h *Header) GetBaseFee() *big.Int {
	return h.BaseFee
}

// GetGasUsed returns the gas used in the header
func (h *Header) GetGasUsed() uint64 {
	return h.GasUsed
}
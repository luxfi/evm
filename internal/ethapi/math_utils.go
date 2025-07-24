// Copyright 2025 Lux Industries, Inc.
// This file contains math utility functions.

package ethapi

import (
	"math"
	"math/big"
)

// BigMax returns the larger of x or y.
func BigMax(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return x
	}
	return y
}

// BigMin returns the smaller of x or y.
func BigMin(x, y *big.Int) *big.Int {
	if x.Cmp(y) < 0 {
		return x
	}
	return y
}

// MaxInt64 is the maximum value for an int64.
const MaxInt64 = math.MaxInt64
// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package set

import (
	"encoding/hex"
	"math/big"
)

// Bits is a bit-set backed by a big.Int
// Holds values ranging from [0, INT_MAX] (arch-dependent)
// Trying to use negative values will result in a panic.
// This implementation is NOT thread-safe.
type Bits struct {
	bits *big.Int
}

// NewBits returns a new instance of Bits with [bits] set to 1.
//
// Invariants:
// 1. Negative bits will cause a panic.
// 2. Duplicate bits are allowed but will cause a no-op.
func NewBits(bits ...int) Bits {
	b := Bits{new(big.Int)}
	for _, bit := range bits {
		b.Add(bit)
	}
	return b
}

// BitsFromBytes returns a Bits from bytes representation
func BitsFromBytes(bytes []byte) Bits {
	return Bits{
		bits: new(big.Int).SetBytes(bytes),
	}
}

// Add sets the [i]'th bit to 1
func (b Bits) Add(i int) {
	b.bits.SetBit(b.bits, i, 1)
}

// Contains returns true if the [i]'th bit is 1, false otherwise
func (b Bits) Contains(i int) bool {
	return b.bits.Bit(i) == 1
}

// Remove sets the [i]'th bit to 0
func (b Bits) Remove(i int) {
	b.bits.SetBit(b.bits, i, 0)
}

// Clear empties the bitset
func (b Bits) Clear() {
	b.bits.SetInt64(0)
}

// Len returns the number of bits set to 1
func (b Bits) Len() int {
	bitLen := b.bits.BitLen()
	count := 0
	for i := 0; i < bitLen; i++ {
		if b.bits.Bit(i) == 1 {
			count++
		}
	}
	return count
}

// Bytes returns the byte representation of this bitset
func (b Bits) Bytes() []byte {
	return b.bits.Bytes()
}

// String returns the hex representation of this bitset
func (b Bits) String() string {
	return hex.EncodeToString(b.bits.Bytes())
}
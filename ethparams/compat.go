// (c) 2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ethparams

// Compatibility constants for geth params that were removed or renamed
const (
	GasLimitBoundDivisor = 1024
	MinGasLimit          = 5000
	BlobTxBlobGasPerBlob = 131072 // Gas consumption of a single data blob (== blob byte size)
)
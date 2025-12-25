// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package mldsa

import (
	"errors"
	"fmt"

	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/geth/common"
)

var (
	// Singleton instance
	MLDSAVerifyPrecompile = &mldsaVerifyPrecompile{}

	_ contract.StatefulPrecompiledContract = &mldsaVerifyPrecompile{}

	ErrInvalidInputLength = errors.New("invalid input length")
	ErrUnsupportedMode    = errors.New("unsupported ML-DSA mode")
)

// ML-DSA mode bytes
const (
	ModeMLDSA44 uint8 = 0x44
	ModeMLDSA65 uint8 = 0x65
	ModeMLDSA87 uint8 = 0x87
)

// Gas costs for ML-DSA verification (per mode)
const (
	MLDSA44VerifyGas    uint64 = 75_000  // Fastest, smallest keys
	MLDSA65VerifyGas    uint64 = 100_000 // Medium
	MLDSA87VerifyGas    uint64 = 150_000 // Slowest, largest keys
	MLDSAVerifyPerByteGas uint64 = 10    // Cost per byte of message
)

// ML-DSA key and signature sizes per mode
const (
	// ML-DSA-44
	MLDSA44PublicKeySize  = 1312
	MLDSA44SignatureSize  = 2420

	// ML-DSA-65
	MLDSA65PublicKeySize  = 1952
	MLDSA65SignatureSize  = 3309

	// ML-DSA-87
	MLDSA87PublicKeySize  = 2592
	MLDSA87SignatureSize  = 4627
)

type mldsaVerifyPrecompile struct{}

// Address returns the address of the ML-DSA verify precompile
func (p *mldsaVerifyPrecompile) Address() common.Address {
	return ContractMLDSAVerifyAddress
}

// RequiredGas calculates the gas required for ML-DSA verification
func (p *mldsaVerifyPrecompile) RequiredGas(input []byte) uint64 {
	if len(input) < 1 {
		return MLDSA65VerifyGas // Default
	}

	// First byte is mode
	mode := input[0]
	var baseGas uint64
	switch mode {
	case ModeMLDSA44:
		baseGas = MLDSA44VerifyGas
	case ModeMLDSA65:
		baseGas = MLDSA65VerifyGas
	case ModeMLDSA87:
		baseGas = MLDSA87VerifyGas
	default:
		baseGas = MLDSA65VerifyGas
	}

	return baseGas
}

// Run implements the ML-DSA signature verification precompile
// Input format: [mode(1)][pubkey][msgLen(32)][signature][message]
func (p *mldsaVerifyPrecompile) Run(
	accessibleState contract.AccessibleState,
	caller common.Address,
	addr common.Address,
	input []byte,
	suppliedGas uint64,
	readOnly bool,
) ([]byte, uint64, error) {
	// Calculate required gas
	gasCost := p.RequiredGas(input)
	if suppliedGas < gasCost {
		return nil, 0, errors.New("out of gas")
	}

	if len(input) < 1 {
		return nil, suppliedGas - gasCost, ErrInvalidInputLength
	}

	// Parse mode byte
	modeByte := input[0]
	var mode mldsa.Mode
	var pubKeySize, sigSize int

	switch modeByte {
	case ModeMLDSA44:
		mode = mldsa.MLDSA44
		pubKeySize = MLDSA44PublicKeySize
		sigSize = MLDSA44SignatureSize
	case ModeMLDSA65:
		mode = mldsa.MLDSA65
		pubKeySize = MLDSA65PublicKeySize
		sigSize = MLDSA65SignatureSize
	case ModeMLDSA87:
		mode = mldsa.MLDSA87
		pubKeySize = MLDSA87PublicKeySize
		sigSize = MLDSA87SignatureSize
	default:
		return nil, suppliedGas - gasCost, fmt.Errorf("%w: 0x%02x", ErrUnsupportedMode, modeByte)
	}

	// Minimum input: mode(1) + pubkey + msgLen(32) + signature
	minSize := 1 + pubKeySize + 32 + sigSize
	if len(input) < minSize {
		return nil, suppliedGas - gasCost, fmt.Errorf("%w: expected at least %d bytes, got %d",
			ErrInvalidInputLength, minSize, len(input))
	}

	// Parse input
	publicKey := input[1 : 1+pubKeySize]
	messageLenBytes := input[1+pubKeySize : 1+pubKeySize+32]
	signature := input[1+pubKeySize+32 : 1+pubKeySize+32+sigSize]

	// Read message length
	messageLen := readUint256(messageLenBytes)

	// Validate total input size
	expectedSize := uint64(minSize) + messageLen
	if uint64(len(input)) != expectedSize {
		return nil, suppliedGas - gasCost, fmt.Errorf("%w: expected %d bytes total, got %d",
			ErrInvalidInputLength, expectedSize, len(input))
	}

	// Extract message
	message := input[minSize:]

	// Parse public key from bytes
	pub, err := mldsa.PublicKeyFromBytes(publicKey, mode)
	if err != nil {
		return nil, suppliedGas - gasCost, fmt.Errorf("invalid public key: %w", err)
	}

	// Verify signature
	valid := pub.Verify(message, signature, nil)

	// Return result as 32-byte word (1 = valid, 0 = invalid)
	result := make([]byte, 32)
	if valid {
		result[31] = 1
	}

	return result, suppliedGas - gasCost, nil
}

// readUint256 reads a big-endian uint256 as uint64
func readUint256(b []byte) uint64 {
	if len(b) != 32 {
		return 0
	}
	// Only read last 8 bytes (assume high bytes are 0 for reasonable message lengths)
	return uint64(b[24])<<56 | uint64(b[25])<<48 | uint64(b[26])<<40 | uint64(b[27])<<32 |
		uint64(b[28])<<24 | uint64(b[29])<<16 | uint64(b[30])<<8 | uint64(b[31])
}

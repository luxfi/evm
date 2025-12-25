// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Post-Quantum Cryptography Precompile Implementation

package pqcrypto

import (
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/crypto/mlkem"
	"github.com/luxfi/crypto/slhdsa"
	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/vm"
)

// ContractAddress is the address of the PQ crypto unified precompile
var ContractAddress = common.HexToAddress("0x0200000000000000000000000000000000000010")

// Function selectors (first 4 bytes of input)
const (
	MLDSAVerifySelector      = "mlds" // "mlds_verify"
	MLKEMEncapsulateSelector = "encp" // "encp_mlkem"
	MLKEMDecapsulateSelector = "decp" // "decp_mlkem"
	SLHDSAVerifySelector     = "slhs" // "slhs_verify"
)

// ML-DSA mode bytes
const (
	MLDSAMode44 uint8 = 0x44
	MLDSAMode65 uint8 = 0x65
	MLDSAMode87 uint8 = 0x87
)

// ML-KEM mode bytes
const (
	MLKEMMode512  uint8 = 0x00
	MLKEMMode768  uint8 = 0x01
	MLKEMMode1024 uint8 = 0x02
)

// SLH-DSA mode bytes
const (
	SLHDSAModeSHA2_128s  uint8 = 0x00
	SLHDSAModeSHA2_128f  uint8 = 0x01
	SLHDSAModeSHA2_192s  uint8 = 0x02
	SLHDSAModeSHA2_192f  uint8 = 0x03
	SLHDSAModeSHA2_256s  uint8 = 0x04
	SLHDSAModeSHA2_256f  uint8 = 0x05
	SLHDSAModeSHAKE_128s uint8 = 0x10
	SLHDSAModeSHAKE_128f uint8 = 0x11
	SLHDSAModeSHAKE_192s uint8 = 0x12
	SLHDSAModeSHAKE_192f uint8 = 0x13
	SLHDSAModeSHAKE_256s uint8 = 0x14
	SLHDSAModeSHAKE_256f uint8 = 0x15
)

// Gas costs for ML-DSA verification (per mode)
const (
	MLDSA44VerifyGas uint64 = 75_000
	MLDSA65VerifyGas uint64 = 100_000
	MLDSA87VerifyGas uint64 = 150_000
	MLDSADefaultGas  uint64 = 100_000
)

// Gas costs for ML-KEM operations (per mode)
const (
	MLKEM512EncapsulateGas  uint64 = 6_000
	MLKEM768EncapsulateGas  uint64 = 8_000
	MLKEM1024EncapsulateGas uint64 = 10_000
	MLKEM512DecapsulateGas  uint64 = 6_000
	MLKEM768DecapsulateGas  uint64 = 8_000
	MLKEM1024DecapsulateGas uint64 = 10_000
	MLKEMDefaultGas         uint64 = 8_000
)

// Gas costs for SLH-DSA verification (per mode)
const (
	SLHDSA128sVerifyGas uint64 = 50_000
	SLHDSA128fVerifyGas uint64 = 75_000
	SLHDSA192sVerifyGas uint64 = 100_000
	SLHDSA192fVerifyGas uint64 = 150_000
	SLHDSA256sVerifyGas uint64 = 175_000
	SLHDSA256fVerifyGas uint64 = 250_000
	SLHDSADefaultGas    uint64 = 100_000
)

var (
	_ contract.StatefulPrecompiledContract = &pqCryptoPrecompile{}

	// Singleton instance
	PQCryptoPrecompile = &pqCryptoPrecompile{}

	errInvalidInput     = errors.New("invalid input")
	errInvalidSignature = errors.New("invalid signature")
)

type pqCryptoPrecompile struct{}

// Address returns the address of the PQ crypto precompile
func (p *pqCryptoPrecompile) Address() common.Address {
	return ContractAddress
}

// RequiredGas calculates the gas required for the given input
func (p *pqCryptoPrecompile) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return 0
	}

	// Parse function selector (first 4 bytes)
	selector := string(input[:4])
	data := input[4:]

	switch selector {
	case MLDSAVerifySelector:
		return p.mldsaRequiredGas(data)
	case MLKEMEncapsulateSelector:
		return p.mlkemEncapsulateRequiredGas(data)
	case MLKEMDecapsulateSelector:
		return p.mlkemDecapsulateRequiredGas(data)
	case SLHDSAVerifySelector:
		return p.slhdsaRequiredGas(data)
	default:
		return 0
	}
}

// mldsaRequiredGas calculates gas for ML-DSA verification based on mode
func (p *pqCryptoPrecompile) mldsaRequiredGas(input []byte) uint64 {
	if len(input) < 1 {
		return MLDSADefaultGas
	}
	mode := input[0]
	switch mode {
	case MLDSAMode44:
		return MLDSA44VerifyGas
	case MLDSAMode65:
		return MLDSA65VerifyGas
	case MLDSAMode87:
		return MLDSA87VerifyGas
	default:
		return MLDSADefaultGas
	}
}

// mlkemEncapsulateRequiredGas calculates gas for ML-KEM encapsulation based on mode
func (p *pqCryptoPrecompile) mlkemEncapsulateRequiredGas(input []byte) uint64 {
	if len(input) < 1 {
		return MLKEMDefaultGas
	}
	mode := input[0]
	switch mode {
	case MLKEMMode512:
		return MLKEM512EncapsulateGas
	case MLKEMMode768:
		return MLKEM768EncapsulateGas
	case MLKEMMode1024:
		return MLKEM1024EncapsulateGas
	default:
		return MLKEMDefaultGas
	}
}

// mlkemDecapsulateRequiredGas calculates gas for ML-KEM decapsulation based on mode
func (p *pqCryptoPrecompile) mlkemDecapsulateRequiredGas(input []byte) uint64 {
	if len(input) < 1 {
		return MLKEMDefaultGas
	}
	mode := input[0]
	switch mode {
	case MLKEMMode512:
		return MLKEM512DecapsulateGas
	case MLKEMMode768:
		return MLKEM768DecapsulateGas
	case MLKEMMode1024:
		return MLKEM1024DecapsulateGas
	default:
		return MLKEMDefaultGas
	}
}

// slhdsaRequiredGas calculates gas for SLH-DSA verification based on mode
func (p *pqCryptoPrecompile) slhdsaRequiredGas(input []byte) uint64 {
	if len(input) < 1 {
		return SLHDSADefaultGas
	}
	mode := input[0]
	switch mode {
	case SLHDSAModeSHA2_128s, SLHDSAModeSHAKE_128s:
		return SLHDSA128sVerifyGas
	case SLHDSAModeSHA2_128f, SLHDSAModeSHAKE_128f:
		return SLHDSA128fVerifyGas
	case SLHDSAModeSHA2_192s, SLHDSAModeSHAKE_192s:
		return SLHDSA192sVerifyGas
	case SLHDSAModeSHA2_192f, SLHDSAModeSHAKE_192f:
		return SLHDSA192fVerifyGas
	case SLHDSAModeSHA2_256s, SLHDSAModeSHAKE_256s:
		return SLHDSA256sVerifyGas
	case SLHDSAModeSHA2_256f, SLHDSAModeSHAKE_256f:
		return SLHDSA256fVerifyGas
	default:
		return SLHDSADefaultGas
	}
}

// Run executes the precompile with the given input
func (p *pqCryptoPrecompile) Run(accessibleState contract.AccessibleState, caller common.Address, addr common.Address, input []byte, suppliedGas uint64, readOnly bool) (ret []byte, remainingGas uint64, err error) {
	if len(input) < 4 {
		return nil, suppliedGas, errInvalidInput
	}

	// Calculate required gas
	requiredGas := p.RequiredGas(input)
	if suppliedGas < requiredGas {
		return nil, 0, vm.ErrOutOfGas
	}
	remainingGas = suppliedGas - requiredGas

	// Parse function selector
	selector := string(input[:4])
	data := input[4:]

	switch selector {
	case MLDSAVerifySelector[:4]:
		return p.mldsaVerify(data)
	case MLKEMEncapsulateSelector[:4]:
		return p.mlkemEncapsulate(data)
	case MLKEMDecapsulateSelector[:4]:
		return p.mlkemDecapsulate(data)
	case SLHDSAVerifySelector[:4]:
		return p.slhdsaVerify(data)
	default:
		return nil, remainingGas, fmt.Errorf("unknown function selector: %x", selector)
	}
}

// mldsaVerify verifies an ML-DSA signature
func (p *pqCryptoPrecompile) mldsaVerify(input []byte) ([]byte, uint64, error) {
	// Input format: [mode(1)] [pubkey_len(2)] [pubkey] [msg_len(2)] [msg] [sig]
	if len(input) < 6 {
		return nil, 0, errInvalidInput
	}

	// Convert precompile mode byte to library mode
	modeByte := input[0]
	var mode mldsa.Mode
	switch modeByte {
	case MLDSAMode44:
		mode = mldsa.MLDSA44
	case MLDSAMode65:
		mode = mldsa.MLDSA65
	case MLDSAMode87:
		mode = mldsa.MLDSA87
	default:
		return nil, 0, fmt.Errorf("invalid ML-DSA mode")
	}
	pubKeyLen := int(input[1])<<8 | int(input[2])

	if len(input) < 3+pubKeyLen+2 {
		return nil, 0, errInvalidInput
	}

	pubKeyBytes := input[3 : 3+pubKeyLen]
	msgLen := int(input[3+pubKeyLen])<<8 | int(input[3+pubKeyLen+1])

	if len(input) < 3+pubKeyLen+2+msgLen {
		return nil, 0, errInvalidInput
	}

	message := input[3+pubKeyLen+2 : 3+pubKeyLen+2+msgLen]
	signature := input[3+pubKeyLen+2+msgLen:]

	// Reconstruct public key
	pubKey, err := mldsa.PublicKeyFromBytes(pubKeyBytes, mode)
	if err != nil {
		return nil, 0, err
	}

	// Verify signature
	valid := pubKey.Verify(message, signature, nil)
	if valid {
		return []byte{1}, 0, nil
	}
	return []byte{0}, 0, nil
}

// mlkemEncapsulate performs ML-KEM encapsulation
func (p *pqCryptoPrecompile) mlkemEncapsulate(input []byte) ([]byte, uint64, error) {
	// Input format: [mode(1)] [pubkey]
	if len(input) < 2 {
		return nil, 0, errInvalidInput
	}

	mode := mlkem.Mode(input[0])
	pubKeyBytes := input[1:]

	// Reconstruct public key
	pubKey, err := mlkem.PublicKeyFromBytes(pubKeyBytes, mode)
	if err != nil {
		return nil, 0, err
	}

	// Encapsulate - returns ciphertext, sharedSecret, error (3 values)
	ciphertext, sharedSecret, err := pubKey.Encapsulate(rand.Reader)
	if err != nil {
		return nil, 0, err
	}

	// Return ciphertext + shared secret
	output := append(ciphertext, sharedSecret...)
	return output, 0, nil
}

// mlkemDecapsulate performs ML-KEM decapsulation
func (p *pqCryptoPrecompile) mlkemDecapsulate(input []byte) ([]byte, uint64, error) {
	// Input format: [mode(1)] [privkey_len(2)] [privkey] [ciphertext]
	if len(input) < 4 {
		return nil, 0, errInvalidInput
	}

	mode := mlkem.Mode(input[0])
	privKeyLen := int(input[1])<<8 | int(input[2])

	if len(input) < 3+privKeyLen {
		return nil, 0, errInvalidInput
	}

	privKeyBytes := input[3 : 3+privKeyLen]
	ciphertext := input[3+privKeyLen:]

	// Reconstruct private key
	privKey, err := mlkem.PrivateKeyFromBytes(privKeyBytes, mode)
	if err != nil {
		return nil, 0, err
	}

	// Decapsulate
	sharedSecret, err := privKey.Decapsulate(ciphertext)
	if err != nil {
		return nil, 0, err
	}

	return sharedSecret, 0, nil
}

// slhdsaVerify verifies an SLH-DSA signature
func (p *pqCryptoPrecompile) slhdsaVerify(input []byte) ([]byte, uint64, error) {
	// Similar to mldsaVerify but for SLH-DSA
	if len(input) < 6 {
		return nil, 0, errInvalidInput
	}

	mode := slhdsa.Mode(input[0])
	pubKeyLen := int(input[1])<<8 | int(input[2])

	if len(input) < 3+pubKeyLen+2 {
		return nil, 0, errInvalidInput
	}

	pubKeyBytes := input[3 : 3+pubKeyLen]
	msgLen := int(input[3+pubKeyLen])<<8 | int(input[3+pubKeyLen+1])

	if len(input) < 3+pubKeyLen+2+msgLen {
		return nil, 0, errInvalidInput
	}

	message := input[3+pubKeyLen+2 : 3+pubKeyLen+2+msgLen]
	signature := input[3+pubKeyLen+2+msgLen:]

	// Reconstruct public key
	pubKey, err := slhdsa.PublicKeyFromBytes(pubKeyBytes, mode)
	if err != nil {
		return nil, 0, err
	}

	// Verify signature
	valid := pubKey.Verify(message, signature, nil)
	if valid {
		return []byte{1}, 0, nil
	}
	return []byte{0}, 0, nil
}

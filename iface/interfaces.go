// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package interfaces

import (
	"errors"
	"math"
	
	"github.com/luxfi/node/ids"
	"github.com/luxfi/node/vms/platformvm/warp"
	"github.com/luxfi/node/vms/platformvm/warp/payload"
)

// Re-export warp types for compatibility
type (
	UnsignedMessage = warp.UnsignedMessage
	WarpSignedMessage = warp.Message
	Signature      = warp.Signature
)

// Re-export payload types
type (
	AddressedCall = payload.AddressedCall
	Hash          = payload.Hash
)

// ParseMessage parses raw bytes into a warp message
func ParseMessage(bytes []byte) (*WarpSignedMessage, error) {
	return warp.ParseMessage(bytes)
}

// ParseUnsignedMessage parses raw bytes into an unsigned warp message
func ParseUnsignedMessage(bytes []byte) (*UnsignedMessage, error) {
	return warp.ParseUnsignedMessage(bytes)
}

// Parse parses a warp payload
func Parse(bytes []byte) (payload.Payload, error) {
	return payload.Parse(bytes)
}

// ParseAddressedCall parses an addressed call payload
func ParseAddressedCall(bytes []byte) (*AddressedCall, error) {
	p, err := payload.Parse(bytes)
	if err != nil {
		return nil, err
	}
	addressedCall, ok := p.(*AddressedCall)
	if !ok {
		return nil, errors.New("not an addressed call")
	}
	return addressedCall, nil
}

// ParseHash parses a hash payload
func ParseHash(bytes []byte) (*Hash, error) {
	p, err := payload.Parse(bytes)
	if err != nil {
		return nil, err
	}
	hash, ok := p.(*Hash)
	if !ok {
		return nil, errors.New("not a hash payload")
	}
	return hash, nil
}

// SignatureFromBytes creates a signature from bytes
func SignatureFromBytes(bytes []byte) (Signature, error) {
	// This is a stub - proper BLS signature parsing would go here
	return nil, errors.New("SignatureFromBytes not implemented")
}

// AggregateSignatures aggregates multiple BLS signatures
func AggregateSignatures(sigs []Signature) (Signature, error) {
	// This is a stub - proper implementation would aggregate BLS signatures
	if len(sigs) == 0 {
		return nil, errors.New("no signatures to aggregate")
	}
	return sigs[0], nil
}

// NewUnsignedMessage creates a new unsigned warp message
func NewUnsignedMessage(networkID uint32, chainID ids.ID, payload []byte) (*UnsignedMessage, error) {
	return warp.NewUnsignedMessage(networkID, chainID, payload)
}

// NewMessage creates a new signed warp message
func NewMessage(unsignedMsg *UnsignedMessage, sig Signature) (*WarpSignedMessage, error) {
	return warp.NewMessage(unsignedMsg, sig)
}

// MaxUint64 is the maximum uint64 value
const MaxUint64 = ^uint64(0)

// Math helpers

// SafeMul multiplies two uint64s and returns overflow flag
func SafeMul(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, false
	}
	if a > math.MaxUint64/b {
		return 0, true
	}
	return a * b, false
}

// SafeAdd adds two uint64s and returns overflow flag  
func SafeAdd(a, b uint64) (uint64, bool) {
	if a > math.MaxUint64-b {
		return 0, true
	}
	return a + b, false
}

// WarpChainContext interface for warp consensus operations
type WarpChainContext interface {
	GetValidatorPublicKey(validationID ids.ID) ([]byte, error)
}

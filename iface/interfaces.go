// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package iface

import (
	"errors"
	"math"
	
	"github.com/luxfi/geth/common"
)

// ParseMessage parses raw bytes into a warp message
func ParseMessage(bytes []byte) (*WarpSignedMessage, error) {
	// TODO: Implement proper message parsing
	// For now, create a basic message structure
	if len(bytes) < 32 {
		return nil, errors.New("message too short")
	}
	
	msg := &WarpSignedMessage{
		UnsignedMessage: UnsignedMessage{
			Payload: bytes,
		},
		Signature: &BLSSignature{},
	}
	return msg, nil
}

// ParseUnsignedMessage parses raw bytes into an unsigned warp message
func ParseUnsignedMessage(bytes []byte) (*UnsignedMessage, error) {
	// TODO: Implement proper unsigned message parsing
	return &UnsignedMessage{
		Payload: bytes,
	}, nil
}

// Parse parses a warp payload
func Parse(bytes []byte) (interface{}, error) {
	// Try to parse as addressed call first
	addressedCall := &AddressedCall{}
	if err := addressedCall.Unmarshal(bytes); err == nil {
		return addressedCall, nil
	}
	
	// Try to parse as hash
	if len(bytes) == 32 {
		hash := &Hash{}
		copy(hash.Hash[:], bytes)
		return hash, nil
	}
	
	return nil, errors.New("unknown payload type")
}

// ParseAddressedCall parses an addressed call payload
func ParseAddressedCall(bytes []byte) (*AddressedCall, error) {
	addressedCall := &AddressedCall{}
	err := addressedCall.Unmarshal(bytes)
	if err != nil {
		return nil, err
	}
	return addressedCall, nil
}

// Hash represents a hash payload
type Hash struct {
	Hash [32]byte
}

// Bytes returns the byte representation of the hash
func (h *Hash) Bytes() []byte {
	return h.Hash[:]
}

// ParseHash parses a hash payload
func ParseHash(bytes []byte) (*Hash, error) {
	if len(bytes) != 32 {
		return nil, errors.New("invalid hash length")
	}
	hash := &Hash{}
	copy(hash.Hash[:], bytes)
	return hash, nil
}

// SignatureFromBytes creates a signature from bytes
func SignatureFromBytes(bytes []byte) (*BLSSignature, error) {
	return &BLSSignature{Bytes: bytes}, nil
}

// SignatureToBytes converts a signature to bytes
func SignatureToBytes(sig *BLSSignature) []byte {
	if sig == nil {
		return nil
	}
	return sig.Bytes
}

// AggregateSignatures aggregates multiple BLS signatures
func AggregateSignatures(sigs []*BLSSignature) (*BLSSignature, error) {
	if len(sigs) == 0 {
		return nil, errors.New("no signatures to aggregate")
	}
	// TODO: Implement proper BLS signature aggregation
	return sigs[0], nil
}

// NewUnsignedMessage creates a new unsigned warp message
func NewUnsignedMessage(networkID uint32, chainID interface{}, payload []byte) (*UnsignedMessage, error) {
	var sourceChainID common.Hash
	switch cid := chainID.(type) {
	case [32]byte:
		sourceChainID = common.Hash(cid)
	case ID:
		sourceChainID = common.Hash(cid)
	case common.Hash:
		sourceChainID = cid
	default:
		return nil, errors.New("unsupported chain ID type")
	}
	
	return &UnsignedMessage{
		NetworkID:     networkID,
		SourceChainID: sourceChainID,
		Payload:       payload,
	}, nil
}

// NewMessage creates a new signed warp message
func NewMessage(unsignedMsg *UnsignedMessage, sig interface{}) (*WarpSignedMessage, error) {
	var blsSig *BLSSignature
	switch s := sig.(type) {
	case *BLSSignature:
		blsSig = s
	case *BitSetSignature:
		// Convert BitSetSignature to BLSSignature
		blsSig = &BLSSignature{
			Bytes: s.Signature[:],
		}
	default:
		return nil, errors.New("unsupported signature type")
	}
	
	return &WarpSignedMessage{
		UnsignedMessage: *unsignedMsg,
		Signature:       blsSig,
		SourceChainID:   unsignedMsg.SourceChainID,
	}, nil
}

// MaxUint64 is the maximum uint64 value
const MaxUint64 = ^uint64(0)

// MaxInt64 is the maximum int64 value
const MaxInt64 = int64(^uint64(0) >> 1)

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

// MaxInt32 is the maximum int32 value
const MaxInt32 = math.MaxInt32

// WarpChainContext interface for warp consensus operations
type WarpChainContext interface {
	GetValidatorPublicKey(validationID [32]byte) ([]byte, error)
}

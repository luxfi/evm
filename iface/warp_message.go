// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package iface

import (
	"encoding/binary"
	"github.com/luxfi/geth/common"
)

// SignatureLen is the length of a BLS signature in bytes
const SignatureLen = 96

// UnsignedMessage represents an unsigned warp message
type UnsignedMessage struct {
	NetworkID     uint32
	SourceChainID common.Hash
	Payload       []byte
}


// ID returns the hash of the unsigned message
func (m *UnsignedMessage) ID() common.Hash {
	// TODO: Implement proper hashing
	return common.Hash{}
}

// Bytes returns the byte representation of the unsigned message
func (m *UnsignedMessage) Bytes() []byte {
	// Format: [NetworkID (4 bytes)][SourceChainID (32 bytes)][PayloadLength (4 bytes)][Payload]
	result := make([]byte, 4+32+4+len(m.Payload))
	binary.BigEndian.PutUint32(result[:4], m.NetworkID)
	copy(result[4:36], m.SourceChainID[:])
	binary.BigEndian.PutUint32(result[36:40], uint32(len(m.Payload)))
	copy(result[40:], m.Payload)
	return result
}

// WarpSignedMessage represents a signed warp message
type WarpSignedMessage struct {
	UnsignedMessage UnsignedMessage
	Signature       *BLSSignature
	SourceChainID   common.Hash
}


// ID returns the ID of the unsigned message
func (m *WarpSignedMessage) ID() common.Hash {
	return m.UnsignedMessage.ID()
}

// Payload returns the payload from the unsigned message
func (m *WarpSignedMessage) Payload() []byte {
	return m.UnsignedMessage.Payload
}

// Bytes returns the byte representation of the signed message
func (m *WarpSignedMessage) Bytes() []byte {
	unsignedBytes := m.UnsignedMessage.Bytes()
	signatureBytes := m.Signature.Bytes
	result := make([]byte, len(unsignedBytes)+len(signatureBytes))
	copy(result, unsignedBytes)
	copy(result[len(unsignedBytes):], signatureBytes)
	return result
}

// BitSetSignature represents a signature with bit set information
type BitSetSignature struct {
	Signers   []byte
	Signature [96]byte
}

// NewHash creates a new hash payload
func NewHash(hash ID) (*Hash, error) {
	return &Hash{
		Hash: common.Hash(hash),
	}, nil
}
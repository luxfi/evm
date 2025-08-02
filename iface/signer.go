// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package iface

// Signer interface for signing messages
type Signer interface {
	Sign(msg []byte) (*BLSSignature, error)
	PublicKey() *BLSPublicKey
}

// Signature is an alias for BLSSignature for backward compatibility
type Signature = BLSSignature
// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"github.com/luxfi/warp"
)

// SignerAdapter adapts a LocalSigner to the warp.Signer interface
type SignerAdapter struct {
	signer *LocalSigner
}

// NewSignerAdapter creates a new adapter
func NewSignerAdapter(signer *LocalSigner) *SignerAdapter {
	return &SignerAdapter{
		signer: signer,
	}
}

// Sign implements the warp.Signer interface
func (a *SignerAdapter) Sign(unsignedMsg *warp.UnsignedMessage) ([]byte, error) {
	return a.signer.SignUnsignedMessage(unsignedMsg)
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
)

var (
	errNoSignatures       = errors.New("no signatures collected")
	errInsufficientQuorum = errors.New("insufficient quorum")
)

// SignatureGetter fetches a signature for a warp message from a specific validator
type SignatureGetter interface {
	// GetSignature fetches a signature for the message from the given node
	GetSignature(ctx context.Context, nodeID ids.NodeID, unsignedMessage *warp.UnsignedMessage) ([]byte, error)
}

// ValidatorInfo contains validator information for signature aggregation
type ValidatorInfo struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	Weight    uint64
	Index     int
}

// SignatureAggregator aggregates BLS signatures from validators
type SignatureAggregator struct {
	signatureGetter SignatureGetter
}

// NewSignatureAggregator creates a new signature aggregator
func NewSignatureAggregator(signatureGetter SignatureGetter) *SignatureAggregator {
	return &SignatureAggregator{
		signatureGetter: signatureGetter,
	}
}

// AggregateSignatures collects signatures from validators and aggregates them
// Returns the signed message bytes if successful
func (a *SignatureAggregator) AggregateSignatures(
	ctx context.Context,
	unsignedMessage *warp.UnsignedMessage,
	validators []*ValidatorInfo,
	quorumNum uint64,
	quorumDen uint64,
) ([]byte, error) {
	if len(validators) == 0 {
		return nil, errNoValidators
	}

	// Calculate total weight
	var totalWeight uint64
	for _, v := range validators {
		newWeight, err := safeAddUint64(totalWeight, v.Weight)
		if err != nil {
			return nil, fmt.Errorf("total weight overflow: %w", err)
		}
		totalWeight = newWeight
	}

	// Required weight for quorum
	requiredWeight := (totalWeight * quorumNum) / quorumDen

	// Collect signatures concurrently
	type sigResult struct {
		index     int
		signature *bls.Signature
		weight    uint64
		err       error
	}

	results := make(chan sigResult, len(validators))
	var wg sync.WaitGroup

	for _, v := range validators {
		wg.Add(1)
		go func(validator *ValidatorInfo) {
			defer wg.Done()

			sigBytes, err := a.signatureGetter.GetSignature(ctx, validator.NodeID, unsignedMessage)
			if err != nil {
				results <- sigResult{index: validator.Index, err: err}
				return
			}

			sig, err := bls.SignatureFromBytes(sigBytes)
			if err != nil {
				results <- sigResult{index: validator.Index, err: fmt.Errorf("invalid signature bytes: %w", err)}
				return
			}

			// Verify signature against validator's public key
			msgBytes := unsignedMessage.Bytes()
			if !bls.Verify(validator.PublicKey, sig, msgBytes) {
				results <- sigResult{index: validator.Index, err: errors.New("signature verification failed")}
				return
			}

			results <- sigResult{index: validator.Index, signature: sig, weight: validator.Weight}
		}(v)
	}

	// Close results channel when all goroutines complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect valid signatures
	signers := warp.NewBitSet()
	signatures := make([]*bls.Signature, 0, len(validators))
	var signedWeight uint64

	for result := range results {
		if result.err != nil {
			// Log but continue - we may still reach quorum
			continue
		}

		signers.Add(result.index)
		signatures = append(signatures, result.signature)
		newWeight, err := safeAddUint64(signedWeight, result.weight)
		if err != nil {
			return nil, fmt.Errorf("signed weight overflow: %w", err)
		}
		signedWeight = newWeight
	}

	if len(signatures) == 0 {
		return nil, errNoSignatures
	}

	if signedWeight < requiredWeight {
		return nil, fmt.Errorf("%w: got %d/%d, need %d", errInsufficientQuorum, signedWeight, totalWeight, requiredWeight)
	}

	// Aggregate signatures
	aggSig, err := bls.AggregateSignatures(signatures)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate signatures: %w", err)
	}

	// Create BitSetSignature
	var aggSigBytes [bls.SignatureLen]byte
	copy(aggSigBytes[:], bls.SignatureToBytes(aggSig))

	bitSetSig := &warp.BitSetSignature{
		Signers:   signers,
		Signature: aggSigBytes,
	}

	// Create signed message
	signedMessage, err := warp.NewMessage(unsignedMessage, bitSetSig)
	if err != nil {
		return nil, fmt.Errorf("failed to create signed message: %w", err)
	}

	return signedMessage.Bytes(), nil
}

// safeAddUint64 adds two uint64 values with overflow check
func safeAddUint64(a, b uint64) (uint64, error) {
	result := a + b
	if result < a {
		return 0, errors.New("uint64 overflow")
	}
	return result, nil
}

// LocalSignatureGetter implements SignatureGetter using local backend
type LocalSignatureGetter struct {
	backend Backend
}

// NewLocalSignatureGetter creates a signature getter using the local backend
func NewLocalSignatureGetter(backend Backend) *LocalSignatureGetter {
	return &LocalSignatureGetter{backend: backend}
}

// GetSignature gets a signature from the local backend (this node)
func (g *LocalSignatureGetter) GetSignature(ctx context.Context, nodeID ids.NodeID, unsignedMessage *warp.UnsignedMessage) ([]byte, error) {
	return g.backend.GetMessageSignature(ctx, unsignedMessage)
}

// NetworkSignatureGetter implements SignatureGetter by fetching from network peers
type NetworkSignatureGetter struct {
	client RequestClient
}

// RequestClient sends requests to peers
type RequestClient interface {
	// SendRequest sends a request to a peer and waits for response
	SendRequest(ctx context.Context, nodeID ids.NodeID, request []byte) ([]byte, error)
}

// NewNetworkSignatureGetter creates a signature getter that fetches from network
func NewNetworkSignatureGetter(client RequestClient) *NetworkSignatureGetter {
	return &NetworkSignatureGetter{client: client}
}

// GetSignature fetches a signature from a network peer
func (g *NetworkSignatureGetter) GetSignature(ctx context.Context, nodeID ids.NodeID, unsignedMessage *warp.UnsignedMessage) ([]byte, error) {
	// Encode the signature request
	request := unsignedMessage.Bytes()

	// Send request to peer
	response, err := g.client.SendRequest(ctx, nodeID, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get signature from %s: %w", nodeID, err)
	}

	return response, nil
}

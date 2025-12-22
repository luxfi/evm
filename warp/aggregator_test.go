// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/warp"
	"github.com/stretchr/testify/require"
)

// mockSignatureGetter implements SignatureGetter for testing
type mockSignatureGetter struct {
	signatures map[ids.NodeID][]byte
	errors     map[ids.NodeID]error
	secretKeys map[ids.NodeID]*bls.SecretKey
}

func newMockSignatureGetter() *mockSignatureGetter {
	return &mockSignatureGetter{
		signatures: make(map[ids.NodeID][]byte),
		errors:     make(map[ids.NodeID]error),
		secretKeys: make(map[ids.NodeID]*bls.SecretKey),
	}
}

func (m *mockSignatureGetter) GetSignature(ctx context.Context, nodeID ids.NodeID, unsignedMessage *warp.UnsignedMessage) ([]byte, error) {
	if err, ok := m.errors[nodeID]; ok {
		return nil, err
	}
	if sig, ok := m.signatures[nodeID]; ok {
		return sig, nil
	}
	// Sign with secret key if available
	if sk, ok := m.secretKeys[nodeID]; ok {
		msgBytes := unsignedMessage.Bytes()
		sig, err := sk.Sign(msgBytes)
		if err != nil {
			return nil, err
		}
		return bls.SignatureToBytes(sig), nil
	}
	return nil, errors.New("no signature available")
}

func (m *mockSignatureGetter) addSigner(nodeID ids.NodeID, sk *bls.SecretKey) {
	m.secretKeys[nodeID] = sk
}

func (m *mockSignatureGetter) addError(nodeID ids.NodeID, err error) {
	m.errors[nodeID] = err
}

func TestSignatureAggregator_AggregateSignatures(t *testing.T) {
	require := require.New(t)

	// Create test validators with BLS keys
	numValidators := 3
	validators := make([]*ValidatorInfo, numValidators)
	secretKeys := make([]*bls.SecretKey, numValidators)

	mockGetter := newMockSignatureGetter()

	for i := 0; i < numValidators; i++ {
		sk, err := bls.NewSecretKey()
		require.NoError(err)
		secretKeys[i] = sk

		nodeID := ids.GenerateTestNodeID()
		validators[i] = &ValidatorInfo{
			NodeID:    nodeID,
			PublicKey: sk.PublicKey(),
			Weight:    100,
			Index:     i,
		}
		mockGetter.addSigner(nodeID, sk)
	}

	aggregator := NewSignatureAggregator(mockGetter)

	// Create test message
	networkID := uint32(1)
	sourceChainID := ids.GenerateTestID()
	payload := []byte("test payload")

	unsignedMsg, err := warp.NewUnsignedMessage(networkID, sourceChainID, payload)
	require.NoError(err)

	// Test successful aggregation with 67% quorum
	signedMsgBytes, err := aggregator.AggregateSignatures(
		context.Background(),
		unsignedMsg,
		validators,
		67, // 67%
		100,
	)
	require.NoError(err)
	require.NotEmpty(signedMsgBytes)

	// Verify the signed message can be parsed
	signedMsg, err := warp.ParseMessage(signedMsgBytes)
	require.NoError(err)
	require.NotNil(signedMsg)

	// Verify signature
	err = signedMsg.Signature.Verify(unsignedMsg.Bytes(), toWarpValidators(validators))
	require.NoError(err)
}

func TestSignatureAggregator_InsufficientQuorum(t *testing.T) {
	require := require.New(t)

	// Create 3 validators
	numValidators := 3
	validators := make([]*ValidatorInfo, numValidators)
	secretKeys := make([]*bls.SecretKey, numValidators)

	mockGetter := newMockSignatureGetter()

	for i := 0; i < numValidators; i++ {
		sk, err := bls.NewSecretKey()
		require.NoError(err)
		secretKeys[i] = sk

		nodeID := ids.GenerateTestNodeID()
		validators[i] = &ValidatorInfo{
			NodeID:    nodeID,
			PublicKey: sk.PublicKey(),
			Weight:    100,
			Index:     i,
		}
		// Only add signer for first validator
		if i == 0 {
			mockGetter.addSigner(nodeID, sk)
		} else {
			mockGetter.addError(nodeID, errors.New("validator unavailable"))
		}
	}

	aggregator := NewSignatureAggregator(mockGetter)

	// Create test message
	networkID := uint32(1)
	sourceChainID := ids.GenerateTestID()
	payload := []byte("test payload")

	unsignedMsg, err := warp.NewUnsignedMessage(networkID, sourceChainID, payload)
	require.NoError(err)

	// Try to aggregate with 67% quorum - should fail (only 1/3 validators available)
	_, err = aggregator.AggregateSignatures(
		context.Background(),
		unsignedMsg,
		validators,
		67,
		100,
	)
	require.Error(err)
	require.Contains(err.Error(), "insufficient quorum")
}

func TestSignatureAggregator_NoValidators(t *testing.T) {
	require := require.New(t)

	mockGetter := newMockSignatureGetter()
	aggregator := NewSignatureAggregator(mockGetter)

	networkID := uint32(1)
	sourceChainID := ids.GenerateTestID()
	payload := []byte("test payload")

	unsignedMsg, err := warp.NewUnsignedMessage(networkID, sourceChainID, payload)
	require.NoError(err)

	// Empty validator set
	_, err = aggregator.AggregateSignatures(
		context.Background(),
		unsignedMsg,
		[]*ValidatorInfo{},
		67,
		100,
	)
	require.Error(err)
	require.ErrorIs(err, errNoValidators)
}

func TestSignatureAggregator_AllValidatorsFail(t *testing.T) {
	require := require.New(t)

	// Create validators but make all fail
	numValidators := 3
	validators := make([]*ValidatorInfo, numValidators)

	mockGetter := newMockSignatureGetter()

	for i := 0; i < numValidators; i++ {
		sk, err := bls.NewSecretKey()
		require.NoError(err)

		nodeID := ids.GenerateTestNodeID()
		validators[i] = &ValidatorInfo{
			NodeID:    nodeID,
			PublicKey: sk.PublicKey(),
			Weight:    100,
			Index:     i,
		}
		mockGetter.addError(nodeID, errors.New("connection failed"))
	}

	aggregator := NewSignatureAggregator(mockGetter)

	networkID := uint32(1)
	sourceChainID := ids.GenerateTestID()
	payload := []byte("test payload")

	unsignedMsg, err := warp.NewUnsignedMessage(networkID, sourceChainID, payload)
	require.NoError(err)

	_, err = aggregator.AggregateSignatures(
		context.Background(),
		unsignedMsg,
		validators,
		67,
		100,
	)
	require.Error(err)
	require.ErrorIs(err, errNoSignatures)
}

// toWarpValidators converts ValidatorInfo to warp.Validator for verification
func toWarpValidators(infos []*ValidatorInfo) []*warp.Validator {
	result := make([]*warp.Validator, len(infos))
	for i, info := range infos {
		result[i] = &warp.Validator{
			PublicKey:      info.PublicKey,
			PublicKeyBytes: bls.PublicKeyToCompressedBytes(info.PublicKey),
			Weight:         info.Weight,
			NodeID:         info.NodeID,
		}
	}
	return result
}

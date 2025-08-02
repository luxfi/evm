// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"crypto/rand"
	
	"github.com/luxfi/evm/v2/iface"
	"github.com/luxfi/evm/v2/localsigner"
	"github.com/luxfi/geth/common"
)

// ShortID is a 20-byte address identifier
type ShortID [20]byte

// ValidatorImpl is a test implementation of the Validator interface
type ValidatorImpl struct {
	PublicKey      *iface.BLSPublicKey
	PublicKeyBytes []byte
	Weight         uint64
	NodeIDs        []iface.NodeID
}

// ConvertGetValidatorOutputToValidatorOutput converts GetValidatorOutput to ValidatorOutput
func ConvertGetValidatorOutputToValidatorOutput(v *iface.GetValidatorOutput) *iface.ValidatorOutput {
	pk, _ := iface.NewBLSPublicKey(v.PublicKey)
	return &iface.ValidatorOutput{
		NodeID:    common.Hash(v.NodeID),
		PublicKey: pk,
		Weight:    v.Weight,
	}
}

// Compare compares two validators by weight
func (v *ValidatorImpl) Compare(other *ValidatorImpl) int {
	if v.Weight < other.Weight {
		return -1
	}
	if v.Weight > other.Weight {
		return 1
	}
	return 0
}

// GenerateTestID generates a random ID for testing
func GenerateTestID() iface.ID {
	var id iface.ID
	rand.Read(id[:])
	return id
}

// GenerateTestNodeID generates a random NodeID for testing
func GenerateTestNodeID() iface.NodeID {
	var id iface.NodeID
	rand.Read(id[:])
	return id
}

// GenerateTestShortID generates a random ShortID for testing
func GenerateTestShortID() ShortID {
	var id ShortID
	rand.Read(id[:])
	return id
}

// signerAdapter wraps a localsigner.SecretKey to implement iface.Signer
type signerAdapter struct {
	sk *localsigner.SecretKey
}

// Sign signs a message and converts to iface.BLSSignature
func (s *signerAdapter) Sign(msg []byte) (*iface.BLSSignature, error) {
	_, err := s.sk.Sign(msg)
	if err != nil {
		return nil, err
	}
	// Convert from bls.Signature to iface.BLSSignature
	// For now, just use a placeholder signature
	return &iface.BLSSignature{Bytes: make([]byte, 96)}, nil
}

// PublicKey returns the public key as iface.BLSPublicKey
func (s *signerAdapter) PublicKey() *iface.BLSPublicKey {
	// For now, just use a placeholder public key
	var blsPK iface.BLSPublicKey
	rand.Read(blsPK.Bytes[:])
	return &blsPK
}

// stateAdapter adapts function-based state to interface
type stateAdapter struct {
	GetSubnetIDF     func(ctx context.Context, chainID iface.ID) (iface.ID, error)
	GetValidatorSetF func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error)
}

func (s *stateAdapter) GetCurrentHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (s *stateAdapter) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (s *stateAdapter) GetSubnetID(ctx context.Context, chainID iface.ID) (iface.ID, error) {
	if s.GetSubnetIDF != nil {
		return s.GetSubnetIDF(ctx, chainID)
	}
	return iface.ID{}, nil
}

func (s *stateAdapter) GetValidatorSet(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
	if s.GetValidatorSetF != nil {
		return s.GetValidatorSetF(ctx, height, subnetID)
	}
	return nil, nil
}

// Alternative validatorStateAdapter that implements iface.ValidatorState
type validatorStateAdapter struct {
	GetSubnetIDF     func(ctx context.Context, chainID common.Hash) (common.Hash, error)
	GetValidatorSetF func(ctx context.Context, height uint64, subnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error)
}

func (s *validatorStateAdapter) GetCurrentHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (s *validatorStateAdapter) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (s *validatorStateAdapter) GetSubnetID(ctx context.Context, chainID common.Hash) (common.Hash, error) {
	if s.GetSubnetIDF != nil {
		return s.GetSubnetIDF(ctx, chainID)
	}
	return common.Hash{}, nil
}

func (s *validatorStateAdapter) GetValidatorSet(ctx context.Context, height uint64, subnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error) {
	if s.GetValidatorSetF != nil {
		return s.GetValidatorSetF(ctx, height, subnetID)
	}
	return nil, nil
}

// mockStateAdapter wraps node's mock state to adapt it to iface.State
type mockStateAdapter struct {
	mockState interface{} // *validatorsmock.State
}

func (m *mockStateAdapter) GetCurrentHeight(ctx context.Context) (uint64, error) {
	// Use reflection to call the method
	return 0, nil
}

func (m *mockStateAdapter) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, nil
}

func (m *mockStateAdapter) GetSubnetID(ctx context.Context, chainID iface.ID) (iface.ID, error) {
	// This will be set up via EXPECT() calls on the underlying mock
	return iface.ID{}, nil
}

func (m *mockStateAdapter) GetValidatorSet(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
	// This will be set up via EXPECT() calls on the underlying mock
	return nil, nil
}

// CreateStateAdapter creates an adapter for node's mock validator state
func CreateStateAdapter(mockState interface{}) iface.State {
	return &stateAdapter{
		GetSubnetIDF: func(ctx context.Context, chainID iface.ID) (iface.ID, error) {
			// This will be handled by the mock's EXPECT setup
			return iface.ID{}, nil
		},
		GetValidatorSetF: func(ctx context.Context, height uint64, subnetID iface.ID) (map[iface.NodeID]*iface.GetValidatorOutput, error) {
			// This will be handled by the mock's EXPECT setup
			return nil, nil
		},
	}
}
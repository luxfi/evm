// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package warp

import (
	"context"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/evm/v2/v2/iface"
)

// ValidatorState wraps the validator state to provide special handling for the Primary Network
type ValidatorState struct {
	validatorState iface.ValidatorState
	subnetID       common.Hash
	chainID        common.Hash
	skipChainID    bool
}

// NewValidatorState creates a new State wrapper
func NewValidatorState(
	validatorState iface.ValidatorState,
	subnetID common.Hash,
	chainID common.Hash,
	skipChainID bool,
) *ValidatorState {
	return &ValidatorState{
		validatorState: validatorState,
		subnetID:       subnetID,
		chainID:        chainID,
		skipChainID:    skipChainID,
	}
}

// GetValidatorSet implements the ValidatorState interface
func (s *ValidatorState) GetValidatorSet(ctx context.Context, height uint64, subnetID common.Hash) (map[common.Hash]*iface.ValidatorOutput, error) {
	return s.validatorState.GetValidatorSet(ctx, height, subnetID)
}

// GetCurrentHeight implements the ValidatorState interface
func (s *ValidatorState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	return s.validatorState.GetCurrentHeight(ctx)
}

// GetMinimumHeight implements the ValidatorState interface
func (s *ValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return s.validatorState.GetMinimumHeight(ctx)
}

// GetSubnetID implements the ValidatorState interface
func (s *ValidatorState) GetSubnetID(ctx context.Context, chainID common.Hash) (common.Hash, error) {
	if s.skipChainID && chainID == s.chainID {
		return s.subnetID, nil
	}
	return s.validatorState.GetSubnetID(ctx, chainID)
}
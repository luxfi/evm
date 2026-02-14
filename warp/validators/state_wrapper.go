// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"context"

	"github.com/luxfi/ids"
	consensuscontext "github.com/luxfi/runtime"
	validators "github.com/luxfi/validators"
)

// ConsensusStateWrapper wraps consensuscontext.ValidatorState to implement validators.State
type ConsensusStateWrapper struct {
	vs consensuscontext.ValidatorState
}

// NewConsensusStateWrapper creates a new wrapper
func NewConsensusStateWrapper(vs consensuscontext.ValidatorState) *ConsensusStateWrapper {
	return &ConsensusStateWrapper{vs: vs}
}

// GetCurrentHeight implements validators.State
func (w *ConsensusStateWrapper) GetCurrentHeight(ctx context.Context) (uint64, error) {
	// Pass context to consensus.ValidatorState.GetCurrentHeight
	return w.vs.GetCurrentHeight(ctx)
}

// GetValidatorSet implements validators.State
func (w *ConsensusStateWrapper) GetValidatorSet(ctx context.Context, height uint64, chainID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	// Pass through to underlying ValidatorState
	return w.vs.GetValidatorSet(ctx, height, chainID)
}

// GetMinimumHeight implements validators.State
func (w *ConsensusStateWrapper) GetMinimumHeight(ctx context.Context) (uint64, error) {
	// consensus.ValidatorState doesn't have GetMinimumHeight, return 0
	return 0, nil
}

// GetNetworkID implements validators.State
func (w *ConsensusStateWrapper) GetNetworkID(ctx context.Context, chainID ids.ID) (ids.ID, error) {
	// consensus.ValidatorState doesn't have GetNetworkID, return empty
	return ids.Empty, nil
}

// GetCurrentValidatorSet implements validators.State
func (w *ConsensusStateWrapper) GetCurrentValidatorSet(ctx context.Context, chainID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	// Get current height and use GetValidatorSet
	height, err := w.vs.GetCurrentHeight(ctx)
	if err != nil {
		return nil, err
	}
	return w.GetValidatorSet(ctx, height, chainID)
}

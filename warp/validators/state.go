// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"context"

	validators "github.com/luxfi/validators"
	"github.com/luxfi/constants"
	"github.com/luxfi/ids"
)

var _ validators.State = (*State)(nil)

// State provides a special case used to handle Lux Warp Message verification for messages sent
// from the Primary Network. Chains have strictly fewer validators than the Primary Network, so we require
// signatures from a threshold of the RECEIVING chain validator set rather than the full Primary Network
// since the receiving chain already relies on a majority of its validators being correct.
type State struct {
	validators.State
	myChainID                    ids.ID
	sourceChainID                ids.ID
	requirePrimaryNetworkSigners bool
}

// NewState returns a wrapper of [validators.State] which special cases the handling of the Primary Network.
//
// The wrapped state will return the [myChainID's] validator set instead of the Primary Network when
// the Primary Network ChainID is passed in.
func NewState(state validators.State, myChainID ids.ID, sourceChainID ids.ID, requirePrimaryNetworkSigners bool) *State {
	return &State{
		State:                        state,
		myChainID:                    myChainID,
		sourceChainID:                sourceChainID,
		requirePrimaryNetworkSigners: requirePrimaryNetworkSigners,
	}
}

func (s *State) GetValidatorSet(
	ctx context.Context,
	height uint64,
	chainID ids.ID,
) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	// If the chainID is anything other than the Primary Network, or Primary
	// Network signers are required (except P-Chain), this is a direct passthrough.
	usePrimary := s.requirePrimaryNetworkSigners && s.sourceChainID != constants.PlatformChainID
	if usePrimary || chainID != constants.PrimaryNetworkID {
		return s.State.GetValidatorSet(ctx, height, chainID)
	}

	// If the requested chain is the primary network, then we return the validator
	// set for the chain that is receiving the message instead.
	return s.State.GetValidatorSet(ctx, height, s.myChainID)
}

// GetWarpValidatorSet returns the warp validator set with BLS public keys for signature aggregation.
// This applies the same Primary Network handling as GetValidatorSet.
func (s *State) GetWarpValidatorSet(
	ctx context.Context,
	height uint64,
	chainID ids.ID,
) (*validators.WarpSet, error) {
	// Apply same logic as GetValidatorSet for Primary Network handling
	usePrimary := s.requirePrimaryNetworkSigners && s.sourceChainID != constants.PlatformChainID
	if usePrimary || chainID != constants.PrimaryNetworkID {
		return s.State.GetWarpValidatorSet(ctx, height, chainID)
	}

	// If the requested chain is the primary network, return the validator
	// set for the chain that is receiving the message instead.
	return s.State.GetWarpValidatorSet(ctx, height, s.myChainID)
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utilstest

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/runtime"
	validators "github.com/luxfi/validators"
	"github.com/luxfi/validators/validatorstest"
)

// EVMTestChainID is a evm specific chain ID for testing
var EVMTestChainID = ids.GenerateTestID()

// testValidatorState wraps validatorstest.State to implement runtime.ValidatorState
type testValidatorState struct {
	*validatorstest.State
}

func (t *testValidatorState) GetCurrentHeight(ctx context.Context) (uint64, error) {
	// Call GetCurrentHeightF if available, otherwise return 0
	if t.State != nil && t.State.GetCurrentHeightF != nil {
		return t.State.GetCurrentHeightF(ctx)
	}
	return 0, nil
}

func (t *testValidatorState) GetMinimumHeight(ctx context.Context) (uint64, error) {
	// Return 0 for test purposes - minimum height for testing
	return 0, nil
}

func (t *testValidatorState) GetValidatorSet(ctx context.Context, height uint64, chainID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	// Delegate to the underlying State's GetValidatorSetF
	if t.State != nil && t.State.GetValidatorSetF != nil {
		return t.State.GetValidatorSetF(ctx, height, chainID)
	}
	return make(map[ids.NodeID]*validators.GetValidatorOutput), nil
}

func (t *testValidatorState) GetCurrentValidators(ctx context.Context, height uint64, chainID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	return t.GetValidatorSet(ctx, height, chainID)
}

func (t *testValidatorState) GetChainID(id ids.ID) (ids.ID, error) {
	return ids.Empty, nil // Default for tests
}

func (t *testValidatorState) GetNetworkID(id ids.ID) (ids.ID, error) {
	return ids.Empty, nil // Default for tests
}

func (t *testValidatorState) GetWarpValidatorSets(ctx context.Context, heights []uint64, netIDs []ids.ID) (map[ids.ID]map[uint64]*validators.WarpSet, error) {
	// Return empty for tests
	return make(map[ids.ID]map[uint64]*validators.WarpSet), nil
}

func (t *testValidatorState) GetWarpValidatorSet(ctx context.Context, height uint64, netID ids.ID) (*validators.WarpSet, error) {
	// Return nil for tests
	return nil, nil
}

// NewTestValidatorState creates a new test validator state
func NewTestValidatorState() runtime.ValidatorState {
	state := &validatorstest.State{
		GetCurrentHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
		},
	}

	return &testValidatorState{State: state}
}

// NewTestValidatorStateFromBase creates a testValidatorState that wraps an existing validatorstest.State
// This is useful when you need to use a specific validatorstest.State with custom functions
// but still implement the runtime.ValidatorState interface.
func NewTestValidatorStateFromBase(baseState *validatorstest.State) runtime.ValidatorState {
	return &testValidatorState{State: baseState}
}

// NewTestRuntime creates a new Runtime suitable for testing with the given chain ID
func NewTestRuntime(t testing.TB, chainID ids.ID) *runtime.Runtime {
	t.Helper()
	return &runtime.Runtime{
		NetworkID:      1,
		ChainID:        chainID,
		NodeID:         ids.GenerateTestNodeID(),
		ValidatorState: NewTestValidatorState(),
	}
}

// NewTestConsensusContext returns a context.Context with runtime properly configured for testing.
func NewTestConsensusContext(t testing.TB) context.Context {
	t.Helper()
	rt := NewTestRuntime(t, EVMTestChainID)
	return runtime.WithContext(context.Background(), rt)
}

// NewTestConsensusContextWithChainID returns a context.Context with runtime properly configured for testing
// with a specific chain ID.
func NewTestConsensusContextWithChainID(t testing.TB, chainID ids.ID) context.Context {
	t.Helper()
	rt := NewTestRuntime(t, chainID)
	return runtime.WithContext(context.Background(), rt)
}

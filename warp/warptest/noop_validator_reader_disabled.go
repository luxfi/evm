//go:build !node_validators

// (c) 2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// warptest exposes common functionality for testing the warp package.
package warptest

import (
	"context"
	"fmt"

	"github.com/luxfi/evm/interfaces"
)

var _ interfaces.State = &NoOpValidatorReader{}

type NoOpValidatorReader struct{}

func (NoOpValidatorReader) GetCurrentHeight(ctx context.Context) (uint64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (NoOpValidatorReader) GetMinimumHeight(ctx context.Context) (uint64, error) {
	return 0, fmt.Errorf("not implemented")
}

func (NoOpValidatorReader) GetSubnetID(ctx context.Context, chainID interfaces.ID) (interfaces.ID, error) {
	return interfaces.ID{}, fmt.Errorf("not implemented")
}

func (NoOpValidatorReader) GetValidatorSet(ctx context.Context, height uint64, subnetID interfaces.ID) (map[interfaces.NodeID]*interfaces.GetValidatorOutput, error) {
	return nil, fmt.Errorf("not implemented")
}
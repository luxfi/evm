// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"

	"github.com/luxfi/vm/chain"
)

// HealthCheck returns the health status of this chain.
// A more granular check (peer count, block lag, etc.) can be added here.
func (vm *VM) HealthCheck(context.Context) (chain.HealthResult, error) {
	return chain.HealthResult{
		Healthy: true,
		Details: nil,
	}, nil
}

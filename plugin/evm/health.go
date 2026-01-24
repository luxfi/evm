// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"

	"github.com/luxfi/vm/chain"
)

// HealthCheck returns the health status of this chain.
func (vm *VM) HealthCheck(context.Context) (chain.HealthResult, error) {
	// TODO perform actual health check
	return chain.HealthResult{
		Healthy: true,
		Details: nil,
	}, nil
}

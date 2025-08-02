// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package handlers

import (
	"github.com/luxfi/evm/v2/v2/params"
	"github.com/luxfi/evm/v2/v2/params/extras"
)

// getTestChainConfig returns a properly configured test chain config
func getTestChainConfig() *params.ChainConfig {
	config := params.TestChainConfig
	
	// Set up the extras properly
	extra := extras.TestChainConfig
	params.WithExtra(config, extra)
	
	return config
}
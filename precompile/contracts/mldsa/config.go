// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package mldsa

import (
	"fmt"

	"github.com/luxfi/evm/precompile/precompileconfig"
)

var _ precompileconfig.Config = &Config{}

// Config implements the precompileconfig.Config interface
type Config struct {
	precompileconfig.Upgrade
}

// NewConfig returns a new ML-DSA precompile config
func NewConfig(blockTimestamp *uint64) *Config {
	return &Config{
		Upgrade: precompileconfig.Upgrade{
			BlockTimestamp: blockTimestamp,
		},
	}
}

// NewDisableConfig returns a config that disables the ML-DSA precompile
func NewDisableConfig(blockTimestamp *uint64) *Config {
	return &Config{
		Upgrade: precompileconfig.Upgrade{
			BlockTimestamp: blockTimestamp,
			Disable:        true,
		},
	}
}

// Key returns the unique key for the ML-DSA precompile config
func (*Config) Key() string { return ConfigKey }

// Verify returns an error if the config is invalid
func (c *Config) Verify(chainConfig precompileconfig.ChainConfig) error {
	// Basic validation - check that timestamp is set for enabling
	if !c.Disable && c.BlockTimestamp == nil {
		return fmt.Errorf("ML-DSA precompile is enabled but no activation timestamp is set")
	}
	return nil
}

// Equal returns true if the provided config is equivalent
func (c *Config) Equal(cfg precompileconfig.Config) bool {
	other, ok := (cfg).(*Config)
	if !ok {
		return false
	}
	return c.Upgrade.Equal(&other.Upgrade)
}

// String returns a string representation of the config
func (c *Config) String() string {
	return fmt.Sprintf("MLDSA{BlockTimestamp: %v, Disable: %v}", c.BlockTimestamp, c.Disable)
}

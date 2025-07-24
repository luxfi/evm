//go:build !legacy_upgrades_off
// Copyright 2025, Lux Industries, Inc.

package params

// -----------------------------------------------------------------------------
// ALWAYS-ON UPGRADE FLAGS
// -----------------------------------------------------------------------------
// We launched post-Durango (Nov-2023).  All Avalanche forks up to Granite are
// considered *already active*.  These helpers are kept only because upstream
// code and unit-tests still expect them to exist.  Feel free to delete this
// file entirely once those dependencies are gone.

// ChainConfig upgrade helpers - all upgrades are always active
func (c *ChainConfig) IsApricotPhase1(time uint64) bool  { return true }
func (c *ChainConfig) IsApricotPhase2(time uint64) bool  { return true }
func (c *ChainConfig) IsApricotPhase3(time uint64) bool  { return true }
func (c *ChainConfig) IsApricotPhase4(time uint64) bool  { return true }
func (c *ChainConfig) IsApricotPhase5(time uint64) bool  { return true }
func (c *ChainConfig) IsApricotPhase6(time uint64) bool  { return true }

// Convenience helpers used by some estimators
func (c *ChainConfig) IsApricotPhasePost6(time uint64) bool { return true }
func (c *ChainConfig) IsApricotPhasePre6(time uint64) bool  { return false } // always AFTER P6

func (c *ChainConfig) IsBanff(time uint64) bool   { return true }
func (c *ChainConfig) IsCortina(time uint64) bool { return true }
// IsDurango is already defined in config.go
func (c *ChainConfig) IsEtna(time uint64) bool    { return true }

// Already on; returns true for any timestamp
// IsFortuna is already defined in config.go
func (c *ChainConfig) IsGranite(time uint64) bool { return true }


// Rules-based helpers for consistency
func (r Rules) IsApricotPhase1(num uint64) bool  { return true }
func (r Rules) IsApricotPhase2(num uint64) bool  { return true }
func (r Rules) IsApricotPhase3(num uint64) bool  { return true }
func (r Rules) IsApricotPhase4(num uint64) bool  { return true }
func (r Rules) IsApricotPhase5(num uint64) bool  { return true }
func (r Rules) IsApricotPhasePre6(num uint64) bool { return false }
func (r Rules) IsApricotPhasePost6(num uint64) bool { return true }
func (r Rules) IsBanff(num uint64) bool   { return true }
func (r Rules) IsCortina(num uint64) bool { return true }
func (r Rules) IsDurango(num uint64) bool { return true }
func (r Rules) IsEtna(num uint64) bool    { return true }
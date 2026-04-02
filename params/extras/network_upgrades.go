// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"fmt"
	"reflect"
	"time"

	"github.com/luxfi/evm/utils"
	ethparams "github.com/luxfi/geth/params"
	"github.com/luxfi/upgrade"
)

var errCannotBeNil = fmt.Errorf("timestamp cannot be nil")

// newTimestampCompatError creates a ConfigCompatError for timestamp mismatches
func newTimestampCompatError(what string, storedtime, newtime *uint64) *ethparams.ConfigCompatError {
	var rew *uint64
	switch {
	case storedtime == nil:
		rew = newtime
	case newtime == nil || *storedtime < *newtime:
		rew = newtime
	default:
		rew = storedtime
	}
	err := &ethparams.ConfigCompatError{
		What:         what,
		StoredTime:   storedtime,
		NewTime:      newtime,
		RewindToTime: 0,
	}
	if rew != nil && *rew > 0 {
		err.RewindToTime = *rew - 1
	}
	return err
}

// NetworkUpgrades contains timestamps that enable network upgrades.
// Lux specific network upgrades are also included here.
// (nil = no fork, 0 = already activated)
type NetworkUpgrades struct {
	// EVMTimestamp is a placeholder that activates Lux Upgrades prior to ApricotPhase6
	EVMTimestamp *uint64 `json:"evmTimestamp,omitempty"`
	// Durango activates the Shanghai Execution Spec Upgrade from Ethereum (https://github.com/ethereum/execution-specs/blob/master/network-upgrades/mainnet-upgrades/shanghai.md#included-eips)
	// and Lux Warp Messaging.
	// Note: EIP-4895 is excluded since withdrawals are not relevant to the Lux C-Chain or Chains running the EVM.
	DurangoTimestamp *uint64 `json:"durangoTimestamp,omitempty"`
	// Placeholder for EtnaTimestamp
	EtnaTimestamp *uint64 `json:"etnaTimestamp,omitempty"`
	// Fortuna has no effect on EVM by itself, but is included for completeness.
	FortunaTimestamp *uint64 `json:"fortunaTimestamp,omitempty"`
	// Granite is a placeholder for the next upgrade.
	GraniteTimestamp *uint64 `json:"graniteTimestamp,omitempty"`
}

func (n *NetworkUpgrades) Equal(other *NetworkUpgrades) bool {
	return reflect.DeepEqual(n, other)
}

func (n *NetworkUpgrades) checkNetworkUpgradesCompatible(newcfg *NetworkUpgrades, time uint64) *ethparams.ConfigCompatError {
	if isForkTimestampIncompatible(n.EVMTimestamp, newcfg.EVMTimestamp, time) {
		return newTimestampCompatError("EVM fork block timestamp", n.EVMTimestamp, newcfg.EVMTimestamp)
	}
	if isForkTimestampIncompatible(n.DurangoTimestamp, newcfg.DurangoTimestamp, time) {
		return newTimestampCompatError("Durango fork block timestamp", n.DurangoTimestamp, newcfg.DurangoTimestamp)
	}
	if isForkTimestampIncompatible(n.EtnaTimestamp, newcfg.EtnaTimestamp, time) {
		return newTimestampCompatError("Etna fork block timestamp", n.EtnaTimestamp, newcfg.EtnaTimestamp)
	}
	// Fortuna is optional, so allow nil values even after the fork time
	// Only check incompatibility if both values are non-nil
	if n.FortunaTimestamp != nil && newcfg.FortunaTimestamp != nil {
		if isForkTimestampIncompatible(n.FortunaTimestamp, newcfg.FortunaTimestamp, time) {
			return newTimestampCompatError("Fortuna fork block timestamp", n.FortunaTimestamp, newcfg.FortunaTimestamp)
		}
	}
	if isForkTimestampIncompatible(n.GraniteTimestamp, newcfg.GraniteTimestamp, time) {
		return newTimestampCompatError("Granite fork block timestamp", n.GraniteTimestamp, newcfg.GraniteTimestamp)
	}

	return nil
}

func (n *NetworkUpgrades) forkOrder() []fork {
	return []fork{
		{name: "evmTimestamp", timestamp: n.EVMTimestamp},
		{name: "durangoTimestamp", timestamp: n.DurangoTimestamp},
		{name: "etnaTimestamp", timestamp: n.EtnaTimestamp},
		{name: "fortunaTimestamp", timestamp: n.FortunaTimestamp, optional: true},
		{name: "graniteTimestamp", timestamp: n.GraniteTimestamp},
	}
}

// SetDefaults sets the default values for the network upgrades.
// Only nil timestamps are overridden with defaults. An explicit value of 0
// means "active at genesis" and is preserved.
func (n *NetworkUpgrades) SetDefaults(agoUpgrades upgrade.Config) {
	defaults := GetNetworkUpgrades(agoUpgrades)
	if n.EVMTimestamp == nil {
		n.EVMTimestamp = defaults.EVMTimestamp
	}
	if n.DurangoTimestamp == nil {
		n.DurangoTimestamp = defaults.DurangoTimestamp
	}
	if n.EtnaTimestamp == nil {
		n.EtnaTimestamp = defaults.EtnaTimestamp
	}
	if n.FortunaTimestamp == nil {
		n.FortunaTimestamp = defaults.FortunaTimestamp
	}
	if n.GraniteTimestamp == nil {
		n.GraniteTimestamp = defaults.GraniteTimestamp
	}
}

// verifyNetworkUpgrades checks that the network upgrades are well formed.
func (n *NetworkUpgrades) verifyNetworkUpgrades(agoUpgrades upgrade.Config) error {
	defaults := GetNetworkUpgrades(agoUpgrades)

	// EVMTimestamp must not be nil
	if n.EVMTimestamp == nil {
		return fmt.Errorf("EVM fork block timestamp is invalid: %w", errCannotBeNil)
	}
	// EVMTimestamp must be 0 (activated at genesis)
	if *n.EVMTimestamp != 0 {
		return fmt.Errorf("EVM fork block timestamp is invalid: must be 0 (activated at genesis), got %d", *n.EVMTimestamp)
	}
	if err := verifyWithDefault(n.EVMTimestamp, defaults.EVMTimestamp); err != nil {
		return fmt.Errorf("EVM fork block timestamp is invalid: %w", err)
	}

	// DurangoTimestamp must not be nil
	if n.DurangoTimestamp == nil {
		return fmt.Errorf("Durango fork block timestamp is invalid: %w", errCannotBeNil)
	}
	if defaults.DurangoTimestamp != nil {
		// If the default activates at genesis, the config must also activate at genesis.
		if *defaults.DurangoTimestamp == 0 && *n.DurangoTimestamp != 0 {
			return fmt.Errorf("Durango fork block timestamp is invalid: cannot be changed from genesis activation (0) to %d", *n.DurangoTimestamp)
		}
		if err := verifyWithDefault(n.DurangoTimestamp, defaults.DurangoTimestamp); err != nil {
			return fmt.Errorf("Durango fork block timestamp is invalid: %w", err)
		}
	}

	// EtnaTimestamp — when the default is set (non-nil), the config must
	// also set it and must not be earlier than the default.
	if defaults.EtnaTimestamp != nil {
		if n.EtnaTimestamp == nil {
			return fmt.Errorf("Etna fork block timestamp is invalid: %w", errCannotBeNil)
		}
		if err := verifyWithDefault(n.EtnaTimestamp, defaults.EtnaTimestamp); err != nil {
			return fmt.Errorf("Etna fork block timestamp is invalid: %w", err)
		}
	}

	// FortunaTimestamp — same rule: if default is set, config must match.
	if defaults.FortunaTimestamp != nil {
		if n.FortunaTimestamp == nil {
			return fmt.Errorf("Fortuna fork block timestamp is invalid: %w", errCannotBeNil)
		}
		if err := verifyWithDefault(n.FortunaTimestamp, defaults.FortunaTimestamp); err != nil {
			return fmt.Errorf("Fortuna fork block timestamp is invalid: %w", err)
		}
	}

	// GraniteTimestamp — same rule.
	if defaults.GraniteTimestamp != nil {
		if n.GraniteTimestamp == nil {
			return fmt.Errorf("Granite fork block timestamp is invalid: %w", errCannotBeNil)
		}
		if err := verifyWithDefault(n.GraniteTimestamp, defaults.GraniteTimestamp); err != nil {
			return fmt.Errorf("Granite fork block timestamp is invalid: %w", err)
		}
	}

	// Check that forks are enabled in order
	if err := checkForks(n.forkOrder(), false); err != nil {
		return err
	}

	return nil
}

func (n *NetworkUpgrades) Override(o *NetworkUpgrades) {
	if o.EVMTimestamp != nil {
		n.EVMTimestamp = o.EVMTimestamp
	}
	if o.DurangoTimestamp != nil {
		n.DurangoTimestamp = o.DurangoTimestamp
	}
	if o.EtnaTimestamp != nil {
		n.EtnaTimestamp = o.EtnaTimestamp
	}
	if o.FortunaTimestamp != nil {
		n.FortunaTimestamp = o.FortunaTimestamp
	}
	if o.GraniteTimestamp != nil {
		n.GraniteTimestamp = o.GraniteTimestamp
	}
}

// IsEVM returns whether [time] represents a block
// with a timestamp after the EVM upgrade time.
func (n NetworkUpgrades) IsEVM(time uint64) bool {
	return isTimestampForked(n.EVMTimestamp, time)
}

// IsDurango returns whether [time] represents a block
// with a timestamp after the Durango upgrade time.
func (n NetworkUpgrades) IsDurango(time uint64) bool {
	return isTimestampForked(n.DurangoTimestamp, time)
}

// IsEtna returns whether [time] represents a block
// with a timestamp after the Etna upgrade time.
func (n NetworkUpgrades) IsEtna(time uint64) bool {
	return isTimestampForked(n.EtnaTimestamp, time)
}

// IsFortuna returns whether [time] represents a block
// with a timestamp after the Fortuna upgrade time.
func (n *NetworkUpgrades) IsFortuna(time uint64) bool {
	return isTimestampForked(n.FortunaTimestamp, time)
}

// IsGranite returns whether [time] represents a block
// with a timestamp after the Granite upgrade time.
func (n *NetworkUpgrades) IsGranite(time uint64) bool {
	return isTimestampForked(n.GraniteTimestamp, time)
}

func (n *NetworkUpgrades) Description() string {
	var banner string
	banner += fmt.Sprintf(" - EVM Timestamp:          @%-10v (https://github.com/luxfi/node/releases/tag/v1.10.0)\n", ptrToString(n.EVMTimestamp))
	banner += fmt.Sprintf(" - Durango Timestamp:            @%-10v (https://github.com/luxfi/node/releases/tag/v1.11.0)\n", ptrToString(n.DurangoTimestamp))
	banner += fmt.Sprintf(" - Etna Timestamp:               @%-10v (https://github.com/luxfi/node/releases/tag/v1.12.0)\n", ptrToString(n.EtnaTimestamp))
	banner += fmt.Sprintf(" - Fortuna Timestamp:            @%-10v (https://github.com/luxfi/node/releases/tag/v1.13.0)\n", ptrToString(n.FortunaTimestamp))
	banner += fmt.Sprintf(" - Granite Timestamp:            @%-10v (https://github.com/luxfi/node/releases/tag/v1.14.0)\n", ptrToString(n.GraniteTimestamp))
	return banner
}

type LuxRules struct {
	IsEVM     bool
	IsDurango bool
	IsEtna    bool
	IsFortuna bool
	IsGranite bool
}

func (n *NetworkUpgrades) GetLuxRules(time uint64) LuxRules {
	return LuxRules{
		IsEVM:     n.IsEVM(time),
		IsDurango: n.IsDurango(time),
		IsEtna:    n.IsEtna(time),
		IsFortuna: n.IsFortuna(time),
		IsGranite: n.IsGranite(time),
	}
}

// GetNetworkUpgrades returns the network upgrades for the specified luxd upgrades.
// Nil values are used to indicate optional upgrades.
// The function respects the upgrade times from the config - if an upgrade is scheduled
// at or after UnscheduledActivationTime, it is considered not scheduled.
// InitiallyActiveTime is treated as "activated at genesis (timestamp 0)".
func GetNetworkUpgrades(agoUpgrade upgrade.Config) NetworkUpgrades {
	// EVM is always activated at genesis (0)
	result := NetworkUpgrades{
		EVMTimestamp: utils.NewUint64(0),
	}

	// Use upgrade.UnscheduledActivationTime as the threshold for "unscheduled"
	// UnscheduledActivationTime is time.Date(9999, time.December, 1, 0, 0, 0, 0, time.UTC)
	unscheduledTime := uint64(upgrade.UnscheduledActivationTime.Unix())

	// InitiallyActiveTime means "always active from genesis" for EVM purposes
	// We convert it to 0 (genesis timestamp) for EVM network upgrades
	initiallyActiveTime := upgrade.InitiallyActiveTime

	// Helper to convert upgrade time to EVM timestamp
	// Returns nil if unscheduled or zero time, 0 if initially active, or the actual timestamp
	toEVMTimestamp := func(t time.Time) *uint64 {
		// Zero time (not set) is treated as unscheduled
		if t.IsZero() {
			return nil
		}
		// Check if scheduled at unscheduled time (far future)
		if t.Unix() >= int64(unscheduledTime) {
			return nil // Unscheduled
		}
		// InitiallyActiveTime means "already active from genesis"
		if t.Equal(initiallyActiveTime) {
			return utils.NewUint64(0) // Genesis activation
		}
		return utils.NewUint64(uint64(t.Unix()))
	}

	// Check DurangoTime
	result.DurangoTimestamp = toEVMTimestamp(agoUpgrade.DurangoTime)

	// Check EtnaTime
	result.EtnaTimestamp = toEVMTimestamp(agoUpgrade.EtnaTime)

	// Check FortunaTime
	result.FortunaTimestamp = toEVMTimestamp(agoUpgrade.FortunaTime)

	// Check GraniteTime
	result.GraniteTimestamp = toEVMTimestamp(agoUpgrade.GraniteTime)

	return result
}

// GetDefaultNetworkUpgrades returns default network upgrades.
// All upgrades are enabled at genesis (timestamp 0) so that every chain
// running the Lux EVM has the full opcode set (PUSH0, MCOPY, TSTORE,
// TLOAD, BLOBHASH, BLOBBASEFEE, etc.) available from block 0.
func GetDefaultNetworkUpgrades() NetworkUpgrades {
	return NetworkUpgrades{
		EVMTimestamp:     utils.NewUint64(0),
		DurangoTimestamp: utils.NewUint64(0),
		EtnaTimestamp:    utils.NewUint64(0),
		FortunaTimestamp: utils.NewUint64(0),
		GraniteTimestamp: utils.NewUint64(0),
	}
}

// verifyWithDefault checks that the provided timestamp is greater than or equal to the default timestamp.
func verifyWithDefault(configTimestamp *uint64, defaultTimestamp *uint64) error {
	if defaultTimestamp == nil {
		return nil
	}

	if configTimestamp == nil {
		return errCannotBeNil
	}

	if *configTimestamp < *defaultTimestamp {
		return fmt.Errorf("provided timestamp (%d) must be greater than or equal to the default timestamp (%d)", *configTimestamp, *defaultTimestamp)
	}
	return nil
}

func ptrToString(val *uint64) string {
	if val == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *val)
}

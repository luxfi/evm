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

// DexSettleActivationTime is THE canonical, network-wide activation boundary for the
// DEX settlement money path 0x9999 — Dec 25 2025 00:00:00 UTC (unix 1766704800).
//
// It is defined ONCE here (DRY) and is identical on every Lux network. 0x9999 is a
// system precompile that takes no per-network parameters (all resolved at runtime from
// the consensus context), so its activation is NOT per-net config — it is a single dated
// fork in the same spirit as the Durango/Quasar/Fortuna/Granite network upgrades above.
//
// The activation has two coupled effects, both gated on this exact timestamp via
// IsDexSettleActive / IsForkTransition (see params/config_extra.go GetExtrasRules and
// core/state_processor_ext.go ApplyPrecompileActivations):
//
//   - DISPATCH: at/after this timestamp, 0x9999 is in the enabled precompile set, so a
//     tx-to-0x9999 / low-level CALL / typed Solidity call dispatches the native
//     settlement contract. Before it, 0x9999 is absent — a value transfer to that
//     address behaves as a plain account, so replaying pre-activation history (the RLP
//     snapshot in ~/work/lux/state) stays byte-identical to canonical state.
//   - MARKER: on the block transition that crosses this timestamp, the precompile-
//     activation marker (SetNonce=1 + SetCode{0x1}) is written into account 0x9999 so
//     EXTCODESIZE>0, eth_getCode!=0x, and Solidity's contract-existence guard passes.
//     Historical genesis is NOT mutated (a genesis-time marker would change the genesis
//     hash and fork pre-activation sync); the marker is installed forward, at the fork.
//
// For a freshly-genesised network whose genesis timestamp is already >= this value, the
// transition fires at genesis (parent=nil), so the marker is present from block 0 — the
// SAME mechanism, no separate genesis-precompile entry.
const DexSettleActivationTime uint64 = 1766704800 // 2025-12-25T00:00:00Z

// dexSettleTimestamp is the activation time as a *uint64 for the IsForkTransition /
// isTimestampForked helpers (which take *uint64; nil = never).
var dexSettleTimestamp = func() *uint64 { t := DexSettleActivationTime; return &t }()

// DexSettleTimestamp returns the canonical 0x9999 activation timestamp pointer. Used by
// the EVM dispatch gate and the marker-installing state transition so both reference the
// SAME single dated fork.
func DexSettleTimestamp() *uint64 { return dexSettleTimestamp }

// IsDexSettleActive reports whether the 0x9999 DEX settlement precompile is active at
// [time] — i.e. [time] is at or after the canonical Dec 25 2025 activation boundary.
func IsDexSettleActive(time uint64) bool { return isTimestampForked(dexSettleTimestamp, time) }

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
	// Placeholder for QuasarTimestamp
	QuasarTimestamp *uint64 `json:"quasarTimestamp,omitempty"`
	// Fortuna has no effect on EVM by itself, but is included for completeness.
	FortunaTimestamp *uint64 `json:"fortunaTimestamp,omitempty"`
	// Granite is a placeholder for the next upgrade.
	GraniteTimestamp *uint64 `json:"graniteTimestamp,omitempty"`
	// StrictPQTimestamp pins the chain to a strict post-quantum profile from
	// the given timestamp onward. When active, classical pairing-based and
	// discrete-log precompiles refuse to execute via contract.RefuseUnderStrictPQ.
	// nil = never activates (classical-permissive, the default for non-Lux
	// chains that integrate Lux precompiles). 0 = active from genesis.
	StrictPQTimestamp *uint64 `json:"strictPQTimestamp,omitempty"`
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
	if isForkTimestampIncompatible(n.QuasarTimestamp, newcfg.QuasarTimestamp, time) {
		return newTimestampCompatError("Quasar fork block timestamp", n.QuasarTimestamp, newcfg.QuasarTimestamp)
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
		{name: "quasarTimestamp", timestamp: n.QuasarTimestamp},
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
	if n.QuasarTimestamp == nil {
		n.QuasarTimestamp = defaults.QuasarTimestamp
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

	// QuasarTimestamp — when the default is set (non-nil), the config must
	// also set it and must not be earlier than the default.
	if defaults.QuasarTimestamp != nil {
		if n.QuasarTimestamp == nil {
			return fmt.Errorf("Quasar fork block timestamp is invalid: %w", errCannotBeNil)
		}
		if err := verifyWithDefault(n.QuasarTimestamp, defaults.QuasarTimestamp); err != nil {
			return fmt.Errorf("Quasar fork block timestamp is invalid: %w", err)
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
	if o.QuasarTimestamp != nil {
		n.QuasarTimestamp = o.QuasarTimestamp
	}
	if o.FortunaTimestamp != nil {
		n.FortunaTimestamp = o.FortunaTimestamp
	}
	if o.GraniteTimestamp != nil {
		n.GraniteTimestamp = o.GraniteTimestamp
	}
	if o.StrictPQTimestamp != nil {
		n.StrictPQTimestamp = o.StrictPQTimestamp
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

// IsQuasar returns whether [time] represents a block
// with a timestamp after the Quasar Edition upgrade time.
func (n NetworkUpgrades) IsQuasar(time uint64) bool {
	return isTimestampForked(n.QuasarTimestamp, time)
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

// IsStrictPQ returns whether [time] represents a block at or after the
// strict-PQ activation timestamp. nil StrictPQTimestamp means the chain
// is classical-permissive (the default for non-Lux chains that integrate
// Lux precompiles).
func (n *NetworkUpgrades) IsStrictPQ(time uint64) bool {
	return isTimestampForked(n.StrictPQTimestamp, time)
}

func (n *NetworkUpgrades) Description() string {
	var banner string
	banner += fmt.Sprintf(" - EVM Timestamp:          @%-10v (https://github.com/luxfi/node/releases/tag/v1.10.0)\n", ptrToString(n.EVMTimestamp))
	banner += fmt.Sprintf(" - Durango Timestamp:            @%-10v (https://github.com/luxfi/node/releases/tag/v1.11.0)\n", ptrToString(n.DurangoTimestamp))
	banner += fmt.Sprintf(" - Quasar Timestamp:             @%-10v (https://github.com/luxfi/node/releases/tag/v1.12.0)\n", ptrToString(n.QuasarTimestamp))
	banner += fmt.Sprintf(" - Fortuna Timestamp:            @%-10v (https://github.com/luxfi/node/releases/tag/v1.13.0)\n", ptrToString(n.FortunaTimestamp))
	banner += fmt.Sprintf(" - Granite Timestamp:            @%-10v (https://github.com/luxfi/node/releases/tag/v1.14.0)\n", ptrToString(n.GraniteTimestamp))
	return banner
}

type LuxRules struct {
	IsEVM     bool
	IsDurango bool
	IsQuasar  bool
	IsFortuna bool
	IsGranite bool
}

func (n *NetworkUpgrades) GetLuxRules(time uint64) LuxRules {
	return LuxRules{
		IsEVM:     n.IsEVM(time),
		IsDurango: n.IsDurango(time),
		IsQuasar:  n.IsQuasar(time),
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

	// Check QuasarTime
	result.QuasarTimestamp = toEVMTimestamp(agoUpgrade.QuasarTime)

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
		QuasarTimestamp:  utils.NewUint64(0),
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

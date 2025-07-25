// (c) 2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

// UpgradeID is an enum for each future upgrade.
type UpgradeID string

const (
	// Add new upgrades here as they are announced
	// UpgradeXVM   UpgradeID = "XVM"   // e.g. activate your X-Chain VM
	// UpgradeZK    UpgradeID = "ZK"    // post-2025 ZK feature
)

// FutureUpgrades holds activation heights (or timestamps).
var FutureUpgrades = map[UpgradeID]uint64{
	// fill these in when you announce them:
	// UpgradeXVM: 1_500_000,
	// UpgradeZK:  2_000_000,
}

// IsUpgradeActive reports whether we've hit the activation block.
func IsUpgradeActive(id UpgradeID, height uint64) bool {
	activation, ok := FutureUpgrades[id]
	return ok && height >= activation
}
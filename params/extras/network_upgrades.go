// (c) 2022, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"fmt"
	"reflect"

	"github.com/luxfi/evm/utils"
	gethparams "github.com/luxfi/geth/params"
	upgrade "github.com/luxfi/node/upgrade"
)

var errCannotBeNil = fmt.Errorf("timestamp cannot be nil")

// newTimestampCompatError creates a new timestamp compatibility error
func newTimestampCompatError(what string, storedtime, newtime *uint64) *gethparams.ConfigCompatError {
	var rew *uint64
	switch {
	case storedtime == nil:
		rew = newtime
	case newtime == nil || *storedtime < *newtime:
		rew = storedtime
	default:
		rew = newtime
	}
	var rewTime uint64
	if rew != nil {
		rewTime = *rew
	}
	return &gethparams.ConfigCompatError{
		What:         what,
		StoredTime:   storedtime,
		NewTime:      newtime,
		RewindToTime: rewTime,
	}
}

// NetworkUpgrades contains timestamps that enable network upgrades.
// Lux specific network upgrades are also included here.
// (nil = no fork, 0 = already activated)
type NetworkUpgrades struct {
	// GenesisTimestamp is when the genesis network upgrade activates.
	// This includes all Ethereum Shanghai upgrades and Lux Warp Messaging.
	// For a clean v2.0.0 launch, this is always set to 0 (activated at genesis).
	GenesisTimestamp *uint64 `json:"genesisTimestamp,omitempty"`
}

func (n *NetworkUpgrades) Equal(other *NetworkUpgrades) bool {
	return reflect.DeepEqual(n, other)
}

func (n *NetworkUpgrades) checkNetworkUpgradesCompatible(newcfg *NetworkUpgrades, time uint64) *gethparams.ConfigCompatError {
	// For v2.0.0, all upgrades are active at genesis, no compatibility checks needed
	return nil
}

func (n *NetworkUpgrades) forkOrder() []fork {
	// For v2.0.0, only genesis upgrade exists and is always active
	return []fork{
		{name: "genesisTimestamp", timestamp: n.GenesisTimestamp},
	}
}

// SetDefaults sets the default values for the network upgrades.
// For v2.0.0, genesis upgrade is always activated at timestamp 0.
func (n *NetworkUpgrades) SetDefaults(agoUpgrades upgrade.Config) {
	if n.GenesisTimestamp == nil {
		n.GenesisTimestamp = utils.NewUint64(0)
	}
}

// verifyNetworkUpgrades checks that the network upgrades are well formed.
func (n *NetworkUpgrades) verifyNetworkUpgrades(agoUpgrades upgrade.Config) error {
	// For v2.0.0, genesis upgrade must be active at timestamp 0
	if n.GenesisTimestamp == nil || *n.GenesisTimestamp != 0 {
		return fmt.Errorf("genesis upgrade must be active at timestamp 0")
	}
	return nil
}

func (n *NetworkUpgrades) Override(o *NetworkUpgrades) {
	// For v2.0.0, no overrides allowed - genesis is always at 0
}

// IsGenesis returns whether [time] represents a block
// with a timestamp after the genesis upgrade time.
// For v2.0.0, this is always true since genesis is at timestamp 0.
func (NetworkUpgrades) IsGenesis(_ uint64) bool {
	return true
}

// IsEtna returns whether [time] represents a block
// with a timestamp after the Etna upgrade time.
// All past upgrades are always enabled in Lux.
func (NetworkUpgrades) IsEtna(_ uint64) bool {
	return true
}

func (n *NetworkUpgrades) Description() string {
	return fmt.Sprintf(" - Genesis Timestamp: @%-10v (All upgrades active at genesis)\n", ptrToString(n.GenesisTimestamp))
}

// GenesisRules contains the rules for genesis
type GenesisRules struct {
	// All upgrades are active at genesis for v2.0.0
	IsGenesis bool
}

func (n *NetworkUpgrades) GetGenesisRules(time uint64) GenesisRules {
	return GenesisRules{
		IsGenesis: true, // Always enabled for v2.0.0
	}
}

// getDefaultNetworkUpgrades returns the network upgrades for the specified luxd upgrades.
// For v2.0.0, all upgrades are enabled from genesis (timestamp 0).
func getDefaultNetworkUpgrades(agoUpgrade upgrade.Config) NetworkUpgrades {
	return NetworkUpgrades{
		GenesisTimestamp: utils.NewUint64(0), // All features enabled from genesis
	}
}


func ptrToString(val *uint64) string {
	if val == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *val)
}

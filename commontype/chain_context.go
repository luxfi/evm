// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package commontype

import (
	"github.com/luxfi/evm/v2/v2/ids"
	"github.com/luxfi/evm/v2/v2/iface"
)

// ChainContext provides Lux-specific blockchain context
type ChainContext struct {
	NetworkID uint32
	SubnetID  ids.SubnetID
	ChainID   ids.ChainID
	NodeID    ids.NodeID

	// Node version
	AppVersion uint32

	// Chain configuration
	ChainDataDir string
	
	// ValidatorState provides access to validator information
	ValidatorState iface.ValidatorState
}

// NodeID is an alias to ids.NodeID for compatibility
type NodeID = ids.NodeID

// SubnetID is an alias to ids.SubnetID for compatibility
type SubnetID = ids.SubnetID

// ChainID is an alias to ids.ChainID for compatibility
type ChainID = ids.ChainID
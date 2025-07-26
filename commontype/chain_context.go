// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package commontype

import (
	"github.com/luxfi/evm/iface"
)

// ChainContext provides Lux-specific blockchain context
type ChainContext struct {
	NetworkID uint32
	SubnetID  iface.SubnetID
	ChainID   iface.ChainID
	NodeID    iface.NodeID

	// Node version
	AppVersion uint32

	// Chain configuration
	ChainDataDir string
}

// NodeID is an alias to iface.NodeID for compatibility
type NodeID = iface.NodeID

// SubnetID is an alias to iface.SubnetID for compatibility
type SubnetID = iface.SubnetID

// ChainID is an alias to iface.ChainID for compatibility
type ChainID = iface.ChainID
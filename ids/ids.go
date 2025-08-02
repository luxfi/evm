// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package ids provides common identifier types used throughout the EVM codebase.
// This package has no dependencies on other EVM packages to avoid import cycles.
package ids

import (
	"fmt"
)

// ID represents a generic 32-byte identifier
type ID [32]byte

// String returns the string representation of an ID
func (id ID) String() string {
	return fmt.Sprintf("%x", id[:])
}

// NodeID is a 32-byte identifier for nodes
type NodeID [32]byte

// String returns the string representation of a NodeID
func (id NodeID) String() string {
	return fmt.Sprintf("%x", id[:])
}

// SubnetID is a 32-byte subnet identifier  
type SubnetID [32]byte

// String returns the string representation of a SubnetID
func (id SubnetID) String() string {
	return fmt.Sprintf("%x", id[:])
}

// ChainID is a 32-byte chain identifier
type ChainID [32]byte

// String returns the string representation of a ChainID
func (id ChainID) String() string {
	return fmt.Sprintf("%x", id[:])
}

// BlockID represents a block identifier
type BlockID = ID
// (c) 2020-2020, Lux Industries, Inc.
// All rights reserved.
// See the file LICENSE for licensing terms.

package linear

import (
	"github.com/luxfi/ids"
)

// Block is the interface for linear blocks
type Block interface {
	ID() ids.ID
	ParentID() ids.ID
	Height() uint64
	Verify() error
	Accept() error
	Reject() error
	Status() Status
}

// Status represents the status of a block
type Status uint8

const (
	Unknown Status = iota
	Processing
	Rejected
	Accepted
)

// Consensus interface for linear consensus
type Consensus interface {
	Add(Block) error
	Finalized() bool
}
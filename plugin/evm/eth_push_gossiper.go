// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"github.com/luxfi/evm/v2/core/types"
)

// EthPushGossiper handles gossiping of Ethereum transactions
type EthPushGossiper struct {
	vm *VM
}

// Add implements the eth.PushGossiper interface
func (e *EthPushGossiper) Add(tx *types.Transaction) {
	// TODO: Implement transaction gossiping
	// For now, this is a no-op
}

// (c) 2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testhelpers

import (
	"math/big"
	
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/holiman/uint256"
	"github.com/luxfi/evm/v2/v2/core/state"
)

// StateDBAdapter wraps a state.StateDB to implement the simpler AddBalance interface
type StateDBAdapter struct {
	*state.StateDB
}

// AddBalance implements the VmStateDB AddBalance interface
func (s *StateDBAdapter) AddBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	return s.StateDB.AddBalance(addr, amount, reason)
}

// MultiCoin methods - these are Lux-specific extensions not in geth
func (s *StateDBAdapter) SubBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	// TODO: Implement multi-coin support
	panic("SubBalanceMultiCoin not implemented")
}

func (s *StateDBAdapter) AddBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	// TODO: Implement multi-coin support
	panic("AddBalanceMultiCoin not implemented")
}

func (s *StateDBAdapter) GetBalanceMultiCoin(addr common.Address, coinID common.Hash) *big.Int {
	// TODO: Implement multi-coin support
	return big.NewInt(0)
}

// Logs returns our log types
func (s *StateDBAdapter) Logs() []*types.Log {
	return s.StateDB.Logs()
}

// GetTxHash returns the current transaction hash
func (s *StateDBAdapter) GetTxHash() common.Hash {
	return s.StateDB.GetTxHash()
}

// NewStateDBAdapter creates a new adapter
func NewStateDBAdapter(statedb *state.StateDB) *StateDBAdapter {
	return &StateDBAdapter{StateDB: statedb}
}
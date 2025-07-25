// (c) 2024, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testhelpers

import (
	"math/big"
	
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	gethTypes "github.com/luxfi/geth/core/types"
	"github.com/holiman/uint256"
	"github.com/luxfi/evm/core/state"
)

// StateDBAdapter wraps a state.StateDB to implement the simpler AddBalance interface
type StateDBAdapter struct {
	*state.StateDB
}

// AddBalance implements the simplified AddBalance interface by providing a default reason
func (s *StateDBAdapter) AddBalance(addr common.Address, amount *uint256.Int) {
	// Use a generic balance change reason
	s.StateDB.AddBalance(addr, amount, tracing.BalanceChangeUnspecified)
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

// AddLog implements the Lux-specific AddLog signature
func (s *StateDBAdapter) AddLog(addr common.Address, topics []common.Hash, data []byte, blockNumber uint64) {
	// Convert to geth log format
	log := &gethTypes.Log{
		Address:     addr,
		Topics:      topics,
		Data:        data,
		BlockNumber: blockNumber,
	}
	s.StateDB.AddLog(log)
}

// NewStateDBAdapter creates a new adapter
func NewStateDBAdapter(statedb *state.StateDB) *StateDBAdapter {
	return &StateDBAdapter{StateDB: statedb}
}
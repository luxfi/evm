// Copyright 2025 Lux Industries, Inc.
// This file contains adapters to make VM StateDB compatible with contract.StateDB.

package vm

import (
	"math/big"

	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/luxfi/evm/v2/v2/precompile/contract"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/holiman/uint256"
)

// StateDBAdapter wraps a VM StateDB to implement contract.StateDB
type StateDBAdapter struct {
	StateDB
}

// Ensure StateDBAdapter implements contract.StateDB
var _ contract.StateDB = (*StateDBAdapter)(nil)

// AddLog implements contract.StateDB by converting the log format
func (s *StateDBAdapter) AddLog(log *types.Log) {
	s.StateDB.AddLog(log.Address, log.Topics, log.Data, log.BlockNumber)
}

// GetBalanceMultiCoin implements contract.StateDB
func (s *StateDBAdapter) GetBalanceMultiCoin(addr common.Address, coinID common.Hash) *big.Int {
	return s.StateDB.GetBalanceMultiCoin(addr, coinID)
}

// GetBalance implements contract.StateDB
func (s *StateDBAdapter) GetBalance(addr common.Address) *uint256.Int {
	return s.StateDB.GetBalance(addr)
}

// AddBalance implements contract.StateDB
func (s *StateDBAdapter) AddBalance(addr common.Address, amount *uint256.Int) {
	s.StateDB.AddBalance(addr, amount, tracing.BalanceChangeUnspecified)
}

// SetNonce implements contract.StateDB
func (s *StateDBAdapter) SetNonce(addr common.Address, nonce uint64) {
	s.StateDB.SetNonce(addr, nonce, tracing.NonceChangeUnspecified)
}

// NewStateDBAdapter creates a new StateDBAdapter
func NewStateDBAdapter(stateDB StateDB) *StateDBAdapter {
	return &StateDBAdapter{StateDB: stateDB}
}
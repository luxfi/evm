// (c) 2020-2020, Lux Industries, Inc.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package state provides a caching layer atop the Ethereum state trie.
package state

import (
	"math/big"
	
	ethstate "github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/state/snapshot"
	gethtypes "github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/common"
	evmtypes "github.com/luxfi/evm/v2/v2/core/types"
)

// StateDB wraps go-ethereum's StateDB with minimal extensions
type StateDB struct {
	*ethstate.StateDB
	thash common.Hash
	
	// Multi-coin balances - map[address][coinID]balance
	multiCoinBalances map[common.Address]map[common.Hash]*big.Int
}

// New creates a new state from a given trie.
func New(root common.Hash, db Database, snaps *snapshot.Tree) (*StateDB, error) {
	// Note: geth's New function doesn't take snapshot.Tree anymore
	ethStateDB, err := ethstate.New(root, db)
	if err != nil {
		return nil, err
	}
	return &StateDB{
		StateDB: ethStateDB,
		multiCoinBalances: make(map[common.Address]map[common.Hash]*big.Int),
	}, nil
}

// AddBalanceMultiCoin adds amount to the multi-coin balance for addr and coinID
func (s *StateDB) AddBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	if s.multiCoinBalances[addr] == nil {
		s.multiCoinBalances[addr] = make(map[common.Hash]*big.Int)
	}
	if s.multiCoinBalances[addr][coinID] == nil {
		s.multiCoinBalances[addr][coinID] = new(big.Int)
	}
	s.multiCoinBalances[addr][coinID].Add(s.multiCoinBalances[addr][coinID], amount)
}

// SubBalanceMultiCoin subtracts amount from the multi-coin balance for addr and coinID
func (s *StateDB) SubBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	if s.multiCoinBalances[addr] == nil {
		s.multiCoinBalances[addr] = make(map[common.Hash]*big.Int)
	}
	if s.multiCoinBalances[addr][coinID] == nil {
		s.multiCoinBalances[addr][coinID] = new(big.Int)
	}
	s.multiCoinBalances[addr][coinID].Sub(s.multiCoinBalances[addr][coinID], amount)
}

// GetBalanceMultiCoin returns the multi-coin balance for addr and coinID
func (s *StateDB) GetBalanceMultiCoin(addr common.Address, coinID common.Hash) *big.Int {
	if s.multiCoinBalances[addr] == nil {
		return new(big.Int)
	}
	if s.multiCoinBalances[addr][coinID] == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(s.multiCoinBalances[addr][coinID])
}

// GetTxHash returns the current transaction hash
func (s *StateDB) GetTxHash() common.Hash {
	return s.thash
}

// Copy creates a deep, independent copy of the state.
// It overrides the embedded StateDB's Copy to ensure our wrapper type is returned.
func (s *StateDB) Copy() *StateDB {
	// Copy the underlying geth StateDB
	gethCopy := s.StateDB.Copy()
	
	// Create new wrapper with copied state
	copy := &StateDB{
		StateDB: gethCopy,
		thash:   s.thash,
		multiCoinBalances: make(map[common.Address]map[common.Hash]*big.Int),
	}
	
	// Deep copy multiCoinBalances
	for addr, coins := range s.multiCoinBalances {
		copy.multiCoinBalances[addr] = make(map[common.Hash]*big.Int)
		for coinID, balance := range coins {
			copy.multiCoinBalances[addr][coinID] = new(big.Int).Set(balance)
		}
	}
	
	return copy
}

// AddLog adds a log entry - wrapper to match vm.StateDB interface
func (s *StateDB) AddLog(addr common.Address, topics []common.Hash, data []byte, blockNumber uint64) {
	// Convert to the log type expected by geth StateDB
	log := &gethtypes.Log{
		Address:     addr,
		Topics:      topics,
		Data:        data,
		BlockNumber: blockNumber,
	}
	s.StateDB.AddLog(log)
}

// GetCommittedStateAP1 retrieves a value from the committed storage for AP1
func (s *StateDB) GetCommittedStateAP1(addr common.Address, hash common.Hash) common.Hash {
	// For now, just return the same as GetCommittedState
	// This is a Lux-specific method for a previous Apricot upgrade
	return s.StateDB.GetCommittedState(addr, hash)
}

// GetLogData returns the log topics and data
func (s *StateDB) GetLogData() (topics [][]common.Hash, data [][]byte) {
	logs := s.StateDB.Logs()
	for _, log := range logs {
		topics = append(topics, log.Topics)
		data = append(data, log.Data)
	}
	return topics, data
}

// GetPredicateStorageSlots returns predicate storage slots
func (s *StateDB) GetPredicateStorageSlots(address common.Address, index int) ([]byte, bool) {
	// This is a stub implementation for Lux-specific predicate functionality
	return nil, false
}

// SetPredicateStorageSlots sets predicate storage slots
func (s *StateDB) SetPredicateStorageSlots(address common.Address, predicates [][]byte) {
	// This is a stub implementation for Lux-specific predicate functionality
}

// SelfDestruct marks the account for deletion - wrapper to match vm.StateDB interface
func (s *StateDB) SelfDestruct(addr common.Address) {
	// The geth StateDB.SelfDestruct returns the balance, but vm.StateDB expects no return
	s.StateDB.SelfDestruct(addr)
}

// Selfdestruct6780 marks the account for deletion per EIP-6780 - wrapper to match vm.StateDB interface
func (s *StateDB) Selfdestruct6780(addr common.Address) {
	// The geth StateDB.SelfDestruct6780 returns the balance, but vm.StateDB expects no return
	s.StateDB.SelfDestruct6780(addr)
}

// SetCode sets the code for an account - wrapper to match vm.StateDB interface
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	// The geth StateDB.SetCode returns the code hash, but vm.StateDB expects no return
	s.StateDB.SetCode(addr, code)
}

// SetState sets the storage state - wrapper to match vm.StateDB interface
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	// The geth StateDB.SetState returns the previous value, but vm.StateDB expects no return
	s.StateDB.SetState(addr, key, value)
}

// Logs returns the logs for the current transaction - wrapper to return our log type

func (s *StateDB) Logs() []*evmtypes.Log {
	gethLogs := s.StateDB.Logs()
	if gethLogs == nil {
		return nil
	}
	logs := make([]*evmtypes.Log, len(gethLogs))
	for i, gethLog := range gethLogs {
		logs[i] = &evmtypes.Log{
			Address:     gethLog.Address,
			Topics:      gethLog.Topics,
			Data:        gethLog.Data,
			BlockNumber: gethLog.BlockNumber,
			TxHash:      gethLog.TxHash,
			TxIndex:     gethLog.TxIndex,
			BlockHash:   gethLog.BlockHash,
			Index:       gethLog.Index,
			Removed:     gethLog.Removed,
		}
	}
	return logs
}

// SetTxContext sets the current transaction hash and index
func (s *StateDB) SetTxContext(thash common.Hash, ti int) {
	s.thash = thash
	s.StateDB.SetTxContext(thash, ti)
}


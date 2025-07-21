// (c) 2024, Hanzo Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extstate

import (
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
	"github.com/holiman/uint256"
	"github.com/luxfi/evm/params"
)

type VmStateDB interface {
	vm.StateDB
	Logs() []*types.Log
	GetTxHash() common.Hash
}

type vmStateDB = VmStateDB

type StateDB struct {
	vmStateDB

	// Ordered storage slots to be used in predicate verification as set in the tx access list.
	// Only set in [StateDB.Prepare], and un-modified through execution.
	predicateStorageSlots map[common.Address][][]byte
}

// New creates a new [*StateDB] with the given [VmStateDB], effectively wrapping it
// with additional functionality.
func New(vm VmStateDB) *StateDB {
	return &StateDB{
		vmStateDB:             vm,
		predicateStorageSlots: make(map[common.Address][][]byte),
	}
}

// AddBalance wrapper to match precompile interface (2 params instead of 3)
func (s *StateDB) AddBalance(addr common.Address, amount *uint256.Int) {
	// Call the underlying AddBalance (may have changed signature)
	s.vmStateDB.AddBalance(addr, amount)
}

// SetState wrapper to match precompile interface (no return value)
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	s.vmStateDB.SetState(addr, key, value)
}

// SetNonce wrapper to match precompile interface (2 params instead of 3)
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	// Call the underlying SetNonce
	s.vmStateDB.SetNonce(addr, nonce)
}

// SetCode wrapper to match stateupgrade interface (no return value)
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	// Call the underlying SetCode
	s.vmStateDB.SetCode(addr, code)
}

// AddLog wrapper to match precompile interface
func (s *StateDB) AddLog(log *types.Log) {
	// The underlying StateDB has a different AddLog signature
	// We need to call it with the components of the log
	if log != nil {
		s.vmStateDB.AddLog(log.Address, log.Topics, log.Data, log.BlockNumber)
	}
}

func (s *StateDB) Prepare(rules params.Rules, sender, coinbase common.Address, dst *common.Address, precompiles []common.Address, list types.AccessList) {
	// FIXME: GetRulesExtra doesn't exist in current params package
	// rulesExtra := params.GetRulesExtra(rules)
	// s.predicateStorageSlots = predicate.PreparePredicateStorageSlots(rulesExtra, list)
	// For now, just pass an empty Rules struct
	// TODO: Map our Rules to ethereum Rules properly
	ethRules := ethparams.Rules{
		ChainID: rules.ChainID,
	}
	s.vmStateDB.Prepare(ethRules, sender, coinbase, dst, precompiles, list)
}

// GetLogData returns the underlying topics and data from each log included in the [StateDB].
// Test helper function.
func (s *StateDB) GetLogData() (topics [][]common.Hash, data [][]byte) {
	for _, log := range s.Logs() {
		topics = append(topics, log.Topics)
		data = append(data, common.CopyBytes(log.Data))
	}
	return topics, data
}

// GetPredicateStorageSlots returns the storage slots associated with the address+index pair as
// a byte slice as well as a boolean indicating if the address+index pair exists.
// A list of access tuples can be included within transaction types post EIP-2930. The address
// is declared directly on the access tuple and the index is the i'th occurrence of an access
// tuple with the specified address.
//
// Ex. AccessList[[AddrA, Predicate1], [AddrB, Predicate2], [AddrA, Predicate3]]
// In this case, the caller could retrieve predicates 1-3 with the following calls:
// GetPredicateStorageSlots(AddrA, 0) -> Predicate1
// GetPredicateStorageSlots(AddrB, 0) -> Predicate2
// GetPredicateStorageSlots(AddrA, 1) -> Predicate3
func (s *StateDB) GetPredicateStorageSlots(address common.Address, index int) ([]byte, bool) {
	predicates, exists := s.predicateStorageSlots[address]
	if !exists || index >= len(predicates) {
		return nil, false
	}
	return predicates[index], true
}

// SetPredicateStorageSlots sets the predicate storage slots for the given address
func (s *StateDB) SetPredicateStorageSlots(address common.Address, predicates [][]byte) {
	s.predicateStorageSlots[address] = predicates
}

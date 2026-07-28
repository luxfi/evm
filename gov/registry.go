// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import (
	"math/big"

	"github.com/luxfi/crypto"
	"github.com/luxfi/geth/common"
)

// RegistryAddress is where governed values live.
//
// It sits in the protocol-system band alongside the coinbase blackhole
// (0x0100..0000) and the retired fee vault (0x0100..0002), and like them it has
// no key. Unlike them it is not codeless: gov/solidity/ParamRegistry.sol is
// installed here by a stateUpgrades entry — the install path Lux already runs in
// production on 96369 (params/extras/state_upgrade.go StateUpgradeAccount.Code
// -> core/state_processor_ext.go applyStateUpgrades -> stateupgrade.Configure ->
// state.SetCode) — so contracts can read these values through an ordinary call.
//
// The registry never holds balance. The stranding that put 3867.549624340766
// LUX beyond reach at 0x0100..0000 needs an account that is keyless AND has
// value; this one is keyless and, by having no payable path and never being
// credited, has none.
var RegistryAddress = common.HexToAddress("0x0100000000000000000000000000000000000003")

// Storage layout, fixed to match gov/solidity/ParamRegistry.sol declaration
// order so that one set of slots serves two readers: the node, which reads
// governed values out of the parent state root while validating headers, and
// Solidity, which reads the same slots through ordinary mapping accessors. The
// node writes; nobody else can; everybody reads the same words.
//
//	slot 0  mapping(uint256 => uint256) _value
//	slot 1  mapping(uint256 => bool)    _isSet
//	slot 2  uint256                     lastAppliedEpoch
//	slot 3  uint256                     lastAppliedBlock
const (
	slotValue            uint64 = 0
	slotIsSet            uint64 = 1
	SlotLastAppliedEpoch uint64 = 2
	SlotLastAppliedBlock uint64 = 3
)

// word renders a uint64 as a 32-byte big-endian EVM word.
func word(v uint64) common.Hash { return common.BigToHash(new(big.Int).SetUint64(v)) }

// mappingSlot is Solidity's layout for mapping(uint256 => T) at [base]:
// keccak256(bytes32(key) . bytes32(base)).
func mappingSlot(key uint64, base uint64) common.Hash {
	var buf [64]byte
	copy(buf[0:32], word(key).Bytes())
	copy(buf[32:64], word(base).Bytes())
	return common.BytesToHash(crypto.Keccak256(buf[:]))
}

// ValueSlot is the slot holding the governed value of [id].
func ValueSlot(id ParamID) common.Hash { return mappingSlot(uint64(id), slotValue) }

// IsSetSlot is the slot holding whether [id] has ever been governed. A separate
// flag rather than a sentinel value, because zero is a legal value for three of
// the governed parameters and "unset" must not be confusable with "voted to 0".
func IsSetSlot(id ParamID) common.Hash { return mappingSlot(uint64(id), slotIsSet) }

// ScalarSlot is the slot of a top-level uint256 at [index].
func ScalarSlot(index uint64) common.Hash { return word(index) }

// StateReader is the read half of the state the registry lives in.
type StateReader interface {
	GetState(common.Address, common.Hash) common.Hash
}

// StateWriter is the write half. Only the node implements a caller of this.
type StateWriter interface {
	StateReader
	SetState(common.Address, common.Hash, common.Hash)
}

// Reader answers "what is parameter [id] right now". Overlay consumes it, and
// tests supply fakes, so the fee-config overlay never has to know whether it is
// reading a real state trie.
type Reader interface {
	Param(id ParamID) (uint64, bool)
}

// StateBackedReader reads governed values out of a state trie.
type StateBackedReader struct{ State StateReader }

// Param returns the governed value of [id], and whether it has been set.
//
// The bound is re-checked on the way out. Bounds are compiled in, so a node
// running newer code with a narrower bound will refuse a value an older epoch
// wrote rather than honour it — the constitution wins over the vote, always,
// and in the same direction.
func (r StateBackedReader) Param(id ParamID) (uint64, bool) {
	if r.State == nil {
		return 0, false
	}
	if r.State.GetState(RegistryAddress, IsSetSlot(id)) == (common.Hash{}) {
		return 0, false
	}
	v := r.State.GetState(RegistryAddress, ValueSlot(id)).Big()
	if !v.IsUint64() {
		return 0, false
	}
	u := v.Uint64()
	if !Governable(id, u) {
		return 0, false
	}
	return u, true
}

// Write stores a governed value. It is the only mutation this package performs,
// and core.ApplyGovernance is its only caller.
func Write(state StateWriter, id ParamID, value uint64) {
	state.SetState(RegistryAddress, ValueSlot(id), word(value))
	state.SetState(RegistryAddress, IsSetSlot(id), word(1))
}

// WriteScalar stores a top-level uint256 (the applied-epoch bookkeeping).
func WriteScalar(state StateWriter, index uint64, value uint64) {
	state.SetState(RegistryAddress, ScalarSlot(index), word(value))
}

// ReadScalar reads a top-level uint256.
func ReadScalar(state StateReader, index uint64) uint64 {
	v := state.GetState(RegistryAddress, ScalarSlot(index)).Big()
	if !v.IsUint64() {
		return 0
	}
	return v.Uint64()
}

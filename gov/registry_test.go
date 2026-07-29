// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/require"
)

// fakeState is a storage trie in a map, so the layout can be checked without a
// database. It is also the reason gov depends on nothing from core.
type fakeState map[common.Hash]common.Hash

func (s fakeState) GetState(_ common.Address, k common.Hash) common.Hash { return s[k] }
func (s fakeState) SetState(_ common.Address, k, v common.Hash)          { s[k] = v }

// keccak is Solidity's hash, spelled out here so the layout test derives the
// expected slot independently of the implementation under test.
func keccak(b []byte) common.Hash { return common.BytesToHash(crypto.Keccak256(b)) }

// TestSolidityLayout is the contract between the two readers. The node writes
// these slots in Go; ParamRegistry.sol reads them as ordinary mappings. If the
// derivation drifts, Solidity silently reads zero and every contract that
// depends on a governed value is quietly wrong — so the slot for one known key
// is pinned against Solidity's documented rule, computed here from first
// principles rather than copied from the implementation.
func TestSolidityLayout(t *testing.T) {
	// Solidity: slot(map[k]) at declaration slot p is keccak256(h(k) . p),
	// both operands left-padded to 32 bytes.
	want := func(key, base uint64) common.Hash {
		var buf [64]byte
		copy(buf[0:32], common.BigToHash(new(big.Int).SetUint64(key)).Bytes())
		copy(buf[32:64], common.BigToHash(new(big.Int).SetUint64(base)).Bytes())
		return keccak(buf[:])
	}

	for _, id := range SortedParamIDs() {
		require.Equalf(t, want(uint64(id), 0), ValueSlot(id), "_value[%d] slot", id)
		require.Equalf(t, want(uint64(id), 1), IsSetSlot(id), "_isSet[%d] slot", id)
		require.NotEqual(t, ValueSlot(id), IsSetSlot(id))
	}

	// Top-level scalars occupy their declaration slot verbatim.
	require.Equal(t, common.BigToHash(big.NewInt(2)), ScalarSlot(SlotLastAppliedEpoch))
	require.Equal(t, common.BigToHash(big.NewInt(3)), ScalarSlot(SlotLastAppliedBlock))

	// No mapping slot may collide with a scalar slot, or a governed value would
	// overwrite the bookkeeping.
	for _, id := range SortedParamIDs() {
		require.NotEqual(t, ScalarSlot(SlotLastAppliedEpoch), ValueSlot(id))
		require.NotEqual(t, ScalarSlot(SlotLastAppliedBlock), ValueSlot(id))
		require.NotEqual(t, ScalarSlot(SlotLastAppliedEpoch), IsSetSlot(id))
		require.NotEqual(t, ScalarSlot(SlotLastAppliedBlock), IsSetSlot(id))
	}
}

// TestUnsetIsNotZero: three governed parameters may legitimately be zero, so
// "never voted" and "voted to zero" must be distinguishable. A sentinel value
// would conflate them; a separate flag does not.
func TestUnsetIsNotZero(t *testing.T) {
	s := fakeState{}
	r := StateBackedReader{State: s}

	_, ok := r.Param(ParamMinBlockGasCost)
	require.False(t, ok, "never written must read as unset")

	Write(s, ParamMinBlockGasCost, 0)
	v, ok := r.Param(ParamMinBlockGasCost)
	require.True(t, ok, "written-to-zero must read as SET")
	require.Equal(t, uint64(0), v)
}

// TestReaderRefusesOutOfBounds: bounds are compiled in, the registry is not.
// A node whose code narrows a bound must ignore a value an earlier epoch wrote
// outside it, rather than honour a rule its own binary no longer permits. This
// is what keeps a released bound meaningful after the fact.
func TestReaderRefusesOutOfBounds(t *testing.T) {
	s := fakeState{}
	// Bypass Write's own path to simulate a value stored under a wider bound.
	s[ValueSlot(ParamGasLimit)] = common.BigToHash(big.NewInt(500_000_000)) // > Max
	s[IsSetSlot(ParamGasLimit)] = common.BigToHash(big.NewInt(1))

	_, ok := StateBackedReader{State: s}.Param(ParamGasLimit)
	require.False(t, ok, "a stored value outside the compiled-in bound must not be honoured")

	// Positive control: the same slots with an in-bounds value do resolve.
	s[ValueSlot(ParamGasLimit)] = common.BigToHash(big.NewInt(20_000_000))
	v, ok := StateBackedReader{State: s}.Param(ParamGasLimit)
	require.True(t, ok)
	require.Equal(t, uint64(20_000_000), v)
}

// TestNilStateReadsNothing: a reader with no state answers "not governed"
// rather than panicking, so an unconfigured caller degrades to today's
// behaviour instead of taking the node down.
func TestNilStateReadsNothing(t *testing.T) {
	_, ok := StateBackedReader{}.Param(ParamGasLimit)
	require.False(t, ok)
}

// TestResolveIgnoresUnset: a chain that has never voted must behave exactly as
// it does today, field for field.
func TestResolveIgnoresUnset(t *testing.T) {
	base := mainnetFeeConfig()
	got, ok := Resolve(base, StateBackedReader{State: fakeState{}})
	require.True(t, ok)
	require.True(t, got.Equal(&base), "an empty registry must not change anything")
}

// TestResolveAppliesOnlyWhatWasVoted: one parameter changes, the other seven
// keep their compiled-in values. Governance is not a wholesale replacement of
// the fee schedule.
func TestResolveAppliesOnlyWhatWasVoted(t *testing.T) {
	base := mainnetFeeConfig()
	s := fakeState{}
	Write(s, ParamGasLimit, 20_000_000)

	got, ok := Resolve(base, StateBackedReader{State: s})
	require.True(t, ok)
	require.Equal(t, uint64(20_000_000), got.GasLimit.Uint64())
	require.Equal(t, base.MinBaseFee.Uint64(), got.MinBaseFee.Uint64())
	require.Equal(t, base.TargetBlockRate, got.TargetBlockRate)
	require.Equal(t, base.TargetGas.Uint64(), got.TargetGas.Uint64())
	require.Equal(t, base.BaseFeeChangeDenominator.Uint64(), got.BaseFeeChangeDenominator.Uint64())
	require.Equal(t, base.MinBlockGasCost.Uint64(), got.MinBlockGasCost.Uint64())
	require.Equal(t, base.MaxBlockGasCost.Uint64(), got.MaxBlockGasCost.Uint64())
	require.Equal(t, base.BlockGasCostStep.Uint64(), got.BlockGasCostStep.Uint64())

	// The input is not mutated: a cached chain-config value must not be
	// rewritten under its owner.
	require.Equal(t, uint64(12_000_000), base.GasLimit.Uint64())
}

// TestGovernanceCannotHaltTheChain is the liveness property, and it is not
// hypothetical. Every governed value is voted separately and each passes its
// own bound check, so an epoch that lowers MaxBlockGasCost and a later epoch
// that raises MinBlockGasCost each look legal in isolation while together they
// violate FeeConfig.Verify's Min <= Max rule. If the resolver returned that
// combination, GetFeeConfigAt would start returning an error, every header
// would fail verification, and the chain would stop — a vote would have killed
// the network. Resolve must fall back to the last valid schedule instead.
func TestGovernanceCannotHaltTheChain(t *testing.T) {
	base := mainnetFeeConfig()
	s := fakeState{}

	// Epoch A: max block gas cost down to zero. Legal on its own.
	Write(s, ParamMaxBlockGasCost, 0)
	got, ok := Resolve(base, StateBackedReader{State: s})
	require.True(t, ok, "each change is individually valid")
	require.Equal(t, uint64(0), got.MaxBlockGasCost.Uint64())
	require.NoError(t, got.Verify())

	// Epoch B: min block gas cost up to 500000. Legal on its own. Together:
	// min > max, which FeeConfig.Verify rejects.
	Write(s, ParamMinBlockGasCost, 500_000)
	hostile := overlay(base, StateBackedReader{State: s})
	require.Error(t, hostile.Verify(), "the combination really is invalid")

	got, ok = Resolve(base, StateBackedReader{State: s})
	require.False(t, ok, "Resolve must report that it declined")
	require.True(t, got.Equal(&base), "and must hand back a schedule the node can keep running on")
	require.NoError(t, got.Verify())
}

// TestMainnetValuesAreInBounds: the bounds must admit what mainnet 96369 runs
// today (measured at height 1098193: gasLimit 12000000, baseFee floor 25 gwei),
// or activating governance would put the live chain outside its own
// constitution.
func TestMainnetValuesAreInBounds(t *testing.T) {
	require.True(t, Governable(ParamGasLimit, 12_000_000), "measured mainnet gasLimit")
	require.True(t, Governable(ParamMinBaseFee, 25_000_000_000), "measured mainnet base fee floor")
	require.True(t, Governable(ParamTargetBlockRate, 2))
	require.True(t, Governable(ParamTargetGas, 15_000_000))
	require.True(t, Governable(ParamBaseFeeChangeDenominator, 36))
	require.True(t, Governable(ParamMinBlockGasCost, 0))
	require.True(t, Governable(ParamMaxBlockGasCost, 1_000_000))
	require.True(t, Governable(ParamBlockGasCostStep, 200_000))
}

// TestNoAddressAnywhere is the constraint the owner set, checked as a fact
// about the code rather than a claim in a comment: nothing in this package's
// decision path accepts, stores, or compares an address. The only address it
// knows is the keyless account it writes to.
func TestNoAddressAnywhere(t *testing.T) {
	// Every exported decision-path entry point takes signals and values only;
	// this compiles precisely because none of them has an address parameter.
	var _ func([]Signal) []Decision = Tally
	var _ func(ParamID, uint64) bool = Governable
	var _ func(commontype.FeeConfig, Reader) (commontype.FeeConfig, bool) = Resolve
	var _ func(uint64) uint64 = Required

	// And the one address that exists is keyless: it is in the protocol system
	// band, not derivable from any public key anyone holds.
	require.Equal(t, common.HexToAddress("0x0100000000000000000000000000000000000003"), RegistryAddress)
}

func mainnetFeeConfig() commontype.FeeConfig {
	return commontype.FeeConfig{
		GasLimit:                 big.NewInt(12_000_000), // measured on 96369
		TargetBlockRate:          2,
		MinBaseFee:               big.NewInt(25_000_000_000), // measured on 96369
		TargetGas:                big.NewInt(15_000_000),
		BaseFeeChangeDenominator: big.NewInt(36),
		MinBlockGasCost:          big.NewInt(0),
		MaxBlockGasCost:          big.NewInt(1_000_000),
		BlockGasCostStep:         big.NewInt(200_000),
	}
}

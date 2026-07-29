// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"math/big"
	"testing"

	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/gov"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/plugin/evm/header"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// forkOf96369 is a read-only fork of Lux mainnet: the same chain ID and the
// same fee schedule the live chain reports at height 1098193
// (gasLimit 12000000, base-fee floor 25 gwei, measured over
// https://api.lux.network/v1/bc/C/rpc), with governance switched on from
// genesis. Nothing here can touch mainnet; the point is that the values the
// mechanism starts from are the ones mainnet actually runs.
func forkOf96369(t *testing.T) *params.ChainConfig {
	t.Helper()
	genesisActive := uint64(0)
	config := params.WithExtra(
		&params.ChainConfig{
			ChainID:             big.NewInt(96369), // measured: eth_chainId = 0x17871
			HomesteadBlock:      big.NewInt(0),
			EIP150Block:         big.NewInt(0),
			EIP155Block:         big.NewInt(0),
			EIP158Block:         big.NewInt(0),
			ByzantiumBlock:      big.NewInt(0),
			ConstantinopleBlock: big.NewInt(0),
			PetersburgBlock:     big.NewInt(0),
			IstanbulBlock:       big.NewInt(0),
			MuirGlacierBlock:    big.NewInt(0),
			BerlinBlock:         big.NewInt(0),
			LondonBlock:         big.NewInt(0),
		},
		&extras.ChainConfig{
			FeeConfig:           mainnetFeeSchedule(),
			NetworkUpgrades:     extras.GetDefaultNetworkUpgrades(),
			GovernanceTimestamp: &genesisActive,
			// Deliberately NO precompiles. FeeManager is not enabled here, and
			// neither is RewardManager: this proves governance needs no
			// allowlist, no admin address and no precompile activation to move
			// a value the node reads.
			GenesisPrecompiles: extras.Precompiles{},
		},
	)
	require.NoError(t, params.GetExtra(config).Verify(), "the fork config must be one a node would start on")
	return config
}

// mainnetFeeSchedule is what 96369 reports today.
func mainnetFeeSchedule() commontype.FeeConfig {
	return commontype.FeeConfig{
		GasLimit:                 big.NewInt(12_000_000),     // measured: eth_getBlockByNumber.gasLimit = 0xb71b00
		MinBaseFee:               big.NewInt(25_000_000_000), // measured: baseFeePerGas = 0x5d21dba00
		TargetBlockRate:          2,
		TargetGas:                big.NewInt(15_000_000),
		BaseFeeChangeDenominator: big.NewInt(36),
		MinBlockGasCost:          big.NewInt(0),
		MaxBlockGasCost:          big.NewInt(1_000_000),
		BlockGasCostStep:         big.NewInt(200_000),
	}
}

func govGenesis(config *params.ChainConfig) *Genesis {
	return &Genesis{
		Config:   config,
		Alloc:    types.GenesisAlloc{},
		BaseFee:  big.NewInt(25_000_000_000),
		GasLimit: params.GetExtra(config).FeeConfig.GasLimit.Uint64(),
	}
}

// signalling returns a chain generator in which the first [n] blocks of every
// epoch carry [s] and the rest are silent — the shape a real epoch has when a
// coalition holding n/EpochLength of the stake is signalling.
func signalling(s gov.Signal, n int) func(int, *BlockGen) {
	nonce, ok := s.Encode()
	if !ok {
		panic("test signal must be encodable")
	}
	return func(i int, b *BlockGen) {
		if (i+1)%int(gov.EpochLength) != 0 && (i+1)%int(gov.EpochLength) <= n {
			b.SetNonce(nonce)
		} else if (i+1)%int(gov.EpochLength) == 0 && n == int(gov.EpochLength) {
			b.SetNonce(nonce)
		}
	}
}

// TestVoteChangesAValueTheNodeReads is the whole claim, end to end, on a real
// BlockChain: a supermajority of blocks signal a new gas limit, the epoch
// closes, and from that point the node resolves 20000000 where it resolved
// 12000000 the block before — through BlockChain.GetFeeConfigAt, the same call
// dummy.verifyHeaderGasFields makes on every header it validates.
//
// No transaction is sent. No contract is called. No address appears anywhere in
// the path. The only input is what validators put in the blocks they produce.
func TestVoteChangesAValueTheNodeReads(t *testing.T) {
	config := forkOf96369(t)
	gspec := govGenesis(config)
	engine := dummy.NewCoinbaseFaker()

	const supermajority = 1512 // exactly 3/4 of 2016
	want := uint64(20_000_000)

	// Build one full epoch plus the block that closes it.
	db, blocks, _, err := GenerateChainWithGenesis(
		gspec, engine, int(gov.EpochLength), 2,
		signalling(gov.Signal{ParamID: gov.ParamGasLimit, Value: want}, supermajority),
	)
	require.NoError(t, err)
	require.Len(t, blocks, int(gov.EpochLength))

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	_, err = chain.InsertChain(blocks)
	require.NoError(t, err, "every block must verify under the rules in force when it was made")
	for _, b := range blocks {
		require.NoError(t, chain.Accept(b))
	}
	chain.DrainAcceptorQueue()

	closing := blocks[len(blocks)-1].Header()
	require.Equal(t, gov.EpochLength, closing.Number.Uint64(), "block 2016 closes epoch 1")
	before := blocks[len(blocks)-2].Header()

	// The block BEFORE the epoch closed: still the compiled-in schedule.
	fcBefore, _, err := chain.GetFeeConfigAt(before)
	require.NoError(t, err)
	require.Equal(t, uint64(12_000_000), fcBefore.GasLimit.Uint64(),
		"nothing may change until the epoch actually closes")

	// The block that closed it: the governed value, resolved from state.
	fcAfter, lastChanged, err := chain.GetFeeConfigAt(closing)
	require.NoError(t, err)
	require.Equal(t, want, fcAfter.GasLimit.Uint64(),
		"a passed vote must change the value the node reads")
	require.Equal(t, gov.EpochLength, lastChanged.Uint64())

	// Everything not voted on is untouched.
	require.Equal(t, uint64(25_000_000_000), fcAfter.MinBaseFee.Uint64())
	require.Equal(t, uint64(2), fcAfter.TargetBlockRate)

	// And it is really in the state root, at the Solidity-visible slots, under
	// the keyless registry account.
	st, err := chain.StateAt(closing.Root)
	require.NoError(t, err)
	require.Equal(t, common.BigToHash(big.NewInt(int64(want))), st.GetState(gov.RegistryAddress, gov.ValueSlot(gov.ParamGasLimit)))
	require.Equal(t, common.BigToHash(big.NewInt(1)), st.GetState(gov.RegistryAddress, gov.IsSetSlot(gov.ParamGasLimit)))
	require.Equal(t, uint64(1), gov.ReadScalar(govState{st}, gov.SlotLastAppliedEpoch))

	// The registry holds no value. Keyless plus funded is how 3867 LUX became
	// unreachable at 0x0100..0000; keyless plus empty is safe.
	require.Zero(t, st.GetBalance(gov.RegistryAddress).Uint64())

	// The consensus consequence: header validity now demands the new limit.
	// This is the enforcement, not a report of it — VerifyGasLimit compares for
	// exact equality (plugin/evm/header/gas_limit.go).
	stale := types.CopyHeader(closing)
	stale.Number = new(big.Int).Add(closing.Number, big.NewInt(1))
	stale.GasLimit = 12_000_000 // what a node that ignored the vote would build
	stale.Time = closing.Time + 2
	require.Error(t,
		header.VerifyGasLimit(params.GetExtra(config), fcAfter, closing, stale),
		"a block built on the pre-vote gas limit must now be rejected")

	fresh := types.CopyHeader(stale)
	fresh.GasLimit = want
	require.NoError(t,
		header.VerifyGasLimit(params.GetExtra(config), fcAfter, closing, fresh),
		"and a block built on the governed limit must be accepted")
}

// TestChainKeepsBuildingOnTheGovernedValue: the vote is not just readable, the
// chain runs on it. Extend past the epoch boundary and every subsequent header
// must carry the new limit and pass full verification.
func TestChainKeepsBuildingOnTheGovernedValue(t *testing.T) {
	config := forkOf96369(t)
	gspec := govGenesis(config)
	engine := dummy.NewCoinbaseFaker()
	want := uint64(20_000_000)

	total := int(gov.EpochLength) + 10
	db, blocks, _, err := GenerateChainWithGenesis(
		gspec, engine, total, 2,
		signalling(gov.Signal{ParamID: gov.ParamGasLimit, Value: want}, 1512),
	)
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	// Insert and accept in chunks, the way a running node does; holding 2000+
	// unaccepted blocks exhausts the snapshot diff layers and fails for reasons
	// that have nothing to do with governance.
	for start := 0; start < len(blocks); start += 256 {
		end := min(start+256, len(blocks))
		n, err := chain.InsertChain(blocks[start:end])
		require.NoErrorf(t, err, "inserted %d of %d in chunk [%d,%d)", n, end-start, start, end)
		for _, b := range blocks[start:end] {
			require.NoError(t, chain.Accept(b))
		}
		chain.DrainAcceptorQueue()
	}

	for i, b := range blocks {
		height := uint64(i + 1)
		switch {
		case height <= gov.EpochLength:
			require.Equalf(t, uint64(12_000_000), b.GasLimit(), "block %d predates the decision", height)
		default:
			require.Equalf(t, want, b.GasLimit(), "block %d must be built on the governed limit", height)
		}
	}
}

// TestBelowThresholdChangesNothing: 1511 of 2016 is one block short. The chain
// must come out of the epoch exactly as it went in. Without this the threshold
// is decoration.
func TestBelowThresholdChangesNothing(t *testing.T) {
	config := forkOf96369(t)
	gspec := govGenesis(config)
	engine := dummy.NewCoinbaseFaker()

	db, blocks, _, err := GenerateChainWithGenesis(
		gspec, engine, int(gov.EpochLength), 2,
		signalling(gov.Signal{ParamID: gov.ParamGasLimit, Value: 20_000_000}, 1511),
	)
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)

	closing := blocks[len(blocks)-1].Header()
	fc, _, err := chain.GetFeeConfigAt(closing)
	require.NoError(t, err)
	require.Equal(t, uint64(12_000_000), fc.GasLimit.Uint64(), "1511 of 2016 must decide nothing")

	// The epoch was still counted — "no supermajority" is distinguishable from
	// "governance never ran".
	st, err := chain.StateAt(closing.Root)
	require.NoError(t, err)
	require.Equal(t, uint64(1), gov.ReadScalar(govState{st}, gov.SlotLastAppliedEpoch))
	require.Equal(t, common.Hash{}, st.GetState(gov.RegistryAddress, gov.IsSetSlot(gov.ParamGasLimit)))
}

// TestSilentChainIsUnchanged: with governance active but nobody signalling —
// which is exactly what mainnet looks like the day after activation, since every
// header there carries a zero nonce — a full epoch passes and nothing moves.
func TestSilentChainIsUnchanged(t *testing.T) {
	config := forkOf96369(t)
	gspec := govGenesis(config)
	engine := dummy.NewCoinbaseFaker()

	db, blocks, _, err := GenerateChainWithGenesis(gspec, engine, int(gov.EpochLength), 2, func(i int, b *BlockGen) {})
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)

	closing := blocks[len(blocks)-1].Header()
	require.Equal(t, types.BlockNonce{}, closing.Nonce, "an unsignalled block carries the same zero nonce mainnet does")

	fc, _, err := chain.GetFeeConfigAt(closing)
	require.NoError(t, err)
	expect := mainnetFeeSchedule()
	require.True(t, fc.Equal(&expect), "a silent epoch changes nothing at all")
}

// TestGovernanceOffIsAZeroChange: with GovernanceTimestamp nil — every chain
// running today — signalled blocks are inert. This is the property that makes
// the code safe to ship before anyone decides to switch it on.
func TestGovernanceOffIsAZeroChange(t *testing.T) {
	config := forkOf96369(t)
	params.GetExtra(config).GovernanceTimestamp = nil
	gspec := govGenesis(config)
	engine := dummy.NewCoinbaseFaker()

	db, blocks, _, err := GenerateChainWithGenesis(
		gspec, engine, int(gov.EpochLength), 2,
		signalling(gov.Signal{ParamID: gov.ParamGasLimit, Value: 20_000_000}, int(gov.EpochLength)),
	)
	require.NoError(t, err)

	chain, err := NewBlockChain(db, DefaultCacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err)
	defer chain.Stop()

	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)

	closing := blocks[len(blocks)-1].Header()
	fc, _, err := chain.GetFeeConfigAt(closing)
	require.NoError(t, err)
	require.Equal(t, uint64(12_000_000), fc.GasLimit.Uint64(),
		"a unanimous vote must be inert while governance is off")

	st, err := chain.StateAt(closing.Root)
	require.NoError(t, err)
	require.Equal(t, common.Hash{}, st.GetState(gov.RegistryAddress, gov.IsSetSlot(gov.ParamGasLimit)))
	require.Zero(t, st.GetNonce(gov.RegistryAddress), "the registry account must not even exist")
}

// govState adapts the concrete state to gov's reader interface for assertions.
type govState struct {
	db interface {
		GetState(common.Address, common.Hash) common.Hash
	}
}

func (s govState) GetState(a common.Address, k common.Hash) common.Hash { return s.db.GetState(a, k) }

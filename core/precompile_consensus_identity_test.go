// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"math/big"
	"testing"

	"github.com/luxfi/evm/consensus"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/ids"
	"github.com/luxfi/runtime"
	"github.com/stretchr/testify/require"
)

// precompile_consensus_identity_test.go closes the VM-level seam the in-process
// dex settleHarness MOCKS away: that harness implements contract.AtomicState
// directly, so NetworkID()/CChainID() read struct fields and NEVER traverse
// runtime.FromContext(env.ConsensusContext()). A real VM build resolves identity
// through the production accessibleStateAdapter, which reads it from the chain
// Runtime embedded in the EVM block context's ConsensusContext. If that context
// is not threaded onto the executing EVM, the adapter returns networkID 0 /
// C-Chain Empty and the 0x9999 value path fail-closes on EVERY real-asset swap
// (ErrAssetResolverIdentityMismatch) before any debit. These tests assert the
// real adapter returns the configured identity on BOTH the block/tx execution
// path and the eth_call block-context construction path.

// chainIdentity is a representative non-trivial chain identity (mirrors the
// localnet the e2e runs: networkID 1337, a concrete C-Chain id).
var (
	testNetworkID = uint32(1337)
	testCChainID  = ids.ID{
		0x73, 0xC5, 0x05, 0x73, 0x6D, 0x39, 0x57, 0xA1,
		0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42,
		0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42,
		0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42,
	}
)

func testRuntime() *runtime.Runtime {
	return &runtime.Runtime{
		NetworkID: testNetworkID,
		ChainID:   testCChainID,
		CChainID:  testCChainID,
	}
}

// newEVMWithConsensusCtx builds a real *vm.EVM whose block context carries the
// given consensus context — the exact composition vm.NewEVM uses in production
// (Context: blockCtx), so accessibleStateAdapter reads the SAME seam a live
// 0x9999 call reads. A nil consensusCtx models the defective path (no runtime).
func newEVMWithConsensusCtx(t *testing.T, consensusCtx context.Context) *vm.EVM {
	t.Helper()
	statedb, err := state.New(types.EmptyRootHash, state.NewDatabaseForTesting())
	require.NoError(t, err)

	blockCtx := vm.BlockContext{
		CanTransfer:      CanTransfer,
		Transfer:         Transfer,
		GetHash:          func(uint64) common.Hash { return common.Hash{} },
		BlockNumber:      big.NewInt(1),
		Time:             1,
		Difficulty:       big.NewInt(0),
		BaseFee:          big.NewInt(0),
		GasLimit:         8_000_000,
		ConsensusContext: consensusCtx,
	}
	return vm.NewEVM(blockCtx, statedb, params.TestChainConfig, vm.Config{})
}

// adapterFor wraps a real geth precompile environment (the production type
// accessibleStateAdapter consumes) over the EVM, exactly as the EVM does when it
// dispatches a call to a stateful precompile.
func adapterFor(evm *vm.EVM) *accessibleStateAdapter {
	env := vm.NewPrecompileEnvironment(evm, common.Address{}, common.HexToAddress("0x9999"), 1_000_000, false)
	return &accessibleStateAdapter{env: env}
}

// TestAccessibleStateIdentityFromConsensusContext is the regression guard: with
// the chain Runtime embedded in the EVM block context's ConsensusContext, the
// production accessibleStateAdapter resolves the real (networkID, cChainID)
// through runtime.FromContext — the seam the 0x9999 swap path cross-checks.
func TestAccessibleStateIdentityFromConsensusContext(t *testing.T) {
	rt := testRuntime()
	evm := newEVMWithConsensusCtx(t, runtime.WithContext(context.Background(), rt))
	a := adapterFor(evm)

	require.Equal(t, testNetworkID, a.NetworkID(),
		"accessibleStateAdapter.NetworkID() must equal the configured chain networkID, "+
			"resolved via runtime.FromContext(env.ConsensusContext())")
	require.Equal(t, testCChainID, a.CChainID(),
		"accessibleStateAdapter.CChainID() must equal the configured chain C-Chain id")
	require.Equal(t, testCChainID, a.ChainID(),
		"accessibleStateAdapter.ChainID() must equal the configured chain id")
}

// TestAccessibleStateIdentityWithoutRuntimeIsZero documents the defect's failure
// mode: when the consensus context carries no runtime (the request-context path
// that the eth_call bug returned, and the bare-blockContext tx path), the adapter
// reports networkID 0 / C-Chain Empty — which fail-closes the value path. This is
// the exact (0, Empty) the real-chain e2e observed; the fixes ensure production
// never reaches the EVM in this state for a chain that has a runtime.
func TestAccessibleStateIdentityWithoutRuntimeIsZero(t *testing.T) {
	// No runtime in the context (models runtime.WithContext on a non-*Runtime, or
	// the raw RPC request context the eth_call path used to return).
	evm := newEVMWithConsensusCtx(t, context.Background())
	a := adapterFor(evm)

	require.Equal(t, uint32(0), a.NetworkID(),
		"with no runtime in the consensus context, NetworkID() must be 0 (fail-closed identity)")
	require.Equal(t, ids.Empty, a.CChainID(),
		"with no runtime in the consensus context, CChainID() must be Empty (fail-closed identity)")
}

// stubChainContext is a minimal core.ChainContext whose ConsensusContext()
// returns the configured context. It models the production composition used by
// NewEVMBlockContext: the miner passes the running *BlockChain, and the eth_call
// doCall path passes internal/ethapi.ChainContext (which, after the api.go fix,
// returns the backend blockchain's runtime-bearing consensus context). A nil
// consensusCtx models a backend that exposes no runtime — the builder must then
// leave the block context's ConsensusContext nil rather than fabricate one.
type stubChainContext struct {
	consensusCtx context.Context
}

func (s *stubChainContext) Engine() consensus.Engine                  { return nil }
func (s *stubChainContext) GetHeader(common.Hash, uint64) *types.Header { return nil }
func (s *stubChainContext) Config() *params.ChainConfig               { return params.TestChainConfig }
func (s *stubChainContext) ConsensusContext() context.Context         { return s.consensusCtx }

// TestNewEVMBlockContextThreadsConsensusContext asserts the block-context builder
// used by BOTH the miner (tx) path and the eth_call path threads the chain's
// consensus context (carrying the runtime) onto the EVM block context — so the
// identity reaches the precompile. This is the construction the eth_call doCall
// path performs via NewChainContext; with the api.go fix the ChainContext returns
// the backend blockchain's runtime-bearing context instead of the request context.
func TestNewEVMBlockContextThreadsConsensusContext(t *testing.T) {
	rt := testRuntime()
	consensusCtx := runtime.WithContext(context.Background(), rt)

	header := &types.Header{
		Number:     big.NewInt(1),
		Time:       1,
		Difficulty: big.NewInt(0),
		GasLimit:   8_000_000,
		BaseFee:    big.NewInt(0),
	}

	// ChainContext that exposes the runtime-bearing consensus context (post-fix):
	// the block context must carry it, and the runtime must be recoverable from it.
	withCtx := NewEVMBlockContext(header, &stubChainContext{consensusCtx: consensusCtx}, nil)
	require.NotNil(t, withCtx.ConsensusContext, "block context must carry the consensus context")
	gotRT := runtime.FromContext(withCtx.ConsensusContext)
	require.NotNil(t, gotRT, "runtime must be recoverable from the threaded consensus context")
	require.Equal(t, testNetworkID, gotRT.NetworkID, "threaded runtime networkID must match")
	require.Equal(t, testCChainID, gotRT.CChainID, "threaded runtime C-Chain id must match")

	// ChainContext that exposes no consensus context: the builder must leave it nil
	// rather than fabricate one (the precompile then fail-closes, by design).
	bare := NewEVMBlockContext(header, &stubChainContext{consensusCtx: nil}, nil)
	require.Nil(t, bare.ConsensusContext,
		"with no consensus context from the chain, the block context must carry no consensus context")
}

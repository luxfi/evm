// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package ethapi

import (
	"context"
	"testing"

	"github.com/luxfi/evm/consensus"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/rpc"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/ids"
	"github.com/luxfi/runtime"
	"github.com/stretchr/testify/require"
)

// chaincontext_consensus_test.go guards the eth_call/estimateGas seam: the block
// context for a call is built via NewEVMBlockContext(header, NewChainContext(ctx,
// b), nil), which reads ChainContext.ConsensusContext(). That method previously
// returned the RPC REQUEST context (no runtime), so a stateful precompile invoked
// on the call path resolved networkID 0 / C-Chain Empty via runtime.FromContext
// and fail-closed before any state change — the exact failure the real-chain e2e
// caught on initialize/swap over a real ERC-20. The fix returns the backend
// blockchain's consensus context (carrying the chain Runtime). These tests assert
// that, and the fallback to the request context when no backend context exists.

// consensusCtxBackend implements just enough of ChainContextBackend and exposes a
// ConsensusContext() — modelling *eth.EthAPIBackend after the fix (which returns
// b.eth.blockchain.ConsensusContext()).
type consensusCtxBackend struct {
	consensusCtx context.Context
}

func (b *consensusCtxBackend) Engine() consensus.Engine { return nil }
func (b *consensusCtxBackend) HeaderByNumber(context.Context, rpc.BlockNumber) (*types.Header, error) {
	return nil, nil
}
func (b *consensusCtxBackend) ChainConfig() *params.ChainConfig { return params.TestChainConfig }
func (b *consensusCtxBackend) ConsensusContext() context.Context { return b.consensusCtx }

// bareCtxBackend implements ChainContextBackend but does NOT expose a consensus
// context — models a backend with no chain runtime; ConsensusContext() must fall
// back to the request context.
type bareCtxBackend struct{}

func (bareCtxBackend) Engine() consensus.Engine { return nil }
func (bareCtxBackend) HeaderByNumber(context.Context, rpc.BlockNumber) (*types.Header, error) {
	return nil, nil
}
func (bareCtxBackend) ChainConfig() *params.ChainConfig { return params.TestChainConfig }

func TestChainContextConsensusContextReturnsBackendRuntime(t *testing.T) {
	rt := &runtime.Runtime{
		NetworkID: 1337,
		ChainID: ids.ID{0x42},
		CChainID: ids.ID{0x42},
	}
	backendCtx := runtime.WithContext(context.Background(), rt)
	// A DISTINCT request context with no runtime — the value the buggy path returned.
	reqCtx := context.WithValue(context.Background(), struct{ k string }{"req"}, "request-scoped")

	cc := NewChainContext(reqCtx, &consensusCtxBackend{consensusCtx: backendCtx})
	got := cc.ConsensusContext()

	require.NotSame(t, reqCtx, got,
		"ConsensusContext() must NOT return the RPC request context when the backend exposes a consensus context")
	gotRT := runtime.FromContext(got)
	require.NotNil(t, gotRT,
		"the chain Runtime must be recoverable from the consensus context the eth_call path uses")
	require.Equal(t, uint32(1337), gotRT.NetworkID, "recovered runtime networkID must match the chain identity")
	require.Equal(t, ids.ID{0x42}, gotRT.CChainID, "recovered runtime C-Chain id must match the chain identity")
}

func TestChainContextConsensusContextFallsBackToRequestContext(t *testing.T) {
	reqCtx := context.WithValue(context.Background(), struct{ k string }{"req"}, "request-scoped")

	cc := NewChainContext(reqCtx, bareCtxBackend{})
	got := cc.ConsensusContext()

	require.Same(t, reqCtx, got,
		"when the backend exposes no consensus context, ConsensusContext() must fall back to the request context")
}

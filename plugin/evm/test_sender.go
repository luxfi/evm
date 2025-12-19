// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/luxfi/p2p"
)

// TestSender is a test implementation of the p2p.Sender interface
type TestSender struct {
	T *testing.T

	CantSendGossip         bool
	CantSendGossipSpecific bool
	CantSendRequest        bool
	CantSendResponse       bool
	CantSendError          bool

	SendGossipF             func(context.Context, p2p.SendConfig, []byte) error
	SendGossipSpecificF     func(context.Context, set.Set[ids.NodeID], []byte) error
	SendRequestF            func(context.Context, set.Set[ids.NodeID], uint32, []byte) error
	SendResponseF           func(context.Context, ids.NodeID, uint32, []byte) error
	SendErrorF              func(context.Context, ids.NodeID, uint32, int32, string) error
	SendCrossChainRequestF  func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainResponseF func(context.Context, ids.ID, uint32, []byte) error
	SendCrossChainErrorF    func(context.Context, ids.ID, uint32, int32, string) error

	// Channel for capturing sent gossip messages
	SentGossip chan []byte
}

// SendGossip implements p2p.Sender
func (s *TestSender) SendGossip(ctx context.Context, config p2p.SendConfig, msg []byte) error {
	if s.SendGossipF != nil {
		return s.SendGossipF(ctx, config, msg)
	}
	if s.SentGossip != nil {
		s.SentGossip <- msg
	}
	if s.CantSendGossip && s.T != nil {
		s.T.Helper()
		s.T.Fatal("unexpectedly called SendGossip")
	}
	return nil
}

// SendGossipSpecific sends gossip to specific nodes
func (s *TestSender) SendGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], msg []byte) error {
	if s.SendGossipSpecificF != nil {
		return s.SendGossipSpecificF(ctx, nodeIDs, msg)
	}
	if s.CantSendGossipSpecific && s.T != nil {
		s.T.Helper()
		s.T.Fatal("unexpectedly called SendGossipSpecific")
	}
	return nil
}

// SendRequest implements p2p.Sender
func (s *TestSender) SendRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error {
	if s.SendRequestF != nil {
		return s.SendRequestF(ctx, nodeIDs, requestID, request)
	}
	if s.CantSendRequest && s.T != nil {
		s.T.Helper()
		s.T.Fatal("unexpectedly called SendRequest")
	}
	return nil
}

// SendResponse implements p2p.Sender
func (s *TestSender) SendResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	if s.SendResponseF != nil {
		return s.SendResponseF(ctx, nodeID, requestID, response)
	}
	if s.CantSendResponse && s.T != nil {
		s.T.Helper()
		s.T.Fatal("unexpectedly called SendResponse")
	}
	return nil
}

// SendError implements p2p.Sender
func (s *TestSender) SendError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendErrorF != nil {
		return s.SendErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	if s.CantSendError && s.T != nil {
		s.T.Helper()
		s.T.Fatal("unexpectedly called SendError")
	}
	return nil
}

// SendCrossChainRequest sends a cross-chain request
func (s *TestSender) SendCrossChainRequest(ctx context.Context, chainID ids.ID, requestID uint32, requestBytes []byte) error {
	if s.SendCrossChainRequestF != nil {
		return s.SendCrossChainRequestF(ctx, chainID, requestID, requestBytes)
	}
	return nil
}

// SendCrossChainResponse sends a cross-chain response
func (s *TestSender) SendCrossChainResponse(ctx context.Context, chainID ids.ID, requestID uint32, responseBytes []byte) error {
	if s.SendCrossChainResponseF != nil {
		return s.SendCrossChainResponseF(ctx, chainID, requestID, responseBytes)
	}
	return nil
}

// SendCrossChainError sends a cross-chain error
func (s *TestSender) SendCrossChainError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if s.SendCrossChainErrorF != nil {
		return s.SendCrossChainErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	return nil
}

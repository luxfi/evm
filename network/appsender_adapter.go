// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/quasar/engine/core"
	"github.com/luxfi/node/quasar/engine/core/appsender"
	"github.com/luxfi/node/utils/set"
)

// appSenderAdapter adapts core.AppSender to appsender.AppSender
type appSenderAdapter struct {
	sender core.AppSender
}

// NewAppSenderAdapter creates a new adapter
func NewAppSenderAdapter(sender core.AppSender) appsender.AppSender {
	return &appSenderAdapter{sender: sender}
}

// SendAppRequest implements appsender.AppSender
func (a *appSenderAdapter) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error {
	// Convert set to slice
	nodes := make([]ids.NodeID, 0, nodeIDs.Len())
	for nodeID := range nodeIDs {
		nodes = append(nodes, nodeID)
	}
	return a.sender.SendAppRequest(ctx, nodes, requestID, request)
}

// SendAppResponse implements appsender.AppSender
func (a *appSenderAdapter) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	return a.sender.SendAppResponse(ctx, nodeID, requestID, response)
}

// SendAppGossip implements appsender.AppSender
func (a *appSenderAdapter) SendAppGossip(ctx context.Context, config appsender.SendConfig, gossip []byte) error {
	// For now, ignore the config and just send the gossip
	return a.sender.SendAppGossip(ctx, gossip)
}

// SendAppError implements appsender.AppSender
func (a *appSenderAdapter) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	return a.sender.SendAppError(ctx, nodeID, requestID, errorCode, errorMessage)
}

// SendCrossChainAppRequest implements appsender.AppSender
func (a *appSenderAdapter) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error {
	return a.sender.SendCrossChainAppRequest(ctx, chainID, requestID, request)
}

// SendCrossChainAppResponse implements appsender.AppSender
func (a *appSenderAdapter) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error {
	return a.sender.SendCrossChainAppResponse(ctx, chainID, requestID, response)
}

// SendAppGossipSpecific implements appsender.AppSender
func (a *appSenderAdapter) SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], gossip []byte) error {
	// Convert set to slice and send to each node
	for range nodeIDs {
		if err := a.sender.SendAppGossip(ctx, gossip); err != nil {
			return err
		}
	}
	return nil
}
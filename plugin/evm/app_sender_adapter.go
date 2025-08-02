// (c) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/v2/quasar/engine/core"
	"github.com/luxfi/node/v2/quasar/engine/core/appsender"
	"github.com/luxfi/node/v2/utils/set"
)

// appSenderAdapter adapts core.AppSender to appsender.AppSender
type appSenderAdapter struct {
	sender core.AppSender
}

// newAppSenderAdapter creates a new adapter
func newAppSenderAdapter(sender core.AppSender) appsender.AppSender {
	return &appSenderAdapter{sender: sender}
}

func (a *appSenderAdapter) SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, message []byte) error {
	return a.sender.SendAppRequest(ctx, nodeIDs.List(), requestID, message)
}

func (a *appSenderAdapter) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, message []byte) error {
	return a.sender.SendAppResponse(ctx, nodeID, requestID, message)
}

func (a *appSenderAdapter) SendAppGossip(ctx context.Context, _ appsender.SendConfig, message []byte) error {
	// Ignore SendConfig as the old interface doesn't support it
	return a.sender.SendAppGossip(ctx, message)
}

func (a *appSenderAdapter) SendAppGossipSpecific(ctx context.Context, nodeIDs set.Set[ids.NodeID], message []byte) error {
	// The old interface doesn't support specific gossip, so we use regular gossip
	return a.sender.SendAppGossip(ctx, message)
}

func (a *appSenderAdapter) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, message []byte) error {
	return a.sender.SendCrossChainAppRequest(ctx, chainID, requestID, message)
}

func (a *appSenderAdapter) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, message []byte) error {
	return a.sender.SendCrossChainAppResponse(ctx, chainID, requestID, message)
}

// Additional methods to fully implement core.AppSender
func (a *appSenderAdapter) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	return a.sender.SendAppError(ctx, nodeID, requestID, errorCode, errorMessage)
}
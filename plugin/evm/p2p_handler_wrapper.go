// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"time"

	"github.com/luxfi/evm/v2/plugin/evm/message"
	"github.com/luxfi/node/v2/quasar/engine/core"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/v2/network/p2p"
)

// p2pHandlerWrapper wraps a message.RequestHandler to implement p2p.Handler
type p2pHandlerWrapper struct {
	handler message.RequestHandler
}

// newP2PHandlerWrapper creates a new p2p handler wrapper
func newP2PHandlerWrapper(handler message.RequestHandler) p2p.Handler {
	return &p2pHandlerWrapper{handler: handler}
}

// AppGossip implements p2p.Handler
func (h *p2pHandlerWrapper) AppGossip(ctx context.Context, nodeID ids.NodeID, gossipBytes []byte) {
	// Network request handler doesn't handle gossip
}

// AppRequest implements p2p.Handler
func (h *p2pHandlerWrapper) AppRequest(ctx context.Context, nodeID ids.NodeID, deadline time.Time, requestBytes []byte) ([]byte, *core.AppError) {
	// For now, we'll return nil since we don't have the requestID
	// In a real implementation, you'd need to parse the request and route it appropriately
	return nil, nil
}

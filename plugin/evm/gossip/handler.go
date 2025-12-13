// Copyright (C) 2019-2025, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gossip

import (
	"context"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/p2p"
	"github.com/luxfi/p2p/gossip"
	"github.com/prometheus/client_golang/prometheus"
)

var _ p2p.Handler = (*txGossipHandler)(nil)

func NewTxGossipHandler[T gossip.Gossipable](
	logger log.Logger,
	marshaller gossip.Marshaller[T],
	mempool gossip.Set[T],
	metrics gossip.Metrics,
	maxMessageSize int,
	throttlingPeriod time.Duration,
	requestsPerPeer float64,
	validators p2p.ValidatorSet,
	registerer prometheus.Registerer,
	namespace string,
) (*txGossipHandler, error) {
	// push gossip messages can be handled from any peer
	handler := gossip.NewHandler(
		logger,
		marshaller,
		mempool,
		metrics,
		maxMessageSize,
		nil, // bloomFilter - not used for tx gossip
	)

	// Create a sliding window throttler for rate limiting
	throttler := p2p.NewSlidingWindowThrottler(
		throttlingPeriod,
		int(requestsPerPeer), // Convert float64 to int for limit
	)

	throttledHandler := p2p.NewThrottlerHandler(
		handler,
		throttler,
		logger,
	)

	// pull gossip requests are filtered by validators and are throttled
	// to prevent spamming
	validatorHandler := p2p.NewValidatorHandler(
		throttledHandler,
		validators,
		logger,
	)

	return &txGossipHandler{
		gossipHandler:  handler,
		requestHandler: validatorHandler,
	}, nil
}

type txGossipHandler struct {
	gossipHandler  p2p.Handler
	requestHandler p2p.Handler
}

// Gossip implements p2p.Handler
func (t *txGossipHandler) Gossip(ctx context.Context, nodeID ids.NodeID, gossipBytes []byte) {
	t.gossipHandler.Gossip(ctx, nodeID, gossipBytes)
}

// Request implements p2p.Handler
func (t *txGossipHandler) Request(ctx context.Context, nodeID ids.NodeID, deadline time.Time, requestBytes []byte) ([]byte, *p2p.Error) {
	return t.requestHandler.Request(ctx, nodeID, deadline, requestBytes)
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/codec"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/math/set"
	"github.com/luxfi/metric"
	"github.com/luxfi/p2p"

	"github.com/luxfi/consensus/version"
)

// SyncedNetworkClient is the interface required by the state sync client
type SyncedNetworkClient interface {
	SendSyncedAppRequestAny(ctx context.Context, minVersion *version.Application, request []byte) ([]byte, ids.NodeID, error)
	SendSyncedAppRequest(ctx context.Context, nodeID ids.NodeID, request []byte) ([]byte, error)
	Gossip(msg []byte) error
	TrackBandwidth(nodeID ids.NodeID, bandwidth float64)
}

// Network handles peer-to-peer networking for the EVM
// It embeds p2p.Network to provide the base functionality
type Network struct {
	*p2p.Network

	sender      p2p.Sender
	codec       codec.Manager
	maxRequests int64
	metrics     metric.Registerer
	log         log.Logger

	// Request tracking for sync
	requestsLock    sync.Mutex
	pendingRequests map[uint32]chan []byte
	nextRequestID   uint32

	// Request handler - stores the message.RequestHandler for sync operations
	// We use interface{} here to avoid import cycles with plugin/evm/message
	requestHandler interface{}

	// Shutdown
	closed    bool
	closeLock sync.Mutex
}

// NewNetwork creates a new Network instance
func NewNetwork(
	ctx context.Context,
	sender p2p.Sender,
	codec codec.Manager,
	maxOutboundActiveRequests int64,
	metrics metric.Registerer,
) (*Network, error) {
	logger := log.New()

	var p2pNet *p2p.Network
	var err error

	if sender != nil {
		p2pNet, err = p2p.NewNetwork(logger, sender, metrics, "evm")
		if err != nil {
			return nil, err
		}
	}

	return &Network{
		Network:         p2pNet,
		sender:          sender,
		codec:           codec,
		maxRequests:     maxOutboundActiveRequests,
		metrics:         metrics,
		log:             logger,
		pendingRequests: make(map[uint32]chan []byte),
	}, nil
}

// Shutdown stops the network
func (n *Network) Shutdown() {
	n.closeLock.Lock()
	defer n.closeLock.Unlock()
	n.closed = true

	// Cancel all pending requests
	n.requestsLock.Lock()
	for _, ch := range n.pendingRequests {
		close(ch)
	}
	n.pendingRequests = make(map[uint32]chan []byte)
	n.requestsLock.Unlock()
}

// P2PValidators returns the p2p validators if available
func (n *Network) P2PValidators() *p2p.Validators {
	// Return nil - validators are set up separately
	return nil
}

// SetRequestHandler sets the handler for incoming requests
// Accepts any type to avoid import cycles with plugin/evm/message
func (n *Network) SetRequestHandler(handler interface{}) {
	n.requestHandler = handler
}

// Size returns the number of connected peers
func (n *Network) Size() int {
	if n.Network == nil || n.Network.Peers == nil {
		return 0
	}
	return len(n.Network.Peers.Sample(1000)) // Approximate count
}

// SendSyncedAppRequestAny sends a request to any available peer
func (n *Network) SendSyncedAppRequestAny(ctx context.Context, minVersion *version.Application, request []byte) ([]byte, ids.NodeID, error) {
	if n.Network == nil || n.Network.Peers == nil {
		return nil, ids.EmptyNodeID, ErrNoPeers
	}

	peers := n.Network.Peers.Sample(1)
	if len(peers) == 0 {
		return nil, ids.EmptyNodeID, ErrNoPeers
	}

	// Try the first available peer
	nodeID := peers[0]
	response, err := n.SendSyncedAppRequest(ctx, nodeID, request)
	return response, nodeID, err
}

// SendSyncedAppRequest sends a request to a specific peer
func (n *Network) SendSyncedAppRequest(ctx context.Context, nodeID ids.NodeID, request []byte) ([]byte, error) {
	if n.sender == nil {
		return nil, ErrNoSender
	}

	n.closeLock.Lock()
	if n.closed {
		n.closeLock.Unlock()
		return nil, ErrNetworkClosed
	}
	n.closeLock.Unlock()

	// Create a channel to receive the response
	responseChan := make(chan []byte, 1)
	requestID := n.allocateRequestID(responseChan)
	defer n.freeRequestID(requestID)

	// Send the request
	nodeIDs := set.NewSet[ids.NodeID](1)
	nodeIDs.Add(nodeID)
	if err := n.sender.SendRequest(ctx, nodeIDs, requestID, request); err != nil {
		return nil, err
	}

	// Wait for response with timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response, ok := <-responseChan:
		if !ok {
			return nil, ErrRequestCancelled
		}
		return response, nil
	}
}

// Gossip broadcasts a message to all peers
func (n *Network) Gossip(msg []byte) error {
	if n.sender == nil {
		return ErrNoSender
	}

	if n.Network == nil || n.Network.Peers == nil {
		return nil // No peers to gossip to
	}

	peers := n.Network.Peers.Sample(100) // Sample up to 100 peers
	if len(peers) == 0 {
		return nil // No peers to gossip to
	}

	nodeIDs := set.NewSet[ids.NodeID](len(peers))
	for _, peer := range peers {
		nodeIDs.Add(peer)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return n.sender.SendGossip(ctx, p2p.SendConfig{NodeIDs: nodeIDs}, msg)
}

// TrackBandwidth records bandwidth usage for a peer
func (n *Network) TrackBandwidth(nodeID ids.NodeID, bandwidth float64) {
	// TODO: Implement bandwidth tracking metrics
}

// AppResponse handles an incoming response
func (n *Network) AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	n.requestsLock.Lock()
	ch, ok := n.pendingRequests[requestID]
	n.requestsLock.Unlock()

	if ok {
		select {
		case ch <- response:
		default:
		}
	}
	return nil
}

// allocateRequestID allocates a new request ID and registers the response channel
func (n *Network) allocateRequestID(responseChan chan []byte) uint32 {
	n.requestsLock.Lock()
	defer n.requestsLock.Unlock()

	requestID := n.nextRequestID
	n.nextRequestID++
	n.pendingRequests[requestID] = responseChan
	return requestID
}

// freeRequestID frees a request ID
func (n *Network) freeRequestID(requestID uint32) {
	n.requestsLock.Lock()
	defer n.requestsLock.Unlock()
	delete(n.pendingRequests, requestID)
}

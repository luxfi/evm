// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/geth/metrics"
	"github.com/luxfi/evm/plugin/evm/message"
	syncHandlers "github.com/luxfi/node/state_sync/handlers"
	syncStats "github.com/luxfi/node/state_sync/handlers/stats"
	"github.com/luxfi/geth/trie"
	"github.com/luxfi/warp/backend"
	// warpHandlers "github.com/luxfi/warp/handlers" // TODO: restore when handlers are fixed
)

var _ message.RequestHandler = &networkHandler{}

type networkHandler struct {
	stateTrieLeafsRequestHandler *syncHandlers.LeafsRequestHandler
	blockRequestHandler          *syncHandlers.BlockRequestHandler
	codeRequestHandler           *syncHandlers.CodeRequestHandler
	// signatureRequestHandler      *warpHandlers.SignatureRequestHandler // TODO: restore when warp is integrated from node
}

// newNetworkHandler constructs the handler for serving network requests.
func newNetworkHandler(
	provider syncHandlers.SyncDataProvider,
	diskDB ethdb.KeyValueReader,
	evmTrieDB *triedb.Database,
	warpBackend warp.Backend,
	networkCodec interfaces.Codec,
) message.RequestHandler {
	syncStats := syncStats.NewHandlerStats(metrics.Enabled)
	return &networkHandler{
		stateTrieLeafsRequestHandler: syncHandlers.NewLeafsRequestHandler(evmTrieDB, provider, networkCodec, syncStats),
		blockRequestHandler:          syncHandlers.NewBlockRequestHandler(provider, networkCodec, syncStats),
		codeRequestHandler:           syncHandlers.NewCodeRequestHandler(diskDB, networkCodec, syncStats),
		// signatureRequestHandler:      warpHandlers.NewSignatureRequestHandler(warpBackend, networkCodec), // TODO: restore
	}
}

func (n networkHandler) HandleStateTrieLeafsRequest(ctx context.Context, nodeID interfaces.NodeID, requestID uint32, leafsRequest message.LeafsRequest) ([]byte, error) {
	return n.stateTrieLeafsRequestHandler.OnLeafsRequest(ctx, nodeID, requestID, leafsRequest)
}

func (n networkHandler) HandleBlockRequest(ctx context.Context, nodeID interfaces.NodeID, requestID uint32, blockRequest message.BlockRequest) ([]byte, error) {
	return n.blockRequestHandler.OnBlockRequest(ctx, nodeID, requestID, blockRequest)
}

func (n networkHandler) HandleCodeRequest(ctx context.Context, nodeID interfaces.NodeID, requestID uint32, codeRequest message.CodeRequest) ([]byte, error) {
	return n.codeRequestHandler.OnCodeRequest(ctx, nodeID, requestID, codeRequest)
}

func (n networkHandler) HandleMessageSignatureRequest(ctx context.Context, nodeID interfaces.NodeID, requestID uint32, messageSignatureRequest message.MessageSignatureRequest) ([]byte, error) {
	// TODO: restore when warp is integrated from node
	return nil, nil
}

func (n networkHandler) HandleBlockSignatureRequest(ctx context.Context, nodeID interfaces.NodeID, requestID uint32, blockSignatureRequest message.BlockSignatureRequest) ([]byte, error) {
	// TODO: restore when warp is integrated from node
	return nil, nil
}

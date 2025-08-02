// Copyright (C) 2020-2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"
	"testing"
	"time"

	commonEng "github.com/luxfi/node/v2/quasar/engine/core"
	"github.com/luxfi/evm/v2/core/types"
	"github.com/luxfi/evm/v2/utils"
	"github.com/luxfi/evm/v2/peer"
	"github.com/luxfi/node/v2/quasar"
	"github.com/luxfi/node/v2/quasar/consensus/engine/enginetest"
	"github.com/luxfi/node/v2/quasar/validators"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/v2/proto/pb/sdk"
	luxlog "github.com/luxfi/log"
	"github.com/luxfi/node/v2/utils/set"
	agoUtils "github.com/luxfi/node/v2/utils"
	"github.com/luxfi/database/memdb"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// testAppSender is a test implementation of commonEng.AppSender
type testAppSender struct {
	SendAppRequestF            func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, msg []byte) error
	SendAppResponseF           func(ctx context.Context, nodeID ids.NodeID, requestID uint32, msg []byte) error
	SendAppErrorF              func(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
	SendAppGossipF             func(ctx context.Context, msg []byte) error
	SendCrossChainAppRequestF  func(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error
	SendCrossChainAppResponseF func(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error
	SendCrossChainAppErrorF    func(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error
}

func (t *testAppSender) SendAppRequest(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, msg []byte) error {
	if t.SendAppRequestF != nil {
		return t.SendAppRequestF(ctx, nodeIDs, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, msg []byte) error {
	if t.SendAppResponseF != nil {
		return t.SendAppResponseF(ctx, nodeID, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if t.SendAppErrorF != nil {
		return t.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

func (t *testAppSender) SendAppGossip(ctx context.Context, msg []byte) error {
	if t.SendAppGossipF != nil {
		return t.SendAppGossipF(ctx, msg)
	}
	return nil
}

func (t *testAppSender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error {
	if t.SendCrossChainAppRequestF != nil {
		return t.SendCrossChainAppRequestF(ctx, chainID, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error {
	if t.SendCrossChainAppResponseF != nil {
		return t.SendCrossChainAppResponseF(ctx, chainID, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if t.SendCrossChainAppErrorF != nil {
		return t.SendCrossChainAppErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	return nil
}

func TestEthTxGossip(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	consensusCtx := utils.TestConsensusContext()
	validatorState := utils.NewTestValidatorState()
	consensusCtx.ValidatorState = validatorState

	responseSender := &enginetest.SenderStub{
		SentAppResponse: make(chan []byte, 1),
	}
	vm := &VM{
		p2pSender: responseSender,
	}

	db := memdb.New()
	// Create a custom app sender that implements the simpler interface
	appSender := &testAppSender{
		SendAppGossipF: func(context.Context, []byte) error { return nil },
	}
	require.NoError(vm.Initialize(
		ctx,
		consensusCtx,
		db,
		[]byte(genesisJSONLatest),
		nil,
		nil,
		[]*commonEng.Fx{},
		appSender,
	))
	require.NoError(vm.SetState(ctx, quasar.NormalOp))

	defer func() {
		require.NoError(vm.Shutdown(ctx))
	}()

	// sender for the peer requesting gossip from [vm]
	peerSender := &enginetest.SenderStub{
		SentAppRequest: make(chan []byte, 1),
	}

	network, err := peer.NewNetwork(luxlog.NewNoOpLogger(), peerSender, prometheus.NewRegistry(), "")
	require.NoError(err)
	client := network.NewClient(peer.TxGossipHandlerID)

	// we only accept gossip requests from validators
	requestingNodeID := ids.GenerateTestNodeID()
	require.NoError(vm.Network.Connected(ctx, requestingNodeID, nil))
	validatorState.GetCurrentHeightF = func(context.Context) (uint64, error) {
		return 0, nil
	}
	validatorState.GetValidatorSetF = func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
		return map[ids.NodeID]*validators.GetValidatorOutput{
			requestingNodeID: {
				NodeID: requestingNodeID,
				Weight: 1,
			},
		}, nil
	}

	// Ask the VM for any new transactions. We should get nothing at first.
	emptyBloomFilter, err := peer.NewBloomFilter(prometheus.NewRegistry(), "", txGossipBloomMinTargetElements, txGossipBloomTargetFalsePositiveRate, txGossipBloomResetFalsePositiveRate)
	require.NoError(err)
	emptyBloomFilterBytes, _ := emptyBloomFilter.Marshal()
	request := &sdk.PullGossipRequest{
		Filter: emptyBloomFilterBytes,
		Salt:   agoUtils.RandomBytes(32),
	}

	requestBytes, err := proto.Marshal(request)
	require.NoError(err)

	wg := &sync.WaitGroup{}
	wg.Add(1)
	onResponse := func(_ context.Context, nodeID ids.NodeID, responseBytes []byte, err error) {
		require.NoError(err)

		response := &sdk.PullGossipResponse{}
		require.NoError(proto.Unmarshal(responseBytes, response))
		require.Empty(response.Gossip)
		wg.Done()
	}
	require.NoError(client.AppRequest(ctx, set.Of(vm.ctx.NodeID), requestBytes, onResponse))
	require.NoError(vm.AppRequest(ctx, requestingNodeID, 1, time.Time{}, <-peerSender.SentAppRequest))
	require.NoError(network.AppResponse(ctx, consensusCtx.NodeID, 1, <-responseSender.SentAppResponse))
	wg.Wait()

	// Issue a tx to the VM
	address := testEthAddrs[0]
	key := testKeys[0]
	tx := types.NewTransaction(0, address, big.NewInt(10), 21000, big.NewInt(testMinGasPrice), nil)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(vm.chainConfig.ChainID), key)
	require.NoError(err)

	errs := vm.txPool.Add([]*types.Transaction{signedTx}, true, true)
	require.Len(errs, 1)
	require.Nil(errs[0])

	// wait so we aren't throttled by the vm
	time.Sleep(5 * time.Second)

	marshaller := GossipEthTxMarshaller{}
	// Ask the VM for new transactions. We should get the newly issued tx.
	wg.Add(1)
	onResponse = func(_ context.Context, nodeID ids.NodeID, responseBytes []byte, err error) {
		require.NoError(err)

		response := &sdk.PullGossipResponse{}
		require.NoError(proto.Unmarshal(responseBytes, response))
		require.Len(response.Gossip, 1)

		gotTx, err := marshaller.UnmarshalGossip(response.Gossip[0])
		require.NoError(err)
		require.Equal(signedTx.Hash(), gotTx.Tx.Hash())

		wg.Done()
	}
	require.NoError(client.AppRequest(ctx, set.Of(vm.ctx.NodeID), requestBytes, onResponse))
	require.NoError(vm.AppRequest(ctx, requestingNodeID, 3, time.Time{}, <-peerSender.SentAppRequest))
	require.NoError(network.AppResponse(ctx, consensusCtx.NodeID, 3, <-responseSender.SentAppResponse))
	wg.Wait()
}

func TestEthTxPushGossipOutbound(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	consensusCtx := utils.TestConsensusContext()
	sender := &enginetest.SenderStub{
		SentAppGossip: make(chan []byte, 1),
	}

	vm := &VM{
		ethTxPullGossiper: peer.NoOpGossiper{},
	}

	require.NoError(vm.Initialize(
		ctx,
		consensusCtx,
		memdb.New(),
		[]byte(genesisJSONLatest),
		nil,
		nil,
		make(chan core.Message),
		nil,
		sender,
	))
	require.NoError(vm.SetState(ctx, quasar.NormalOp))

	defer func() {
		require.NoError(vm.Shutdown(ctx))
	}()

	address := testEthAddrs[0]
	key := testKeys[0]
	tx := types.NewTransaction(0, address, big.NewInt(10), 21000, big.NewInt(testMinGasPrice), nil)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(vm.chainConfig.ChainID), key)
	require.NoError(err)

	// issue a tx
	require.NoError(vm.txPool.Add([]*types.Transaction{signedTx}, true, true)[0])
	vm.ethTxPushGossiper.Get().Add(&GossipEthTx{signedTx})

	sent := <-sender.SentAppGossip
	got := &sdk.PushGossip{}

	// we should get a message that has the protocol prefix and the gossip
	// message
	require.Equal(byte(peer.TxGossipHandlerID), sent[0])
	require.NoError(proto.Unmarshal(sent[1:], got))

	marshaller := GossipEthTxMarshaller{}
	require.Len(got.Gossip, 1)
	gossipedTx, err := marshaller.UnmarshalGossip(got.Gossip[0])
	require.NoError(err)
	require.Equal(ids.ID(signedTx.Hash()), gossipedTx.GossipID())
}

// Tests that a gossiped tx is added to the mempool and forwarded
func TestEthTxPushGossipInbound(t *testing.T) {
	require := require.New(t)
	ctx := context.Background()
	consensusCtx := utils.TestConsensusContext()

	sender := &enginetest.Sender{}
	vm := &VM{
		ethTxPullGossiper: peer.NoOpGossiper{},
	}

	require.NoError(vm.Initialize(
		ctx,
		consensusCtx,
		memdb.New(),
		[]byte(genesisJSONLatest),
		nil,
		nil,
		make(chan core.Message),
		nil,
		sender,
	))
	require.NoError(vm.SetState(ctx, quasar.NormalOp))

	defer func() {
		require.NoError(vm.Shutdown(ctx))
	}()

	address := testEthAddrs[0]
	key := testKeys[0]
	tx := types.NewTransaction(0, address, big.NewInt(10), 21000, big.NewInt(testMinGasPrice), nil)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(vm.chainConfig.ChainID), key)
	require.NoError(err)

	marshaller := GossipEthTxMarshaller{}
	gossipedTx := &GossipEthTx{
		Tx: signedTx,
	}
	gossipedTxBytes, err := marshaller.MarshalGossip(gossipedTx)
	require.NoError(err)

	inboundGossip := &sdk.PushGossip{
		Gossip: [][]byte{gossipedTxBytes},
	}

	inboundGossipBytes, err := proto.Marshal(inboundGossip)
	require.NoError(err)

	inboundGossipMsg := append(binary.AppendUvarint(nil, peer.TxGossipHandlerID), inboundGossipBytes...)
	require.NoError(vm.AppGossip(ctx, ids.EmptyNodeID, inboundGossipMsg))

	require.True(vm.txPool.Has(signedTx.Hash()))
}

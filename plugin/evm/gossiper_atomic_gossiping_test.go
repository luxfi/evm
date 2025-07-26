// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/evm/plugin/evm/atomic"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/network/p2p"
	"github.com/luxfi/node/proto/pb/sdk"
	"github.com/luxfi/node/utils/set"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	commonEng "github.com/luxfi/node/consensus/engine"
)

// show that a txID discovered from gossip is requested to the same node only if
// the txID is unknown
func TestMempoolAtmTxsAppGossipHandling(t *testing.T) {
	assert := assert.New(t)

	tvm := newVM(t, testVMConfig{})
	defer func() {
		assert.NoError(tvm.vm.Shutdown(context.Background()))
	}()

	nodeID := ids.GenerateTestNodeID()

	var (
		txGossiped     int
		txGossipedLock sync.Mutex
		txRequested    bool
	)
	tvm.appSender.CantSendAppGossip = false
	tvm.appSender.SendAppGossipF = func(context.Context, commonEng.SendConfig, []byte) error {
		txGossipedLock.Lock()
		defer txGossipedLock.Unlock()

		txGossiped++
		return nil
	}
	tvm.appSender.SendAppRequestF = func(context.Context, set.Set[ids.NodeID], uint32, []byte) error {
		txRequested = true
		return nil
	}

	// Create conflicting transactions
	importTxs := createImportTxOptions(t, tvm.atomicVM, tvm.atomicMemory)
	tx, conflictingTx := importTxs[0], importTxs[1]

	// gossip tx and check it is accepted and gossiped
	marshaller := atomic.GossipAtomicTxMarshaller{}
	gossipTx := &atomic.GossipAtomicTx{Tx: tx}
	txBytes, err := marshaller.MarshalGossip(gossipTx)
	assert.NoError(err)
	tvm.vm.ctx.Lock.Unlock()

	msgBytes, err := buildAtomicPushGossip(txBytes)
	assert.NoError(err)

	// show that no txID is requested
	assert.NoError(tvm.vm.AppGossip(context.Background(), nodeID, msgBytes))
	time.Sleep(500 * time.Millisecond)

	tvm.vm.ctx.Lock.Lock()

	assert.False(txRequested, "tx should not have been requested")
	txGossipedLock.Lock()
	assert.Equal(0, txGossiped, "tx should not have been gossiped")
	txGossipedLock.Unlock()
	assert.True(tvm.atomicVM.AtomicMempool.Has(tx.ID()))

	tvm.vm.ctx.Lock.Unlock()

	// show that tx is not re-gossiped
	assert.NoError(tvm.vm.AppGossip(context.Background(), nodeID, msgBytes))

	tvm.vm.ctx.Lock.Lock()

	txGossipedLock.Lock()
	assert.Equal(0, txGossiped, "tx should not have been gossiped")
	txGossipedLock.Unlock()

	// show that conflicting tx is not added to mempool
	marshaller = atomic.GossipAtomicTxMarshaller{}
	gossipConflictingTx := &atomic.GossipAtomicTx{Tx: conflictingTx}
	txBytes, err = marshaller.MarshalGossip(gossipConflictingTx)
	assert.NoError(err)

	tvm.vm.ctx.Lock.Unlock()

	msgBytes, err = buildAtomicPushGossip(txBytes)
	assert.NoError(err)
	assert.NoError(tvm.vm.AppGossip(context.Background(), nodeID, msgBytes))

	tvm.vm.ctx.Lock.Lock()

	assert.False(tvm.atomicVM.AtomicMempool.Has(conflictingTx.ID()))
}

// show that txs already marked as invalid are not re-requested on gossiping
func TestMempoolAtmTxsAppGossipHandlingDiscardedTx(t *testing.T) {
	assert := assert.New(t)

	tvm := newVM(t, testVMConfig{})
	defer func() {
		assert.NoError(tvm.vm.Shutdown(context.Background()))
	}()
	mempool := tvm.atomicVM.AtomicMempool

	var (
		txGossiped     int
		txGossipedLock sync.Mutex
		txRequested    bool
	)
	tvm.appSender.CantSendAppGossip = false
	tvm.appSender.SendAppGossipF = func(context.Context, commonEng.SendConfig, []byte) error {
		txGossipedLock.Lock()
		defer txGossipedLock.Unlock()

		txGossiped++
		return nil
	}
	tvm.appSender.SendAppRequestF = func(context.Context, set.Set[ids.NodeID], uint32, []byte) error {
		txRequested = true
		return nil
	}

	// Create a transaction and mark it as invalid by discarding it
	importTxs := createImportTxOptions(t, tvm.atomicVM, tvm.atomicMemory)
	tx, conflictingTx := importTxs[0], importTxs[1]
	txID := tx.ID()

	mempool.AddRemoteTx(tx)
	mempool.NextTx()
	mempool.DiscardCurrentTx(txID)

	// Check the mempool does not contain the discarded transaction
	assert.False(mempool.Has(txID))

	// Gossip the transaction ID
	nodeID := ids.GenerateTestNodeID()
	marshaller := atomic.GossipAtomicTxMarshaller{}
	gossipTx := &atomic.GossipAtomicTx{Tx: tx}
	txBytes, err := marshaller.MarshalGossip(gossipTx)
	assert.NoError(err)

	tvm.vm.ctx.Lock.Unlock()

	msgBytes, err := buildAtomicPushGossip(txBytes)
	assert.NoError(err)

	// show that no txID is requested
	assert.NoError(tvm.vm.AppGossip(context.Background(), nodeID, msgBytes))
	time.Sleep(500 * time.Millisecond)

	tvm.vm.ctx.Lock.Lock()

	assert.False(txRequested, "tx should not have been requested")
	txGossipedLock.Lock()
	assert.Equal(0, txGossiped, "tx should not have been gossiped")
	txGossipedLock.Unlock()
	assert.False(mempool.Has(txID), "discarded tx should not be in the atomic mempool")

	// Gossip conflicting tx and check that it is accepted
	conflictingGossipTx := &atomic.GossipAtomicTx{Tx: conflictingTx}
	txBytes, err = marshaller.MarshalGossip(conflictingGossipTx)
	assert.NoError(err)
	tvm.vm.ctx.Lock.Unlock()

	msgBytes, err = buildAtomicPushGossip(txBytes)
	assert.NoError(err)
	assert.NoError(tvm.vm.AppGossip(context.Background(), nodeID, msgBytes))

	tvm.vm.ctx.Lock.Lock()

	assert.True(mempool.Has(conflictingTx.ID()), "conflicting tx should be in the atomic mempool")
}

func buildAtomicPushGossip(msg []byte) ([]byte, error) {
	pushGossip := &sdk.PushGossip{
		Gossip: [][]byte{msg},
	}
	pushGossipBytes, err := proto.Marshal(pushGossip)
	if err != nil {
		return nil, err
	}
	// Atomic Tx Gossip handler ID is 0x00
	return append(binary.AppendUvarint(nil, p2p.AtomicTxGossipProtocol), pushGossipBytes...), nil
}
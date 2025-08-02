// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"encoding/binary"
	"sync"
	"testing"
	"time"

	evmatomic "github.com/luxfi/evm/v2/plugin/evm/atomic"
	nodeatomic "github.com/luxfi/node/v2/chains/atomic"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/v2/network/p2p"
	"github.com/luxfi/node/v2/proto/pb/sdk"
	"github.com/luxfi/node/v2/utils/set"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	commonEng "github.com/luxfi/node/v2/quasar/consensus/engine"
	"github.com/luxfi/node/v2/quasar/consensus/engine/enginetest"
	"github.com/luxfi/node/v2/network/p2p/gossip"
	"github.com/luxfi/node/v2/vms/components/lux"
	"github.com/luxfi/node/v2/quasar/engine/core/appsender"
)

// testVMConfig contains configuration for test VM
type testVMConfig struct {
	genesisJSON string
}

// testVM wraps VM instance and related test utilities
type testVM struct {
	vm           *VM
	atomicVM     *VM  // The atomic VM is the same as the main VM
	atomicMemory *nodeatomic.Memory
	appSender    *enginetest.Sender
}

// newVM creates a new VM instance for testing
func newVM(t *testing.T, config testVMConfig) *testVM {
	if config.genesisJSON == "" {
		config.genesisJSON = genesisJSONLatest
	}
	
	issuer, vm, _, appSender := GenesisVM(t, true, config.genesisJSON, "", "")
	
	// Consume the initial message
	select {
	case <-issuer:
	default:
	}
	
	// Since SharedMemory is an interface and we can't cast directly to atomic.Memory,
	// we'll set it to nil for now. Tests that need atomic memory will need to be updated.
	var atomicMemory *nodeatomic.Memory = nil
	
	return &testVM{
		vm:           vm,
		atomicVM:     vm,  // The atomic VM is the same as the main VM
		atomicMemory: atomicMemory,
		appSender:    appSender,
	}
}

// createImportTxOptions creates a set of import transactions for testing
func createImportTxOptions(t *testing.T, atomicVM *VM, atomicMemory *nodeatomic.Memory) []*evmatomic.Tx {
	// Create some test UTXOs and import transactions
	// This is a simplified version - you may need to add more realistic test data
	importTx1 := &evmatomic.Tx{
		UnsignedAtomicTx: &evmatomic.UnsignedImportTx{
			NetworkID:    exportTestNetworkID,
			BlockchainID: exportTestCChainID,
			SourceChain:  exportTestXChainID,
			ImportedInputs: []*lux.TransferableInput{
				// Add test inputs
			},
			Outs: []evmatomic.EVMOutput{
				{
					Address: exportTestEthAddrs[0],
					Amount:  1000000,
					AssetID: exportTestLUXAssetID,
				},
			},
		},
	}
	
	importTx2 := &evmatomic.Tx{
		UnsignedAtomicTx: &evmatomic.UnsignedImportTx{
			NetworkID:    exportTestNetworkID,
			BlockchainID: exportTestCChainID,
			SourceChain:  exportTestXChainID,
			ImportedInputs: []*lux.TransferableInput{
				// Add test inputs that conflict with importTx1
			},
			Outs: []evmatomic.EVMOutput{
				{
					Address: exportTestEthAddrs[0],
					Amount:  2000000,
					AssetID: exportTestLUXAssetID,
				},
			},
		},
	}
	
	return []*evmatomic.Tx{importTx1, importTx2}
}

// TODO: Fix these tests to work with the new VM structure
// The tests expect AtomicMempool which no longer exists in the current VM implementation
/*
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
	tvm.appSender.SendAppGossipF = func(context.Context, appsender.SendConfig, []byte) error {
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
	marshaller := evmatomic.GossipAtomicTxMarshaller{}
	gossipTx := &evmatomic.GossipAtomicTx{Tx: tx}
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
	marshaller = evmatomic.GossipAtomicTxMarshaller{}
	gossipConflictingTx := &evmatomic.GossipAtomicTx{Tx: conflictingTx}
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
	tvm.appSender.SendAppGossipF = func(context.Context, appsender.SendConfig, []byte) error {
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
	marshaller := evmatomic.GossipAtomicTxMarshaller{}
	gossipTx := &evmatomic.GossipAtomicTx{Tx: tx}
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

*/

func buildAtomicPushGossip(msg []byte) ([]byte, error) {
	pushGossip := &sdk.PushGossip{
		Gossip: [][]byte{msg},
	}
	pushGossipBytes, err := proto.Marshal(pushGossip)
	if err != nil {
		return nil, err
	}
	// Atomic Tx Gossip handler ID is 0x00
	// TODO: Use correct protocol ID from p2p package when available
	atomicTxGossipProtocol := uint64(0x00)
	return append(binary.AppendUvarint(nil, atomicTxGossipProtocol), pushGossipBytes...), nil
}
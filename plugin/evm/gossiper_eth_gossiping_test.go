// Copyright (C) 2019-2025, Lux Partners Limited. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/set"

	commonEng "github.com/luxfi/node/quasar/consensus/engine"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/crypto"

	"github.com/stretchr/testify/assert"

	"github.com/luxfi/evm/v2/v2/core"
	"github.com/luxfi/evm/v2/v2/params"
	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/luxfi/node/network/p2p/gossip"
	"github.com/luxfi/node/quasar/consensus/engine/enginetest"
	nodeatomic "github.com/luxfi/node/chains/atomic"
	enginecore "github.com/luxfi/node/quasar/engine/core"
)


func fundAddressByGenesis(addrs []common.Address) (string, error) {
	balance := big.NewInt(0xffffffffffffff)
	genesis := &core.Genesis{
		Difficulty: common.Big0,
		GasLimit:   uint64(5000000),
	}
	funds := make(types.GenesisAlloc)
	for _, addr := range addrs {
		funds[addr] = types.GenesisAccount{
			Balance: balance,
		}
	}
	genesis.Alloc = funds
	genesis.Config = params.TestChainConfig

	bytes, err := json.Marshal(genesis)
	return string(bytes), err
}

func getValidEthTxsGossiper(key *ecdsa.PrivateKey, count int, gasPrice *big.Int) []*types.Transaction {
	res := make([]*types.Transaction, count)

	to := common.Address{}
	amount := big.NewInt(0)
	gasLimit := uint64(37000)

	for i := 0; i < count; i++ {
		tx, _ := types.SignTx(
			types.NewTransaction(
				uint64(i),
				to,
				amount,
				gasLimit,
				gasPrice,
				[]byte(strings.Repeat("aaaaaaaaaa", 100))),
			types.HomesteadSigner{}, key)
		tx.SetTime(time.Now().Add(-1 * time.Minute))
		res[i] = tx
	}
	return res
}

// show that a geth tx discovered from gossip is requested to the same node that
// gossiped it
func TestMempoolEthTxsAppGossipHandling(t *testing.T) {
	assert := assert.New(t)

	key, err := crypto.GenerateKey()
	assert.NoError(err)

	addr := crypto.PubkeyToAddress(key.PublicKey)

	genesisJSON, err := fundAddressByGenesis([]common.Address{addr})
	assert.NoError(err)

	tvm := newVM(t, testVMConfig{
		genesisJSON: genesisJSON,
	})
	defer func() {
		err := tvm.vm.Shutdown(context.Background())
		assert.NoError(err)
	}()
	// TODO: These methods no longer exist in the current txPool implementation
	// tvm.vm.txPool.SetGasTip(common.Big1)
	// tvm.vm.txPool.SetMinFee(common.Big0)

	var (
		wg          sync.WaitGroup
		txRequested bool
	)
	tvm.appSender.CantSendAppGossip = false
	tvm.appSender.SendAppRequestF = func(_ context.Context, _ set.Set[ids.NodeID], _ uint32, _ []byte) error {
		txRequested = true
		return nil
	}
	wg.Add(1)
	tvm.appSender.SendAppGossipF = func(_ context.Context, _ enginecore.SendConfig, _ []byte) error {
		wg.Done()
		return nil
	}

	// prepare a tx
	tx := getValidEthTxsGossiper(key, 1, common.Big1)[0]

	// Txs must be submitted over the API to be included in push gossip.
	// (i.e., txs received via p2p are not included in push gossip)
	err = tvm.vm.eth.APIBackend.SendTx(context.Background(), tx)
	assert.NoError(err)
	assert.False(txRequested, "tx should not be requested")

	// wait for transaction to be re-gossiped
	attemptAwait(t, &wg, 5*time.Second)
}

func attemptAwait(t *testing.T, wg *sync.WaitGroup, delay time.Duration) {
	ticker := make(chan struct{})

	// Wait for [wg] and then close [ticket] to indicate that
	// the wait group has finished.
	go func() {
		wg.Wait()
		close(ticker)
	}()

	select {
	case <-time.After(delay):
		t.Fatal("Timed out waiting for wait group to complete")
	case <-ticker:
		// The wait group completed without issue
	}
}
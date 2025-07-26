// (c) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"

	"github.com/luxfi/evm/core/txpool"
	"github.com/luxfi/evm/core/types"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/network/p2p/gossip"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	_ gossip.Gossipable               = (*GossipEthTx)(nil)
	_ gossip.Marshaller[*GossipEthTx] = (*GossipEthTxMarshaller)(nil)
)

type GossipEthTxMarshaller struct{}

func (g GossipEthTxMarshaller) MarshalGossip(tx *GossipEthTx) ([]byte, error) {
	return tx.Tx.MarshalBinary()
}

func (g GossipEthTxMarshaller) UnmarshalGossip(bytes []byte) (*GossipEthTx, error) {
	tx := &types.Transaction{}
	if err := tx.UnmarshalBinary(bytes); err != nil {
		return nil, err
	}
	return &GossipEthTx{
		Tx: tx,
	}, nil
}

type GossipEthTx struct {
	Tx *types.Transaction
}

func (tx *GossipEthTx) GossipID() ids.ID {
	return ids.ID(common.Hash(tx.Tx.Hash()))
}

// GossipEthTxPool is an implementation of gossip.Set[*GossipEthTx]
type GossipEthTxPool struct {
	txPool *txpool.TxPool
}

func NewGossipEthTxPool(txPool *txpool.TxPool, sdkMetrics *prometheus.Registry) (*GossipEthTxPool, error) {
	return &GossipEthTxPool{
		txPool: txPool,
	}, nil
}

func (g *GossipEthTxPool) Add(tx *GossipEthTx) error {
	errs := g.txPool.Add([]*types.Transaction{tx.Tx}, true, false)
	return errs[0]
}

func (g *GossipEthTxPool) Has(txID ids.ID) bool {
	return g.txPool.Has(common.Hash(txID))
}

func (g *GossipEthTxPool) Iterate(f func(*GossipEthTx) bool) {
	_, pending := g.txPool.Content()
	for _, txs := range pending {
		for _, tx := range txs {
			if !f(&GossipEthTx{Tx: tx}) {
				return
			}
		}
	}
}

func (g *GossipEthTxPool) GetFilter() ([]byte, []byte) {
	return nil, nil
}

// Subscribe subscribes to transaction pool events
func (g *GossipEthTxPool) Subscribe(ctx context.Context) {
	// This method should block and handle transaction pool events
	// For now, we'll just wait for the context to be cancelled
	<-ctx.Done()
}

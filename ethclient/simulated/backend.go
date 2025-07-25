// Copyright 2023 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package simulated

import (
	"errors"
	"math/big"
	"time"

	"github.com/luxfi/node/utils/timer/mockable"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/constants"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/core/rawdb"
	"github.com/luxfi/evm/core/types"
	"github.com/luxfi/evm/eth"
	"github.com/luxfi/evm/eth/ethconfig"
	"github.com/luxfi/evm/ethclient"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/node"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/rpc"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/ethdb"
)

// fakePushGossiper is a no-op gossiper for simulated backend

type fakePushGossiper struct{}

func (*fakePushGossiper) Add(*types.Transaction) {}

// Client exposes the methods provided by the Ethereum RPC client.
type Client interface {
	ethclient.Client
}

// simClient wraps ethclient. This exists to prevent extracting ethclient.Client
// from the Client interface returned by Backend.
type simClient struct {
	client ethclient.Client
}

// Implement all ethclient.Client interface methods by delegating to the wrapped client
func (s simClient) Client() *rpc.Client { return s.client.Client() }
func (s simClient) Close() { s.client.Close() }
func (s simClient) ChainID(ctx context.Context) (*big.Int, error) { return s.client.ChainID(ctx) }
func (s simClient) BlockByHash(ctx context.Context, hash common.Hash) (*types.Block, error) {
	return s.client.BlockByHash(ctx, hash)
}
func (s simClient) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	return s.client.BlockByNumber(ctx, number)
}
func (s simClient) BlockNumber(ctx context.Context) (uint64, error) {
	return s.client.BlockNumber(ctx)
}
func (s simClient) BlockReceipts(ctx context.Context, blockNrOrHash rpc.BlockNumberOrHash) ([]*types.Receipt, error) {
	return s.client.BlockReceipts(ctx, blockNrOrHash)
}
func (s simClient) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return s.client.HeaderByHash(ctx, hash)
}
func (s simClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return s.client.HeaderByNumber(ctx, number)
}
func (s simClient) TransactionByHash(ctx context.Context, hash common.Hash) (tx *types.Transaction, isPending bool, err error) {
	return s.client.TransactionByHash(ctx, hash)
}
func (s simClient) TransactionSender(ctx context.Context, tx *types.Transaction, block common.Hash, index uint) (common.Address, error) {
	return s.client.TransactionSender(ctx, tx, block, index)
}
func (s simClient) TransactionCount(ctx context.Context, blockHash common.Hash) (uint, error) {
	return s.client.TransactionCount(ctx, blockHash)
}
func (s simClient) TransactionInBlock(ctx context.Context, blockHash common.Hash, index uint) (*types.Transaction, error) {
	return s.client.TransactionInBlock(ctx, blockHash, index)
}
func (s simClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return s.client.TransactionReceipt(ctx, txHash)
}
func (s simClient) SyncProgress(ctx context.Context) error {
	return s.client.SyncProgress(ctx)
}
func (s simClient) SubscribeNewAcceptedTransactions(ctx context.Context, ch chan<- *common.Hash) (iface.Subscription, error) {
	return s.client.SubscribeNewAcceptedTransactions(ctx, ch)
}
func (s simClient) SubscribeNewPendingTransactions(ctx context.Context, ch chan<- *common.Hash) (iface.Subscription, error) {
	return s.client.SubscribeNewPendingTransactions(ctx, ch)
}
func (s simClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (iface.Subscription, error) {
	return s.client.SubscribeNewHead(ctx, ch)
}
func (s simClient) NetworkID(ctx context.Context) (*big.Int, error) {
	return s.client.NetworkID(ctx)
}
func (s simClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	return s.client.BalanceAt(ctx, account, blockNumber)
}
func (s simClient) BalanceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (*big.Int, error) {
	return s.client.BalanceAtHash(ctx, account, blockHash)
}
func (s simClient) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
	return s.client.StorageAt(ctx, account, key, blockNumber)
}
func (s simClient) StorageAtHash(ctx context.Context, account common.Address, key common.Hash, blockHash common.Hash) ([]byte, error) {
	return s.client.StorageAtHash(ctx, account, key, blockHash)
}
func (s simClient) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
	return s.client.CodeAt(ctx, account, blockNumber)
}
func (s simClient) CodeAtHash(ctx context.Context, account common.Address, blockHash common.Hash) ([]byte, error) {
	return s.client.CodeAtHash(ctx, account, blockHash)
}
func (s simClient) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return s.client.NonceAt(ctx, account, blockNumber)
}
func (s simClient) NonceAtHash(ctx context.Context, account common.Address, blockHash common.Hash) (uint64, error) {
	return s.client.NonceAtHash(ctx, account, blockHash)
}
func (s simClient) FilterLogs(ctx context.Context, q iface.FilterQuery) ([]types.Log, error) {
	return s.client.FilterLogs(ctx, q)
}
func (s simClient) SubscribeFilterLogs(ctx context.Context, q iface.FilterQuery, ch chan<- types.Log) (iface.Subscription, error) {
	return s.client.SubscribeFilterLogs(ctx, q, ch)
}
func (s simClient) AcceptedCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return s.client.AcceptedCodeAt(ctx, account)
}
func (s simClient) AcceptedNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return s.client.AcceptedNonceAt(ctx, account)
}
func (s simClient) AcceptedCallContract(ctx context.Context, call iface.CallMsg) ([]byte, error) {
	return s.client.AcceptedCallContract(ctx, call)
}
func (s simClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return s.client.SuggestGasPrice(ctx)
}
func (s simClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return s.client.SuggestGasTipCap(ctx)
}
func (s simClient) EstimateGas(ctx context.Context, call iface.CallMsg) (gas uint64, err error) {
	return s.client.EstimateGas(ctx, call)
}
func (s simClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return s.client.SendTransaction(ctx, tx)
}
func (s simClient) CallContract(ctx context.Context, call iface.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return s.client.CallContract(ctx, call, blockNumber)
}
func (s simClient) CallContractAtHash(ctx context.Context, call iface.CallMsg, blockHash common.Hash) ([]byte, error) {
	return s.client.CallContractAtHash(ctx, call, blockHash)
}
func (s simClient) AsymmetricKeyLocalAvailable(ctx context.Context, addr common.Address) (bool, error) {
	return s.client.AsymmetricKeyLocalAvailable(ctx, addr)
}
func (s simClient) AsymmetricKeyMaxChunks(ctx context.Context) (int, error) {
	return s.client.AsymmetricKeyMaxChunks(ctx)
}
func (s simClient) NotifyL1Validators(ctx context.Context, txHash common.Hash) error {
	return s.client.NotifyL1Validators(ctx, txHash)
}

// Backend is a simulated blockchain. You can use it to test your contracts or
// other code that interacts with the Ethereum chain.
type Backend struct {
	eth    *eth.Ethereum
	client simClient
	clock  *mockable.Clock
	server *rpc.Server
}

// NewBackend creates a new simulated blockchain that can be used as a backend for
// contract bindings in unit tests.
//
// A simulated backend always uses chainID 1337.
func NewBackend(alloc types.GenesisAlloc, options ...func(nodeConf *node.Config, ethConf *ethconfig.Config)) *Backend {
	chainConfig := *params.TestChainConfig
	chainConfig.ChainID = big.NewInt(1337)

	// Create the default configurations for the outer node shell and the Ethereum
	// service to mutate with the options afterwards
	nodeConf := node.DefaultConfig

	ethConf := ethconfig.DefaultConfig()
	ethConf.Genesis = &core.Genesis{
		Config: &chainConfig,
		Alloc:  alloc,
	}
	ethConf.AllowUnfinalizedQueries = true
	ethConf.Miner.Etherbase = constants.BlackholeAddr
	ethConf.Miner.TestOnlyAllowDuplicateBlocks = true
	ethConf.TxPool.NoLocals = true

	for _, option := range options {
		option(&nodeConf, &ethConf)
	}
	// Assemble the Ethereum stack to run the chain with
	stack, err := node.New(&nodeConf)
	if err != nil {
		panic(err) // this should never happen
	}
	sim, err := newWithNode(stack, &ethConf, 0)
	if err != nil {
		panic(err) // this should never happen
	}
	return sim
}

// newWithNode sets up a simulated backend on an existing node. The provided node
// must not be started and will be started by this method.
func newWithNode(stack *node.Node, conf *ethconfig.Config, blockPeriod uint64) (*Backend, error) {
	chaindb := rawdb.NewMemoryDatabase()
	clock := &mockable.Clock{}
	clock.Set(time.Unix(0, 0))

	engine := dummy.NewFakerWithModeAndClock(
		dummy.Mode{ModeSkipCoinbase: true}, clock,
	)

	backend, err := eth.New(
		stack, &eth.Config{
			Config: *conf,
			Genesis: conf.Genesis,
		}, &fakePushGossiper{}, chaindb, eth.Settings{}, common.Hash{},
		engine, clock,
	)
	if err != nil {
		return nil, err
	}
	server := rpc.NewServer(0 * time.Second)
	for _, api := range backend.APIs() {
		if err := server.RegisterName(api.Namespace, api.Service); err != nil {
			return nil, err
		}
	}
	return &Backend{
		eth:    backend,
		client: simClient{client: ethclient.NewClient(rpc.DialInProc(server))},
		clock:  clock,
		server: server,
	}, nil
}

// Close shuts down the simBackend.
// The simulated backend can't be used afterwards.
func (n *Backend) Close() error {
	if n.client.client != nil {
		n.client.client.Close()
	}
	n.server.Stop()
	return nil
}

// Commit seals a block and moves the chain forward to a new empty block.
func (n *Backend) Commit(accept bool) common.Hash {
	hash, err := n.buildBlock(accept, 10)
	if err != nil {
		panic(err)
	}
	return hash
}

func (n *Backend) buildBlock(accept bool, gap uint64) (common.Hash, error) {
	chain := n.eth.BlockChain()
	parent := chain.CurrentBlock()

	if err := n.eth.TxPool().Sync(); err != nil {
		return common.Hash{}, err
	}

	n.clock.Set(time.Unix(int64(parent.Time+gap), 0))
	block, err := n.eth.Miner().GenerateBlock(nil)
	if err != nil {
		return common.Hash{}, err
	}
	if err := chain.InsertBlock(block); err != nil {
		return common.Hash{}, err
	}
	if accept {
		if err := n.acceptAncestors(block); err != nil {
			return common.Hash{}, err
		}
		chain.DrainAcceptorQueue()
	}
	return block.Hash(), nil
}

func (n *Backend) acceptAncestors(block *types.Block) error {
	chain := n.eth.BlockChain()
	lastAccepted := chain.LastConsensusAcceptedBlock()

	// Accept all ancestors of the block
	toAccept := []*types.Block{block}
	for block.ParentHash() != lastAccepted.Hash() {
		block = chain.GetBlockByHash(block.ParentHash())
		toAccept = append(toAccept, block)
		if block.NumberU64() < lastAccepted.NumberU64() {
			return errors.New("last accepted must be an ancestor of the block to accept")
		}
	}

	for i := len(toAccept) - 1; i >= 0; i-- {
		if err := chain.Accept(toAccept[i]); err != nil {
			return err
		}
	}
	return nil
}

// Rollback removes all pending transactions, reverting to the last committed state.
func (n *Backend) Rollback() {
	// Flush all transactions from the transaction pools
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(common.Big1, 256), common.Big1)
	original := n.eth.TxPool().GasTip()
	n.eth.TxPool().SetGasTip(maxUint256)
	n.eth.TxPool().SetGasTip(original)
}

// Fork creates a side-chain that can be used to simulate reorgs.
//
// This function should be called with the ancestor block where the new side
// chain should be started. Transactions (old and new) can then be applied on
// top and Commit-ed.
//
// Note, the side-chain will only become canonical (and trigger the events) when
// it becomes longer. Until then CallContract will still operate on the current
// canonical chain.
//
// There is a % chance that the side chain becomes canonical at the same length
// to simulate live network behavior.
func (n *Backend) Fork(parentHash common.Hash) error {
	chain := n.eth.BlockChain()

	if chain.CurrentBlock().Hash() == parentHash {
		return nil
	}

	parent := chain.GetBlockByHash(parentHash)
	if parent == nil {
		return errors.New("parent block not found")
	}

	ch := make(chan core.NewTxPoolReorgEvent, 1)
	sub := n.eth.TxPool().SubscribeNewReorgEvent(ch)
	defer sub.Unsubscribe()

	if err := n.eth.BlockChain().SetPreference(parent); err != nil {
		return err
	}
	for {
		select {
		case reorg := <-ch:
			// Wait for tx pool to reorg, then flush the tx pool
			if reorg.Head.Hash() == parent.Hash() {
				n.Rollback()
				return nil
			}
		case <-time.After(2 * time.Second):
			return errors.New("fork not accepted")
		}
	}
}

// AdjustTime changes the block timestamp and creates a new block.
// It can only be called on empty blocks.
func (n *Backend) AdjustTime(adjustment time.Duration) error {
	_, err := n.buildBlock(false, uint64(adjustment))
	return err
}

// Client returns a client that accesses the simulated chain.
func (n *Backend) Client() Client {
	return n.client
}

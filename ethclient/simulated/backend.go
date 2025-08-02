// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.

package simulated

import (
	"context"
	"errors"
	"math/big"
	"time"

	"github.com/luxfi/evm/v2/v2/accounts/abi/bind"
	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/luxfi/evm/v2/v2/eth/ethconfig"
	"github.com/luxfi/evm/v2/v2/iface"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/rpc"
	"github.com/luxfi/geth/trie"
)

// Client exposes the methods provided by the Ethereum RPC client.
type Client interface {
	bind.ContractBackend
	Close()
	ChainID(context.Context) (*big.Int, error)
	BlockByNumber(context.Context, *big.Int) (*types.Block, error)
	BlockNumber(context.Context) (uint64, error)
	HeaderByHash(context.Context, common.Hash) (*types.Header, error)
	HeaderByNumber(context.Context, *big.Int) (*types.Header, error)
	TransactionReceipt(context.Context, common.Hash) (*types.Receipt, error)
	TransactionByHash(context.Context, common.Hash) (*types.Transaction, bool, error)
	BalanceAt(context.Context, common.Address, *big.Int) (*big.Int, error)
	NonceAt(context.Context, common.Address, *big.Int) (uint64, error)
	SuggestGasPrice(context.Context) (*big.Int, error)
	EstimateBaseFee(context.Context) (*big.Int, error)
}

// minimalClient implements a minimal client for testing
type minimalClient struct {
	blockNum uint64
}

// Implement all required methods with minimal functionality
func (m *minimalClient) Client() *rpc.Client { return nil }
func (m *minimalClient) Close() {}

func (m *minimalClient) ChainID(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1337), nil
}

func (m *minimalClient) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
	num := uint64(0)
	if number != nil && number.Sign() > 0 {
		num = number.Uint64()
	}
	header := &types.Header{
		Number:     big.NewInt(int64(num)),
		Time:       uint64(num * 10),
		Difficulty: big.NewInt(0),
		GasLimit:   8000000,
		BaseFee:    big.NewInt(7),
	}
	block := types.NewBlock(header, nil, nil, nil, trie.NewStackTrie(nil))
	return block, nil
}

func (m *minimalClient) BlockNumber(ctx context.Context) (uint64, error) {
	return m.blockNum, nil
}

func (m *minimalClient) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return &types.Header{
		Number:     number,
		Time:       0,
		Difficulty: big.NewInt(0),
		GasLimit:   8000000,
		BaseFee:    big.NewInt(7),
	}, nil
}

func (m *minimalClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return &types.Receipt{
		Status:      1,
		BlockNumber: big.NewInt(1),
	}, nil
}

func (m *minimalClient) SendTransaction(ctx context.Context, tx *iface.Transaction) error {
	return nil
}

func (m *minimalClient) TransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, bool, error) {
	return nil, false, errors.New("not implemented")
}

func (m *minimalClient) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
	return big.NewInt(10000000000000000), nil
}

func (m *minimalClient) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return 0, nil
}

func (m *minimalClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1000000000), nil
}

func (m *minimalClient) EstimateBaseFee(ctx context.Context) (*big.Int, error) {
	return big.NewInt(7), nil
}

// Implement bind.ContractBackend methods
func (m *minimalClient) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	return nil, nil
}

func (m *minimalClient) CallContract(ctx context.Context, call iface.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("not implemented")
}

func (m *minimalClient) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	return nil, errors.New("not implemented")
}

func (m *minimalClient) AcceptedCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return nil, nil
}

func (m *minimalClient) AcceptedNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, nil
}

func (m *minimalClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1000000000), nil
}

func (m *minimalClient) EstimateGas(ctx context.Context, call iface.CallMsg) (uint64, error) {
	return 21000, nil
}

func (m *minimalClient) FilterLogs(ctx context.Context, query iface.FilterQuery) ([]iface.Log, error) {
	return nil, errors.New("not implemented")
}

func (m *minimalClient) SubscribeFilterLogs(ctx context.Context, query iface.FilterQuery, ch chan<- iface.Log) (iface.Subscription, error) {
	return nil, errors.New("not implemented")
}

// Backend is a simulated blockchain for testing.
type Backend struct {
	client *minimalClient
}

// NewBackend creates a new simulated backend.
func NewBackend(alloc types.GenesisAlloc, options ...func(*ethconfig.Config)) *Backend {
	return &Backend{
		client: &minimalClient{blockNum: 0},
	}
}

// Client returns the underlying client.
func (b *Backend) Client() Client {
	return b.client
}

// Close shuts down the backend.
func (b *Backend) Close() error {
	return nil
}

// Commit seals a block and moves the chain forward.
func (b *Backend) Commit(accept bool) common.Hash {
	b.client.blockNum++
	return common.Hash{}
}

// Fork creates a side-chain for reorg testing.
func (b *Backend) Fork(parentHash common.Hash) error {
	return nil
}

// Rollback removes all pending transactions.
func (b *Backend) Rollback() {
}

// AdjustTime changes the block timestamp.
func (b *Backend) AdjustTime(adjustment time.Duration) error {
	return nil
}

// Verify interfaces
var (
	_ bind.ContractBackend = (*minimalClient)(nil)
	_ Client              = (*minimalClient)(nil)
)
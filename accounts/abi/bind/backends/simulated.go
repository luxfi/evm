// (c) 2020-2020, Lux Industries, Inc.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2015 The go-ethereum Authors
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

package backends

import (
	"context"
	"errors"
	"math/big"

	"github.com/luxfi/evm/accounts/abi/bind"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/evm/core/types"
	"github.com/luxfi/evm/iface"
)

// SimulatedBackend is a simulated blockchain.
// Deprecated: This is a stub implementation. Use a real backend for testing.
type SimulatedBackend struct {
	// Stub implementation
}

// Verify that SimulatedBackend implements required interfaces
var (
	_ bind.AcceptedContractCaller = (*SimulatedBackend)(nil)
	_ bind.ContractBackend        = (*SimulatedBackend)(nil)
	_ bind.DeployBackend          = (*SimulatedBackend)(nil)
)

// NewSimulatedBackend creates a new binding backend using a simulated blockchain
// for testing purposes.
//
// Deprecated: This is a stub implementation.
func NewSimulatedBackend(alloc types.GenesisAlloc, gasLimit uint64) *SimulatedBackend {
	return &SimulatedBackend{}
}

// CodeAt returns the code associated with a certain account in the blockchain.
func (b *SimulatedBackend) CodeAt(ctx context.Context, contract common.Address, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// CallContract executes a contract call.
func (b *SimulatedBackend) CallContract(ctx context.Context, call iface.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// PendingCodeAt returns the code associated with an account in the pending state.
func (b *SimulatedBackend) PendingCodeAt(ctx context.Context, account common.Address) ([]byte, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// PendingNonceAt retrieves the current pending nonce associated with an account.
func (b *SimulatedBackend) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, errors.New("simulated backend is deprecated")
}

// SuggestGasPrice retrieves the currently suggested gas price.
func (b *SimulatedBackend) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// SuggestGasTipCap retrieves the currently suggested gas tip cap.
func (b *SimulatedBackend) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// EstimateGas estimates the gas needed to execute a specific transaction.
func (b *SimulatedBackend) EstimateGas(ctx context.Context, call iface.CallMsg) (uint64, error) {
	return 0, errors.New("simulated backend is deprecated")
}

// SendTransaction injects a signed transaction into the pending pool for execution.
func (b *SimulatedBackend) SendTransaction(ctx context.Context, tx *iface.Transaction) error {
	return errors.New("simulated backend is deprecated")
}

// FilterLogs executes a log filter operation.
func (b *SimulatedBackend) FilterLogs(ctx context.Context, query iface.FilterQuery) ([]iface.Log, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// SubscribeFilterLogs creates a background log filtering operation.
func (b *SimulatedBackend) SubscribeFilterLogs(ctx context.Context, query iface.FilterQuery, ch chan<- iface.Log) (iface.Subscription, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// HeaderByNumber returns a block header from the blockchain.
func (b *SimulatedBackend) HeaderByNumber(ctx context.Context, number *big.Int) (*types.Header, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// AcceptedCodeAt returns the code associated with a certain account at the accepted state.
func (b *SimulatedBackend) AcceptedCodeAt(ctx context.Context, contract common.Address) ([]byte, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// AcceptedNonceAt retrieves the current accepted nonce associated with an account.
func (b *SimulatedBackend) AcceptedNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, errors.New("simulated backend is deprecated")
}

// AcceptedCallContract executes a contract call at the accepted state.
func (b *SimulatedBackend) AcceptedCallContract(ctx context.Context, call iface.CallMsg) ([]byte, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// Fork sets the head to a new block, which is based on the provided parentHash.
func (b *SimulatedBackend) Fork(ctx context.Context, parentHash common.Hash) error {
	return errors.New("simulated backend is deprecated")
}

// Commit mines a new block with the pending state.
func (b *SimulatedBackend) Commit() common.Hash {
	return common.Hash{}
}

// Rollback reverts all pending transactions.
func (b *SimulatedBackend) Rollback() {
	// No-op
}

// TransactionReceipt retrieves the receipt associated with a transaction.
func (b *SimulatedBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, errors.New("simulated backend is deprecated")
}

// NonceAt retrieves the current nonce associated with an account.
func (b *SimulatedBackend) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
	return 0, errors.New("simulated backend is deprecated")
}
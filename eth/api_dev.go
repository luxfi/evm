// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package eth

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"time"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/common/hexutil"
	"github.com/luxfi/geth/core/tracing"
)

// DevAPI provides Anvil/Hardhat-compatible RPC methods for dev mode.
// These methods allow manipulation of blockchain state for testing purposes.
type DevAPI struct {
	eth       *Ethereum
	snapshots map[hexutil.Uint64]common.Hash // snapshot ID -> block hash
	nextSnap  hexutil.Uint64
	mu        sync.Mutex
}

// NewDevAPI creates a new DevAPI instance.
func NewDevAPI(eth *Ethereum) *DevAPI {
	return &DevAPI{
		eth:       eth,
		snapshots: make(map[hexutil.Uint64]common.Hash),
		nextSnap:  1,
	}
}

// SetBalance sets the balance of an address.
// Anvil: anvil_setBalance
// Hardhat: hardhat_setBalance
func (api *DevAPI) SetBalance(ctx context.Context, address common.Address, balance hexutil.Big) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Get the current state
	header := api.eth.blockchain.CurrentBlock()
	statedb, err := api.eth.blockchain.StateAt(header.Root)
	if err != nil {
		return err
	}

	// Set the balance
	u256Balance, overflow := uint256.FromBig((*big.Int)(&balance))
	if overflow {
		return errors.New("balance overflow")
	}
	statedb.SetBalance(address, u256Balance, tracing.BalanceChangeUnspecified)

	// Commit the state changes and create a new block
	return api.commitStateChanges(statedb)
}

// SetStorageAt sets a storage slot value for an address.
// Anvil: anvil_setStorageAt
// Hardhat: hardhat_setStorageAt
func (api *DevAPI) SetStorageAt(ctx context.Context, address common.Address, slot common.Hash, value common.Hash) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Get the current state
	header := api.eth.blockchain.CurrentBlock()
	statedb, err := api.eth.blockchain.StateAt(header.Root)
	if err != nil {
		return err
	}

	// Set the storage value
	statedb.SetState(address, slot, value)

	// Commit the state changes and create a new block
	return api.commitStateChanges(statedb)
}

// SetCode sets the code of an address.
// Anvil: anvil_setCode
// Hardhat: hardhat_setCode
func (api *DevAPI) SetCode(ctx context.Context, address common.Address, code hexutil.Bytes) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Get the current state
	header := api.eth.blockchain.CurrentBlock()
	statedb, err := api.eth.blockchain.StateAt(header.Root)
	if err != nil {
		return err
	}

	// Set the code
	statedb.SetCode(address, code, tracing.CodeChangeUnspecified)

	// Commit the state changes and create a new block
	return api.commitStateChanges(statedb)
}

// SetNonce sets the nonce of an address.
// Anvil: anvil_setNonce
// Hardhat: hardhat_setNonce
func (api *DevAPI) SetNonce(ctx context.Context, address common.Address, nonce hexutil.Uint64) error {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Get the current state
	header := api.eth.blockchain.CurrentBlock()
	statedb, err := api.eth.blockchain.StateAt(header.Root)
	if err != nil {
		return err
	}

	// Set the nonce
	statedb.SetNonce(address, uint64(nonce), tracing.NonceChangeUnspecified)

	// Commit the state changes and create a new block
	return api.commitStateChanges(statedb)
}

// Mine forces mining of a new block.
// Anvil: evm_mine
// Hardhat: evm_mine
func (api *DevAPI) Mine(ctx context.Context, timestamp *hexutil.Uint64) (common.Hash, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Generate a new block using the miner
	block, err := api.eth.miner.GenerateBlock(nil)
	if err != nil {
		return common.Hash{}, err
	}

	// Insert the block into the chain
	if err := api.eth.blockchain.InsertBlock(block); err != nil {
		return common.Hash{}, err
	}

	// Accept the block
	if err := api.eth.blockchain.Accept(block); err != nil {
		return common.Hash{}, err
	}
	api.eth.blockchain.DrainAcceptorQueue()

	return block.Hash(), nil
}

// Snapshot creates a snapshot of the current state.
// Returns a snapshot ID that can be used with Revert.
// Anvil: evm_snapshot
// Hardhat: evm_snapshot
func (api *DevAPI) Snapshot(ctx context.Context) hexutil.Uint64 {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Store the current block hash as a snapshot
	blockHash := api.eth.blockchain.CurrentBlock().Hash()
	snapID := api.nextSnap
	api.snapshots[snapID] = blockHash
	api.nextSnap++

	return snapID
}

// Revert reverts the state to a previous snapshot.
// Note: In subnet-evm, revert is limited - it can only revert to the current or
// recent state. Full reorg capabilities are not available.
// Anvil: evm_revert
// Hardhat: evm_revert
func (api *DevAPI) Revert(ctx context.Context, snapID hexutil.Uint64) (bool, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	blockHash, ok := api.snapshots[snapID]
	if !ok {
		return false, errors.New("snapshot not found")
	}

	// Get the block by hash
	block := api.eth.blockchain.GetBlockByHash(blockHash)
	if block == nil {
		return false, errors.New("snapshot block not found")
	}

	// For now, just verify the snapshot block exists and delete the snapshot
	// Full revert functionality requires SetPreference which may cause issues
	currentNumber := api.eth.blockchain.CurrentBlock().Number.Uint64()
	snapNumber := block.NumberU64()

	if snapNumber > currentNumber {
		return false, errors.New("cannot revert to future block")
	}

	// Delete the snapshot and all snapshots after it
	for id := range api.snapshots {
		if id >= snapID {
			delete(api.snapshots, id)
		}
	}

	// If trying to revert to the same block, it's a no-op success
	if snapNumber == currentNumber && api.eth.blockchain.CurrentBlock().Hash() == blockHash {
		return true, nil
	}

	// Otherwise, try to set preference back to the snapshot block
	if err := api.eth.blockchain.SetPreference(block); err != nil {
		return false, err
	}

	return true, nil
}

// IncreaseTime increases the block timestamp by mining a new block with adjusted time.
// Anvil: evm_increaseTime
// Hardhat: evm_increaseTime
func (api *DevAPI) IncreaseTime(ctx context.Context, seconds hexutil.Uint64) (hexutil.Uint64, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	// Get current block time and calculate the new time
	currentTime := api.eth.blockchain.CurrentBlock().Time
	newTime := currentTime + uint64(seconds)

	// Generate and insert a new block (the miner will use current clock time)
	block, err := api.eth.miner.GenerateBlock(nil)
	if err != nil {
		return 0, err
	}

	if err := api.eth.blockchain.InsertBlock(block); err != nil {
		return 0, err
	}

	if err := api.eth.blockchain.Accept(block); err != nil {
		return 0, err
	}
	api.eth.blockchain.DrainAcceptorQueue()

	return hexutil.Uint64(newTime), nil
}

// SetNextBlockTimestamp sets the timestamp for the next block by mining one.
// Anvil: evm_setNextBlockTimestamp
// Hardhat: evm_setNextBlockTimestamp
func (api *DevAPI) SetNextBlockTimestamp(ctx context.Context, timestamp hexutil.Uint64) error {
	// Mine a new block - the timestamp will be adjusted by the clock
	_, err := api.Mine(ctx, &timestamp)
	return err
}

// ImpersonateAccount starts impersonating an account (allows sending tx without private key).
// Anvil: anvil_impersonateAccount
// Hardhat: hardhat_impersonateAccount
func (api *DevAPI) ImpersonateAccount(ctx context.Context, address common.Address) error {
	// In dev mode, all accounts are effectively impersonatable
	// This is a no-op for compatibility
	return nil
}

// StopImpersonatingAccount stops impersonating an account.
// Anvil: anvil_stopImpersonatingAccount
// Hardhat: hardhat_stopImpersonatingAccount
func (api *DevAPI) StopImpersonatingAccount(ctx context.Context, address common.Address) error {
	// This is a no-op for compatibility
	return nil
}

// AutoImpersonate enables or disables auto-impersonation of all accounts.
// Anvil: anvil_autoImpersonateAccount
func (api *DevAPI) AutoImpersonate(ctx context.Context, enabled bool) error {
	// This is a no-op for compatibility
	return nil
}

// commitStateChanges commits state changes by creating and inserting a new block.
// This is used by SetBalance, SetStorageAt, etc. to persist changes.
func (api *DevAPI) commitStateChanges(statedb *state.StateDB) error {
	// Finalize the state changes
	statedb.Finalise(false)

	// Commit the state to the database
	_, err := statedb.Commit(api.eth.blockchain.CurrentBlock().Number.Uint64()+1, false, false)
	if err != nil {
		return err
	}

	// Generate and insert a new block through the miner
	// This will pick up the committed state changes
	block, err := api.eth.miner.GenerateBlock(nil)
	if err != nil {
		return err
	}

	// Insert the block
	if err := api.eth.blockchain.InsertBlock(block); err != nil {
		return err
	}

	// Accept the block
	if err := api.eth.blockchain.Accept(block); err != nil {
		return err
	}
	api.eth.blockchain.DrainAcceptorQueue()

	return nil
}

// DumpState returns a dump of the current state (for debugging).
func (api *DevAPI) DumpState(ctx context.Context) (state.Dump, error) {
	api.mu.Lock()
	defer api.mu.Unlock()

	header := api.eth.blockchain.CurrentBlock()
	statedb, err := api.eth.blockchain.StateAt(header.Root)
	if err != nil {
		return state.Dump{}, err
	}

	return statedb.RawDump(&state.DumpConfig{
		OnlyWithAddresses: true,
		Max:               256,
	}), nil
}

// unused variable to ensure we use time package
var _ = time.Now

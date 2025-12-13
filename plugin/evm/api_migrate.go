// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"fmt"

	"github.com/luxfi/migrate/rpcapi"
)

// MigrateAPI provides RPC methods for blockchain migration.
// This is the unified API for importing blocks into SubnetEVM with full execution.
// It uses the shared migrate/rpcapi package for consistent behavior with C-Chain.
type MigrateAPI struct {
	vm *VM
}

// NewMigrateAPI creates a new MigrateAPI instance
func NewMigrateAPI(vm *VM) *MigrateAPI {
	return &MigrateAPI{vm: vm}
}

// vmLogAdapter adapts the VM logger to the rpcapi.Logger interface
type vmLogAdapter struct {
	vm *VM
}

func (l vmLogAdapter) Info(msg string, ctx ...interface{}) {
	l.vm.logger.Info(msg, ctx...)
}

// ImportBlocks imports blocks and executes all transactions with full consensus.
// This is THE method for blockchain migration - it uses InsertChain to properly:
// 1. Validate each block
// 2. Execute all transactions via StateProcessor.Process()
// 3. Commit state changes to the database
// 4. Update canonical chain pointers
func (api *MigrateAPI) ImportBlocks(blocks []rpcapi.BlockEntry) (*rpcapi.ImportBlocksReply, error) {
	reply := &rpcapi.ImportBlocksReply{}
	err := rpcapi.ImportBlocks(api.vm.blockChain, vmLogAdapter{vm: api.vm}, blocks, reply)
	return reply, err
}

// GetChainInfo returns basic chain information for debugging
func (api *MigrateAPI) GetChainInfo() (map[string]interface{}, error) {
	head := api.vm.blockChain.CurrentBlock()
	if head == nil {
		return nil, fmt.Errorf("no current block")
	}

	return map[string]interface{}{
		"chainId":       api.vm.chainConfig.ChainID.Uint64(),
		"networkId":     api.vm.chainCtx.NetworkID,
		"genesisHash":   api.vm.genesisHash.Hex(),
		"currentHeight": head.Number.Uint64(),
		"currentHash":   head.Hash().Hex(),
		"stateRoot":     head.Root.Hex(),
		"vmVersion":     Version,
	}, nil
}

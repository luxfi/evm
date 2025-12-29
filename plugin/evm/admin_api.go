// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/eth"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/luxfi/log"
	"github.com/luxfi/utils/profiler"
)

// AdminAPI provides admin-level RPC methods using geth's RPC server
// This enables underscore notation: admin_importChain, admin_exportChain, etc.
type AdminAPI struct {
	vm       *VM
	profiler profiler.Profiler
}

// NewAdminAPI creates a new AdminAPI instance for geth RPC server
func NewAdminAPI(vm *VM, performanceDir string) *AdminAPI {
	return &AdminAPI{
		vm:       vm,
		profiler: profiler.New(performanceDir),
	}
}

// ImportChainResult represents the response from admin_importChain
type ImportChainResult struct {
	Success        bool   `json:"success"`
	BlocksImported int    `json:"blocksImported,omitempty"`
	HeightBefore   uint64 `json:"heightBefore,omitempty"`
	HeightAfter    uint64 `json:"heightAfter,omitempty"`
	Message        string `json:"message,omitempty"`
}

// ImportChain imports a blockchain from a local file
// RPC: admin_importChain
func (api *AdminAPI) ImportChain(ctx context.Context, file string) (*ImportChainResult, error) {
	log.Info("admin_importChain called", "file", file)

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	if api.vm.eth == nil {
		return nil, fmt.Errorf("ethereum backend not initialized")
	}

	chain := api.vm.eth.BlockChain()
	if chain == nil {
		return nil, fmt.Errorf("blockchain not initialized")
	}

	// Get chain state before import
	currentBlock := chain.CurrentBlock()
	genesisHash := chain.Genesis().Hash().Hex()
	beforeNum := currentBlock.Number.Uint64()

	// Import the chain from file
	totalImported, err := importBlocksFromFile(chain, api.vm.eth, file)
	if err != nil {
		return nil, fmt.Errorf("import failed after %d blocks: %w", totalImported, err)
	}

	// Get chain state after import
	afterBlock := chain.CurrentBlock()
	afterNum := afterBlock.Number.Uint64()

	// Check if any blocks were actually imported
	if totalImported == 0 {
		return nil, fmt.Errorf("no blocks imported (before=%d, after=%d, genesis=%s)", beforeNum, afterNum, genesisHash)
	}

	if afterNum == beforeNum {
		return nil, fmt.Errorf("block height unchanged after import: %d blocks parsed but height still %d", totalImported, afterNum)
	}

	result := &ImportChainResult{
		Success:        true,
		BlocksImported: totalImported,
		HeightBefore:   beforeNum,
		HeightAfter:    afterNum,
		Message:        fmt.Sprintf("imported %d blocks, height %d -> %d", totalImported, beforeNum, afterNum),
	}
	log.Info("admin_importChain: completed", "imported", totalImported, "height", afterNum)
	return result, nil
}

// ExportChain exports a blockchain to a local file
// RPC: admin_exportChain
func (api *AdminAPI) ExportChain(ctx context.Context, file string, first, last uint64) (bool, error) {
	log.Info("admin_exportChain called", "file", file, "first", first, "last", last)

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	if api.vm.eth == nil {
		return false, fmt.Errorf("ethereum backend not initialized")
	}

	chain := api.vm.eth.BlockChain()
	if chain == nil {
		return false, fmt.Errorf("blockchain not initialized")
	}

	// Open output file
	out, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false, fmt.Errorf("failed to open output file: %w", err)
	}
	defer out.Close()

	// Use gzip if file ends with .gz
	var writer io.Writer = out
	if strings.HasSuffix(file, ".gz") {
		gzWriter := gzip.NewWriter(out)
		defer gzWriter.Close()
		writer = gzWriter
	}

	// Export blocks
	current := chain.CurrentBlock().Number.Uint64()
	if last > current {
		last = current
	}

	exported := 0
	for i := first; i <= last; i++ {
		block := chain.GetBlockByNumber(i)
		if block == nil {
			return false, fmt.Errorf("block %d not found", i)
		}
		if err := rlp.Encode(writer, block); err != nil {
			return false, fmt.Errorf("failed to encode block %d: %w", i, err)
		}
		exported++
	}

	log.Info("admin_exportChain: completed", "blocks", exported, "first", first, "last", last)
	return true, nil
}

// StartCPUProfiler starts a cpu profile writing to the specified file
// RPC: admin_startCPUProfiler
func (api *AdminAPI) StartCPUProfiler(ctx context.Context) error {
	log.Info("admin_startCPUProfiler called")

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	return api.profiler.StartCPUProfiler()
}

// StopCPUProfiler stops the cpu profile
// RPC: admin_stopCPUProfiler
func (api *AdminAPI) StopCPUProfiler(ctx context.Context) error {
	log.Info("admin_stopCPUProfiler called")

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	return api.profiler.StopCPUProfiler()
}

// MemoryProfile runs a memory profile writing to the specified file
// RPC: admin_memoryProfile
func (api *AdminAPI) MemoryProfile(ctx context.Context) error {
	log.Info("admin_memoryProfile called")

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	return api.profiler.MemoryProfile()
}

// LockProfile runs a mutex profile writing to the specified file
// RPC: admin_lockProfile
func (api *AdminAPI) LockProfile(ctx context.Context) error {
	log.Info("admin_lockProfile called")

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	return api.profiler.LockProfile()
}

// SetLogLevel sets the log level
// RPC: admin_setLogLevel
func (api *AdminAPI) SetLogLevel(ctx context.Context, level string) error {
	log.Info("admin_setLogLevel called", "level", level)

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	if err := api.vm.logger.SetLogLevel(level); err != nil {
		return fmt.Errorf("failed to parse log level: %w", err)
	}
	return nil
}

// GetVMConfig returns the VM configuration
// RPC: admin_getVMConfig
func (api *AdminAPI) GetVMConfig(ctx context.Context) (interface{}, error) {
	return &api.vm.config, nil
}

// importBlocksFromFile imports blocks from an RLP-encoded file
func importBlocksFromFile(chain *core.BlockChain, eth *eth.Ethereum, file string) (int, error) {
	in, err := os.Open(file)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(file, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return 0, fmt.Errorf("failed to create gzip reader: %w", err)
		}
	}

	stream := rlp.NewStream(reader, 0)
	blocks := make([]*types.Block, 0, 2500)
	totalParsed := 0
	totalImported := 0
	skippedGenesis := false

	for batch := 0; ; batch++ {
		// Load a batch of blocks from the input file
		for len(blocks) < cap(blocks) {
			block := new(types.Block)
			if err := stream.Decode(block); err == io.EOF {
				break
			} else if err != nil {
				return totalImported, fmt.Errorf("block %d: failed to parse: %w", totalParsed, err)
			}
			// ignore the genesis block when importing blocks
			if block.NumberU64() == 0 {
				skippedGenesis = true
				continue
			}
			blocks = append(blocks, block)
			totalParsed++
		}

		if len(blocks) == 0 {
			if totalParsed == 0 && !skippedGenesis {
				return 0, fmt.Errorf("no blocks found in file (possibly wrong format or empty)")
			}
			break
		}

		// Get first block info for error messages
		firstBlock := blocks[0]
		firstNum := firstBlock.NumberU64()
		parentHash := firstBlock.ParentHash().Hex()

		// Check parent exists (for first block in batch)
		if firstNum > 0 && !chain.HasBlock(firstBlock.ParentHash(), firstNum-1) {
			// Check if parent is genesis
			genesisHash := chain.Genesis().Hash()
			if firstNum == 1 && firstBlock.ParentHash() != genesisHash {
				return totalImported, fmt.Errorf("batch %d: block 1 parent mismatch - expected genesis %s, got %s",
					batch, genesisHash.Hex(), parentHash)
			}
			return totalImported, fmt.Errorf("batch %d: parent block missing - firstBlock=%d, parentHash=%s, hasGenesis=%v, skippedGenesis=%v, totalParsed=%d",
				batch, firstNum, parentHash, chain.HasBlock(genesisHash, 0), skippedGenesis, totalParsed)
		}

		// Import the batch
		log.Info("admin_importChain: inserting batch", "batch", batch, "blocks", len(blocks), "firstNum", firstNum)
		n, err := chain.InsertChain(blocks)
		if err != nil {
			return totalImported, fmt.Errorf("batch %d: insert failed after %d of %d blocks - firstBlock=%d, parentHash=%s, error=%w",
				batch, n, len(blocks), firstNum, parentHash, err)
		}
		log.Info("admin_importChain: InsertChain completed", "batch", batch, "inserted", n)

		// Set the last accepted block directly to make blocks queryable via RPC.
		lastInsertedBlock := blocks[n-1]
		if err := chain.SetLastAcceptedBlockDirect(lastInsertedBlock); err != nil {
			return totalImported, fmt.Errorf("batch %d: failed to set last accepted block: %w", batch, err)
		}

		// CRITICAL: Call PostImportCallback to update acceptedBlockDB for persistence across restarts.
		// Without this, the VM's lastAcceptedKey won't be updated, causing blocks to be lost on restart.
		if err := eth.CallPostImportCallback(lastInsertedBlock.Hash(), lastInsertedBlock.NumberU64()); err != nil {
			return totalImported, fmt.Errorf("batch %d: failed to update acceptedBlockDB: %w", batch, err)
		}
		log.Info("admin_importChain: blocks finalized", "batch", batch, "count", n, "lastAccepted", lastInsertedBlock.NumberU64())

		totalImported += n
		blocks = blocks[:0]
	}

	if totalImported == 0 {
		return 0, fmt.Errorf("no blocks imported (parsed=%d)", totalParsed)
	}

	return totalImported, nil
}

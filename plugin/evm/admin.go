// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/luxfi/evm/api"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/plugin/evm/client"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/utils/profiler"
)

// Admin is the API service for admin API calls
type Admin struct {
	vm       *VM
	profiler profiler.Profiler
}

func NewAdminService(vm *VM, performanceDir string) *Admin {
	return &Admin{
		vm:       vm,
		profiler: profiler.New(performanceDir),
	}
}

// StartCPUProfiler starts a cpu profile writing to the specified file
func (p *Admin) StartCPUProfiler(_ *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: StartCPUProfiler called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.StartCPUProfiler()
}

// StopCPUProfiler stops the cpu profile
func (p *Admin) StopCPUProfiler(r *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: StopCPUProfiler called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.StopCPUProfiler()
}

// MemoryProfile runs a memory profile writing to the specified file
func (p *Admin) MemoryProfile(_ *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: MemoryProfile called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.MemoryProfile()
}

// LockProfile runs a mutex profile writing to the specified file
func (p *Admin) LockProfile(_ *http.Request, _ *struct{}, _ *api.EmptyReply) error {
	log.Info("Admin: LockProfile called")

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	return p.profiler.LockProfile()
}

func (p *Admin) SetLogLevel(_ *http.Request, args *client.SetLogLevelArgs, reply *api.EmptyReply) error {
	log.Info("EVM: SetLogLevel called", "logLevel", args.Level)

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	if err := p.vm.logger.SetLogLevel(args.Level); err != nil {
		return fmt.Errorf("failed to parse log level: %w ", err)
	}
	return nil
}

func (p *Admin) GetVMConfig(_ *http.Request, _ *struct{}, reply *client.ConfigReply) error {
	reply.Config = &p.vm.config
	return nil
}

// ImportChainArgs represents the arguments for ImportChain
type ImportChainArgs struct {
	File string `json:"file"`
}

// ImportChainReply represents the response from ImportChain
type ImportChainReply struct {
	Success       bool   `json:"success"`
	BlocksImported int    `json:"blocksImported,omitempty"`
	HeightBefore  uint64 `json:"heightBefore,omitempty"`
	HeightAfter   uint64 `json:"heightAfter,omitempty"`
	Message       string `json:"message,omitempty"`
}

// ImportChain imports a blockchain from a local file
func (p *Admin) ImportChain(_ *http.Request, args *ImportChainArgs, reply *ImportChainReply) error {
	log.Info("ImportChain called", "file", args.File)

	p.vm.vmLock.Lock()
	defer p.vm.vmLock.Unlock()

	if p.vm.eth == nil {
		return fmt.Errorf("ethereum backend not initialized")
	}

	chain := p.vm.eth.BlockChain()
	if chain == nil {
		return fmt.Errorf("blockchain not initialized")
	}

	// Ensure genesis state is accessible before import
	if err := chain.EnsureGenesisState(); err != nil {
		log.Error("ImportChain: failed to ensure genesis state", "error", err)
		return fmt.Errorf("failed to ensure genesis state: %w", err)
	}

	// Get chain state before import
	currentBlock := chain.CurrentBlock()
	genesisHash := chain.Genesis().Hash().Hex()
	beforeNum := currentBlock.Number.Uint64()
	log.Info("ImportChain: starting import", "file", args.File, "currentHeight", beforeNum, "genesis", genesisHash)

	// Import the chain from file - returns total imported and last block hash
	totalImported, lastBlockHash, err := importBlocksFromFileWithCount(chain, args.File)
	if err != nil {
		return fmt.Errorf("import failed after %d blocks: %w", totalImported, err)
	}

	// Get chain state after import
	afterBlock := chain.CurrentBlock()
	afterNum := afterBlock.Number.Uint64()

	// Check if any blocks were actually imported
	if totalImported == 0 {
		return fmt.Errorf("no blocks imported (before=%d, after=%d, genesis=%s)", beforeNum, afterNum, genesisHash)
	}

	if afterNum == beforeNum {
		return fmt.Errorf("block height unchanged after import: %d blocks parsed but height still %d", totalImported, afterNum)
	}

	// Get the last imported block for state commitment
	lastBlock := chain.GetBlockByHash(lastBlockHash)
	if lastBlock == nil {
		return fmt.Errorf("failed to get last imported block by hash %s", lastBlockHash.Hex())
	}

	// CRITICAL FIX: Commit the imported state to disk
	// Without this, the state trie is only in memory and will be lost on shutdown.
	log.Info("ImportChain: committing imported state", "block", lastBlock.NumberU64(), "root", lastBlock.Root())
	if err := chain.AcceptImportedState(lastBlock); err != nil {
		return fmt.Errorf("failed to commit imported state: %w", err)
	}
	log.Info("ImportChain: state committed successfully")

	// CRITICAL FIX: Update the blockchain's canonical head pointers
	// This writes HeadBlockHash and HeadHeaderHash to rawdb, which is read on restart.
	log.Info("ImportChain: setting last accepted block in blockchain", "block", lastBlock.NumberU64(), "hash", lastBlock.Hash().Hex())
	if err := chain.SetLastAcceptedBlockDirect(lastBlock); err != nil {
		return fmt.Errorf("failed to set last accepted block: %w", err)
	}
	log.Info("ImportChain: blockchain head updated successfully")

	// CRITICAL FIX: Update the VM layer's acceptedBlockDB
	// Without this, ReadLastAccepted() returns genesis hash on restart because
	// acceptedBlockDB is not updated by the chain-level import path.
	blkID := ids.ID(lastBlockHash)
	log.Info("ImportChain: updating acceptedBlockDB", "hash", lastBlockHash.Hex(), "height", lastBlock.NumberU64())

	// Abort any pending versiondb changes before we modify it
	p.vm.versiondb.Abort()

	if err := p.vm.acceptedBlockDB.Put(lastAcceptedKey, blkID[:]); err != nil {
		return fmt.Errorf("failed to put last accepted block: %w", err)
	}

	// Commit the versiondb to persist the acceptedBlockDB change
	if err := p.vm.versiondb.Commit(); err != nil {
		return fmt.Errorf("failed to commit versiondb: %w", err)
	}

	// CRITICAL: Force sync to disk to ensure persistence across restarts.
	// Without this, async writes may be lost if network stops before flush.
	if err := p.vm.versiondb.Sync(); err != nil {
		log.Warn("ImportChain: sync failed (non-fatal)", "error", err)
		// Don't return error - the commit succeeded, sync is best-effort
	}

	log.Info("ImportChain: acceptedBlockDB updated and persisted successfully")

	reply.Success = true
	reply.BlocksImported = totalImported
	reply.HeightBefore = beforeNum
	reply.HeightAfter = afterNum
	reply.Message = fmt.Sprintf("imported %d blocks, height %d -> %d (persisted)", totalImported, beforeNum, afterNum)
	log.Info("ImportChain: completed with persistence", "imported", totalImported, "height", afterNum, "lastHash", lastBlockHash.Hex())
	return nil
}

// importBlocksFromFileWithCount imports blocks from an RLP-encoded file and returns the count and last block hash
func importBlocksFromFileWithCount(chain *core.BlockChain, file string) (int, common.Hash, error) {
	in, err := os.Open(file)
	if err != nil {
		return 0, common.Hash{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(file, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return 0, common.Hash{}, fmt.Errorf("failed to create gzip reader: %w", err)
		}
	}

	stream := rlp.NewStream(reader, 0)
	blocks := make([]*types.Block, 0, 2500)
	totalParsed := 0
	totalImported := 0
	skippedGenesis := false
	var lastBlockHash common.Hash

	for batch := 0; ; batch++ {
		// Load a batch of blocks from the input file
		for len(blocks) < cap(blocks) {
			block := new(types.Block)
			if err := stream.Decode(block); err == io.EOF {
				break
			} else if err != nil {
				return totalImported, lastBlockHash, fmt.Errorf("block %d: failed to parse: %w", totalParsed, err)
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
				return 0, common.Hash{}, fmt.Errorf("no blocks found in file (possibly wrong format or empty)")
			}
			break
		}

		// Get first block info for error messages
		firstBlock := blocks[0]
		firstNum := firstBlock.NumberU64()
		parentHash := firstBlock.ParentHash().Hex()

		// Skip check for existing blocks - let InsertChain handle it
		// This ensures we update canonical chain even if blocks exist

		// Check parent exists (for first block in batch)
		if firstNum > 0 && !chain.HasBlock(firstBlock.ParentHash(), firstNum-1) {
			// Check if parent is genesis
			genesisHash := chain.Genesis().Hash()
			if firstNum == 1 && firstBlock.ParentHash() != genesisHash {
				return totalImported, lastBlockHash, fmt.Errorf("batch %d: block 1 parent mismatch - expected genesis %s, got %s",
					batch, genesisHash.Hex(), parentHash)
			}
			return totalImported, lastBlockHash, fmt.Errorf("batch %d: parent block missing - firstBlock=%d, parentHash=%s, hasGenesis=%v, skippedGenesis=%v, totalParsed=%d",
				batch, firstNum, parentHash, chain.HasBlock(genesisHash, 0), skippedGenesis, totalParsed)
		}

		// Import the batch
		log.Info("ImportChain: inserting batch", "batch", batch, "blocks", len(blocks), "firstNum", firstNum)
		n, err := chain.InsertChain(blocks)
		if err != nil {
			return totalImported, lastBlockHash, fmt.Errorf("batch %d: insert failed after %d of %d blocks - firstBlock=%d, parentHash=%s, error=%w",
				batch, n, len(blocks), firstNum, parentHash, err)
		}
		log.Info("ImportChain: InsertChain completed", "batch", batch, "inserted", n)

		// Set the last accepted block directly to make blocks queryable via RPC.
		// This bypasses the acceptor queue which can cause issues with imported blocks.
		// The lastAccepted is used as the finalization boundary for RPC queries.
		lastInsertedBlock := blocks[n-1]
		if err := chain.SetLastAcceptedBlockDirect(lastInsertedBlock); err != nil {
			return totalImported, lastBlockHash, fmt.Errorf("batch %d: failed to set last accepted block: %w", batch, err)
		}
		log.Info("ImportChain: blocks finalized", "batch", batch, "count", n, "lastAccepted", lastInsertedBlock.NumberU64())

		// Track the last imported block hash for persistence
		lastBlockHash = lastInsertedBlock.Hash()
		totalImported += n
		blocks = blocks[:0]
	}

	if totalImported == 0 {
		return 0, common.Hash{}, fmt.Errorf("no blocks imported (parsed=%d)", totalParsed)
	}

	return totalImported, lastBlockHash, nil
}

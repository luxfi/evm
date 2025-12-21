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
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
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

	// Get chain state before import
	currentBlock := chain.CurrentBlock()
	genesisHash := chain.Genesis().Hash().Hex()
	beforeNum := currentBlock.Number.Uint64()

	// Import the chain from file
	totalImported, err := importBlocksFromFileWithCount(chain, args.File)
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

	reply.Success = true
	reply.BlocksImported = totalImported
	reply.HeightBefore = beforeNum
	reply.HeightAfter = afterNum
	reply.Message = fmt.Sprintf("imported %d blocks, height %d -> %d", totalImported, beforeNum, afterNum)
	log.Info("ImportChain: completed", "imported", totalImported, "height", afterNum)
	return nil
}

// importBlocksFromFileWithCount imports blocks from an RLP-encoded file and returns the count
func importBlocksFromFileWithCount(chain *core.BlockChain, file string) (int, error) {
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

		// Skip check for existing blocks - let InsertChain handle it
		// This ensures we update canonical chain even if blocks exist

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
		log.Info("ImportChain: inserting batch", "batch", batch, "blocks", len(blocks), "firstNum", firstNum)
		n, err := chain.InsertChain(blocks)
		if err != nil {
			return totalImported, fmt.Errorf("batch %d: insert failed after %d of %d blocks - firstBlock=%d, parentHash=%s, error=%w",
				batch, n, len(blocks), firstNum, parentHash, err)
		}
		log.Info("ImportChain: InsertChain completed", "batch", batch, "inserted", n)

		// Set the last accepted block directly to make blocks queryable via RPC.
		// This bypasses the acceptor queue which can cause issues with imported blocks.
		// The lastAccepted is used as the finalization boundary for RPC queries.
		lastInsertedBlock := blocks[n-1]
		if err := chain.SetLastAcceptedBlockDirect(lastInsertedBlock); err != nil {
			return totalImported, fmt.Errorf("batch %d: failed to set last accepted block: %w", batch, err)
		}
		log.Info("ImportChain: blocks finalized", "batch", batch, "count", n, "lastAccepted", lastInsertedBlock.NumberU64())

		totalImported += n
		blocks = blocks[:0]
	}

	if totalImported == 0 {
		return 0, fmt.Errorf("no blocks imported (parsed=%d)", totalParsed)
	}

	return totalImported, nil
}

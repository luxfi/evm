// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/luxfi/evm/core"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/rlp"
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
	"github.com/luxfi/metric/profiler"
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

// ImportChain imports a blockchain from a local file.
// State is committed periodically during import for restart-safety.
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

	beforeNum := chain.CurrentBlock().Number.Uint64()

	// Import blocks with periodic state commits. acceptedBlockDB is updated
	// atomically with each state commit so there's no crash window where
	// state is persisted but acceptedBlockDB is stale.
	persistAccepted := func(hash common.Hash, height uint64) error {
		var blkID ids.ID
		copy(blkID[:], hash[:])
		if err := api.vm.acceptedBlockDB.Put(lastAcceptedKey, blkID[:]); err != nil {
			return fmt.Errorf("failed to update acceptedBlockDB: %w", err)
		}
		if err := api.vm.versiondb.Commit(); err != nil {
			return fmt.Errorf("failed to commit versiondb: %w", err)
		}
		return nil
	}

	totalImported, lastHash, lastHeight, err := importBlocksFromFile(chain, file, persistAccepted)
	if err != nil {
		return nil, fmt.Errorf("import failed: %w", err)
	}
	log.Info("admin_importChain: complete", "hash", lastHash.Hex(), "height", lastHeight)

	return &ImportChainResult{
		Success:        true,
		BlocksImported: totalImported,
		HeightBefore:   beforeNum,
		HeightAfter:    lastHeight,
		Message:        fmt.Sprintf("imported %d blocks, height %d -> %d", totalImported, beforeNum, lastHeight),
	}, nil
}

// ExportChainResult is the response shape from admin_exportChain.
//
// FirstHash + LastHash let the client verify a previously-written export
// matches the same chain state (idempotency check) and let the operator
// store a content-addressable identifier alongside the file.
//
// Status is one of: "ok" (exported [first..last] inclusive), "noop"
// (file already existed with matching hashes), "interrupted" (ctx canceled
// before [last] reached — Resume reads sentinel and picks up at HighestExported+1).
type ExportChainResult struct {
	Success         bool        `json:"success"`
	Status          string      `json:"status"`
	BlocksExported  uint64      `json:"blocksExported"`
	First           uint64      `json:"first"`
	Last            uint64      `json:"last"`
	HighestExported uint64      `json:"highestExported"`
	FirstHash       common.Hash `json:"firstHash"`
	LastHash        common.Hash `json:"lastHash"`
	SentinelPath    string      `json:"sentinelPath"`
	Message         string      `json:"message,omitempty"`
}

// exportSentinel is the JSON written to <outputFile>.sentinel after every
// commit-checkpoint and at terminal states. The CLI reads it on subsequent
// runs to decide noop-success vs resume-from-N.
type exportSentinel struct {
	Status          string      `json:"status"` // "in_progress" | "done" | "interrupted"
	First           uint64      `json:"first"`
	Last            uint64      `json:"last"` // target last (may exceed HighestExported on interrupt)
	HighestExported uint64      `json:"highestExported"`
	FirstHash       common.Hash `json:"firstHash"`
	LastHash        common.Hash `json:"lastHash"` // hash of HighestExported (not of `Last`)
	UpdatedAt       time.Time   `json:"updatedAt"`
}

// exportProgressInterval is how often the sentinel gets rewritten. 1024 was
// chosen so a 1M-block C-Chain export writes ~1000 sentinel updates — plenty
// of resume granularity without flooding the disk.
const exportProgressInterval = 1024

// ExportChain exports a blockchain segment to a local RLP file.
// RPC: admin_exportChain
//
// Behavior:
//   - Streams blocks block-by-block via rlp.Encode (never loads the whole
//     chain into memory).
//   - Writes a sentinel file at <file>.sentinel after every
//     exportProgressInterval blocks and on terminal states.
//   - Idempotent: if `file` already exists with a valid sentinel whose
//     {First,Last,FirstHash,LastHash} match the request, returns Status=noop
//     without re-exporting.
//   - Interruptable: ctx.Done() flushes the partial file + writes a sentinel
//     with Status=interrupted, HighestExported set to the last block fully
//     encoded. The caller can re-invoke with the same args; this implementation
//     does not yet append (always overwrites) — the CLI is expected to read
//     the sentinel and decide whether to resume by raising First.
func (api *AdminAPI) ExportChain(ctx context.Context, file string, first, last uint64) (*ExportChainResult, error) {
	log.Info("admin_exportChain called", "file", file, "first", first, "last", last)

	api.vm.vmLock.Lock()
	defer api.vm.vmLock.Unlock()

	if api.vm.eth == nil {
		return nil, fmt.Errorf("ethereum backend not initialized")
	}

	chain := api.vm.eth.BlockChain()
	if chain == nil {
		return nil, fmt.Errorf("blockchain not initialized")
	}

	// Clamp `last` to the current head — the caller may have requested
	// a future height (the common case from `--to-height latest` translating
	// to math.MaxUint64).
	current := chain.CurrentBlock().Number.Uint64()
	if last > current {
		last = current
	}
	if first > last {
		return nil, fmt.Errorf("first (%d) > last (%d) — empty range", first, last)
	}

	// Resolve expected first/last hashes from the live chain so we can
	// (a) write them into the sentinel and (b) check against an existing
	// sentinel for the idempotency no-op.
	firstBlock := chain.GetBlockByNumber(first)
	if firstBlock == nil {
		return nil, fmt.Errorf("first block %d not found", first)
	}
	lastBlock := chain.GetBlockByNumber(last)
	if lastBlock == nil {
		return nil, fmt.Errorf("last block %d not found", last)
	}
	expectedFirstHash := firstBlock.Hash()
	expectedLastHash := lastBlock.Hash()

	sentinelPath := file + ".sentinel"

	// Idempotency: if both the file and a "done" sentinel exist with matching
	// {first,last,firstHash,lastHash}, return noop-success. This is what makes
	// admin_exportChain safe to call from a cron Job — repeated invocations
	// against an already-exported height range are zero-cost.
	if existing, ok := readSentinel(sentinelPath); ok {
		if existing.Status == "done" &&
			existing.First == first && existing.Last == last &&
			existing.FirstHash == expectedFirstHash &&
			existing.LastHash == expectedLastHash {
			if _, err := os.Stat(file); err == nil {
				log.Info("admin_exportChain: noop (existing matches request)",
					"file", file, "first", first, "last", last)
				return &ExportChainResult{
					Success:         true,
					Status:          "noop",
					BlocksExported:  last - first + 1,
					First:           first,
					Last:            last,
					HighestExported: last,
					FirstHash:       expectedFirstHash,
					LastHash:        expectedLastHash,
					SentinelPath:    sentinelPath,
					Message:         "existing export matches request, no-op",
				}, nil
			}
		}
	}

	// Open output file (truncate — this is an overwrite, not append).
	out, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %w", err)
	}
	defer out.Close()

	var writer io.Writer = out
	// Use gzip if file ends with .gz
	if strings.HasSuffix(file, ".gz") {
		gzWriter := gzip.NewWriter(out)
		defer gzWriter.Close()
		writer = gzWriter
	}

	// Initialize sentinel as in_progress before the first write so a kill -9
	// in the loop leaves a readable marker.
	writeSentinel(sentinelPath, exportSentinel{
		Status:          "in_progress",
		First:           first,
		Last:            last,
		HighestExported: 0, // zero is a sentinel for "nothing written yet"
		FirstHash:       expectedFirstHash,
		LastHash:        common.Hash{},
		UpdatedAt:       time.Now().UTC(),
	})

	var exported uint64
	var highestHash common.Hash
	for i := first; i <= last; i++ {
		// Cooperative cancellation: every block boundary checks ctx so a
		// long export (1M+ blocks) can be aborted in <1s. On cancel we
		// flush gzip + write the "interrupted" sentinel.
		select {
		case <-ctx.Done():
			highest := i - 1
			if exported == 0 {
				// No blocks written yet — the file is empty. Still emit
				// an interrupted sentinel so the operator sees the attempt.
				writeSentinel(sentinelPath, exportSentinel{
					Status:          "interrupted",
					First:           first,
					Last:            last,
					HighestExported: 0,
					FirstHash:       expectedFirstHash,
					LastHash:        common.Hash{},
					UpdatedAt:       time.Now().UTC(),
				})
				log.Warn("admin_exportChain: canceled before first block written",
					"file", file, "err", ctx.Err())
				return &ExportChainResult{
					Success:         false,
					Status:          "interrupted",
					BlocksExported:  0,
					First:           first,
					Last:            last,
					HighestExported: 0,
					FirstHash:       expectedFirstHash,
					LastHash:        common.Hash{},
					SentinelPath:    sentinelPath,
					Message:         fmt.Sprintf("canceled before first block: %v", ctx.Err()),
				}, ctx.Err()
			}
			writeSentinel(sentinelPath, exportSentinel{
				Status:          "interrupted",
				First:           first,
				Last:            last,
				HighestExported: highest,
				FirstHash:       expectedFirstHash,
				LastHash:        highestHash,
				UpdatedAt:       time.Now().UTC(),
			})
			log.Warn("admin_exportChain: interrupted",
				"file", file, "highest", highest, "err", ctx.Err())
			return &ExportChainResult{
				Success:         false,
				Status:          "interrupted",
				BlocksExported:  exported,
				First:           first,
				Last:            last,
				HighestExported: highest,
				FirstHash:       expectedFirstHash,
				LastHash:        highestHash,
				SentinelPath:    sentinelPath,
				Message:         fmt.Sprintf("interrupted at block %d: %v", highest, ctx.Err()),
			}, ctx.Err()
		default:
		}

		block := chain.GetBlockByNumber(i)
		if block == nil {
			// Sentinel reflects partial state for diagnostics.
			writeSentinel(sentinelPath, exportSentinel{
				Status:          "interrupted",
				First:           first,
				Last:            last,
				HighestExported: i - 1,
				FirstHash:       expectedFirstHash,
				LastHash:        highestHash,
				UpdatedAt:       time.Now().UTC(),
			})
			return nil, fmt.Errorf("block %d not found", i)
		}
		if err := rlp.Encode(writer, block); err != nil {
			writeSentinel(sentinelPath, exportSentinel{
				Status:          "interrupted",
				First:           first,
				Last:            last,
				HighestExported: i - 1,
				FirstHash:       expectedFirstHash,
				LastHash:        highestHash,
				UpdatedAt:       time.Now().UTC(),
			})
			return nil, fmt.Errorf("failed to encode block %d: %w", i, err)
		}
		exported++
		highestHash = block.Hash()

		// Periodic sentinel update so the CLI / operator can poll progress.
		if exported%exportProgressInterval == 0 {
			writeSentinel(sentinelPath, exportSentinel{
				Status:          "in_progress",
				First:           first,
				Last:            last,
				HighestExported: i,
				FirstHash:       expectedFirstHash,
				LastHash:        highestHash,
				UpdatedAt:       time.Now().UTC(),
			})
		}
	}

	// Terminal "done" sentinel — this is the marker the next admin_exportChain
	// call reads for the idempotency no-op.
	writeSentinel(sentinelPath, exportSentinel{
		Status:          "done",
		First:           first,
		Last:            last,
		HighestExported: last,
		FirstHash:       expectedFirstHash,
		LastHash:        expectedLastHash,
		UpdatedAt:       time.Now().UTC(),
	})
	log.Info("admin_exportChain: completed",
		"blocks", exported, "first", first, "last", last,
		"firstHash", expectedFirstHash.Hex(), "lastHash", expectedLastHash.Hex())
	return &ExportChainResult{
		Success:         true,
		Status:          "ok",
		BlocksExported:  exported,
		First:           first,
		Last:            last,
		HighestExported: last,
		FirstHash:       expectedFirstHash,
		LastHash:        expectedLastHash,
		SentinelPath:    sentinelPath,
		Message:         fmt.Sprintf("exported %d blocks [%d..%d]", exported, first, last),
	}, nil
}

// readSentinel returns the parsed sentinel and ok=true when the file exists
// and contains a valid JSON exportSentinel. Returns ok=false on any error so
// callers fall through to the no-existing-state code path.
func readSentinel(path string) (exportSentinel, bool) {
	var s exportSentinel
	data, err := os.ReadFile(path)
	if err != nil {
		return s, false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, false
	}
	return s, true
}

// writeSentinel writes the JSON sentinel atomically (tmp + rename) so a kill
// during the write never leaves a half-written sentinel file behind.
func writeSentinel(path string, s exportSentinel) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		log.Warn("admin_exportChain: sentinel marshal failed", "err", err)
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		log.Warn("admin_exportChain: sentinel write failed", "err", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Warn("admin_exportChain: sentinel rename failed", "err", err)
	}
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

// importBlocksFromFile imports blocks from an RLP-encoded file.
// It commits state to disk every CommitInterval blocks to ensure restart-safety.
// afterCommit is called after each state commit with the latest block hash and height,
// allowing the caller to persist metadata (e.g. acceptedBlockDB) atomically with state.
// Returns (blocksImported, lastBlockHash, lastBlockHeight, error).
func importBlocksFromFile(chain *core.BlockChain, file string, afterCommit func(common.Hash, uint64) error) (int, common.Hash, uint64, error) {
	// Ensure genesis state is accessible before import
	if err := chain.EnsureGenesisState(); err != nil {
		return 0, common.Hash{}, 0, fmt.Errorf("failed to ensure genesis state: %w", err)
	}

	in, err := os.Open(file)
	if err != nil {
		return 0, common.Hash{}, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer in.Close()

	var reader io.Reader = in
	if strings.HasSuffix(file, ".gz") {
		if reader, err = gzip.NewReader(reader); err != nil {
			return 0, common.Hash{}, 0, fmt.Errorf("failed to create gzip reader: %w", err)
		}
	}

	stream := rlp.NewStream(reader, 0)
	blocks := make([]*types.Block, 0, 2500)
	totalParsed := 0
	totalImported := 0
	skippedGenesis := false
	commitInterval := chain.CommitInterval()
	var lastCommitHeight uint64

	// Skip blocks already in the canonical chain (resume after crash/restart).
	// The current head has committed state; blocks at or below it are already imported.
	currentHead := chain.CurrentBlock().Number.Uint64()
	if currentHead > 0 {
		log.Info("ImportChain: resuming from current head", "head", currentHead)
	}

	for batch := 0; ; batch++ {
		// Load a batch of blocks from the input file
		for len(blocks) < cap(blocks) {
			block := new(types.Block)
			if err := stream.Decode(block); err == io.EOF {
				break
			} else if err != nil {
				return totalImported, common.Hash{}, 0, fmt.Errorf("block %d: failed to parse: %w", totalParsed, err)
			}
			if block.NumberU64() == 0 {
				skippedGenesis = true
				continue
			}
			// Skip blocks already in the canonical chain
			if block.NumberU64() <= currentHead {
				continue
			}
			blocks = append(blocks, block)
			totalParsed++
		}

		if len(blocks) == 0 {
			if totalParsed == 0 && !skippedGenesis {
				return 0, common.Hash{}, 0, fmt.Errorf("no blocks found in file")
			}
			break
		}

		firstBlock := blocks[0]
		firstNum := firstBlock.NumberU64()
		parentHash := firstBlock.ParentHash().Hex()

		// Verify parent block exists
		if firstNum > 0 && !chain.HasBlock(firstBlock.ParentHash(), firstNum-1) {
			genesisHash := chain.Genesis().Hash()
			if firstNum == 1 && firstBlock.ParentHash() != genesisHash {
				return totalImported, common.Hash{}, 0, fmt.Errorf("batch %d: block 1 parent mismatch - expected genesis %s, got %s",
					batch, genesisHash.Hex(), parentHash)
			}
			return totalImported, common.Hash{}, 0, fmt.Errorf("batch %d: parent block missing - firstBlock=%d, parentHash=%s",
				batch, firstNum, parentHash)
		}

		// Insert blocks
		log.Info("ImportChain: inserting batch", "batch", batch, "blocks", len(blocks), "firstNum", firstNum)
		n, err := chain.InsertChain(blocks)
		if err != nil {
			return totalImported, common.Hash{}, 0, fmt.Errorf("batch %d: insert failed after %d blocks: %w", batch, n, err)
		}

		lastInsertedBlock := blocks[n-1]
		currentHeight := lastInsertedBlock.NumberU64()

		// Update last accepted so RPC can query imported blocks
		if err := chain.SetLastAcceptedBlockDirect(lastInsertedBlock); err != nil {
			return totalImported, common.Hash{}, 0, fmt.Errorf("batch %d: failed to set last accepted: %w", batch, err)
		}

		// Periodically commit state trie to disk to ensure restart-safety.
		// Without this, all trie nodes accumulate in memory and are lost on crash,
		// causing "required historical state unavailable (reexec=8192)" on restart.
		if currentHeight-lastCommitHeight >= commitInterval {
			log.Info("ImportChain: committing state", "block", currentHeight)
			if err := chain.ForceCommitState(lastInsertedBlock); err != nil {
				return totalImported, common.Hash{}, 0, fmt.Errorf("failed to commit state at block %d: %w", currentHeight, err)
			}
			if afterCommit != nil {
				if err := afterCommit(lastInsertedBlock.Hash(), currentHeight); err != nil {
					return totalImported, common.Hash{}, 0, fmt.Errorf("afterCommit failed at block %d: %w", currentHeight, err)
				}
			}
			lastCommitHeight = currentHeight
		}

		totalImported += n
		log.Info("ImportChain: batch done", "batch", batch, "imported", n, "total", totalImported, "height", currentHeight)
		blocks = blocks[:0]
	}

	if totalImported == 0 {
		return 0, common.Hash{}, 0, fmt.Errorf("no blocks imported (parsed=%d)", totalParsed)
	}

	// Final state commit to ensure the last blocks are persisted
	finalBlock := chain.CurrentBlock()
	finalHeight := finalBlock.Number.Uint64()
	finalHash := finalBlock.Hash()
	if finalHeight > lastCommitHeight {
		log.Info("ImportChain: final state commit", "block", finalHeight)
		finalFullBlock := chain.GetBlock(finalHash, finalHeight)
		if finalFullBlock == nil {
			return totalImported, common.Hash{}, 0, fmt.Errorf("final block %d not found after import", finalHeight)
		}
		if err := chain.ForceCommitState(finalFullBlock); err != nil {
			return totalImported, common.Hash{}, 0, fmt.Errorf("failed to commit final state at block %d: %w", finalHeight, err)
		}
		if afterCommit != nil {
			if err := afterCommit(finalHash, finalHeight); err != nil {
				return totalImported, common.Hash{}, 0, fmt.Errorf("afterCommit failed at final block %d: %w", finalHeight, err)
			}
		}
	}

	log.Info("ImportChain: completed", "imported", totalImported, "height", finalHeight)
	return totalImported, finalHash, finalHeight, nil
}

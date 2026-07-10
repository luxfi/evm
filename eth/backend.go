// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2014 The go-ethereum Authors
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

// Package eth implements the Ethereum protocol.
package eth

import (
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/luxfi/evm/consensus"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/core/bloombits"
	"github.com/luxfi/evm/core/state/pruner"
	"github.com/luxfi/evm/core/txpool"
	"github.com/luxfi/evm/core/txpool/legacypool"
	"github.com/luxfi/evm/eth/ethconfig"
	"github.com/luxfi/evm/eth/filters"
	"github.com/luxfi/evm/eth/gasprice"
	"github.com/luxfi/evm/eth/tracers"
	"github.com/luxfi/evm/internal/ethapi"
	"github.com/luxfi/evm/internal/shutdowncheck"
	"github.com/luxfi/evm/miner"
	"github.com/luxfi/evm/node"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/plugin/evm/customrawdb"
	"github.com/luxfi/evm/rpc"
	"github.com/luxfi/geth/accounts"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/geth/event"
	log "github.com/luxfi/log"
	"github.com/luxfi/runtime"
	"github.com/luxfi/timer/mockable"
)

// Config contains the configuration options of the ETH protocol.
// Deprecated: use ethconfig.Config instead.
type Config = ethconfig.Config

var DefaultSettings Settings = Settings{MaxBlocksPerRequest: 2000}

type Settings struct {
	MaxBlocksPerRequest int64 // Maximum number of blocks to serve per getLogs request

	// ChainRuntime is the chain Runtime the VM already holds (networkID / X / C chain ids,
	// plus the cross-chain SharedMemory capability). It is threaded explicitly into
	// core.NewBlockChain so blocks re-executed during startup recovery see the identity they
	// were accepted under. Required for chains running the identity-gated 0x9999 DEX value
	// path; nil is fine otherwise.
	ChainRuntime *runtime.Runtime
}

// PushGossiper sends pushes pending transactions to peers until they are
// removed from the mempool.
type PushGossiper interface {
	Add(*types.Transaction)
}

// Ethereum implements the Ethereum full node service.
type Ethereum struct {
	config *Config

	// Handlers
	txPool *txpool.TxPool

	blockchain *core.BlockChain
	gossiper   PushGossiper

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests     chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer      *core.ChainIndexer             // Bloom indexer operating during block imports
	closeBloomHandler chan struct{}

	APIBackend *EthAPIBackend

	miner     *miner.Miner
	etherbase common.Address

	networkID     uint64
	netRPCService *ethapi.NetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and etherbase)

	// lastQuasarHeight is the highest EXPORT-FINAL (Quasar, ⅔-by-stake) block number — the block
	// the eth `finalized`/`safe` tags resolve to. It is pushed by the consensus engine strictly
	// AFTER the block's local (Nova majority) Accept, and is DURABLE (the plugin VM persists it and
	// reloads it on boot), so the export surface never reports a reorgable Nova-accepted block as
	// finalized and never regresses on restart. Monotone; 0 (→ genesis) before the first export.
	lastQuasarHeight atomic.Uint64

	shutdownTracker *shutdowncheck.ShutdownTracker // Tracks if and when the node has shutdown ungracefully

	stackRPCs []rpc.API

	settings Settings // Settings for Ethereum API

	// PostImportCallback is called after successful block import via admin_importChain.
	// This allows the VM layer to update its acceptedBlockDB after admin API imports.
	postImportCallback func(lastBlockHash common.Hash, lastBlockHeight uint64) error
}

// roundUpCacheSize returns [input] rounded up to the next multiple of [allocSize]
func roundUpCacheSize(input int, allocSize int) int {
	cacheChunks := (input + allocSize - 1) / allocSize
	return cacheChunks * allocSize
}

// New creates a new Ethereum object (including the
// initialisation of the common Ethereum object)
func New(
	stack *node.Node,
	config *Config,
	gossiper PushGossiper,
	chainDb ethdb.Database,
	settings Settings,
	lastAcceptedHash common.Hash,
	engine consensus.Engine,
	clock *mockable.Clock,
) (*Ethereum, error) {
	if chainDb == nil {
		return nil, errors.New("chainDb cannot be nil")
	}

	// round TrieCleanCache and SnapshotCache up to nearest 64MB, since fastcache will mmap
	// memory in 64MBs chunks.
	config.TrieCleanCache = roundUpCacheSize(config.TrieCleanCache, 64)
	config.SnapshotCache = roundUpCacheSize(config.SnapshotCache, 64)

	log.Info(
		"Allocated memory caches",
		"trie clean", common.StorageSize(config.TrieCleanCache)*1024*1024,
		"trie dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024,
		"snapshot clean", common.StorageSize(config.SnapshotCache)*1024*1024,
	)

	scheme, err := customrawdb.ParseStateSchemeExt(config.StateScheme, chainDb)
	if err != nil {
		return nil, err
	}
	// Try to recover offline state pruning only in hash-based.
	if scheme == rawdb.HashScheme {
		// Note: RecoverPruning must be called to handle the case that we are midway through offline pruning.
		// If the data directory is changed in between runs preventing RecoverPruning from performing its job correctly,
		// it may cause DB corruption.
		// Since RecoverPruning will only continue a pruning run that already began, we do not need to ensure that
		// reprocessState has already been called and completed successfully. To ensure this, we must maintain
		// that Prune is only run after reprocessState has finished successfully.
		if err := pruner.RecoverPruning(config.OfflinePruningDataDirectory, chainDb); err != nil {
			log.Error("Failed to recover state", "error", err)
		}
	}

	networkID := config.NetworkId
	if networkID == 0 {
		networkID = config.Genesis.Config.ChainID.Uint64()
	}
	eth := &Ethereum{
		config:            config,
		gossiper:          gossiper,
		chainDb:           chainDb,
		eventMux:          new(event.TypeMux),
		accountManager:    stack.AccountManager(),
		engine:            engine,
		closeBloomHandler: make(chan struct{}),
		networkID:         networkID,
		etherbase:         config.Miner.Etherbase,
		bloomRequests:     make(chan chan *bloombits.Retrieval),
		bloomIndexer:      NewBloomIndexer(chainDb, params.BloomBitsBlocks, params.BloomConfirms),
		settings:          settings,
		shutdownTracker:   shutdowncheck.NewShutdownTracker(chainDb),
	}
	bcVersion := rawdb.ReadDatabaseVersion(chainDb)
	dbVer := "<nil>"
	if bcVersion != nil {
		dbVer = fmt.Sprintf("%d", *bcVersion)
	}
	log.Info("Initialising Ethereum protocol", "network", networkID, "dbversion", dbVer)

	if !config.SkipBcVersionCheck {
		if bcVersion != nil && *bcVersion > core.BlockChainVersion {
			return nil, fmt.Errorf("database version is v%d, EVM %s only supports v%d", *bcVersion, params.VersionWithMeta, core.BlockChainVersion)
		} else if bcVersion == nil || *bcVersion < core.BlockChainVersion {
			log.Warn("Upgrade blockchain database version", "from", dbVer, "to", core.BlockChainVersion)
			rawdb.WriteDatabaseVersion(chainDb, core.BlockChainVersion)
		}
	}

	// If the context is not set, avoid a panic. Only necessary during firewood use.
	chainDataDir := ""
	if ctx := params.GetExtra(config.Genesis.Config).ConsensusCtx; ctx != nil {
		// Consensus context does not expose GetChainDataDir; use a default sub-path.
		chainDataDir = filepath.Join(chainDataDir, "chain")
	}

	var (
		vmConfig = vm.Config{
			EnablePreimageRecording: config.EnablePreimageRecording,
		}
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:                  config.TrieCleanCache,
			TrieDirtyLimit:                  config.TrieDirtyCache,
			TrieDirtyCommitTarget:           config.TrieDirtyCommitTarget,
			TriePrefetcherParallelism:       config.TriePrefetcherParallelism,
			Pruning:                         config.Pruning,
			AcceptorQueueLimit:              config.AcceptorQueueLimit,
			CommitInterval:                  config.CommitInterval,
			PopulateMissingTries:            config.PopulateMissingTries,
			PopulateMissingTriesParallelism: config.PopulateMissingTriesParallelism,
			AllowMissingTries:               config.AllowMissingTries,
			SnapshotDelayInit:               config.SnapshotDelayInit,
			SnapshotLimit:                   config.SnapshotCache,
			SnapshotWait:                    config.SnapshotWait,
			SnapshotVerify:                  config.SnapshotVerify,
			SnapshotNoBuild:                 config.SkipSnapshotRebuild,
			Preimages:                       config.Preimages,
			AcceptedCacheSize:               config.AcceptedCacheSize,
			TransactionHistory:              config.TransactionHistory,
			SkipTxIndexing:                  config.SkipTxIndexing,
			StateHistory:                    config.StateHistory,
			StateScheme:                     scheme,
			ChainDataDir:                    chainDataDir,
		}
	)

	if err := eth.precheckPopulateMissingTries(); err != nil {
		return nil, err
	}
	eth.blockchain, err = core.NewBlockChain(chainDb, cacheConfig, config.Genesis, eth.engine, vmConfig, lastAcceptedHash, config.SkipUpgradeCheck, settings.ChainRuntime)
	if err != nil {
		return nil, err
	}

	// Free airdrop data to save memory usage
	defer func() {
		config.Genesis.AirdropData = nil
	}()

	if err := eth.handleOfflinePruning(cacheConfig, config.Genesis, vmConfig, lastAcceptedHash); err != nil {
		return nil, err
	}

	eth.bloomIndexer.Start(eth.blockchain)

	// config.BlobPool.Datadir = ""
	// blobPool := blobpool.New(config.BlobPool, &chainWithFinalBlock{eth.blockchain})

	legacyPool := legacypool.New(config.TxPool, eth.blockchain)

	eth.txPool, err = txpool.New(config.TxPool.PriceLimit, eth.blockchain, []txpool.SubPool{legacyPool}) //, blobPool})
	if err != nil {
		return nil, err
	}

	eth.miner = miner.New(eth, &config.Miner, eth.blockchain.Config(), eth.EventMux(), eth.engine, clock)

	allowUnprotectedTxHashes := make(map[common.Hash]struct{})
	for _, txHash := range config.AllowUnprotectedTxHashes {
		allowUnprotectedTxHashes[txHash] = struct{}{}
	}

	eth.APIBackend = &EthAPIBackend{
		extRPCEnabled:              stack.Config().ExtRPCEnabled(),
		allowUnprotectedTxs:        config.AllowUnprotectedTxs,
		allowUnprotectedTxHashes:   allowUnprotectedTxHashes,
		allowUnfinalizedQueries:    config.AllowUnfinalizedQueries,
		historicalProofQueryWindow: config.HistoricalProofQueryWindow,
		eth:                        eth,
	}
	if config.Pruning {
		eth.APIBackend.historicalProofQueryWindow = config.StateHistory
	}
	if config.AllowUnprotectedTxs {
		log.Info("Unprotected transactions allowed")
	}
	gpoParams := config.GPO
	gpoParams.MinPrice = new(big.Int).SetUint64(config.TxPool.PriceLimit)
	eth.APIBackend.gpo, err = gasprice.NewOracle(eth.APIBackend, gpoParams)
	if err != nil {
		return nil, err
	}

	// Start the RPC service
	eth.netRPCService = ethapi.NewNetAPI(eth.NetVersion())

	eth.stackRPCs = stack.APIs()

	// Successful startup; push a marker and check previous unclean shutdowns.
	eth.shutdownTracker.MarkStartup()

	return eth, nil
}

// APIs return the collection of RPC services the ethereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Ethereum) APIs() []rpc.API {
	apis := ethapi.GetAPIs(s.APIBackend)

	// Append tracing APIs
	apis = append(apis, tracers.APIs(s.APIBackend)...)

	// Add the APIs from the node
	apis = append(apis, s.stackRPCs...)

	// Create [filterSystem] with the log cache size set in the config.
	filterSystem := filters.NewFilterSystem(s.APIBackend, filters.Config{
		Timeout: 5 * time.Minute,
	})

	// Append all the local APIs and return
	devAPI := NewDevAPI(s)
	return append(apis, []rpc.API{
		{
			Namespace: "eth",
			Service:   NewEthereumAPI(s),
			Name:      "eth",
		}, {
			Namespace: "eth",
			Service:   filters.NewFilterAPI(filterSystem),
			Name:      "eth-filter",
		}, {
			Namespace: "admin",
			Service:   NewAdminAPI(s),
			Name:      "admin",
		}, {
			Namespace: "debug",
			Service:   NewDebugAPI(s),
			Name:      "debug",
		}, {
			Namespace: "net",
			Service:   s.netRPCService,
			Name:      "net",
		},
		// Dev APIs for Anvil/Hardhat compatibility (eth namespace)
		{
			Namespace: "eth",
			Service:   devAPI,
			Name:      "eth-dev",
		},
		// Dev APIs (evm namespace)
		{
			Namespace: "evm",
			Service:   devAPI,
			Name:      "evm-dev",
		},
		// Dev APIs (anvil namespace)
		{
			Namespace: "anvil",
			Service:   devAPI,
			Name:      "anvil-dev",
		},
		// Dev APIs (hardhat namespace)
		{
			Namespace: "hardhat",
			Service:   devAPI,
			Name:      "hardhat-dev",
		},
	}...)
}

func (s *Ethereum) Etherbase() (eb common.Address, err error) {
	s.lock.RLock()
	etherbase := s.etherbase
	s.lock.RUnlock()

	if etherbase != (common.Address{}) {
		return etherbase, nil
	}
	return common.Address{}, fmt.Errorf("etherbase must be explicitly specified")
}

// SetEtherbase sets the mining reward address.
func (s *Ethereum) SetEtherbase(etherbase common.Address) {
	s.lock.Lock()
	s.etherbase = etherbase
	s.lock.Unlock()

	s.miner.SetEtherbase(etherbase)
}

func (s *Ethereum) Miner() *miner.Miner { return s.miner }

func (s *Ethereum) AccountManager() *accounts.Manager { return s.accountManager }
func (s *Ethereum) BlockChain() *core.BlockChain      { return s.blockchain }
func (s *Ethereum) TxPool() *txpool.TxPool            { return s.txPool }
func (s *Ethereum) EventMux() *event.TypeMux          { return s.eventMux }
func (s *Ethereum) Engine() consensus.Engine          { return s.engine }
func (s *Ethereum) ChainDb() ethdb.Database           { return s.chainDb }

func (s *Ethereum) NetVersion() uint64               { return s.networkID }
func (s *Ethereum) ArchiveMode() bool                { return !s.config.Pruning }
func (s *Ethereum) BloomIndexer() *core.ChainIndexer { return s.bloomIndexer }

// SetPostImportCallback registers a callback that is called after successful
// block import via admin_importChain. This allows the VM layer to update its
// acceptedBlockDB after admin API imports.
func (s *Ethereum) SetPostImportCallback(cb func(lastBlockHash common.Hash, lastBlockHeight uint64) error) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.postImportCallback = cb
}

// CallPostImportCallback invokes the post-import callback if set.
// Returns nil if no callback is registered.
func (s *Ethereum) CallPostImportCallback(lastBlockHash common.Hash, lastBlockHeight uint64) error {
	s.lock.RLock()
	cb := s.postImportCallback
	s.lock.RUnlock()

	if cb == nil {
		return nil
	}
	return cb(lastBlockHash, lastBlockHeight)
}

// Start implements node.Lifecycle, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *Ethereum) Start() {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers(params.BloomBitsBlocks)

	// Regularly update shutdown marker
	s.shutdownTracker.Start()
}

// Stop implements node.Lifecycle, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *Ethereum) Stop() error {
	_ = s.bloomIndexer.Close()
	close(s.closeBloomHandler)
	_ = s.txPool.Close()
	s.blockchain.Stop()
	_ = s.engine.Close()

	// Clean shutdown marker as the last thing before closing db
	s.shutdownTracker.Stop()
	log.Info("Stopped shutdownTracker")

	_ = s.chainDb.Close()
	log.Info("Closed chaindb")
	s.eventMux.Stop()
	log.Info("Stopped EventMux")
	return nil
}

func (s *Ethereum) LastAcceptedBlock() *types.Block {
	return s.blockchain.LastAcceptedBlock()
}

// NewTestEthereum builds an Ethereum wrapping only the given blockchain — TEST SUPPORT so a test
// in another package (e.g. plugin/evm's boot-wiring reconcile test) can drive the
// `finalized`/`safe` + export-height resolution against a REAL chain without the full node stack.
// ONLY the blockchain-backed reads (LastAcceptedBlock, GetBlockByNumber, the export-height clamp)
// are wired; everything else is nil. Not for production use.
func NewTestEthereum(chain *core.BlockChain) *Ethereum {
	return &Ethereum{blockchain: chain}
}

// SetLastQuasarHeight records the highest EXPORT-FINAL (Quasar, ⅔-by-stake) block number.
// Monotone: a value at or below the current height is ignored (the export frontier only ever
// advances). Called by the plugin VM from the consensus export-frontier observer, and once on
// boot with the persisted value.
func (s *Ethereum) SetLastQuasarHeight(h uint64) {
	for {
		cur := s.lastQuasarHeight.Load()
		if h <= cur {
			return
		}
		if s.lastQuasarHeight.CompareAndSwap(cur, h) {
			return
		}
	}
}

// LastQuasarHeight returns the RAW highest EXPORT-FINAL (Quasar) block number the VM has recorded
// (0 before the first export). This is the persisted/in-mem value; export SURFACES must instead
// read ClampedQuasarHeight (which never exceeds the accept tip). Used by the node's boot re-seed
// of the consensus export frontier, where the reconcile has already run so raw == clamped.
func (s *Ethereum) LastQuasarHeight() uint64 { return s.lastQuasarHeight.Load() }

// ClampedQuasarHeight is the EXPORT-FINAL (Quasar) height CLAMPED to the served accept tip —
// min(LastQuasarHeight, LastAccepted.Number). This is the ONE place the clamp lives: BOTH the RPC
// `finalized`/`safe` resolver (EthAPIBackend.quasarHeight) and the warp export gate
// (warpBlockClient.LastQuasarHeight) read it, so an export surface can NEVER name a block above the
// served accept tip — STRUCTURALLY, regardless of reconcile timing — with no second inlined copy of
// the clamp to drift. In normal operation quasar ≤ accept so the clamp is invisible; it binds only
// in the transient rewind window before reconcileQuasarWithAccept clears the stale key.
func (s *Ethereum) ClampedQuasarHeight() uint64 {
	h := s.lastQuasarHeight.Load()
	if acc := s.blockchain.LastAcceptedBlock(); acc != nil {
		if n := acc.NumberU64(); n < h {
			return n
		}
	}
	return h
}

// ResetLastQuasarHeight FORCE-sets the in-memory export height, bypassing the monotone guard of
// SetLastQuasarHeight. It exists for the ONE legal regression of the otherwise-irreversible
// export frontier: a rewind / RLP re-import that lowered the accept tip below the persisted
// quasar height (the plugin VM's reconcileQuasarWithAccept calls this with 0 — conservative:
// nothing export-final until the rebuilt chain re-certifies). Normal operation NEVER calls it.
func (s *Ethereum) ResetLastQuasarHeight(h uint64) { s.lastQuasarHeight.Store(h) }

// precheckPopulateMissingTries returns an error if config flags should prevent
// [populateMissingTries]
//
// NOTE: [populateMissingTries] is called from [New] to ensure all
// state is repaired before any async processes (specifically snapshot re-generation)
// are started which could interfere with historical re-generation.
func (s *Ethereum) precheckPopulateMissingTries() error {
	if s.config.PopulateMissingTries != nil && (s.config.Pruning || s.config.OfflinePruning) {
		return fmt.Errorf("cannot run populate missing tries when pruning (enabled: %t)/offline pruning (enabled: %t) is enabled", s.config.Pruning, s.config.OfflinePruning)
	}

	if s.config.PopulateMissingTries == nil {
		// Delete the populate missing tries marker to indicate that the node started with
		// populate missing tries disabled.
		if err := customrawdb.DeletePopulateMissingTries(s.chainDb); err != nil {
			return fmt.Errorf("failed to write populate missing tries disabled marker: %w", err)
		}
		return nil
	}

	if lastRun, err := customrawdb.ReadPopulateMissingTries(s.chainDb); err == nil {
		log.Error("Populate missing tries is not meant to be left enabled permanently. Please disable populate missing tries and allow your node to start successfully before running again.")
		return fmt.Errorf("cannot start chain with populate missing tries enabled on consecutive starts (last=%v)", lastRun)
	}

	// Note: Time Marker is written inside of [populateMissingTries] once it
	// succeeds inside of [NewBlockChain]
	return nil
}

func (s *Ethereum) handleOfflinePruning(cacheConfig *core.CacheConfig, gspec *core.Genesis, vmConfig vm.Config, lastAcceptedHash common.Hash) error {
	if s.config.OfflinePruning && !s.config.Pruning {
		return core.ErrRefuseToCorruptArchiver
	}

	if !s.config.OfflinePruning {
		// Delete the offline pruning marker to indicate that the node started with offline pruning disabled.
		if err := customrawdb.DeleteOfflinePruning(s.chainDb); err != nil {
			return fmt.Errorf("failed to write offline pruning disabled marker: %w", err)
		}
		return nil
	}

	// Perform offline pruning after NewBlockChain has been called to ensure that we have rolled back the chain
	// to the last accepted block before pruning begins.
	// If offline pruning marker is on disk, then we force the node to be started with offline pruning disabled
	// before allowing another run of offline pruning.
	if lastRun, err := customrawdb.ReadOfflinePruning(s.chainDb); err == nil {
		log.Error("Offline pruning is not meant to be left enabled permanently. Please disable offline pruning and allow your node to start successfully before running offline pruning again.")
		return fmt.Errorf("cannot start chain with offline pruning enabled on consecutive starts (last=%v)", lastRun)
	}

	// Clean up middle roots
	if err := s.blockchain.CleanBlockRootsAboveLastAccepted(); err != nil {
		return err
	}
	targetRoot := s.blockchain.LastAcceptedBlock().Root()

	// Allow the blockchain to be garbage collected immediately, since we will shut down the chain after offline pruning completes.
	s.blockchain.Stop()
	s.blockchain = nil
	log.Info("Starting offline pruning", "dataDir", s.config.OfflinePruningDataDirectory, "bloomFilterSize", s.config.OfflinePruningBloomFilterSize)
	prunerConfig := pruner.Config{
		BloomSize: s.config.OfflinePruningBloomFilterSize,
		Datadir:   s.config.OfflinePruningDataDirectory,
	}

	pruner, err := pruner.NewPruner(s.chainDb, prunerConfig)
	if err != nil {
		return fmt.Errorf("failed to create new pruner with data directory: %s, size: %d, due to: %w", s.config.OfflinePruningDataDirectory, s.config.OfflinePruningBloomFilterSize, err)
	}
	if err := pruner.Prune(targetRoot); err != nil {
		return fmt.Errorf("failed to prune blockchain with target root: %s due to: %w", targetRoot, err)
	}
	// Note: Time Marker is written inside of [Prune] before compaction begins
	// (considered an optional optimization)
	s.blockchain, err = core.NewBlockChain(s.chainDb, cacheConfig, gspec, s.engine, vmConfig, lastAcceptedHash, s.config.SkipUpgradeCheck, s.settings.ChainRuntime)
	if err != nil {
		return fmt.Errorf("failed to re-initialize blockchain after offline pruning: %w", err)
	}

	return nil
}

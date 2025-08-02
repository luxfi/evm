// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	// Node consensus imports
	"github.com/luxfi/evm/v2/commontype"
	"github.com/luxfi/evm/v2/consensus/dummy"
	evmconstants "github.com/luxfi/evm/v2/constants"
	"github.com/luxfi/evm/v2/core"
	"github.com/luxfi/evm/v2/core/rawdb"
	"github.com/luxfi/evm/v2/core/txpool"
	"github.com/luxfi/evm/v2/core/types"
	"github.com/luxfi/evm/v2/eth"
	"github.com/luxfi/evm/v2/eth/ethconfig"
	"github.com/luxfi/evm/v2/iface"
	evmids "github.com/luxfi/evm/v2/ids"
	"github.com/luxfi/evm/v2/miner"
	"github.com/luxfi/evm/v2/params"
	"github.com/luxfi/evm/v2/params/extras"
	"github.com/luxfi/evm/v2/plugin/evm/message"
	"github.com/luxfi/evm/v2/rpc"
	"github.com/luxfi/geth/node"
	"github.com/luxfi/geth/triedb"
	triedbhashdb "github.com/luxfi/geth/triedb/hashdb"
	nodequasar "github.com/luxfi/node/v2/quasar"
	commonEng "github.com/luxfi/node/v2/quasar/engine/core"
	chainblock "github.com/luxfi/node/v2/quasar/engine/chain/block"
	consensuschain "github.com/luxfi/node/v2/quasar/chain"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
	statesyncclient "github.com/luxfi/node/v2/state_sync/client"
	"github.com/luxfi/node/v2/state_sync/client/stats"
	"github.com/luxfi/node/v2/utils/constants"
	"github.com/luxfi/node/v2/utils/timer/mockable"
	"github.com/luxfi/node/v2/utils/units"
	"github.com/luxfi/warp/backend"
	luxmetrics "github.com/luxfi/metrics"
	"github.com/prometheus/client_golang/prometheus"

	// Force-load tracer engine to trigger registration
	//
	// We must import this package (not referenced elsewhere) so that the native "callTracer"
	// is added to a map of client-accessible tracers. In geth, this is done
	// inside of cmd/geth.

	_ "github.com/luxfi/geth/eth/tracers/js"
	_ "github.com/luxfi/geth/eth/tracers/native"

	// Force-load precompiles to trigger registration
	luxRPC "github.com/gorilla/rpc/v2"
	"github.com/luxfi/evm/v2/precompile/contracts/warp"
	"github.com/luxfi/evm/v2/precompile/precompileconfig"
	_ "github.com/luxfi/evm/v2/precompile/registry"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/geth/rlp"
	"github.com/luxfi/log"

	// Additional node imports

	rpcjson "github.com/gorilla/rpc/v2/json"
	"github.com/luxfi/evm/v2/peer"
	nodeMetrics "github.com/luxfi/node/v2/api/metrics"
	"github.com/luxfi/node/v2/codec"
	"github.com/luxfi/node/v2/quasar/validators"
	"github.com/luxfi/node/v2/network/p2p"
	"github.com/luxfi/node/v2/network/p2p/gossip"
	"github.com/luxfi/node/v2/utils"
	"github.com/luxfi/node/v2/utils/perms"
	"github.com/luxfi/node/v2/utils/profiler"
	"github.com/luxfi/node/v2/version"
	"github.com/luxfi/node/v2/vms/components/chain"
)

var (
	_ chainblock.ChainVM                      = &VM{}
	_ chainblock.BuildBlockWithContextChainVM = &VM{}
	_ chainblock.StateSyncableVM              = &VM{}
	_ statesyncclient.EthBlockParser     = &VM{}
)

const (
	// Max time from current time allowed for blocks, before they're considered future blocks
	// and fail verification
	maxFutureBlockTime     = 10 * time.Second
	decidedCacheSize       = 10 * units.MiB
	missingCacheSize       = 50
	unverifiedCacheSize    = 5 * units.MiB
	bytesToIDCacheSize     = 5 * units.MiB
	warpSignatureCacheSize = 500

	// Prefixes for metrics gatherers
	ethMetricsPrefix        = "eth"
	sdkMetricsPrefix        = "sdk"
	chainStateMetricsPrefix = "chain_state"

	// gossip constants
	pushGossipDiscardedElements          = 16_384
	txGossipBloomMinTargetElements       = 8 * 1024
	txGossipBloomTargetFalsePositiveRate = 0.01
	txGossipBloomResetFalsePositiveRate  = 0.05
	txGossipBloomChurnMultiplier         = 3
	txGossipTargetMessageSize            = 20 * units.KiB
	maxValidatorSetStaleness             = time.Minute
	txGossipThrottlingPeriod             = 10 * time.Second
	txGossipThrottlingLimit              = 2
	txGossipPollSize                     = 1
)

// Define the API endpoints for the VM
const (
	adminEndpoint        = "/admin"
	ethRPCEndpoint       = "/rpc"
	ethWSEndpoint        = "/ws"
	validatorsEndpoint   = "/validators"
	ethTxGossipNamespace = "eth_tx_gossip"
)

var (
	// Set last accepted key to be longer than the keys used to store accepted block IDs.
	lastAcceptedKey    = []byte("last_accepted_key")
	acceptedPrefix     = []byte("linear_accepted")
	metadataPrefix     = []byte("metadata")
	warpPrefix         = []byte("warp")
	ethDBPrefix        = []byte("ethdb")
	validatorsDBPrefix = []byte("validators")
)

var (
	errEmptyBlock                    = errors.New("empty block")
	errUnsupportedFXs                = errors.New("unsupported feature extensions")
	errInvalidBlock                  = errors.New("invalid block")
	errInvalidNonce                  = errors.New("invalid nonce")
	errUnclesUnsupported             = errors.New("uncles unsupported")
	errNilBaseFeeEVM                 = errors.New("nil base fee is invalid after evm")
	errNilBlockGasCostEVM            = errors.New("nil blockGasCost is invalid after evm")
	errInvalidHeaderPredicateResults = errors.New("invalid header predicate results")
	errInitializingLogger            = errors.New("failed to initialize logger")
)


// VM implements the block.ChainVM interface
type VM struct {
	ctx *nodequasar.Context
	// vmLock is used to coordinate global VM operations.
	vmLock sync.RWMutex
	// [cancel] may be nil until [commonEng.NormalOp] starts
	cancel context.CancelFunc
	// *chain.State helps to implement the VM interface by wrapping blocks
	// with an efficient caching layer.
	*chain.State

	config Config

	networkID   uint64
	genesisHash common.Hash
	chainConfig *params.ChainConfig
	ethConfig   ethconfig.Config

	// pointers to eth constructs
	eth        *eth.Ethereum
	txPool     *txpool.TxPool
	blockChain *core.BlockChain
	miner      *miner.Miner

	// [versiondb] is the VM's current versioned database
	versiondb iface.VersionDB

	// [db] is the VM's current database
	db database.Database

	// metadataDB is used to store one off keys.
	metadataDB database.Database

	// [chaindb] is the database supplied to the Ethereum backend
	chaindb ethdb.Database

	usingStandaloneDB bool

	// [acceptedBlockDB] is the database to store the last accepted
	// block.
	acceptedBlockDB database.Database
	// [warpDB] is used to store warp message signatures
	// set to a prefixDB with the prefix [warpPrefix]
	warpDB database.Database

	validatorsDB database.Database

	toEngine chan<- commonEng.Message

	syntacticBlockValidator BlockValidator

	builder *blockBuilder

	clock *mockable.Clock

	shutdownChan chan struct{}
	shutdownWg   sync.WaitGroup

	// Continuous Profiler
	profiler profiler.ContinuousProfiler

	Network      *p2p.Network
	networkCodec codec.Manager

	p2pValidators *p2p.Validators

	// Metrics
	multiGatherer nodeMetrics.MultiGatherer
	sdkMetrics    luxmetrics.Metrics

	bootstrapped utils.Atomic[bool]

	logger log.Logger
	// State sync server and client
	StateSyncServer
	StateSyncClient

	// Lux Warp Messaging backend
	// Used to serve BLS signatures of warp messages over RPC
	warpBackend backend.Backend

	// Initialize only sets these if nil so they can be overridden in tests
	p2pSender          commonEng.AppSender
	ethTxGossipHandler p2p.Handler
	ethTxPushGossiper  utils.Atomic[*gossip.PushGossiper[*GossipEthTx]]
	ethTxPullGossiper  gossip.Gossiper

	validatorsManager validators.Manager

	chainAlias string
	// RPC handlers (should be stopped before closing chaindb)
	rpcHandlers []interface{ Stop() }

	// Network client for state sync
	client peer.NetworkClient
}

// Initialize implements the block.ChainVM interface
func (vm *VM) Initialize(
	_ context.Context,
	chainCtx *nodequasar.Context,
	db database.Database,
	genesisBytes []byte,
	upgradeBytes []byte,
	configBytes []byte,
	fxs []*commonEng.Fx,
	appSender commonEng.AppSender,
) error {
	vm.config.SetDefaults()
	if len(configBytes) > 0 {
		if err := json.Unmarshal(configBytes, &vm.config); err != nil {
			return fmt.Errorf("failed to unmarshal config %s: %w", string(configBytes), err)
		}
	}
	if err := vm.config.Validate(); err != nil {
		return err
	}
	// We should deprecate config flags as the first thing, before we do anything else
	// because this can set old flags to new flags. log the message after we have
	// initialized the logger.
	deprecateMsg := ""

	vm.ctx = chainCtx

	// Create logger
	// BCLookup is not available in newer consensus versions
	// Just use chain ID as alias
	vm.chainAlias = vm.ctx.ChainID.String()

	// Create a luxfi/log logger for the VM
	// TODO: Eventually the node should use luxfi/log directly
	vm.logger = log.New("chain", vm.chainAlias)

	vm.logger.Info("Initializing Lux EVM VM", "Version", Version, "Config", vm.config)

	if deprecateMsg != "" {
		vm.logger.Warn("Deprecation Warning", "msg", deprecateMsg)
	}

	if len(fxs) > 0 {
		return errUnsupportedFXs
	}

	// Enable debug-level metrics that might impact runtime performance
	// Note: metrics.EnabledExpensive is removed in newer geth versions
	// The config is still available but not used by geth metrics

	vm.toEngine = make(chan commonEng.Message, 1)
	vm.shutdownChan = make(chan struct{}, 1)

	if err := vm.initializeMetrics(); err != nil {
		return fmt.Errorf("failed to initialize metrics: %w", err)
	}

	// Initialize the database
	if err := vm.initializeDBs(db); err != nil {
		return fmt.Errorf("failed to initialize databases: %w", err)
	}

	if vm.config.InspectDatabase {
		if err := vm.inspectDatabases(); err != nil {
			return err
		}
	}

	g := new(core.Genesis)
	if err := json.Unmarshal(genesisBytes, g); err != nil {
		return err
	}

	if g.Config == nil {
		g.Config = params.EVMDefaultChainConfig
	}

	// Set the Lux Context on the ChainConfig
	configExtra := params.GetExtra(g.Config)
	configExtra.LuxContext = extras.LuxContext{
		ConsensusCtx: chainCtx,
	}

	params.SetNetworkUpgradeDefaults(g.Config)

	// Load airdrop file if provided
	if vm.config.AirdropFile != "" {
		airdropData, err := os.ReadFile(vm.config.AirdropFile)
		if err != nil {
			return fmt.Errorf("could not read airdrop file '%s': %w", vm.config.AirdropFile, err)
		}
		g.AirdropData = airdropData
	}
	vm.syntacticBlockValidator = NewBlockValidator()

	if configExtra.FeeConfig == commontype.EmptyFeeConfig {
		vm.logger.Info("No fee config given in genesis, setting default fee config", "DefaultFeeConfig", params.DefaultFeeConfig)
		configExtra.FeeConfig = params.DefaultFeeConfig
	}

	// Apply upgradeBytes (if any) by unmarshalling them into [chainConfig.UpgradeConfig].
	// Initializing the chain will verify upgradeBytes are compatible with existing values.
	// This should be called before g.Verify().
	if len(upgradeBytes) > 0 {
		var upgradeConfig extras.UpgradeConfig
		if err := json.Unmarshal(upgradeBytes, &upgradeConfig); err != nil {
			return fmt.Errorf("failed to parse upgrade bytes: %w", err)
		}
		configExtra.UpgradeConfig = upgradeConfig
	}

	if configExtra.UpgradeConfig.NetworkUpgradeOverrides != nil {
		overrides := configExtra.UpgradeConfig.NetworkUpgradeOverrides
		marshaled, err := json.Marshal(overrides)
		if err != nil {
			vm.logger.Warn("Failed to marshal network upgrade overrides", "error", err, "overrides", overrides)
		} else {
			vm.logger.Info("Applying network upgrade overrides", "overrides", string(marshaled))
		}
		configExtra.Override(overrides)
	}

	params.SetEthUpgrades(g.Config, configExtra.NetworkUpgrades)

	if err := configExtra.Verify(); err != nil {
		return fmt.Errorf("failed to verify genesis: %w", err)
	}

	vm.ethConfig = ethconfig.NewDefaultConfig()
	vm.ethConfig.Genesis = g
	// NetworkID here is different tha Lux's NetworkID.
	// Lux's NetworkID represents the Lux network is running on
	// like Testnet, Mainnet, Local, etc.
	// The NetworkId here is kept same as ChainID to be compatible with
	// Ethereum tooling.
	vm.ethConfig.NetworkId = g.Config.ChainID.Uint64()

	// Set minimum price for mining and default gas price oracle value to the min
	// gas price to prevent so transactions and blocks all use the correct fees
	vm.ethConfig.RPCGasCap = vm.config.RPCGasCap
	vm.ethConfig.RPCEVMTimeout = vm.config.APIMaxDuration.Duration
	vm.ethConfig.RPCTxFeeCap = vm.config.RPCTxFeeCap

	vm.ethConfig.TxPool.Locals = vm.config.PriorityRegossipAddresses
	vm.ethConfig.TxPool.NoLocals = !vm.config.LocalTxsEnabled
	vm.ethConfig.TxPool.PriceLimit = vm.config.TxPoolPriceLimit
	vm.ethConfig.TxPool.PriceBump = vm.config.TxPoolPriceBump
	vm.ethConfig.TxPool.AccountSlots = vm.config.TxPoolAccountSlots
	vm.ethConfig.TxPool.GlobalSlots = vm.config.TxPoolGlobalSlots
	vm.ethConfig.TxPool.AccountQueue = vm.config.TxPoolAccountQueue
	vm.ethConfig.TxPool.GlobalQueue = vm.config.TxPoolGlobalQueue
	vm.ethConfig.TxPool.Lifetime = vm.config.TxPoolLifetime.Duration

	vm.ethConfig.AllowUnfinalizedQueries = vm.config.AllowUnfinalizedQueries
	vm.ethConfig.AllowUnprotectedTxs = vm.config.AllowUnprotectedTxs
	vm.ethConfig.AllowUnprotectedTxHashes = vm.config.AllowUnprotectedTxHashes
	vm.ethConfig.Preimages = vm.config.Preimages
	vm.ethConfig.Pruning = vm.config.Pruning
	vm.ethConfig.TrieCleanCache = vm.config.TrieCleanCache
	vm.ethConfig.TrieDirtyCache = vm.config.TrieDirtyCache
	vm.ethConfig.TrieDirtyCommitTarget = vm.config.TrieDirtyCommitTarget
	vm.ethConfig.TriePrefetcherParallelism = vm.config.TriePrefetcherParallelism
	vm.ethConfig.SnapshotCache = vm.config.SnapshotCache
	vm.ethConfig.AcceptorQueueLimit = vm.config.AcceptorQueueLimit
	vm.ethConfig.PopulateMissingTries = vm.config.PopulateMissingTries
	vm.ethConfig.PopulateMissingTriesParallelism = vm.config.PopulateMissingTriesParallelism
	vm.ethConfig.AllowMissingTries = vm.config.AllowMissingTries
	vm.ethConfig.SnapshotDelayInit = vm.config.StateSyncEnabled
	vm.ethConfig.SnapshotWait = vm.config.SnapshotWait
	vm.ethConfig.SnapshotVerify = vm.config.SnapshotVerify
	vm.ethConfig.HistoricalProofQueryWindow = vm.config.HistoricalProofQueryWindow
	vm.ethConfig.OfflinePruning = vm.config.OfflinePruning
	vm.ethConfig.OfflinePruningBloomFilterSize = vm.config.OfflinePruningBloomFilterSize
	vm.ethConfig.OfflinePruningDataDirectory = vm.config.OfflinePruningDataDirectory
	vm.ethConfig.CommitInterval = vm.config.CommitInterval
	vm.ethConfig.SkipUpgradeCheck = vm.config.SkipUpgradeCheck
	vm.ethConfig.AcceptedCacheSize = vm.config.AcceptedCacheSize
	vm.ethConfig.TransactionHistory = vm.config.TransactionHistory
	vm.ethConfig.SkipTxIndexing = vm.config.SkipTxIndexing

	// Create directory for offline pruning
	if len(vm.ethConfig.OfflinePruningDataDirectory) != 0 {
		if err := os.MkdirAll(vm.ethConfig.OfflinePruningDataDirectory, perms.ReadWriteExecute); err != nil {
			vm.logger.Error("failed to create offline pruning data directory", "error", err)
			return err
		}
	}

	// Handle custom fee recipient
	if common.IsHexAddress(vm.config.FeeRecipient) {
		address := common.HexToAddress(vm.config.FeeRecipient)
		vm.logger.Info("Setting fee recipient", "address", address)
		vm.ethConfig.Miner.Etherbase = address
	} else {
		vm.logger.Info("Config has not specified any coinbase address. Defaulting to the blackhole address.")
		vm.ethConfig.Miner.Etherbase = evmconstants.BlackholeAddr
	}

	vm.chainConfig = g.Config
	vm.networkID = vm.ethConfig.NetworkId

	// create genesisHash after applying upgradeBytes in case
	// upgradeBytes modifies genesis.
	vm.genesisHash = vm.ethConfig.Genesis.ToBlock().Hash() // must create genesis hash before [vm.readLastAccepted]
	lastAcceptedHash, lastAcceptedHeight, err := vm.readLastAccepted()
	if err != nil {
		return err
	}
	vm.logger.Info("read last accepted",
		"hash", lastAcceptedHash,
		"height", lastAcceptedHeight,
	)

	// initialize peer network
	if vm.p2pSender == nil {
		vm.p2pSender = appSender
	}

	// Wrap the AppSender to adapt between interfaces
	adaptedSender := newAppSenderAdapter(vm.p2pSender)
	
	// Get prometheus registry from Lux metrics for p2p network
	var p2pNetwork *p2p.Network
	if promRegistry, ok := nodeMetrics.GetPrometheusRegistry(vm.sdkMetrics); ok {
		p2pNetwork, err = p2p.NewNetwork(vm.ctx.Log, adaptedSender, promRegistry, "p2p")
		if err != nil {
			return fmt.Errorf("failed to initialize p2p network: %w", err)
		}
	} else {
		return fmt.Errorf("could not get prometheus registry for p2p network")
	}
	vm.p2pValidators = p2p.NewValidators(p2pNetwork.Peers, vm.ctx.Log, vm.ctx.SubnetID, vm.ctx.ValidatorState, maxValidatorSetStaleness)
	vm.networkCodec = message.Codec
	vm.Network = p2pNetwork
	// Create peer network wrapper (peer.NewNetwork expects core.AppSender, not appsender.AppSender)
	peerNetwork := peer.NewNetwork(p2pNetwork, vm.p2pSender, vm.networkCodec, vm.ctx.NodeID, 16)
	vm.client = peer.NewNetworkClient(peerNetwork)

	// Use the standard validators manager
	vm.validatorsManager = validators.NewManager()

	// Initialize warp backend
	offchainWarpMessages := make([][]byte, len(vm.config.WarpOffChainMessages))
	for i, hexMsg := range vm.config.WarpOffChainMessages {
		offchainWarpMessages[i] = []byte(hexMsg)
	}
	// TODO: Re-enable when warp backend is implemented
	// warpSignatureCache := lru.NewCache[ids.ID, []byte](warpSignatureCacheSize)
	// meteredCache, err := metercacher.New("warp_signature_cache", vm.sdkMetrics, warpSignatureCache)
	// if err != nil {
	// 	return fmt.Errorf("failed to create warp signature cache: %w", err)
	// }

	// clear warpdb on initialization if config enabled
	if vm.config.PruneWarpDB {
		if err := database.Clear(vm.warpDB, ethdb.IdealBatchSize); err != nil {
			return fmt.Errorf("failed to prune warpDB: %w", err)
		}
	}

	// TODO: Implement warp backend properly
	// backend.New doesn't exist, need to find proper constructor
	vm.warpBackend = nil

	if err := vm.initializeChain(lastAcceptedHash, vm.ethConfig); err != nil {
		return err
	}

	go vm.ctx.Log.RecoverAndPanic(vm.startContinuousProfiler)

	// TODO: Fix warp handler - vm.warpBackend doesn't implement lp118.Verifier
	// Commenting out for now since warp backend is nil
	// warpHandler := lp118.NewCachedHandler(meteredCache, vm.warpBackend, vm.ctx.WarpSigner)
	// vm.Network.AddHandler(p2p.SignatureRequestHandlerID, warpHandler)

	vm.setAppRequestHandlers()

	vm.StateSyncServer = NewStateSyncServer(&stateSyncServerConfig{
		Chain:            vm.blockChain,
		SyncableInterval: vm.config.StateSyncCommitInterval,
	})
	return vm.initializeStateSyncClient(lastAcceptedHeight)
}

func (vm *VM) initializeMetrics() error {
	// Create Lux metrics with prometheus backend
	vm.sdkMetrics = nodeMetrics.CreateLuxMetrics(sdkMetricsPrefix)
	
	// Type assert Metrics to MultiGatherer
	metricsGatherer, ok := vm.ctx.Metrics.(nodeMetrics.MultiGatherer)
	if !ok {
		return fmt.Errorf("metrics does not implement MultiGatherer")
	}
	
	// Get the prometheus registry from Lux metrics and register it
	if promRegistry, ok := nodeMetrics.GetPrometheusRegistry(vm.sdkMetrics); ok {
		return metricsGatherer.Register(sdkMetricsPrefix, promRegistry)
	}
	return fmt.Errorf("could not get prometheus registry from Lux metrics")
}

func (vm *VM) initializeChain(lastAcceptedHash common.Hash, ethConfig ethconfig.Config) error {
	nodecfg := &node.Config{
		Version:               Version,
		KeyStoreDir:           vm.config.KeystoreDirectory,
		ExternalSigner:        vm.config.KeystoreExternalSigner,
		InsecureUnlockAllowed: vm.config.KeystoreInsecureUnlockAllowed,
	}
	node, err := node.New(nodecfg)
	if err != nil {
		return err
	}
	vm.eth, err = eth.New(
		node,
		&vm.ethConfig,
		&EthPushGossiper{vm: vm},
		vm.chaindb,
		eth.Settings{MaxBlocksPerRequest: vm.config.MaxBlocksPerRequest},
		lastAcceptedHash,
		dummy.NewFakerWithClock(NewClockWrapper(vm.clock)),
		vm.clock,
	)
	if err != nil {
		return err
	}
	vm.eth.SetEtherbase(ethConfig.Miner.Etherbase)
	vm.txPool = vm.eth.TxPool()
	vm.blockChain = vm.eth.BlockChain()
	vm.miner = vm.eth.Miner()
	lastAccepted := vm.blockChain.LastAcceptedBlock()
	feeConfig, _, err := vm.blockChain.GetFeeConfigAtHeader(lastAccepted.Header())
	if err != nil {
		return err
	}
	// Set the minimum gas tip to the minimum base fee
	// Note: SetMinFee no longer exists in the new txpool implementation
	vm.txPool.SetGasTip(feeConfig.MinBaseFee)

	vm.eth.Start()
	return vm.initChainState(lastAccepted)
}

// initializeStateSyncClient initializes the client for performing state sync.
// If state sync is disabled, this function will wipe any ongoing summary from
// disk to ensure that we do not continue syncing from an invalid snapshot.
func (vm *VM) initializeStateSyncClient(lastAcceptedHeight uint64) error {
	// parse nodeIDs from state sync IDs in vm config
	var stateSyncIDs []ids.NodeID
	if vm.config.StateSyncEnabled && len(vm.config.StateSyncIDs) > 0 {
		nodeIDs := strings.Split(vm.config.StateSyncIDs, ",")
		stateSyncIDs = make([]ids.NodeID, len(nodeIDs))
		for i, nodeIDString := range nodeIDs {
			nodeID, err := ids.NodeIDFromString(nodeIDString)
			if err != nil {
				return fmt.Errorf("failed to parse %s as NodeID: %w", nodeIDString, err)
			}
			stateSyncIDs[i] = nodeID
		}
	}

	vm.StateSyncClient = NewStateSyncClient(&stateSyncClientConfig{
		chain: vm.eth,
		client: statesyncclient.NewClient(
			&statesyncclient.Config{
				SendRequest: func(ctx context.Context, peerID ids.NodeID, req []byte) ([]byte, error) {
					// SendAppRequestAny sends to any peer, so we use SendAppRequest for specific peer
					return vm.client.SendAppRequest(ctx, peerID, req)
				},
				Logger:           asGethLogger(vm.logger),
				Stats:            stats.NewClientSyncerStats(),
				StateSyncNodeIDs: stateSyncIDs,
				BlockParser:      vm,
			},
		),
		enabled:              vm.config.StateSyncEnabled,
		skipResume:           vm.config.StateSyncSkipResume,
		stateSyncMinBlocks:   vm.config.StateSyncMinBlocks,
		stateSyncRequestSize: vm.config.StateSyncRequestSize,
		lastAcceptedHeight:   lastAcceptedHeight, // TODO clean up how this is passed around
		chaindb:              vm.chaindb,
		metadataDB:           vm.metadataDB,
		acceptedBlockDB:      vm.acceptedBlockDB,
		db:                   &vm.db,
		toEngine:             vm.toEngine,
	})

	// If StateSync is disabled, clear any ongoing summary so that we will not attempt to resume
	// sync using a snapshot that has been modified by the node running normal operations.
	if !vm.config.StateSyncEnabled {
		return vm.StateSyncClient.ClearOngoingSummary()
	}

	return nil
}

func (vm *VM) initChainState(lastAcceptedBlock *types.Block) error {
	block := vm.newBlock(lastAcceptedBlock)

	// Create wrapper functions to adapt between chainblock.Block and consensuschain.Block
	getBlockWrapper := func(ctx context.Context, id ids.ID) (consensuschain.Block, error) {
		blk, err := vm.getBlock(ctx, id)
		if err != nil {
			return nil, err
		}
		// Our Block now implements consensuschain.Block interface properly
		return blk.(consensuschain.Block), nil
	}
	
	unmarshalBlockWrapper := func(ctx context.Context, b []byte) (consensuschain.Block, error) {
		blk, err := vm.parseBlock(ctx, b)
		if err != nil {
			return nil, err
		}
		return blk.(consensuschain.Block), nil
	}
	
	buildBlockWrapper := func(ctx context.Context) (consensuschain.Block, error) {
		blk, err := vm.BuildBlock(ctx)
		if err != nil {
			return nil, err
		}
		return blk.(consensuschain.Block), nil
	}
	
	buildBlockWithContextWrapper := func(ctx context.Context, blockCtx *chainblock.Context) (consensuschain.Block, error) {
		blk, err := vm.BuildBlockWithContext(ctx, blockCtx)
		if err != nil {
			return nil, err
		}
		return blk.(consensuschain.Block), nil
	}

	config := &chain.Config{
		DecidedCacheSize:      decidedCacheSize,
		MissingCacheSize:      missingCacheSize,
		UnverifiedCacheSize:   unverifiedCacheSize,
		BytesToIDCacheSize:    bytesToIDCacheSize,
		GetBlock:              getBlockWrapper,
		UnmarshalBlock:        unmarshalBlockWrapper,
		BuildBlock:            buildBlockWrapper,
		BuildBlockWithContext: buildBlockWithContextWrapper,
		LastAcceptedBlock:     block,
	}

	// Register chain state metrics using Lux metrics
	chainStateMetrics := nodeMetrics.CreateLuxMetrics(chainStateMetricsPrefix)
	
	// Get prometheus registry from Lux metrics for chain state
	var chainStateRegisterer prometheus.Registerer
	if promRegistry, ok := nodeMetrics.GetPrometheusRegistry(chainStateMetrics); ok {
		chainStateRegisterer = promRegistry
	} else {
		return fmt.Errorf("could not get prometheus registry for chain state metrics")
	}
	
	state, err := chain.NewMeteredState(chainStateRegisterer, config)
	if err != nil {
		return fmt.Errorf("could not create metered state: %w", err)
	}
	vm.State = state

	// Register the chain state metrics with the node's metrics gatherer
	metricsGatherer, ok := vm.ctx.Metrics.(nodeMetrics.MultiGatherer)
	if !ok {
		return fmt.Errorf("metrics does not implement MultiGatherer")
	}
	return metricsGatherer.Register(chainStateMetricsPrefix, chainStateRegisterer.(prometheus.Gatherer))
}

func (vm *VM) SetState(_ context.Context, state nodequasar.State) error {
	vm.vmLock.Lock()
	defer vm.vmLock.Unlock()
	switch state {
	case nodequasar.StateSyncing:
		vm.bootstrapped.Set(false)
		return nil
	case nodequasar.Bootstrapping:
		return vm.onBootstrapStarted()
	case nodequasar.NormalOp:
		return vm.onNormalOperationsStarted()
	default:
		return fmt.Errorf("unknown state: %v", state)
	}
}

// onBootstrapStarted marks this VM as bootstrapping
func (vm *VM) onBootstrapStarted() error {
	vm.bootstrapped.Set(false)
	if err := vm.StateSyncClient.Error(); err != nil {
		return err
	}
	// After starting bootstrapping, do not attempt to resume a previous state sync.
	if err := vm.StateSyncClient.ClearOngoingSummary(); err != nil {
		return err
	}
	// Ensure snapshots are initialized before bootstrapping (i.e., if state sync is skipped).
	// Note calling this function has no effect if snapshots are already initialized.
	vm.blockChain.InitializeSnapshots()

	return nil
}

// onNormalOperationsStarted marks this VM as bootstrapped
func (vm *VM) onNormalOperationsStarted() error {
	if vm.bootstrapped.Get() {
		return nil
	}
	vm.bootstrapped.Set(true)

	ctx, cancel := context.WithCancel(context.TODO())
	vm.cancel = cancel

	// TODO: Initialize validators manager properly
	// The standard validators manager doesn't have Initialize/DispatchSync methods

	// Initialize goroutines related to block building
	// once we enter normal operation as there is no need to handle mempool gossip before this point.
	ethTxGossipMarshaller := GossipEthTxMarshaller{}
	ethTxGossipClient := vm.Network.NewClient(p2p.TxGossipHandlerID, p2p.WithValidatorSampling(vm.p2pValidators))
	
	// Get prometheus registry from Lux metrics for gossip metrics
	var ethTxGossipMetrics gossip.Metrics
	if promRegistry, ok := nodeMetrics.GetPrometheusRegistry(vm.sdkMetrics); ok {
		var err error
		ethTxGossipMetrics, err = gossip.NewMetrics(promRegistry, ethTxGossipNamespace)
		if err != nil {
			return fmt.Errorf("failed to initialize eth tx gossip metrics: %w", err)
		}
	} else {
		return fmt.Errorf("could not get prometheus registry from Lux metrics")
	}
	
	ethTxPool, err := NewGossipEthTxPool(vm.txPool, vm.sdkMetrics)
	if err != nil {
		return fmt.Errorf("failed to initialize gossip eth tx pool: %w", err)
	}
	vm.shutdownWg.Add(1)
	go func() {
		ethTxPool.Subscribe(ctx)
		vm.shutdownWg.Done()
	}()

	// Use default gossip parameters since they're not in the config
	pushGossipParams := gossip.BranchingFactor{
		StakePercentage: 0.9,
		Validators:      100,
		Peers:           0,
	}
	pushRegossipParams := gossip.BranchingFactor{
		Validators: 10,
		Peers:      0,
	}

	ethTxPushGossiper := vm.ethTxPushGossiper.Get()
	if ethTxPushGossiper == nil {
		ethTxPushGossiper, err = gossip.NewPushGossiper[*GossipEthTx](
			ethTxGossipMarshaller,
			ethTxPool,
			vm.p2pValidators,
			ethTxGossipClient,
			ethTxGossipMetrics,
			pushGossipParams,
			pushRegossipParams,
			pushGossipDiscardedElements,
			txGossipTargetMessageSize,
			1*time.Minute, // default regossip frequency
		)
		if err != nil {
			return fmt.Errorf("failed to initialize eth tx push gossiper: %w", err)
		}
		vm.ethTxPushGossiper.Set(ethTxPushGossiper)
	}

	// NOTE: gossip network must be initialized first otherwise ETH tx gossip will not work.
	vm.builder = vm.NewBlockBuilder(vm.toEngine)
	vm.builder.awaitSubmittedTxs()

	if vm.ethTxGossipHandler == nil {
		vm.ethTxGossipHandler = gossip.NewHandler[*GossipEthTx](
			vm.ctx.Log,
			ethTxGossipMarshaller,
			ethTxPool,
			ethTxGossipMetrics,
			txGossipTargetMessageSize,
		)
	}

	if err := vm.Network.AddHandler(p2p.TxGossipHandlerID, vm.ethTxGossipHandler); err != nil {
		return fmt.Errorf("failed to add eth tx gossip handler: %w", err)
	}

	if vm.ethTxPullGossiper == nil {
		ethTxPullGossiper := gossip.NewPullGossiper[*GossipEthTx](
			vm.ctx.Log,
			ethTxGossipMarshaller,
			ethTxPool,
			ethTxGossipClient,
			ethTxGossipMetrics,
			txGossipPollSize,
		)

		vm.ethTxPullGossiper = gossip.ValidatorGossiper{
			Gossiper:   ethTxPullGossiper,
			NodeID:     vm.ctx.NodeID,
			Validators: vm.p2pValidators,
		}
	}

	vm.shutdownWg.Add(1)
	go func() {
		gossip.Every(ctx, vm.ctx.Log, ethTxPushGossiper, 100*time.Millisecond) // default push gossip frequency
		vm.shutdownWg.Done()
	}()
	vm.shutdownWg.Add(1)
	go func() {
		gossip.Every(ctx, vm.ctx.Log, vm.ethTxPullGossiper, 1*time.Second) // default pull gossip frequency
		vm.shutdownWg.Done()
	}()

	return nil
}

// setAppRequestHandlers sets the request handlers for the VM to serve state sync
// requests.
func (vm *VM) setAppRequestHandlers() {
	// Create standalone EVM TrieDB (read only) for serving leafs requests.
	// We create a standalone TrieDB here, so that it has a standalone cache from the one
	// used by the node when processing blocks.
	// Use geth's hashdb config
	evmTrieDB := triedb.NewDatabase(
		vm.chaindb,
		&triedb.Config{
			HashDB: &triedbhashdb.Config{
				CleanCacheSize: vm.config.StateSyncServerTrieCache * units.MiB,
			},
		},
	)

	networkHandler := newNetworkHandler(vm.blockChain, vm.chaindb, evmTrieDB, vm.warpBackend, newCodecWrapper(vm.networkCodec))
	// Use a custom handler ID for network requests
	const networkRequestHandlerID = 100
	p2pHandler := newP2PHandlerWrapper(networkHandler)
	if err := vm.Network.AddHandler(networkRequestHandlerID, p2pHandler); err != nil {
		vm.logger.Error("Failed to add network request handler", "error", err)
	}
}

// Shutdown implements the ChainVM interface
func (vm *VM) Shutdown(context.Context) error {
	vm.vmLock.Lock()
	defer vm.vmLock.Unlock()
	if vm.ctx == nil {
		return nil
	}
	if vm.cancel != nil {
		vm.cancel()
	}
	if vm.bootstrapped.Get() {
		// TODO: Shutdown validators manager properly
		// The standard validators manager doesn't have a Shutdown method
	}
	// TODO: Network shutdown - p2p.Network doesn't have Shutdown method
	if err := vm.StateSyncClient.Shutdown(); err != nil {
		vm.logger.Error("error stopping state syncer", "err", err)
	}
	close(vm.shutdownChan)
	// Stop RPC handlers before eth.Stop which will close the database
	for _, handler := range vm.rpcHandlers {
		handler.Stop()
	}
	vm.eth.Stop()
	vm.logger.Info("Ethereum backend stop completed")
	if vm.usingStandaloneDB {
		if err := vm.db.Close(); err != nil {
			vm.logger.Error("failed to close database", "error", err)
		} else {
			vm.logger.Info("Database closed")
		}
	}
	vm.shutdownWg.Wait()
	vm.logger.Info("EVM Shutdown completed")
	return nil
}

// BuildBlock builds a block to be wrapped by ChainState
func (vm *VM) BuildBlock(ctx context.Context) (chainblock.Block, error) {
	return vm.BuildBlockWithContext(ctx, nil)
}

func (vm *VM) BuildBlockWithContext(ctx context.Context, proposerVMBlockCtx *chainblock.Context) (chainblock.Block, error) {
	if proposerVMBlockCtx != nil {
		vm.logger.Debug("Building block with context", "pChainBlockHeight", proposerVMBlockCtx.PChainHeight)
	} else {
		vm.logger.Debug("Building block without context")
	}
	// Convert consensus context to commontype.ChainContext
	chainCtx := &commontype.ChainContext{
		NetworkID: vm.ctx.NetworkID,
		SubnetID:  evmids.SubnetID(vm.ctx.SubnetID),
		ChainID:   evmids.ChainID(vm.ctx.ChainID),
		NodeID: func() evmids.NodeID {
			var nodeID evmids.NodeID
			copy(nodeID[:], vm.ctx.NodeID[:])
			return nodeID
		}(),
		AppVersion: 0, // TODO: Get app version
		ChainDataDir: "", // TODO: Get chain data dir
		ValidatorState: nil, // TODO: Implement ValidatorState interface
	}

	// Convert block context if available
	var blockCtx *commontype.BlockContext
	if proposerVMBlockCtx != nil {
		blockCtx = &commontype.BlockContext{
			PChainHeight: proposerVMBlockCtx.PChainHeight,
		}
	}

	predicateCtx := &precompileconfig.PredicateContext{
		ConsensusCtx:       chainCtx,
		ProposerVMBlockCtx: blockCtx,
	}

	block, err := vm.miner.GenerateBlock(predicateCtx)
	vm.builder.handleGenerateBlock()
	if err != nil {
		return nil, err
	}

	// Note: the status of block is set by ChainState
	blk := vm.newBlock(block)

	// Verify is called on a non-wrapped block here, such that this
	// does not add [blk] to the processing blocks map in ChainState.
	//
	// TODO cache verification since Verify() will be called by the
	// consensus engine as well.
	//
	// Note: this is only called when building a new block, so caching
	// verification will only be a significant optimization for nodes
	// that produce a large number of blocks.
	// We call verify without writes here to avoid generating a reference
	// to the blk state root in the triedb when we are going to call verify
	// again from the consensus engine with writes enabled.
	if err := blk.verify(predicateCtx, false /*=writes*/); err != nil {
		return nil, fmt.Errorf("block failed verification due to: %w", err)
	}

	vm.logger.Debug("built block",
		"id", blk.ID(),
	)
	// Marks the current transactions from the mempool as being successfully issued
	// into a interfaces.
	return blk, nil
}

// ParseBlock parses [b] into a block.
func (vm *VM) ParseBlock(ctx context.Context, b []byte) (chainblock.Block, error) {
	return vm.parseBlock(ctx, b)
}

// parseBlock parses [b] into a block to be wrapped by ChainState.
func (vm *VM) parseBlock(_ context.Context, b []byte) (chainblock.Block, error) {
	ethBlock := new(types.Block)
	if err := rlp.DecodeBytes(b, ethBlock); err != nil {
		return nil, err
	}

	// Note: the status of block is set by ChainState
	block := vm.newBlock(ethBlock)
	// Performing syntactic verification in ParseBlock allows for
	// short-circuiting bad blocks before they are processed by the VM.
	if err := block.syntacticVerify(); err != nil {
		return nil, fmt.Errorf("syntactic block verification failed: %w", err)
	}
	return block, nil
}

func (vm *VM) ParseEthBlock(b []byte) (*types.Block, error) {
	block, err := vm.parseBlock(context.TODO(), b)
	if err != nil {
		return nil, err
	}

	// Type assert to *Block to get the ethBlock
	evmBlock, ok := block.(*Block)
	if !ok {
		return nil, fmt.Errorf("expected *Block but got %T", block)
	}
	return evmBlock.ethBlock, nil
}

// GetBlock attempts to retrieve block [id] from the VM.
func (vm *VM) GetBlock(ctx context.Context, id ids.ID) (chainblock.Block, error) {
	return vm.getBlock(ctx, id)
}

// getBlock attempts to retrieve block [id] from the VM to be wrapped
// by ChainState.
func (vm *VM) getBlock(_ context.Context, id ids.ID) (chainblock.Block, error) {
	ethBlock := vm.blockChain.GetBlockByHash(common.Hash(id))
	// If [ethBlock] is nil, return [database.ErrNotFound] here
	// so that the miss is considered cacheable.
	if ethBlock == nil {
		return nil, database.ErrNotFound
	}
	// Note: the status of block is set by ChainState
	return vm.newBlock(ethBlock), nil
}

// GetAcceptedBlock attempts to retrieve block [blkID] from the VM. This method
// only returns accepted blocks.
func (vm *VM) GetAcceptedBlock(ctx context.Context, blkID ids.ID) (chainblock.Block, error) {
	blk, err := vm.GetBlock(ctx, blkID)
	if err != nil {
		return nil, err
	}

	height := blk.Height()
	acceptedBlkID, err := vm.GetBlockIDAtHeight(ctx, height)
	if err != nil {
		return nil, err
	}

	if acceptedBlkID != blkID {
		// The provided block is not accepted.
		return nil, database.ErrNotFound
	}
	return blk, nil
}

// SetPreference sets what the current tail of the chain is
func (vm *VM) SetPreference(ctx context.Context, blkID ids.ID) error {
	// Since each internal handler used by [vm.State] always returns a block
	// with non-nil ethBlock value, GetBlockInternal should never return a
	// (*Block) with a nil ethBlock value.
	block, err := vm.GetBlockInternal(ctx, blkID)
	if err != nil {
		return fmt.Errorf("failed to set preference to %s: %w", blkID, err)
	}

	// Type assert to *Block to get the ethBlock
	evmBlock, ok := block.(*Block)
	if !ok {
		return fmt.Errorf("expected *Block but got %T", block)
	}
	return vm.blockChain.SetPreference(evmBlock.ethBlock)
}

// GetBlockIDAtHeight returns the canonical block at [height].
// Note: the engine assumes that if a block is not found at [height], then
// [database.ErrNotFound] will be returned. This indicates that the VM has state
// synced and does not have all historical blocks available.
func (vm *VM) GetBlockIDAtHeight(_ context.Context, height uint64) (ids.ID, error) {
	lastAcceptedBlock := vm.LastAcceptedBlock()
	if lastAcceptedBlock.Height() < height {
		return ids.ID{}, database.ErrNotFound
	}

	hash := vm.blockChain.GetCanonicalHash(height)
	if hash == (common.Hash{}) {
		return ids.ID{}, database.ErrNotFound
	}
	return ids.ID(hash), nil
}

func (vm *VM) Version(context.Context) (string, error) {
	return Version, nil
}

// NewHandler returns a new Handler for a service where:
//   - The handler's functionality is defined by [service]
//     [service] should be a gorilla RPC service (see https://www.gorillatoolkit.org/pkg/rpc/v2)
//   - The name of the service is [name]
func newHandler(name string, service interface{}) (http.Handler, error) {
	server := luxRPC.NewServer()
	codec := rpcjson.NewCodec()
	server.RegisterCodec(codec, "application/json")
	server.RegisterCodec(codec, "application/json;charset=UTF-8")
	return server, server.RegisterService(service, name)
}

// CreateHandlers makes new http handlers that can handle API calls
func (vm *VM) CreateHandlers(context.Context) (map[string]http.Handler, error) {
	handler := rpc.NewServer(vm.config.APIMaxDuration.Duration)
	// TODO: Add HttpBodyLimit to Config if needed
	// if vm.config.HttpBodyLimit > 0 {
	//	handler.SetHTTPBodyLimit(int(vm.config.HttpBodyLimit))
	// }

	enabledAPIs := vm.config.EthAPIs()
	// Convert geth rpc.API to our local rpc.API type
	gethAPIs := vm.eth.APIs()
	localAPIs := make([]rpc.API, len(gethAPIs))
	for i, api := range gethAPIs {
		localAPIs[i] = rpc.API{
			Namespace: api.Namespace,
			Version:   api.Version,
			Service:   api.Service,
			Name:      api.Namespace, // Use namespace as name
		}
	}
	if err := attachEthService(handler, localAPIs, enabledAPIs); err != nil {
		return nil, err
	}

	apis := make(map[string]http.Handler)
	if vm.config.AdminAPIEnabled {
		adminAPI, err := newHandler("admin", NewAdminService(vm, os.ExpandEnv(fmt.Sprintf("%s_subnet_evm_performance_%s", vm.config.AdminAPIDir, vm.chainAlias))))
		if err != nil {
			return nil, fmt.Errorf("failed to register service for admin API due to %w", err)
		}
		apis[adminEndpoint] = adminAPI
		enabledAPIs = append(enabledAPIs, "evm-admin")
	}

	if vm.config.ValidatorsAPIEnabled {
		validatorsAPI, err := newHandler("validators", &ValidatorsAPI{vm})
		if err != nil {
			return nil, fmt.Errorf("failed to register service for validators API due to %w", err)
		}
		apis[validatorsEndpoint] = validatorsAPI
		enabledAPIs = append(enabledAPIs, "validators")
	}

	// RPC APIs
	if vm.config.ChainAPIEnabled {
		if err := handler.RegisterName("linear", &ChainAPI{vm}); err != nil {
			return nil, err
		}
		enabledAPIs = append(enabledAPIs, "linear")
	}

	if vm.config.WarpAPIEnabled {
		// TODO: Implement warp API when the warp package is available
		// The warp API requires proper implementation of the warp service
		// if err := handler.RegisterName("warp", warp.NewAPI(vm.ctx.NetworkID, vm.ctx.SubnetID, vm.ctx.ChainID, vm.ctx.ValidatorState, vm.warpBackend, vm.client, vm.requirePrimaryNetworkSigners)); err != nil {
		//     return nil, err
		// }
		// enabledAPIs = append(enabledAPIs, "warp")
		vm.logger.Warn("Warp API is enabled but not implemented")
	}

	vm.logger.Info("enabling apis",
		"apis", enabledAPIs,
	)
	apis[ethRPCEndpoint] = handler
	apis[ethWSEndpoint] = handler.WebsocketHandlerWithDuration(
		[]string{"*"},
		vm.config.APIMaxDuration.Duration,
		vm.config.WSCPURefillRate.Duration,
		vm.config.WSCPUMaxStored.Duration,
	)

	vm.rpcHandlers = append(vm.rpcHandlers, handler)
	return apis, nil
}

// WaitForEvent implements a VM interface method
// TODO: determine proper Event type from node package
func (vm *VM) WaitForEvent(ctx context.Context) (commonEng.Message, error) {
	return commonEng.Message{}, fmt.Errorf("WaitForEvent not implemented")
}

func (vm *VM) NewHTTPHandler(ctx context.Context) (http.Handler, error) {
	return nil, nil
}

func (*VM) CreateHTTP2Handler(context.Context) (http.Handler, error) {
	return nil, nil
}

/*
 ******************************************************************************
 *********************************** Helpers **********************************
 ******************************************************************************
 */

// GetCurrentNonce returns the nonce associated with the address at the
// preferred block
func (vm *VM) GetCurrentNonce(address common.Address) (uint64, error) {
	// Note: current state uses the state of the preferred interfaces.
	state, err := vm.blockChain.State()
	if err != nil {
		return 0, err
	}
	return state.GetNonce(address), nil
}

func (vm *VM) chainConfigExtra() *extras.ChainConfig {
	return params.GetExtra(vm.chainConfig)
}

func (vm *VM) rules(number *big.Int, time uint64) extras.Rules {
	ethrules := vm.chainConfig.Rules(number, time)
	return *params.GetRulesExtra(ethrules)
}

// currentRules returns the chain rules for the current interfaces.
func (vm *VM) currentRules() extras.Rules {
	header := vm.eth.APIBackend.CurrentHeader()
	return vm.rules(header.Number, header.Time)
}

// requirePrimaryNetworkSigners returns true if warp messages from the primary
// network must be signed by the primary network interfaces.
// This is necessary when the subnet is not validating the primary network.
func (vm *VM) requirePrimaryNetworkSigners() bool {
	switch c := vm.currentRules().Precompiles[warp.ContractAddress].(type) {
	case *warp.Config:
		return c.RequirePrimaryNetworkSigners
	default: // includes nil due to non-presence
		return false
	}
}

func (vm *VM) startContinuousProfiler() {
	// If the profiler directory is empty, return immediately
	// without creating or starting a continuous interfaces.
	if vm.config.ContinuousProfilerDir == "" {
		return
	}
	vm.profiler = profiler.NewContinuous(
		filepath.Join(vm.config.ContinuousProfilerDir),
		vm.config.ContinuousProfilerFrequency.Duration,
		vm.config.ContinuousProfilerMaxFiles,
	)
	defer vm.profiler.Shutdown()

	vm.shutdownWg.Add(1)
	go func() {
		defer vm.shutdownWg.Done()
		vm.logger.Info("Dispatching continuous profiler", "dir", vm.config.ContinuousProfilerDir, "freq", vm.config.ContinuousProfilerFrequency, "maxFiles", vm.config.ContinuousProfilerMaxFiles)
		err := vm.profiler.Dispatch()
		if err != nil {
			vm.logger.Error("continuous profiler failed", "err", err)
		}
	}()
	// Wait for shutdownChan to be closed
	<-vm.shutdownChan
}

// readLastAccepted reads the last accepted hash from [acceptedBlockDB] and returns the
// last accepted block hash and height by reading directly from [vm.chaindb] instead of relying
// on [chain].
// Note: assumes [vm.chaindb] and [vm.genesisHash] have been initialized.
func (vm *VM) readLastAccepted() (common.Hash, uint64, error) {
	// Attempt to load last accepted block to determine if it is necessary to
	// initialize state with the genesis interfaces.
	lastAcceptedBytes, lastAcceptedErr := vm.acceptedBlockDB.Get(lastAcceptedKey)
	switch {
	case lastAcceptedErr == database.ErrNotFound:
		// If there is nothing in the database, return the genesis block hash and height
		return vm.genesisHash, 0, nil
	case lastAcceptedErr != nil:
		return common.Hash{}, 0, fmt.Errorf("failed to get last accepted block ID due to: %w", lastAcceptedErr)
	case len(lastAcceptedBytes) != common.HashLength:
		return common.Hash{}, 0, fmt.Errorf("last accepted bytes should have been length %d, but found %d", common.HashLength, len(lastAcceptedBytes))
	default:
		lastAcceptedHash := common.BytesToHash(lastAcceptedBytes)
		height := rawdb.ReadHeaderNumber(vm.chaindb, lastAcceptedHash)
		if height == nil {
			return common.Hash{}, 0, fmt.Errorf("failed to retrieve header number of last accepted block: %s", lastAcceptedHash)
		}
		return lastAcceptedHash, *height, nil
	}
}

// attachEthService registers the backend RPC services provided by Ethereum
// to the provided handler under their assigned namespaces.
func attachEthService(handler *rpc.Server, apis []rpc.API, names []string) error {
	enabledServicesSet := make(map[string]struct{})
	for _, ns := range names {
		enabledServicesSet[ns] = struct{}{}
	}

	apiSet := make(map[string]rpc.API)
	for _, api := range apis {
		if existingAPI, exists := apiSet[api.Name]; exists {
			return fmt.Errorf("duplicated API name: %s, namespaces %s and %s", api.Name, api.Namespace, existingAPI.Namespace)
		}
		apiSet[api.Name] = api
	}

	for name := range enabledServicesSet {
		api, exists := apiSet[name]
		if !exists {
			return fmt.Errorf("API service %s not found", name)
		}
		if err := handler.RegisterName(api.Namespace, api.Service); err != nil {
			return err
		}
	}

	return nil
}

// getMandatoryNetworkUpgrades returns the mandatory network upgrades for the specified network ID,
// along with a flag that indicates if returned upgrades should be strictly enforced.
func getMandatoryNetworkUpgrades(networkID uint32) (params.MandatoryNetworkUpgrades, bool) {
	switch networkID {
	case constants.MainnetID:
		return params.MainnetNetworkUpgrades, true
	case constants.TestnetID:
		return params.MainnetNetworkUpgrades, true // TODO: Define TestnetNetworkUpgrades
	case constants.UnitTestID:
		return params.MainnetNetworkUpgrades, false // TODO: Define UnitTestNetworkUpgrades
	default:
		return params.MainnetNetworkUpgrades, false // TODO: Define LocalNetworkUpgrades
	}
}

// Connected handles new peer connections
func (vm *VM) Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion *version.Application) error {
	vm.vmLock.Lock()
	defer vm.vmLock.Unlock()

	// TODO: Implement validator connection tracking
	// validators.Manager doesn't have a Connect method

	return vm.Network.Connected(ctx, nodeID, nodeVersion)
}

func (vm *VM) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	vm.vmLock.Lock()
	defer vm.vmLock.Unlock()

	// TODO: Implement validator disconnection tracking
	// validators.Manager doesn't have a Disconnect method

	return vm.Network.Disconnected(ctx, nodeID)
}

// AppGossip handles incoming gossip messages
func (vm *VM) AppGossip(ctx context.Context, nodeID ids.NodeID, msg []byte) error {
	// For now, we don't handle app gossip messages
	return nil
}

// AppRequest handles incoming app requests
func (vm *VM) AppRequest(ctx context.Context, nodeID ids.NodeID, requestID uint32, deadline time.Time, request []byte) error {
	// For now, we don't handle app requests
	return nil
}

// AppResponse handles incoming app responses
func (vm *VM) AppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
	// For now, we don't handle app responses
	return nil
}

// AppRequestFailed handles failed app requests
func (vm *VM) AppRequestFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32, appErr *commonEng.AppError) error {
	// For now, we don't handle failed app requests
	return nil
}

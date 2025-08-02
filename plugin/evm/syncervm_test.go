// (c) 2021-2022, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/evm/v2/v2/consensus/dummy"
	"github.com/luxfi/evm/v2/v2/constants"
	"github.com/luxfi/evm/v2/v2/core"
	"github.com/luxfi/evm/v2/v2/core/rawdb"
	"github.com/luxfi/evm/v2/v2/core/types"
	"github.com/luxfi/evm/v2/v2/utils/utilstest"
	"github.com/luxfi/evm/v2/v2/params"
	"github.com/luxfi/evm/v2/v2/plugin/evm/customrawdb"
	"github.com/luxfi/evm/v2/v2/plugin/evm/database"
	"github.com/luxfi/evm/v2/v2/predicate"
	"github.com/luxfi/evm/v2/v2/sync/statesync/statesynctest"
	"github.com/luxfi/geth/common"
	gethtypes "github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/log"
	"github.com/luxfi/geth/rlp"
	"github.com/luxfi/geth/trie"
	"github.com/luxfi/geth/triedb"
	"github.com/luxfi/node/quasar"
	"github.com/luxfi/node/quasar/engine/chain/block"
	commonEng "github.com/luxfi/node/quasar/engine/core"
	"github.com/luxfi/node/quasar/consensus/engine/enginetest"
	"github.com/luxfi/node/version"
	nodedatabase "github.com/luxfi/database"
	"github.com/luxfi/database/prefixdb"
	"github.com/luxfi/ids"
	"github.com/luxfi/metrics"
	statesyncclient "github.com/luxfi/node/state_sync/client"
	"github.com/luxfi/node/utils/set"
	"github.com/stretchr/testify/assert"
	"github.com/luxfi/evm/v2/v2/utils"
	"github.com/stretchr/testify/require"
)

// testRegistry is a simple no-op metrics registry for testing
type testRegistry struct{}

func (t *testRegistry) Register(c metrics.Collector) error          { return nil }
func (t *testRegistry) MustRegister(c metrics.Collector)            {}
func (t *testRegistry) Unregister(c metrics.Collector) bool         { return true }
func (t *testRegistry) Gather() ([]*metrics.MetricFamily, error)    { return nil, nil }

// testAppSender is a test implementation of commonEng.AppSender
type testAppSender struct {
	SendAppRequestF            func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, msg []byte) error
	SendAppResponseF           func(ctx context.Context, nodeID ids.NodeID, requestID uint32, msg []byte) error
	SendAppErrorF              func(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error
	SendAppGossipF             func(ctx context.Context, msg []byte) error
	SendCrossChainAppRequestF  func(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error
	SendCrossChainAppResponseF func(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error
	SendCrossChainAppErrorF    func(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error
}

func (t *testAppSender) SendAppRequest(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, msg []byte) error {
	if t.SendAppRequestF != nil {
		return t.SendAppRequestF(ctx, nodeIDs, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, msg []byte) error {
	if t.SendAppResponseF != nil {
		return t.SendAppResponseF(ctx, nodeID, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendAppError(ctx context.Context, nodeID ids.NodeID, requestID uint32, errorCode int32, errorMessage string) error {
	if t.SendAppErrorF != nil {
		return t.SendAppErrorF(ctx, nodeID, requestID, errorCode, errorMessage)
	}
	return nil
}

func (t *testAppSender) SendAppGossip(ctx context.Context, msg []byte) error {
	if t.SendAppGossipF != nil {
		return t.SendAppGossipF(ctx, msg)
	}
	return nil
}

func (t *testAppSender) SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error {
	if t.SendCrossChainAppRequestF != nil {
		return t.SendCrossChainAppRequestF(ctx, chainID, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, msg []byte) error {
	if t.SendCrossChainAppResponseF != nil {
		return t.SendCrossChainAppResponseF(ctx, chainID, requestID, msg)
	}
	return nil
}

func (t *testAppSender) SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error {
	if t.SendCrossChainAppErrorF != nil {
		return t.SendCrossChainAppErrorF(ctx, chainID, requestID, errorCode, errorMessage)
	}
	return nil
}

func TestSkipStateSync(t *testing.T) {
	rand.Seed(1)
	test := syncTest{
		syncableInterval:   256,
		stateSyncMinBlocks: 300, // must be greater than [syncableInterval] to skip sync
		syncMode:           block.StateSyncSkipped,
	}
	vmSetup := createSyncServerAndClientVMs(t, test, parentsToGet)

	testSyncerVM(t, vmSetup, test)
}

func TestStateSyncFromScratch(t *testing.T) {
	rand.Seed(1)
	test := syncTest{
		syncableInterval:   256,
		stateSyncMinBlocks: 50, // must be less than [syncableInterval] to perform sync
		syncMode:           block.StateSyncStatic,
	}
	vmSetup := createSyncServerAndClientVMs(t, test, parentsToGet)

	testSyncerVM(t, vmSetup, test)
}

func TestStateSyncFromScratchExceedParent(t *testing.T) {
	rand.Seed(1)
	numToGen := parentsToGet + uint64(32)
	test := syncTest{
		syncableInterval:   numToGen,
		stateSyncMinBlocks: 50, // must be less than [syncableInterval] to perform sync
		syncMode:           block.StateSyncStatic,
	}
	vmSetup := createSyncServerAndClientVMs(t, test, int(numToGen))

	testSyncerVM(t, vmSetup, test)
}

func TestStateSyncToggleEnabledToDisabled(t *testing.T) {
	rand.Seed(1)

	var lock sync.Mutex
	reqCount := 0
	test := syncTest{
		syncableInterval:   256,
		stateSyncMinBlocks: 50, // must be less than [syncableInterval] to perform sync
		syncMode:           block.StateSyncStatic,
		responseIntercept: func(syncerVM *VM, nodeID ids.NodeID, requestID uint32, response []byte) {
			lock.Lock()
			defer lock.Unlock()

			reqCount++
			// Fail all requests after number 50 to interrupt the sync
			if reqCount > 50 {
				if err := syncerVM.AppRequestFailed(context.Background(), nodeID, requestID, commonEng.ErrTimeout); err != nil {
					panic(err)
				}
				cancel := syncerVM.StateSyncClient.(*stateSyncerClient).cancel
				if cancel != nil {
					cancel()
				} else {
					t.Fatal("state sync client not populated correctly")
				}
			} else {
				syncerVM.AppResponse(context.Background(), nodeID, requestID, response)
			}
		},
		expectedErr: context.Canceled,
	}
	vmSetup := createSyncServerAndClientVMs(t, test, parentsToGet)

	// Perform sync resulting in early termination.
	testSyncerVM(t, vmSetup, test)

	test.syncMode = block.StateSyncStatic
	test.responseIntercept = nil
	test.expectedErr = nil

	syncDisabledVM := &VM{}
	// Create a custom app sender that implements the simpler interface
	appSender := &testAppSender{
		SendAppGossipF: func(context.Context, []byte) error { return nil },
		SendAppRequestF: func(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, request []byte) error {
			if len(nodeIDs) == 0 {
				t.Fatal("expected at least 1 nodeID")
			}
			nodeID := nodeIDs[0]
			go vmSetup.serverVM.AppRequest(ctx, nodeID, requestID, time.Now().Add(1*time.Second), request)
			return nil
		},
	}
	// Reset metrics to allow re-initialization
	vmSetup.syncerVM.ctx.Metrics = &testRegistry{}
	stateSyncDisabledConfigJSON := `{"state-sync-enabled":false}`
	if err := syncDisabledVM.Initialize(
		context.Background(),
		vmSetup.syncerVM.ctx,
		vmSetup.syncerDB,
		[]byte(genesisJSONLatest),
		nil,
		[]byte(stateSyncDisabledConfigJSON),
		[]*commonEng.Fx{},
		appSender,
	); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if err := syncDisabledVM.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	if height := syncDisabledVM.LastAcceptedBlockInternal().Height(); height != 0 {
		t.Fatalf("Unexpected last accepted height: %d", height)
	}

	enabled, err := syncDisabledVM.StateSyncEnabled(context.Background())
	assert.NoError(t, err)
	assert.False(t, enabled, "sync should be disabled")

	// Process the first 10 blocks from the serverVM
	for i := uint64(1); i < 10; i++ {
		ethBlock := vmSetup.serverVM.blockChain.GetBlockByNumber(i)
		if ethBlock == nil {
			t.Fatalf("VM Server did not have a block available at height %d", i)
		}
		b, err := rlp.EncodeToBytes(ethBlock)
		if err != nil {
			t.Fatal(err)
		}
		blk, err := syncDisabledVM.ParseBlock(context.Background(), b)
		if err != nil {
			t.Fatal(err)
		}
		if err := blk.Verify(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := blk.Accept(); err != nil {
			t.Fatal(err)
		}
	}
	// Verify the snapshot disk layer matches the last block root
	lastRoot := syncDisabledVM.blockChain.CurrentBlock().Root
	if err := syncDisabledVM.blockChain.Snapshots().Verify(lastRoot); err != nil {
		t.Fatal(err)
	}
	syncDisabledVM.blockChain.DrainAcceptorQueue()

	// Create a new VM from the same database with state sync enabled.
	syncReEnabledVM := &VM{}
	// Enable state sync in configJSON
	configJSON := fmt.Sprintf(
		`{"state-sync-enabled":true, "state-sync-min-blocks":%d}`,
		test.stateSyncMinBlocks,
	)
	// Reset metrics to allow re-initialization
	vmSetup.syncerVM.ctx.Metrics = &testRegistry{}
	if err := syncReEnabledVM.Initialize(
		context.Background(),
		vmSetup.syncerVM.ctx,
		vmSetup.syncerDB,
		[]byte(genesisJSONLatest),
		nil,
		[]byte(configJSON),
		[]*commonEng.Fx{},
		appSender,
	); err != nil {
		t.Fatal(err)
	}

	// override [serverVM]'s SendAppResponse function to trigger AppResponse on [syncerVM]
	vmSetup.serverAppSender.SendAppResponseF = func(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
		if test.responseIntercept == nil {
			go syncReEnabledVM.AppResponse(ctx, nodeID, requestID, response)
		} else {
			go test.responseIntercept(syncReEnabledVM, nodeID, requestID, response)
		}

		return nil
	}

	// connect peer to [syncerVM]
	testVersion := &version.Application{
		Name:  "test",
		Major: 1,
		Minor: 0,
		Patch: 0,
	}
	assert.NoError(t, syncReEnabledVM.Connected(
		context.Background(),
		vmSetup.serverVM.ctx.NodeID,
		testVersion,
	))

	enabled, err = syncReEnabledVM.StateSyncEnabled(context.Background())
	assert.NoError(t, err)
	assert.True(t, enabled, "sync should be enabled")

	vmSetup.syncerVM = syncReEnabledVM
	testSyncerVM(t, vmSetup, test)
}

func TestVMShutdownWhileSyncing(t *testing.T) {
	var (
		lock    sync.Mutex
		vmSetup *syncVMSetup
	)
	reqCount := 0
	test := syncTest{
		syncableInterval:   256,
		stateSyncMinBlocks: 50, // must be less than [syncableInterval] to perform sync
		syncMode:           block.StateSyncStatic,
		responseIntercept: func(syncerVM *VM, nodeID ids.NodeID, requestID uint32, response []byte) {
			lock.Lock()
			defer lock.Unlock()

			reqCount++
			// Shutdown the VM after 50 requests to interrupt the sync
			if reqCount == 50 {
				// Note this verifies the VM shutdown does not time out while syncing.
				require.NoError(t, vmSetup.shutdownOnceSyncerVM.Shutdown(context.Background()))
			} else if reqCount < 50 {
				err := syncerVM.AppResponse(context.Background(), nodeID, requestID, response)
				require.NoError(t, err)
			}
		},
		expectedErr: context.Canceled,
	}
	vmSetup = createSyncServerAndClientVMs(t, test, parentsToGet)
	// Perform sync resulting in early termination.
	testSyncerVM(t, vmSetup, test)
}

func createSyncServerAndClientVMs(t *testing.T, test syncTest, numBlocks int) *syncVMSetup {
	require := require.New(t)
	// configure [serverVM]
	_, serverVM, _, serverAppSender := GenesisVM(t, true, genesisJSONLatest, "", "")
	t.Cleanup(func() {
		log.Info("Shutting down server VM")
		require.NoError(serverVM.Shutdown(context.Background()))
	})
	generateAndAcceptBlocks(t, serverVM, numBlocks, func(i int, gen *core.BlockGen) {
		b, err := predicate.NewResults().Bytes()
		if err != nil {
			t.Fatal(err)
		}
		gen.AppendExtra(b)

		tx := types.NewTransaction(gen.TxNonce(testEthAddrs[0]), testEthAddrs[1], common.Big1, params.TxGas, big.NewInt(testMinGasPrice), nil)
		signedTx, err := types.SignTx(tx, types.NewEIP155Signer(serverVM.chainConfig.ChainID), testKeys[0])
		require.NoError(err)
		gen.AddTx(signedTx)
	}, nil)

	// make some accounts
	trieDB := triedb.NewDatabase(serverVM.chaindb, nil)
	root, accounts := statesynctest.FillAccountsWithOverlappingStorage(t, trieDB, types.EmptyRootHash, 1000, 16)

	// patch serverVM's lastAcceptedBlock to have the new root
	// and update the vm's state so the trie with accounts will
	// be returned by StateSyncGetLastSummary
	lastAccepted := serverVM.blockChain.LastAcceptedBlock()
	patchedBlock := patchBlock(lastAccepted, root, serverVM.chaindb)
	blockBytes, err := rlp.EncodeToBytes(patchedBlock)
	require.NoError(err)
	internalBlock, err := serverVM.parseBlock(context.Background(), blockBytes)
	require.NoError(err)
	require.NoError(serverVM.State.SetLastAcceptedBlock(internalBlock))

	// patch syncableInterval for test
	serverVM.StateSyncServer.(*stateSyncServer).syncableInterval = test.syncableInterval

	// initialise [syncerVM] with blank genesis state
	stateSyncEnabledJSON := fmt.Sprintf(`{"state-sync-enabled":true, "state-sync-min-blocks": %d, "tx-lookup-limit": %d}`, test.stateSyncMinBlocks, 4)
	syncerEngineChan, syncerVM, syncerDB, syncerAppSender := GenesisVM(t, false, genesisJSONLatest, stateSyncEnabledJSON, "")
	shutdownOnceSyncerVM := &shutdownOnceVM{VM: syncerVM}
	t.Cleanup(func() {
		require.NoError(shutdownOnceSyncerVM.Shutdown(context.Background()))
	})
	require.NoError(syncerVM.SetState(context.Background(), quasar.StateSyncing))
	enabled, err := syncerVM.StateSyncEnabled(context.Background())
	require.NoError(err)
	require.True(enabled)

	// override [serverVM]'s SendAppResponse function to trigger AppResponse on [syncerVM]
	serverAppSender.SendAppResponseF = func(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error {
		if test.responseIntercept == nil {
			go syncerVM.AppResponse(ctx, nodeID, requestID, response)
		} else {
			go test.responseIntercept(syncerVM, nodeID, requestID, response)
		}

		return nil
	}

	// connect peer to [syncerVM]
	testVersion := &version.Application{
		Name:  "test",
		Major: 1,
		Minor: 0,
		Patch: 0,
	}
	require.NoError(
		syncerVM.Connected(
			context.Background(),
			serverVM.ctx.NodeID,
			testVersion,
		),
	)

	// override [syncerVM]'s SendAppRequest function to trigger AppRequest on [serverVM]
	syncerAppSender.SendAppRequestF = func(ctx context.Context, nodeSet set.Set[ids.NodeID], requestID uint32, request []byte) error {
		nodeID, hasItem := nodeSet.Pop()
		require.True(hasItem, "expected nodeSet to contain at least 1 nodeID")
		err := serverVM.AppRequest(ctx, nodeID, requestID, time.Now().Add(1*time.Second), request)
		require.NoError(err)
		return nil
	}

	return &syncVMSetup{
		serverVM:             serverVM,
		serverAppSender:      serverAppSender,
		fundedAccounts:       accounts,
		syncerVM:             syncerVM,
		syncerDB:             syncerDB,
		syncerEngineChan:     syncerEngineChan,
		shutdownOnceSyncerVM: shutdownOnceSyncerVM,
	}
}

// syncVMSetup contains the required set up for a client VM to perform state sync
// off of a server VM.
type syncVMSetup struct {
	serverVM        *VM
	serverAppSender *enginetest.Sender

	fundedAccounts map[*utilstest.Key]*gethtypes.StateAccount

	syncerVM             *VM
	syncerDB             nodedatabase.Database
	syncerEngineChan     <-chan commonEng.Message
	shutdownOnceSyncerVM *shutdownOnceVM
}

type shutdownOnceVM struct {
	*VM
	shutdownOnce sync.Once
}

func (vm *shutdownOnceVM) Shutdown(ctx context.Context) error {
	var err error
	vm.shutdownOnce.Do(func() { err = vm.VM.Shutdown(ctx) })
	return err
}

// syncTest contains both the actual VMs as well as the parameters with the expected output.
type syncTest struct {
	responseIntercept  func(vm *VM, nodeID ids.NodeID, requestID uint32, response []byte)
	stateSyncMinBlocks uint64
	syncableInterval   uint64
	syncMode           block.StateSyncMode
	expectedErr        error
}

func testSyncerVM(t *testing.T, vmSetup *syncVMSetup, test syncTest) {
	t.Helper()
	var (
		require          = require.New(t)
		serverVM         = vmSetup.serverVM
		fundedAccounts   = vmSetup.fundedAccounts
		syncerVM         = vmSetup.syncerVM
		// syncerEngineChan = vmSetup.syncerEngineChan // No longer used in new architecture
	)
	// get last summary and test related methods
	summary, err := serverVM.GetLastStateSummary(context.Background())
	require.NoError(err, "error getting state sync last summary")
	parsedSummary, err := syncerVM.ParseStateSummary(context.Background(), summary.Bytes())
	require.NoError(err, "error parsing state summary")
	retrievedSummary, err := serverVM.GetStateSummary(context.Background(), parsedSummary.Height())
	require.NoError(err, "error getting state sync summary at height")
	require.Equal(summary, retrievedSummary)

	syncMode, err := parsedSummary.Accept(context.Background())
	require.NoError(err, "error accepting state summary")
	require.Equal(test.syncMode, syncMode)
	if syncMode == block.StateSyncSkipped {
		return
	}

	// TODO: State sync completion notification has changed in the new architecture
	// The engine no longer sends StateSyncDone messages
	// msg := <-syncerEngineChan
	// require.Equal(interfaces.StateSyncDone, msg)

	// If the test is expected to error, assert the correct error is returned and finish the test.
	err = syncerVM.StateSyncClient.Error()
	if test.expectedErr != nil {
		require.ErrorIs(err, test.expectedErr)
		// Note we re-open the database here to avoid a closed error when the test is for a shutdown VM.
		// Create a prefixed database directly with the chainDB we have
		chaindb := syncerVM.chaindb
		assertSyncPerformedHeights(t, chaindb, map[uint64]struct{}{})
		return
	}
	require.NoError(err, "state sync failed")

	// set [syncerVM] to bootstrapping and verify the last accepted block has been updated correctly
	// and that we can bootstrap and process some blocks.
	require.NoError(syncerVM.SetState(context.Background(), quasar.Bootstrapping))
	require.Equal(serverVM.LastAcceptedBlock().Height(), syncerVM.LastAcceptedBlock().Height(), "block height mismatch between syncer and server")
	require.Equal(serverVM.LastAcceptedBlock().ID(), syncerVM.LastAcceptedBlock().ID(), "blockID mismatch between syncer and server")
	require.True(syncerVM.blockChain.HasState(syncerVM.blockChain.LastAcceptedBlock().Root()), "unavailable state for last accepted block")
	assertSyncPerformedHeights(t, syncerVM.chaindb, map[uint64]struct{}{retrievedSummary.Height(): {}})

	lastNumber := syncerVM.blockChain.LastAcceptedBlock().NumberU64()
	// check the last block is indexed
	lastSyncedBlock := rawdb.ReadBlock(syncerVM.chaindb, rawdb.ReadCanonicalHash(syncerVM.chaindb, lastNumber), lastNumber)
	for _, tx := range lastSyncedBlock.Transactions() {
		index := rawdb.ReadTxLookupEntry(syncerVM.chaindb, tx.Hash())
		require.NotNilf(index, "Miss transaction indices, number %d hash %s", lastNumber, tx.Hash().Hex())
	}

	// tail should be the last block synced
	if syncerVM.ethConfig.TransactionHistory != 0 {
		tail := lastSyncedBlock.NumberU64()

		core.CheckTxIndices(t, &tail, tail, syncerVM.chaindb, true)
	}

	blocksToBuild := 10
	txsPerBlock := 10
	toAddress := testEthAddrs[1] // arbitrary choice
	generateAndAcceptBlocks(t, syncerVM, blocksToBuild, func(_ int, gen *core.BlockGen) {
		b, err := predicate.NewResults().Bytes()
		if err != nil {
			t.Fatal(err)
		}
		gen.AppendExtra(b)
		i := 0
		for k := range fundedAccounts {
			tx := types.NewTransaction(gen.TxNonce(common.BytesToAddress(k.Address.Bytes())), toAddress, big.NewInt(1), 21000, big.NewInt(testMinGasPrice), nil)
			signedTx, err := types.SignTx(tx, types.NewEIP155Signer(serverVM.chainConfig.ChainID), k.PrivateKey)
			require.NoError(err)
			gen.AddTx(signedTx)
			i++
			if i >= txsPerBlock {
				break
			}
		}
	},
		func(block *types.Block) {
			if syncerVM.ethConfig.TransactionHistory != 0 {
				tail := block.NumberU64() - syncerVM.ethConfig.TransactionHistory + 1
				// tail should be the minimum last synced block, since we skipped it to the last block
				if tail < lastSyncedBlock.NumberU64() {
					tail = lastSyncedBlock.NumberU64()
				}
				core.CheckTxIndices(t, &tail, block.NumberU64(), syncerVM.chaindb, true)
			}
		},
	)

	// check we can transition to [NormalOp] state and continue to process blocks.
	require.NoError(syncerVM.SetState(context.Background(), quasar.NormalOp))
	require.True(syncerVM.bootstrapped.Get())

	// Generate blocks after we have entered normal consensus as well
	generateAndAcceptBlocks(t, syncerVM, blocksToBuild, func(_ int, gen *core.BlockGen) {
		b, err := predicate.NewResults().Bytes()
		require.NoError(err)
		gen.AppendExtra(b)
		i := 0
		for k := range fundedAccounts {
			tx := types.NewTransaction(gen.TxNonce(common.BytesToAddress(k.Address.Bytes())), toAddress, big.NewInt(1), 21000, big.NewInt(testMinGasPrice), nil)
			signedTx, err := types.SignTx(tx, types.NewEIP155Signer(serverVM.chainConfig.ChainID), k.PrivateKey)
			require.NoError(err)
			gen.AddTx(signedTx)
			i++
			if i >= txsPerBlock {
				break
			}
		}
	},
		func(block *types.Block) {
			if syncerVM.ethConfig.TransactionHistory != 0 {
				tail := block.NumberU64() - syncerVM.ethConfig.TransactionHistory + 1
				// tail should be the minimum last synced block, since we skipped it to the last block
				if tail < lastSyncedBlock.NumberU64() {
					tail = lastSyncedBlock.NumberU64()
				}
				core.CheckTxIndices(t, &tail, block.NumberU64(), syncerVM.chaindb, true)
			}
		},
	)
}

// patchBlock returns a copy of [blk] with [root] and updates [db] to
// include the new block as canonical for [blk]'s height.
// This breaks the digestibility of the chain since after this call
// [blk] does not necessarily define a state transition from its parent
// state to the new state root.
func patchBlock(blk *types.Block, root common.Hash, db ethdb.Database) *types.Block {
	header := blk.Header()
	header.Root = root
	receipts := rawdb.ReadRawReceipts(db, blk.Hash(), blk.NumberU64())
	newBlk := types.NewBlock(
		header, blk.Transactions(), blk.Uncles(), receipts, trie.NewStackTrie(nil),
	)
	rawdb.WriteBlock(db, newBlk)
	rawdb.WriteCanonicalHash(db, newBlk.Hash(), newBlk.NumberU64())
	return newBlk
}

// generateAndAcceptBlocks uses [core.GenerateChain] to generate blocks, then
// calls Verify and Accept on each generated block
// TODO: consider using this helper function in vm_test.go and elsewhere in this package to clean up tests
func generateAndAcceptBlocks(t *testing.T, vm *VM, numBlocks int, gen func(int, *core.BlockGen), accepted func(*types.Block)) {
	t.Helper()

	// acceptExternalBlock defines a function to parse, verify, and accept a block once it has been
	// generated by GenerateChain
	acceptExternalBlock := func(block *types.Block) {
		bytes, err := rlp.EncodeToBytes(block)
		if err != nil {
			t.Fatal(err)
		}
		vmBlock, err := vm.ParseBlock(context.Background(), bytes)
		if err != nil {
			t.Fatal(err)
		}
		if err := vmBlock.Verify(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := vmBlock.Accept(); err != nil {
			t.Fatal(err)
		}

		if accepted != nil {
			accepted(block)
		}
	}
	_, _, err := core.GenerateChain(
		vm.chainConfig,
		vm.blockChain.LastAcceptedBlock(),
		dummy.NewETHFaker(),
		vm.chaindb,
		numBlocks,
		10,
		func(i int, g *core.BlockGen) {
			g.SetOnBlockGenerated(acceptExternalBlock)
			g.SetCoinbase(constants.BlackholeAddr) // necessary for syntactic validation of the block
			gen(i, g)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	vm.blockChain.DrainAcceptorQueue()
}

// assertSyncPerformedHeights iterates over all heights the VM has synced to and
// verifies it matches [expected].
func assertSyncPerformedHeights(t *testing.T, db ethdb.Iteratee, expected map[uint64]struct{}) {
	it := customrawdb.NewSyncPerformedIterator(db)
	defer it.Release()

	found := make(map[uint64]struct{}, len(expected))
	for it.Next() {
		found[customrawdb.UnpackSyncPerformedKey(it.Key())] = struct{}{}
	}
	require.NoError(t, it.Error())
	require.Equal(t, expected, found)
}

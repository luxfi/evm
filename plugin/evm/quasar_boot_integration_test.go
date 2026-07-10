// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"encoding/binary"
	"math/big"
	"sync"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/database"
	"github.com/luxfi/database/memdb"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/eth"
	"github.com/luxfi/evm/params"
	gethcommon "github.com/luxfi/geth/common"
	gethtypes "github.com/luxfi/geth/core/types"
	gethvm "github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// buildAcceptedTestChain builds a REAL canonical chain (genesis + n accepted blocks) and returns
// the eth backend wrapping it. Accept tip = n. This is the actual blockchain the boot reconcile
// reads LastAcceptedBlock() / GetBlockByNumber() from — no hand-picked heights.
func buildAcceptedTestChain(t *testing.T, n int) (*core.BlockChain, *eth.Ethereum) {
	t.Helper()
	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	cryptoAddr := crypto.PubkeyToAddress(key.PublicKey)
	addr := gethcommon.BytesToAddress(cryptoAddr[:])
	bal, _ := new(big.Int).SetString("100000000000000000000000", 10)
	gspec := &core.Genesis{Config: params.TestChainConfig, Alloc: gethtypes.GenesisAlloc{addr: {Balance: bal}}}
	engine := dummy.NewETHFaker()
	coinbase := gethcommon.HexToAddress("0x0100000000000000000000000000000000000000")
	genDb, blocks, _, err := core.GenerateChainWithGenesis(gspec, engine, n, 10, func(i int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
	})
	require.NoError(t, err)
	chain, err := core.NewBlockChain(genDb, core.DefaultCacheConfig, gspec, engine, gethvm.Config{}, gethcommon.Hash{}, false, nil)
	require.NoError(t, err)
	t.Cleanup(func() { chain.Stop() })
	_, err = chain.InsertChain(blocks)
	require.NoError(t, err)
	for _, b := range blocks {
		require.NoError(t, chain.Accept(b))
	}
	chain.DrainAcceptorQueue()
	require.EqualValues(t, n, chain.LastAcceptedBlock().NumberU64())
	return chain, eth.NewTestEthereum(chain)
}

// TestBootReconcile_RealLastAccepted_ClearsStaleQuasarOnRewind exercises the REAL boot wiring
// (vm.go initializeChain:1065-1066 and the auto-import-after-reconcile at Initialize): the reconcile
// is driven by the ACTUAL restored LastAcceptedBlock of a real chain — not a hand-picked height —
// across a genuine rewind (SetLastAcceptedBlockDirect, the primitive both the runbook rewind and
// admin/auto importBlocksFromFile use). It proves: (a) the belt (ClampedQuasarHeight) clamps
// `finalized` to the rewound tip IMMEDIATELY; and (b) the suspenders (reconcileQuasarWithAccept)
// clear the stale durable + in-mem export height once given the real post-rewind accept tip.
func TestBootReconcile_RealLastAccepted_ClearsStaleQuasarOnRewind(t *testing.T) {
	const n = 10
	chain, e := buildAcceptedTestChain(t, n)
	db := memdb.New()
	pvm := &VM{eth: e, acceptedBlockDB: db}

	// A valid export height (8 ≤ accept tip 10). Boot reconcile with the REAL LastAcceptedBlock is a
	// clean no-op (export frontier within the accept tip).
	pvm.SetLastQuasarFinalized(8)
	require.EqualValues(t, 8, e.LastQuasarHeight())
	pvm.reconcileQuasarWithAccept(pvm.eth.LastAcceptedBlock().NumberU64()) // real height = 10
	require.EqualValues(t, 8, e.LastQuasarHeight(), "no reconcile while export height ≤ real accept tip")
	require.EqualValues(t, 8, e.ClampedQuasarHeight(), "finalized == export height when it is within the accept tip")

	// REWIND to block 5 (the SetLastAcceptedBlockDirect primitive the rewind/re-import runbook uses):
	// accept tip drops to 5, below the persisted export height 8.
	require.NoError(t, chain.SetLastAcceptedBlockDirect(chain.GetBlockByNumber(5)))
	require.EqualValues(t, 5, e.LastAcceptedBlock().NumberU64())

	// BELT — structural, before any reconcile: `finalized` (ClampedQuasarHeight) clamps to the new
	// accept tip (5), NEVER naming the stale block 8 above the served tip.
	require.EqualValues(t, 5, e.ClampedQuasarHeight(), "finalized MUST clamp to the rewound accept tip immediately")

	// SUSPENDERS — the REAL boot/auto-import wiring: reconcile with the ACTUAL restored
	// LastAcceptedBlock (5), not a hand-picked height → clears the stale export height (8).
	pvm.reconcileQuasarWithAccept(pvm.eth.LastAcceptedBlock().NumberU64())
	require.Zero(t, e.LastQuasarHeight(), "boot reconcile must clear the stale export height on a real rewind")
	_, err := db.Get(quasarHeightKey)
	require.ErrorIs(t, err, database.ErrNotFound, "durable export key must be cleared on a real rewind")
	require.Zero(t, e.ClampedQuasarHeight(), "nothing export-final until the rebuilt chain re-certifies")
}

// TestSetLastQuasarFinalized_LockedAgainstReconcile drives the observer callback
// (SetLastQuasarFinalized) and the re-import reconcile (reconcileQuasarWithAccept, run under the
// caller's vmLock exactly as admin_importChain does) concurrently against the SAME atomic + durable
// key. With SetLastQuasarFinalized taking vmLock, the two writers are mutually excluded: the final
// state is always consistent (durable key == in-mem height), never a torn Put(H)/Delete interleave.
// Runs under -race in a CGO-on environment; the lock is the fix.
func TestSetLastQuasarFinalized_LockedAgainstReconcile(t *testing.T) {
	e := &eth.Ethereum{}
	db := memdb.New()
	pvm := &VM{eth: e, acceptedBlockDB: db}

	var wg sync.WaitGroup
	for i := uint64(1); i <= 200; i++ {
		wg.Add(2)
		go func(h uint64) { defer wg.Done(); pvm.SetLastQuasarFinalized(h) }(i)
		go func(h uint64) {
			defer wg.Done()
			// Mirror admin_importChain: hold vmLock across the reconcile (its documented invariant).
			pvm.vmLock.Lock()
			pvm.reconcileQuasarWithAccept(h % 50) // sometimes below the export height ⇒ a reset
			pvm.vmLock.Unlock()
		}(i)
	}
	wg.Wait()

	// Consistency invariant: the durable key and the in-mem height agree (no torn write). A cleared
	// key ⇔ in-mem 0; a present key ⇔ its uint64 == in-mem height.
	inMem := e.LastQuasarHeight()
	b, err := db.Get(quasarHeightKey)
	if err == database.ErrNotFound {
		require.Zero(t, inMem, "durable key cleared ⇒ in-mem must be 0")
	} else {
		require.NoError(t, err)
		require.EqualValues(t, binary.BigEndian.Uint64(b), inMem, "durable key and in-mem height must agree (no torn write)")
	}
}

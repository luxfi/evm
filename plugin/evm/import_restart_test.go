// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"math/big"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/params"
	gethcommon "github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	gethtypes "github.com/luxfi/geth/core/types"
	gethvm "github.com/luxfi/geth/core/vm"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

// importFixture is an RLP export of n blocks in which block i pays recipient
// i+1 wei, so the recipient's balance at height h is h(h+1)/2 and every
// height's state is distinct.
type importFixture struct {
	gspec     *core.Genesis
	file      string
	recipient gethcommon.Address
	n         uint64
}

func newImportFixture(t *testing.T, n int) *importFixture {
	t.Helper()
	key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	senderAddr := crypto.PubkeyToAddress(key.PublicKey)
	sender := gethcommon.BytesToAddress(senderAddr[:])
	recipient := gethcommon.HexToAddress("0x000000000000000000000000000000000000aaaa")
	bal, _ := new(big.Int).SetString("100000000000000000000000", 10)
	gspec := &core.Genesis{Config: params.TestChainConfig, Alloc: gethtypes.GenesisAlloc{sender: {Balance: bal}}}
	signer := gethtypes.LatestSigner(params.TestChainConfig)
	coinbase := gethcommon.HexToAddress("0x0100000000000000000000000000000000000000")
	_, blocks, _, err := core.GenerateChainWithGenesis(gspec, dummy.NewETHFaker(), n, 10, func(i int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		tx, err := gethtypes.SignTx(gethtypes.NewTransaction(uint64(i), recipient, big.NewInt(int64(i+1)), 21000, b.BaseFee(), nil), signer, key)
		require.NoError(t, err)
		b.AddTx(tx)
	})
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), "chain.rlp")
	f, err := os.Create(file)
	require.NoError(t, err)
	for _, b := range blocks {
		require.NoError(t, rlp.Encode(f, b))
	}
	require.NoError(t, f.Close())
	return &importFixture{gspec: gspec, file: file, recipient: recipient, n: uint64(n)}
}

func archiveConfig(commitInterval uint64) *core.CacheConfig {
	c := *core.DefaultCacheConfig
	c.Pruning = false
	c.CommitInterval = commitInterval
	return &c
}

func (fx *importFixture) open(t *testing.T, db ethdb.Database, cfg *core.CacheConfig, lastAccepted gethcommon.Hash) (*core.BlockChain, error) {
	t.Helper()
	return core.NewBlockChain(db, cfg, fx.gspec, dummy.NewETHFaker(), gethvm.Config{}, lastAccepted, false, nil)
}

// requireArchive checks what an archive node exists to serve at every height
// 1..n: the block, its state, and a lookup for each of its transactions.
func (fx *importFixture) requireArchive(t *testing.T, chain *core.BlockChain, db ethdb.Database, msg string) {
	t.Helper()
	for h := uint64(1); h <= fx.n; h++ {
		b := chain.GetBlockByNumber(h)
		require.NotNilf(t, b, "%s: block %d", msg, h)
		require.Truef(t, chain.HasState(b.Root()), "%s: state at height %d missing", msg, h)
		st, err := chain.StateAt(b.Root())
		require.NoErrorf(t, err, "%s: state at height %d", msg, h)
		require.EqualValuesf(t, h*(h+1)/2, st.GetBalance(fx.recipient).Uint64(), "%s: recipient balance at height %d", msg, h)
		for _, tx := range b.Transactions() {
			require.NotNilf(t, rawdb.ReadTxLookupEntry(db, tx.Hash()), "%s: tx %s at height %d not indexed", msg, tx.Hash(), h)
		}
	}
}

// TestImportArchive_HistorySurvivesRestart imports an RLP into an archive chain
// (pruning disabled), stops it, reopens a chain on the same database, and requires
// the state at every imported height and a lookup for every imported transaction.
// The import runs in rounds smaller than the chain, so heights inside a round are
// checked and not only a round's last.
//
// Commit interval 1 is the shape that left an imported testnet unable to start:
// the startup head repair re-executes the tip from the height before it, whose
// state the import had never committed.
func TestImportArchive_HistorySurvivesRestart(t *testing.T) {
	for _, tc := range []struct {
		name           string
		commitInterval uint64
	}{
		{"default commit interval", core.DefaultCacheConfig.CommitInterval},
		{"commit interval 1", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const n, round = 24, 5
			fx := newImportFixture(t, n)
			cfg := archiveConfig(tc.commitInterval)
			db := rawdb.NewMemoryDatabase()

			chain, err := fx.open(t, db, cfg, gethcommon.Hash{})
			require.NoError(t, err)
			imported, tipHash, tip, err := importBlocksFromFile(chain, fx.file, round, nil)
			require.NoError(t, err)
			require.Equal(t, n, imported)
			require.EqualValues(t, n, tip)
			chain.Stop()

			reopened, err := fx.open(t, db, cfg, tipHash)
			require.NoError(t, err, "an imported archive must start again")
			t.Cleanup(reopened.Stop)
			require.Equal(t, tipHash, reopened.LastAcceptedBlock().Hash())
			fx.requireArchive(t, reopened, db, "after restart")
		})
	}
}

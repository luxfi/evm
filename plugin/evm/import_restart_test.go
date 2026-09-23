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
	"github.com/luxfi/geth/rlp"
	"github.com/stretchr/testify/require"
)

// TestImportArchive_HistorySurvivesRestart imports an RLP into an archive chain
// (pruning disabled), stops it, reopens a chain on the same database, and requires
// what an archive node exists to serve: the state at every imported height and a
// lookup for every imported transaction. The import runs in rounds smaller than
// the chain, so heights inside a round are checked and not only a round's last.
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
			key, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
			senderAddr := crypto.PubkeyToAddress(key.PublicKey)
			sender := gethcommon.BytesToAddress(senderAddr[:])
			recipient := gethcommon.HexToAddress("0x000000000000000000000000000000000000aaaa")
			bal, _ := new(big.Int).SetString("100000000000000000000000", 10)
			gspec := &core.Genesis{Config: params.TestChainConfig, Alloc: gethtypes.GenesisAlloc{sender: {Balance: bal}}}
			engine := dummy.NewETHFaker()
			signer := gethtypes.LatestSigner(params.TestChainConfig)

			// Block i pays the recipient i+1 wei: its balance at height h is h(h+1)/2,
			// so every height's state is distinct.
			coinbase := gethcommon.HexToAddress("0x0100000000000000000000000000000000000000")
			_, blocks, _, err := core.GenerateChainWithGenesis(gspec, engine, n, 10, func(i int, b *core.BlockGen) {
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

			archive := *core.DefaultCacheConfig
			archive.Pruning = false
			archive.CommitInterval = tc.commitInterval
			db := rawdb.NewMemoryDatabase()

			chain, err := core.NewBlockChain(db, &archive, gspec, engine, gethvm.Config{}, gethcommon.Hash{}, false, nil)
			require.NoError(t, err)
			imported, tipHash, tip, err := importBlocksFromFile(chain, file, round, nil)
			require.NoError(t, err)
			require.Equal(t, n, imported)
			require.EqualValues(t, n, tip)
			chain.Stop()

			reopened, err := core.NewBlockChain(db, &archive, gspec, engine, gethvm.Config{}, tipHash, false, nil)
			require.NoError(t, err, "an imported archive must start again")
			t.Cleanup(reopened.Stop)
			require.Equal(t, tipHash, reopened.LastAcceptedBlock().Hash())

			for h := uint64(1); h <= n; h++ {
				b := reopened.GetBlockByNumber(h)
				require.NotNilf(t, b, "block %d", h)
				require.Truef(t, reopened.HasState(b.Root()), "state at height %d missing after restart", h)
				st, err := reopened.StateAt(b.Root())
				require.NoErrorf(t, err, "state at height %d", h)
				require.EqualValuesf(t, h*(h+1)/2, st.GetBalance(recipient).Uint64(), "recipient balance at height %d", h)
				for _, tx := range b.Transactions() {
					require.NotNilf(t, rawdb.ReadTxLookupEntry(db, tx.Hash()), "tx %s at height %d not indexed", tx.Hash(), h)
				}
			}
		})
	}
}

// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"testing"

	"github.com/luxfi/evm/core"
	gethcommon "github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/ethdb"
	"github.com/stretchr/testify/require"
)

// dyingDB is a disk under a process that is killed after a set number of writes:
// the writes before that reach it and every write after is lost. Reads see only
// what reached it. A budget below zero never dies and counts the writes made.
type dyingDB struct {
	ethdb.Database
	budget int
	writes int
}

func (d *dyingDB) alive() bool {
	d.writes++
	if d.budget < 0 {
		return true
	}
	if d.budget == 0 {
		return false
	}
	d.budget--
	return true
}

func (d *dyingDB) Put(key, value []byte) error {
	if !d.alive() {
		return nil
	}
	return d.Database.Put(key, value)
}

func (d *dyingDB) Delete(key []byte) error {
	if !d.alive() {
		return nil
	}
	return d.Database.Delete(key)
}

func (d *dyingDB) DeleteRange(start, end []byte) error {
	if !d.alive() {
		return nil
	}
	return d.Database.DeleteRange(start, end)
}

func (d *dyingDB) NewBatch() ethdb.Batch { return &dyingBatch{d.Database.NewBatch(), d} }

func (d *dyingDB) NewBatchWithSize(size int) ethdb.Batch {
	return &dyingBatch{d.Database.NewBatchWithSize(size), d}
}

// A batch reaches disk whole or not at all, so it costs one write.
type dyingBatch struct {
	ethdb.Batch
	db *dyingDB
}

func (b *dyingBatch) Write() error {
	if !b.db.alive() {
		return nil
	}
	return b.Batch.Write()
}

// persistAccepted is what persistAcceptedBlock records, written to db the way the
// VM's accepted-block database is written: after the import has accepted and
// committed a round.
func persistAccepted(db ethdb.KeyValueWriter) func(gethcommon.Hash, uint64) error {
	return func(hash gethcommon.Hash, _ uint64) error { return db.Put(lastAcceptedKey, hash[:]) }
}

// readAccepted is readLastAccepted over db: the recorded hash, or genesis when
// nothing was recorded.
func readAccepted(t *testing.T, db ethdb.KeyValueReader, genesis gethcommon.Hash) gethcommon.Hash {
	t.Helper()
	b, err := db.Get(lastAcceptedKey)
	if err != nil {
		return genesis
	}
	return gethcommon.BytesToHash(b)
}

// TestImportArchive_KilledAnywhereRestarts kills an archive import at every write
// it makes, reopens the chain from the accepted pointer the VM would read, resumes
// the import and restarts once more. Every kill point must reopen with the head at
// the accepted block, resume to the tip, and end with every height's state and
// every transaction's lookup on disk.
//
// A kill partway through accepting a round is the one that left lux-mainnet's
// C-Chain unable to start ("required historical state unavailable (reexec=2)"):
// the round's acceptor tip was on disk, above the accepted pointer, and the state
// of the blocks it named was not.
func TestImportArchive_KilledAnywhereRestarts(t *testing.T) {
	for _, tc := range []struct {
		name           string
		commitInterval uint64
	}{
		{"default commit interval", core.DefaultCacheConfig.CommitInterval},
		{"commit interval 1", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const n, round = 12, 5
			fx := newImportFixture(t, n)
			cfg := archiveConfig(tc.commitInterval)

			// A clean import sizes the sweep.
			clean := &dyingDB{Database: rawdb.NewMemoryDatabase(), budget: -1}
			chain, err := fx.open(t, clean, cfg, gethcommon.Hash{})
			require.NoError(t, err)
			genesis := chain.Genesis().Hash()
			start := clean.writes
			_, _, _, err = importBlocksFromFile(chain, fx.file, round, persistAccepted(clean))
			require.NoError(t, err)
			chain.Stop()
			total := clean.writes - start
			require.Positive(t, total)

			for kill := 0; kill < total; kill++ {
				disk := rawdb.NewMemoryDatabase()
				dying := &dyingDB{Database: disk, budget: -1}
				chain, err := fx.open(t, dying, cfg, gethcommon.Hash{})
				require.NoError(t, err)
				dying.budget = kill // the import's first kill writes reach disk
				_, _, _, _ = importBlocksFromFile(chain, fx.file, round, persistAccepted(dying))
				chain.Stop()

				accepted := readAccepted(t, disk, genesis)
				reopened, err := fx.open(t, disk, cfg, accepted)
				require.NoErrorf(t, err, "killed after %d of %d writes: the chain must start again", kill, total)
				require.Equalf(t, accepted, reopened.LastAcceptedBlock().Hash(), "killed after %d of %d writes: accepted block", kill, total)
				require.Equalf(t, accepted, reopened.CurrentBlock().Hash(), "killed after %d of %d writes: head must be the accepted block", kill, total)

				head := reopened.CurrentBlock().Number.Uint64()
				if _, _, _, err := importBlocksFromFile(reopened, fx.file, round, persistAccepted(disk)); err != nil {
					require.Truef(t, isNothingToImportError(err, head), "killed after %d of %d writes: resume: %v", kill, total, err)
				}
				reopened.Stop()

				final, err := fx.open(t, disk, cfg, readAccepted(t, disk, genesis))
				require.NoErrorf(t, err, "killed after %d of %d writes: restart after resuming", kill, total)
				require.EqualValuesf(t, n, final.LastAcceptedBlock().NumberU64(), "killed after %d of %d writes: resumed tip", kill, total)
				fx.requireArchive(t, final, disk, "killed and resumed")
				final.Stop()
			}
		})
	}
}

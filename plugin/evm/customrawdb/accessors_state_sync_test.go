// (c) 2023, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package customrawdb

import (
	"testing"

	ethrawdb "github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/require"
)

func TestClearPrefix(t *testing.T) {
	require := require.New(t)
	db := ethrawdb.NewMemoryDatabase()
	// add a key that should be cleared
	require.NoError(WriteSyncSegment(db, common.Hash{1}, common.Hash{}))

	// add a key that should not be cleared
	key := append(syncSegmentsPrefix, []byte("foo")...)
	require.NoError(db.Put(key, []byte("bar")))

	require.NoError(ClearAllSyncSegments(db))

	count := 0
	it := db.NewIterator(syncSegmentsPrefix, nil)
	defer it.Release()
	for it.Next() {
		count++
	}
	require.NoError(it.Error())
	require.Equal(1, count)
}

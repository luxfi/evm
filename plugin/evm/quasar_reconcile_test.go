// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"encoding/binary"
	"testing"

	"github.com/luxfi/database"
	"github.com/luxfi/database/memdb"
	"github.com/luxfi/evm/eth"
	"github.com/stretchr/testify/require"
)

// TestReconcileQuasarWithAccept_ClearsStaleHeightOnRewind proves the ONE legal regression of the
// otherwise-irreversible EXPORT (Quasar) frontier: a rewind / RLP re-import (SetLastAcceptedBlockDirect,
// the EVM-rewind→re-import restore runbook) that lowers the accept tip below a persisted export
// height MUST clear that stale height — both the durable acceptedBlockDB key and the in-memory value
// — so a rebuilt-with-a-DIFFERENT-canonical block is never reported export-final to a bridge/DEX
// consumer. This is the case RED flagged: the durable key + the monotone/advance-only guards keep the
// height stuck high across a rewind unless this reconcile yields to it.
func TestReconcileQuasarWithAccept_ClearsStaleHeightOnRewind(t *testing.T) {
	e := &eth.Ethereum{}
	e.SetLastQuasarHeight(1085000) // a persisted export height (pre-rewind)
	db := memdb.New()
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], 1085000)
	require.NoError(t, db.Put(quasarHeightKey, buf[:]))
	vm := &VM{eth: e, acceptedBlockDB: db}

	// Normal operation: the accept tip is at/above the export height → a clean no-op.
	vm.reconcileQuasarWithAccept(1085000)
	require.EqualValues(t, 1085000, e.LastQuasarHeight(), "no regression: export height unchanged when the accept tip is at/above it")
	got, err := db.Get(quasarHeightKey)
	require.NoError(t, err)
	require.EqualValues(t, 1085000, binary.BigEndian.Uint64(got), "durable key preserved in normal operation")

	// REWIND: the accept tip drops to 1084996 (below the persisted export height) → clear it.
	vm.reconcileQuasarWithAccept(1084996)
	require.Zero(t, e.LastQuasarHeight(), "export height MUST reset to 0 when the accept tip rewinds below it")
	_, err = db.Get(quasarHeightKey)
	require.ErrorIs(t, err, database.ErrNotFound, "durable export (quasar) key MUST be cleared on a rewind")

	// Idempotent: reconciling again after the reset is a clean no-op (no stale height to clear).
	vm.reconcileQuasarWithAccept(1084996)
	require.Zero(t, e.LastQuasarHeight())
}

// TestSetLastQuasarFinalized_PersistsBeforeAdvancing proves the LOW fix: the durable key is the
// source of truth. SetLastQuasarFinalized writes the acceptedBlockDB BEFORE advancing the in-memory
// height, so a crash between the two can only ever leave the in-mem height at or BEHIND the durable
// key on the next boot — never ahead of it (which would be a monotonicity dip that briefly reports a
// Nova block as export-final). On a persist FAILURE the in-mem height must NOT advance.
func TestSetLastQuasarFinalized_PersistsBeforeAdvancing(t *testing.T) {
	e := &eth.Ethereum{}
	db := memdb.New()
	vm := &VM{eth: e, acceptedBlockDB: db}

	vm.SetLastQuasarFinalized(42)
	require.EqualValues(t, 42, e.LastQuasarHeight(), "in-mem advances after a successful persist")
	got, err := db.Get(quasarHeightKey)
	require.NoError(t, err)
	require.EqualValues(t, 42, binary.BigEndian.Uint64(got), "durable key holds the advanced height")

	// Monotone: a lower height is ignored (in-mem and durable both unchanged).
	vm.SetLastQuasarFinalized(40)
	require.EqualValues(t, 42, e.LastQuasarHeight())

	// Persist failure ⇒ in-mem must NOT advance (the durable key stays authoritative). A closed DB
	// makes Put fail.
	require.NoError(t, db.Close())
	vm.SetLastQuasarFinalized(100)
	require.EqualValues(t, 42, e.LastQuasarHeight(), "in-mem must NOT advance when the durable persist fails")
}

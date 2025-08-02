// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"errors"
	"io"

	"github.com/luxfi/evm/v2/iface"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/database"
)

// DatabaseWrapper wraps a node database to provide compatibility with EVM database interfaces
type DatabaseWrapper struct {
	database.Database
}

// NewDatabaseWrapper creates a new database wrapper
func NewDatabaseWrapper(db database.Database) iface.Database {
	return &DatabaseWrapper{Database: db}
}

// HealthCheck implements the HealthChecker interface
func (d *DatabaseWrapper) HealthCheck() (interface{}, error) {
	// Call the underlying health check from node database
	_, err := d.Database.HealthCheck(context.Background())
	return nil, err
}

// Close closes the database
func (d *DatabaseWrapper) Close() error {
	return d.Database.Close()
}

// Has implements ethdb interface
func (d *DatabaseWrapper) Has(key []byte) (bool, error) {
	return d.Database.Has(key)
}

// Get implements ethdb interface
func (d *DatabaseWrapper) Get(key []byte) ([]byte, error) {
	return d.Database.Get(key)
}

// Put implements ethdb interface
func (d *DatabaseWrapper) Put(key []byte, value []byte) error {
	return d.Database.Put(key, value)
}

// Delete implements ethdb interface
func (d *DatabaseWrapper) Delete(key []byte) error {
	return d.Database.Delete(key)
}

// DeleteRange deletes all of the keys (and values) in the range [start,end)
// (inclusive on start, exclusive on end).
func (d *DatabaseWrapper) DeleteRange(start, end []byte) error {
	// Node database doesn't support DeleteRange, so we return an error
	return errors.New("DeleteRange not supported")
}

// NewBatch implements ethdb interface
func (d *DatabaseWrapper) NewBatch() ethdb.Batch {
	return &BatchWrapper{Batch: d.Database.NewBatch()}
}

// NewBatchWithSize creates a write-only database batch with pre-allocated buffer.
func (d *DatabaseWrapper) NewBatchWithSize(size int) ethdb.Batch {
	// Node database doesn't support size hints, so we just create a regular batch
	return &BatchWrapper{Batch: d.Database.NewBatch()}
}

// NewIterator implements ethdb interface
func (d *DatabaseWrapper) NewIterator(prefix []byte, start []byte) ethdb.Iterator {
	return d.Database.NewIterator()
}

// Compact implements ethdb interface
func (d *DatabaseWrapper) Compact(start []byte, limit []byte) error {
	return d.Database.Compact(start, limit)
}

// BatchWrapper wraps a node batch to provide compatibility with ethdb.Batch interface
type BatchWrapper struct {
	database.Batch
}

// DeleteRange implements ethdb.Batch interface
func (b *BatchWrapper) DeleteRange(start, end []byte) error {
	// Node database doesn't support DeleteRange, so we return an error
	return errors.New("DeleteRange not supported")
}

// ValueSize implements ethdb.Batch interface
func (b *BatchWrapper) ValueSize() int {
	return b.Batch.Size()
}

// Replay implements ethdb.Batch interface
func (b *BatchWrapper) Replay(w ethdb.KeyValueWriter) error {
	return b.Batch.Replay(w)
}

// Ancient methods - not supported by node database, required by ethdb.Database
func (d *DatabaseWrapper) Ancient(kind string, number uint64) ([]byte, error) {
	return nil, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) AncientRange(kind string, start, count, maxBytes uint64) ([][]byte, error) {
	return nil, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) Ancients() (uint64, error) {
	return 0, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) Tail() (uint64, error) {
	return 0, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) AncientSize(kind string) (uint64, error) {
	return 0, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) ModifyAncients(fn func(ethdb.AncientWriteOp) error) (int64, error) {
	return 0, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) TruncateHead(n uint64) (uint64, error) {
	return 0, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) TruncateTail(n uint64) (uint64, error) {
	return 0, errors.New("ancient store not supported")
}

func (d *DatabaseWrapper) Sync() error {
	return nil
}

func (d *DatabaseWrapper) MigrateTable(string, func([]byte) ([]byte, error)) error {
	return errors.New("table migration not supported")
}

func (d *DatabaseWrapper) NewSnapshot() (interface{}, error) {
	return nil, errors.New("snapshots not supported")
}

func (d *DatabaseWrapper) Stat() (string, error) {
	return "", errors.New("stats not supported")
}

func (d *DatabaseWrapper) AncientDatadir() (string, error) {
	return "", errors.New("ancient datadir not supported")
}

// ReadAncients reads multiple ancient values in one go
func (d *DatabaseWrapper) ReadAncients(fn func(ethdb.AncientReaderOp) error) (err error) {
	return errors.New("ancient store not supported")
}

// SyncAncient flushes the accumulated writes to disk synchronously.
func (d *DatabaseWrapper) SyncAncient() error {
	return errors.New("ancient store not supported")
}

// SyncKeyValue flushes the key-value store to disk synchronously.
func (d *DatabaseWrapper) SyncKeyValue() error {
	// Node database doesn't have SyncKeyValue, so we just return nil
	return nil
}

// Ensure interfaces are satisfied
var (
	_ iface.Database      = &DatabaseWrapper{}
	_ iface.HealthChecker = &DatabaseWrapper{}
	_ ethdb.Database      = &DatabaseWrapper{}
	_ io.Closer           = &DatabaseWrapper{}
)

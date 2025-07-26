// (c) 2020-2021, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/luxfi/evm/core/rawdb"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/plugin/evm/database"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/log"
	nodedb "github.com/luxfi/database"
	"github.com/luxfi/database/factory"
	"github.com/luxfi/database/prefixdb"
	"github.com/luxfi/node/utils/constants"
	luxlog "github.com/luxfi/log"
)

const (
	dbMetricsPrefix = "db"
	meterDBGatherer = "meterdb"
)

type DatabaseConfig struct {
	// If true, all writes are to memory and are discarded at shutdown.
	ReadOnly bool `json:"readOnly"`

	// Path to database
	Path string `json:"path"`

	// Name of the database type to use
	Name string `json:"name"`

	// Config bytes (JSON) for the database
	// See relevant (pebbledb, leveldb) config options
	Config []byte `json:"-"`
}

// initializeDBs initializes the databases used by the VM.
// If [useStandaloneDB] is true, the chain will use a standalone database for its state.
// Otherwise, the chain will use the provided [avaDB] for its state.
func (vm *VM) initializeDBs(avaDB nodedb.Database) error {
	db := avaDB
	// skip standalone database initialization if we are running in unit tests
	if vm.ctx.NetworkID != constants.UnitTestID {
		// first initialize the accepted block database to check if we need to use a standalone database
		acceptedDB := prefixdb.New(acceptedPrefix, avaDB)
		useStandAloneDB, err := vm.useStandaloneDatabase(acceptedDB)
		if err != nil {
			return err
		}
		if useStandAloneDB {
			// If we are using a standalone database, we need to create a new database
			// for the chain state.
			dbConfig, err := getDatabaseConfig(vm.config, vm.ctx.ChainDataDir)
			if err != nil {
				return err
			}
			log.Info("Using standalone database for the chain state", "DatabaseConfig", dbConfig)
			db, err = newStandaloneDatabase(dbConfig, vm.ctx.Metrics, vm.ctx.Log)
			if err != nil {
				return err
			}
			vm.usingStandaloneDB = true
		}
	}
	// Use NewNested rather than New so that the structure of the database
	// remains the same regardless of the provided baseDB type.
	// Wrap the prefixed database to provide iface.Database interface
	ethDB := NewDatabaseWrapper(prefixdb.NewNested(ethDBPrefix, db))
	vm.chaindb = rawdb.NewDatabase(database.WrapDatabase(ethDB))
	// For now, we don't use a versioned database - just use the regular db
	vm.versiondb = nil // TODO: Implement version database if needed
	vm.acceptedBlockDB = prefixdb.New(acceptedPrefix, db)
	vm.metadataDB = prefixdb.New(metadataPrefix, db)
	vm.db = db
	// Note warpDB and validatorsDB are not part of versiondb because it is not necessary
	// that they are committed to the database atomically with
	// the last accepted interfaces.
	// [warpDB] is used to store warp message signatures
	// set to a prefixDB with the prefix [warpPrefix]
	vm.warpDB = prefixdb.New(warpPrefix, db)
	// [validatorsDB] is used to store the current validator set and uptimes
	// set to a prefixDB with the prefix [validatorsDBPrefix]
	vm.validatorsDB = prefixdb.New(validatorsDBPrefix, db)
	return nil
}

func (vm *VM) inspectDatabases() error {
	start := time.Now()
	log.Info("Starting database inspection")
	if err := rawdb.InspectDatabase(vm.chaindb, nil, nil); err != nil {
		return err
	}
	if err := inspectDB(NewDatabaseWrapper(vm.acceptedBlockDB), "acceptedBlockDB"); err != nil {
		return err
	}
	if err := inspectDB(NewDatabaseWrapper(vm.metadataDB), "metadataDB"); err != nil {
		return err
	}
	if err := inspectDB(NewDatabaseWrapper(vm.warpDB), "warpDB"); err != nil {
		return err
	}
	if err := inspectDB(NewDatabaseWrapper(vm.validatorsDB), "validatorsDB"); err != nil {
		return err
	}
	log.Info("Completed database inspection", "elapsed", time.Since(start))
	return nil
}

// useStandaloneDatabase returns true if the chain can and should use a standalone database
// other than given by [db] in Initialize()
func (vm *VM) useStandaloneDatabase(acceptedDB nodedb.Database) (bool, error) {
	// Standalone database functionality is disabled for now
	// TODO: Add database configuration fields to Config if needed
	return false, nil
}

// getDatabaseConfig returns the database configuration for the chain
// to be used by separate, standalone database.
func getDatabaseConfig(config Config, chainDataDir string) (DatabaseConfig, error) {
	var configBytes []byte
	// Database configuration fields are not available in current Config
	// Use default configuration
	dbPath := filepath.Join(chainDataDir, "db")

	return DatabaseConfig{
		Name:     "badgerdb", // default database type
		ReadOnly: false,
		Path:     dbPath,
		Config:   configBytes,
	}, nil
}

func inspectDB(db iface.Database, label string) error {
	it := db.NewIterator(nil, nil)
	defer it.Release()

	var (
		count  int64
		start  = time.Now()
		logged = time.Now()

		// Totals
		total common.StorageSize
	)
	// Inspect key-value database first.
	for it.Next() {
		var (
			key  = it.Key()
			size = common.StorageSize(len(key) + len(it.Value()))
		)
		total += size
		count++
		if count%1000 == 0 && time.Since(logged) > 8*time.Second {
			log.Info("Inspecting database", "label", label, "count", count, "elapsed", common.PrettyDuration(time.Since(start)))
			logged = time.Now()
		}
	}
	// Display the database statistic.
	log.Info("Database statistics", "label", label, "total", total.String(), "count", count)
	return nil
}

func newStandaloneDatabase(dbConfig DatabaseConfig, gatherer interface{}, logger interface{}) (nodedb.Database, error) {
	dbPath := filepath.Join(dbConfig.Path, dbConfig.Name)

	// Convert the logger interface to luxfi/log.Logger
	var luxLogger luxlog.Logger
	if l, ok := logger.(luxlog.Logger); ok {
		luxLogger = l
	} else {
		// Fallback to noop logger if conversion fails
		luxLogger = luxlog.NewNoopLogger()
	}
	
	// Use the factory to create the database
	db, err := factory.New(
		dbConfig.Name,
		dbPath,
		false, // not read-only
		dbConfig.Config,
		gatherer, // Use the provided gatherer
		luxLogger,
		"evm", // metrics prefix
		"",    // meter db reg name
	)
	if err != nil {
		return nil, fmt.Errorf("couldn't create database: %w", err)
	}
	
	return db, nil
}

// (c) 2019-2021, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/log"
	"github.com/luxfi/evm/iface/core/rawdb"
	"github.com/luxfi/evm/plugin/evm/config"
	"github.com/luxfi/evm/plugin/evm/database"
	"github.com/luxfi/evm/iface"
	luxdatabase "github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
	"github.com/luxfi/evm/iface"
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
func (vm *VM) initializeDBs(avaDB luxinterfaces.Database) error {
	db := avaDB
	// skip standalone database initialization if we are running in unit tests
	if vm.ctx.NetworkID != constants.UnitTestID {
		// first initialize the accepted block database to check if we need to use a standalone database
		acceptedDB := interfaces.NewPrefixDB(acceptedPrefix, avaDB)
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
	vm.chaindb = rawdb.NewDatabase(database.WrapDatabase(interfaces.NewPrefixDBNested(ethDBPrefix, db)))
	vm.versiondb = interfaces.NewVersionDB(db)
	vm.acceptedBlockDB = interfaces.NewPrefixDB(acceptedPrefix, vm.versiondb)
	vm.metadataDB = interfaces.NewPrefixDB(metadataPrefix, vm.versiondb)
	vm.db = db
	// Note warpDB and validatorsDB are not part of versiondb because it is not necessary
	// that they are committed to the database atomically with
	// the last accepted interfaces.
	// [warpDB] is used to store warp message signatures
	// set to a prefixDB with the prefix [warpPrefix]
	vm.warpDB = interfaces.NewPrefixDB(warpPrefix, db)
	// [validatorsDB] is used to store the current validator set and uptimes
	// set to a prefixDB with the prefix [validatorsDBPrefix]
	vm.validatorsDB = interfaces.NewPrefixDB(validatorsDBPrefix, db)
	return nil
}

func (vm *VM) inspectDatabases() error {
	start := time.Now()
	log.Info("Starting database inspection")
	if err := rawdb.InspectDatabase(vm.chaindb, nil, nil); err != nil {
		return err
	}
	if err := inspectDB(vm.acceptedBlockDB, "acceptedBlockDB"); err != nil {
		return err
	}
	if err := inspectDB(vm.metadataDB, "metadataDB"); err != nil {
		return err
	}
	if err := inspectDB(vm.warpDB, "warpDB"); err != nil {
		return err
	}
	if err := inspectDB(vm.validatorsDB, "validatorsDB"); err != nil {
		return err
	}
	log.Info("Completed database inspection", "elapsed", time.Since(start))
	return nil
}

// useStandaloneDatabase returns true if the chain can and should use a standalone database
// other than given by [db] in Initialize()
func (vm *VM) useStandaloneDatabase(acceptedDB luxinterfaces.Database) (bool, error) {
	// no config provided, use default
	standaloneDBFlag := vm.interfaces.UseStandaloneDatabase
	if standaloneDBFlag != nil {
		return standaloneDBFlag.Bool(), nil
	}

	// check if the chain can use a standalone database
	_, err := acceptedDB.Get(lastAcceptedKey)
	if err == luxdatabase.ErrNotFound {
		// If there is nothing in the database, we can use the standalone database
		return true, nil
	}
	return false, err
}

// getDatabaseConfig returns the database configuration for the chain
// to be used by separate, standalone database.
func getDatabaseConfig(config interfaces.Config, chainDataDir string) (DatabaseConfig, error) {
	var (
		configBytes []byte
		err         error
	)
	if len(interfaces.DatabaseConfigContent) != 0 {
		dbConfigContent := interfaces.DatabaseConfigContent
		configBytes, err = base64.StdEncoding.DecodeString(dbConfigContent)
		if err != nil {
			return DatabaseConfig{}, fmt.Errorf("unable to decode base64 content: %w", err)
		}
	} else if len(interfaces.DatabaseConfigFile) != 0 {
		configPath := interfaces.DatabaseConfigFile
		configBytes, err = os.ReadFile(configPath)
		if err != nil {
			return DatabaseConfig{}, err
		}
	}

	dbPath := filepath.Join(chainDataDir, "db")
	if len(interfaces.DatabasePath) != 0 {
		dbPath = interfaces.DatabasePath
	}

	return DatabaseConfig{
		Name:     interfaces.DatabaseType,
		ReadOnly: interfaces.DatabaseReadOnly,
		Path:     dbPath,
		Config:   configBytes,
	}, nil
}

func inspectDB(db luxinterfaces.Database, label string) error {
	it := db.NewIterator()
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

func newStandaloneDatabase(dbConfig DatabaseConfig, gatherer interfaces.MultiGatherer, logger logging.Logger) (luxinterfaces.Database, error) {
	dbPath := filepath.Join(dbConfig.Path, dbConfig.Name)

	dbConfigBytes := dbConfig.Config
	// If the database is pebble, we need to set the config
	// to use no sync. Sync mode in pebble has an issue with OSs like MacOS.
	if dbConfig.Name == interfaces.Name {
		cfg := interfaces.DefaultConfig
		// Default to "no sync" for pebble db
		cfg.Sync = false
		if len(dbConfigBytes) > 0 {
			if err := interfaces.Unmarshal(dbConfigBytes, &cfg); err != nil {
				return nil, err
			}
		}
		var err error
		// Marshal the config back to bytes to ensure that new defaults are applied
		dbConfigBytes, err = interfaces.Marshal(cfg)
		if err != nil {
			return nil, err
		}
	}

	var db luxinterfaces.Database
	switch dbConfig.Name {
	case interfaces.Name:
		db, err = interfaces.New(dbPath, dbConfigBytes, logger, gatherer)
	case interfaces.Name:
		db, err = interfaces.New(dbPath, dbConfigBytes, logger, gatherer)
	default:
		return nil, fmt.Errorf("unknown database type: %s", dbConfig.Name)
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't create database: %w", err)
	}

	return db, nil
}

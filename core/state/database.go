// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	ethstate "github.com/luxfi/geth/core/state"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/geth/triedb"
	// "github.com/luxfi/evm/triedb/firewood"
)

type (
	Database = ethstate.Database
	Trie     = ethstate.Trie
)

// NewDatabase creates a state database with default trie configuration.
// This is a convenience wrapper that creates a triedb internally with nil config.
// The snapshot tree is set to nil (no snapshot support).
func NewDatabase(db ethdb.Database) Database {
	trieDB := triedb.NewDatabase(db, nil)
	return ethstate.NewDatabase(trieDB, nil)
}

// NewDatabaseWithConfig creates a state database with custom trie configuration.
// This is a convenience wrapper that creates a triedb with the specified config.
// The snapshot tree is set to nil (no snapshot support).
func NewDatabaseWithConfig(db ethdb.Database, config *triedb.Config) Database {
	trieDB := triedb.NewDatabase(db, config)
	return ethstate.NewDatabase(trieDB, nil)
}

// NewDatabaseWithNodeDB creates a state database using an existing triedb.
// This allows sharing a triedb instance across multiple state databases.
// The snapshot tree is set to nil (no snapshot support).
// Note: The db parameter is kept for backward compatibility but is not used
// since the triedb already contains the underlying database reference.
func NewDatabaseWithNodeDB(db ethdb.Database, tdb *triedb.Database) Database {
	return ethstate.NewDatabase(tdb, nil)
}

// func wrapIfFirewood(db Database) Database {
// 	fw, ok := db.TrieDB().Backend().(*firewood.Database)
// 	if !ok {
// 		return db
// 	}
// 	return &firewoodAccessorDb{
// 		Database: db,
// 		fw:       fw,
// 	}
// }

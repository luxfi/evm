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
// Copyright 2023 The go-ethereum Authors
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

package eth

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	log "github.com/luxfi/log"
)

// AdminAPI is the collection of Ethereum full node related APIs for node
// administration.
type AdminAPI struct {
	eth *Ethereum
}

// NewAdminAPI creates a new instance of AdminAPI.
func NewAdminAPI(eth *Ethereum) *AdminAPI {
	return &AdminAPI{eth: eth}
}

// ExportChain exports the current blockchain into a local file,
// or a range of blocks if first and last are non-nil.
func (api *AdminAPI) ExportChain(file string, first *uint64, last *uint64) (bool, error) {
	if first == nil && last != nil {
		return false, errors.New("last cannot be specified without first")
	}
	if first != nil && last == nil {
		head := api.eth.BlockChain().CurrentHeader().Number.Uint64()
		last = &head
	}
	if _, err := os.Stat(file); err == nil {
		// File already exists. Allowing overwrite could be a DoS vector,
		// since the 'file' may point to arbitrary paths on the drive.
		return false, errors.New("location would overwrite an existing file")
	}
	// Make sure we can create the file to export into
	out, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return false, err
	}
	defer out.Close()

	var writer io.Writer = out
	if strings.HasSuffix(file, ".gz") {
		writer = gzip.NewWriter(writer)
		defer writer.(*gzip.Writer).Close()
	}

	// Export the blockchain
	if first != nil {
		if err := api.eth.BlockChain().ExportN(writer, *first, *last); err != nil {
			return false, err
		}
	} else if err := api.eth.BlockChain().Export(writer); err != nil {
		return false, err
	}
	return true, nil
}

// NOTE: admin_importChain is DELIBERATELY not implemented on this eth-package AdminAPI. There is
// EXACTLY ONE admin_importChain — the plugin VM's (plugin/evm/admin_api.go), which runs under
// vmLock AND calls reconcileQuasarWithAccept after each SetLastAcceptedBlockDirect so a re-import
// that lowers the accept tip clears the stale EXPORT-FINAL (Quasar) height. An eth-package
// ImportChain here (which neither reconciled nor skipped ≤-head blocks) would reopen the two-tier
// finality-safety rewind hole the moment a future RegisterName reorder stopped the plugin one from
// shadowing it. Do not re-add it here.

// WriteGenesisStateSpec writes the genesis state spec to the database.
// This is required for RLP import to work when the genesis state trie is not accessible.
// The genesis allocs are stored in the database, enabling block_validator.go's special
// handling for block 1 (which allows import even without the genesis state trie).
func (api *AdminAPI) WriteGenesisStateSpec(genesisFile string) (bool, error) {
	// Read genesis file
	data, err := os.ReadFile(genesisFile)
	if err != nil {
		return false, fmt.Errorf("failed to read genesis file: %w", err)
	}

	// Parse genesis to get alloc
	var genesis struct {
		Alloc types.GenesisAlloc `json:"alloc"`
	}
	if err := json.Unmarshal(data, &genesis); err != nil {
		return false, fmt.Errorf("failed to parse genesis: %w", err)
	}

	// Marshal alloc to JSON
	allocJSON, err := json.Marshal(genesis.Alloc)
	if err != nil {
		return false, fmt.Errorf("failed to marshal alloc: %w", err)
	}

	// Get the genesis block hash
	chain := api.eth.BlockChain()
	genesisBlock := chain.GetBlockByNumber(0)
	if genesisBlock == nil {
		return false, errors.New("genesis block not found")
	}

	// Write to database using proper rawdb method
	db := api.eth.ChainDb()
	rawdb.WriteGenesisStateSpec(db, genesisBlock.Hash(), allocJSON)

	log.Info("WriteGenesisStateSpec: written",
		"genesisHash", genesisBlock.Hash().Hex(),
		"allocSize", len(allocJSON))

	return true, nil
}

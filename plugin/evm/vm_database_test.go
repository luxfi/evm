// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/luxfi/evm/plugin/evm/config"
	"github.com/luxfi/geth/ethdb"
	"github.com/luxfi/geth/ethdb/memorydb"
)

// configFromChainJSON parses a C-Chain config the way the VM does, so a test
// exercises the same path a node's --cchain-ancient flags travel down.
func configFromChainJSON(t *testing.T, body string) config.Config {
	t.Helper()
	var c config.Config
	c.SetDefaults(defaultTxPoolConfig)
	if body != "" {
		if err := json.Unmarshal([]byte(body), &c); err != nil {
			t.Fatalf("parse chain config: %v", err)
		}
	}
	return c
}

// TestChainDatabaseWithoutAncientStore is the default posture: the chain
// database holds everything and there is no ancient store behind it.
func TestChainDatabaseWithoutAncientStore(t *testing.T) {
	cfg := configFromChainJSON(t, "")
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config rejected: %v", err)
	}
	db, err := openChainDatabase(memorydb.New(), cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Ancients(); err == nil {
		t.Fatal("a chain database with no ancient store reported one")
	}
}

// TestChainDatabaseOwnsItsAncientStore covers the node that writes the store.
func TestChainDatabaseOwnsItsAncientStore(t *testing.T) {
	dir := t.TempDir()
	cfg := configFromChainJSON(t, `{"ancient-dir":"`+dir+`","freeze-threshold":100}`)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("writer config rejected: %v", err)
	}
	if cfg.AncientDir != dir || cfg.FreezeThreshold != 100 || cfg.AncientShared {
		t.Fatalf("parsed %+v", cfg)
	}
	db, err := openChainDatabase(memorydb.New(), cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Ancients(); err != nil {
		t.Fatalf("the store is not readable: %v", err)
	}
	// A writer brings the store into being, which is what a reader then shares.
	if _, err := os.Stat(filepath.Join(dir, "chain")); err != nil {
		t.Fatalf("the writer did not create the store: %v", err)
	}
}

// TestChainDatabaseSharesAnotherNodesAncientStore covers the node that reads
// one: it opens the same directory while the writer holds it, and writes
// nothing of its own.
func TestChainDatabaseSharesAnotherNodesAncientStore(t *testing.T) {
	dir := t.TempDir()

	writer, err := openChainDatabase(memorydb.New(), configFromChainJSON(t,
		`{"ancient-dir":"`+dir+`","freeze-threshold":100}`))
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	defer writer.Close()

	cfg := configFromChainJSON(t,
		`{"ancient-dir":"`+dir+`","ancient-read-only":true,"ancient-shared":true,"freeze-threshold":100}`)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("shared reader config rejected: %v", err)
	}
	reader, err := openChainDatabase(memorydb.New(), cfg)
	if err != nil {
		t.Fatalf("a reader could not open the store the writer holds: %v", err)
	}
	defer reader.Close()

	if _, err := reader.Ancients(); err != nil {
		t.Fatalf("the shared store is not readable: %v", err)
	}
	_, err = reader.ModifyAncients(func(op ethdb.AncientWriteOp) error {
		return op.AppendRaw("hashes", 0, make([]byte, 32))
	})
	if err == nil {
		t.Fatal("a shared reader was allowed to write to the store")
	}
}

// TestAncientConfigInvariants: the combinations a node can be started with, and
// the ones it refuses because they do not describe a real arrangement.
func TestAncientConfigInvariants(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "sharing a store nobody named",
			body: `{"ancient-shared":true}`,
			want: "ancient-shared needs ancient-dir",
		},
		{
			name: "no hot window",
			body: `{"ancient-dir":"/srv/lux/ancient","freeze-threshold":0}`,
			want: "must be at least 1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configFromChainJSON(t, tt.body)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("%s was accepted", tt.body)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not mention %q", err, tt.want)
			}
		})
	}

	// The arrangements that describe a real set of nodes: one that owns its
	// store, and one that reads a store another node owns.
	for _, body := range []string{
		`{"ancient-dir":"/srv/lux/ancient"}`,
		`{"ancient-dir":"/srv/lux/ancient","ancient-shared":true,"freeze-threshold":100}`,
	} {
		cfg := configFromChainJSON(t, body)
		if err := cfg.Validate(); err != nil {
			t.Fatalf("%s was refused: %v", body, err)
		}
	}
}

// TestFreezeThresholdDefaults checks a config that only names a directory still
// gets a hot window, rather than a zero that would mean freeze everything.
func TestFreezeThresholdDefaults(t *testing.T) {
	cfg := configFromChainJSON(t, `{"ancient-dir":"/srv/lux/ancient"}`)
	if cfg.FreezeThreshold == 0 {
		t.Fatal("no default hot window")
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("rejected: %v", err)
	}
	// The store the config names does not have to exist for a writer; it does
	// for a reader, which is how a mistyped shared path is caught at boot
	// instead of silently serving a chain with no history.
	shared := configFromChainJSON(t,
		`{"ancient-dir":"/nonexistent/lux/ancient","ancient-read-only":true,"ancient-shared":true}`)
	if err := shared.Validate(); err != nil {
		t.Fatalf("rejected: %v", err)
	}
	_, err := openChainDatabase(memorydb.New(), shared)
	if err == nil {
		t.Fatal("a shared reader opened a store that is not there")
	}
	if _, statErr := os.Stat("/nonexistent"); !os.IsNotExist(statErr) {
		t.Fatal("the failed open created the path")
	}
}

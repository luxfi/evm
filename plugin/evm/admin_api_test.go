// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/luxfi/geth/common"
)

// TestWriteSentinel_AtomicAndRoundtrip exercises the persistence helper that
// admin_exportChain calls every exportProgressInterval blocks. The write must
// land atomically (tmp + rename) and the bytes must round-trip through
// readSentinel byte-for-byte.
func TestWriteSentinel_AtomicAndRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "C-1082780.rlp.sentinel")

	want := exportSentinel{
		Status:          "in_progress",
		First:           0,
		Last:            1082780,
		HighestExported: 1024,
		FirstHash:       common.HexToHash("0x3f4fa2a000000000000000000000000000000000000000000000000000000000"),
		LastHash:        common.HexToHash("0xdeadbeef00000000000000000000000000000000000000000000000000000000"),
		UpdatedAt:       time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC),
	}

	writeSentinel(path, want)

	// File exists; tmp file does not.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("sentinel not written: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("tmp file not removed after rename: err=%v", err)
	}

	got, ok := readSentinel(path)
	if !ok {
		t.Fatal("readSentinel returned ok=false on a freshly-written sentinel")
	}
	if got.Status != want.Status {
		t.Errorf("Status: got=%q want=%q", got.Status, want.Status)
	}
	if got.First != want.First || got.Last != want.Last || got.HighestExported != want.HighestExported {
		t.Errorf("range: got={first=%d last=%d highest=%d} want={first=%d last=%d highest=%d}",
			got.First, got.Last, got.HighestExported, want.First, want.Last, want.HighestExported)
	}
	if got.FirstHash != want.FirstHash {
		t.Errorf("FirstHash: got=%s want=%s", got.FirstHash.Hex(), want.FirstHash.Hex())
	}
	if got.LastHash != want.LastHash {
		t.Errorf("LastHash: got=%s want=%s", got.LastHash.Hex(), want.LastHash.Hex())
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt: got=%v want=%v", got.UpdatedAt, want.UpdatedAt)
	}
}

// TestReadSentinel_AbsentReturnsFalse covers the no-prior-export code path so
// admin_exportChain doesn't crash on first invocation against a fresh PVC.
func TestReadSentinel_AbsentReturnsFalse(t *testing.T) {
	_, ok := readSentinel(filepath.Join(t.TempDir(), "nope.sentinel"))
	if ok {
		t.Error("readSentinel returned ok=true for a missing file")
	}
}

// TestReadSentinel_CorruptReturnsFalse ensures a half-written sentinel from a
// pre-atomic-write version (or a manually-edited bad file) doesn't poison the
// idempotency check by returning a zero-valued struct that happens to match
// {First=0, Last=0}.
func TestReadSentinel_CorruptReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.sentinel")
	if err := os.WriteFile(path, []byte("{not-json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, ok := readSentinel(path)
	if ok {
		t.Error("readSentinel returned ok=true on corrupt JSON")
	}
}

// TestSentinel_JSONShape locks the on-disk shape so a future refactor that
// renames a field doesn't silently break operators reading the file.
func TestSentinel_JSONShape(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shape.sentinel")
	writeSentinel(path, exportSentinel{
		Status:          "done",
		First:           1,
		Last:            2,
		HighestExported: 2,
		FirstHash:       common.HexToHash("0x11"),
		LastHash:        common.HexToHash("0x22"),
		UpdatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	wantKeys := []string{"status", "first", "last", "highestExported", "firstHash", "lastHash", "updatedAt"}
	for _, k := range wantKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("sentinel missing key %q (have %v)", k, mapKeys(m))
		}
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestExportChainResult_JSONShape locks the wire shape that the CLI and the
// operator both decode. A renamed field here is a breaking change.
func TestExportChainResult_JSONShape(t *testing.T) {
	r := &ExportChainResult{
		Success:         true,
		Status:          "ok",
		BlocksExported:  100,
		First:           0,
		Last:            99,
		HighestExported: 99,
		FirstHash:       common.HexToHash("0xaa"),
		LastHash:        common.HexToHash("0xbb"),
		SentinelPath:    "/data/exports/C-99.rlp.sentinel",
		Message:         "exported 100 blocks [0..99]",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		"success", "status", "blocksExported", "first", "last",
		"highestExported", "firstHash", "lastHash", "sentinelPath",
	}
	for _, k := range wantKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("ExportChainResult missing key %q (have %v)", k, mapKeys(m))
		}
	}
}

// TestExportProgressInterval_Stable pins the constant so a future bump
// (which would change disk-write frequency on long exports) is a deliberate
// edit, not a stealth refactor.
func TestExportProgressInterval_Stable(t *testing.T) {
	if exportProgressInterval != 1024 {
		t.Errorf("exportProgressInterval = %d, want 1024 (change requires updating LLM.md)",
			exportProgressInterval)
	}
}

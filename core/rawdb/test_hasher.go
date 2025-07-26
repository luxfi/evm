// Copyright 2025 Lux Industries, Inc.
// This file contains test utilities for hashing.

package rawdb

import (
	"hash"

	"github.com/luxfi/geth/common"
	"golang.org/x/crypto/sha3"
)

// testHasher is the helper tool for transaction/receipt list hashing.
// The original hasher is trie, in order to get rid of import cycle,
// use the testing hasher instead.
type testHasher struct {
	hasher hash.Hash
}

// NewTestHasher returns a new testHasher instance.
func NewTestHasher() *testHasher {
	return &testHasher{hasher: sha3.NewLegacyKeccak256()}
}

// Reset resets the hash state.
func (h *testHasher) Reset() {
	h.hasher.Reset()
}

// Update updates the intermediate trie state with the given key-value pair.
// Note that this contains whatever intermediate logic the internal trie uses.
func (h *testHasher) Update(key, value []byte) error {
	// Simple concatenation for test purposes
	h.hasher.Write(key)
	h.hasher.Write(value)
	return nil
}

// Hash returns the accumulated hash value.
func (h *testHasher) Hash() common.Hash {
	return common.BytesToHash(h.hasher.Sum(nil))
}


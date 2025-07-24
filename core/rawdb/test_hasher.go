// Copyright 2025 Lux Industries, Inc.
// This file contains test utilities for hashing.

package rawdb

import (
	"hash"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/rlp"
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

// hashItems is a test utility to compute the hash of a list of items.
func hashItems[T any](items []T) common.Hash {
	h := sha3.NewLegacyKeccak256()
	for _, item := range items {
		if data, err := rlp.EncodeToBytes(item); err == nil {
			h.Write(data)
		}
	}
	return common.BytesToHash(h.Sum(nil))
}
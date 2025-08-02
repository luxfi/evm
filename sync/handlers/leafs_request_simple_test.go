// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package handlers

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/evm/core/state/snapshot"
	"github.com/luxfi/evm/plugin/evm/message"
	"github.com/luxfi/evm/sync/handlers/stats/statstest"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/triedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simpleSnapshotProvider implements SnapshotProvider for testing
type simpleSnapshotProvider struct{}

func (s *simpleSnapshotProvider) Snapshots() *snapshot.Tree {
	return nil // No snapshots available
}

func TestLeafsRequestHandlerSimple(t *testing.T) {
	// Create a simple trie database
	memdb := triedb.NewDatabase(nil, nil)
	
	// Create test stats
	testStats := &statstest.TestHandlerStats{}
	
	// Create a simple snapshot provider
	snapshotProvider := &simpleSnapshotProvider{}
	
	// Create the handler
	handler := NewLeafsRequestHandler(memdb, common.HashLength, snapshotProvider, message.Codec, testStats)
	
	// Test with various invalid requests
	tests := []struct {
		name        string
		request     message.LeafsRequest
		expectNil   bool
		description string
	}{
		{
			name: "empty_root",
			request: message.LeafsRequest{
				Root:    common.Hash{},
				Account: common.Hash{},
				Start:   []byte{},
				End:     []byte{0xff},
				Limit:   10,
			},
			expectNil:   true,
			description: "empty root should return nil",
		},
		{
			name: "zero_limit",
			request: message.LeafsRequest{
				Root:    common.HexToHash("0x1234"),
				Account: common.Hash{},
				Start:   []byte{},
				End:     []byte{0xff},
				Limit:   0,
			},
			expectNil:   true,
			description: "zero limit should return nil",
		},
		{
			name: "start_after_end",
			request: message.LeafsRequest{
				Root:    common.HexToHash("0x1234"),
				Account: common.Hash{},
				Start:   []byte{0xff},
				End:     []byte{0x00},
				Limit:   10,
			},
			expectNil:   true,
			description: "start after end should return nil",
		},
		{
			name: "missing_trie",
			request: message.LeafsRequest{
				Root:    common.HexToHash("0x1234567890abcdef"),
				Account: common.Hash{},
				Start:   []byte{},
				End:     []byte{0xff},
				Limit:   10,
			},
			expectNil:   true,
			description: "missing trie should return nil",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			responseBytes, err := handler.OnLeafsRequest(context.Background(), ids.GenerateTestNodeID(), 1, tt.request)
			require.NoError(t, err, "handler should not return error")
			if tt.expectNil {
				assert.Nil(t, responseBytes, tt.description)
			} else {
				assert.NotNil(t, responseBytes, tt.description)
			}
		})
	}
	
	// Verify stats were updated
	assert.Greater(t, testStats.LeafsRequestCount, uint32(0), "request count should be greater than 0")
	assert.Greater(t, testStats.InvalidLeafsRequestCount, uint32(0), "invalid request count should be greater than 0")
}
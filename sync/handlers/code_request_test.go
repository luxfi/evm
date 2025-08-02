// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package handlers

import (
	"context"
	"testing"

	"github.com/luxfi/evm/v2/v2/plugin/evm/message"
	"github.com/luxfi/evm/v2/v2/sync/handlers/stats/statstest"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/assert"
)

func TestCodeRequestHandler(t *testing.T) {
	tests := map[string]struct {
		codeBytes     [][]byte
		codeHashes    []common.Hash
		expectedBytes [][]byte
		assertStats   func(t *testing.T, stats *statstest.TestHandlerStats)
	}{
		"zero hashes": {
			codeBytes:     nil,
			codeHashes:    []common.Hash{},
			expectedBytes: [][]byte{},
			assertStats: func(t *testing.T, stats *statstest.TestHandlerStats) {
				assert.Equal(t, uint32(1), stats.CodeRequestCount)
				assert.Equal(t, uint32(0), stats.DuplicateHashesRequested)
				assert.Equal(t, uint32(0), stats.TooManyHashesRequested)
				assert.Equal(t, uint32(0), stats.MissingCodeHashCount)
				assert.Equal(t, uint32(0), stats.CodeBytesReturnedSum)
			},
		},
		"single code": {
			codeBytes:     [][]byte{{1, 2, 3}},
			codeHashes:    []common.Hash{{1}},
			expectedBytes: [][]byte{{1, 2, 3}},
			assertStats: func(t *testing.T, stats *statstest.TestHandlerStats) {
				assert.Equal(t, uint32(1), stats.CodeRequestCount)
				assert.Equal(t, uint32(0), stats.DuplicateHashesRequested)
				assert.Equal(t, uint32(0), stats.TooManyHashesRequested)
				assert.Equal(t, uint32(0), stats.MissingCodeHashCount)
				assert.Equal(t, uint32(3), stats.CodeBytesReturnedSum)
			},
		},
		"multiple hashes": {
			codeBytes:     [][]byte{{1, 2, 3}, {1}, {255, 0, 15, 16}},
			codeHashes:    []common.Hash{{1}, {2}, {3}},
			expectedBytes: [][]byte{{1, 2, 3}, {1}, {255, 0, 15, 16}},
			assertStats: func(t *testing.T, stats *statstest.TestHandlerStats) {
				assert.Equal(t, uint32(1), stats.CodeRequestCount)
				assert.Equal(t, uint32(0), stats.DuplicateHashesRequested)
				assert.Equal(t, uint32(0), stats.TooManyHashesRequested)
				assert.Equal(t, uint32(0), stats.MissingCodeHashCount)
				assert.Equal(t, uint32(8), stats.CodeBytesReturnedSum)
			},
		},
		"not found code": {
			codeBytes:     [][]byte{{1, 2, 3}, nil, {255, 0, 15, 16}},
			codeHashes:    []common.Hash{{1}, {2}, {3}},
			expectedBytes: nil,
			assertStats: func(t *testing.T, stats *statstest.TestHandlerStats) {
				assert.Equal(t, uint32(1), stats.CodeRequestCount)
				assert.Equal(t, uint32(0), stats.DuplicateHashesRequested)
				assert.Equal(t, uint32(0), stats.TooManyHashesRequested)
				assert.Equal(t, uint32(1), stats.MissingCodeHashCount)
				assert.Equal(t, uint32(0), stats.CodeBytesReturnedSum)
			},
		},
		"too many hashes": {
			codeBytes:     make([][]byte, message.MaxCodeHashesPerRequest+1),
			codeHashes:    make([]common.Hash, message.MaxCodeHashesPerRequest+1),
			expectedBytes: nil,
			assertStats: func(t *testing.T, stats *statstest.TestHandlerStats) {
				assert.Equal(t, uint32(1), stats.CodeRequestCount)
				assert.Equal(t, uint32(0), stats.DuplicateHashesRequested)
				assert.Equal(t, uint32(1), stats.TooManyHashesRequested)
				assert.Equal(t, uint32(0), stats.MissingCodeHashCount)
				assert.Equal(t, uint32(0), stats.CodeBytesReturnedSum)
			},
		},
		"duplicate hashes": {
			codeBytes:     [][]byte{{1, 2, 3}, {1}, {255, 0, 15, 16}},
			codeHashes:    []common.Hash{{1}, {2}, {1}},
			expectedBytes: nil,
			assertStats: func(t *testing.T, stats *statstest.TestHandlerStats) {
				assert.Equal(t, uint32(1), stats.CodeRequestCount)
				assert.Equal(t, uint32(1), stats.DuplicateHashesRequested)
				assert.Equal(t, uint32(0), stats.TooManyHashesRequested)
				assert.Equal(t, uint32(0), stats.MissingCodeHashCount)
				assert.Equal(t, uint32(0), stats.CodeBytesReturnedSum)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			db := rawdb.NewMemoryDatabase()

			assert.Equal(t, len(test.codeBytes), len(test.codeHashes))
			codeMap := make(map[common.Hash][]byte)
			for i, codeBytes := range test.codeBytes {
				if codeBytes != nil {
					codeMap[test.codeHashes[i]] = codeBytes
					rawdb.WriteCode(db, test.codeHashes[i], codeBytes)
				}
			}

			codeRequest := message.CodeRequest{
				Hashes: test.codeHashes,
			}

			testStats := &statstest.TestHandlerStats{}
			codeRequestHandler := NewCodeRequestHandler(db, message.Codec, testStats)
			responseBytes, err := codeRequestHandler.OnCodeRequest(context.Background(), ids.GenerateTestNodeID(), 1, codeRequest)
			assert.NoError(t, err)

			if test.expectedBytes == nil {
				assert.Nil(t, responseBytes)
			} else {
				var response message.CodeResponse
				_, err = message.Codec.Unmarshal(responseBytes, &response)
				assert.NoError(t, err)
				assert.Equal(t, test.expectedBytes, response.Data)
			}

			test.assertStats(t, testStats)
		})
	}
}

func TestIsUnique(t *testing.T) {
	tests := map[string]struct {
		hashes       []common.Hash
		expectUnique bool
	}{
		"unique": {
			hashes:       []common.Hash{{1}, {2}, {3}},
			expectUnique: true,
		},
		"not unique": {
			hashes:       []common.Hash{{1}, {2}, {1}},
			expectUnique: false,
		},
		"empty": {
			hashes:       []common.Hash{},
			expectUnique: true,
		},
		"single": {
			hashes:       []common.Hash{{1}},
			expectUnique: true,
		},
		"all same": {
			hashes:       []common.Hash{{1}, {1}, {1}},
			expectUnique: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expectUnique, isUnique(test.hashes))
		})
	}
}
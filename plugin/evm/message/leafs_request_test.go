// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package message

import (
	"bytes"
	"context"
	"encoding/base64"
	"math/rand"
	"testing"

	"github.com/luxfi/ids"

	"github.com/luxfi/geth/common"
	"github.com/stretchr/testify/assert"
)

// TestMarshalLeafsRequest asserts that the structure or serialization logic hasn't changed, primarily to
// ensure compatibility with the network.
func TestMarshalLeafsRequest(t *testing.T) {
	// generate some random code data
	// use deterministic random source for testing
	r := rand.New(rand.NewSource(1))

	startBytes := make([]byte, common.HashLength)
	endBytes := make([]byte, common.HashLength)

	_, err := r.Read(startBytes)
	assert.NoError(t, err)

	_, err = r.Read(endBytes)
	assert.NoError(t, err)

	leafsRequest := LeafsRequest{
		Root:  common.BytesToHash([]byte("im ROOTing for ya")),
		Start: startBytes,
		End:   endBytes,
		Limit: 1024,
	}

	base64LeafsRequest := "AAAAAAAAAAAAAAAAAAAAAABpbSBST09UaW5nIGZvciB5YQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAIAAAAFL9/AchgmVPFj9fD5piHXKVZsdNEAN8TXu7BAfR4sZJIAAAAIGFWthoHQ2G0ekeABZ5OctmlNLEIqzSCKAHKTlIf2mZAAQ="

	leafsRequestBytes, err := Codec.Marshal(Version, leafsRequest)
	assert.NoError(t, err)
	assert.Equal(t, base64LeafsRequest, base64.StdEncoding.EncodeToString(leafsRequestBytes))

	var l LeafsRequest
	_, err = Codec.Unmarshal(leafsRequestBytes, &l)
	assert.NoError(t, err)
	assert.Equal(t, leafsRequest.Root, l.Root)
	assert.Equal(t, leafsRequest.Start, l.Start)
	assert.Equal(t, leafsRequest.End, l.End)
	assert.Equal(t, leafsRequest.Limit, l.Limit)
}

// TestMarshalLeafsResponse asserts that the structure or serialization logic hasn't changed, primarily to
// ensure compatibility with the network.
func TestMarshalLeafsResponse(t *testing.T) {
	// generate some random code data
	// use deterministic random source for testing
	r := rand.New(rand.NewSource(1))

	keysBytes := make([][]byte, 16)
	valsBytes := make([][]byte, 16)
	for i := range keysBytes {
		keysBytes[i] = make([]byte, common.HashLength)
		valsBytes[i] = make([]byte, r.Intn(8)+8) // min 8 bytes, max 16 bytes

		_, err := r.Read(keysBytes[i])
		assert.NoError(t, err)
		_, err = r.Read(valsBytes[i])
		assert.NoError(t, err)
	}

	nextKey := make([]byte, common.HashLength)
	_, err := r.Read(nextKey)
	assert.NoError(t, err)

	proofVals := make([][]byte, 4)
	for i := range proofVals {
		proofVals[i] = make([]byte, r.Intn(8)+8) // min 8 bytes, max 16 bytes

		_, err = r.Read(proofVals[i])
		assert.NoError(t, err)
	}

	leafsResponse := LeafsResponse{
		Keys:      keysBytes,
		Vals:      valsBytes,
		More:      true,
		ProofVals: proofVals,
	}

	base64LeafsResponse := "AAAQAAAAIAAAAE8WP18PmmIdcpVmx00QA3xNe7sEB9HixkmBhVrYaB0NIAAAAGagByk5SH9pmeudGKRHhARdh/PGfPInRumVr1olNnlRIAAAAK2zfFghtmgLTnyLdjobHUnUlVyEhiFjJSU/7HON16niIAAAAIYVu9oIMfUFmHWSHmaKW98sf8SERZLSVyvNBmjS1sUvIAAAAHHb2Wiw9xcu2FeUuzWLDDtSXaF4b5//CUJ52xlE69ehIAAAAPhMiSs77qX090OR9EXRWv1ClAQDdPaSS5jL+HE/jZYtIAAAAMr8yuOmvI+effHZKTM/+ZOTO+pvWzr23gN0NmxHGeQ6IAAAABZZpE856x5YScYHfbtXIvVxeiiaJm+XZHmBmY6+qJwLIAAAAHOq53hmZ/fpNs1PJKv334ZrqlYDg2etYUXeHuj0qLCZIAAAAHiN5WOvpGfUnexqQOmh0AfwM8KCMGG90Oqln45NpkMBIAAAAKAQ13yW6oCnpmX2BvamO389/SVnwYl55NYPJmhtm/L7IAAAAAfuKbpk+Eq0PKDG5rkcH9O+iZBDQXnTr0SRo2kBLbktIAAAALsXyQKL6ZFOt2ScbJNHgAl50YMDVvKlTD3qsqS0R11jIAAAAOqxOTXzHYRIRRfpJK73iuFRwAdVklg2twdYhWUMMOwpIAAAAHnqPf5BNqv3UrO4Jx0D6USzyds2a3UEX479adIq5UEZIAAAADLWEMqsbjP+qjJjo5lDcCS6nJsUZ4onTwGpEK4pX277EAAAAAkAAACG0ekeABZ5OcsMAAAAuqL/bNRxxIPxX7kLCgAAAIv5IRGcFg8HAkQIAAAAUFTi0INr+EwOAAAAnQ97usvgJVqlt9RL7EAJAAAAfI0BkZLCQiTiCwAAABsGfYm8fwHx9XOYDQAAAEs3OXARXoLtb0ElyPoKAAAAPr34iDoK2L6cOQoAAAAFIg0LKWiLc0uOCAAAACbJAf81TN4WDAAAABhPw50XNP9XFkKJUwsAAACvvo+1aYfHf1gYUgoAAACjcDk0v1CijaECDgAAAEfLVT12lCZ670686kBrDQAAAP5fWr9EzN4mO1YGYz4EAAAADgAAAFcyXwVWMEo+Pq4Uwo0MDQAAAOo50qHks46vP0TGxu8OAAAAg2Ly9WQIVMFd/KyqiiwLAAAA7M5aOpS00zilFD4="

	leafsResponseBytes, err := Codec.Marshal(Version, leafsResponse)
	assert.NoError(t, err)
	assert.Equal(t, base64LeafsResponse, base64.StdEncoding.EncodeToString(leafsResponseBytes))

	var l LeafsResponse
	_, err = Codec.Unmarshal(leafsResponseBytes, &l)
	assert.NoError(t, err)
	assert.Equal(t, leafsResponse.Keys, l.Keys)
	assert.Equal(t, leafsResponse.Vals, l.Vals)
	assert.False(t, l.More) // make sure it is not serialized
	assert.Equal(t, leafsResponse.ProofVals, l.ProofVals)
}

func TestLeafsRequestValidation(t *testing.T) {
	mockRequestHandler := &mockHandler{}

	tests := map[string]struct {
		request        LeafsRequest
		assertResponse func(t *testing.T)
	}{
		"node type StateTrieNode": {
			request: LeafsRequest{
				Root:  common.BytesToHash([]byte("some hash goes here")),
				Start: bytes.Repeat([]byte{0x00}, common.HashLength),
				End:   bytes.Repeat([]byte{0xff}, common.HashLength),
				Limit: 10,
			},
			assertResponse: func(t *testing.T) {
				assert.True(t, mockRequestHandler.handleStateTrieCalled)
				assert.False(t, mockRequestHandler.handleBlockRequestCalled)
				assert.False(t, mockRequestHandler.handleCodeRequestCalled)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _ = test.request.Handle(context.Background(), ids.GenerateTestNodeID(), 1, mockRequestHandler)
			test.assertResponse(t)
			mockRequestHandler.reset()
		})
	}
}

var _ RequestHandler = (*mockHandler)(nil)

type mockHandler struct {
	handleStateTrieCalled,
	handleBlockRequestCalled,
	handleCodeRequestCalled bool
}

func (m *mockHandler) HandleStateTrieLeafsRequest(context.Context, ids.NodeID, uint32, LeafsRequest) ([]byte, error) {
	m.handleStateTrieCalled = true
	return nil, nil
}

func (m *mockHandler) HandleBlockRequest(context.Context, ids.NodeID, uint32, BlockRequest) ([]byte, error) {
	m.handleBlockRequestCalled = true
	return nil, nil
}

func (m *mockHandler) HandleCodeRequest(context.Context, ids.NodeID, uint32, CodeRequest) ([]byte, error) {
	m.handleCodeRequestCalled = true
	return nil, nil
}

func (m *mockHandler) reset() {
	m.handleStateTrieCalled = false
	m.handleBlockRequestCalled = false
	m.handleCodeRequestCalled = false
}

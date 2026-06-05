// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package message

import (
	"context"
	"fmt"

	"github.com/luxfi/ids"
)

// Request represents a Network request type
type Request interface {
	// Requests should implement String() for logging.
	fmt.Stringer

	// Handle allows `Request` to call respective methods on handler to handle
	// this particular request type
	Handle(ctx context.Context, nodeID ids.NodeID, requestID uint32, handler RequestHandler) ([]byte, error)
}

// RequestMarshaler is the minimal surface RequestToBytes needs from the
// package codec. The concrete *manager satisfies it; callers should pass
// [Codec].
type RequestMarshaler interface {
	Marshal(version uint16, source interface{}) ([]byte, error)
}

// RequestToBytes marshals the given request object into bytes. The
// `marshaler` argument exists for symmetry with the legacy codec.Manager
// signature; in-tree callers always pass [Codec].
func RequestToBytes(marshaler RequestMarshaler, request Request) ([]byte, error) {
	return marshaler.Marshal(Version, &request)
}

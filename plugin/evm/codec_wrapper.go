// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"github.com/luxfi/evm/v2/iface"
	"github.com/luxfi/node/v2/codec"
)

// codecWrapper wraps a node codec.Manager to implement iface.Codec
type codecWrapper struct {
	codec codec.Manager
}

// newCodecWrapper creates a new codec wrapper
func newCodecWrapper(codec codec.Manager) iface.Codec {
	return &codecWrapper{codec: codec}
}

// Marshal implements iface.Codec
func (c *codecWrapper) Marshal(v interface{}) ([]byte, error) {
	// Use version 0 for marshaling
	return c.codec.Marshal(0, v)
}

// Unmarshal implements iface.Codec
func (c *codecWrapper) Unmarshal(b []byte, v interface{}) error {
	_, err := c.codec.Unmarshal(b, v)
	return err
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package messages

import (
	"errors"
	"fmt"
)

var errWrongType = errors.New("wrong payload type")

// Payload provides a common interface for all payloads implemented by this
// package.
type Payload interface {
	// Bytes returns the binary representation of this payload.
	Bytes() []byte

	// initialize the payload with the provided binary representation.
	initialize(b []byte)
}

// Parse dispatches on the type discriminator and decodes the matching
// payload shape. Hand-rolled binary, no codec.Manager.
func Parse(b []byte) (Payload, error) {
	typ, err := peekType(b)
	if err != nil {
		return nil, err
	}
	switch typ {
	case TypeValidatorUptime:
		v := &ValidatorUptime{}
		if err := unmarshalValidatorUptime(b, v); err != nil {
			return nil, err
		}
		v.initialize(b)
		return v, nil
	default:
		return nil, fmt.Errorf("%w: unknown type %d", errInvalidType, typ)
	}
}

// initialize encodes p to its wire bytes and stores them on p so
// subsequent Bytes() calls are zero-alloc.
func initialize(p Payload) error {
	var (
		bytes []byte
		err   error
	)
	switch x := p.(type) {
	case *ValidatorUptime:
		bytes, err = marshalValidatorUptime(x)
	default:
		return fmt.Errorf("couldn't marshal %T payload: unknown type", p)
	}
	if err != nil {
		return fmt.Errorf("couldn't marshal %T payload: %w", p, err)
	}
	p.initialize(bytes)
	return nil
}

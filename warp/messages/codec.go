// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package messages

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/luxfi/constants"
	"github.com/luxfi/ids"
)

const (
	CodecVersion = uint16(0)

	MaxMessageSize = 24 * constants.KiB

	// TypeValidatorUptime is the wire discriminator for ValidatorUptime.
	// Single-payload-family today; bump only when adding new shapes.
	TypeValidatorUptime = uint16(0)

	// Fixed wire layout for ValidatorUptime:
	//   u16 version | u16 type | 32B validation_id | u64 total_uptime
	validatorUptimeLen = 2 + 2 + 32 + 8
)

var (
	errShortBuffer    = errors.New("warp/messages: short buffer")
	errInvalidVersion = errors.New("warp/messages: invalid version")
	errInvalidType    = errors.New("warp/messages: invalid payload type")
	errTooLarge       = errors.New("warp/messages: payload exceeds MaxMessageSize")
)

// marshalValidatorUptime encodes ValidatorUptime to its wire bytes.
// Hand-rolled fixed-layout binary — no codec.Manager dependency.
func marshalValidatorUptime(v *ValidatorUptime) ([]byte, error) {
	out := make([]byte, validatorUptimeLen)
	binary.BigEndian.PutUint16(out[0:2], CodecVersion)
	binary.BigEndian.PutUint16(out[2:4], TypeValidatorUptime)
	copy(out[4:36], v.ValidationID[:])
	binary.BigEndian.PutUint64(out[36:44], v.TotalUptime)
	return out, nil
}

func unmarshalValidatorUptime(b []byte, v *ValidatorUptime) error {
	if uint64(len(b)) > uint64(MaxMessageSize) {
		return errTooLarge
	}
	if len(b) != validatorUptimeLen {
		return fmt.Errorf("%w: got %d want %d", errShortBuffer, len(b), validatorUptimeLen)
	}
	ver := binary.BigEndian.Uint16(b[0:2])
	if ver != CodecVersion {
		return fmt.Errorf("%w: got %d want %d", errInvalidVersion, ver, CodecVersion)
	}
	typ := binary.BigEndian.Uint16(b[2:4])
	if typ != TypeValidatorUptime {
		return fmt.Errorf("%w: got %d want %d", errInvalidType, typ, TypeValidatorUptime)
	}
	copy(v.ValidationID[:], b[4:36])
	v.TotalUptime = binary.BigEndian.Uint64(b[36:44])
	return nil
}

// peekType reads the type discriminator without consuming the buffer.
// Used by Parse to dispatch to the right unmarshaler.
func peekType(b []byte) (uint16, error) {
	if len(b) < 4 {
		return 0, errShortBuffer
	}
	ver := binary.BigEndian.Uint16(b[0:2])
	if ver != CodecVersion {
		return 0, fmt.Errorf("%w: got %d want %d", errInvalidVersion, ver, CodecVersion)
	}
	return binary.BigEndian.Uint16(b[2:4]), nil
}

// validationIDFromBytes is a small helper that converts the bytes
// portion of a wire payload directly into ids.ID. Avoids a heap alloc
// in Parse hot path.
func validationIDFromBytes(b []byte) ids.ID {
	var id ids.ID
	copy(id[:], b)
	return id
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package state

import (
	"errors"
	"fmt"
	"time"

	"github.com/luxfi/utils/wrappers"
)

const (
	// codecVersion is the wire-version byte prefix retained on the disk
	// format. The serialised layout is fixed-shape big-endian:
	//
	//   [u16 version][i64 UpDuration][u64 LastUpdated][20 NodeID]
	//   [u64 Weight][u64 StartTime][u8 IsActive][u8 IsL1Validator]
	//
	// 2 + 8 + 8 + 20 + 8 + 8 + 1 + 1 = 56 bytes.
	codecVersion uint16 = 0

	validatorDataLen = 2 + 8 + 8 + 20 + 8 + 8 + 1 + 1
)

// ErrUnknownVersion is returned when a wire blob carries a codec version
// other than codecVersion. Decomplected from codec.Manager but preserved
// as an exported sentinel because tests assert on it.
var ErrUnknownVersion = errors.New("unknown validator-data codec version")

// marshalValidatorData renders the fixed-shape big-endian wire format
// for validatorData. Total length is validatorDataLen.
func marshalValidatorData(d *validatorData) []byte {
	p := wrappers.Packer{Bytes: make([]byte, 0, validatorDataLen), MaxSize: validatorDataLen}
	p.PackShort(codecVersion)
	p.PackLong(uint64(d.UpDuration))
	p.PackLong(d.LastUpdated)
	p.PackFixedBytes(d.NodeID[:])
	p.PackLong(d.Weight)
	p.PackLong(d.StartTime)
	p.PackBool(d.IsActive)
	p.PackBool(d.IsL1Validator)
	if p.Errored() {
		// MaxSize is sized exactly; programmer error if this trips.
		panic(fmt.Sprintf("marshalValidatorData: %v", p.Err))
	}
	return p.Bytes
}

// unmarshalValidatorData populates d from a wire-format blob. Returns
// ErrUnknownVersion for an unexpected version byte and
// wrappers.ErrInsufficientLength for a truncated buffer. Version is
// checked before length so a known-bad version is reported uniformly
// across short and full-length buffers.
func unmarshalValidatorData(bytes []byte, d *validatorData) error {
	if len(bytes) < 2 {
		return fmt.Errorf("%w: validator data needs at least %d bytes for version, got %d",
			wrappers.ErrInsufficientLength, 2, len(bytes))
	}
	p := wrappers.Packer{Bytes: bytes}
	ver := p.UnpackShort()
	if ver != codecVersion {
		return fmt.Errorf("%w: %d", ErrUnknownVersion, ver)
	}
	if len(bytes) < validatorDataLen {
		return fmt.Errorf("%w: validator data needs %d bytes, got %d",
			wrappers.ErrInsufficientLength, validatorDataLen, len(bytes))
	}
	d.UpDuration = time.Duration(p.UnpackLong())
	d.LastUpdated = p.UnpackLong()
	copy(d.NodeID[:], p.UnpackFixedBytes(20))
	d.Weight = p.UnpackLong()
	d.StartTime = p.UnpackLong()
	d.IsActive = p.UnpackBool()
	d.IsL1Validator = p.UnpackBool()
	if p.Errored() {
		return p.Err
	}
	return nil
}

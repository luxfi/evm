// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gov

import (
	"encoding/binary"

	"github.com/luxfi/geth/core/types"
)

// Signal is one validator's stated preference for one parameter, carried in one
// block. It is a value, not a message to anyone: nothing is sent, nothing is
// acknowledged, and nobody can refuse to relay it, because it rides in a block
// the validator was already producing.
type Signal struct {
	ParamID ParamID
	Value   uint64
}

// The signal occupies types.Header.Nonce — eight bytes that Lux's consensus
// does not read, does not write and does not constrain, but which are covered
// by the block hash and therefore already agreed by every node. Measured on
// mainnet 96369 at twelve heights spanning genesis to 1098193, header.nonce is
// 0x0000000000000000 at every one; header.Extra, by contrast, is 86 bytes and
// fully occupied by the dynamic-fee window and predicate bytes. Nonce is the
// only free, hash-committed, fixed-width field on the block.
//
//	byte 0    magic — 0xA7. Separates a signal from the zero nonce that every
//	          block in the chain's history carries, so activating governance on
//	          an existing chain reads all prior blocks as abstentions.
//	byte 1    ParamID
//	bytes 2-7 value, big-endian uint48
//
// The 48-bit value ceiling (281474976710655) covers every member of Bounds with
// four orders of magnitude to spare. It does not cover a supply cap in wei, and
// that is not an accident: supply is not governable here at any width.
const (
	signalMagic byte   = 0xA7
	MaxValue    uint64 = 1<<48 - 1
)

// Encode renders a signal as a header nonce. Values above MaxValue and IDs
// outside the constitution are rejected rather than truncated — a silently
// truncated signal would be a vote for a number nobody chose.
func (s Signal) Encode() (types.BlockNonce, bool) {
	if s.Value > MaxValue || !Governable(s.ParamID, s.Value) {
		return types.BlockNonce{}, false
	}
	var n types.BlockNonce
	n[0] = signalMagic
	n[1] = byte(s.ParamID)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], s.Value)
	copy(n[2:], buf[2:]) // low 6 bytes
	return n, true
}

// DecodeSignal reads a signal out of a header nonce. The second result is false
// for an abstention: a zero nonce, a nonce from some other use of the field, an
// ID that is not governable, or a value outside its bound. Discarding
// out-of-bounds signals here — before any counting — is what makes a bound
// unwidenable by vote: an illegal value cannot win a tally it never entered.
func DecodeSignal(n types.BlockNonce) (Signal, bool) {
	if n[0] != signalMagic {
		return Signal{}, false
	}
	var buf [8]byte
	copy(buf[2:], n[2:])
	s := Signal{ParamID: ParamID(n[1]), Value: binary.BigEndian.Uint64(buf[:])}
	if !Governable(s.ParamID, s.Value) {
		return Signal{}, false
	}
	return s, true
}

// SignalFromHeader is DecodeSignal over a header, for callers that have one.
func SignalFromHeader(h *types.Header) (Signal, bool) {
	if h == nil {
		return Signal{}, false
	}
	return DecodeSignal(h.Nonce)
}

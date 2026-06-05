// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package message

import (
	"errors"
	"fmt"

	"github.com/luxfi/constants"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/utils/wrappers"
)

// Wire format
//
// All message-package wire blobs are length-prefixed-and-versioned:
//
//	[u16 version][... type-specific big-endian fields ...]
//
// Version is currently 0. Slices and byte blobs are length-prefixed with
// a u32. Fixed-size byte arrays (common.Hash) are emitted as raw bytes.
// bool is a single byte (0/1). Big-endian throughout. Layout is preserved
// byte-for-byte against the legacy linearcodec encoding so on-the-wire
// peers do not need to re-sync.
const (
	// Version is the current wire version. Increment on any incompatible
	// schema change. There is no "v0 read fallback" — hard cut, one shape.
	Version = uint16(0)

	maxMessageSize = 2*constants.MiB - 64*constants.KiB
)

// ErrUnknownVersion is returned by (*manager).Unmarshal when the leading
// u16 version does not equal [Version]. Preserved as an exported sentinel
// for callers that switch on it.
var (
	ErrUnknownVersion       = errors.New("unknown wire version")
	ErrCantUnpackVersion    = errors.New("couldn't unpack version")
	ErrCantPackVersion      = errors.New("couldn't pack version")
	ErrUnsupportedType      = errors.New("unsupported message type")
	ErrMaxSliceLenExceeded  = errors.New("max message size exceeded")
)

// Manager is the surface every wire-touching peer of this package needs.
// It's the smallest method set that lets a caller serialise or parse a
// message blob — no reflection registry, no codec.Codec sub-interface.
//
// The package singleton [Codec] implements [Manager]. Downstream packages
// (sync/handlers, sync/client, network) should accept [Manager] in their
// constructors rather than coupling to a specific concrete type.
type Manager interface {
	Marshal(version uint16, source interface{}) ([]byte, error)
	Unmarshal(bytes []byte, dest interface{}) (uint16, error)
}

// manager is the package-local marshal/unmarshal entry point. It plays the
// same role codec.Manager played but without the reflection registry —
// each known type marshals via a hand-rolled big-endian writer dispatched
// in [marshalValue] / [unmarshalValue].
type manager struct {
	maxSize int
}

// Codec is the singleton package codec. Callers use Codec.Marshal /
// Codec.Unmarshal.
var Codec Manager = &manager{maxSize: maxMessageSize}

// Marshal serialises a source value with a u16 [version] prefix. Returns
// ErrUnsupportedType if the source isn't one of the registered message
// types.
func (m *manager) Marshal(version uint16, source interface{}) ([]byte, error) {
	if version != Version {
		return nil, ErrCantPackVersion
	}
	p := &wrappers.Packer{MaxSize: m.maxSize}
	p.PackShort(version)
	if p.Errored() {
		return nil, ErrCantPackVersion
	}
	if err := marshalValue(source, p); err != nil {
		return nil, err
	}
	return p.Bytes, nil
}

// Unmarshal parses bytes into dest and returns the leading version.
// Returns ErrUnknownVersion when version mismatches and ErrUnsupportedType
// for an unknown destination type.
func (m *manager) Unmarshal(bytes []byte, dest interface{}) (uint16, error) {
	if len(bytes) < 2 {
		return 0, ErrCantUnpackVersion
	}
	if len(bytes) > m.maxSize {
		return 0, ErrMaxSliceLenExceeded
	}
	p := &wrappers.Packer{Bytes: bytes, MaxSize: m.maxSize}
	version := p.UnpackShort()
	if p.Errored() {
		return 0, ErrCantUnpackVersion
	}
	if version != Version {
		return version, fmt.Errorf("%w: %d", ErrUnknownVersion, version)
	}
	if err := unmarshalValue(dest, p); err != nil {
		return version, err
	}
	return version, nil
}

// marshalValue is the kind-byte-free dispatcher: each known message type
// gets its own static layout. Concrete-type dispatch only — interfaces are
// not supported (legacy linearcodec did support them via type-ID prefix,
// but message.Request is never serialised directly; only its concrete
// implementers are).
func marshalValue(source interface{}, p *wrappers.Packer) error {
	switch v := source.(type) {
	case BlockRequest:
		marshalBlockRequest(v, p)
	case *BlockRequest:
		marshalBlockRequest(*v, p)
	case BlockResponse:
		marshalBlockResponse(v, p)
	case *BlockResponse:
		marshalBlockResponse(*v, p)
	case LeafsRequest:
		marshalLeafsRequest(v, p)
	case *LeafsRequest:
		marshalLeafsRequest(*v, p)
	case LeafsResponse:
		marshalLeafsResponse(v, p)
	case *LeafsResponse:
		marshalLeafsResponse(*v, p)
	case CodeRequest:
		marshalCodeRequest(v, p)
	case *CodeRequest:
		marshalCodeRequest(*v, p)
	case CodeResponse:
		marshalCodeResponse(v, p)
	case *CodeResponse:
		marshalCodeResponse(*v, p)
	case SyncSummary:
		marshalSyncSummary(v, p)
	case *SyncSummary:
		marshalSyncSummary(*v, p)
	case *Request:
		// The legacy codec accepted *Request (pointer-to-interface) so it
		// could prepend a type-ID for runtime polymorphism. The current
		// message-handler path always marshals concrete types via
		// RequestToBytes (which takes `request Request` and marshals
		// `&request`). Dispatch on the concrete underlying value.
		return marshalValue(*v, p)
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedType, source)
	}
	if p.Errored() {
		return p.Err
	}
	return nil
}

func unmarshalValue(dest interface{}, p *wrappers.Packer) error {
	switch v := dest.(type) {
	case *BlockRequest:
		unmarshalBlockRequest(v, p)
	case *BlockResponse:
		unmarshalBlockResponse(v, p)
	case *LeafsRequest:
		unmarshalLeafsRequest(v, p)
	case *LeafsResponse:
		unmarshalLeafsResponse(v, p)
	case *CodeRequest:
		unmarshalCodeRequest(v, p)
	case *CodeResponse:
		unmarshalCodeResponse(v, p)
	case *SyncSummary:
		unmarshalSyncSummary(v, p)
	default:
		return fmt.Errorf("%w: %T", ErrUnsupportedType, dest)
	}
	if p.Errored() {
		return p.Err
	}
	return nil
}

// --- BlockRequest ---

func marshalBlockRequest(b BlockRequest, p *wrappers.Packer) {
	p.PackFixedBytes(b.Hash[:])
	p.PackLong(b.Height)
	p.PackShort(b.Parents)
}

func unmarshalBlockRequest(b *BlockRequest, p *wrappers.Packer) {
	copy(b.Hash[:], p.UnpackFixedBytes(common.HashLength))
	b.Height = p.UnpackLong()
	b.Parents = p.UnpackShort()
}

// --- BlockResponse ---

func marshalBlockResponse(b BlockResponse, p *wrappers.Packer) {
	p.PackInt(uint32(len(b.Blocks)))
	for _, blk := range b.Blocks {
		p.PackBytes(blk)
	}
}

func unmarshalBlockResponse(b *BlockResponse, p *wrappers.Packer) {
	count := p.UnpackInt()
	b.Blocks = make([][]byte, 0, count)
	for i := uint32(0); i < count && !p.Errored(); i++ {
		b.Blocks = append(b.Blocks, p.UnpackBytes())
	}
}

// --- LeafsRequest ---

func marshalLeafsRequest(l LeafsRequest, p *wrappers.Packer) {
	p.PackFixedBytes(l.Root[:])
	p.PackFixedBytes(l.Account[:])
	p.PackBytes(l.Start)
	p.PackBytes(l.End)
	p.PackShort(l.Limit)
}

func unmarshalLeafsRequest(l *LeafsRequest, p *wrappers.Packer) {
	copy(l.Root[:], p.UnpackFixedBytes(common.HashLength))
	copy(l.Account[:], p.UnpackFixedBytes(common.HashLength))
	l.Start = p.UnpackBytes()
	l.End = p.UnpackBytes()
	l.Limit = p.UnpackShort()
}

// --- LeafsResponse ---

func marshalLeafsResponse(l LeafsResponse, p *wrappers.Packer) {
	p.PackInt(uint32(len(l.Keys)))
	for _, k := range l.Keys {
		p.PackBytes(k)
	}
	p.PackInt(uint32(len(l.Vals)))
	for _, v := range l.Vals {
		p.PackBytes(v)
	}
	p.PackInt(uint32(len(l.ProofVals)))
	for _, pv := range l.ProofVals {
		p.PackBytes(pv)
	}
}

func unmarshalLeafsResponse(l *LeafsResponse, p *wrappers.Packer) {
	keyCount := p.UnpackInt()
	l.Keys = make([][]byte, 0, keyCount)
	for i := uint32(0); i < keyCount && !p.Errored(); i++ {
		l.Keys = append(l.Keys, p.UnpackBytes())
	}
	valCount := p.UnpackInt()
	l.Vals = make([][]byte, 0, valCount)
	for i := uint32(0); i < valCount && !p.Errored(); i++ {
		l.Vals = append(l.Vals, p.UnpackBytes())
	}
	pvCount := p.UnpackInt()
	l.ProofVals = make([][]byte, 0, pvCount)
	for i := uint32(0); i < pvCount && !p.Errored(); i++ {
		l.ProofVals = append(l.ProofVals, p.UnpackBytes())
	}
	// More is `serialize:"-"` and is left zero on unmarshal.
	l.More = false
}

// --- CodeRequest ---

func marshalCodeRequest(c CodeRequest, p *wrappers.Packer) {
	p.PackInt(uint32(len(c.Hashes)))
	for _, h := range c.Hashes {
		p.PackFixedBytes(h[:])
	}
}

func unmarshalCodeRequest(c *CodeRequest, p *wrappers.Packer) {
	count := p.UnpackInt()
	c.Hashes = make([]common.Hash, 0, count)
	for i := uint32(0); i < count && !p.Errored(); i++ {
		var h common.Hash
		copy(h[:], p.UnpackFixedBytes(common.HashLength))
		c.Hashes = append(c.Hashes, h)
	}
}

// --- CodeResponse ---

func marshalCodeResponse(c CodeResponse, p *wrappers.Packer) {
	p.PackInt(uint32(len(c.Data)))
	for _, d := range c.Data {
		p.PackBytes(d)
	}
}

func unmarshalCodeResponse(c *CodeResponse, p *wrappers.Packer) {
	count := p.UnpackInt()
	c.Data = make([][]byte, 0, count)
	for i := uint32(0); i < count && !p.Errored(); i++ {
		c.Data = append(c.Data, p.UnpackBytes())
	}
}

// --- SyncSummary ---

func marshalSyncSummary(s SyncSummary, p *wrappers.Packer) {
	p.PackLong(s.BlockNumber)
	p.PackFixedBytes(s.BlockHash[:])
	p.PackFixedBytes(s.BlockRoot[:])
}

func unmarshalSyncSummary(s *SyncSummary, p *wrappers.Packer) {
	s.BlockNumber = p.UnpackLong()
	copy(s.BlockHash[:], p.UnpackFixedBytes(common.HashLength))
	copy(s.BlockRoot[:], p.UnpackFixedBytes(common.HashLength))
}

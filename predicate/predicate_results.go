// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package predicate

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/luxfi/constants"
	"github.com/luxfi/evm/plugin/evm/upgrade/feewindow"
	"github.com/luxfi/geth/common"
)

const (
	Version        = uint16(0)
	MaxResultsSize = constants.MiB

	hashLen = 32 // common.Hash
	addrLen = 20 // common.Address
)

var (
	errShortBuffer  = errors.New("predicate: short buffer")
	errBadVersion   = errors.New("predicate: invalid version")
	errTooLarge     = errors.New("predicate: results exceed MaxResultsSize")
)

// TxResults is a map of results for each precompile address to the resulting byte array.
type TxResults map[common.Address][]byte

// Results encodes the precompile predicate results included in a block on a per transaction basis.
// Results is not thread-safe.
type Results struct {
	Results map[common.Hash]TxResults
}

func (r Results) GetPredicateResults(txHash common.Hash, address common.Address) []byte {
	results, ok := r.Results[txHash]
	if !ok {
		return nil
	}
	return results[address]
}

// NewResults returns an empty predicate results.
func NewResults() *Results {
	return &Results{
		Results: make(map[common.Hash]TxResults),
	}
}

func NewResultsFromMap(results map[common.Hash]TxResults) *Results {
	return &Results{
		Results: results,
	}
}

// ParseResults parses [b] into predicate results.
//
// Wire format (big-endian throughout):
//
//	u16 version
//	u32 outer_count
//	  per outer: 32B txHash, u32 inner_count
//	    per inner: 20B address, u32 bytes_len, bytes
//
// Hand-rolled binary, no codec.Manager dependency. Forward-only —
// any incompatible schema change requires a Version bump.
func ParseResults(b []byte) (*Results, error) {
	if len(b) < 2 {
		return nil, errShortBuffer
	}
	if uint64(len(b)) > uint64(MaxResultsSize) {
		return nil, errTooLarge
	}
	ver := binary.BigEndian.Uint16(b[0:2])
	if ver != Version {
		return nil, fmt.Errorf("%w: got %d want %d", errBadVersion, ver, Version)
	}
	if len(b) < 6 {
		return nil, errShortBuffer
	}
	outerN := binary.BigEndian.Uint32(b[2:6])
	off := uint64(6)
	res := &Results{Results: make(map[common.Hash]TxResults, outerN)}
	for i := uint32(0); i < outerN; i++ {
		if uint64(len(b)) < off+uint64(hashLen)+4 {
			return nil, errShortBuffer
		}
		var hash common.Hash
		copy(hash[:], b[off:off+hashLen])
		off += hashLen
		innerN := binary.BigEndian.Uint32(b[off : off+4])
		off += 4
		txRes := make(TxResults, innerN)
		for j := uint32(0); j < innerN; j++ {
			if uint64(len(b)) < off+uint64(addrLen)+4 {
				return nil, errShortBuffer
			}
			var addr common.Address
			copy(addr[:], b[off:off+addrLen])
			off += addrLen
			bytesLen := binary.BigEndian.Uint32(b[off : off+4])
			off += 4
			if uint64(len(b)) < off+uint64(bytesLen) {
				return nil, errShortBuffer
			}
			val := make([]byte, bytesLen)
			copy(val, b[off:off+uint64(bytesLen)])
			off += uint64(bytesLen)
			txRes[addr] = val
		}
		res.Results[hash] = txRes
	}
	return res, nil
}

// GetResults returns the byte array results for [txHash] from precompile [address] if available.
func (r *Results) GetResults(txHash common.Hash, address common.Address) []byte {
	txResults, ok := r.Results[txHash]
	if !ok {
		return nil
	}
	return txResults[address]
}

// SetTxResults sets the predicate results for the given [txHash]. Overrides results if present.
func (r *Results) SetTxResults(txHash common.Hash, txResults TxResults) {
	// If there are no tx results, don't store an entry in the map
	if len(txResults) == 0 {
		delete(r.Results, txHash)
		return
	}
	r.Results[txHash] = txResults
}

// DeleteTxResults deletes the predicate results for the given [txHash].
func (r *Results) DeleteTxResults(txHash common.Hash) {
	delete(r.Results, txHash)
}

// Bytes marshals the current state of predicate results.
func (r *Results) Bytes() ([]byte, error) {
	// pre-size: 2 (ver) + 4 (outer) + per-entry overhead.
	size := uint64(6)
	for _, inner := range r.Results {
		size += hashLen + 4
		for _, v := range inner {
			size += addrLen + 4 + uint64(len(v))
		}
	}
	if size > uint64(MaxResultsSize) {
		return nil, errTooLarge
	}
	out := make([]byte, size)
	binary.BigEndian.PutUint16(out[0:2], Version)
	binary.BigEndian.PutUint32(out[2:6], uint32(len(r.Results)))
	off := uint64(6)

	// Sort outer keys for deterministic wire output (matches legacy
	// linearcodec map-entry sort behavior — load-bearing for block
	// hash determinism across validators).
	hashes := make([]common.Hash, 0, len(r.Results))
	for h := range r.Results {
		hashes = append(hashes, h)
	}
	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i][:], hashes[j][:]) < 0
	})

	for _, hash := range hashes {
		inner := r.Results[hash]
		copy(out[off:off+hashLen], hash[:])
		off += hashLen
		binary.BigEndian.PutUint32(out[off:off+4], uint32(len(inner)))
		off += 4

		addrs := make([]common.Address, 0, len(inner))
		for a := range inner {
			addrs = append(addrs, a)
		}
		sort.Slice(addrs, func(i, j int) bool {
			return bytes.Compare(addrs[i][:], addrs[j][:]) < 0
		})
		for _, addr := range addrs {
			val := inner[addr]
			copy(out[off:off+addrLen], addr[:])
			off += addrLen
			binary.BigEndian.PutUint32(out[off:off+4], uint32(len(val)))
			off += 4
			copy(out[off:off+uint64(len(val))], val)
			off += uint64(len(val))
		}
	}
	return out, nil
}

func (r *Results) String() string {
	sb := strings.Builder{}

	sb.WriteString(fmt.Sprintf("PredicateResults: (Size = %d)", len(r.Results)))
	for txHash, results := range r.Results {
		for address, result := range results {
			sb.WriteString(fmt.Sprintf("\n%s    %s: %x", txHash, address, result))
		}
	}

	return sb.String()
}

// ParseResultsFromHeaderExtra parses predicate results from a block header's extra data.
// The extra data format is: [windowData (WindowSize bytes)][predicateResultsData...]
// Returns nil if there are no predicate results (extra data <= WindowSize).
func ParseResultsFromHeaderExtra(extra []byte) (*Results, error) {
	// Check if extra data has predicate results beyond the window
	if len(extra) <= feewindow.WindowSize {
		return nil, nil
	}

	predicateBytes := extra[feewindow.WindowSize:]
	return ParseResults(predicateBytes)
}

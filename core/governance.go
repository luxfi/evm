// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/evm/gov"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
	log "github.com/luxfi/log"
)

// AncestorReader is the slice of the chain governance needs: the ancestors of the
// block being processed. BlockChain, the miner's chain and the test chain maker
// all satisfy it already.
type AncestorReader interface {
	GetHeader(hash common.Hash, number uint64) *types.Header
}

// ApplyGovernance tallies a closed epoch and writes what passed into the
// ParamRegistry. It is the ONLY writer of that account, there is no other, and
// it takes no address: nobody can call it, nobody can stop it, and nobody can
// aim it. It runs on every node, during ordinary block processing, and its
// output lands in the state root like any other write.
//
// It does nothing at all except on the block that closes an epoch, and nothing
// ever before the governance activation timestamp.
//
// Determinism, which is the whole ballgame here — a state write that two nodes
// compute differently is a chain split:
//
//   - The window is the EpochLength headers ending at [parent]. Headers are
//     consensus values, already committed by hash, and every node walking back
//     from the same parent sees the same bytes.
//   - Signals decode by a pure function; illegal and out-of-bounds ones are
//     dropped before counting, identically everywhere.
//   - The tally sorts its output by ParamID, so the writes happen in a fixed
//     order regardless of Go's map iteration.
//   - Any header whose time predates activation is skipped, so a chain that
//     turns governance on mid-life counts only blocks whose producers knew the
//     field had meaning. (On 96369 those bytes are zero at every height sampled
//     from genesis to 1098193, so they would decode as abstentions anyway; the
//     skip does not rely on that.)
//   - A short or broken window — the chain is younger than one epoch, or an
//     ancestor is missing — decides nothing rather than deciding from a partial
//     count.
func ApplyGovernance(
	config *params.ChainConfig,
	chain AncestorReader,
	parent *types.Header,
	blockNumber uint64,
	blockTime uint64,
	statedb *state.StateDB,
) error {
	extra := params.GetExtra(config)
	if extra == nil || !extra.IsGovernance(blockTime) {
		return nil
	}
	if !gov.IsEpochEnd(blockNumber) {
		return nil
	}
	if parent == nil || parent.Number.Uint64()+1 != blockNumber {
		return nil
	}

	window, ok := collectWindow(extra, chain, parent)
	if !ok {
		return nil
	}

	decisions := gov.Tally(window)

	// Record that the epoch was counted even when nothing carried, so a reader
	// can tell "no supermajority" from "governance never ran". SetNonce keeps
	// the account out of the EIP-161 empty-account sweep, which considers only
	// nonce, balance and code — never storage.
	if statedb.GetNonce(gov.RegistryAddress) == 0 {
		statedb.SetNonce(gov.RegistryAddress, 1, tracing.NonceChangeUnspecified)
	}
	adapter := registryState{statedb}
	gov.WriteScalar(adapter, gov.SlotLastAppliedEpoch, gov.EpochOf(blockNumber))
	gov.WriteScalar(adapter, gov.SlotLastAppliedBlock, blockNumber)

	for _, d := range decisions {
		gov.Write(adapter, d.ParamID, d.Value)
		log.Info("governance: parameter changed by validator supermajority",
			"param", gov.Bounds[d.ParamID].Name,
			"value", d.Value,
			"blocks", d.Count,
			"of", gov.EpochLength,
			"needed", gov.Required(gov.EpochLength),
			"epoch", gov.EpochOf(blockNumber),
		)
	}
	return nil
}

// collectWindow walks back EpochLength headers from [parent] inclusive and
// decodes each one's signal. The second result is false when the window cannot
// be formed in full.
func collectWindow(extra interface {
	IsGovernance(uint64) bool
}, chain AncestorReader, parent *types.Header) ([]gov.Signal, bool) {
	if chain == nil {
		return nil, false
	}
	window := make([]gov.Signal, 0, gov.EpochLength)
	h := parent
	for i := uint64(0); i < gov.EpochLength; i++ {
		if h == nil {
			return nil, false // window runs off the front of the chain
		}
		if extra.IsGovernance(h.Time) {
			if s, ok := gov.SignalFromHeader(h); ok {
				window = append(window, s)
			}
		}
		n := h.Number.Uint64()
		if n == 0 {
			if i+1 < gov.EpochLength {
				return nil, false // chain younger than one epoch
			}
			break
		}
		h = chain.GetHeader(h.ParentHash, n-1)
	}
	// The denominator is the whole window, not the signals in it: abstention is
	// a vote against. Pad the tail so Tally divides by EpochLength.
	for uint64(len(window)) < gov.EpochLength {
		window = append(window, gov.Signal{})
	}
	return window, true
}

// registryState adapts state.StateDB to the narrow interface gov writes
// through, so package gov stays free of any dependency on the EVM's state
// machinery and remains testable with a map.
type registryState struct{ db *state.StateDB }

func (s registryState) GetState(a common.Address, k common.Hash) common.Hash {
	return s.db.GetState(a, k)
}

func (s registryState) SetState(a common.Address, k common.Hash, v common.Hash) {
	s.db.SetState(a, k, v)
}

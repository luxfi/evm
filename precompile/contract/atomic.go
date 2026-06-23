// Copyright (C) 2025-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package contract

import (
	"github.com/luxfi/geth/common"
	"github.com/luxfi/ids"
	"github.com/luxfi/vm/chains/atomic"
)

// AtomicState is the OPTIONAL host capability the EVM's AccessibleState
// implements so a stateful precompile can move value across primary-network
// chains via atomic shared memory (the platformvm / dexvm import-export
// primitive). It is the internal twin of
// github.com/luxfi/precompile/contract.AtomicState; the registry bridge forwards
// between the two structurally-identical surfaces.
//
// It is deliberately separate from AccessibleState (which has many mock
// implementers) so only the concrete EVM adapter carries the capability and
// every other precompile is unaffected. A precompile type-asserts it and reverts
// when absent.
type AtomicState interface {
	// AtomicMemory returns this chain's atomic shared-memory handle (nil when the
	// host wired none — single-chain dev / non-atomic harness).
	AtomicMemory() atomic.SharedMemory
	// NetworkID is the numeric network identifier.
	NetworkID() uint32
	// ChainID is this (C-Chain / EVM) chain's id.
	ChainID() ids.ID
	// CChainID is the C-Chain peer id (== ChainID on the C-Chain itself).
	CChainID() ids.ID
	// GovernanceController is the per-network DEX governance authority address — the
	// SOLE caller permitted to toggle the 0x9999 settlement kill switches or seed its
	// pots. A governance CONTRACT resolved by the host from its deployment topology,
	// never a dev-mnemonic EOA. The zero address means no authority is configured, which
	// the precompile treats as fail-closed (halt/seed revert). Internal twin of the
	// external contract.AtomicState method the registry bridge forwards.
	GovernanceController() common.Address
	// DChainID is the D-Chain (dexvm) blockchain id the C<->D atomic seam routes to,
	// resolved by the host from the chain topology (the consensus context's
	// blockchain-alias lookup of "D"). ids.Empty on a network with no dexvm deployed.
	// This is the runtime-resolved D peer for the always-on DEX settlement precompile
	// (0x9999) — zero per-net config; deterministic per network on every validator.
	DChainID() ids.ID
	// TxID is the executing transaction's id.
	TxID() ids.ID
	// CallIndex is this precompile invocation's per-tx ordinal.
	CallIndex() uint32
}

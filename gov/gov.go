// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package gov is validator-stake governance for the values a Lux node reads
// out of chain state.
//
// # The shape of the problem
//
// A parameter is governable exactly when the node READS it from consensus
// state rather than from its own binary. Lux already has that seam and uses it
// in production: BlockChain.GetFeeConfigAt(parent) resolves the fee schedule
// from the PARENT BLOCK'S STATE ROOT, and dummy.verifyHeaderGasFields rejects
// any block that disagrees with it. Whatever is behind that seam is enforced by
// consensus, on every node, with no operator involved. Everything NOT behind
// such a seam — the P-chain emission curve in genesis/pkg/genesis/params.go,
// the 50/50 fee split in core/fee_split.go — is compiled in, and no vote of any
// kind can move it without a new binary and a coordinated flag day. This
// package governs the first category and deliberately does not pretend to
// govern the second.
//
// # The electorate, without an oracle
//
// P-chain stake is not readable from the C-Chain EVM. Every route that makes it
// readable — a validator-set precompile, a warp attestation — must be a
// PREDICATE, because contract.BlockContext (precompile/contract/interfaces.go)
// exposes only Number() and Timestamp(); the P-chain height a lookup would have
// to be pinned to lives in precompileconfig.PredicateContext.ProposerVMBlockCtx
// and nowhere else. A plain stateful precompile calling GetValidatorSet would
// read a value that differs per node and fork the chain. So every EVM-side route
// costs a node release, and then still owes an answer for how a P-chain nodeID
// maps to an EVM voting address.
//
// This package takes the route that needs neither. Lux selects block proposers
// in proportion to validator weight, and that weight already includes every
// delegation (state_validators.go AddWeight, called once per delegator). So a
// validator's share of proposed blocks IS its share of delegated stake, sampled.
// Counting a signal carried in the blocks themselves is therefore stake-weighted
// by construction: no oracle, no identity mapping, no registry of who may vote,
// and nothing to capture. It is the mechanism Bitcoin uses (BIP-9 version bits),
// transplanted to the field Lux leaves free.
//
// # No admin, by construction
//
// Nothing in this package takes an address. There is no allowlist, no owner, no
// role, no proposer privilege, no veto, no timelock held by anyone. The only
// input is the header chain; the only output is a storage write the node makes
// to itself. A validator's entire power is to set eight bytes in the blocks it
// was already going to produce, and to be ignored unless a supermajority of a
// whole epoch's blocks agrees.
//
// The FeeManager precompile also exposes fee config through the same read seam,
// but it gates writes behind allowlist.AdminAddresses — precisely the key the
// owner forbids. This package reuses the SEAM and refuses the ALLOWLIST.
//
// # What a supermajority still cannot do
//
// Bounds are compiled in (see Bounds). A signal for a value outside them is not
// merely outvoted, it is never counted. A 100% cartel cannot set GasLimit to
// zero, cannot set MinBaseFee to zero, cannot mint, cannot move funds, and
// cannot widen the bounds. Moving a bound is a node release — deliberately, so
// that the blast radius of capturing the vote is bounded by code review rather
// than by trust.
package gov

// EpochLength is the number of blocks in one tally window, and simultaneously
// the denominator of the threshold. Abstention counts against a change, exactly
// as an unset version bit does in BIP-9: a quiet epoch changes nothing.
//
// 2016 is Bitcoin's retarget window. At the C-Chain's ~2s block time that is
// ~67 minutes, long enough that per-validator block share concentrates tightly
// around stake share (for 5 equal-weight validators, sd of one validator's
// share over 2016 blocks is sqrt(2016*0.2*0.8)/2016 = 0.89%), and short enough
// that a decision is not a season away.
const EpochLength uint64 = 2016

// Threshold is 3/4 of EpochLength — 1512 of 2016 blocks.
//
// Chosen so the decision boundary sits in the wide gap between coalition sizes
// rather than near one. With 5 equal-weight validators (the measured mainnet
// shape), 3 signalling validators land at 60% +/- 1.1% and 4 land at 80% +/-
// 0.9%; 75% separates them by more than 13 standard deviations, so the outcome
// is a fact about who agreed, never about which way the sampling fell.
const (
	ThresholdNum uint64 = 3
	ThresholdDen uint64 = 4
)

// IsEpochEnd reports whether the block at [number] closes a tally window.
// Genesis closes nothing.
func IsEpochEnd(number uint64) bool {
	return number != 0 && number%EpochLength == 0
}

// EpochOf returns the index of the window that the block at [number] closes.
func EpochOf(number uint64) uint64 { return number / EpochLength }

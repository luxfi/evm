// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"sync"
	"testing"

	"github.com/luxfi/geth/common"
)

// The builder must not build while the transaction pool is still catching up to the
// chain's head.
//
// In that window the pool still lists the transactions the newly accepted block just
// mined, so "is there work?" answers yes; the miner then assembles against the NEW
// head, finds nothing left, and returns an empty block — which the anti-empty policy
// discards. Nothing has changed, so the builder is woken again immediately. Measured on
// lux-devnet 2026-08-13: 948 discarded builds in ten minutes on a HEALTHY chain.
//
// This barrier is upstream's and we had dropped it, along with the two hashes it
// compares. It is restored rather than reinvented.
func TestPendingPoolUpdateGatesBuilding(t *testing.T) {
	newBuilder := func() *blockBuilder {
		b := &blockBuilder{}
		b.pendingSignal = sync.NewCond(&b.buildBlockLock)
		return b
	}

	head := common.HexToHash("0xaa")
	next := common.HexToHash("0xbb")

	t.Run("converged: nothing blocks the build", func(t *testing.T) {
		b := newBuilder()
		b.setChainHeadHash(head)
		b.setMempoolHeadHash(head)
		if b.pendingPoolUpdate() {
			t.Fatal("both views agree on the head, so nothing should be pending")
		}
	})

	t.Run("chain moved, pool has not: hold", func(t *testing.T) {
		b := newBuilder()
		b.setChainHeadHash(head)
		b.setMempoolHeadHash(head)
		b.setChainHeadHash(next) // a block was accepted
		if !b.pendingPoolUpdate() {
			t.Fatal("the pool has not reset onto the new head yet — building here is the spin")
		}
	})

	t.Run("pool catches up: release", func(t *testing.T) {
		b := newBuilder()
		b.setChainHeadHash(next)
		b.setMempoolHeadHash(head)
		if !b.pendingPoolUpdate() {
			t.Fatal("still behind")
		}
		b.setMempoolHeadHash(next) // the pool's reorg event lands
		if b.pendingPoolUpdate() {
			t.Fatal("the pool reported the new head, so the builder must be released")
		}
	})
}

// A builder whose hashes were never set starts converged (both zero), so a node that
// has produced no blocks yet is not held closed waiting for an event that only a block
// can cause. Initialising them to the current head at startup keeps that true for a
// node restarting onto an existing chain.
func TestBarrierStartsConverged(t *testing.T) {
	b := &blockBuilder{}
	b.pendingSignal = sync.NewCond(&b.buildBlockLock)
	if b.pendingPoolUpdate() {
		t.Fatal("a fresh builder must not be gated, or it can never build its first block")
	}
}

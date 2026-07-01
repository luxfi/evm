// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
)

// newWakeTestBuilder builds a blockBuilder wired with an injectable pending-tx
// predicate and only the bounded-wake backstop running (no tx-submit
// subscription). Starting only the backstop models a lost/missed submit
// notification, so the periodic re-poll is the sole way a parked builder can
// discover a pending tx — which is exactly the property under test.
func newWakeTestBuilder(pending func() bool) (*blockBuilder, func()) {
	shutdown := make(chan struct{})
	b := &blockBuilder{
		shutdownChan: shutdown,
		shutdownWg:   &sync.WaitGroup{},
		pendingTxs:   pending,
	}
	b.pendingSignal = sync.NewCond(&b.buildBlockLock)
	b.startPendingTxPoll()
	return b, func() { close(shutdown) }
}

// TestBuilderWakesOnPendingTxAfterIdle is the fail-on-old / pass-on-fix
// regression for the idle-builder wake race. It reproduces the node bridge's
// exact call — waitForEvent with a past earliestBuild (so pacing adds no delay)
// — on an idle builder, then delivers a pending tx with NO pendingSignal
// broadcast. On the pre-fix code the only wake source is the tx-submit
// broadcast; a broadcast delivered to no parked waiter is lost, so a builder
// that parks afterward never re-polls and the tx sits pending indefinitely
// (this test times out). With the bounded-wake backstop the parked builder
// re-polls the mempool within builderPollInterval and returns PendingTxs.
func TestBuilderWakesOnPendingTxAfterIdle(t *testing.T) {
	var pending atomic.Bool // starts false: mempool empty
	b, stop := newWakeTestBuilder(pending.Load)
	defer stop()

	// earliestBuild in the past => no pacing delay, so the result is bounded by
	// the wake alone, not by the target-block-rate pace.
	earliestBuild := time.Now().Add(-time.Hour)

	got := make(chan block.Message, 1)
	go func() {
		msg, err := b.waitForEvent(context.Background(), earliestBuild)
		if err != nil {
			t.Errorf("waitForEvent: %v", err)
			return
		}
		got <- msg
	}()

	// While the mempool is empty the builder must stay parked: a demand-driven
	// chain must not build empty blocks.
	select {
	case <-got:
		t.Fatal("waitForEvent returned before any pending tx (idle build)")
	case <-time.After(100 * time.Millisecond):
	}

	// A tx arrives, but no submit-signal broadcast is delivered for it — only
	// the bounded-wake backstop can wake the parked builder.
	pending.Store(true)

	select {
	case msg := <-got:
		if msg.Type != block.PendingTxs {
			t.Fatalf("got message type %v, want PendingTxs", msg.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("builder did not wake on pending tx after idle within bound (lost wakeup)")
	}
}

// TestBuilderPollBackstopStopsOnShutdown asserts the backstop goroutine exits on
// shutdown (no leak) so the node-lifetime ticker is correctly torn down.
func TestBuilderPollBackstopStopsOnShutdown(t *testing.T) {
	b, stop := newWakeTestBuilder(func() bool { return false })

	done := make(chan struct{})
	go func() {
		b.shutdownWg.Wait()
		close(done)
	}()

	stop()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pending-tx poll goroutine did not exit on shutdown")
	}
}

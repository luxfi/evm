// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/core/txpool"
	log "github.com/luxfi/log"
)

const (
	// Retry floor after a build attempt, so a BuildBlock that fails (e.g. the
	// block gas cost is momentarily uncoverable) does not hot-loop. Primary
	// pacing is to the target block rate (see waitForEvent); this only bounds
	// the retry cadence when the target-rate time has already passed.
	minBlockBuildingRetryDelay = 100 * time.Millisecond

	// builderPollInterval bounds how long a parked waitForNeedToBuild can sleep
	// before it re-polls the mempool, independent of the tx-submit subscription.
	// The subscription (awaitSubmittedTxs) is the primary wake; this periodic
	// re-poll is the backstop that closes the subscribe-gap window — a tx that
	// arrives while no builder is parked (between two WaitForEvent calls, or
	// while the proposervm is sleeping out its slot) broadcasts to no waiter and
	// is otherwise only caught on the next needToBuild() poll. With this backstop
	// a tx-after-idle ALWAYS wakes a parked builder within builderPollInterval,
	// so a demand-driven chain mines the first tx within a bounded time with no
	// keep-warm hack. It only ever produces a build when needToBuild() is true,
	// so idle chains stay empty-block-free; it changes wake TIMING only, never
	// block CONTENTS, so validators still build identical blocks.
	builderPollInterval = 500 * time.Millisecond
)

type blockBuilder struct {
	ctx context.Context

	txPool *txpool.TxPool

	shutdownChan <-chan struct{}
	shutdownWg   *sync.WaitGroup

	pendingSignal *sync.Cond

	buildBlockLock sync.Mutex

	// lastBuildTime is the time when the last block was built.
	// This is used to ensure that we don't build blocks too frequently,
	// but at least after a minimum delay of minBlockBuildingRetryDelay.
	lastBuildTime time.Time

	// pendingTxs reports whether the mempool holds at least one buildable
	// transaction. It is the single source of truth for needToBuild and is
	// injectable so the wake path can be tested without a live txpool. Set by
	// NewBlockBuilder to poll the real pool.
	pendingTxs func() bool
}

func (vm *VM) NewBlockBuilder() *blockBuilder {
	b := &blockBuilder{
		ctx:          context.Background(),
		txPool:       vm.txPool,
		shutdownChan: vm.shutdownChan,
		shutdownWg:   &vm.shutdownWg,
	}
	// Use empty filter to check if ANY pending transactions exist. The miner
	// applies proper fee filters when building; a MinTip filter here would
	// reject valid legacy transactions (no GasTipCap) and stall production.
	b.pendingTxs = func() bool {
		return b.txPool.PendingSize(txpool.PendingFilter{}) > 0
	}
	b.pendingSignal = sync.NewCond(&b.buildBlockLock)
	return b
}

// handleGenerateBlock is called from the VM immediately after BuildBlock.
func (b *blockBuilder) handleGenerateBlock() {
	b.buildBlockLock.Lock()
	defer b.buildBlockLock.Unlock()
	b.lastBuildTime = time.Now()
}

// needToBuild returns true if there are outstanding transactions to be issued
// into a block.
func (b *blockBuilder) needToBuild() bool {
	return b.pendingTxs()
}

// signalCanBuild signals that a new block can be built.
func (b *blockBuilder) signalCanBuild() {
	b.buildBlockLock.Lock()
	defer b.buildBlockLock.Unlock()
	b.pendingSignal.Broadcast()
}

// awaitSubmittedTxs waits for new transactions to be submitted
// and notifies the VM when the tx pool has transactions to be
// put into a new block.
func (b *blockBuilder) awaitSubmittedTxs() {
	// txSubmitChan is invoked when new transactions are issued as well as on re-orgs which
	// may orphan transactions that were previously in a preferred block.
	txSubmitChan := make(chan core.NewTxsEvent)
	b.txPool.SubscribeTransactions(txSubmitChan, true)

	// Bounded-wake backstop: guarantees a parked builder re-polls the mempool
	// even if a tx-submit notification is ever delivered to no waiter.
	b.startPendingTxPoll()

	b.shutdownWg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic in awaitSubmittedTxs", "error", r)
				panic(r)
			}
		}()
		defer b.shutdownWg.Done()

		for {
			select {
			case <-txSubmitChan:
				log.Trace("New tx detected, trying to generate a block")
				b.signalCanBuild()
			case <-b.shutdownChan:
				return
			}
		}
	}()
}

// startPendingTxPoll starts the bounded-wake backstop: a node-lifetime ticker
// that periodically broadcasts pendingSignal so any builder parked in
// waitForNeedToBuild re-evaluates needToBuild() (which polls the live mempool)
// at least every builderPollInterval. This closes the subscribe-gap window
// without a keep-warm hack: a tx that arrived while no builder was parked — so
// the awaitSubmittedTxs broadcast reached no waiter — is picked up on the next
// tick rather than sitting pending indefinitely. It is decomplected from the
// tx-submit subscription so the two wake sources are independent (and the
// backstop is testable without a live txpool).
func (b *blockBuilder) startPendingTxPoll() {
	b.shutdownWg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic in pending-tx poll", "error", r)
				panic(r)
			}
		}()
		defer b.shutdownWg.Done()

		ticker := time.NewTicker(builderPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.signalCanBuild()
			case <-b.shutdownChan:
				return
			}
		}
	}()
}

// waitForEvent waits until a block needs to be built, then paces the signal so
// the next block is not built before [earliestBuild]. earliestBuild is
// parent.Time + TargetBlockRate: building earlier makes the block gas cost rise
// (it scales with how far ahead of the target rate a block lands), and a block
// whose transactions cannot cover that cost is rejected by verifyBlockFee and
// never mines — so a single small-tip tx would stall forever without this pace.
// Pacing to the target rate also gives the mempool time to gossip-converge
// across validators, so they build identical blocks (same ID) instead of racing
// to finalize conflicting blocks at the same height. A short retry floor keeps a
// failed BuildBlock from hot-looping when earliestBuild has already passed.
func (b *blockBuilder) waitForEvent(ctx context.Context, earliestBuild time.Time) (block.Message, error) {
	lastBuildTime, err := b.waitForNeedToBuild(ctx)
	if err != nil {
		return block.Message{}, err
	}
	timeUntilNextBuild := time.Until(nextBuildTime(earliestBuild, lastBuildTime))
	if timeUntilNextBuild <= 0 {
		return block.Message{Type: block.PendingTxs}, nil
	}
	log.Debug("Pacing block build to the target block rate", "timeUntilNextBuild", timeUntilNextBuild)
	select {
	case <-ctx.Done():
		return block.Message{}, ctx.Err()
	case <-time.After(timeUntilNextBuild):
		return block.Message{Type: block.PendingTxs}, nil
	}
}

// nextBuildTime is the earliest wall-clock instant the next block may be built.
// It is the later of two floors:
//   - earliestBuild: parent.Time + TargetBlockRate — building before this raises
//     the block's required fee, so a small-tip tx cannot cover it and the block
//     is rejected by verifyBlockFee (the stall this pacing exists to prevent).
//   - lastBuildTime + minBlockBuildingRetryDelay: a retry floor that keeps a
//     failed BuildBlock from hot-looping once earliestBuild has already passed.
//
// A zero lastBuildTime (no block built this session yet) means only earliestBuild
// applies. Pure function of its inputs (no clock read) so the pacing decision is
// deterministically testable; the wall-clock comparison stays in waitForEvent.
func nextBuildTime(earliestBuild, lastBuildTime time.Time) time.Time {
	if lastBuildTime.IsZero() {
		return earliestBuild
	}
	if retry := lastBuildTime.Add(minBlockBuildingRetryDelay); retry.After(earliestBuild) {
		return retry
	}
	return earliestBuild
}

// waitForNeedToBuild waits until needToBuild returns true.
// It returns the last time a block was built.
func (b *blockBuilder) waitForNeedToBuild(ctx context.Context) (time.Time, error) {
	// Start a goroutine to broadcast when context is cancelled
	// This wakes up the Wait() call so we can check context cancellation
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			b.pendingSignal.Broadcast()
		case <-done:
		}
	}()
	defer close(done)

	b.buildBlockLock.Lock()
	defer b.buildBlockLock.Unlock()
	for !b.needToBuild() {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return time.Time{}, ctx.Err()
		default:
		}
		b.pendingSignal.Wait()
	}
	return b.lastBuildTime, nil
}

// AutominingConfig contains configuration for automining.
type AutominingConfig struct {
	// BuildBlock builds a new block and returns it wrapped for consensus.
	// The block must implement both Verify() and Accept() methods.
	BuildBlock func(ctx context.Context) (interface {
		Verify(context.Context) error
		Accept(context.Context) error
	}, error)
	// Interval is the minimum time between block builds.
	Interval time.Duration
}

// startAutomining starts the automining loop that builds and accepts blocks
// immediately when there are pending transactions. After each block, it
// drains any remaining pending transactions by building additional blocks
// until the pool is empty. This ensures rapid multi-tx deploys (e.g., forge
// script broadcasting 17 contract creations) all get mined promptly.
func (b *blockBuilder) startAutomining(config AutominingConfig) {
	// Subscribe to transaction pool events
	txSubmitChan := make(chan core.NewTxsEvent, 256)
	b.txPool.SubscribeTransactions(txSubmitChan, true)

	b.shutdownWg.Add(1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Error("panic in automining", "error", r)
				panic(r)
			}
		}()
		defer b.shutdownWg.Done()

		log.Info("Automining started")

		for {
			select {
			case <-txSubmitChan:
				// Small delay to batch initial burst of transactions
				time.Sleep(config.Interval)

				// Drain any buffered tx events so we don't re-trigger
				drained := 0
			drain:
				for {
					select {
					case <-txSubmitChan:
						drained++
					default:
						break drain
					}
				}
				if drained > 0 {
					log.Info("Automining: drained buffered tx events", "count", drained)
				}

				// Build blocks until no more pending transactions
				for {
					b.automineBlock(config)

					// Check if there are still pending txs
					pending := b.txPool.PendingSize(txpool.PendingFilter{})
					if pending == 0 {
						break
					}
					log.Info("Automining: pending txs remain, building another block", "pending", pending)
					// Brief pause between blocks to let state settle
					time.Sleep(10 * time.Millisecond)
				}

			case <-b.shutdownChan:
				log.Info("Automining stopped")
				return
			}
		}
	}()
}

// automineBlock builds, verifies, and accepts a block immediately.
func (b *blockBuilder) automineBlock(config AutominingConfig) {
	ctx := context.Background()

	// Build the block
	blk, err := config.BuildBlock(ctx)
	if err != nil {
		log.Error("Automining: failed to build block", "err", err)
		return
	}

	// Verify the block before accepting
	if err := blk.Verify(ctx); err != nil {
		log.Error("Automining: failed to verify block", "err", err)
		return
	}

	// Accept the block (after verification)
	if err := blk.Accept(ctx); err != nil {
		log.Error("Automining: failed to accept block", "err", err)
		return
	}

	log.Info("Automining: block built, verified, and accepted")
}

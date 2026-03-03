// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2018 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"sync"

	"github.com/luxfi/evm/core/parallel"
	"github.com/luxfi/geth/core/types"
)

// txSenderCacherRequest is a request for recovering transaction senders with a
// specific signature scheme and caching it into the transactions themselves.
//
// The inc field defines the number of transactions to skip after each recovery,
// which is used to feed the same underlying input array to different threads but
// ensure they process the early transactions fast.
type txSenderCacherRequest struct {
	signer types.Signer
	txs    []*types.Transaction
	inc    int
}

// TxSenderCacher is a helper structure to concurrently ecrecover transaction
// senders from digital signatures on background threads.
type TxSenderCacher struct {
	threads int
	tasks   chan *txSenderCacherRequest

	// synchronization & cleanup
	wg      sync.WaitGroup
	tasksMu sync.RWMutex
}

// NewTxSenderCacher creates a new transaction sender background cacher and starts
// as many processing goroutines as allowed by the GOMAXPROCS on construction.
func NewTxSenderCacher(threads int) *TxSenderCacher {
	cacher := &TxSenderCacher{
		tasks:   make(chan *txSenderCacherRequest, threads),
		threads: threads,
	}
	for i := 0; i < threads; i++ {
		cacher.wg.Add(1)
		go func() {
			defer cacher.wg.Done()
			cacher.cache()
		}()
	}
	return cacher
}

// cache is an infinite loop, caching transaction senders from various forms of
// data structures.
func (cacher *TxSenderCacher) cache() {
	for task := range cacher.tasks {
		for i := 0; i < len(task.txs); i += task.inc {
			types.Sender(task.signer, task.txs[i])
		}
	}
}

// Recover recovers the senders from a batch of transactions and caches them
// back into the same data structures. There is no validation being done, nor
// any reaction to invalid signatures. That is up to calling code later.
//
// GPU fast path: when a GPUAccelerator is available, batch-recover all senders
// on GPU first (~50ms for 47K sigs vs 1600ms on CPU). Any sigs the GPU misses
// fall through to the CPU goroutine pool.
func (cacher *TxSenderCacher) Recover(signer types.Signer, txs []*types.Transaction) {
	cacher.tasksMu.RLock()
	defer cacher.tasksMu.RUnlock()

	if len(txs) == 0 {
		return
	}
	if cacher.tasks == nil {
		return
	}

	// GPU fast path: batch ecrecover on Metal/CUDA
	gpu := parallel.DefaultGPU()
	if gpu.Available() {
		recovered, err := gpu.BatchEcrecover(txs)
		if err == nil && recovered != nil {
			// Pre-cache recovered senders into the transactions.
			// types.Sender checks the cache first, so subsequent CPU calls
			// for already-recovered txs become no-ops.
			for _, tx := range txs {
				if addr, ok := recovered[tx.Hash()]; ok {
					types.CacheSender(signer, tx, addr)
				}
			}
			// If GPU recovered all senders, we're done
			if len(recovered) == len(txs) {
				return
			}
		}
		// Fall through to CPU for any the GPU missed
	}

	// CPU fallback: fan out to goroutines
	tasks := cacher.threads
	if len(txs) < tasks*4 {
		tasks = (len(txs) + 3) / 4
	}
	for i := 0; i < tasks; i++ {
		cacher.tasks <- &txSenderCacherRequest{
			signer: signer,
			txs:    txs[i:],
			inc:    tasks,
		}
	}
}

// Shutdown stops the threads started by newTxSenderCacher
func (cacher *TxSenderCacher) Shutdown() {
	// Hold the lock on tasksMu to make sure we don't close
	// the channel in the middle of Recover, which would
	// cause it to write to a closed channel.
	cacher.tasksMu.Lock()
	defer cacher.tasksMu.Unlock()

	close(cacher.tasks)
	cacher.wg.Wait()
	cacher.tasks = nil
}

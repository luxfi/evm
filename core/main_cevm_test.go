// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package core

import (
	"fmt"
	"os"
	"testing"

	"github.com/luxfi/evm/core/parallel"
)

// TestMain (cevm build) runs the full ./core suite and then prints the aggregate
// cevm shadow-verifier / applier counters. Because core/state_processor.go routes
// EVERY block through parallel.DefaultExecutor().ExecuteBlock — which under -tags
// cevm is the cevmShadowExecutor — these counters are the honest, whole-suite
// tally of how many real test blocks cevm's independent real-MPT process_block
// root byte-matched the Go EVM's header.Root, and among those how many cevm went
// on to APPLY (four-way byte-match against the header, post-state written into the
// Go statedb, receipts returned) versus declined back to Go. Declined blocks fall
// through to the Go EVM with the live statedb untouched, so the suite result is
// unaffected by cevm; applied blocks are byte-identical by construction (the gate
// keeps a writeback only on a full state-root/bloom/receipt-hash/gas match).
//
// The default (!cevm) build keeps the goleak TestMain in main_test.go; this
// variant drops goleak (orthogonal to the cevm reporting run) so the shadow
// tally is the sole added behaviour.
func TestMain(m *testing.M) {
	code := m.Run()
	s := parallel.CevmShadowStats()
	total := s.Processed + s.DeclinedTooLarge + s.DeclinedNoPreimage + s.DeclinedTx + s.Errored
	fmt.Printf("\n===== CEVM SHADOW VERIFIER / APPLIER — whole ./core suite tally =====\n")
	fmt.Printf("cevm shadow enabled : %v\n", parallel.CevmShadowEnabled())
	fmt.Printf("blocks observed     : %d (every block InsertChain/Process ran through ExecuteBlock)\n", total)
	fmt.Printf("  processed (cevm ran process_block) : %d\n", s.Processed)
	fmt.Printf("    agree  (cevm root == Go header.Root, byte-exact) : %d\n", s.Agree)
	fmt.Printf("      applied  (four-way gate passed; cevm EXECUTED the block) : %d\n", s.Applied)
	fmt.Printf("      reverted (gated writeback mismatch → declined to Go)     : %d\n", s.RevertedMismatch)
	fmt.Printf("    disagree                                         : %d\n", s.Disagree)
	fmt.Printf("    finalize-gap (withdrawals move header.Root)      : %d\n", s.FinalizeGap)
	fmt.Printf("  declined-too-large (pre-state over snapshot caps)  : %d\n", s.DeclinedTooLarge)
	fmt.Printf("  declined-no-preimage (address unrecoverable)       : %d\n", s.DeclinedNoPreimage)
	fmt.Printf("  declined-tx (sender unrecoverable)                 : %d\n", s.DeclinedTx)
	fmt.Printf("  errored (process_block failed/panicked)            : %d\n", s.Errored)
	if s.Processed > 0 {
		fmt.Printf("agreement rate over PROCESSED blocks : %d/%d\n", s.Agree, s.Processed)
	}
	if s.Agree > 0 {
		fmt.Printf("apply rate over AGREED blocks        : %d/%d\n", s.Applied, s.Agree)
	}
	fmt.Printf("=====================================================================\n")
	os.Exit(code)
}

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

package parallel

import "testing"

// TestDefaultExecutorIsCevmShadow confirms that under -tags cevm the package
// init() registered the cevm shadow verifier, so parallel.DefaultExecutor()
// (the executor core/state_processor.go:124 calls on EVERY block) is the
// cevmShadowExecutor — NOT the no-op fallbackExecutor. This is the precondition
// for the whole shadow-verification claim: if this fails, cevm never observes a
// single block.
func TestDefaultExecutorIsCevmShadow(t *testing.T) {
	if !CevmShadowEnabled() {
		t.Fatal("CevmShadowEnabled() is false under -tags cevm — the real bridge is not linked")
	}
	exec := DefaultExecutor()
	if _, ok := exec.(cevmShadowExecutor); !ok {
		t.Fatalf("DefaultExecutor() = %T, want parallel.cevmShadowExecutor (cevm shadow not registered)", exec)
	}
	t.Logf("DefaultExecutor() = %T — cevm shadow verifier is wired into the block-execution seam", exec)
}

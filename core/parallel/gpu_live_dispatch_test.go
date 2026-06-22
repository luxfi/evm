// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo && darwin && gpu

// Live-dispatch proof for the GPU ecrecover path.
//
// This exercises the EXACT production gate at sender_cacher.go:106
//   gpu := parallel.DefaultGPU(); if gpu.Available() { ... }
// and the batch recovery at gpu_bridge.go, against the real Metal backend.
//
// It is intentionally NOT a unit test of recoverV (that lives in
// gpu_bridge_test.go). It asserts the integration: the registered bridge
// reports Available()==true (i.e. luxgpu.GetBackend() != CPU) AND that a real
// signed transaction round-trips through luxgpu.BatchEcrecover to the SAME
// sender address that the CPU signer derives. A CPU-fallthrough or a wrong
// Metal kernel would fail the address equality.
//
// Run with the Metal backend reachable (no env needed after the
// plugin_loader self-location fix):
//   CGO_ENABLED=1 go test -tags gpu -run TestGPULiveDispatch ./core/parallel/

package parallel

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
)

func TestGPULiveDispatch(t *testing.T) {
	// 1. The production gate. Before the loader fix this returned false
	//    (gpu_status:"none"); it must now be true with Metal present.
	g := DefaultGPU()
	if !g.Available() {
		t.Fatalf("DefaultGPU().Available() = false; GPU backend not detected. " +
			"The Metal backend plugin was not loaded — the live ecrecover path " +
			"would fall back to CPU (gpu_status:\"none\").")
	}
	t.Logf("DefaultGPU().Available() = true (GPU backend detected)")

	// 2. Build a real EIP-155 signed transaction with a known key.
	//    crypto.PubkeyToAddress yields luxfi/crypto/common.Address; the bridge
	//    and types use luxfi/geth/common.Address. Derive the expected sender via
	//    the geth signer (signer.Sender) so the comparison stays in one type
	//    system and equals exactly what the CPU recovery path would produce.
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	chainID := big.NewInt(1)
	signer := types.LatestSignerForChainID(chainID)
	tx, err := types.SignTx(
		types.NewTransaction(0, common.Address{}, big.NewInt(1), 21000, big.NewInt(1), nil),
		signer, key,
	)
	if err != nil {
		t.Fatalf("SignTx: %v", err)
	}

	// Canonical sender per the CPU signer — the GPU result must match this.
	wantSender, err := signer.Sender(tx)
	if err != nil {
		t.Fatalf("signer.Sender (CPU reference): %v", err)
	}

	// 3. Dispatch through the GPU bridge. This calls luxgpu.BatchEcrecover,
	//    which hits the Metal secp256k1 batch-recover vtable when Available().
	got, err := g.BatchEcrecover([]*types.Transaction{tx})
	if err != nil {
		t.Fatalf("BatchEcrecover (GPU path): %v", err)
	}

	// 4. The GPU-recovered sender MUST equal the key's address. If the Metal
	//    kernel were wrong, or the path silently fell through to a stub, the
	//    address would differ or be absent.
	h := signer.Hash(tx)
	addr, ok := got[h]
	if !ok {
		t.Fatalf("GPU BatchEcrecover returned no result for tx hash %s", h.Hex())
	}
	if addr != wantSender {
		t.Fatalf("GPU-recovered sender = %s, want %s (Metal kernel mismatch or CPU fallthrough)",
			addr.Hex(), wantSender.Hex())
	}
	t.Logf("GPU BatchEcrecover recovered correct sender %s via Metal vtable", addr.Hex())
}

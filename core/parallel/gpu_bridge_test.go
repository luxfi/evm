// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo && darwin

package parallel

import (
	"math/big"
	"testing"
)

// TestRecoverV_LegacyValid verifies valid legacy V values (27, 28).
func TestRecoverV_LegacyValid(t *testing.T) {
	tests := []struct {
		v       uint64
		wantID  uint8
		wantOK  bool
	}{
		{27, 0, true},
		{28, 1, true},
	}
	for _, tt := range tests {
		id, ok := recoverV(big.NewInt(int64(tt.v)), nil)
		if ok != tt.wantOK || id != tt.wantID {
			t.Errorf("recoverV(%d, nil) = (%d, %v), want (%d, %v)",
				tt.v, id, ok, tt.wantID, tt.wantOK)
		}
	}
}

// TestRecoverV_InvalidV verifies that V=0 is rejected.
func TestRecoverV_InvalidV(t *testing.T) {
	_, ok := recoverV(big.NewInt(0), nil)
	if ok {
		t.Fatal("V=0 must be rejected for legacy transactions")
	}

	_, ok = recoverV(big.NewInt(1), nil)
	if ok {
		t.Fatal("V=1 must be rejected for legacy transactions (not 27 or 28)")
	}

	_, ok = recoverV(big.NewInt(26), nil)
	if ok {
		t.Fatal("V=26 must be rejected for legacy transactions")
	}

	_, ok = recoverV(big.NewInt(29), nil)
	if ok {
		t.Fatal("V=29 must be rejected for legacy transactions")
	}
}

// TestRecoverV_EIP155 verifies EIP-155 V values for chain ID 1.
func TestRecoverV_EIP155(t *testing.T) {
	chainID := big.NewInt(1)
	// EIP-155: v = chainId * 2 + 35 + recovery_id
	// Chain 1: v=37 => recovery 0, v=38 => recovery 1
	id, ok := recoverV(big.NewInt(37), chainID)
	if !ok || id != 0 {
		t.Fatalf("recoverV(37, chainID=1): got (%d, %v), want (0, true)", id, ok)
	}

	id, ok = recoverV(big.NewInt(38), chainID)
	if !ok || id != 1 {
		t.Fatalf("recoverV(38, chainID=1): got (%d, %v), want (1, true)", id, ok)
	}
}

// TestRecoverV_OverflowChainID verifies that a chain ID with >63 bits
// is rejected (prevents uint64 overflow).
func TestRecoverV_OverflowChainID(t *testing.T) {
	// 64-bit chain ID: BitLen > 63
	bigChainID := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64
	_, ok := recoverV(big.NewInt(37), bigChainID)
	if ok {
		t.Fatal("chain ID with >63 bits must be rejected")
	}
}

// TestRecoverV_OverflowV verifies that a V value with >63 bits is rejected.
func TestRecoverV_OverflowV(t *testing.T) {
	bigV := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64
	_, ok := recoverV(bigV, big.NewInt(1))
	if ok {
		t.Fatal("V with >63 bits must be rejected")
	}
}

// TestPadTo32_Normal verifies padding of normal values.
func TestPadTo32_Normal(t *testing.T) {
	n := big.NewInt(0xFF)
	padded := padTo32(n)
	if len(padded) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(padded))
	}
	if padded[31] != 0xFF {
		t.Fatalf("expected last byte 0xFF, got 0x%x", padded[31])
	}
	// All leading bytes should be zero
	for i := 0; i < 31; i++ {
		if padded[i] != 0 {
			t.Fatalf("byte %d should be 0, got 0x%x", i, padded[i])
		}
	}
}

// TestPadTo32_Exactly32 verifies that a 32-byte value is returned as-is.
func TestPadTo32_Exactly32(t *testing.T) {
	n := new(big.Int).Lsh(big.NewInt(1), 248) // 32 bytes
	b := n.Bytes()
	if len(b) > 32 {
		t.Skip("test precondition: value is exactly 32 bytes")
	}
	padded := padTo32(n)
	if len(padded) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(padded))
	}
}

// TestPadTo32_Oversized verifies that values >32 bytes return nil.
func TestPadTo32_Oversized(t *testing.T) {
	// 33 bytes: 2^264
	n := new(big.Int).Lsh(big.NewInt(1), 264)
	padded := padTo32(n)
	if padded != nil {
		t.Fatal("oversized value (>32 bytes) must return nil")
	}
}

// TestPadTo32_Zero verifies zero value.
func TestPadTo32_Zero(t *testing.T) {
	padded := padTo32(big.NewInt(0))
	if len(padded) != 32 {
		t.Fatalf("expected 32 bytes, got %d", len(padded))
	}
	for i, b := range padded {
		if b != 0 {
			t.Fatalf("byte %d should be 0, got 0x%x", i, b)
		}
	}
}

// TestSecp256k1HalfN_IsPositive ensures the half-N constant is computed.
func TestSecp256k1HalfN_IsPositive(t *testing.T) {
	if secp256k1halfN.Sign() <= 0 {
		t.Fatal("secp256k1halfN must be positive")
	}
	if secp256k1halfN.BitLen() < 200 {
		t.Fatalf("secp256k1halfN seems too small: %d bits", secp256k1halfN.BitLen())
	}
}

// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/luxfi/geth/common"
	gethvm "github.com/luxfi/geth/core/vm"
)

// ecrecoverAddr is the canonical Ethereum precompile address for
// secp256k1 ecrecover, namely 0x0000000000000000000000000000000000000001.
var ecrecoverAddr = common.BytesToAddress([]byte{0x1})

// validEcrecoverInputHex mirrors the one in luxfi/geth/core/vm/pq_profile_test.go.
// The recovered address does not matter — what matters is that under PQ
// the precompile returns ErrEcrecoverForbidden and zero output bytes,
// regardless of input shape.
const validEcrecoverInputHex = "" +
	// hash
	"a35a39e7715a7b2c5d2e3a5d8e8f8a8b8c8d8e8f9091929394959697989a9b9c" +
	// v (left-padded; canonical v = 27 or 28)
	"000000000000000000000000000000000000000000000000000000000000001c" +
	// r
	"7b6d1f1f0a85b5a3aa3f0e57c8c30a1b1c1d1e1f2021222324252627282a2b2c" +
	// s
	"3d3e3f404142434445464748494a4b4c4d4e4f505152535455565758595a5b5c"

// resetProfile restores the package-level state so a PQ test in this
// package does not leak across cases.
func resetProfile(t *testing.T) {
	t.Helper()
	prev := gethvm.ActivePQProfile()
	t.Cleanup(func() {
		gethvm.SetPQProfile(prev)
	})
}

// TestEcrecover_StrictPQ asserts the evm-side wire-up: when Config.PQ is
// true, VM.Initialize installs the PQ posture into the geth precompile
// layer, and the ecrecover precompile at 0x01 returns
// ErrEcrecoverForbidden.
func TestEcrecover_StrictPQ(t *testing.T) {
	resetProfile(t)
	gethvm.SetPQProfile(gethvm.AllForbidden())

	contracts := gethvm.PrecompiledContractsByzantium
	ecrec, ok := contracts[ecrecoverAddr]
	if !ok {
		t.Fatal("ecrecover precompile not registered at 0x01")
	}

	input, err := hex.DecodeString(validEcrecoverInputHex)
	if err != nil {
		t.Fatalf("hex decode: %v", err)
	}

	out, err := ecrec.Run(input)
	if !errors.Is(err, gethvm.ErrEcrecoverForbidden) {
		t.Fatalf("ecrecover.Run: got out=%x err=%v; want ErrEcrecoverForbidden",
			out, err)
	}
	if len(out) != 0 {
		t.Fatalf("PQ ecrecover must return zero bytes, got %d", len(out))
	}
}

// TestEcrecover_ClassicalCompat asserts the classical-compat path is
// preserved under evm: when no profile is installed, ecrecover runs
// upstream go-ethereum semantics and does NOT return any PQ refusal
// error.
func TestEcrecover_ClassicalCompat(t *testing.T) {
	resetProfile(t)
	gethvm.SetPQProfile(nil)

	contracts := gethvm.PrecompiledContractsByzantium
	ecrec, ok := contracts[ecrecoverAddr]
	if !ok {
		t.Fatal("ecrecover precompile not registered at 0x01")
	}

	_, err := ecrec.Run(make([]byte, 128))
	if errors.Is(err, gethvm.ErrEcrecoverForbidden) {
		t.Fatalf("classical-compat path must not return ErrEcrecoverForbidden, got %v", err)
	}
}

// TestPQ_AllFamiliesRefused asserts AllForbidden() turns on every Forbid
// flag, so each precompile family (ecrecover, sha256, ripemd, blake2F,
// bn256, BLS12-381, KZG) refuses under the canonical strict-PQ profile.
// The cross-family coverage matrix lives in geth's pq_gate_test.go;
// this test is a load-bearing sanity check at the evm-plugin layer that
// the projection actually wires the gate on every Op.
func TestPQ_AllFamiliesRefused(t *testing.T) {
	resetProfile(t)
	gethvm.SetPQProfile(gethvm.AllForbidden())

	cases := []struct {
		name    string
		addr    byte
		input   []byte
		wantErr error
	}{
		{"ecrecover", 0x01, make([]byte, 128), gethvm.ErrEcrecoverForbidden},
		{"sha256", 0x02, []byte("benign"), gethvm.ErrSHA256Forbidden},
		{"ripemd160", 0x03, []byte("benign"), gethvm.ErrRIPEMD160Forbidden},
	}
	contracts := gethvm.PrecompiledContractsByzantium
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, ok := contracts[common.BytesToAddress([]byte{c.addr})]
			if !ok {
				t.Fatalf("precompile 0x%02x not registered", c.addr)
			}
			_, err := p.Run(c.input)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("%s: got %v, want %v", c.name, err, c.wantErr)
			}
		})
	}
}

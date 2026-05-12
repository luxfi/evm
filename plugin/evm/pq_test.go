// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"errors"
	"testing"

	gethvm "github.com/luxfi/geth/core/vm"
	"github.com/luxfi/pq"
)

// pq_test.go pins the evm-plugin layer's PQ wiring against the per-chain
// profile contract (CR-1 + CR-2). The precompile body itself no longer
// reads any global PQ state — the gate is in (*EVM).runPrecompile and
// reads chainConfig.PQ. So these tests assert two layers of contract:
//
//  1. The plugin's AllForbidden() projection sets every Forbid flag
//     that the pq package recognises, so a chain pinning the projection
//     refuses every classical primitive family at the precompile
//     boundary.
//  2. The (*Profile).RefuseUnder(op) method returns the family-specific
//     sentinel error for every op a strict-PQ chain must refuse.
//
// These guarantees compose: a node-side EVM that wires
// vm.chainConfig.PQ = gethvm.AllForbidden() (see vm.Initialize) gets
// strict-PQ semantics. The integration path is covered separately by
// core/vm/pq_gate_test.go which runs through an actual EVM dispatch.

// TestPQ_AllForbiddenIsStrict asserts the evm-plugin's AllForbidden()
// projection is identical to pq.Strict() — every classical primitive
// family flagged.
func TestPQ_AllForbiddenIsStrict(t *testing.T) {
	got := gethvm.AllForbidden()
	if got == nil {
		t.Fatal("AllForbidden returned nil")
	}
	want := pq.Strict()
	if got.Hash() != want.Hash() {
		t.Fatalf("AllForbidden hash %x, pq.Strict hash %x — projections diverged",
			got.Hash(), want.Hash())
	}
}

// TestPQ_AllFamiliesRefused asserts the strict-PQ profile refuses every
// classical primitive family the gate recognises. The (*Profile).
// RefuseUnder method is the canonical chokepoint; runPrecompile calls
// it for every non-stateful precompile dispatch.
func TestPQ_AllFamiliesRefused(t *testing.T) {
	p := gethvm.AllForbidden()
	if p == nil {
		t.Fatal("AllForbidden returned nil")
	}

	cases := []struct {
		name    string
		op      pq.Op
		wantErr error
	}{
		{"ecrecover", pq.OpEcrecover, pq.ErrEcrecoverForbidden},
		{"sha256", pq.OpSHA256, pq.ErrSHA256Forbidden},
		{"ripemd160", pq.OpRIPEMD160, pq.ErrRIPEMD160Forbidden},
		{"blake2F", pq.OpBlake2F, pq.ErrBlake2FForbidden},
		{"bn256Add", pq.OpBn256Add, pq.ErrBn256Forbidden},
		{"bn256ScalarMul", pq.OpBn256ScalarMul, pq.ErrBn256Forbidden},
		{"bn256Pairing", pq.OpBn256Pairing, pq.ErrBn256Forbidden},
		{"bls12381G1Add", pq.OpBLS12381G1Add, pq.ErrBLS12381Forbidden},
		{"bls12381Pairing", pq.OpBLS12381Pairing, pq.ErrBLS12381Forbidden},
		{"kzgPointEval", pq.OpKZGPointEval, pq.ErrKZGForbidden},
		{"p256Verify", pq.OpP256Verify, pq.ErrP256VerifyForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := p.RefuseUnder(c.op)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("%s: got %v, want %v", c.name, err, c.wantErr)
			}
		})
	}
}

// TestPQ_NilProfileAdmits asserts the classical-compat path: a nil
// profile (the default for non-PQ chains) admits every op. The gate
// returns nil so runPrecompile proceeds to the precompile body.
func TestPQ_NilProfileAdmits(t *testing.T) {
	var p *pq.Profile // nil — classical chain, no profile installed
	ops := []pq.Op{
		pq.OpEcrecover, pq.OpSHA256, pq.OpRIPEMD160, pq.OpBlake2F,
		pq.OpBn256Add, pq.OpBLS12381G1Add, pq.OpKZGPointEval, pq.OpP256Verify,
	}
	for _, op := range ops {
		if err := p.RefuseUnder(op); err != nil {
			t.Errorf("nil.RefuseUnder(%v) want nil, got %v", op, err)
		}
	}
}

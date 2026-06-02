// Copyright (C) 2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"testing"

	"github.com/luxfi/geth/core/rawdb"
)

// TestResolveStateScheme captures the contract that fixed the lux-mainnet
// 2026-06-02 chain-creation wedge.
//
// Without this normalisation, an L2 EVM chain with no operator-supplied
// StateScheme would let eth/backend.go's customrawdb.ParseStateSchemeExt
// inherit geth's "path by default for empty DB" behavior, and the VM would
// panic in eth.New because it does not support path mode.
func TestResolveStateScheme(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provided string
		want     string
		wantErr  bool
	}{
		{
			name:     "empty defaults to hash (the fresh-L2 case)",
			provided: "",
			want:     rawdb.HashScheme,
		},
		{
			name:     "hash passes through",
			provided: rawdb.HashScheme,
			want:     rawdb.HashScheme,
		},
		{
			name:     "path is refused",
			provided: rawdb.PathScheme,
			wantErr:  true,
		},
		{
			name:     "unknown passes through (let downstream decide)",
			provided: "firewood",
			want:     "firewood",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveStateScheme(tc.provided)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveStateScheme(%q) = %q, nil; want error", tc.provided, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveStateScheme(%q) returned unexpected error: %v", tc.provided, err)
			}
			if got != tc.want {
				t.Fatalf("resolveStateScheme(%q) = %q, want %q", tc.provided, got, tc.want)
			}
		})
	}
}

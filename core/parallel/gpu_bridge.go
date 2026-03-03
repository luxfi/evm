// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo && darwin

// Bridge that connects the EVM parallel interface to the lux/gpu package.
// Build with: CGO_ENABLED=1 go build -tags parallel,gpu
//
// This eliminates the signature recovery bottleneck:
//   CPU ecrecover: 1613ms for 47K sigs (87% of block time)
//   GPU ecrecover: ~50ms for 47K sigs (32x speedup)

package parallel

import (
	"math/big"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/crypto/secp256k1"
	luxgpu "github.com/luxfi/gpu"
)

// secp256k1halfN is the half-order of secp256k1 for EIP-2 low-S enforcement.
var secp256k1halfN = new(big.Int).Rsh(secp256k1.S256().Params().N, 1)

type gpuBridge struct{}

func init() {
	RegisterGPU(&gpuBridge{})
}

func (g *gpuBridge) Available() bool {
	return luxgpu.DefaultContext != nil && luxgpu.GetBackend() != luxgpu.CPU
}

func (g *gpuBridge) BatchEcrecover(txs []*types.Transaction) (map[common.Hash]common.Address, error) {
	if len(txs) == 0 {
		return nil, nil
	}

	n := len(txs)
	sigs := make([]luxgpu.Signature, n)
	// Track which tx indices have valid GPU input (per-tx signer)
	valid := make([]bool, n)

	for i, tx := range txs {
		v, r, s := tx.RawSignatureValues()
		if r == nil || s == nil {
			continue
		}

		// EIP-2: reject high-S signatures (malleability protection).
		// The CPU path enforces this in ValidateSignatureValues.
		// We must enforce it here too, or GPU returns a different address.
		if s.Cmp(secp256k1halfN) > 0 {
			continue // skip; CPU fallback will reject this tx
		}

		// Reject oversized R or S (> 32 bytes = > 2^256)
		if len(r.Bytes()) > 32 || len(s.Bytes()) > 32 {
			continue
		}

		// Per-tx signer — different chain IDs get different signers.
		// CRITICAL: using a single signer for the whole batch would produce
		// wrong signing hashes for txs with different chain IDs (e.g., legacy
		// unprotected txs mixed with EIP-155 txs), leading to wrong sender recovery.
		signer := types.LatestSignerForChainID(tx.ChainId())
		hash := signer.Hash(tx)

		// Pack r (32 bytes big-endian, left-padded)
		copy(sigs[i].R[:], padTo32(r))

		// Pack s
		copy(sigs[i].S[:], padTo32(s))

		// Recovery ID — guard against overflow for large chain IDs
		vid, ok := recoverV(v, tx.ChainId())
		if !ok {
			continue
		}
		sigs[i].V = vid

		// Message hash
		copy(sigs[i].MsgHash[:], hash[:])
		valid[i] = true
	}

	results, err := luxgpu.BatchEcrecover(sigs)
	if err != nil {
		return nil, err
	}

	addrs := make(map[common.Hash]common.Address, n)
	for i, tx := range txs {
		if !valid[i] {
			continue // was skipped due to validation; CPU fallback handles it
		}
		if i >= len(results) || !results[i].Valid {
			continue
		}
		var addr common.Address
		copy(addr[:], results[i].Address[:])
		// Reject zero address — GPU kernel bug or edge case
		if addr == (common.Address{}) {
			continue
		}
		addrs[tx.Hash()] = addr
	}
	return addrs, nil
}

func (g *gpuBridge) BatchKeccak(_ [][]byte) ([]common.Hash, error) {
	return nil, nil // not yet available; CPU path used
}

// padTo32 returns a 32-byte big-endian representation of a big.Int.
func padTo32(n *big.Int) []byte {
	b := n.Bytes()
	if len(b) > 32 {
		return nil // caller must check; should not happen after validation
	}
	if len(b) == 32 {
		return b
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}

// recoverV extracts the recovery ID (0 or 1) from the transaction's V value.
// Returns (id, false) if v or chainID overflows uint64.
func recoverV(v *big.Int, chainID *big.Int) (uint8, bool) {
	if v.BitLen() > 63 {
		return 0, false
	}
	vVal := v.Uint64()
	if chainID != nil && chainID.Sign() > 0 {
		if chainID.BitLen() > 63 {
			return 0, false
		}
		cid := chainID.Uint64()
		if vVal >= 35 {
			return uint8((vVal - 35 - cid*2) & 1), true
		}
	}
	if vVal != 27 && vVal != 28 {
		return 0, false // invalid V for legacy tx
	}
	return uint8(vVal - 27), true
}

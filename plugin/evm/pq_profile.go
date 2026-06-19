// Copyright (C) 2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import "github.com/luxfi/pq"

// pq_profile.go — the strict-PQ precompile profile a Lux-derived EVM chain
// installs on its ChainConfig. The EVM plugin OWNS the profile it installs
// (vm.Initialize sets vm.chainConfig.PQ), so the projection lives here, in
// one place, constructed from the shared luxfi/pq primitives.
//
// THE GUARDRAIL. LuxStrictPQ refuses every quantum-breakable precompile
// family EXCEPT the standard alt_bn128 (BN254) precompiles at 0x06/0x07/
// 0x08 (EIP-196 / EIP-197). Those are Ethereum-compatibility precompiles
// general-purpose dapps depend on; they are NOT a Lux settlement-security
// surface. Lux's security-critical pairing / DLOG usage lives entirely in
// the CUSTOM precompiles (precompile/zk @ 0x0900, the 0x22 Pedersen
// sub-path, the Z-Chain zkvm verifiers) and the consensus cert — each
// gated by its own strict-PQ switch (contract.RefuseUnderStrictPQ /
// RegisterZKPrecompiles / VerifyUnderPolicy), all driven by the SAME
// profile bit (config.PQ, which also pins extras.StrictPQTimestamp=0).
//
// pq.Strict() (the library maximal profile, re-exported as
// gethvm.AllForbidden) forbids bn256 too; a Lux chain must NOT install
// that — it would break dapp bn256. We construct the carve-out here as a
// pq.Profile literal (every field of pq.Strict() except ForbidBn256).
func LuxStrictPQ() *pq.Profile {
	return &pq.Profile{
		ForbidEcrecover:  true,
		ForbidP256Verify: true,
		ForbidSHA256:     true,
		ForbidRIPEMD160:  true,
		ForbidBlake2F:    true,
		ForbidBn256:      false, // Ethereum-compat dapp precompiles — see doc
		ForbidBLS12381:   true,
		ForbidKZG:        true,
	}
}

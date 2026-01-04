// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Module to facilitate the registration of precompiles and their configuration.
package registry

// Force imports of each precompile to ensure each precompile's init function runs and registers itself
// with the registry.
import (
	// Chain-integrated precompiles (stay in evm)
	_ "github.com/luxfi/evm/precompile/contracts/deployerallowlist"
	_ "github.com/luxfi/evm/precompile/contracts/feemanager"
	_ "github.com/luxfi/evm/precompile/contracts/nativeminter"
	_ "github.com/luxfi/evm/precompile/contracts/rewardmanager"
	_ "github.com/luxfi/evm/precompile/contracts/txallowlist"
	_ "github.com/luxfi/evm/precompile/contracts/warp"

	// ============================================
	// Post-Quantum Cryptography (0x0600-0x06FF)
	// ============================================
	_ "github.com/luxfi/precompile/mldsa"    // ML-DSA signature verification (FIPS 204)
	_ "github.com/luxfi/precompile/mlkem"    // ML-KEM key encapsulation (FIPS 203)
	_ "github.com/luxfi/precompile/slhdsa"   // SLH-DSA stateless hash signatures (FIPS 205)
	_ "github.com/luxfi/precompile/pqcrypto" // Unified PQ crypto operations

	// ============================================
	// Privacy/Encryption (0x0700-0x07FF)
	// ============================================
	_ "github.com/luxfi/precompile/fhe"   // Fully Homomorphic Encryption
	_ "github.com/luxfi/precompile/ecies" // Elliptic Curve Integrated Encryption
	_ "github.com/luxfi/precompile/ring"  // Ring signatures (anonymity)
	_ "github.com/luxfi/precompile/hpke"  // Hybrid Public Key Encryption

	// ============================================
	// Threshold Signatures (0x0800-0x08FF)
	// ============================================
	_ "github.com/luxfi/precompile/frost"    // FROST threshold Schnorr
	_ "github.com/luxfi/precompile/cggmp21"  // CGGMP21 threshold ECDSA
	_ "github.com/luxfi/precompile/ringtail" // Threshold lattice (post-quantum)

	// ============================================
	// ZK Proofs (0x0900-0x09FF)
	// ============================================
	_ "github.com/luxfi/precompile/kzg4844" // KZG commitments (EIP-4844)

	// ============================================
	// Curves (0x0A00-0x0AFF)
	// ============================================
	_ "github.com/luxfi/precompile/secp256r1" // P-256/secp256r1 verification

	// ============================================
	// AI Mining (0x0300-0x03FF)
	// ============================================
	_ "github.com/luxfi/precompile/ai" // AI mining rewards, TEE verification

	// ============================================
	// DEX (0x0400-0x04FF)
	// ============================================
	_ "github.com/luxfi/precompile/dex" // Uniswap v4-style DEX PoolManager

	// ============================================
	// Graph/Query Layer (0x0500-0x05FF)
	// ============================================
	_ "github.com/luxfi/precompile/graph" // GraphQL query interface
)

// LP-ALIGNED ADDRESSING (LP-9015):
// All precompiles use LP-aligned addressing with format: 0x1PCII
// where P = Family Page, C = Chain Slot, II = Item Index
//
// Family Pages (P nibble aligns with LP range first digit):
//   P=0: Core (LP-0xxx) - DeployerAllowList, TxAllowList, NativeMinter, RewardManager, Quasar
//   P=2: PQ/Identity (LP-2xxx) - ML-DSA, ML-KEM, SLH-DSA, PQCrypto
//   P=3: EVM/Crypto (LP-3xxx) - FeeManager, Hashing
//   P=4: Privacy/ZK (LP-4xxx) - FHE, ZK proofs
//   P=5: Threshold (LP-5xxx) - FROST, CGGMP21, Ringtail
//   P=6: Bridges (LP-6xxx) - Warp
//   P=7: AI (LP-7xxx) - AI mining, attestation
//   P=9: DEX (LP-9xxx) - PoolManager, Router
//
// Chain Slots (C nibble):
//   C=0: P-Chain, C=1: X-Chain, C=2: C-Chain, C=3: Q-Chain, etc.
//
// Reserved Range: 0x10000-0x1FFFF (64K addresses for LP-aligned precompiles)
//
// LP-Aligned Precompile Addresses:
// DeployerAllowListAddress = common.HexToAddress("0x10201") // P=0, C=2, II=01
// TxAllowListAddress       = common.HexToAddress("0x10203") // P=0, C=2, II=03
// NativeMinterAddress      = common.HexToAddress("0x10204") // P=0, C=2, II=04
// RewardManagerAddress     = common.HexToAddress("0x10205") // P=0, C=2, II=05
// QuasarAddress            = common.HexToAddress("0x1020A") // P=0, C=2, II=0A
// MLDSAVerifyAddress       = common.HexToAddress("0x12202") // P=2, C=2, II=02
// MLKEMAddress             = common.HexToAddress("0x12203") // P=2, C=2, II=03
// SLHDSAAddress            = common.HexToAddress("0x12204") // P=2, C=2, II=04
// PQCryptoAddress          = common.HexToAddress("0x12201") // P=2, C=2, II=01
// FeeManagerAddress        = common.HexToAddress("0x1320F") // P=3, C=2, II=0F
// FROSTAddress             = common.HexToAddress("0x15201") // P=5, C=2, II=01
// CGGMP21Address           = common.HexToAddress("0x15202") // P=5, C=2, II=02
// RingtailAddress          = common.HexToAddress("0x15203") // P=5, C=2, II=03
// WarpAddress              = common.HexToAddress("0x16201") // P=6, C=2, II=01
// AIAddress                = common.HexToAddress("0x17201") // P=7, C=2, II=01
// DEXPoolManagerAddress    = common.HexToAddress("0x19201") // P=9, C=2, II=01

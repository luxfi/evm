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
	_ "github.com/luxfi/precompile/pqcrypto" // Unified PQ crypto operations
	_ "github.com/luxfi/precompile/slhdsa"   // SLH-DSA stateless hash signatures (FIPS 205)

	// ============================================
	// Privacy/Encryption (0x0700-0x07FF)
	// ============================================
	_ "github.com/luxfi/precompile/ecies" // Elliptic Curve Integrated Encryption
	_ "github.com/luxfi/precompile/fhe"   // Fully Homomorphic Encryption
	_ "github.com/luxfi/precompile/hpke"  // Hybrid Public Key Encryption
	_ "github.com/luxfi/precompile/ring"  // Ring signatures (anonymity)

	// ============================================
	// Threshold Signatures (0x0800-0x08FF)
	// ============================================
	_ "github.com/luxfi/precompile/cggmp21"  // CGGMP21 threshold ECDSA
	_ "github.com/luxfi/precompile/frost"    // FROST threshold Schnorr
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
	// DEX (LP-9xxx) - QuantumSwap Native DEX
	// ============================================
	_ "github.com/luxfi/precompile/dex" // Native DEX PoolManager (LP-9010)

	// ============================================
	// Graph/Query Layer (0x0500-0x05FF)
	// ============================================
	_ "github.com/luxfi/precompile/graph" // GraphQL query interface
)

// LP-ALIGNED ADDRESSING (LP-9015):
// DEX precompiles use trailing LP number format: 0x0000...00LPNUM
// The LP number IS the address suffix - maximum simplicity.
//
// Address format: 0x0000000000000000000000000000000000LPNUM
//
// DEX Precompiles (LP-9xxx - QuantumSwap Native DEX):
//   POOL_MANAGER   = 0x0000...9010  // LP-9010 - Singleton pool manager
//   ORACLE_HUB     = 0x0000...9011  // LP-9011 - Multi-source price aggregation
//   SWAP_ROUTER    = 0x0000...9012  // LP-9012 - Swap routing
//   HOOKS_REGISTRY = 0x0000...9013  // LP-9013 - Hook contract registry
//   FLASH_LOAN     = 0x0000...9014  // LP-9014 - Flash loan facility
//   CLOB           = 0x0000...9020  // LP-9020 - Central limit order book
//   VAULT          = 0x0000...9030  // LP-9030 - DeFi vault operations
//   PRICE_FEED     = 0x0000...9040  // LP-9040 - Price feed aggregator
//
// Bridge Precompiles (LP-6xxx):
//   TELEPORT       = 0x0000...6010  // LP-6010 - Cross-chain teleportation
//
// Same addresses work across ALL Lux EVM chains (C-Chain, Zoo, Hanzo, SPC)
// See github.com/luxfi/precompile/registry for canonical Go addresses

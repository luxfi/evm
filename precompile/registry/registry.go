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
	// LP-4200 Unified PQCrypto Block (0x012201..0x012208)
	// ============================================
	_ "github.com/luxfi/precompile/mlkem"    // 0x012201 ML-KEM key encapsulation (FIPS 203)
	_ "github.com/luxfi/precompile/mldsa"    // 0x012202 ML-DSA signature verification (FIPS 204)
	_ "github.com/luxfi/precompile/slhdsa"   // 0x012203 SLH-DSA stateless hash signatures (FIPS 205)
	// 0x012204 Pulsar (Module-LWE threshold FIPS 204) imported below under Threshold
	_ "github.com/luxfi/precompile/p3q"      // 0x012205 P3Q — LP-218 Post-Quantum Pulsar Proof — Solidity-callable Pulsar verifier
	// 0x012206 Corona (Ring-LWE threshold) imported below under Threshold
	_ "github.com/luxfi/precompile/magnetar" // 0x012207 Magnetar (public-DKG MPC threshold SLH-DSA, FIPS 205 byte-equal)
	_ "github.com/luxfi/precompile/hqc"      // 0x012208 HQC (code-based KEM, family-disjoint backup)
	_ "github.com/luxfi/precompile/starkfri" // 0x012220 STARK-FRI strict-PQ STARK verifier (formerly misnamed P3Q at 0x012205)

	// ============================================
	// Privacy/Encryption (0x0700-0x07FF)
	// ============================================
	// REMOVED: ecies -- secret keys in calldata are public on-chain
	_ "github.com/luxfi/precompile/anchor" // On-chain checkpoint anchoring (LP-7200)
	_ "github.com/luxfi/precompile/fhe"    // Fully Homomorphic Encryption
	_ "github.com/luxfi/precompile/hpke"   // HPKE seal (public-key encrypt only)
	_ "github.com/luxfi/precompile/ring"   // Ring signature verify only

	// ============================================
	// Threshold Signatures (0x0800-0x08FF)
	// ============================================
	_ "github.com/luxfi/precompile/cggmp21" // CGGMP21 threshold ECDSA
	_ "github.com/luxfi/precompile/frost"   // FROST threshold Schnorr
	_ "github.com/luxfi/precompile/corona"  // 0x012206 Corona (Ring-LWE threshold, FIPS-equivalent)
	_ "github.com/luxfi/precompile/pulsar"  // 0x012204 Pulsar (Module-LWE threshold FIPS 204)

	// ============================================
	// ZK Proofs (0x0900-0x09FF)
	// ============================================
	_ "github.com/luxfi/precompile/kzg4844" // KZG commitments (EIP-4844)
	_ "github.com/luxfi/precompile/zk"      // ZK proof verification (Groth16, PLONK, Halo2)

	// ============================================
	// Curves (0x0A00-0x0AFF)
	// ============================================
	_ "github.com/luxfi/precompile/secp256r1" // P-256/secp256r1 verification

	// ============================================
	// AI Mining (0x0300-0x03FF)
	// ============================================
	_ "github.com/luxfi/precompile/ai"            // AI mining + atomic cross-chain mint (0x0300..00)
	_ "github.com/luxfi/precompile/modelregistry" // Versioned model-commitment registry (0x0300..02)
	_ "github.com/luxfi/precompile/inference"     // Deterministic on-chain int8 inference (0x0300..03)

	// ============================================
	// DEX (LP-9xxx) - QuantumSwap Native DEX
	// ============================================
	_ "github.com/luxfi/precompile/dex" // Native DEX settlement money path (LP-9999) + views (9998/9997/9996) + router (9012)

	// ============================================
	// Graph/Query Layer (0x0500-0x05FF)
	// ============================================
	_ "github.com/luxfi/precompile/graph" // GraphQL query interface

	// ============================================
	// Hashing (0x0504)
	// ============================================
	_ "github.com/luxfi/precompile/blake3" // Blake3 hash function

	// ============================================
	// Dead Address Routing
	// ============================================
	_ "github.com/luxfi/precompile/dead" // Dead/burn address handlers (0x0, 0xdead)

	// ============================================
	// Quasar Edition rollout — net-new precompiles activated at
	// blockTimestamp 1766708400 per ~/work/lux/genesis/configs/mainnet/upgrade.json.
	// Each must be side-effect imported here so its init() registers the
	// configKey with modules.RegisteredModules before luxd parses
	// upgrade.json — without this the parser rejects the activation with
	// "unknown precompile config".
	// ============================================
	_ "github.com/luxfi/precompile/attestation"  // attestationConfig
	_ "github.com/luxfi/precompile/babyjubjub"   // babyjubjubConfig
	_ "github.com/luxfi/precompile/bls12381"     // bls12381{G1,G2}{Add,Mul,MSM}Config + bls12381PairingConfig
	_ "github.com/luxfi/precompile/bridge"       // bridgeRegistrarConfig
	_ "github.com/luxfi/precompile/compute"      // computeMarketConfig
	_ "github.com/luxfi/precompile/curve25519"   // curve25519Config
	_ "github.com/luxfi/precompile/ed25519"      // ed25519Config
	_ "github.com/luxfi/precompile/math"         // fixedPointMathConfig
	_ "github.com/luxfi/precompile/pasta"        // pastaConfig
	_ "github.com/luxfi/precompile/pedersen"     // pedersenConfig
	_ "github.com/luxfi/precompile/poseidon"     // poseidonConfig
	_ "github.com/luxfi/precompile/sr25519"      // sr25519Verify
	_ "github.com/luxfi/precompile/stableswap"   // stableSwapConfig
	_ "github.com/luxfi/precompile/vrf"          // vrfConfig
	_ "github.com/luxfi/precompile/x25519"       // x25519Config
	_ "github.com/luxfi/precompile/xwing"        // xwingConfig
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

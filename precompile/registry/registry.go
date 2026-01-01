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

// This list is kept just for reference. The actual addresses defined in respective packages of precompiles.
// Note: it is important that none of these addresses conflict with each other or any other precompiles
// in core/vm/contracts.go.
// The first stateful precompiles were added in coreth to support nativeAssetCall and nativeAssetBalance. New stateful precompiles
// originating in coreth will continue at this prefix, so we reserve this range in evm so that they can be migrated into
// evm without issue.
// These start at the address: 0x0100000000000000000000000000000000000000 and will increment by 1.
// Optional precompiles implemented in evm start at 0x0200000000000000000000000000000000000000 and will increment by 1
// from here to reduce the risk of conflicts.
// For forks of evm, users should start at 0x0300000000000000000000000000000000000000 to ensure
// that their own modifications do not conflict with stateful precompiles that may be added to evm
// in the future.
// ContractDeployerAllowListAddress = common.HexToAddress("0x0200000000000000000000000000000000000000")
// ContractNativeMinterAddress      = common.HexToAddress("0x0200000000000000000000000000000000000001")
// TxAllowListAddress               = common.HexToAddress("0x0200000000000000000000000000000000000002")
// FeeManagerAddress                = common.HexToAddress("0x0200000000000000000000000000000000000003")
// RewardManagerAddress             = common.HexToAddress("0x0200000000000000000000000000000000000004")
// WarpAddress                      = common.HexToAddress("0x0200000000000000000000000000000000000005")
// MLDSAVerifyAddress               = common.HexToAddress("0x0200000000000000000000000000000000000006")
// SLHDSAVerifyAddress              = common.HexToAddress("0x0200000000000000000000000000000000000007")
// PQCryptoAddress                  = common.HexToAddress("0x0200000000000000000000000000000000000010")
// ADD YOUR PRECOMPILE HERE
// {YourPrecompile}Address          = common.HexToAddress("0x03000000000000000000000000000000000000??")

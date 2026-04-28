// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build fhe

package registry

// FHE precompile registration is gated behind the `fhe` build tag because
// luxfi/precompile@v0.5.11/fhe/fhe_ops.go calls fhe.NewKeyGeneratorFromSeed,
// which is not present in any released luxfi/fhe (latest v1.7.9 ships only
// NewKeyGenerator). Default builds skip this import so the rest of the EVM
// compiles cleanly. Once luxfi/precompile or luxfi/fhe ship the seeded keygen
// API, drop the build tag and move this import back into registry.go.
import _ "github.com/luxfi/precompile/fhe" // Fully Homomorphic Encryption

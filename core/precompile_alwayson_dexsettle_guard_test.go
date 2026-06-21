// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build dexsettle_guard

// This file holds the ONE cross-repo DRY guard that pins the 0x9999 activation
// timestamp across the two layers that cannot share a Go symbol:
//
//   - the CANONICAL definition, extras.DexSettleActivationTime (this evm layer),
//   - the layer-local MIRROR, dex.DexSettleActivationTime (luxfi/precompile).
//
// It is RELEASE-COUPLED on purpose. dex.DexSettleActivationTime lives in
// luxfi/precompile; evm/go.mod must be bumped to a precompile version that
// EXPORTS that symbol before this guard can compile. Because the precompile tag
// is published by CI/CD (never from a dev checkout), keeping this test always-on
// would make evm/core fail to build (`undefined: dex.DexSettleActivationTime`)
// against any precompile tag predating the symbol — a silent CI break that only
// an untracked local go.work could paper over.
//
// The `dexsettle_guard` build tag decouples WHAT the guard asserts from WHEN it
// is compiled. Default CI (`go test ./core/`) excludes this file, so evm/core
// builds and tests against the pinned precompile tag with no go.work. The release
// step that bumps evm/go.mod to precompile >= the symbol-exporting tag runs:
//
//	go test -tags dexsettle_guard ./core/
//
// at which point dex.DexSettleActivationTime exists and the DRY invariant is
// enforced atomically with the bump. See the release checklist in
// precompile_alwayson_test.go's package doc and LLM.md.
//
// Everything else about 0x9999 activation (dispatch gate, marker install, the
// override race, replay safety) is pinned UNCONDITIONALLY in
// precompile_alwayson_test.go using only evm-local symbols — those run on every
// CI build. Only this cross-repo symbol equality is tag-gated.
package core

import (
	"testing"

	"github.com/luxfi/evm/params/extras"
	// dex provides the layer-local mirror DexSettleActivationTime asserted equal
	// to the canonical extras value here. This is the ONLY reference in evm to a
	// precompile symbol that may post-date evm/go.mod's pinned precompile tag,
	// which is exactly why this file is behind the dexsettle_guard build tag.
	"github.com/luxfi/precompile/dex"
	"github.com/stretchr/testify/require"
)

// TestDexSettleActivationTime_LayerMirrorsMatch is the DRY-safety guard for the
// 0x9999 activation timestamp. It imports BOTH layers' constants and fails CI the
// instant either moves without the other. If the two drift, a settlement could
// dispatch (gated by extras) at a timestamp where the precompile withholds its log
// (gated by the stale mirror) — or vice versa — opening a settlement-without-log /
// log-without-settlement window and a consensus split on replay.
func TestDexSettleActivationTime_LayerMirrorsMatch(t *testing.T) {
	require.Equal(t, extras.DexSettleActivationTime, dex.DexSettleActivationTime,
		"dex.DexSettleActivationTime (precompile layer-local mirror) must equal the canonical extras.DexSettleActivationTime; "+
			"the DEX log gate and the EVM dispatch gate MUST fire on the same dated fork")
}

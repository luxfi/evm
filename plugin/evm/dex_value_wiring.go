// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"fmt"

	"github.com/luxfi/chains/dexvm/registry"
	"github.com/luxfi/dex/pkg/dexcore"
	"github.com/luxfi/ids"
	"github.com/luxfi/precompile/dex"
	"github.com/luxfi/runtime"

	"github.com/luxfi/geth/log"
)

// dex_value_wiring.go is the SEAM-1 TRUST ROOT of the 0x9999 native value DEX: the live
// boot wiring that turns the always-on precompile from fail-closed-by-absence into a
// LIVE, real-money router. The 0x9999 precompile has been active since the Dec 25 2025
// activation, and its fail-closed admission machinery (asset_resolver.go,
// asset_onchain_verifier.go in precompile/dex) consults an INSTALLED resolver/D-client.
// Until that install runs, every value swap reverts ErrNoAssetResolver /
// ErrNoOnChainVerifier. This is the ONE production caller that installs them, in the EVM
// plugin boot — the same place the removed value-activation gate used to bind, but this is
// NOT a gate: it is the wiring that makes the money path live, derived ENTIRELY from the
// node's real chain context, never a constant.
//
// WHAT MAKES AN ASSET "REAL" (the trust root, layered — Red should re-review this chain):
//
//  1. The per-network asset MANIFEST (registry.EmbeddedManifestFor by EVM chainID, or the
//     runtime-rooted localnet native manifest). Content-hashed, CI-proven against the live
//     net's eth_getCode before the artifact ships, compiled into the binary.
//  2. registry.NewRuntimeVerifier binds the manifest's DECLARED (networkID, cChainID) to
//     the node's ACTUAL running ids (from runtime.Runtime). A wrong-net / wrong-C-Chain
//     manifest is REFUSED here — the resolver is never built over a mismatched identity.
//  3. registry.Register (via Manifest.ApplyTo) admits each asset only after VerifyOnChain
//     proves identity + decimals; the synthetic-config deny-scan (RefuseUnderSyntheticConfig)
//     refuses any ASCII-ticker / mock / Liquidity-universe / disabled-market reference.
//  4. At SWAP time the precompile additionally (a) cross-checks the installed resolver's
//     bound (networkID, cChainID) against the consensus-supplied AtomicState identity
//     (fail-closed on mismatch), and (b) runs the live EXTCODESIZE verifier so a
//     self-destructed / never-deployed token is refused right then.
//
// So the resolver admits ONLY assets that are registered in the content-pinned,
// identity-bound, CI-proven manifest AND backed by live on-chain code at swap time. An
// unknown/synthetic/disabled/wrong-network/wrong-chain asset has no admission and the swap
// reverts. There is no ASCII-ticker, no left-padded symbol, no hardcoded fake market.

// registryAssetResolver adapts a *registry.Registry to the precompile's value-path
// dexcore.AssetResolver port. It is bound to the node's running (networkID, cChainID,
// xChainID) so it answers ONLY for the chain the node actually runs: it derives the
// canonical AssetID from those bound ids (cChainID for the EVM kinds, xChainID for UTXO),
// then resolves it in the registry. The registry's DeriveAssetID and dexcore's
// DeriveAssetID are byte-identical (same domain tag, same length-prefixed SHA-256 fold,
// same kind bytes), so the id this resolver returns is exactly the id the value path
// expects — by construction, never by coincidence.
type registryAssetResolver struct {
	reg       *registry.Registry
	networkID uint32
	cChainID  ids.ID
	xChainID  ids.ID
}

var _ dexcore.AssetResolver = (*registryAssetResolver)(nil)

// sourceChainFor returns the chain id the asset's canonical id is rooted at: the C-Chain
// for the two EVM kinds, the X-Chain for a UTXO asset. The value path only ever resolves
// EVM kinds (a V4 currency address is always native or ERC-20), but the UTXO arm keeps the
// adapter complete and correct so the SAME resolver also gates the Initialize admission
// for every kind.
func (r *registryAssetResolver) sourceChainFor(kind dexcore.AssetKind) ids.ID {
	if kind == dexcore.AssetKindUTXO {
		return r.xChainID
	}
	return r.cChainID
}

// ResolveEnabled implements dexcore.AssetResolver: derive the canonical AssetID from the
// resolver's bound network identity and the per-asset (kind, ref), then resolve it in the
// registry, returning the asset's on-chain decimals. An unregistered (synthetic) or
// disabled asset yields a non-nil error (registry.ErrUnknownAsset / ErrAssetDisabled), so
// the value path reverts. A malformed ref is rejected by DeriveAssetID before any lookup.
func (r *registryAssetResolver) ResolveEnabled(kind dexcore.AssetKind, ref []byte) (ids.ID, uint8, error) {
	// dexcore.AssetKind and registry.AssetKind are wire-pinned to the SAME values
	// (EVM_NATIVE=1, ERC20=2, UTXO=3), so the numeric kind crosses the boundary directly.
	id, err := registry.DeriveAssetID(r.networkID, r.sourceChainFor(kind), registry.AssetKind(kind), ref)
	if err != nil {
		return ids.Empty, 0, err
	}
	asset, err := r.reg.MustResolveEnabled(id)
	if err != nil {
		return ids.Empty, 0, err
	}
	return id, asset.Decimals, nil
}

// installDEXValuePath is the SINGLE production install of the 0x9999 value-path trust
// root. It (1) selects the per-network asset manifest from the node's real EVM chainID,
// (2) builds an identity-bound registry from it (refusing a wrong-net/wrong-chain manifest
// or any synthetic config), (3) installs the registry-backed resolver bound to the node's
// (networkID, cChainID), and (4) installs the local in-process native C<->D client for the
// book/ledger. It is wired for ALL networks (mainnet 96369 / testnet 96368 / devnet 96370
// / localnet 1337) so the DEX is live out-of-the-box, consistent with the precompile being
// active since the Dec 25 2025 activation.
//
// FAIL-CLOSED, NEVER FAIL-OPEN: it returns an error on ANY problem (no chain context,
// missing/mismatched manifest, registry refusal). The caller LOGS the error and does NOT
// install a resolver — leaving the precompile's installedAssetResolver nil, so every value
// swap reverts ErrNoAssetResolver. A construction failure can never open the money path;
// it can only keep it closed. The node still boots for all non-DEX usage.
//
// Idempotent: InstallAssetResolver allows a same-identity re-install and refuses a
// conflicting one; InstallDChainClient refuses an install after pool state has accumulated.
// Initialize runs once per VM, so neither guard trips in normal operation.
func installDEXValuePath(rt *runtime.Runtime, evmChainID uint64) error {
	if rt == nil {
		return fmt.Errorf("dex value path: no chain runtime (cannot derive identity; staying fail-closed)")
	}
	networkID := rt.NetworkID
	// Mirror EXACTLY the precompile AtomicState.CChainID() resolution so the identity this
	// install binds matches the identity the swap path cross-checks byte-for-byte (a
	// mismatch is fail-closed at swap time, ErrAssetResolverIdentityMismatch). On the
	// C-Chain CChainID == ChainID; prefer the explicit field when set.
	cChainID := rt.CChainID
	if cChainID == ids.Empty {
		cChainID = rt.ChainID
	}
	if cChainID == ids.Empty {
		return fmt.Errorf("dex value path: node C-Chain id is empty (cannot bind resolver; staying fail-closed)")
	}

	manifest, err := manifestForNode(networkID, cChainID, evmChainID)
	if err != nil {
		return err
	}

	// Identity-bind the manifest to the node's running ids; refuses a wrong-net/wrong-chain
	// manifest. This is the seam-1 identity gate: the resolver is never built over a manifest
	// that disagrees with the chain the node actually runs.
	rv, err := registry.NewRuntimeVerifier(networkID, cChainID, rt.XChainID, manifest)
	if err != nil {
		return fmt.Errorf("dex value path: runtime verifier: %w", err)
	}

	// Populate the registry: register every manifest asset (each proven via the runtime
	// verifier), create every manifest market, and run the fail-closed synthetic-config
	// deny-scan under the locked-down default policy. ApplyTo == AdmitInto + the gate.
	reg := registry.New(registry.DefaultDexAssetPolicy().AllowedKindsOrDefault()...)
	if err := manifest.ApplyTo(reg, rv, registry.DefaultDexAssetPolicy()); err != nil {
		return fmt.Errorf("dex value path: build registry from manifest %q: %w", manifest.Network, err)
	}

	resolver := &registryAssetResolver{
		reg:       reg,
		networkID: networkID,
		cChainID:  cChainID,
		xChainID:  rt.XChainID,
	}
	if err := dex.InstallAssetResolver(resolver, networkID, cChainID); err != nil {
		return fmt.Errorf("dex value path: install asset resolver: %w", err)
	}

	// The local in-process native C<->D client for the book/ledger custody seam — NOT a
	// remote keeper/venue/ZAP engine. Brand is white-labeled by network for user-facing
	// error/log strings. NewNativeDChainClient defaults to the OSS "Lux DEX" identity.
	if err := dex.InstallDChainClient(dex.NewNativeDChainClient(dexBrandFor(networkID))); err != nil {
		return fmt.Errorf("dex value path: install local D-Chain client: %w", err)
	}

	log.Info("0x9999 native DEX value path LIVE",
		"network", manifest.Network,
		"networkID", networkID,
		"evmChainID", evmChainID,
		"cChainID", cChainID,
		"registeredAssets", reg.Len(),
	)
	return nil
}

// manifestForNode selects the asset manifest for the node's network: the committed,
// content-hashed embedded manifest for mainnet/testnet/devnet (selected by EVM chainID),
// or the runtime-rooted native-only manifest for localnet (whose C-Chain id is not fixed).
// An unknown EVM chainID with no committed manifest and that is not localnet fails closed
// (no manifest -> no resolver -> swaps revert).
func manifestForNode(networkID uint32, cChainID ids.ID, evmChainID uint64) (*registry.Manifest, error) {
	if evmChainID == registry.LocalnetEVMChainID {
		// Localnet: synthesise the native-only manifest from the node's LIVE C-Chain id.
		m, err := registry.LocalnetNativeManifest(networkID, cChainID)
		if err != nil {
			return nil, fmt.Errorf("dex value path: localnet native manifest: %w", err)
		}
		return m, nil
	}
	m, _, err := registry.EmbeddedManifestFor(evmChainID)
	if err != nil {
		return nil, fmt.Errorf("dex value path: no asset manifest (staying fail-closed): %w", err)
	}
	return m, nil
}

// dexBrandFor maps a Lux networkID to the user-facing DEX brand for error/log strings.
// Only the Lux primary networks (1/2/3/1337) are the canonical Lux DEX; any other id is a
// sovereign L1 that white-labels downstream, so it gets the empty default (the native
// client maps "" to "Lux DEX" for the OSS package, which downstream tenants override).
func dexBrandFor(networkID uint32) string {
	switch networkID {
	case 1, 2, 3, 1337:
		return "Lux DEX"
	default:
		return "" // sovereign L1: white-label default
	}
}

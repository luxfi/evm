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

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/log"
)

// dex_value_wiring.go is the SEAM-1 TRUST ROOT of the 0x9999 native value DEX: the live
// boot wiring that turns the always-on precompile from fail-closed-by-absence into a LIVE,
// PERMISSIONLESS, real-money router. The 0x9999 precompile has been active since the Dec 25
// 2025 activation, and its admission machinery (asset_resolver.go, asset_onchain_verifier.go
// in precompile/dex) consults an INSTALLED resolver + the live EXTCODESIZE verifier. Until
// that install runs, every value swap reverts ErrNoAssetResolver / ErrNoOnChainVerifier.
// This is the ONE production caller that installs them, in the EVM plugin boot — derived
// ENTIRELY from the node's real chain context, never a constant, never an allowlist.
//
// THE ADMISSION MODEL IS PERMISSIONLESS (the corrected product rule). The registry is a
// canonical RESOLVER, not a permission gate. ANY asset may trade if its canonical identity
// resolves on this network AND it is proven REAL on-chain. There is NO per-network asset
// manifest gating which assets are tradeable; the boot installs a resolver bound to the
// node's running identity and the precompile's EXTCODESIZE verifier proves reality per swap.
//
// WHAT MAKES AN ASSET TRADEABLE (the admission predicate, layered):
//
//  1. RESOLVE (canonicalAssetResolver, installed here): the asset's canonical 32-byte
//     AssetID derives from its real coordinates (kind + canonical ref) rooted at the node's
//     bound (networkID, cChainID/xChainID). This is PURE canonical math (registry.DeriveAssetID
//     — the same domain-separated SHA-256 fold dexcore uses, byte-identical). It refuses ONLY
//     a malformed reference or one that cannot be rooted at this network/chain. It does NOT
//     consult a registered-asset set, an Enabled flag, or a manifest — any well-formed
//     EVM-native/ERC-20 reference on the bound C-Chain resolves.
//  2. IDENTITY CROSS-CHECK at swap time: the precompile cross-checks the installed resolver's
//     bound (networkID, cChainID) against the consensus-supplied AtomicState identity
//     (fail-closed on mismatch — a resolver bound to the wrong chain never admits here).
//  3. ON-CHAIN PROOF at swap time (the AUTHORITATIVE gate): the precompile runs the live
//     EXTCODESIZE verifier — an ERC-20 must have contract code at its address on THIS C-Chain
//     RIGHT NOW; the native coin is always real; a UTXO is real on its source chain. A
//     fabricated/synthetic asset (an ASCII-ticker "address", a never-deployed or
//     self-destructed contract) has no code, so it is refused HERE — by on-chain proof, never
//     by a list.
//
// So the value path admits ANY asset whose canonical identity resolves on this network AND
// that is backed by live on-chain code at swap time — permissionlessly. A synthetic asset is
// refused because it fails the on-chain proof, not because it is unlisted. Markets open
// permissionlessly over any two such real assets (OpenMarket: resolve + prove + derive +
// create-if-absent).

// canonicalAssetResolver is the PERMISSIONLESS value-path dexcore.AssetResolver. It is bound
// to the node's running (networkID, cChainID, xChainID) so it answers ONLY for the chain the
// node actually runs: it derives the canonical AssetID from those bound ids (cChainID for the
// EVM kinds, xChainID for UTXO) via the pure canonical fold. It holds NO registered-asset set
// and NO manifest — admission is canonical resolution here plus the precompile's on-chain
// proof, never membership. registry.DeriveAssetID and dexcore.DeriveAssetID are byte-identical
// (same domain tag, same length-prefixed SHA-256 fold, same kind bytes), so the id this
// resolver returns is exactly the id the value path expects — by construction.
type canonicalAssetResolver struct {
	networkID uint32
	cChainID  ids.ID
	xChainID  ids.ID
}

var _ dexcore.AssetResolver = (*canonicalAssetResolver)(nil)

// sourceChainFor returns the chain id the asset's canonical id is rooted at: the C-Chain for
// the two EVM kinds, the X-Chain for a UTXO asset. The value path only ever resolves EVM
// kinds (a V4 currency address is always native or ERC-20), but the UTXO arm keeps the
// resolver complete and correct so the SAME resolver also roots the Initialize admission for
// every kind.
func (r *canonicalAssetResolver) sourceChainFor(kind dexcore.AssetKind) ids.ID {
	if kind == dexcore.AssetKindUTXO {
		return r.xChainID
	}
	return r.cChainID
}

// ResolveAsset implements dexcore.AssetResolver PERMISSIONLESSLY: it derives the canonical
// AssetID from the resolver's bound network identity and the per-asset (kind, ref) and
// returns it. It admits ANY well-formed reference on the bound network — there is NO
// membership lookup, NO Enabled flag, NO manifest. It returns a non-nil error ONLY when the
// reference is malformed (DeriveAssetID rejects the shape via canonicalRefFor) or cannot be
// rooted at the bound chain. The AUTHORITATIVE reality check (does this ERC-20 actually have
// code?) is the precompile's on-chain verifier, run next on the value path — not this
// resolver's concern.
//
// decimals: the native coin's intrinsic 18 is a pure-identity fact and is returned. An
// ERC-20/UTXO's decimals is a LIVE-CHAIN fact (read by the on-chain verifier's chain view,
// the single source of that fact), so the resolver returns 0 for those; the value path does
// not consume per-asset decimals (it keys value by the established left-pad), so returning 0
// is correct and non-regressive.
func (r *canonicalAssetResolver) ResolveAsset(kind dexcore.AssetKind, ref []byte) (ids.ID, uint8, error) {
	// dexcore.AssetKind and registry.AssetKind are wire-pinned to the SAME values
	// (EVM_NATIVE=1, ERC20=2, UTXO=3), so the numeric kind crosses the boundary directly.
	id, err := registry.DeriveAssetID(r.networkID, r.sourceChainFor(kind), registry.AssetKind(kind), ref)
	if err != nil {
		return ids.Empty, 0, err
	}
	var decimals uint8
	if kind == dexcore.AssetKindEVMNative {
		decimals = 18 // the chain's own coin: a pure-identity fact
	}
	return id, decimals, nil
}

// installDEXValuePath is the SINGLE production install of the 0x9999 value-path trust root.
// It (1) derives the node's running identity (networkID, cChainID, xChainID) from the chain
// runtime, (2) installs a PERMISSIONLESS canonical resolver bound to that identity, and (3)
// installs the local in-process native C<->D client for the book/ledger. It is wired for ALL
// networks so the DEX is live out-of-the-box, consistent with the precompile being active
// since the Dec 25 2025 activation.
//
// PERMISSIONLESS: it requires NO per-network asset manifest. A node brings the value path
// live from its own chain identity alone; thereafter ANY asset trades if its canonical
// identity resolves AND it is proven real on-chain at swap time. There is no list to ship,
// no admin to approve an asset, no pre-registration.
//
// FAIL-CLOSED, NEVER FAIL-OPEN: it returns an error on ANY problem (no chain context, empty
// C-Chain id, install conflict). The caller LOGS the error and does NOT install a resolver —
// leaving the precompile's installedAssetResolver nil, so every value swap reverts
// ErrNoAssetResolver. A construction failure can never open the money path; it can only keep
// it closed. The node still boots for all non-DEX usage.
//
// Idempotent: InstallAssetResolver allows a same-identity re-install and refuses a conflicting
// one; InstallDChainClient refuses an install after pool state has accumulated. Initialize
// runs once per VM, so neither guard trips in normal operation.
func installDEXValuePath(rt *runtime.Runtime, evmChainID uint64) error {
	if rt == nil {
		return fmt.Errorf("dex value path: no chain runtime (cannot derive identity; staying fail-closed)")
	}
	networkID := rt.NetworkID
	// Mirror EXACTLY the precompile AtomicState.CChainID() resolution so the identity this
	// install binds matches the identity the swap path cross-checks byte-for-byte (a mismatch
	// is fail-closed at swap time, ErrAssetResolverIdentityMismatch). On the C-Chain
	// CChainID == ChainID; prefer the explicit field when set.
	cChainID := rt.CChainID
	if cChainID == ids.Empty {
		cChainID = rt.ChainID
	}
	if cChainID == ids.Empty {
		return fmt.Errorf("dex value path: node C-Chain id is empty (cannot bind resolver; staying fail-closed)")
	}

	// DEX GOVERNANCE AUTHORITY (HIGH-3): bind the per-network governance controller onto
	// the chain runtime so the always-on 0x9999 precompile resolves its halt/seed authority
	// from the SAME runtime seam as networkID/cChainID/dChainID (contract.AtomicState.
	// GovernanceController()). It is a governance CONTRACT per network, NEVER a hardcoded /
	// mnemonic-derivable EOA. dexGovernanceFor returns the zero address for any network that
	// has not configured its governance contract, which the precompile treats as fail-closed
	// (halt/seed uncallable) — strictly safer than a default key. This is the ONE production
	// site that sets the runtime's governance controller.
	rt.Lock.Lock()
	rt.GovernanceController = dexGovernanceFor(networkID)
	gov := rt.GovernanceController
	rt.Lock.Unlock()

	// PERMISSIONLESS resolver: bound to the node's running identity, admits any well-formed
	// real reference on this network. No manifest, no registered-asset set, no Enabled flag —
	// the AUTHORITATIVE reality check is the precompile's per-swap EXTCODESIZE verifier.
	resolver := &canonicalAssetResolver{
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

	log.Info("0x9999 native DEX value path LIVE (permissionless)",
		"networkID", networkID,
		"evmChainID", evmChainID,
		"cChainID", cChainID,
		"xChainID", rt.XChainID,
		"governanceController", govForLog(gov),
	)
	return nil
}

// govForLog renders the governance controller for the boot log, making an unset
// authority loud rather than a silent zero address (so operators see when a network is
// running fail-closed and must deploy + wire its governance contract).
func govForLog(gov ids.ShortID) string {
	if gov == ids.ShortEmpty {
		return "<unset: halt/seed FAIL-CLOSED — deploy + wire a governance contract for this network>"
	}
	return common.Address(gov).Hex()
}

// dexGovernanceFor maps a Lux networkID to its DEX GOVERNANCE CONTROLLER — the address
// of the per-network governance authority (a Governor/Timelock/multisig CONTRACT, e.g.
// node/contracts/governance/FeeTimelock.sol) that is the SOLE caller permitted to halt
// 0x9999 settlement or seed its pots. This is the decentralized replacement for the
// retired single hardcoded EOA (the mnemonic-derivable 0x9011… DefaultDAOTreasury that
// anyone with the dev mnemonic could use to censor an asset or DoS the DEX).
//
// CRITICAL OPERATOR DIRECTIVE: each entry MUST be a governance CONTRACT whose authority
// is NOT derivable from any developer mnemonic — NEVER an EOA, and NEVER the retired
// 0x9011E888251AB053B7bD1cdB598Db4f9DEd94714. A network with no entry returns the zero
// address and runs FAIL-CLOSED: its halt switches and pot-seeding are uncallable until
// its governance contract is deployed and its address is set here. Fail-closed (no one
// can halt) is strictly safer than fail-open (a single key can halt) — so an unset
// network can never be censored or DoS'd through this authority.
//
// The address is bound onto the chain runtime at boot (installDEXValuePath) and surfaced
// to the always-on 0x9999 precompile via contract.AtomicState.GovernanceController() —
// the SAME runtime seam networkID/cChainID/dChainID flow through, with ZERO per-net
// config file. Returning ids.ShortID keeps this in the runtime's geth-free 20-byte type;
// it is the same 20 bytes as the governance contract's EVM address.
func dexGovernanceFor(networkID uint32) ids.ShortID {
	// Intentionally EMPTY of real addresses until each network deploys its governance
	// contract: no governance contract is deployed at a known address on any Lux network
	// yet, and binding a placeholder/EOA here would re-introduce exactly the centralized-
	// key vulnerability this fix removes. Every network therefore runs fail-closed (no
	// halt authority) — provably no single key can DoS the DEX — until its real Governor/
	// Timelock CONTRACT address is wired in below, per network:
	//
	//	switch networkID {
	//	case 1:    // Lux mainnet  → mainnet DEX Governor/Timelock contract
	//	case 2:    // Lux testnet  → testnet DEX Governor/Timelock contract
	//	case 3:    // Lux devnet   → devnet  DEX Governor/Timelock contract
	//	case 1337: // Lux localnet → localnet DEX Governor/Timelock contract
	//	}
	//
	// Each case MUST be a governance CONTRACT address (NEVER a dev-mnemonic EOA, NEVER
	// 0x9011…94714). Sovereign L1s (any other networkID) white-label downstream and wire
	// their own governance contract the same way.
	_ = networkID
	return ids.ShortEmpty
}

// dexBrandFor maps a Lux networkID to the user-facing DEX brand for error/log strings. Only
// the Lux primary networks (1/2/3/1337) are the canonical Lux DEX; any other id is a
// sovereign L1 that white-labels downstream, so it gets the empty default (the native client
// maps "" to "Lux DEX" for the OSS package, which downstream tenants override).
func dexBrandFor(networkID uint32) string {
	switch networkID {
	case 1, 2, 3, 1337:
		return "Lux DEX"
	default:
		return "" // sovereign L1: white-label default
	}
}

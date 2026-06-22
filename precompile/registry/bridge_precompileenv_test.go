// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"context"
	"math/big"
	"testing"

	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	gethvm "github.com/luxfi/geth/core/vm"
	"github.com/luxfi/geth/params"
)

// bridge_precompileenv_test.go pins the CONSUMER side of the ERC-20 settlement env
// wiring — the fix that makes accessibleStateBridge.GetPrecompileEnv() return a LIVE
// Call surface instead of nil. Before the fix, this returned nil unconditionally, and
// the DEX 0x9999 precompile's ERC-20 leg refused every transferFrom/transfer with
// ErrERC20VaultUnavailable (no token could ever move cross-chain). The bridge now
// forwards the geth env's Call to the external precompile via the callableEnv shape.
//
// Three properties are pinned:
//  1. WIRED: when the internal adapter exposes a geth env (production), the bridge
//     returns a non-nil external env whose Call routes to the geth env.
//  2. FAIL-SECURE: when the internal adapter does NOT expose a geth env (a test mock
//     or non-EVM caller), the bridge returns nil so the precompile refuses rather
//     than mint an unbacked claim.
//  3. SIGNATURE: the returned env satisfies the EXACT callableEnv shape the DEX
//     precompile (luxfi/precompile/dex/module_erc20.go) type-asserts.

// callableEnv is byte-identical to the assertion in luxfi/precompile/dex's
// module_erc20.go. Keep it in lock-step: if the precompile's expected Call signature
// drifts from what precompileEnvBridge provides, this test fails to compile.
type callableEnv interface {
	Call(addr common.Address, input []byte, gas uint64, value *big.Int) (ret []byte, gasLeft uint64, err error)
}

// recordingEnv is a fake geth vm.PrecompileEnvironment whose Call records its
// arguments and returns a sentinel — enough to prove the bridge forwards Call to the
// underlying geth env with the value, no caller proxying, and the variadic tail
// dropped. All other methods are unused stubs (the bridge only forwards Call/ReadOnly).
type recordingEnv struct {
	calls    int
	lastAddr common.Address
	lastIn   []byte
	lastGas  uint64
	lastVal  *big.Int
	lastOpts int // number of CallOption passed — MUST be 0 (no proxying)
	readOnly bool
}

func (e *recordingEnv) Call(addr common.Address, input []byte, gas uint64, value *big.Int, opts ...gethvm.CallOption) ([]byte, uint64, error) {
	e.calls++
	e.lastAddr = addr
	e.lastIn = input
	e.lastGas = gas
	e.lastVal = value
	e.lastOpts = len(opts)
	return []byte("ok"), gas, nil
}

func (e *recordingEnv) ReadOnly() bool                            { return e.readOnly }
func (e *recordingEnv) BlockHeader() (*types.Header, error)       { return &types.Header{}, nil }
func (e *recordingEnv) Rules() params.Rules                       { return params.Rules{} }
func (e *recordingEnv) BlockNumber() *big.Int                     { return big.NewInt(0) }
func (e *recordingEnv) BlockTime() uint64                         { return 0 }
func (e *recordingEnv) Addresses() gethvm.PrecompileAddresses     { return gethvm.PrecompileAddresses{} }
func (e *recordingEnv) ChainConfig() *params.ChainConfig          { return nil }
func (e *recordingEnv) StateDB() gethvm.StateDB                   { return nil }
func (e *recordingEnv) ReadOnlyState() gethvm.StateDB             { return nil }
func (e *recordingEnv) UseGas(uint64) bool                        { return true }
func (e *recordingEnv) Gas() uint64                               { return 0 }
func (e *recordingEnv) ConsensusContext() context.Context         { return context.Background() }
func (e *recordingEnv) CallIndex() uint32                         { return 0 }

var _ gethvm.PrecompileEnvironment = (*recordingEnv)(nil)

// internalWithEnv is a minimal internal contract.AccessibleState that ALSO exposes the
// geth env via GetPrecompileEnv — the production shape (evm/core's accessibleStateAdapter).
type internalWithEnv struct {
	env gethvm.PrecompileEnvironment
}

func (m *internalWithEnv) GetStateDB() contract.StateDB                       { return nil }
func (m *internalWithEnv) GetBlockContext() contract.BlockContext             { return nil }
func (m *internalWithEnv) GetConsensusContext() context.Context               { return context.Background() }
func (m *internalWithEnv) GetChainConfig() precompileconfig.ChainConfig       { return nil }
func (m *internalWithEnv) GetPrecompileEnv() gethvm.PrecompileEnvironment     { return m.env }

var _ contract.AccessibleState = (*internalWithEnv)(nil)

// internalNoEnv is an internal contract.AccessibleState that does NOT expose an env
// (a test mock / non-EVM caller). The bridge must return nil for it (fail-secure).
type internalNoEnv struct{}

func (m *internalNoEnv) GetStateDB() contract.StateDB                 { return nil }
func (m *internalNoEnv) GetBlockContext() contract.BlockContext       { return nil }
func (m *internalNoEnv) GetConsensusContext() context.Context         { return context.Background() }
func (m *internalNoEnv) GetChainConfig() precompileconfig.ChainConfig { return nil }

var _ contract.AccessibleState = (*internalNoEnv)(nil)

// TestBridge_GetPrecompileEnv_WiredForwardsCall proves property (1) + (3): with a geth
// env present, the bridge returns a non-nil external env that satisfies callableEnv
// and forwards Call to the geth env — with the value passed through and ZERO call
// options (no caller proxying, so the token sees msg.sender == the precompile self,
// matching the depositor's approve(0x9999) allowance).
func TestBridge_GetPrecompileEnv_WiredForwardsCall(t *testing.T) {
	rec := &recordingEnv{readOnly: false}
	bridge := &accessibleStateBridge{internal: &internalWithEnv{env: rec}}

	env := bridge.GetPrecompileEnv()
	if env == nil {
		t.Fatal("GetPrecompileEnv() returned nil with a geth env present — ERC-20 settlement would refuse on-chain")
	}
	if env.ReadOnly() != false {
		t.Fatal("ReadOnly() not forwarded")
	}

	// Property (3): the EXACT shape the DEX precompile type-asserts.
	c, ok := env.(callableEnv)
	if !ok {
		t.Fatal("returned env does NOT satisfy callableEnv — DEX ERC-20 leg would refuse with ErrERC20VaultUnavailable")
	}

	// Property (1): Call forwards to the geth env with value and no options.
	tokenAddr := common.HexToAddress("0x000000000000000000000000000000000000C0FE")
	input := []byte{0x23, 0xb8, 0x72, 0xdd, 0x01} // transferFrom selector + a byte
	ret, gasLeft, err := c.Call(tokenAddr, input, 100_000, big.NewInt(0))
	if err != nil {
		t.Fatalf("Call forwarded an error: %v", err)
	}
	if string(ret) != "ok" || gasLeft != 100_000 {
		t.Fatalf("Call did not forward to the geth env: ret=%q gasLeft=%d", ret, gasLeft)
	}
	if rec.calls != 1 {
		t.Fatalf("geth env Call invoked %d times, want 1", rec.calls)
	}
	if rec.lastAddr != tokenAddr || string(rec.lastIn) != string(input) || rec.lastGas != 100_000 {
		t.Fatalf("Call args not forwarded faithfully: addr=%s gas=%d", rec.lastAddr, rec.lastGas)
	}
	if rec.lastOpts != 0 {
		t.Fatalf("Call forwarded %d CallOptions, want 0 — caller proxying would break the ERC-20 allowance (token would check allowance for the wrong spender)", rec.lastOpts)
	}
}

// TestBridge_GetPrecompileEnv_NoEnv_FailsSecure proves property (2): without a geth
// env, the bridge returns nil so the precompile fails-secure (ErrERC20VaultUnavailable)
// rather than mint an unbacked claim. This is the safe default on any chain/harness
// that does not provide an EVM execution environment.
func TestBridge_GetPrecompileEnv_NoEnv_FailsSecure(t *testing.T) {
	bridge := &accessibleStateBridge{internal: &internalNoEnv{}}
	if env := bridge.GetPrecompileEnv(); env != nil {
		t.Fatalf("GetPrecompileEnv() returned non-nil (%T) without a geth env — would expose a broken Call surface", env)
	}
}

// TestBridge_GetPrecompileEnv_TypedNilEnv_FailsSecure guards the typed-nil trap: when
// the internal adapter exposes the envProvider accessor but its env is nil, the bridge
// MUST still return an untyped nil (not a *precompileEnvBridge wrapping a nil env),
// so the precompile's nil check fires instead of nil-dereferencing on Call.
func TestBridge_GetPrecompileEnv_TypedNilEnv_FailsSecure(t *testing.T) {
	bridge := &accessibleStateBridge{internal: &internalWithEnv{env: nil}}
	if env := bridge.GetPrecompileEnv(); env != nil {
		t.Fatalf("GetPrecompileEnv() returned non-nil (%T) for a nil env — typed-nil trap; precompile would nil-deref on Call", env)
	}
}

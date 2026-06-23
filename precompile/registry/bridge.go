// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Bridge between external precompile registry (github.com/luxfi/precompile/modules)
// and the EVM's internal precompile registry (github.com/luxfi/evm/precompile/modules).
//
// External precompiles register into their own registry via init() functions.
// This bridge copies them into the EVM's registry with adapter types so the
// config parser and execution engine can find and run them.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/holiman/uint256"

	// Internal (EVM) types
	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/evm/precompile/modules"
	"github.com/luxfi/evm/precompile/precompileconfig"

	// External precompile types
	extcontract "github.com/luxfi/precompile/contract"
	"github.com/luxfi/precompile/dex"
	extmodules "github.com/luxfi/precompile/modules"
	extconfig "github.com/luxfi/precompile/precompileconfig"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	ethtypes "github.com/luxfi/geth/core/types"
	gethvm "github.com/luxfi/geth/core/vm"
	"github.com/luxfi/geth/log"
	"github.com/luxfi/ids"
	"github.com/luxfi/vm/chains/atomic"
)

func init() {
	bridgeExternalModules()
}

// bridgeExternalModules copies all modules from the external precompile registry
// into the EVM's internal registry, wrapping them with adapter types.
func bridgeExternalModules() {
	for _, extMod := range extmodules.RegisteredModules() {
		// Skip if already registered internally (e.g., warp, allowlist, etc.)
		if _, exists := modules.GetPrecompileModule(extMod.ConfigKey); exists {
			continue
		}

		intMod := modules.Module{
			ConfigKey:    extMod.ConfigKey,
			Address:      extMod.Address,
			Contract:     &contractBridge{ext: extMod.Contract},
			Configurator: &configuratorBridge{ext: extMod.Configurator, key: extMod.ConfigKey},
			AlwaysOn:     extMod.AlwaysOn,
		}

		if err := modules.RegisterBridgedModule(intMod); err != nil {
			log.Warn("Failed to bridge external precompile module",
				"key", extMod.ConfigKey,
				"address", extMod.Address,
				"error", err,
			)
			continue
		}

		log.Debug("Bridged external precompile into EVM registry",
			"key", extMod.ConfigKey,
			"address", extMod.Address,
		)
	}
}

// =============================================================================
// Contract Bridge
// =============================================================================

// contractBridge wraps an external StatefulPrecompiledContract to implement the
// internal contract.StatefulPrecompiledContract interface.
type contractBridge struct {
	ext extcontract.StatefulPrecompiledContract
}

func (b *contractBridge) Run(
	accessibleState contract.AccessibleState,
	caller common.Address,
	addr common.Address,
	input []byte,
	suppliedGas uint64,
	readOnly bool,
) ([]byte, uint64, error) {
	// Wrap the internal AccessibleState as an external one
	extState := &accessibleStateBridge{internal: accessibleState}
	return b.ext.Run(extState, caller, addr, input, suppliedGas, readOnly)
}

// =============================================================================
// AccessibleState Bridge (internal → external)
// =============================================================================

// accessibleStateBridge wraps an internal AccessibleState to implement
// the external contract.AccessibleState interface.
type accessibleStateBridge struct {
	internal contract.AccessibleState
}

func (a *accessibleStateBridge) GetStateDB() extcontract.StateDB {
	return &stateDBBridge{internal: a.internal.GetStateDB()}
}

func (a *accessibleStateBridge) GetBlockContext() extcontract.BlockContext {
	return &blockContextBridge{internal: a.internal.GetBlockContext()}
}

func (a *accessibleStateBridge) GetConsensusContext() context.Context {
	return a.internal.GetConsensusContext()
}

func (a *accessibleStateBridge) GetChainConfig() extconfig.ChainConfig {
	return &extChainConfigBridge{internal: a.internal.GetChainConfig()}
}

// envProvider is the concrete accessor the internal accessibleStateAdapter
// (evm/core/precompile_overrider.go) exposes to hand the registry bridge the raw
// geth precompile execution environment. It is kept OFF the EVM-INTERNAL
// contract.AccessibleState interface (which has many implementers/mocks) and
// type-asserted here so only the production adapter — which actually holds a geth
// env — can supply one. (NOTE: the EXTERNAL contract.AccessibleState interface this
// bridge implements DOES expose GetPrecompileEnv() to every external precompile;
// GetPrecompileEnv below gates which of them actually receive a callable env.)
type envProvider interface {
	GetPrecompileEnv() gethvm.PrecompileEnvironment
}

// callableEnvAllowlist is the set of precompile self-addresses that legitimately need
// the EVM Call sub-call surface (to move ERC-20 value through the token contract for
// C<->D settlement). It is the DEX settlement family ONLY:
//   - 0x9999 (LXSettleAddress): the sole money path — deposit/withdraw/swap legs pull
//     and push ERC-20 via transferFrom/transfer with 0x9999 as msg.sender.
//   - 0x9996 (DEXPositionManagerAddress): composes 0x9999; its LP-commit leg moves
//     ERC-20 liquidity with 0x9996 as msg.sender.
//
// Every OTHER external precompile (the PQ/ZK verifiers at 0x0122xx, etc.) gets a nil
// callable env and falls back to its fail-secure refusal. Those precompiles do not
// type-assert callableEnv today, so this changes no live behaviour — it is
// defense-in-depth so a FUTURE external precompile cannot silently acquire EVM sub-
// call power (with itself as msg.sender) merely by asserting the optional capability.
// The addresses are the canonical luxfi/precompile/dex constants (one source of
// truth; that package is already in this module's dependency graph via registry.go).
var callableEnvAllowlist = map[common.Address]struct{}{
	common.HexToAddress(dex.LXSettleAddress):           {},
	common.HexToAddress(dex.DEXPositionManagerAddress): {},
}

// GetPrecompileEnv returns the external precompile execution environment with a live
// Call surface, forwarded from the underlying geth env. This is what makes the DEX
// 0x9999 settlement precompile's ERC-20 leg execute on-chain: the precompile sub-
// calls the token contract (transferFrom / transfer / balanceOf) through this env's
// Call. When the internal adapter does not provide a geth env (a non-EVM caller or a
// test mock), this returns nil and the precompile fails-secure (ErrERC20VaultUnavailable)
// rather than mint an unbacked claim.
//
// SHARED-INTERFACE NOTE: this method IS on the shared external contract.AccessibleState
// interface, so every external precompile receives this bridge. The Call surface is a
// powerful capability (the sub-call's msg.sender is the asserting precompile's own
// address), so it is GATED here: only the DEX settlement addresses in
// callableEnvAllowlist receive a non-nil callable env; all others get nil (fail-secure
// refusal). This is defense-in-depth on top of each settlement Run's CALL-only guard.
func (a *accessibleStateBridge) GetPrecompileEnv() extcontract.PrecompileEnvironment {
	p, ok := a.internal.(envProvider)
	if !ok {
		return nil
	}
	env := p.GetPrecompileEnv()
	if env == nil {
		return nil
	}
	// Gate the Call surface to the DEX settlement family. The precompile self-address
	// the geth env carries identifies WHO is asking; only those addresses get Call.
	if _, allowed := callableEnvAllowlist[env.Addresses().Self]; !allowed {
		return nil
	}
	return &precompileEnvBridge{env: env}
}

// precompileEnvBridge adapts a geth vm.PrecompileEnvironment to the external
// contract.PrecompileEnvironment, ADDING the Call method the DEX precompile type-
// asserts (callableEnv). contract.PrecompileEnvironment itself declares only
// ReadOnly(); Call is an OPTIONAL concrete capability the precompile reaches via a
// type assertion, so adding it here does not widen the shared interface.
type precompileEnvBridge struct {
	env gethvm.PrecompileEnvironment
}

func (p *precompileEnvBridge) ReadOnly() bool { return p.env.ReadOnly() }

// Call forwards a contract sub-call to the geth env. The caller of the sub-call is
// the asserting precompile's SELF address (the geth env's default — NO caller
// proxying). For the DEX settlement family this is the settlement address itself
// (0x9999, or 0x9996 for LP commits): that matches the allowance a depositor granted
// the vault via approve(0x9999, amount), so transferFrom pulls against the correct
// allowance. The settlement Run's CALL-only guard guarantees self is the settlement
// address (never a DELEGATECALL delegator). The variadic CallOption tail of the geth
// signature is intentionally dropped (no proxying); the external callableEnv shape is
// the no-option form the precompile expects.
func (p *precompileEnvBridge) Call(addr common.Address, input []byte, gas uint64, value *big.Int) ([]byte, uint64, error) {
	return p.env.Call(addr, input, gas, value)
}

// accessibleStateBridge forwards the OPTIONAL cross-chain atomic capability
// (external contract.AtomicState) to the internal AccessibleState when the
// concrete internal adapter implements the internal contract.AtomicState. A
// precompile in github.com/luxfi/precompile/* type-asserts the external
// AtomicState; this bridge makes that assertion succeed by delegating to the
// internal adapter (the EVM's accessibleStateAdapter). When the internal state
// is not atomic-capable (a test mock), AtomicMemory() returns nil so the
// precompile reverts.
var _ extcontract.AtomicState = (*accessibleStateBridge)(nil)

// internalAtomic mirrors the internal contract.AtomicState so the bridge can
// type-assert the internal AccessibleState without importing it by name twice.
type internalAtomic interface {
	AtomicMemory() atomic.SharedMemory
	NetworkID() uint32
	ChainID() ids.ID
	CChainID() ids.ID
	GovernanceController() common.Address
	DChainID() ids.ID
	TxID() ids.ID
	CallIndex() uint32
}

func (a *accessibleStateBridge) atomicOrNil() internalAtomic {
	if at, ok := a.internal.(internalAtomic); ok {
		return at
	}
	return nil
}

func (a *accessibleStateBridge) AtomicMemory() atomic.SharedMemory {
	if at := a.atomicOrNil(); at != nil {
		return at.AtomicMemory()
	}
	return nil
}

func (a *accessibleStateBridge) NetworkID() uint32 {
	if at := a.atomicOrNil(); at != nil {
		return at.NetworkID()
	}
	return 0
}

func (a *accessibleStateBridge) ChainID() ids.ID {
	if at := a.atomicOrNil(); at != nil {
		return at.ChainID()
	}
	return ids.Empty
}

func (a *accessibleStateBridge) CChainID() ids.ID {
	if at := a.atomicOrNil(); at != nil {
		return at.CChainID()
	}
	return ids.Empty
}

// GovernanceController delegates to the inner atomic capability — the per-network DEX
// governance authority (a governance contract, never a dev-mnemonic EOA). Returns the
// zero address when the inner state is not atomic-capable, which the precompile treats
// as fail-closed (halt/seed revert).
func (a *accessibleStateBridge) GovernanceController() common.Address {
	if at := a.atomicOrNil(); at != nil {
		return at.GovernanceController()
	}
	return common.Address{}
}

func (a *accessibleStateBridge) DChainID() ids.ID {
	if at := a.atomicOrNil(); at != nil {
		return at.DChainID()
	}
	return ids.Empty
}

func (a *accessibleStateBridge) TxID() ids.ID {
	if at := a.atomicOrNil(); at != nil {
		return at.TxID()
	}
	return ids.Empty
}

func (a *accessibleStateBridge) CallIndex() uint32 {
	if at := a.atomicOrNil(); at != nil {
		return at.CallIndex()
	}
	return 0
}

// =============================================================================
// StateDB Bridge (internal → external)
// =============================================================================

// stateDBBridge wraps an internal contract.StateDB to implement
// the external contract.StateDB interface.
//
// The external interface is wider (SubBalance, tracing reasons, MultiCoin, etc.).
// Methods not directly available on the internal interface either:
//   - Use type assertions to reach the underlying vm.StateDB
//   - Provide reasonable defaults
type stateDBBridge struct {
	internal contract.StateDB
}

func (s *stateDBBridge) GetState(addr common.Address, key common.Hash) common.Hash {
	return s.internal.GetState(addr, key)
}

func (s *stateDBBridge) SetState(addr common.Address, key, val common.Hash) common.Hash {
	s.internal.SetState(addr, key, val)
	return val
}

func (s *stateDBBridge) SetNonce(addr common.Address, nonce uint64, _ tracing.NonceChangeReason) {
	s.internal.SetNonce(addr, nonce)
}

func (s *stateDBBridge) GetNonce(addr common.Address) uint64 {
	return s.internal.GetNonce(addr)
}

func (s *stateDBBridge) GetBalance(addr common.Address) *uint256.Int {
	return s.internal.GetBalance(addr)
}

func (s *stateDBBridge) AddBalance(addr common.Address, amount *uint256.Int, _ tracing.BalanceChangeReason) uint256.Int {
	s.internal.AddBalance(addr, amount)
	// Return new balance
	bal := s.internal.GetBalance(addr)
	if bal != nil {
		return *bal
	}
	return uint256.Int{}
}

func (s *stateDBBridge) SubBalance(addr common.Address, amount *uint256.Int, _ tracing.BalanceChangeReason) uint256.Int {
	// The internal contract.StateDB doesn't have SubBalance.
	// Reach the underlying vm.StateDB via type assertion.
	type subBalancer interface {
		SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) uint256.Int
	}
	sb, ok := s.internal.(subBalancer)
	if !ok {
		// FAIL CLOSED. There is NO safe fallback: the internal contract.StateDB has no
		// way to subtract, so silently returning a zero "previous balance" without
		// debiting (the old behaviour) would let a value-moving precompile — e.g. the
		// 0x9999 DEX custody vault on withdraw — treat the debit as successful while the
		// balance is untouched, MINTING native value. A failed debit on the money path
		// must abort the call, never proceed. We panic: the EVM recovers a precompile
		// panic into a reverted call, so the enclosing value move is rolled back rather
		// than committed half-done. In production the internal StateDB is always the
		// concrete stateDBAdapter{vm.StateDB} (precompile_overrider.go), which DOES
		// implement SubBalance, so this panic is structurally unreachable; it exists to
		// make any future StateDB that omits SubBalance fail loudly instead of minting.
		log.Error("stateDBBridge: SubBalance on a StateDB without subtraction support — failing closed to avoid minting native value",
			"address", addr, "amount", amount)
		panic(fmt.Sprintf("stateDBBridge: internal StateDB %T cannot SubBalance; refusing to credit without debiting (addr=%s amount=%s)", s.internal, addr, amount))
	}
	return sb.SubBalance(addr, amount, tracing.BalanceChangeUnspecified)
}

func (s *stateDBBridge) GetBalanceMultiCoin(addr common.Address, coinID common.Hash) *big.Int {
	// MultiCoin not available through internal interface
	return new(big.Int)
}

func (s *stateDBBridge) AddBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	// MultiCoin not available through internal interface
}

func (s *stateDBBridge) SubBalanceMultiCoin(addr common.Address, coinID common.Hash, amount *big.Int) {
	// MultiCoin not available through internal interface
}

func (s *stateDBBridge) CreateAccount(addr common.Address) {
	s.internal.CreateAccount(addr)
}

func (s *stateDBBridge) Exist(addr common.Address) bool {
	return s.internal.Exist(addr)
}

func (s *stateDBBridge) AddLog(log *ethtypes.Log) {
	s.internal.AddLog(log)
}

func (s *stateDBBridge) Logs() []*ethtypes.Log {
	// Try type assertion for Logs()
	type logger interface {
		Logs() []*ethtypes.Log
	}
	if l, ok := s.internal.(logger); ok {
		return l.Logs()
	}
	return nil
}

func (s *stateDBBridge) GetPredicateStorageSlots(addr common.Address, index int) ([]byte, bool) {
	return s.internal.GetPredicateStorageSlots(addr, index)
}

func (s *stateDBBridge) TxHash() common.Hash {
	return s.internal.GetTxHash()
}

func (s *stateDBBridge) Snapshot() int {
	return s.internal.Snapshot()
}

func (s *stateDBBridge) RevertToSnapshot(id int) {
	s.internal.RevertToSnapshot(id)
}

// =============================================================================
// BlockContext Bridge (internal → external)
// =============================================================================

type blockContextBridge struct {
	internal contract.BlockContext
}

func (b *blockContextBridge) Number() *big.Int {
	return b.internal.Number()
}

func (b *blockContextBridge) Timestamp() uint64 {
	return b.internal.Timestamp()
}

func (b *blockContextBridge) GetPredicateResults(txHash common.Hash, addr common.Address) []byte {
	return b.internal.GetPredicateResults(txHash, addr)
}

// =============================================================================
// ChainConfig Bridge (internal → external)
// =============================================================================

type extChainConfigBridge struct {
	internal precompileconfig.ChainConfig
}

func (c *extChainConfigBridge) IsDurango(time uint64) bool {
	return c.internal.IsDurango(time)
}

// IsStrictPQ satisfies the external contract.StrictPQReporter interface so
// classical precompiles in github.com/luxfi/precompile/* refuse to execute
// when the chain pins a strict post-quantum profile. The internal config
// (chainConfigAdapter) delegates to extras.IsStrictPQ. We type-assert
// rather than widen precompileconfig.ChainConfig so non-Lux chains that
// integrate Lux precompiles remain classical-permissive by default.
func (c *extChainConfigBridge) IsStrictPQ(time uint64) bool {
	type strictPQ interface {
		IsStrictPQ(time uint64) bool
	}
	if r, ok := c.internal.(strictPQ); ok {
		return r.IsStrictPQ(time)
	}
	return false
}

// =============================================================================
// Configurator Bridge (external → internal)
// =============================================================================

// configuratorBridge wraps an external Configurator to implement the
// internal contract.Configurator interface.
type configuratorBridge struct {
	ext extcontract.Configurator
	key string
}

func (c *configuratorBridge) MakeConfig() precompileconfig.Config {
	extCfg := c.ext.MakeConfig()
	return &configBridge{ext: extCfg, key: c.key}
}

func (c *configuratorBridge) MakeGenesisConfig() precompileconfig.Config {
	extCfg := c.ext.MakeConfig()
	// Set timestamp to 0 for genesis activation
	cfg := &configBridge{ext: extCfg, key: c.key}
	cfg.genesisTimestamp = new(uint64) // *uint64 pointing to 0
	return cfg
}

func (c *configuratorBridge) Configure(
	chainConfig precompileconfig.ChainConfig,
	cfg precompileconfig.Config,
	state contract.StateDB,
	blockContext contract.ConfigurationBlockContext,
) error {
	// Unwrap the config if it's our bridge type
	var extCfg extconfig.Config
	if bridged, ok := cfg.(*configBridge); ok {
		extCfg = bridged.ext
	} else {
		// Config was not created by us — create a default
		extCfg = c.ext.MakeConfig()
	}

	// Create external StateDB and BlockContext adapters
	extState := &stateDBBridge{internal: state}
	extBlock := &configBlockContextBridge{internal: blockContext}
	extChain := &extChainConfigBridge{internal: chainConfig}

	return c.ext.Configure(extChain, extCfg, extState, extBlock)
}

// configBlockContextBridge wraps internal ConfigurationBlockContext for external use
type configBlockContextBridge struct {
	internal contract.ConfigurationBlockContext
}

func (b *configBlockContextBridge) Number() *big.Int {
	return b.internal.Number()
}

func (b *configBlockContextBridge) Timestamp() uint64 {
	return b.internal.Timestamp()
}

// =============================================================================
// Config Bridge (external → internal)
// =============================================================================

// configBridge wraps an external precompileconfig.Config to implement the
// internal precompileconfig.Config interface.
//
// Critically, this must support JSON marshal/unmarshal since the config
// is deserialized from upgrade.json.
type configBridge struct {
	ext              extconfig.Config
	key              string
	genesisTimestamp *uint64 // override timestamp for genesis configs
}

func (c *configBridge) Key() string {
	return c.ext.Key()
}

func (c *configBridge) Timestamp() *uint64 {
	if c.genesisTimestamp != nil {
		return c.genesisTimestamp
	}
	return c.ext.Timestamp()
}

func (c *configBridge) IsDisabled() bool {
	return c.ext.IsDisabled()
}

func (c *configBridge) Equal(other precompileconfig.Config) bool {
	otherBridge, ok := other.(*configBridge)
	if !ok {
		return false
	}
	return c.ext.Equal(otherBridge.ext)
}

func (c *configBridge) Verify(chainConfig precompileconfig.ChainConfig) error {
	extChain := &extChainConfigBridge{internal: chainConfig}
	return c.ext.Verify(extChain)
}

// UnmarshalJSON handles the format difference between internal and external configs.
//
// Internal configs embed precompileconfig.Upgrade (no JSON tag), so fields like
// "blockTimestamp" are at the top level: {"blockTimestamp":0}
//
// External configs use a tagged field: Upgrade `json:"upgrade,omitempty"`, so they
// expect: {"upgrade":{"blockTimestamp":0}}
//
// This method translates between the two formats.
func (c *configBridge) UnmarshalJSON(data []byte) error {
	// Try direct unmarshal first
	if err := json.Unmarshal(data, c.ext); err != nil {
		return err
	}
	// If timestamp is already set, we're done (external config handled it)
	if c.ext.Timestamp() != nil {
		return nil
	}

	// Timestamp is nil — the external config likely uses json:"upgrade" tag
	// and the data has top-level "blockTimestamp". Wrap it.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil // not a JSON object, but initial unmarshal succeeded
	}
	if ts, ok := raw["blockTimestamp"]; ok {
		// Build {"upgrade":{"blockTimestamp":N,"disable":false}}
		upgradeObj := map[string]json.RawMessage{"blockTimestamp": ts}
		if dis, ok := raw["disable"]; ok {
			upgradeObj["disable"] = dis
		}
		wrapped, _ := json.Marshal(map[string]json.RawMessage{
			"upgrade": mustMarshal(upgradeObj),
		})
		return json.Unmarshal(wrapped, c.ext)
	}

	return nil // no blockTimestamp in data at all
}

// MarshalJSON delegates to the external config's JSON marshaling.
func (c *configBridge) MarshalJSON() ([]byte, error) {
	return json.Marshal(c.ext)
}

// String returns a debug representation
func (c *configBridge) String() string {
	return fmt.Sprintf("configBridge{key=%s, ext=%v}", c.key, c.ext)
}

// mustMarshal marshals v to json.RawMessage, panicking on error (should never fail for map types).
func mustMarshal(v interface{}) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("mustMarshal: %v", err))
	}
	return data
}

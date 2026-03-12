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
	extmodules "github.com/luxfi/precompile/modules"
	extconfig "github.com/luxfi/precompile/precompileconfig"

	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	ethtypes "github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/log"
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

func (a *accessibleStateBridge) GetPrecompileEnv() extcontract.PrecompileEnvironment {
	return nil // Not used by any external precompile
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
	// Try to reach the underlying vm.StateDB via type assertion.
	type subBalancer interface {
		SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) uint256.Int
	}
	if sb, ok := s.internal.(subBalancer); ok {
		return sb.SubBalance(addr, amount, tracing.BalanceChangeUnspecified)
	}

	// Fallback: use snapshot + manual balance calculation
	// Get current balance, compute new, use AddBalance with negative (not possible)
	// Instead, log warning. This path should not be hit in practice because the
	// concrete stateDBAdapter in precompile_overrider.go wraps vm.StateDB which has SubBalance.
	bal := s.internal.GetBalance(addr)
	if bal != nil && bal.Cmp(amount) >= 0 {
		// Workaround: We can't subtract directly, so we reconstruct via the underlying
		// types. In practice this codepath is unreachable because the internal StateDB
		// is always a stateDBAdapter{vm.StateDB} which implements SubBalance.
		log.Error("stateDBBridge: SubBalance fallback used — precompile state may be incorrect",
			"address", addr, "amount", amount)
	}
	return uint256.Int{}
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

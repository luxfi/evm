// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"context"
	"math/big"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/precompile/contract"
	"github.com/luxfi/evm/precompile/modules"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	gethparams "github.com/luxfi/geth/params"
	"github.com/luxfi/runtime"
)

func init() {
	// Register the rules hook to populate Rules.Payload with our PrecompileOverrider
	gethparams.SetRulesHook(precompileHook)
}

// precompileHook populates Rules.Payload with the LuxPrecompileOverrider
func precompileHook(c *gethparams.ChainConfig, rules *gethparams.Rules, num *big.Int, isMerge bool, timestamp uint64) {
	// Store context for GetRulesExtra to use
	params.SetRulesContext(rules, c, timestamp)

	// Set the payload to our PrecompileOverrider
	rules.Payload = &LuxPrecompileOverrider{
		chainConfig: c,
		timestamp:   timestamp,
	}
}

// LuxPrecompileOverrider implements vm.PrecompileOverrider to provide
// custom Lux precompiles (warp, nativeminter, etc.) to the EVM.
type LuxPrecompileOverrider struct {
	chainConfig *gethparams.ChainConfig
	timestamp   uint64
}

// PrecompileOverride returns the precompile at the given address if it's
// an active Lux custom precompile.
func (o *LuxPrecompileOverrider) PrecompileOverride(addr common.Address) (vm.PrecompiledContract, bool) {
	// Get the extras rules to check active precompiles
	rulesExtra := params.GetRulesExtra(gethparams.Rules{})
	if !rulesExtra.IsPrecompileEnabled(addr) {
		return nil, false
	}

	// Find the module for this address
	for _, module := range modules.RegisteredModules() {
		if module.Address == addr && module.Contract != nil {
			// Wrap the contract in our adapter
			return &precompileAdapter{
				address:  addr,
				contract: module.Contract,
			}, true
		}
	}

	return nil, false
}

// precompileAdapter wraps a contract.StatefulPrecompiledContract to implement
// geth's vm.StatefulPrecompiledContract interface.
type precompileAdapter struct {
	address  common.Address
	contract contract.StatefulPrecompiledContract
}

// Name returns the precompile name
func (p *precompileAdapter) Name() string {
	// Get the config key for this address
	for _, module := range modules.RegisteredModules() {
		if module.Address == p.address {
			return module.ConfigKey
		}
	}
	return "unknown"
}

// RequiredGas returns the gas required for this precompile
func (p *precompileAdapter) RequiredGas(input []byte) uint64 {
	// For stateful precompiles, gas is calculated in RunStateful
	return 0
}

// Run implements PrecompiledContract.Run (stateless - not used for stateful precompiles)
func (p *precompileAdapter) Run(input []byte) ([]byte, error) {
	// Stateful precompiles use RunStateful instead
	return nil, vm.ErrExecutionReverted
}

// RunStateful implements StatefulPrecompiledContract.RunStateful
func (p *precompileAdapter) RunStateful(env vm.PrecompileEnvironment, input []byte, suppliedGas uint64) ([]byte, uint64, error) {
	// Create an AccessibleState adapter
	accessibleState := &accessibleStateAdapter{env: env}

	// Get caller and self addresses from environment
	addresses := env.Addresses()

	// Call the inner precompile
	ret, remainingGas, err := p.contract.Run(
		accessibleState,
		addresses.Caller,
		addresses.Self,
		input,
		suppliedGas,
		env.ReadOnly(),
	)

	return ret, remainingGas, err
}

// accessibleStateAdapter adapts vm.PrecompileEnvironment to contract.AccessibleState
type accessibleStateAdapter struct {
	env vm.PrecompileEnvironment
}

func (a *accessibleStateAdapter) GetStateDB() contract.StateDB {
	return &stateDBAdapter{stateDB: a.env.StateDB()}
}

func (a *accessibleStateAdapter) GetBlockContext() contract.BlockContext {
	return &blockContextAdapter{env: a.env}
}

func (a *accessibleStateAdapter) GetConsensusContext() context.Context {
	// The PrecompileEnvironment returns *runtime.Runtime, wrap it in context
	if rt := a.env.ConsensusRuntime(); rt != nil {
		return runtime.WithContext(context.Background(), rt)
	}
	return context.Background()
}

func (a *accessibleStateAdapter) GetChainConfig() precompileconfig.ChainConfig {
	return &chainConfigAdapter{config: a.env.ChainConfig()}
}

// stateDBAdapter adapts vm.StateDB to contract.StateDB
type stateDBAdapter struct {
	stateDB vm.StateDB
}

func (s *stateDBAdapter) GetState(addr common.Address, hash common.Hash) common.Hash {
	return s.stateDB.GetState(addr, hash)
}

func (s *stateDBAdapter) SetState(addr common.Address, key, value common.Hash) {
	s.stateDB.SetState(addr, key, value)
}

func (s *stateDBAdapter) SetNonce(addr common.Address, nonce uint64) {
	s.stateDB.SetNonce(addr, nonce, tracing.NonceChangeUnspecified)
}

func (s *stateDBAdapter) GetNonce(addr common.Address) uint64 {
	return s.stateDB.GetNonce(addr)
}

func (s *stateDBAdapter) GetBalance(addr common.Address) *uint256.Int {
	return s.stateDB.GetBalance(addr)
}

func (s *stateDBAdapter) AddBalance(addr common.Address, amount *uint256.Int) {
	s.stateDB.AddBalance(addr, amount, tracing.BalanceChangeUnspecified)
}

func (s *stateDBAdapter) CreateAccount(addr common.Address) {
	s.stateDB.CreateAccount(addr)
}

func (s *stateDBAdapter) Exist(addr common.Address) bool {
	return s.stateDB.Exist(addr)
}

func (s *stateDBAdapter) AddLog(log *types.Log) {
	s.stateDB.AddLog(log)
}

func (s *stateDBAdapter) GetPredicateStorageSlots(addr common.Address, index int) ([]byte, bool) {
	// This requires accessing the predicate storage which is not exposed in the standard StateDB
	// For now, return empty - this is only used for advanced predicates
	return nil, false
}

func (s *stateDBAdapter) GetTxHash() common.Hash {
	return s.stateDB.TxHash()
}

func (s *stateDBAdapter) Snapshot() int {
	return s.stateDB.Snapshot()
}

func (s *stateDBAdapter) RevertToSnapshot(id int) {
	s.stateDB.RevertToSnapshot(id)
}

// blockContextAdapter adapts vm.PrecompileEnvironment to contract.BlockContext
type blockContextAdapter struct {
	env vm.PrecompileEnvironment
}

func (b *blockContextAdapter) Number() *big.Int {
	return b.env.BlockNumber()
}

func (b *blockContextAdapter) Timestamp() uint64 {
	return b.env.BlockTime()
}

func (b *blockContextAdapter) GetPredicateResults(txHash common.Hash, precompileAddress common.Address) []byte {
	return nil // Not needed for standard precompiles
}

// chainConfigAdapter adapts geth's ChainConfig to precompileconfig.ChainConfig
type chainConfigAdapter struct {
	config *gethparams.ChainConfig
}

func (c *chainConfigAdapter) GetFeeConfig() commontype.FeeConfig {
	extra := params.GetExtra(c.config)
	return extra.FeeConfig
}

func (c *chainConfigAdapter) AllowedFeeRecipients() bool {
	extra := params.GetExtra(c.config)
	return extra.AllowFeeRecipients
}

func (c *chainConfigAdapter) IsDurango(time uint64) bool {
	extra := params.GetExtra(c.config)
	return extra.IsDurango(time)
}

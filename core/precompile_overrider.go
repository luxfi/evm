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
	"github.com/luxfi/ids"
	"github.com/luxfi/runtime"
	"github.com/luxfi/vm/chains/atomic"
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
//
// The enabled-set decision is computed from THIS overrider's OWN per-EVM fields
// (o.chainConfig + o.timestamp, set in precompileHook at the EVM's construction),
// NOT from the process-global lastRulesContext that params.GetRulesExtra(Rules{})
// would read. The geth EVM invokes PrecompileOverride lazily during opcode
// execution (when a CALL targets a precompile address), which is a DIFFERENT
// point in time than when precompileHook ran. Reading the last-writer-wins global
// here is a consensus-divergence bug for any TIMESTAMP-GATED precompile (genesis
// precompiles, precompileUpgrades): a concurrent eth_call/estimateGas/worker
// goroutine can rewrite the global timestamp between this EVM's construction and its
// tx dispatch, so a replayed block could see a different enabled set than it built
// against. Binding the decision to o.timestamp makes every replay of a given block
// produce the SAME enabled set on every validator, regardless of concurrent activity.
//
// The DEX settlement money path 0x9999 is AlwaysOn (FIRST-RUN, no dated fork): it is
// in the enabled set at EVERY timestamp, so it is immune to timestamp/global skew by
// construction — present from genesis on, on every replay. params.GetExtrasRules is a
// pure function of its arguments (it does NOT read the global) and injects the
// AlwaysOn modules unconditionally, so 0x9999 resolves here identically with no racy
// read. params.ChainConfig is a type alias of geth's ChainConfig, so o.chainConfig is
// passed directly.
func (o *LuxPrecompileOverrider) PrecompileOverride(addr common.Address) (vm.PrecompiledContract, bool) {
	extrasRules := params.GetExtrasRules(gethparams.Rules{}, o.chainConfig, o.timestamp)
	if cfg, ok := extrasRules.Precompiles[addr]; !ok || cfg.IsDisabled() {
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
	return a.env.ConsensusContext()
}

func (a *accessibleStateAdapter) GetChainConfig() precompileconfig.ChainConfig {
	return &chainConfigAdapter{config: a.env.ChainConfig()}
}

// GetPrecompileEnv exposes the underlying geth precompile execution environment so
// the registry bridge (precompile/registry/bridge.go) can forward its Call surface
// to an external precompile. This EVM-INTERNAL accessor is NOT on the EVM-internal
// contract.AccessibleState interface (which has many implementers/mocks that must
// stay minimal); the bridge type-asserts this concrete accessor (envProvider).
//
// NOTE on reach (do not mistake this for the only control): the bridge DOES expose
// GetPrecompileEnv() on the SHARED EXTERNAL contract.AccessibleState interface, so
// the Call surface is reachable by every external precompile that type-asserts it.
// The bridge therefore gates which precompiles get a non-nil callable env (only the
// DEX settlement addresses 0x9999/0x9996) and each settlement Run pins self to its
// own address — see precompile/registry/bridge.go and the dex settle CALL-only guard.
//
// The env's Call is what the DEX 0x9999 settlement precompile uses to move ERC-20
// value (transferFrom / transfer / balanceOf on the token contract) for its C<->D
// leg — the analog of the EVM pre-moving msg.value into 0x9999 for native LUX.
// Returns the concrete vm.PrecompileEnvironment, which already implements Call; the
// bridge narrows it to the external contract.PrecompileEnvironment shape the
// precompile type-asserts.
func (a *accessibleStateAdapter) GetPrecompileEnv() vm.PrecompileEnvironment {
	return a.env
}

// accessibleStateAdapter implements contract.AtomicState so a precompile that
// type-asserts the optional capability can reach the primary network's atomic
// shared memory and chain identity. The source is the chain Runtime embedded in
// the consensus context (runtime.WithContext at initializeChain); when the host
// wired no shared memory (single-chain dev / non-atomic harness) AtomicMemory()
// returns nil and the calling precompile reverts rather than fabricate value.
var _ contract.AtomicState = (*accessibleStateAdapter)(nil)

// runtimeFromCtx pulls the chain Runtime out of the consensus context, or nil.
func (a *accessibleStateAdapter) runtimeFromCtx() *runtime.Runtime {
	return runtime.FromContext(a.env.ConsensusContext())
}

func (a *accessibleStateAdapter) AtomicMemory() atomic.SharedMemory {
	rt := a.runtimeFromCtx()
	if rt == nil {
		return nil
	}
	return rt.GetSharedMemory()
}

func (a *accessibleStateAdapter) NetworkID() uint32 {
	if rt := a.runtimeFromCtx(); rt != nil {
		return rt.NetworkID
	}
	return 0
}

func (a *accessibleStateAdapter) ChainID() ids.ID {
	if rt := a.runtimeFromCtx(); rt != nil {
		return rt.ChainID
	}
	return ids.Empty
}

func (a *accessibleStateAdapter) CChainID() ids.ID {
	if rt := a.runtimeFromCtx(); rt != nil {
		// On the C-Chain CChainID == ChainID; prefer the explicit field when set so
		// the binding is correct on chains that distinguish the two.
		if rt.CChainID != ids.Empty {
			return rt.CChainID
		}
		return rt.ChainID
	}
	return ids.Empty
}

// GovernanceController returns the per-network DEX governance authority address the
// host wired onto the chain runtime (installDEXValuePath, from the deployment topology)
// — the SOLE caller permitted to halt 0x9999 settlement or seed its pots. It is a
// governance CONTRACT, never a dev-mnemonic EOA. Returns the zero address when the
// runtime is absent or the network configured no governance controller, which the
// precompile treats as fail-closed (halt/seed revert). ids.ShortID and common.Address
// are both [20]byte, so the conversion is exact.
func (a *accessibleStateAdapter) GovernanceController() common.Address {
	if rt := a.runtimeFromCtx(); rt != nil {
		return common.Address(rt.GovernanceController)
	}
	return common.Address{}
}

// DChainID resolves the D-Chain (dexvm) blockchain id from the runtime chain topology
// — the consensus context's blockchain-alias lookup of "D". The node registers the
// dexvm chain under the "D"/"dex"/"dexvm" aliases at startup (initChainAliases), before
// any chain bootstraps, so the lookup is populated and identical on every validator and
// every re-execution. This is the always-on DEX settlement seam's D peer with ZERO
// per-net config. Returns ids.Empty when the runtime is absent or the network has no
// dexvm deployed (the "D" alias does not resolve) — the calling precompile then keeps
// the seam closed rather than guess a peer.
func (a *accessibleStateAdapter) DChainID() ids.ID {
	rt := a.runtimeFromCtx()
	if rt == nil {
		return ids.Empty
	}
	bc := rt.GetBCLookup()
	if bc == nil {
		return ids.Empty
	}
	dID, err := bc.Lookup("D")
	if err != nil {
		return ids.Empty
	}
	return dID
}

func (a *accessibleStateAdapter) TxID() ids.ID {
	// The EVM tx hash is the transaction identity the precompile binds its
	// cross-chain object to. ids.ID and common.Hash are both 32 bytes.
	return ids.ID(a.env.StateDB().TxHash())
}

func (a *accessibleStateAdapter) CallIndex() uint32 {
	return a.env.CallIndex()
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

// SubBalance forwards to the underlying vm.StateDB with the FULL external
// signature (addr, amount, reason) -> prev. The registry's stateDBBridge type-
// asserts the internal contract.StateDB for exactly this method so that a
// precompile which moves native value (e.g. the DEX 0x9010 custody vault: a
// withdraw debits the vault before releasing to the caller) actually subtracts
// the balance instead of hitting the bridge's no-op fallback. AddBalance is the
// narrow internal pair; SubBalance is provided here so the pair is complete on
// the concrete adapter without widening the internal interface (which has many
// implementers/mocks). Without this the bridge logged "SubBalance fallback used"
// and silently MINTED native value (vault unchanged, caller credited).
func (s *stateDBAdapter) SubBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	return s.stateDB.SubBalance(addr, amount, reason)
}

func (s *stateDBAdapter) CreateAccount(addr common.Address) {
	s.stateDB.CreateAccount(addr)
}

func (s *stateDBAdapter) Exist(addr common.Address) bool {
	return s.stateDB.Exist(addr)
}

// GetCodeSize forwards the EXTCODESIZE primitive to the underlying vm.StateDB. It is the
// concrete-adapter half of an OPTIONAL capability the DEX 0x9999 value path type-asserts
// (codeStater) to prove an ERC-20 asset is backed by LIVE on-chain code BEFORE admitting a
// swap/market over it (C1 real-asset admission). Like SubBalance above, it is provided on
// the concrete adapter — NOT added to the narrow internal contract.StateDB interface (which
// has many implementers/mocks) — so the live-reality proof reads the same authoritative
// state the value moves through, while the interface stays minimal. Without it a token whose
// contract self-destructed (or was never deployed on a relaunched chain) could be traded
// against a phantom; with it such an asset has code size 0 and the value path refuses.
func (s *stateDBAdapter) GetCodeSize(addr common.Address) int {
	return s.stateDB.GetCodeSize(addr)
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

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/luxfi/evm/commontype"
	"github.com/luxfi/evm/precompile/modules"
	"github.com/luxfi/evm/precompile/precompileconfig"
	"github.com/luxfi/evm/utils"
	"github.com/luxfi/geth/common"
	ethparams "github.com/luxfi/geth/params"
	"github.com/luxfi/upgrade"
)

var (
	// TODO: UnscheduledActivationTime seems to be removed from upgrade package
	UnscheduledActivationTime = time.Unix(1<<63-1, 0) // Max time value
	InitiallyActiveTime       = time.Unix(0, 0)       // Unix epoch

	// LegacyWarpAddress is the historical warp precompile address used in C-Chain
	// and legacy Subnet-EVM chains. New chains should use the LP-aligned address (0x16201).
	LegacyWarpAddress = common.HexToAddress("0x0200000000000000000000000000000000000005")

	DefaultFeeConfig = commontype.FeeConfig{
		GasLimit:        big.NewInt(8_000_000),
		TargetBlockRate: 2, // in seconds

		MinBaseFee:               big.NewInt(25_000_000_000),
		TargetGas:                big.NewInt(15_000_000),
		BaseFeeChangeDenominator: big.NewInt(36),

		MinBlockGasCost:  big.NewInt(0),
		MaxBlockGasCost:  big.NewInt(1_000_000),
		BlockGasCostStep: big.NewInt(200_000),
	}

	// TestFeeConfig uses minimal fees for backward compatibility with tests
	TestFeeConfig = commontype.FeeConfig{
		GasLimit:        big.NewInt(8_000_000),
		TargetBlockRate: 2,

		MinBaseFee:               big.NewInt(1), // 1 wei for test compatibility
		TargetGas:                big.NewInt(15_000_000),
		BaseFeeChangeDenominator: big.NewInt(36),

		MinBlockGasCost:  big.NewInt(0),
		MaxBlockGasCost:  big.NewInt(1_000_000),
		BlockGasCostStep: big.NewInt(200_000),
	}

	EVMDefaultChainConfig = &ChainConfig{
		FeeConfig:          DefaultFeeConfig,
		NetworkUpgrades:    GetDefaultNetworkUpgrades(), // TODO: upgrade.GetConfig seems to be removed
		GenesisPrecompiles: Precompiles{},
	}

	TestChainConfig = &ChainConfig{
		FeeConfig:          TestFeeConfig, // Use TestFeeConfig with MinBaseFee=1 for test compatibility
		NetworkUpgrades:    GetDefaultNetworkUpgrades(),
		GenesisPrecompiles: Precompiles{},
	}

	TestPreEVMChainConfig = copyAndSet(TestChainConfig, func(c *ChainConfig) {
		c.NetworkUpgrades = NetworkUpgrades{
			EVMTimestamp:     utils.TimeToNewUint64(UnscheduledActivationTime),
			DurangoTimestamp: utils.TimeToNewUint64(UnscheduledActivationTime),
			EtnaTimestamp:    utils.TimeToNewUint64(UnscheduledActivationTime),
			FortunaTimestamp: utils.TimeToNewUint64(UnscheduledActivationTime),
		}
	})

	TestEVMChainConfig = copyAndSet(TestChainConfig, func(c *ChainConfig) {
		c.NetworkUpgrades = NetworkUpgrades{
			EVMTimestamp:     utils.NewUint64(0),
			DurangoTimestamp: utils.TimeToNewUint64(UnscheduledActivationTime),
			EtnaTimestamp:    utils.TimeToNewUint64(UnscheduledActivationTime),
			FortunaTimestamp: utils.TimeToNewUint64(UnscheduledActivationTime),
		}
	})

	TestDurangoChainConfig = copyAndSet(TestChainConfig, func(c *ChainConfig) {
		c.NetworkUpgrades = NetworkUpgrades{
			EVMTimestamp:     utils.NewUint64(0),
			DurangoTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
			EtnaTimestamp:    utils.TimeToNewUint64(UnscheduledActivationTime),
			FortunaTimestamp: utils.TimeToNewUint64(UnscheduledActivationTime),
		}
	})

	TestEtnaChainConfig = copyAndSet(TestChainConfig, func(c *ChainConfig) {
		c.NetworkUpgrades = NetworkUpgrades{
			EVMTimestamp:     utils.NewUint64(0),
			DurangoTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
			EtnaTimestamp:    utils.TimeToNewUint64(InitiallyActiveTime),
			FortunaTimestamp: utils.TimeToNewUint64(UnscheduledActivationTime),
		}
	})

	TestFortunaChainConfig = copyAndSet(TestChainConfig, func(c *ChainConfig) {
		c.NetworkUpgrades = NetworkUpgrades{
			EVMTimestamp:     utils.NewUint64(0),
			DurangoTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
			EtnaTimestamp:    utils.TimeToNewUint64(InitiallyActiveTime),
			FortunaTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
		}
	})

	TestGraniteChainConfig = copyAndSet(TestChainConfig, func(c *ChainConfig) {
		c.NetworkUpgrades = NetworkUpgrades{
			EVMTimestamp:     utils.NewUint64(0),
			DurangoTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
			EtnaTimestamp:    utils.TimeToNewUint64(InitiallyActiveTime),
			FortunaTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
			GraniteTimestamp: utils.TimeToNewUint64(InitiallyActiveTime),
		}
	})
)

func copyAndSet(c *ChainConfig, set func(*ChainConfig)) *ChainConfig {
	newConfig := *c
	set(&newConfig)
	return &newConfig
}

// UpgradeConfig includes the following configs that may be specified in upgradeBytes:
// - Timestamps that enable lux network upgrades,
// - Enabling or disabling precompiles as network upgrades.
type UpgradeConfig struct {
	// Config for timestamps that enable network upgrades.
	NetworkUpgradeOverrides *NetworkUpgrades `json:"networkUpgradeOverrides,omitempty"`

	// Config for modifying state as a network upgrade.
	StateUpgrades []StateUpgrade `json:"stateUpgrades,omitempty"`

	// Config for enabling and disabling precompiles as network upgrades.
	PrecompileUpgrades []PrecompileUpgrade `json:"precompileUpgrades,omitempty"`
}

// LuxContext provides Lux specific context directly into the EVM.
type LuxContext struct {
	ConsensusCtx context.Context
}

type ChainConfig struct {
	NetworkUpgrades // Config for timestamps that enable network upgrades.

	LuxContext `json:"-"` // Lux specific context set during VM initialization. Not serialized.

	FeeConfig          commontype.FeeConfig `json:"feeConfig"`                    // Set the configuration for the dynamic fee algorithm
	AllowFeeRecipients bool                 `json:"allowFeeRecipients,omitempty"` // Allows fees to be collected by block builders.
	GenesisPrecompiles Precompiles          `json:"-"`                            // Config for enabling precompiles from genesis. JSON encode/decode will be handled by the custom marshaler/unmarshaler.
	UpgradeConfig      `json:"-"`           // Config specified in upgradeBytes (lux network upgrades or enable/disabling precompiles). Not serialized.

	// AddressBook provides per-chain address overrides for precompile addresses.
	// This allows historical chains (e.g., C-Chain) to use legacy addresses
	// (like 0x0200...0005 for warp) while new chains use LP-aligned addresses.
	// Resolution order: AddressBook[configKey] -> module.Address (compiled-in default)
	AddressBook map[string]common.Address `json:"addressBook,omitempty"`
}

// GetPrecompileAddress returns the address for a precompile given its config key.
// Resolution order:
//  1. AddressBook[configKey] if present (per-chain override)
//  2. Module's compiled-in Address (default)
//
// This allows historical chains (e.g., C-Chain with warp at 0x0200...0005) to use
// legacy addresses while new chains use LP-aligned addresses (0x16201).
func (c *ChainConfig) GetPrecompileAddress(configKey string) common.Address {
	// Check addressBook first for per-chain override
	if c.AddressBook != nil {
		if addr, ok := c.AddressBook[configKey]; ok {
			return addr
		}
	}
	// Fall back to module's compiled-in default address
	if module, ok := modules.GetPrecompileModule(configKey); ok {
		return module.Address
	}
	// Return zero address if not found (should not happen for registered precompiles)
	return common.Address{}
}

func (c *ChainConfig) CheckConfigCompatible(newConfig *ethparams.ChainConfig, headNumber *big.Int, headTimestamp uint64) *ethparams.ConfigCompatError {
	if c == nil {
		return nil
	}
	// Note: Cannot type assert concrete type *ethparams.ChainConfig to *ChainConfig
	// For now, we skip the extra compatibility checks and return nil
	// TODO: Implement proper compatibility checking between ethparams.ChainConfig and this ChainConfig
	return nil
}

func (c *ChainConfig) checkConfigCompatible(newcfg *ChainConfig, headNumber *big.Int, headTimestamp uint64) *ethparams.ConfigCompatError {
	if err := c.checkNetworkUpgradesCompatible(&newcfg.NetworkUpgrades, headTimestamp); err != nil {
		return err
	}
	// Check that the precompiles on the new config are compatible with the existing precompile config.
	if err := c.checkPrecompilesCompatible(newcfg.PrecompileUpgrades, headTimestamp); err != nil {
		return err
	}

	// Check that the state upgrades on the new config are compatible with the existing state upgrade config.
	if err := c.checkStateUpgradesCompatible(newcfg.StateUpgrades, headTimestamp); err != nil {
		return err
	}

	return nil
}

func (c *ChainConfig) Description() string {
	if c == nil {
		return ""
	}
	var banner string

	banner += "Lux Upgrades (timestamp based):\n"
	banner += c.NetworkUpgrades.Description()
	banner += "\n"

	upgradeConfigBytes, err := json.Marshal(c.UpgradeConfig)
	if err != nil {
		upgradeConfigBytes = []byte("cannot marshal UpgradeConfig")
	}
	banner += fmt.Sprintf("Upgrade Config: %s", string(upgradeConfigBytes))
	banner += "\n"

	feeBytes, err := json.Marshal(c.FeeConfig)
	if err != nil {
		feeBytes = []byte("cannot marshal FeeConfig")
	}
	banner += fmt.Sprintf("Fee Config: %s\n", string(feeBytes))

	banner += fmt.Sprintf("Allow Fee Recipients: %v\n", c.AllowFeeRecipients)

	return banner
}

// isForkTimestampIncompatible returns true if a fork scheduled at timestamp s1
// cannot be rescheduled to timestamp s2 because head is already past the fork.
func isForkTimestampIncompatible(s1, s2 *uint64, head uint64) bool {
	return (isTimestampForked(s1, head) || isTimestampForked(s2, head)) && !configTimestampEqual(s1, s2)
}

// isTimestampForked returns whether a fork scheduled at timestamp s is active
// at the given head timestamp.
func isTimestampForked(s *uint64, head uint64) bool {
	if s == nil {
		return false
	}
	return *s <= head
}

func configTimestampEqual(x, y *uint64) bool {
	if x == nil {
		return y == nil
	}
	if y == nil {
		return x == nil
	}
	return *x == *y
}

// UnmarshalJSON parses the JSON-encoded data and stores the result in the
// object pointed to by c.
// This is a custom unmarshaler to handle the GenesisPrecompiles field.
//
// The genesisPrecompiles field is now stored under an explicit "genesisPrecompiles" key
// to ensure deterministic serialization. For backwards compatibility, we also check
// for inline precompile configs at the root level.
//
// IMPORTANT: Explicit is authoritative. If "genesisPrecompiles" is missing or empty,
// it means NO precompiles are enabled at genesis - we do NOT fall back to defaults.
func (c *ChainConfig) UnmarshalJSON(data []byte) error {
	// Alias ChainConfigExtra to avoid recursion
	type _ChainConfigExtra ChainConfig
	tmp := _ChainConfigExtra{}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	// At this point we have populated all fields except GenesisPrecompiles
	*c = ChainConfig(tmp)

	// Try to unmarshal from explicit "genesisPrecompiles" key first (new format)
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if gpData, ok := raw["genesisPrecompiles"]; ok {
		// New format: explicit genesisPrecompiles key
		if err := json.Unmarshal(gpData, &c.GenesisPrecompiles); err != nil {
			return err
		}
	} else {
		// Backwards compatibility: try inlined precompile configs at root level
		if err := json.Unmarshal(data, &c.GenesisPrecompiles); err != nil {
			return err
		}
	}

	// Ensure GenesisPrecompiles is never nil (explicit is authoritative - empty means none)
	if c.GenesisPrecompiles == nil {
		c.GenesisPrecompiles = Precompiles{}
	}

	return nil
}

// MarshalJSON returns the JSON encoding of c.
// This is a custom marshaler to handle the GenesisPrecompiles field.
//
// GenesisPrecompiles is serialized under an explicit "genesisPrecompiles" key
// with deterministic key ordering (sorted alphabetically) to ensure consistent
// hash computation across different Go versions and map iteration orders.
func (c *ChainConfig) MarshalJSON() ([]byte, error) {
	// Alias ChainConfigExtra to avoid recursion
	type _ChainConfigExtra ChainConfig
	tmp, err := json.Marshal(_ChainConfigExtra(*c))
	if err != nil {
		return nil, err
	}

	// Unmarshal to raw map to add genesisPrecompiles
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(tmp, &raw); err != nil {
		return nil, err
	}

	// Marshal GenesisPrecompiles with deterministic key order
	if len(c.GenesisPrecompiles) > 0 {
		gpBytes, err := c.GenesisPrecompiles.MarshalJSONDeterministic()
		if err != nil {
			return nil, err
		}
		raw["genesisPrecompiles"] = gpBytes
	}

	return json.Marshal(raw)
}

// SetAllGenesisPrecompiles populates GenesisPrecompiles with default configs
// for all registered precompile modules (timestamp = 0). This ensures deterministic
// genesis hash when "all precompiles active at genesis" is the intended state.
// Call this when building genesis configs for new chains.
func (c *ChainConfig) SetAllGenesisPrecompiles() {
	c.GenesisPrecompiles = make(Precompiles)
	for _, module := range modules.RegisteredModules() {
		c.GenesisPrecompiles[module.ConfigKey] = module.Configurator.MakeGenesisConfig()
	}
}

// AllGenesisPrecompiles returns a new Precompiles map populated with default
// genesis configs for all registered precompile modules (timestamp = 0).
// This is the authoritative source for "all precompiles active at genesis".
func AllGenesisPrecompiles() Precompiles {
	precompiles := make(Precompiles)
	for _, module := range modules.RegisteredModules() {
		precompiles[module.ConfigKey] = module.Configurator.MakeGenesisConfig().(precompileconfig.Config)
	}
	return precompiles
}

type fork struct {
	name      string
	block     *big.Int // some go-ethereum forks use block numbers
	timestamp *uint64  // Lux forks use timestamps
	optional  bool     // if true, the fork may be nil and next fork is still allowed
}

func (c *ChainConfig) CheckConfigForkOrder() error {
	if c == nil {
		return nil
	}
	// Note: In Lux, upgrades must take place via block timestamps instead
	// of block numbers since blocks are produced asynchronously. Therefore, we do
	// not check block timestamp forks in the same way as block number forks since
	// it would not be a meaningful comparison. Instead, we only check that the
	// Lux upgrades are enabled in order.
	// Note: we do not add the precompile configs here because they are optional
	// and independent, i.e. the order in which they are enabled does not impact
	// the correctness of the chain config.
	return checkForks(c.forkOrder(), false)
}

// checkForks checks that forks are enabled in order and returns an error if not.
// `blockFork` is true if the fork is a block number fork, false if it is a timestamp fork
func checkForks(forks []fork, blockFork bool) error {
	lastFork := fork{}
	for _, cur := range forks {
		if lastFork.name != "" {
			switch {
			// Non-optional forks must all be present in the chain config up to the last defined fork
			case lastFork.block == nil && lastFork.timestamp == nil && (cur.block != nil || cur.timestamp != nil):
				if cur.block != nil {
					return fmt.Errorf("unsupported fork ordering: %v not enabled, but %v enabled at block %v",
						lastFork.name, cur.name, cur.block)
				} else {
					return fmt.Errorf("unsupported fork ordering: %v not enabled, but %v enabled at timestamp %v",
						lastFork.name, cur.name, cur.timestamp)
				}

			// Fork (whether defined by block or timestamp) must follow the fork definition sequence
			case (lastFork.block != nil && cur.block != nil) || (lastFork.timestamp != nil && cur.timestamp != nil):
				if lastFork.block != nil && lastFork.block.Cmp(cur.block) > 0 {
					return fmt.Errorf("unsupported fork ordering: %v enabled at block %v, but %v enabled at block %v",
						lastFork.name, lastFork.block, cur.name, cur.block)
				} else if lastFork.timestamp != nil && *lastFork.timestamp > *cur.timestamp {
					return fmt.Errorf("unsupported fork ordering: %v enabled at timestamp %v, but %v enabled at timestamp %v",
						lastFork.name, lastFork.timestamp, cur.name, cur.timestamp)
				}

				// Timestamp based forks can follow block based ones, but not the other way around
				if lastFork.timestamp != nil && cur.block != nil {
					return fmt.Errorf("unsupported fork ordering: %v used timestamp ordering, but %v reverted to block ordering",
						lastFork.name, cur.name)
				}
			}
		}
		// If it was optional and not set, then ignore it
		if !cur.optional || (cur.block != nil || cur.timestamp != nil) {
			lastFork = cur
		}
	}
	return nil
}

// Verify verifies chain config.
func (c *ChainConfig) Verify() error {
	if err := c.FeeConfig.Verify(); err != nil {
		return fmt.Errorf("invalid fee config: %w", err)
	}

	// Verify the precompile upgrades are internally consistent given the existing chainConfig.
	if err := c.verifyPrecompileUpgrades(); err != nil {
		return fmt.Errorf("invalid precompile upgrades: %w", err)
	}
	// Verify the state upgrades are internally consistent given the existing chainConfig.
	if err := c.verifyStateUpgrades(); err != nil {
		return fmt.Errorf("invalid state upgrades: %w", err)
	}

	// Verify the network upgrades are internally consistent given the existing chainConfig.
	// Use default config for validation
	agoUpgrades := upgrade.Config{}
	if err := c.verifyNetworkUpgrades(agoUpgrades); err != nil {
		return fmt.Errorf("invalid network upgrades: %w", err)
	}

	return nil
}

// IsPrecompileEnabled returns whether precompile with `address` is enabled at `timestamp`.
func (c *ChainConfig) IsPrecompileEnabled(address common.Address, timestamp uint64) bool {
	config := c.GetActivePrecompileConfig(address, timestamp)
	return config != nil && !config.IsDisabled()
}

// GetFeeConfig returns the original FeeConfig contained in the genesis ChainConfig.
// Implements precompile.ChainConfig interface.
func (c *ChainConfig) GetFeeConfig() commontype.FeeConfig {
	if c.FeeConfig.GasLimit == nil {
		fmt.Printf("DEBUG GetFeeConfig: GasLimit is nil, c=%p\n", c)
	} else {
		fmt.Printf("DEBUG GetFeeConfig: GasLimit=%v, c=%p\n", c.FeeConfig.GasLimit, c)
	}
	return c.FeeConfig
}

// AllowedFeeRecipients returns the original AllowedFeeRecipients parameter contained in the genesis ChainConfig.
// Implements precompile.ChainConfig interface.
func (c *ChainConfig) AllowedFeeRecipients() bool {
	return c.AllowFeeRecipients
}

// IsForkTransition returns true if `fork` activates during the transition from
// `parent` to `current`.
// Taking `parent` as a pointer allows for us to pass nil when checking forks
// that activate during genesis.
// Note: `parent` and `current` can be either both timestamp values, or both
// block number values, since this function works for both block number and
// timestamp activated forks.
func IsForkTransition(fork *uint64, parent *uint64, current uint64) bool {
	var parentForked bool
	if parent != nil {
		parentForked = isTimestampForked(fork, *parent)
	}
	currentForked := isTimestampForked(fork, current)
	return !parentForked && currentForked
}

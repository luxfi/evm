// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package extras

import (
	"fmt"

	"github.com/luxfi/evm/precompile/modules"
)

// RewardManagerConfigKey names the precompile that owns the fee destination.
// Referenced by key rather than by import so params keeps no dependency on any
// individual precompile package.
const RewardManagerConfigKey = "rewardManagerConfig"

// verifyFeeSplit refuses a configuration that burns half of every fee while the
// other half has nowhere governed to land.
//
// FeeSplit decides HOW MUCH of a fee is kept. WHERE the kept part goes is always
// the configured coinbase, which BlockChain.GetCoinbaseAt resolves from the
// RewardManager precompile's state — and, when that precompile is not enabled,
// falls back to the keyless blackhole 0x0100..00. Scheduling the split without
// RewardManager therefore burns half of every fee (irreversible, by design) and
// strands the other half at an address nobody holds a key to (irreversible, by
// accident). The C-Chain has ~3867 LUX stuck at that blackhole today, which is
// exactly what this check exists to stop repeating at twice the rate.
//
// This is a configuration invariant, not a consensus rule: it runs at genesis
// setup and VM init (ChainConfig.Verify), so a node refuses to start on such a
// config rather than producing blocks that quietly destroy value. It can never
// fork a chain.
//
// Two ways to satisfy it, both governed:
//   - enable rewardManagerConfig at or before the split, with the DAO as
//     adminAddresses and a real rewardAddress; or
//   - set allowFeeRecipients, the explicit "block builders keep the fees" policy.
func (c *ChainConfig) verifyFeeSplit() error {
	if c.FeeSplitTimestamp == nil {
		return nil // dormant: nothing to check
	}
	if c.AllowFeeRecipients {
		return nil // destination is the block builder, by explicit policy
	}
	module, ok := modules.GetPrecompileModule(RewardManagerConfigKey)
	if !ok {
		return fmt.Errorf(
			"feeSplitTimestamp is set to %d but %s is not registered in this build, so the reward half has no governed destination",
			*c.FeeSplitTimestamp, RewardManagerConfigKey)
	}
	if !c.IsPrecompileEnabled(module.Address, *c.FeeSplitTimestamp) {
		return fmt.Errorf(
			"feeSplitTimestamp is set to %d but %s (%s) is not enabled at that time: half of every fee would be burned and the other half stranded at the keyless blackhole. Enable %s at or before %d, or set allowFeeRecipients",
			*c.FeeSplitTimestamp, RewardManagerConfigKey, module.Address,
			RewardManagerConfigKey, *c.FeeSplitTimestamp)
	}
	return nil
}

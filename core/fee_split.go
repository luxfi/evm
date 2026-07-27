// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/holiman/uint256"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// creditTxFee disburses the fee paid by a single transaction.
//
// Two concerns, deliberately kept apart:
//
//   - HOW MUCH is kept — a protocol constant. FeeSplit inactive: all of it.
//     FeeSplit active: floor(fee/2); the remainder is credited to no account,
//     which reduces the sum of all balances (a true burn, identical in mechanism
//     to the EIP-1559 base-fee burn — not a transfer to a dead address, which
//     would leave supply unchanged). The ratio is a single right shift: fixed,
//     auditable, and not settable by any transaction. Changing it is a node
//     release plus a coordinated fork, by design — monetary policy should not
//     move at the speed of a multisig.
//
//   - WHERE it goes — chain state, never a compiled-in constant. [coinbase] is
//     the fee sink configured at the parent block, resolved by
//     BlockChain.GetCoinbaseAt: the RewardManager precompile's stored reward
//     address when that precompile is enabled, otherwise the blackhole. It is
//     already part of header validity (dummy.verifyCoinbase rejects a block whose
//     header coinbase disagrees with the configured one), so the destination is
//     verifiable from the header alone and is changeable on-chain, by transaction,
//     through the RewardManager admin — i.e. governable — without touching this
//     code or shipping a binary.
//
// Braiding the two — crediting the kept half to an address baked into the binary
// — would make the destination un-governable and would silently bypass
// RewardManager the moment FeeSplit activated. It does not, and must not.
//
// Determinism: fee is a consensus value (gasUsed * effectiveGasPrice); the split
// is pure uint256 integer arithmetic to an address already agreed by consensus,
// so every validator computes an identical post-state and the split cannot fork
// the chain. The odd wei on an odd fee is assigned to the burn (conservative: it
// never over-credits the reward side, and it keeps burn >= reward exactly).
func creditTxFee(stateDB vm.StateDB, config *ethparams.ChainConfig, coinbase common.Address, blockTime uint64, fee *uint256.Int) {
	reward := fee
	if cfg := params.GetExtra(config); cfg != nil && cfg.IsFeeSplit(blockTime) {
		reward = new(uint256.Int).Rsh(fee, 1) // floor(fee/2); fee-reward is the burn
	}
	stateDB.AddBalance(coinbase, reward, tracing.BalanceIncreaseRewardTransactionFee)
}

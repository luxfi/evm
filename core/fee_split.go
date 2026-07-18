// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import (
	"github.com/holiman/uint256"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
)

// creditTxFee disburses the fee paid by a single transaction.
//
// Legacy behavior (FeeSplit inactive) — the full fee is credited to the coinbase
// (the blackhole 0x0100..00 on the C-Chain), matching historical Subnet-EVM.
//
// FeeSplit active — the fee is divided 50/50, deterministically:
//
//   - rewardShare = fee / 2 (floor)          -> credited to extras.FeeRewardVault.
//     This half stays in circulation, parked in a protocol-owned account, to be
//     atomically exported to the P-Chain staking-reward pool where it is paid out
//     through the native staking-reward path (see the "Tx-fee 50/50 split" design
//     in LLM.md). It is NOT minted; it is moved, so it can never double-mint.
//
//   - burnShare   = fee - rewardShare (ceil)  -> credited to NO account.
//     Never re-crediting it removes it from the account trie, i.e. the sum of all
//     balances (the supply) decreases by exactly burnShare every transaction. This
//     is a TRUE burn — identical in mechanism to the EIP-1559 base-fee burn — not a
//     transfer to a dead address (which would leave supply unchanged).
//
// Determinism: fee is a consensus value (gasUsed * effectiveGasPrice); the split
// is pure uint256 integer arithmetic (a single right shift) to a fixed address, so
// every validator computes an identical post-state and the split cannot fork the
// chain. The odd wei on an odd fee is assigned to the burn (conservative: it never
// over-credits the reward side, and it keeps burn >= reward exactly).
func creditTxFee(stateDB vm.StateDB, config *ethparams.ChainConfig, coinbase common.Address, blockTime uint64, fee *uint256.Int) {
	if cfg := params.GetExtra(config); cfg != nil && cfg.IsFeeSplit(blockTime) {
		rewardShare := new(uint256.Int).Rsh(fee, 1) // floor(fee/2)
		stateDB.AddBalance(extras.FeeRewardVault, rewardShare, tracing.BalanceIncreaseRewardTransactionFee)
		// burnShare = fee - rewardShare is intentionally left uncredited (true burn).
		return
	}
	stateDB.AddBalance(coinbase, fee, tracing.BalanceIncreaseRewardTransactionFee)
}

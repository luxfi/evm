// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"fmt"
	"math/big"

	"github.com/luxfi/geth/common"

	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/plugin/evm/customtypes"
	"github.com/luxfi/evm/plugin/evm/header"
	"github.com/luxfi/evm/plugin/evm/upgrade/legacy"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/trie"
)

var legacyMinGasPrice = big.NewInt(legacy.BaseFee)

type BlockValidator interface {
	SyntacticVerify(b *Block, rules params.Rules) error
}

type blockValidator struct{}

func NewBlockValidator() BlockValidator {
	return &blockValidator{}
}

func (v blockValidator) SyntacticVerify(b *Block, rules params.Rules) error {
	rulesExtra := params.GetRulesExtra(rules)
	if b == nil || b.ethBlock == nil {
		return errInvalidBlock
	}
	ethHeader := b.ethBlock.Header()
	blockHash := b.ethBlock.Hash()

	// Skip verification of the genesis block since it should already be marked as accepted.
	if blockHash == b.vm.genesisHash {
		return nil
	}

	// Perform block and header sanity checks
	if ethHeader.Number == nil || !ethHeader.Number.IsUint64() {
		return errInvalidBlock
	}
	if ethHeader.Difficulty == nil || !ethHeader.Difficulty.IsUint64() ||
		ethHeader.Difficulty.Uint64() != 1 {
		return fmt.Errorf("invalid difficulty: %d", ethHeader.Difficulty)
	}
	if ethHeader.Nonce.Uint64() != 0 {
		return fmt.Errorf(
			"expected nonce to be 0 but got %d: %w",
			ethHeader.Nonce.Uint64(), errInvalidNonce,
		)
	}

	if ethHeader.MixDigest != (common.Hash{}) {
		return fmt.Errorf("invalid mix digest: %v", ethHeader.MixDigest)
	}

	// Verify the extra data is well-formed.
	if err := header.VerifyExtra(rulesExtra.LuxRules, ethHeader.Extra); err != nil {
		return err
	}

	if rulesExtra.IsEVM {
		if ethHeader.BaseFee == nil {
			return errNilBaseFeeEVM
		}
		if bfLen := ethHeader.BaseFee.BitLen(); bfLen > 256 {
			return fmt.Errorf("too large base fee: bitlen %d", bfLen)
		}
	}

	// Check that the tx hash in the header matches the body
	txsHash := types.DeriveSha(b.ethBlock.Transactions(), trie.NewStackTrie(nil))
	if txsHash != ethHeader.TxHash {
		return fmt.Errorf("invalid txs hash %v does not match calculated txs hash %v", ethHeader.TxHash, txsHash)
	}
	// Check that the uncle hash in the header matches the body
	uncleHash := types.CalcUncleHash(b.ethBlock.Uncles())
	if uncleHash != ethHeader.UncleHash {
		return fmt.Errorf("invalid uncle hash %v does not match calculated uncle hash %v", ethHeader.UncleHash, uncleHash)
	}

	// Block must not have any uncles
	if len(b.ethBlock.Uncles()) > 0 {
		return errUnclesUnsupported
	}

	// EMPTY-BLOCK POLICY IS BUILD-ONLY — deliberately NOT enforced here. SyntacticVerify runs at
	// PARSE, VERIFY, and ACCEPT; rejecting an empty block here made a consensus-FINALIZED empty
	// block un-acceptable, which (with the fail-closed finalize→VM.Accept fix) HALTS the chain,
	// and (before it) let the decided-floor run ahead of the EVM — the phantom-floor freeze.
	// Finality MUST override the anti-spam empty rule: an α-of-K-certified block is applied
	// regardless of emptiness (the cert is still required — this does NOT weaken any other
	// structural check). The demand-driven "don't PRODUCE empty blocks" rule now lives solely on
	// the build path (buildBlockWithContext, vm.go), so a proposer still never emits an empty
	// block while a finalized one still Accepts.
	txs := b.ethBlock.Transactions()

	if !rulesExtra.IsEVM {
		// Make sure that all the txs have the correct fee set.
		for _, tx := range txs {
			if tx.GasPrice().Cmp(legacyMinGasPrice) < 0 {
				return fmt.Errorf("block contains tx %s with gas price too low (%d < %d)", tx.Hash(), tx.GasPrice(), legacyMinGasPrice)
			}
		}
	}

	// Make sure the block isn't too far in the future
	blockTimestamp := b.ethBlock.Time()
	if maxBlockTime := uint64(b.vm.clock.Time().Add(maxFutureBlockTime).Unix()); blockTimestamp > maxBlockTime {
		return fmt.Errorf("block timestamp is too far in the future: %d > allowed %d", blockTimestamp, maxBlockTime)
	}

	// BlockGasCost is NOT verified here. It is NOT part of the RLP block — it is DERIVED from the
	// parent header, so requiring it at parse time made ParseBlock STATEFUL (a bootstrap descent
	// parses ancestry blocks ahead of the accepted height, whose parents are not yet present, and
	// they all failed here with errNilBlockGasCostEVM — masked by the ZAP errorToZAP mapping as the
	// misleading "malformed block id"). SyntacticVerify is decomplected to STATELESS block-intrinsic
	// checks only. BlockGasCost is populated (block.ensureBlockGasCost, with the parent present) and
	// ENFORCED at Verify/Accept — the consensus/dummy header verification (InsertBlockManual) computes
	// the expected BlockGasCost from the parent and rejects a mismatch — so no non-connecting or
	// gas-cost-invalid block can be ACCEPTED just because it now PARSES.
	if rulesExtra.IsEVM {
		if blockGasCost := customtypes.GetHeaderExtra(ethHeader).BlockGasCost; blockGasCost != nil && !blockGasCost.IsUint64() {
			return fmt.Errorf("too large blockGasCost: %d", blockGasCost)
		}
	}

	// Lux does NOT use Ethereum beacon chain or blob transactions
	// Skip Cancun-specific field requirements for pre-Cancun blocks
	// Only reject if blob gas was actually used (which shouldn't happen on Lux)
	if ethHeader.BlobGasUsed != nil && *ethHeader.BlobGasUsed > 0 {
		return fmt.Errorf("blobs not enabled on lux networks: used %d blob gas", *ethHeader.BlobGasUsed)
	}
	return nil
}

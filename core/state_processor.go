// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// This file is a derived work, based on the go-ethereum library whose original
// notices appear below.
//
// It is distributed under a license compatible with the licensing terms of the
// original code from which it is derived.
//
// Much love to the original authors for their work.
// **********
// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"fmt"
	"math/big"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus"
	"github.com/luxfi/evm/core/parallel"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/params/extras"
	"github.com/luxfi/evm/predicate"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	ethparams "github.com/luxfi/geth/params"
	log "github.com/luxfi/log"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, parent *types.Header, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts    types.Receipts
		usedGas     = new(uint64)
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		gp          = new(GasPool).AddGas(block.GasLimit())
	)

	// Configure any upgrades that should go into effect during this block.
	blockContext := NewBlockContext(block.Number(), block.Time())
	err := ApplyUpgrades(p.config, &parent.Time, blockContext, statedb)
	if err != nil {
		log.Error("failed to configure precompiles processing block", "hash", block.Hash(), "number", block.NumberU64(), "timestamp", block.Time(), "err", err)
		return nil, nil, 0, err
	}

	var (
		context = NewEVMBlockContext(header, p.bc, nil)
		signer  = types.MakeSigner(p.config, header.Number, header.Time)
	)
	// Get rules for predicate storage slot computation
	rules := p.config.Rules(header.Number, params.IsMergeTODO, header.Time)
	rulesExtra := params.GetRulesExtra(rules)

	// Parse predicate results from block header extra data
	var predicateResults *predicate.Results
	if rulesExtra.PredicatersExist {
		var parseErr error
		predicateResults, parseErr = predicate.ParseResultsFromHeaderExtra(header.Extra)
		if parseErr != nil {
			log.Debug("failed to parse predicate results from header", "hash", block.Hash(), "err", parseErr)
		}
	}

	// Predicate results are validated separately through the predicate verification
	// path. The consensus identity (networkID, cChainID) reaches stateful precompiles
	// via context.ConsensusContext, set by NewEVMBlockContext(header, p.bc, …) above
	// from the running *BlockChain and preserved onto evm.Context by vm.NewEVM.
	_ = predicateResults
	vmenv := vm.NewEVM(context, statedb, p.config, cfg)
	if beaconRoot := block.BeaconRoot(); beaconRoot != nil {
		ProcessBeaconBlockRoot(*beaconRoot, vmenv, statedb)
	}

	// Deterministic Block-STM parallel execution — gated OFF by default
	// (parallel.Enabled; nothing in production flips it, sequential below is the
	// live path). When enabled, every block runs through ExecuteVerified, which
	// executes the block in parallel AND recomputes the sequential root locally,
	// commits ONLY when the two are byte-identical, and otherwise fails closed to
	// the sequential loop below. The committed (state, receipts) pair is therefore
	// exactly what a sequential validator commits, so a mixed parallel/sequential
	// validator set cannot fork: a parallel root differing from sequential by one
	// byte is never committed.
	//
	// The parallel base reader reads the parent (committed) state root, so the
	// engine is eligible only when the live pre-transaction state still equals
	// parent.Root — i.e. no uncommitted pre-tx mutation (a precompile upgrade or an
	// EIP-4788 beacon-root write) is pending. Those rare blocks fall through to
	// sequential. The IntermediateRoot probe runs only under the flag, so the
	// default (flag-off) path pays nothing.
	if parallel.Enabled.Load() {
		deleteEmpty := p.config.IsEIP158(blockNumber)
		if statedb.IntermediateRoot(deleteEmpty) == parent.Root {
			txs := block.Transactions()
			exec := parallel.NewExecutor(p.bc.stateCache, parent.Root, txs, block.GasLimit(), deleteEmpty, 0,
				func(vmsdb vm.StateDB, i int) (*types.Receipt, error) {
					return p.applyParallelTx(txs[i], i, vmsdb, signer, header, blockHash, blockNumber, cfg)
				})
			// block.Root() is the post-transaction state root (DummyEngine.Finalize
			// mutates no state), so it is the parity reference ExecuteVerified pins
			// against its own local sequential recomputation. A forged header.Root is
			// caught here; a genuine parallel/sequential divergence fails closed.
			if parReceipts, ok := exec.ExecuteVerified(statedb, block.Root()); ok {
				for _, r := range parReceipts {
					*usedGas += r.GasUsed
					allLogs = append(allLogs, r.Logs...)
				}
				receipts = parReceipts
				// Finalize runs on the committed (sequential-equal) statedb, exactly as
				// in the sequential path below.
				if err := p.engine.Finalize(p.bc, block, parent, statedb, receipts); err != nil {
					return nil, nil, 0, fmt.Errorf("engine finalization check failed: %w", err)
				}
				return receipts, allLogs, *usedGas, nil
			}
			// ok=false: pre-state untouched. Fall through to the sequential loop,
			// which recomputes and (for an invalid block) surfaces the real error.
		}
	}

	// Sequential execution is the live consensus path (and the fail-closed target
	// of the gated parallel path above).
	//
	// If a modular EVM backend (revm, cevm) is registered, dispatch
	// through it instead of the default geth interpreter.
	txExec := parallel.DefaultTransactionExecutor()
	if txExec != nil {
		log.Info("EVM backend", "active", parallel.ActiveBackend(), "available", parallel.AvailableBackends())
	}
	for i, tx := range block.Transactions() {
		msg, err := TransactionToMessage(tx, signer, header.BaseFee)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		statedb.SetTxContext(tx.Hash(), i)

		// Compute predicate storage slots for this transaction
		var predicateStorageSlots map[common.Address][][]byte
		if rulesExtra.PredicatersExist {
			extrasRules := &extras.Rules{
				Predicaters: rulesExtra.Predicaters,
				LuxRules:    rulesExtra.LuxRules,
			}
			predicateStorageSlots = predicate.PreparePredicateStorageSlots(extrasRules, tx.AccessList())
		}
		// StatefulPrecompileHook is not yet exposed by luxfi/geth.
		_ = predicateStorageSlots

		// Try modular backend (revm/cevm) before default geth path.
		if txExec != nil {
			if backendReceipt, backendErr := txExec.ExecuteTransaction(
				p.config, header, tx, statedb, cfg, gp.Gas(),
			); backendReceipt != nil {
				if backendErr != nil {
					return nil, nil, 0, fmt.Errorf("backend tx %d [%v]: %w", i, tx.Hash().Hex(), backendErr)
				}
				// Deduct gas consumed by the backend from the pool.
				if err := gp.SubGas(backendReceipt.GasUsed); err != nil {
					return nil, nil, 0, fmt.Errorf("backend tx %d [%v] gas overflow: %w", i, tx.Hash().Hex(), err)
				}
				*usedGas += backendReceipt.GasUsed
				backendReceipt.CumulativeGasUsed = *usedGas
				receipts = append(receipts, backendReceipt)
				allLogs = append(allLogs, backendReceipt.Logs...)
				continue
			}
		}

		receipt, err := applyTransaction(msg, p.config, gp, statedb, blockNumber, blockHash, header.Time, tx, usedGas, vmenv)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("could not apply tx %d [%v]: %w", i, tx.Hash().Hex(), err)
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	if err := p.engine.Finalize(p.bc, block, parent, statedb, receipts); err != nil {
		return nil, nil, 0, fmt.Errorf("engine finalization check failed: %w", err)
	}

	return receipts, allLogs, *usedGas, nil
}

// applyParallelTx executes one transaction against vmsdb for the Block-STM engine
// and builds its receipt, mirroring applyTransaction EXACTLY except for two
// concerns the parallel executor owns: it does NOT Finalise the state (the engine
// reproduces the per-transaction Finalise cadence) and it does NOT accumulate
// CumulativeGasUsed (the engine assigns it in transaction order). A FRESH block
// context is built per call so each speculative worker has a private BLOCKHASH
// cache (GetHashFn's cache is not concurrency-safe) — without this, concurrent
// workers would race it.
//
// Logs and bloom are populated only when vmsdb is the canonical sequential
// *state.StateDB: that is the only StateDB whose per-block log index counter is
// block-correct. The parallel cross-check runs each transaction on its own
// speculative hooked StateDB and discards these receipts (it is consensus-compared
// by state ROOT only), so a degraded receipt there is immaterial — ExecuteVerified
// returns and commits only the sequential reference's receipts.
func (p *StateProcessor) applyParallelTx(tx *types.Transaction, i int, vmsdb vm.StateDB, signer types.Signer, header *types.Header, blockHash common.Hash, blockNumber *big.Int, cfg vm.Config) (*types.Receipt, error) {
	msg, err := TransactionToMessage(tx, signer, header.BaseFee)
	if err != nil {
		return nil, err
	}
	// Per-call block context ⇒ private GetHashFn cache ⇒ race-free across workers.
	blockContext := NewEVMBlockContext(header, p.bc, nil)
	vmenv := vm.NewEVM(blockContext, vmsdb, p.config, cfg)
	vmenv.SetTxContext(NewEVMTxContext(msg))
	// A fresh, permissive per-tx gas pool: the speculative parallel path cannot
	// share a mutable block pool across re-executing workers. The block gas LIMIT
	// is enforced as a faithful reservation in the sequential reference inside
	// ExecuteVerified (executor.executeSequential), so an over-limit block is
	// rejected there, not here.
	gp := new(GasPool).AddGas(header.GasLimit)
	result, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, err
	}
	receipt := &types.Receipt{Type: tx.Type(), GasUsed: result.UsedGas}
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	if tx.Type() == types.BlobTxType {
		receipt.BlobGasUsed = uint64(len(tx.BlobHashes()) * ethparams.BlobTxBlobGasPerBlob)
		receipt.BlobGasPrice = vmenv.Context.BlobBaseFee
	}
	if msg.To == nil {
		var cryptoAddr crypto.Address
		copy(cryptoAddr[:], vmenv.Origin[:])
		createdAddr := crypto.CreateAddress(cryptoAddr, tx.Nonce())
		receipt.ContractAddress = common.BytesToAddress(createdAddr[:])
	}
	if cs, ok := vmsdb.(*state.StateDB); ok {
		receipt.Logs = cs.GetLogs(tx.Hash(), blockNumber.Uint64(), blockHash, header.Time)
		receipt.Bloom = types.CreateBloom(receipt)
		receipt.BlockHash = blockHash
		receipt.BlockNumber = blockNumber
		receipt.TransactionIndex = uint(i)
	}
	return receipt, nil
}

func applyTransaction(msg *Message, config *params.ChainConfig, gp *GasPool, statedb *state.StateDB, blockNumber *big.Int, blockHash common.Hash, blockTime uint64, tx *types.Transaction, usedGas *uint64, evm *vm.EVM) (*types.Receipt, error) {
	// Create a new context to be used in the EVM environment.
	txContext := NewEVMTxContext(msg)
	evm.SetTxContext(txContext)

	// Apply the transaction to the current state (included in the env).
	result, err := ApplyMessage(evm, msg, gp)
	if err != nil {
		return nil, err
	}

	// Update the state with pending changes.
	var root []byte
	if config.IsByzantium(blockNumber) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(blockNumber)).Bytes()
	}
	*usedGas += result.UsedGas

	// Create a new receipt for the transaction, storing the intermediate root and gas used
	// by the tx.
	receipt := &types.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: *usedGas}
	if result.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = result.UsedGas

	if tx.Type() == types.BlobTxType {
		receipt.BlobGasUsed = uint64(len(tx.BlobHashes()) * ethparams.BlobTxBlobGasPerBlob)
		receipt.BlobGasPrice = evm.Context.BlobBaseFee
	}

	// If the transaction created a contract, store the creation address in the receipt.
	if msg.To == nil {
		// Convert common.Address to crypto.Address
		var cryptoAddr crypto.Address
		copy(cryptoAddr[:], evm.Origin[:])
		createdAddr := crypto.CreateAddress(cryptoAddr, tx.Nonce())
		// Convert crypto.Address back to common.Address
		receipt.ContractAddress = common.BytesToAddress(createdAddr[:])
	}

	// Set the receipt logs and create the bloom filter.
	receipt.Logs = statedb.GetLogs(tx.Hash(), blockNumber.Uint64(), blockHash, blockTime)
	receipt.Bloom = types.CreateBloom(receipt)
	receipt.BlockHash = blockHash
	receipt.BlockNumber = blockNumber
	receipt.TransactionIndex = uint(statedb.TxIndex())
	return receipt, err
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, blockContext vm.BlockContext, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, error) {
	return ApplyTransactionWithResults(config, bc, blockContext, gp, statedb, header, tx, usedGas, cfg, nil)
}

// ApplyTransactionWithResults is like ApplyTransaction but accepts pre-computed predicate results.
// This is used by the miner when it has already verified predicates.
func ApplyTransactionWithResults(config *params.ChainConfig, bc ChainContext, blockContext vm.BlockContext, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config, predicateResults *predicate.Results) (*types.Receipt, error) {
	msg, err := TransactionToMessage(tx, types.MakeSigner(config, header.Number, header.Time), header.BaseFee)
	if err != nil {
		return nil, err
	}
	// Create a new context to be used in the EVM environment
	txContext := NewEVMTxContext(msg)

	// Compute predicate storage slots from the transaction's access list
	rules := config.Rules(header.Number, params.IsMergeTODO, header.Time)
	rulesExtra := params.GetRulesExtra(rules)
	var predicateStorageSlots map[common.Address][][]byte
	if rulesExtra.PredicatersExist {
		extrasRules := &extras.Rules{
			Predicaters: rulesExtra.Predicaters,
			LuxRules:    rulesExtra.LuxRules,
		}
		predicateStorageSlots = predicate.PreparePredicateStorageSlots(extrasRules, tx.AccessList())
	}

	// Thread the chain's consensus context (which embeds the chain Runtime via
	// runtime.WithContext at initializeChain) into the EVM block context so a
	// stateful precompile can recover the real (networkID, cChainID) through
	// runtime.FromContext(env.ConsensusContext()). geth's vm.BlockContext exposes
	// ConsensusContext and vm.NewEVM preserves it onto evm.Context, so this is the
	// supported seam — the prior "StatefulPrecompileHook not exposed" note was stale.
	//
	// The miner already builds blockContext via NewEVMBlockContext(env.header,
	// w.chain, …), which sets ConsensusContext from the running *BlockChain; the
	// nil-guard makes that path a no-op while guaranteeing any caller that passes a
	// bare blockContext (so the consensus identity would otherwise be absent, i.e.
	// networkID 0 / C-Chain Empty) still sees the real identity. bc can be nil in
	// test contexts (e.g., AddTxWithVMConfig), in which case there is nothing to thread.
	if bc != nil && blockContext.ConsensusContext == nil {
		blockContext.ConsensusContext = bc.ConsensusContext()
	}
	_ = predicateStorageSlots
	vmenv := vm.NewEVM(blockContext, statedb, config, cfg)
	vmenv.SetTxContext(txContext)
	return applyTransaction(msg, config, gp, statedb, header.Number, header.Hash(), header.Time, tx, usedGas, vmenv)
}

// ProcessBeaconBlockRoot applies the EIP-4788 system call to the beacon block root
// contract. This method is exported to be used in tests.
func ProcessBeaconBlockRoot(beaconRoot common.Hash, vmenv *vm.EVM, statedb *state.StateDB) {
	// If EIP-4788 is enabled, we need to invoke the beaconroot storage contract with
	// the new root
	msg := &Message{
		From:      ethparams.SystemAddress,
		GasLimit:  30_000_000,
		GasPrice:  common.Big0,
		GasFeeCap: common.Big0,
		GasTipCap: common.Big0,
		To:        &ethparams.BeaconRootsAddress,
		Data:      beaconRoot[:],
	}
	vmenv.SetTxContext(NewEVMTxContext(msg))
	statedb.AddAddressToAccessList(ethparams.BeaconRootsAddress)
	_, _, _ = vmenv.Call(msg.From, *msg.To, msg.Data, 30_000_000, common.U2560)
	statedb.Finalise(true)
}

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cevm

// Diverse applier-conformance harness for the cevm gated APPLIER.
//
// cevm is wired into luxfi/evm as a consensus-SAFE, apply-then-verify GATED
// APPLIER (cevm_shadow.go). Today it APPLIES only the proven "safe subset"
// (pure value transfers + non-reverting calls into existing code) and DECLINES
// everything else back to the Go EVM. This test MEASURES exactly which
// per-tx-type scenarios cevm applies vs declines — the punch-list for widening
// cevm toward full parity.
//
// Method: for each scenario it builds a SEPARATE small chain (preimages ON so
// the shadow can seed its keccak-MPT from the pre-block trie), generates ONE
// block that exercises exactly ONE tx-type (contracts for the CALL scenarios are
// PRE-DEPLOYED in genesis so the measured block contains only the call), resets
// the package-level cevm shadow counters, runs the block through
// chain.InsertChain (which routes every block through
// parallel.DefaultExecutor().ExecuteBlock = the cevm applier), and reads the
// resulting ShadowStats delta. Because each chain inserts exactly one non-empty
// block, the post-insert counters ARE that scenario's verdict.
//
// Two independent facts are recorded per scenario:
//   - InsertChain result: the Go EVM ALWAYS applies the block correctly whether
//     or not cevm applies it (cevm returns (nil,nil) on any decline and the Go
//     EVM runs). So InsertChain must succeed for every scenario; a failure here
//     is a REAL bug and is flagged LOUD via t.Errorf.
//   - cevm verdict: APPLIED (four-way gate passed) or DECLINED with the single
//     counter that incremented (disagree / reverted-mismatch / errored / a
//     declined-* reason).
//
// Built only under -tags cevm (needs the linked C++ EVM). Lives in the external
// test package so it may import core without an import cycle. This file is a
// MEASUREMENT harness only — it does NOT modify cevm_shadow.go or the bridge.
package parallel_test

import (
	"math/big"
	"testing"

	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/core"
	"github.com/luxfi/evm/core/parallel"
	"github.com/luxfi/evm/params"
	"github.com/luxfi/evm/plugin/evm/upgrade/legacy"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/stretchr/testify/require"
)

// Minimal runtime bytecodes pre-deployed in genesis for the CALL scenarios.
// Each is a self-contained runtime (no constructor) so it can be dropped
// straight into a genesis Account.Code and invoked by a bare call.
var (
	// PUSH1 0x2a PUSH1 0x00 SSTORE STOP — stores 0x2a at slot 0 and halts.
	// A non-reverting call into existing code: the proven safe subset.
	codeSSTORE = common.FromHex("602a60005500")

	// mem[0..31]=0xff; LOG1(offset=0,size=32,topic=0x42); STOP.
	// PUSH1 0xff PUSH1 0x00 MSTORE PUSH1 0x42 PUSH1 0x20 PUSH1 0x00 LOG1 STOP.
	codeLOG1 = common.FromHex("60ff600052604260206000a100")

	// PUSH1 0x00 PUSH1 0x00 REVERT — reverts with empty returndata.
	codeREVERT = common.FromHex("60006000fd")

	// CREATE init code: CODECOPY the 6-byte runtime (codeSSTORE) at offset 0x0c
	// and RETURN it. Deploys a contract whose runtime is codeSSTORE.
	// PUSH1 6 PUSH1 0x0c PUSH1 0 CODECOPY PUSH1 6 PUSH1 0 RETURN <runtime>.
	initCREATE = common.FromHex("6006600c60003960066000f3602a60005500")
)

// scenarioResult is the recorded outcome of one tx-type scenario.
type scenarioResult struct {
	name      string
	insertOK  bool
	insertErr string
	applied   bool
	reason    string
	stats     parallel.ShadowStats
}

// classify reduces a single-block ShadowStats delta to (applied, reason). The
// executor increments exactly one terminal counter per block, so the first
// non-zero counter is the verdict.
func classify(s parallel.ShadowStats) (applied bool, reason string) {
	switch {
	case s.Applied > 0:
		return true, ""
	case s.Disagree > 0:
		return false, "DISAGREE (cevm root != Go header.Root)"
	case s.FinalizeGap > 0:
		return false, "FINALIZE_GAP (withdrawals move header.Root)"
	case s.RevertedMismatch > 0:
		return false, "REVERTED_MISMATCH (root agreed, four-way apply gate failed)"
	case s.DeclinedTooLarge > 0:
		return false, "DECLINED_TOO_LARGE (pre-state exceeds snapshot caps)"
	case s.DeclinedNoPreimage > 0:
		return false, "DECLINED_NO_PREIMAGE (account address preimage unrecoverable)"
	case s.DeclinedTx > 0:
		return false, "DECLINED_TX (tx sender unrecoverable)"
	case s.Errored > 0:
		return false, "ERRORED (cevm process_block failed/panicked)"
	default:
		return false, "NO_OP (executor returned early; block not processed)"
	}
}

// runScenario builds a fresh preimages-ON chain from gspec, generates ONE block
// via gen, runs it through InsertChain (the cevm applier path), and records the
// per-scenario ShadowStats delta and verdict.
func runScenario(t *testing.T, name string, gspec *core.Genesis, gen func(int, *core.BlockGen)) scenarioResult {
	t.Helper()
	engine := dummy.NewCoinbaseFaker()

	// Preimages ON + snapshots OFF: the shadow verifier recovers 20-byte
	// addresses from the pre-block trie-key preimages (its leaf key is
	// keccak256(addr)); without them it declines honestly (DeclinedNoPreimage).
	db := rawdb.NewMemoryDatabase()
	cacheConfig := *core.DefaultCacheConfig
	cacheConfig.Preimages = true
	cacheConfig.SnapshotLimit = 0

	chain, err := core.NewBlockChain(db, &cacheConfig, gspec, engine, vm.Config{}, common.Hash{}, false, nil)
	require.NoError(t, err, "NewBlockChain")
	defer chain.Stop()

	blocks, _, err := core.GenerateChain(gspec.Config, chain.Genesis(), engine, db, 1, 10, gen)
	require.NoError(t, err, "GenerateChain")

	// Reset counters, then insert exactly one non-empty block so the resulting
	// stats snapshot IS this scenario's verdict (delta from zero).
	parallel.ResetCevmShadowStats()
	_, insErr := chain.InsertChain(blocks)
	s := parallel.CevmShadowStats()

	res := scenarioResult{name: name, stats: s, insertOK: insErr == nil}
	if insErr != nil {
		res.insertErr = insErr.Error()
		// The Go EVM must apply every block regardless of cevm's verdict; an
		// InsertChain failure is a real consensus bug, not a cevm decline.
		t.Errorf("SCENARIO %s: InsertChain FAILED (REAL BUG — Go EVM must always apply): %v", name, insErr)
	}
	res.applied, res.reason = classify(s)

	if res.applied {
		t.Logf("SCENARIO %s: cevm APPLIED", name)
	} else {
		t.Logf("SCENARIO %s: cevm DECLINED (%s)", name, res.reason)
	}
	t.Logf("  stats: %+v", s)
	return res
}

// TestCevmApplierConformance measures cevm's real per-tx-type apply/decline
// coverage across a diverse, realistic set of scenarios and prints a summary
// table plus the honest headline "cevm applier covers X/9 diverse tx-types".
func TestCevmApplierConformance(t *testing.T) {
	if !parallel.CevmShadowEnabled() {
		t.Skip("cevm bridge not linked (build with -tags cevm)")
	}

	// One funded sender key (same fixture key the shadow e2e uses) + a fixed
	// coinbase. Every scenario starts this sender at nonce 0 on a fresh chain.
	key1, _ := crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	cryptoFrom := crypto.PubkeyToAddress(key1.PublicKey)
	from := common.BytesToAddress(cryptoFrom[:])
	coinbase := common.HexToAddress("0xCafE00000000000000000000000000000000C0DE")
	funds := new(big.Int).Mul(big.NewInt(1000), big.NewInt(params.Ether))

	// Deterministic non-sender addresses used across scenarios.
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	sstoreAddr := common.HexToAddress("0x00000000000000000000000000000000C0de5501")
	logAddr := common.HexToAddress("0x00000000000000000000000000000000c0DE1061")
	revertAddr := common.HexToAddress("0x00000000000000000000000000000000c0dE5EEd")
	identityPrecompile := common.HexToAddress("0x0000000000000000000000000000000000000004")
	warmAddr := common.HexToAddress("0x00000000000000000000000000000000AcCe5511")

	// newGenesis builds a genesis funding `from` plus any pre-deployed contracts.
	newGenesis := func(contracts map[common.Address]types.Account) *core.Genesis {
		alloc := types.GenesisAlloc{from: {Balance: funds}}
		for addr, acct := range contracts {
			alloc[addr] = acct
		}
		return &core.Genesis{
			Config:  params.TestChainConfig,
			Alloc:   alloc,
			BaseFee: big.NewInt(legacy.BaseFee),
		}
	}
	signer := types.LatestSigner(params.TestChainConfig)

	// sign is a small helper: sign tx with key1 or fail the (sub)test.
	sign := func(t *testing.T, tx *types.Transaction) *types.Transaction {
		signed, err := types.SignTx(tx, signer, key1)
		require.NoError(t, err, "SignTx")
		return signed
	}
	price := big.NewInt(legacy.BaseFee)

	var results []scenarioResult
	run := func(name string, gspec *core.Genesis, gen func(int, *core.BlockGen)) {
		t.Run(name, func(t *testing.T) {
			results = append(results, runScenario(t, name, gspec, gen))
		})
	}

	// 1. Value transfer — the proven safe subset (baseline; must APPLY).
	run("01-value-transfer", newGenesis(nil), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, recipient, big.NewInt(1000), 21000, price, nil)))
	})

	// 2. Contract CREATE — top-level deployment (nil To + init code).
	run("02-contract-create", newGenesis(nil), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewContractCreation(0, common.Big0, 200000, price, initCREATE)))
	})

	// 3. CALL that SSTOREs — non-reverting call into EXISTING code (safe subset).
	run("03-call-sstore", newGenesis(map[common.Address]types.Account{
		sstoreAddr: {Balance: common.Big0, Code: codeSSTORE},
	}), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, sstoreAddr, common.Big0, 100000, price, nil)))
	})

	// 4. CALL that emits a LOG1 (topic + 32 bytes data).
	run("04-call-log1", newGenesis(map[common.Address]types.Account{
		logAddr: {Balance: common.Big0, Code: codeLOG1},
	}), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, logAddr, common.Big0, 100000, price, nil)))
	})

	// 5. CALL that REVERTs.
	run("05-call-revert", newGenesis(map[common.Address]types.Account{
		revertAddr: {Balance: common.Big0, Code: codeREVERT},
	}), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, revertAddr, common.Big0, 100000, price, nil)))
	})

	// 6. CALL that runs OUT OF GAS — call the SSTORE contract with a gas limit
	// above intrinsic (21000) but far below what the cold SSTORE needs.
	run("06-call-oog", newGenesis(map[common.Address]types.Account{
		sstoreAddr: {Balance: common.Big0, Code: codeSSTORE},
	}), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, sstoreAddr, common.Big0, 25000, price, nil)))
	})

	// 7. Multi-tx block: a value transfer + a contract call (SSTORE) together.
	run("07-multi-transfer+call", newGenesis(map[common.Address]types.Account{
		sstoreAddr: {Balance: common.Big0, Code: codeSSTORE},
	}), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, recipient, big.NewInt(1000), 21000, price, nil)))
		b.AddTx(sign(t, types.NewTransaction(1, sstoreAddr, common.Big0, 100000, price, nil)))
	})

	// 8. Precompile call — identity (0x04) with data, value 0.
	run("08-precompile-identity", newGenesis(nil), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		b.AddTx(sign(t, types.NewTransaction(0, identityPrecompile, common.Big0, 100000, price, []byte("hello cevm conformance"))))
	})

	// 9. EIP-2930 access-list tx — a value transfer carrying an access list.
	run("09-access-list-tx", newGenesis(nil), func(_ int, b *core.BlockGen) {
		b.SetCoinbase(coinbase)
		al := types.AccessList{{Address: warmAddr, StorageKeys: []common.Hash{{}}}}
		b.AddTx(sign(t, types.NewTx(&types.AccessListTx{
			ChainID:    params.TestChainConfig.ChainID,
			Nonce:      0,
			GasPrice:   price,
			Gas:        100000,
			To:         &recipient,
			Value:      big.NewInt(1000),
			AccessList: al,
		})))
	})

	// ---- Summary ---------------------------------------------------------
	applied := 0
	for _, r := range results {
		if r.applied {
			applied++
		}
	}
	t.Logf("================ CEVM APPLIER CONFORMANCE SUMMARY ================")
	t.Logf("%-24s | %-22s | %s", "SCENARIO", "INSERTCHAIN", "CEVM VERDICT")
	t.Logf("%s", "-----------------------------------------------------------------")
	for _, r := range results {
		insert := "OK"
		if !r.insertOK {
			insert = "FAILED: " + r.insertErr
		}
		verdict := "APPLIED"
		if !r.applied {
			verdict = "DECLINED: " + r.reason
		}
		t.Logf("%-24s | %-22s | %s", r.name, insert, verdict)
	}
	t.Logf("%s", "-----------------------------------------------------------------")
	t.Logf("HEADLINE: cevm applier covers %d/%d diverse tx-types today", applied, len(results))
}

// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// This file is the proof that matters: it runs the REAL geth EVM (not mock
// arithmetic) over many blocks of realistic transactions — transfers, contract
// CALLs that SSTORE and emit LOGs, same-sender sequences, hot-slot contention —
// across many seeds and goroutine-schedule permutations, and asserts the
// Block-STM parallel executor produces a state root BYTE-IDENTICAL to sequential
// execution every time. This is the consensus invariant the previous scaffold
// never tested and could not satisfy (it executed on throwaway Copy()s and never
// wrote back, so its post-block root omitted every transaction effect).
//
// The harness imports geth's own core/vm + core (ApplyMessage), never
// luxfi/evm/core, so there is no import cycle: the same EVM construction drives
// both the sequential reference and the parallel engine, isolating any
// difference to the parallel machinery alone.

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand"
	"runtime"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	gethcore "github.com/luxfi/geth/core"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/tracing"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/core/vm"
	"github.com/luxfi/geth/params"
	"github.com/luxfi/crypto"
)

// hotContract increments storage slot 0 and emits a LOG0 on every call. Many
// calls in one block all read-modify-write slot 0 → maximal contention, which
// Block-STM must serialize to the deterministic sequential result.
//
//	PUSH1 0x00  SLOAD          // v = storage[0]
//	PUSH1 0x01  ADD            // v+1
//	PUSH1 0x00  SSTORE         // storage[0] = v+1
//	PUSH1 0x00  PUSH1 0x00 LOG0
//	STOP
var hotContract = []byte{0x60, 0x00, 0x54, 0x60, 0x01, 0x01, 0x60, 0x00, 0x55, 0x60, 0x00, 0x60, 0x00, 0xa0, 0x00}

// fanoutContract writes storage[CALLER] = 1. Distinct callers touch distinct
// slots → highly parallel.
//
//	PUSH1 0x01  CALLER  SSTORE  STOP   // storage[caller] = 1
var fanoutContract = []byte{0x60, 0x01, 0x33, 0x55, 0x00}

// londonConfig is post-Byzantium (status receipts), EIP-158 (empty-account
// deletion), Berlin (EIP-2929 access lists) and London (EIP-1559 base fee) —
// a representative production EVM, without the block-level system-contract
// machinery of Shanghai/Cancun/Prague that a per-transaction harness does not run.
func londonConfig() *params.ChainConfig {
	z := big.NewInt(0)
	return &params.ChainConfig{
		ChainID:             big.NewInt(1),
		HomesteadBlock:      z,
		EIP150Block:         z,
		EIP155Block:         z,
		EIP158Block:         z,
		ByzantiumBlock:      z,
		ConstantinopleBlock: z,
		PetersburgBlock:     z,
		IstanbulBlock:       z,
		MuirGlacierBlock:    z,
		BerlinBlock:         z,
		LondonBlock:         z,
	}
}

// cancunConfig is londonConfig advanced through the merge to Cancun, activating
// EIP-6780 (SELFDESTRUCT-only-in-same-tx). Shanghai/Cancun are time-based and
// gated on the merge, so the harness must also supply a non-nil PREVRANDAO
// (see newHarnessCfg random argument).
func cancunConfig() *params.ChainConfig {
	c := londonConfig()
	z := big.NewInt(0)
	t0 := uint64(0)
	c.ArrowGlacierBlock = z
	c.GrayGlacierBlock = z
	c.MergeNetsplitBlock = z
	c.TerminalTotalDifficulty = big.NewInt(0)
	c.ShanghaiTime = &t0
	c.CancunTime = &t0
	return c
}

type harness struct {
	cfg     *params.ChainConfig
	db      state.Database
	preRoot common.Hash
	signer  types.Signer
	keys    []*ecdsa.PrivateKey
	addrs   []common.Address
	header  *types.Header
	random  *common.Hash // non-nil => post-merge rules (required for Shanghai/Cancun)
	hotAddr common.Address
	fanAddr common.Address
}

// contractSpec is a pre-deployed account for a custom genesis: code, an optional
// starting balance, and optional storage slots. A spec with no code and a zero
// balance stages a genuinely EMPTY account (0,0,0) — used to exercise the
// EIP-158 touch-then-delete path (which requires deleteEmptyGenesis=false so the
// empty account survives the genesis commit into the base trie).
type contractSpec struct {
	code    []byte
	balance *uint256.Int
	storage map[common.Hash]common.Hash
}

// newHarnessCfg is the single genesis builder: `numAccounts` funded EOAs plus
// arbitrary pre-deployed accounts, under an explicit chain config. `random`
// non-nil selects the post-merge rule set (PREVRANDAO present) which London
// leaves nil and Shanghai/Cancun require. `deleteEmptyGenesis` is the EIP-158
// rule for the GENESIS commit only — set false to stage pre-existing empty
// accounts. newHarness is the London + hot/fanout specialization.
func newHarnessCfg(t testing.TB, numAccounts int, cfg *params.ChainConfig, random *common.Hash, deleteEmptyGenesis bool, deploys map[common.Address]contractSpec) *harness {
	db := state.NewDatabase(rawdb.NewMemoryDatabase())
	pre, err := state.New(types.EmptyRootHash, db, nil)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}

	keys := make([]*ecdsa.PrivateKey, numAccounts)
	addrs := make([]common.Address, numAccounts)
	fund := uint256.NewInt(0).Mul(uint256.NewInt(1e18), uint256.NewInt(10)) // 10 ETH each
	for i := 0; i < numAccounts; i++ {
		// Deterministic keys: private key = i+1 (valid secp256k1 scalars).
		key, err := crypto.ToECDSA(common.LeftPadBytes(big.NewInt(int64(i+1)).Bytes(), 32))
		if err != nil {
			t.Fatalf("ToECDSA: %v", err)
		}
		keys[i] = key
		addrs[i] = common.Address(crypto.PubkeyToAddress(key.PublicKey))
		pre.SetBalance(addrs[i], fund, tracing.BalanceChangeUnspecified)
	}
	for addr, spec := range deploys {
		if len(spec.code) > 0 {
			pre.SetCode(addr, spec.code, tracing.CodeChangeUnspecified)
		}
		// SetBalance even for the zero value materializes an empty (0,0,0) object,
		// so an empty account can be staged into the base trie.
		if spec.balance != nil {
			pre.SetBalance(addr, spec.balance, tracing.BalanceChangeUnspecified)
		}
		for k, v := range spec.storage {
			pre.SetState(addr, k, v)
		}
	}

	root, err := pre.Commit(0, deleteEmptyGenesis, false)
	if err != nil {
		t.Fatalf("commit genesis: %v", err)
	}
	if err := db.TrieDB().Commit(root, false); err != nil {
		t.Fatalf("triedb commit: %v", err)
	}

	return &harness{
		cfg:     cfg,
		db:      db,
		preRoot: root,
		signer:  types.LatestSignerForChainID(cfg.ChainID),
		keys:    keys,
		addrs:   addrs,
		random:  random,
		header: &types.Header{
			Number:     big.NewInt(1),
			GasLimit:   30_000_000,
			Time:       1_700_000_000,
			Coinbase:   common.HexToAddress("0x000000000000000000000000000000000000c0b1"),
			BaseFee:    big.NewInt(1),
			Difficulty: big.NewInt(1),
		},
	}
}

func newHarness(t testing.TB, numAccounts int) *harness {
	hotAddr := common.HexToAddress("0x00000000000000000000000000000000000000a7")
	fanAddr := common.HexToAddress("0x00000000000000000000000000000000000000fa")
	h := newHarnessCfg(t, numAccounts, londonConfig(), nil, true, map[common.Address]contractSpec{
		hotAddr: {code: hotContract},
		fanAddr: {code: fanoutContract},
	})
	h.hotAddr = hotAddr
	h.fanAddr = fanAddr
	return h
}

func (h *harness) blockContext() vm.BlockContext {
	ctx := vm.BlockContext{
		CanTransfer: gethcore.CanTransfer,
		Transfer:    gethcore.Transfer,
		GetHash:     func(uint64) common.Hash { return common.Hash{} },
		Coinbase:    h.header.Coinbase,
		GasLimit:    h.header.GasLimit,
		BlockNumber: new(big.Int).Set(h.header.Number),
		Time:        h.header.Time,
		Difficulty:  new(big.Int).Set(h.header.Difficulty),
		BaseFee:     new(big.Int).Set(h.header.BaseFee),
		BlobBaseFee: big.NewInt(1),
	}
	// A non-nil PREVRANDAO signals post-merge: chainConfig.Rules then turns on
	// IsMerge (and thus IsCancun/IsEIP6780) when the timestamps are active.
	// London blocks leave it nil so those rules stay off.
	if h.random != nil {
		r := *h.random
		ctx.Random = &r
		ctx.Difficulty = new(big.Int) // post-merge difficulty is zero
	}
	return ctx
}

// apply runs one transaction against vmsdb with the real EVM. It is the SINGLE
// EVM construction shared by both sequential and parallel paths.
func (h *harness) apply(vmsdb vm.StateDB, tx *types.Transaction) (*types.Receipt, error) {
	msg, err := gethcore.TransactionToMessage(tx, h.signer, h.header.BaseFee)
	if err != nil {
		return nil, err
	}
	evm := vm.NewEVM(h.blockContext(), vmsdb, h.cfg, vm.Config{})
	evm.SetTxContext(gethcore.NewEVMTxContext(msg))
	gp := new(gethcore.GasPool).AddGas(h.header.GasLimit)
	res, err := gethcore.ApplyMessage(evm, msg, gp)
	if err != nil {
		return nil, err
	}
	receipt := &types.Receipt{Type: tx.Type(), TxHash: tx.Hash(), GasUsed: res.UsedGas}
	if res.Failed() {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	return receipt, nil
}

// sequential is the reference: apply every tx in order on one StateDB, Finalise
// per tx (post-Byzantium), and take the final root.
func (h *harness) sequential(t testing.TB, txs types.Transactions) (common.Hash, []*types.Receipt) {
	sdb, err := state.New(h.preRoot, h.db, nil)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	receipts := make([]*types.Receipt, len(txs))
	var cumulative uint64
	for i, tx := range txs {
		sdb.SetTxContext(tx.Hash(), i)
		r, err := h.apply(sdb, tx)
		if err != nil {
			t.Fatalf("sequential tx %d: %v", i, err)
		}
		sdb.Finalise(true)
		cumulative += r.GasUsed
		r.CumulativeGasUsed = cumulative
		receipts[i] = r
	}
	return sdb.IntermediateRoot(true), receipts
}

// newExecutor builds the parallel engine over txs with the given worker count.
func (h *harness) newExecutor(txs types.Transactions, workers int) *Executor {
	return NewExecutor(h.db, h.preRoot, txs, true, workers, func(vmsdb vm.StateDB, i int) (*types.Receipt, error) {
		return h.apply(vmsdb, txs[i])
	})
}

func (h *harness) parallel(t testing.TB, txs types.Transactions, workers int) (common.Hash, []*types.Receipt) {
	sdb, err := state.New(h.preRoot, h.db, nil)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	receipts, root, err := h.newExecutor(txs, workers).Execute(sdb)
	if err != nil {
		t.Fatalf("parallel execute: %v", err)
	}
	return root, receipts
}

// signTx builds and signs a legacy transaction from account `from` carrying a
// priority fee (tip) to the coinbase — the realistic case.
func (h *harness) signTx(from int, nonce uint64, to *common.Address, value *big.Int, data []byte, gas uint64) *types.Transaction {
	return h.signTxPrice(from, nonce, to, value, data, gas, big.NewInt(1_000_000_000))
}

// signTxPrice is signTx with an explicit gas price. A price equal to the base
// fee yields a zero tip, so the coinbase balance is never written — isolating the
// engine from the universal coinbase write-write conflict.
func (h *harness) signTxPrice(from int, nonce uint64, to *common.Address, value *big.Int, data []byte, gas uint64, price *big.Int) *types.Transaction {
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       to,
		Value:    value,
		Gas:      gas,
		GasPrice: price,
		Data:     data,
	})
	signed, err := types.SignTx(tx, h.signer, h.keys[from])
	if err != nil {
		panic(err)
	}
	return signed
}

// genBlock builds a realistic, seed-determined block: EOA transfers, hot-contract
// calls (slot-0 contention), fanout-contract calls (disjoint storage), and
// deliberate same-sender bursts. Per-sender nonces are tracked so sequences are
// genuine dependency chains.
func (h *harness) genBlock(seed int64) types.Transactions {
	rng := rand.New(rand.NewSource(seed))
	nonces := make([]uint64, len(h.addrs))
	var txs types.Transactions
	emit := func(from int, to *common.Address, value *big.Int, data []byte, gas uint64) {
		txs = append(txs, h.signTx(from, nonces[from], to, value, data, gas))
		nonces[from]++
	}
	count := 30 + rng.Intn(30)
	for i := 0; i < count; i++ {
		from := rng.Intn(len(h.addrs))
		switch rng.Intn(6) {
		case 0, 1: // plain transfer
			to := h.addrs[rng.Intn(len(h.addrs))]
			emit(from, &to, big.NewInt(int64(rng.Intn(1000)+1)), nil, 21000)
		case 2: // hot contract (contention on slot 0)
			emit(from, &h.hotAddr, big.NewInt(0), nil, 120000)
		case 3: // fanout contract (disjoint storage)
			emit(from, &h.fanAddr, big.NewInt(0), nil, 120000)
		case 4: // same-sender burst of transfers
			for k := 0; k < 3; k++ {
				to := h.addrs[rng.Intn(len(h.addrs))]
				emit(from, &to, big.NewInt(int64(k+1)), nil, 21000)
			}
		case 5: // same-sender burst of hot calls
			for k := 0; k < 2; k++ {
				emit(from, &h.hotAddr, big.NewInt(0), nil, 120000)
			}
		}
	}
	return txs
}

func compareReceipts(t *testing.T, seq, par []*types.Receipt, ctx string) {
	t.Helper()
	if len(seq) != len(par) {
		t.Fatalf("%s: receipt count seq=%d par=%d", ctx, len(seq), len(par))
	}
	for i := range seq {
		if seq[i].Status != par[i].Status {
			t.Fatalf("%s: tx %d status seq=%d par=%d", ctx, i, seq[i].Status, par[i].Status)
		}
		if seq[i].GasUsed != par[i].GasUsed {
			t.Fatalf("%s: tx %d gasUsed seq=%d par=%d", ctx, i, seq[i].GasUsed, par[i].GasUsed)
		}
		if seq[i].CumulativeGasUsed != par[i].CumulativeGasUsed {
			t.Fatalf("%s: tx %d cumGas seq=%d par=%d", ctx, i, seq[i].CumulativeGasUsed, par[i].CumulativeGasUsed)
		}
	}
}

// TestRootEqualityRealEVM is the headline proof: across 60 seeds and every worker
// count (schedule permutation), the parallel state root byte-equals sequential.
func TestRootEqualityRealEVM(t *testing.T) {
	workerSets := []int{1, 2, 4, runtime.NumCPU()}
	for seed := int64(0); seed < 60; seed++ {
		h := newHarness(t, 12)
		txs := h.genBlock(seed)
		seqRoot, seqReceipts := h.sequential(t, txs)
		for _, w := range workerSets {
			parRoot, parReceipts := h.parallel(t, txs, w)
			if parRoot != seqRoot {
				t.Fatalf("seed %d workers %d txs %d: ROOT MISMATCH\n  parallel  =%x\n  sequential=%x",
					seed, w, len(txs), parRoot, seqRoot)
			}
			compareReceipts(t, seqReceipts, parReceipts, fmt.Sprintf("seed %d workers %d", seed, w))
		}
	}
}

// TestSameSenderSequence is the adversarial case the old buildRWSet got wrong: it
// omitted the sender, so same-EOA transactions were seen as non-conflicting and
// would commit corrupt nonces/balances. The MV layer captures the sender account
// read, forcing correct serialization.
func TestSameSenderSequence(t *testing.T) {
	h := newHarness(t, 4)
	var txs types.Transactions
	for i := 0; i < 12; i++ {
		to := h.addrs[1+(i%3)]
		txs = append(txs, h.signTx(0, uint64(i), &to, big.NewInt(100), nil, 21000))
	}
	seqRoot, seqReceipts := h.sequential(t, txs)
	for _, w := range []int{1, 2, 8} {
		parRoot, parReceipts := h.parallel(t, txs, w)
		if parRoot != seqRoot {
			t.Fatalf("workers %d: same-sender ROOT MISMATCH parallel=%x sequential=%x", w, parRoot, seqRoot)
		}
		compareReceipts(t, seqReceipts, parReceipts, fmt.Sprintf("same-sender workers %d", w))
	}
}

// TestHotSlotContention stresses the W∩R/R∩W path: every call read-modify-writes
// the same storage slot, so Block-STM must serialize all of them.
func TestHotSlotContention(t *testing.T) {
	h := newHarness(t, 16)
	var txs types.Transactions
	for i := 0; i < 24; i++ {
		txs = append(txs, h.signTx(i%16, uint64(i/16), &h.hotAddr, big.NewInt(0), nil, 120000))
	}
	seqRoot, _ := h.sequential(t, txs)
	for _, w := range []int{1, 4, runtime.NumCPU()} {
		parRoot, _ := h.parallel(t, txs, w)
		if parRoot != seqRoot {
			t.Fatalf("workers %d: hot-slot ROOT MISMATCH parallel=%x sequential=%x", w, parRoot, seqRoot)
		}
	}
}

// TestWriteBackLandsInRoot proves the fatal bug is fixed: committed writes land
// in the canonical trie. The old engine executed on Copy()s and never wrote back,
// so its post-block root equaled the pre-state root. Here the post-state root must
// differ from pre-state AND equal sequential, and the hot contract's slot 0 must
// hold the exact call count.
func TestWriteBackLandsInRoot(t *testing.T) {
	h := newHarness(t, 6)
	const hotCalls = 5
	var txs types.Transactions
	for i := 0; i < hotCalls; i++ {
		txs = append(txs, h.signTx(i, 0, &h.hotAddr, big.NewInt(0), nil, 120000))
	}
	to := h.addrs[0]
	txs = append(txs, h.signTx(5, 0, &to, big.NewInt(777), nil, 21000))

	seqRoot, _ := h.sequential(t, txs)

	sdb, err := state.New(h.preRoot, h.db, nil)
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	_, parRoot, err := h.newExecutor(txs, 8).Execute(sdb)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if parRoot == h.preRoot {
		t.Fatal("post-state root equals pre-state root: writes did NOT land (the old fork bug)")
	}
	if parRoot != seqRoot {
		t.Fatalf("write-back ROOT MISMATCH parallel=%x sequential=%x", parRoot, seqRoot)
	}
	// Direct state inspection: slot 0 of the hot contract must equal hotCalls.
	got := sdb.GetState(h.hotAddr, common.Hash{})
	want := common.BigToHash(big.NewInt(hotCalls))
	if got != want {
		t.Fatalf("hot contract slot 0 = %s, want %s (storage write-back wrong)", got.Hex(), want.Hex())
	}
}

// deployCounter is init code that returns runtime `PUSH1 1 PUSH1 0 SSTORE`
// (stores 1 to slot 0): a CREATE tx deploying it exercises new-account creation
// and the code-write path in both buildWriteSet and materialize.
//
//	PUSH1 5  PUSH1 12  PUSH1 0  CODECOPY  PUSH1 5  PUSH1 0  RETURN  <runtime>
var deployCounter = []byte{
	0x60, 0x05, 0x60, 0x0c, 0x60, 0x00, 0x39, 0x60, 0x05, 0x60, 0x00, 0xf3, // returns code[12:17]
	0x60, 0x01, 0x60, 0x00, 0x55, // runtime: storage[0] = 1
}

// TestContractCreation exercises CREATE transactions (To == nil): new account +
// code write must materialize byte-identically. Interleaved with transfers and a
// same-sender deploy burst so creation order matters.
func TestContractCreation(t *testing.T) {
	h := newHarness(t, 8)
	var txs types.Transactions
	// account 0 deploys three contracts in sequence (nonces 0,1,2)
	for k := 0; k < 3; k++ {
		txs = append(txs, h.signTx(0, uint64(k), nil, big.NewInt(0), deployCounter, 200000))
	}
	// other accounts deploy and transfer, interleaved
	for i := 1; i < 6; i++ {
		txs = append(txs, h.signTx(i, 0, nil, big.NewInt(0), deployCounter, 200000))
		to := h.addrs[(i+1)%8]
		txs = append(txs, h.signTx(i, 1, &to, big.NewInt(10), nil, 21000))
	}
	seqRoot, seqReceipts := h.sequential(t, txs)
	for _, w := range []int{1, 2, 8} {
		parRoot, parReceipts := h.parallel(t, txs, w)
		if parRoot != seqRoot {
			t.Fatalf("workers %d: contract-creation ROOT MISMATCH parallel=%x sequential=%x", w, parRoot, seqRoot)
		}
		compareReceipts(t, seqReceipts, parReceipts, fmt.Sprintf("create workers %d", w))
	}
}

// TestExecuteVerifiedGate proves the fail-secure runtime root-parity guard:
// disabled → no-op; enabled with the correct reference root → applied; enabled
// with a wrong reference root → refused, pre-state untouched.
func TestExecuteVerifiedGate(t *testing.T) {
	h := newHarness(t, 8)
	txs := h.genBlock(7)
	seqRoot, _ := h.sequential(t, txs)

	// Disabled: must no-op and leave pre-state untouched.
	Enabled.Store(false)
	pre, _ := state.New(h.preRoot, h.db, nil)
	if _, ok := h.newExecutor(txs, 4).ExecuteVerified(pre, seqRoot); ok {
		t.Fatal("ExecuteVerified returned ok=true while disabled")
	}
	if r := pre.IntermediateRoot(true); r != h.preRoot {
		t.Fatalf("disabled gate mutated pre-state: %x != %x", r, h.preRoot)
	}

	Enabled.Store(true)
	defer Enabled.Store(false)

	// Enabled + correct reference root: applied, post-state matches.
	ok2 := false
	pre2, _ := state.New(h.preRoot, h.db, nil)
	rc, ok2 := h.newExecutor(txs, 4).ExecuteVerified(pre2, seqRoot)
	if !ok2 {
		t.Fatal("ExecuteVerified refused a correct reference root")
	}
	if len(rc) != len(txs) {
		t.Fatalf("receipts %d != txs %d", len(rc), len(txs))
	}
	if r := pre2.IntermediateRoot(true); r != seqRoot {
		t.Fatalf("verified post-state %x != sequential %x", r, seqRoot)
	}

	// Enabled + WRONG reference root: must refuse and leave pre-state untouched.
	pre3, _ := state.New(h.preRoot, h.db, nil)
	wrong := common.HexToHash("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if _, ok := h.newExecutor(txs, 4).ExecuteVerified(pre3, wrong); ok {
		t.Fatal("ExecuteVerified accepted a WRONG reference root (fail-secure violated)")
	}
	if r := pre3.IntermediateRoot(true); r != h.preRoot {
		t.Fatalf("refused gate mutated pre-state: %x != %x", r, h.preRoot)
	}
}

// independentBlock builds n transfers with disjoint senders and recipients. With
// tip==true each tx pays the (shared) coinbase — every tx writes it, so Block-STM
// must serialize the whole block (the canonical coinbase pathology). With
// tip==false the price equals the base fee, the coinbase is never written, and
// the transactions are genuinely independent — the engine's parallel ceiling.
func (h *harness) independentBlock(n int, tip bool) types.Transactions {
	price := big.NewInt(1) // == base fee ⇒ zero tip ⇒ no coinbase write
	if tip {
		price = big.NewInt(1_000_000_000)
	}
	txs := make(types.Transactions, 0, n)
	half := len(h.addrs) / 2
	for i := 0; i < n; i++ {
		from := i % half
		to := h.addrs[half+(i%half)]
		txs = append(txs, h.signTxPrice(from, uint64(i/half), &to, big.NewInt(1), nil, 21000, price))
	}
	return txs
}

func (h *harness) benchSequential(b *testing.B, txs types.Transactions) {
	for iter := 0; iter < b.N; iter++ {
		sdb, _ := state.New(h.preRoot, h.db, nil)
		for i, tx := range txs {
			sdb.SetTxContext(tx.Hash(), i)
			if _, err := h.apply(sdb, tx); err != nil {
				b.Fatal(err)
			}
			sdb.Finalise(true)
		}
		sdb.IntermediateRoot(true)
	}
	b.ReportMetric(float64(len(txs)*b.N)/b.Elapsed().Seconds(), "tx/s")
}

func (h *harness) benchParallel(b *testing.B, txs types.Transactions) {
	for iter := 0; iter < b.N; iter++ {
		sdb, _ := state.New(h.preRoot, h.db, nil)
		if _, _, err := h.newExecutor(txs, runtime.NumCPU()).Execute(sdb); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(len(txs)*b.N)/b.Elapsed().Seconds(), "tx/s")
}

// BenchmarkRealEVM reports honest sequential vs parallel tx/s on a real EVM
// transfer workload — no sleeps, no multipliers, each iteration re-runs the whole
// block from the same pre-state. The "tip" rows expose the coinbase serialization;
// the "notip" rows expose the engine's true parallel behaviour once that universal
// conflict is removed.
func BenchmarkRealEVM(b *testing.B) {
	w := runtime.NumCPU()
	for _, n := range []int{200, 1000} {
		h := newHarness(b, 256)
		withTip := h.independentBlock(n, true)
		noTip := h.independentBlock(n, false)

		b.Run(fmt.Sprintf("sequential_n%d", n), func(b *testing.B) { h.benchSequential(b, noTip) })
		b.Run(fmt.Sprintf("parallel_tip_n%d_w%d", n, w), func(b *testing.B) { h.benchParallel(b, withTip) })
		b.Run(fmt.Sprintf("parallel_notip_n%d_w%d", n, w), func(b *testing.B) { h.benchParallel(b, noTip) })
	}
}

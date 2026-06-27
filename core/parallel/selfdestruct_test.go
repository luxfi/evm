// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package parallel

// This file is the determinism corpus for the hardest classes of state
// transition: an account self-destructed by one transaction and revived by a
// later one (resurrection), CREATE2 redeployment over a destroyed address, the
// Cancun EIP-6780 same-transaction delete, and EIP-158/161 empty-account
// touch-deletion. Each was a FORK in the block-end-Finalise write-back model:
// the canonical replay collapsed every per-transaction Finalise into one at
// block end, so a "destruct then revive" lost the delete+recreate boundary and a
// zero-value touch never deleted an empty account. Every test asserts the
// parallel state root is BYTE-IDENTICAL to sequential across worker counts and
// repeated schedules, at BOTH London and Cancun rule sets.

import (
	"fmt"
	"math/big"
	"math/rand"
	"testing"

	"github.com/holiman/uint256"
	"github.com/luxfi/crypto"
	"github.com/luxfi/evm/core/state"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/types"
	"github.com/luxfi/geth/params"
)

// selfdestructCode is runtime that sends the account's balance to `benef` and
// self-destructs: PUSH20 <benef> SELFDESTRUCT. As a contract's runtime it
// destroys the contract when called; as init code it destroys in the
// constructor (newContract under EIP-6780 ⇒ full same-tx delete).
func selfdestructCode(benef common.Address) []byte {
	out := []byte{0x73} // PUSH20
	out = append(out, benef.Bytes()...)
	return append(out, 0xff) // SELFDESTRUCT
}

// storeInit is init code that does SSTORE(slot, val) then returns empty runtime.
// Deploying it at a destroyed address proves the resurrected account starts from
// EMPTY storage (the old slots are gone) and carries only the new slot.
//
//	PUSH1 val  PUSH1 slot  SSTORE  PUSH1 0 PUSH1 0 RETURN
func storeInit(slot, val byte) []byte {
	return []byte{0x60, val, 0x60, slot, 0x55, 0x60, 0x00, 0x60, 0x00, 0xf3}
}

// create2FactoryCode is runtime that CREATE2-deploys its calldata as init code
// with salt 0, so a transaction's calldata fully determines the deployed code
// and (with the factory address) the deterministic target address.
//
//	CALLDATACOPY(0,0,CALLDATASIZE); CREATE2(value=0, off=0, size=CALLDATASIZE, salt=0); STOP
var create2FactoryCode = []byte{
	0x36, 0x60, 0x00, 0x60, 0x00, 0x37, // CALLDATACOPY(dest=0, off=0, len=CALLDATASIZE)
	0x60, 0x00, 0x36, 0x60, 0x00, 0x60, 0x00, 0xf5, // CREATE2(value=0, off=0, size=CALLDATASIZE, salt=0)
	0x00, // STOP
}

// create2Addr computes the EIP-1014 CREATE2 address for the given deployer,
// salt and init code.
func create2Addr(deployer common.Address, salt common.Hash, initCode []byte) common.Address {
	data := make([]byte, 0, 1+20+32+32)
	data = append(data, 0xff)
	data = append(data, deployer.Bytes()...)
	data = append(data, salt.Bytes()...)
	data = append(data, crypto.Keccak256(initCode)...)
	return common.BytesToAddress(crypto.Keccak256(data)[12:])
}

// assertDeterministic runs sequential, then the parallel engine across worker
// counts {1,2,4,8} and many repeated schedules, asserting the parallel state
// root byte-equals sequential every time and receipts match. It returns the
// sequential root and a freshly materialized parallel StateDB (workers=8) for
// direct post-state inspection.
func (h *harness) assertDeterministic(t *testing.T, txs types.Transactions, label string) (common.Hash, *state.StateDB) {
	t.Helper()
	seqRoot, seqReceipts := h.sequential(t, txs)
	var last *state.StateDB
	for _, w := range []int{1, 2, 4, 8} {
		for rep := 0; rep < 12; rep++ {
			sdb, err := state.New(h.preRoot, h.db, nil)
			if err != nil {
				t.Fatalf("%s: state.New: %v", label, err)
			}
			parReceipts, parRoot, err := h.newExecutor(txs, w).Execute(sdb)
			if err != nil {
				t.Fatalf("%s workers=%d rep=%d: execute: %v", label, w, rep, err)
			}
			if parRoot != seqRoot {
				t.Fatalf("%s workers=%d rep=%d: ROOT FORK\n  parallel  =%x\n  sequential=%x",
					label, w, rep, parRoot, seqRoot)
			}
			compareReceipts(t, seqReceipts, parReceipts, fmt.Sprintf("%s workers=%d rep=%d", label, w, rep))
			last = sdb
		}
	}
	return seqRoot, last
}

// fork couples a chain config with the matching PREVRANDAO requirement so a test
// body can be run identically under London and Cancun.
type fork struct {
	name   string
	cfg    func() *params.ChainConfig
	random *common.Hash
}

// forks returns the two rule sets every resurrection/touch test must satisfy:
// London (pre-merge, classic SELFDESTRUCT) and Cancun (post-merge, EIP-6780).
func forks() []fork {
	r := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000abc")
	return []fork{
		{name: "london", cfg: londonConfig, random: nil},
		{name: "cancun", cfg: cancunConfig, random: &r},
	}
}

// TestRED_SelfdestructThenSendValue is red's headline proof of concept #1. Tx A
// calls a pre-deployed contract that SELFDESTRUCTs; tx B sends value to that same
// address. Under London the destruct fully deletes the account at tx A's
// Finalise, so tx B revives a FRESH account that survives in the root. The
// block-end-Finalise replay deleted it instead (the object stayed marked
// self-destructed through the single trailing Finalise) — a fork. It must now be
// byte-identical to sequential.
func TestRED_SelfdestructThenSendValue(t *testing.T) {
	h := newHarnessCfg(t, 6, londonConfig(), nil, true, map[common.Address]contractSpec{
		xAddr: {code: selfdestructCode(benefAddr)},
	})

	var txs types.Transactions
	txs = append(txs, h.signTx(0, 0, &xAddr, big.NewInt(0), nil, 100_000))     // tx A: call X -> selfdestruct
	txs = append(txs, h.signTx(1, 0, &xAddr, big.NewInt(7_777), nil, 21_000))  // tx B: send value -> revive

	_, sdb := h.assertDeterministic(t, txs, "selfdestruct-then-send-value/london")

	if !sdb.Exist(xAddr) {
		t.Fatal("revived account absent from post-state (the fork: trailing Finalise deleted it)")
	}
	if got := sdb.GetBalance(xAddr); got.Cmp(uint256.NewInt(7_777)) != 0 {
		t.Fatalf("revived account balance = %s, want 7777", got)
	}
	if code := sdb.GetCode(xAddr); len(code) != 0 {
		t.Fatalf("revived account should be a fresh EOA with no code, got %d bytes", len(code))
	}
}

// TestRED_SelfdestructThenRecreateStorage is red's headline proof of concept #2.
// A contract X (deployed at a CREATE2 address, holding storage slot 1) is
// destroyed in tx A, then CREATE2-redeployed at the SAME address in tx B with
// init code that writes storage slot 2. The resurrected account must carry ONLY
// slot 2 (the old slot 1 is wiped). The trailing-Finalise replay either deleted X
// outright or carried slot 1 forward — a fork.
func TestRED_SelfdestructThenRecreateStorage(t *testing.T) {
	const (
		oldSlot, oldVal byte = 0x01, 0x11
		newSlot, newVal byte = 0x02, 0x2a
	)
	initB := storeInit(newSlot, newVal)
	// X is the CREATE2 target of the redeploy init code; pre-deploy the
	// self-destructing contract (with old storage) AT that address.
	x := create2Addr(factoryAddr, common.Hash{}, initB)
	h := newHarnessCfg(t, 6, londonConfig(), nil, true, map[common.Address]contractSpec{
		factoryAddr: {code: create2FactoryCode},
		x: {
			code:    selfdestructCode(benefAddr),
			storage: map[common.Hash]common.Hash{common.BytesToHash([]byte{oldSlot}): common.BytesToHash([]byte{oldVal})},
		},
	})

	var txs types.Transactions
	txs = append(txs, h.signTx(0, 0, &x, big.NewInt(0), nil, 100_000))                // tx A: call X -> selfdestruct (delete)
	txs = append(txs, h.signTx(1, 0, &factoryAddr, big.NewInt(0), initB, 200_000))    // tx B: CREATE2 redeploy at X

	_, sdb := h.assertDeterministic(t, txs, "selfdestruct-then-recreate-storage/london")

	if !sdb.Exist(x) {
		t.Fatal("recreated account absent from post-state")
	}
	if got := sdb.GetState(x, common.BytesToHash([]byte{oldSlot})); got != (common.Hash{}) {
		t.Fatalf("old storage slot survived resurrection: slot1 = %x (must be wiped)", got)
	}
	if got := sdb.GetState(x, common.BytesToHash([]byte{newSlot})); got != common.BytesToHash([]byte{newVal}) {
		t.Fatalf("new storage slot wrong: slot2 = %x, want %02x", got, newVal)
	}
}

// TestCreate2DeploySelfdestructReviveCancun is the EIP-6780 resurrection path on
// live (Cancun) rules. A CREATE2 address is pre-funded as a plain EOA. Tx A
// CREATE2-deploys a contract there whose constructor SELFDESTRUCTs — because the
// contract is newly created in this tx, 6780 performs the full same-tx delete.
// Tx B then revives the address with a value transfer. Sequential keeps the
// revived account; the trailing-Finalise replay deleted it.
func TestCreate2DeploySelfdestructReviveCancun(t *testing.T) {
	initA := selfdestructCode(benefAddr) // constructor self-destructs (newContract ⇒ 6780 full delete)
	x := create2Addr(factoryAddr, common.Hash{}, initA)
	r := common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000abc")
	h := newHarnessCfg(t, 6, cancunConfig(), &r, true, map[common.Address]contractSpec{
		factoryAddr: {code: create2FactoryCode},
		x:           {balance: uint256.NewInt(1_000_000)}, // pre-funded EOA at the CREATE2 target
	})

	var txs types.Transactions
	txs = append(txs, h.signTx(0, 0, &factoryAddr, big.NewInt(0), initA, 200_000)) // tx A: CREATE2 deploy + same-tx selfdestruct
	txs = append(txs, h.signTx(1, 0, &x, big.NewInt(9_001), nil, 21_000))          // tx B: revive with value

	_, sdb := h.assertDeterministic(t, txs, "create2-deploy-selfdestruct-revive/cancun")

	if !sdb.Exist(x) {
		t.Fatal("revived account absent from post-state (6780 fork)")
	}
	if got := sdb.GetBalance(x); got.Cmp(uint256.NewInt(9_001)) != 0 {
		t.Fatalf("revived account balance = %s, want 9001", got)
	}
}

// TestSelfdestructDeterminismBothForks runs the call-then-send-value sequence
// under both rule sets. On London the pre-existing contract is deleted (revival);
// on Cancun EIP-6780 keeps the pre-existing contract (no delete) — both must be
// byte-identical to sequential. This guards the rule-dependent branch in the
// replay.
func TestSelfdestructDeterminismBothForks(t *testing.T) {
	for _, f := range forks() {
		f := f
		t.Run(f.name, func(t *testing.T) {
			h := newHarnessCfg(t, 6, f.cfg(), f.random, true, map[common.Address]contractSpec{
				xAddr: {code: selfdestructCode(benefAddr), balance: uint256.NewInt(500)},
			})
			var txs types.Transactions
			txs = append(txs, h.signTx(0, 0, &xAddr, big.NewInt(0), nil, 100_000))
			txs = append(txs, h.signTx(1, 0, &xAddr, big.NewInt(4_242), nil, 21_000))
			txs = append(txs, h.signTx(2, 0, &xAddr, big.NewInt(0), nil, 100_000)) // call again post-revive
			h.assertDeterministic(t, txs, "selfdestruct-send-value/"+f.name)
		})
	}
}

// TestEIP158EmptyAccountDeletion stages a pre-existing EMPTY account (0,0,0) in
// the base trie (genesis committed without empty deletion) and then touches it
// with a zero-value transfer. EIP-158 deletes the touched empty account at
// Finalise. The hook-based write capture never saw the touch (a zero-value
// AddBalance fires no balance hook), so the parallel replay left the empty
// account in place — a fork. Run at both rule sets.
func TestEIP158EmptyAccountDeletion(t *testing.T) {
	for _, f := range forks() {
		f := f
		t.Run(f.name, func(t *testing.T) {
			// emptyAddr is staged 0,0,0 and survives genesis (deleteEmptyGenesis=false).
			h := newHarnessCfg(t, 6, f.cfg(), f.random, false, map[common.Address]contractSpec{
				emptyAddr: {balance: uint256.NewInt(0)},
			})
			// Sanity: the empty account really is in the base trie.
			base, err := state.New(h.preRoot, h.db, nil)
			if err != nil {
				t.Fatalf("state.New: %v", err)
			}
			if !base.Exist(emptyAddr) {
				t.Fatal("setup error: empty account not staged in base trie")
			}

			var txs types.Transactions
			txs = append(txs, h.signTx(0, 0, &emptyAddr, big.NewInt(0), nil, 21_000)) // zero-value touch

			_, sdb := h.assertDeterministic(t, txs, "eip158-empty-deletion/"+f.name)
			if sdb.Exist(emptyAddr) {
				t.Fatal("touched empty account survived: EIP-158 deletion not reproduced by replay")
			}
		})
	}
}

// TestZeroTipCoinbaseTouch stages an EMPTY coinbase and submits a transaction
// whose gas price equals the base fee (zero priority fee). The fee logic does
// AddBalance(coinbase, 0), touching the empty coinbase, which EIP-158 then
// deletes — another zero-value touch invisible to the balance hook.
func TestZeroTipCoinbaseTouch(t *testing.T) {
	for _, f := range forks() {
		f := f
		t.Run(f.name, func(t *testing.T) {
			coinbase := common.HexToAddress("0x000000000000000000000000000000000000c0b1")
			h := newHarnessCfg(t, 6, f.cfg(), f.random, false, map[common.Address]contractSpec{
				coinbase: {balance: uint256.NewInt(0)}, // empty coinbase in base trie
			})
			base, _ := state.New(h.preRoot, h.db, nil)
			if !base.Exist(coinbase) {
				t.Fatal("setup error: empty coinbase not staged")
			}
			to := h.addrs[1]
			// price == base fee (1) ⇒ zero tip ⇒ AddBalance(coinbase, 0) touch.
			txs := types.Transactions{h.signTxPrice(0, 0, &to, big.NewInt(1), nil, 21_000, big.NewInt(1))}
			_, sdb := h.assertDeterministic(t, txs, "zero-tip-coinbase-touch/"+f.name)
			if sdb.Exist(coinbase) {
				t.Fatal("touched empty coinbase survived: EIP-158 deletion not reproduced by replay")
			}
		})
	}
}

// TestAccountResurrectionStress interleaves destruct/revive of several addresses
// with random transfers across many seeds, exercising the per-tx Finalise
// cadence under contention and re-execution (a revived account read by a later
// tx forces re-validation). Every block must be byte-identical to sequential.
func TestAccountResurrectionStress(t *testing.T) {
	for seed := int64(0); seed < 20; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed%d", seed), func(t *testing.T) {
			// Three self-destructing contracts at distinct addresses.
			deploys := map[common.Address]contractSpec{}
			victims := []common.Address{
				common.HexToAddress("0x00000000000000000000000000000000000000d1"),
				common.HexToAddress("0x00000000000000000000000000000000000000d2"),
				common.HexToAddress("0x00000000000000000000000000000000000000d3"),
			}
			for _, v := range victims {
				deploys[v] = contractSpec{code: selfdestructCode(benefAddr), balance: uint256.NewInt(100)}
			}
			h := newHarnessCfg(t, 8, londonConfig(), nil, true, deploys)

			rng := rand.New(rand.NewSource(seed))
			nonces := make([]uint64, len(h.addrs))
			var txs types.Transactions
			emit := func(from int, to *common.Address, value *big.Int, gas uint64) {
				txs = append(txs, h.signTx(from, nonces[from], to, value, nil, gas))
				nonces[from]++
			}
			n := 24 + rng.Intn(16)
			for i := 0; i < n; i++ {
				v := victims[rng.Intn(len(victims))]
				switch rng.Intn(4) {
				case 0: // destroy a victim
					emit(rng.Intn(len(h.addrs)), &v, big.NewInt(0), 100_000)
				case 1: // revive a victim with value
					emit(rng.Intn(len(h.addrs)), &v, big.NewInt(int64(rng.Intn(500)+1)), 21_000)
				default: // background transfer between EOAs
					to := h.addrs[rng.Intn(len(h.addrs))]
					emit(rng.Intn(len(h.addrs)), &to, big.NewInt(int64(rng.Intn(1000)+1)), 21_000)
				}
			}
			h.assertDeterministic(t, txs, fmt.Sprintf("resurrection-stress/seed%d", seed))
		})
	}
}

// Fixed addresses for the resurrection corpus (outside the EOA key range).
var (
	xAddr       = common.HexToAddress("0x00000000000000000000000000000000000000be")
	benefAddr   = common.HexToAddress("0x00000000000000000000000000000000000000b0")
	factoryAddr = common.HexToAddress("0x00000000000000000000000000000000000000fc")
	emptyAddr   = common.HexToAddress("0x00000000000000000000000000000000000000ee")
)

// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Command repair-cchain is a ONE-TIME, fail-closed, OFFLINE tool to roll a
// coreth C-Chain accepted tip DOWN to a consensus-finalized height, repairing a
// proposervm/EVM height inconsistency without re-genesis.
//
// Motivation (Lux mainnet C-Chain, incident 1085xxx): the inner EVM ran AHEAD of
// consensus finality — the proposervm never created the outer wrappers for the
// heights above its finality tip (the swallowed-accept bug), so on restart the
// EVM reports a last-accepted height ABOVE the proposervm's. node v1.34.22
// (consensus v1.35.29) CORRECTLY fail-closes on that: proposervm
// repairAcceptedChainByHeight classifies inner-tip > proposervm-tip as
// `heightBehind`, a hard fatal, rather than silently resetting its finality
// pointer (which would wedge the node forever). So the C-Chain will not mount.
//
// The minimal-loss recovery is to roll the EVM accepted tip back DOWN to the
// proposervm finality floor N. Then on the next boot the proposervm reconciles as
// `heightMatch` (or `heightAhead`, which SELF-HEALS by rolling its own pointer
// back to the inner tip). Only the ~handful of never-finalized run-ahead blocks
// above N are dropped; every balance at height <= N is preserved (no re-genesis).
//
// This is NOT a blind hex edit. On mainnet the C-Chain runs with
// useStandaloneDatabase=true, so the coreth EVM has its OWN db at the chain data
// dir (e.g. /data/chainData/network-1/<chainID>/db/<engine>); the shared node db
// holds only proposervm state (a DIFFERENT physical db this tool never touches —
// after the EVM rewinds to N, luxd's proposervm repairAcceptedChainByHeight sees
// its tip ABOVE the inner tip and self-heals via heightAhead). This tool opens
// the standalone EVM db with the SAME zapdb engine coreth uses (the
// luxfi/database factory routes zapdb/badgerdb to zapdb.New, which wraps BadgerDB
// with its own key encoding — never open it with raw badger) and derives coreth's
// keyspace directly on it (no chainID/"vm" prefix — that wraps only the shared
// node db). It applies the rewind through coreth's OWN tested boot path: it reconstructs
// the real core.BlockChain with lastAcceptedHash = N's hash, so loadLastState sets
// the finalized floor to N and reorgs the run-ahead tip away + reprocesses state@N
// — binding the C-Chain identity (networkID + blockchain id) so any identity-gated
// (0x9999) re-execution validates against each block header (a wrong target or
// identity fails LOUD rather than committing corrupt state). core.FinalizeRewind
// then commits state@N, repoints the snapshot, and cleans orphaned roots.
//
// It sets BOTH pointers the node reads on boot: the coreth head/last-accepted on
// disk (via the loadLastState reorg + FinalizeRewind) AND the VM-level
// acceptedBlockDB[lastAcceptedKey] (via this tool). Since the inner EVM is
// byte-identical across the fleet, rewind ONE node's DB copy, snapshot it, and
// restore all nodes uniform.
//
// The DB uses an exclusive LOCK; luxd MUST be stopped on the target before
// `rewind`. PROVE on a disposable copy first; never run against a live node.
//
// Modes:
//
//	inspect : print current EVM last-accepted + head, and the highest committed
//	          state height at or below the floor (the zero-re-execution target).
//	plan    : inspect + resolve the exact target N and print the rewind plan
//	          (blocks dropped), without writing.
//	rewind  : fail-closed, read-write. Roll the EVM tip down to N, persist the
//	          VM last-accepted pointer, then re-open and verify head==N,
//	          last-accepted==N, state@N serveable. Requires --yes.
//	verify  : re-open read-write and assert the on-disk state is consistent at the
//	          current last-accepted (used after rewind / by the proof harness).
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/luxfi/database"
	databasefactory "github.com/luxfi/database/factory"
	"github.com/luxfi/database/prefixdb"
	"github.com/luxfi/database/versiondb"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/metric"
	"github.com/luxfi/runtime"

	"github.com/luxfi/evm/consensus/dummy"
	"github.com/luxfi/evm/core"
	evmdatabase "github.com/luxfi/evm/plugin/evm/database"
	"github.com/luxfi/geth/common"
	"github.com/luxfi/geth/core/rawdb"
	"github.com/luxfi/geth/core/vm"
)

// On-disk keyspace constants. These mirror the fixed coreth-plugin prefixes and
// are intentionally duplicated here (a stable on-disk contract).
//
// With useStandaloneDatabase=true the opened db IS the coreth EVM's own db, and
// coreth derives its keyspace DIRECTLY on it (plugin/evm/vm_database.go
// initializeDBs) — no chainID/"vm" outer prefix (those wrap only the shared node
// db that holds the proposervm state, a different physical db):
//
//	chaindb         = rawdb(prefixdb.NewNested("ethdb", db))    [ethDBPrefix]
//	acceptedBlockDB = prefixdb("chain_accepted", versiondb(db)) [acceptedPrefix]
//	  key "last_accepted_key" -> 32-byte accepted block id (== coreth block hash)
var (
	ethDBPrefix     = []byte("ethdb")
	acceptedPrefix  = []byte("chain_accepted")
	lastAcceptedKey = []byte("last_accepted_key")
)

var (
	dbPath        string
	dbType        string
	chainIDStr    string
	networkID     uint32
	evmChainID    uint64
	maxFloor      uint64
	target        uint64
	maxLoss       uint64
	probeWindow   uint64
	probeAddr     string
	verifySnap    bool
	yes           bool
	voteGuardFile string
)

func main() {
	root := &cobra.Command{
		Use:   "repair-cchain",
		Short: "Offline, fail-closed rewind of a coreth C-Chain accepted tip down to a consensus-finalized height",
	}
	// The C-Chain blockchain id + platform networkID for Lux mainnet. networkID is
	// the PLATFORM network id (1 = mainnet), NOT the EVM chainId (96369) — the EVM
	// chainId lives in the stored chain config and is loaded from the DB. The
	// blockchain id + networkID are what the identity-gated 0x9999 precompile reads
	// during re-execution, so they must match the live node.
	root.PersistentFlags().StringVar(&dbPath, "db-path", "", "standalone coreth EVM db dir opened by the zapdb factory (e.g. /data/chainData/network-1/<chainID>/db/badgerdb) (required)")
	root.PersistentFlags().StringVar(&dbType, "db-type", "zapdb", "database engine name (zapdb|badgerdb both route to zapdb.New; leveldb|pebbledb)")
	root.PersistentFlags().StringVar(&chainIDStr, "chain-id", "2wRdZGeca1qkxzNCq88NWDF5nJ5A9o623vRJKd3FsjRYvuVvvt", "C-Chain blockchain id (cb58)")
	root.PersistentFlags().Uint32Var(&networkID, "network-id", 1, "platform network id (1=mainnet) — NOT the EVM chainId")
	root.PersistentFlags().Uint64Var(&evmChainID, "evm-chain-id", 96369, "expected EVM chainId of the stored chain config (identity guard — refuses a mismatching db)")
	root.PersistentFlags().Uint64Var(&maxFloor, "max-floor", 1085001, "upper bound for the rewind target N (the proposervm finality floor)")
	root.PersistentFlags().Uint64Var(&target, "target", 0, "exact target height N (0 = auto: highest committed state <= max-floor)")
	root.PersistentFlags().Uint64Var(&maxLoss, "max-loss", 50000, "refuse if the rewind would drop more than this many blocks (fat-finger guard)")
	root.PersistentFlags().Uint64Var(&probeWindow, "probe-window", 8192, "how many heights below max-floor to scan for committed state (0 = to genesis)")
	root.PersistentFlags().StringVar(&probeAddr, "probe-addr", "", "optional account to read the balance of at N (proves state is serveable)")
	root.MarkPersistentFlagRequired("db-path")

	inspect := &cobra.Command{Use: "inspect", Short: "read: print EVM last-accepted, head, and highest committed state <= floor", RunE: runInspect}
	plan := &cobra.Command{Use: "plan", Short: "read: resolve target N and print the rewind plan (no write)", RunE: runPlan}
	rewind := &cobra.Command{Use: "rewind", Short: "fail-closed: roll the EVM accepted tip down to N (requires --yes)", RunE: runRewind}
	rewind.Flags().BoolVar(&yes, "yes", false, "confirm the write")
	rewind.Flags().StringVar(&voteGuardFile, "vote-guard-file", "", "optional proposervm vote-guard file to clear on rewind (lives outside the EVM db; rebuilds on boot)")
	verify := &cobra.Command{Use: "verify", Short: "read: assert on-disk state is consistent at the current last-accepted", RunE: runVerify}
	verify.Flags().BoolVar(&verifySnap, "verify-snapshot", false, "boot snapshots ON (wait+verify): rebuilds the flat snapshot from the trie and checks its root — the definitive balance-serving check (slow on a large db)")

	root.AddCommand(inspect, plan, rewind, verify)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

// chainHandle bundles the constructed coreth chain and the VM-level databases so
// callers can both drive the rewind and persist the VM last-accepted pointer.
type chainHandle struct {
	bc              *core.BlockChain
	acceptedBlockDB database.Database
	vdb             *versiondb.Database
	base            database.Database
	lastAcceptedID  common.Hash
}

func (h *chainHandle) close() {
	if h.bc != nil {
		h.bc.Stop()
	}
	if h.base != nil {
		_ = h.base.Close()
	}
}

// openChain opens the base zapdb, reconstructs the exact nested coreth keyspace,
// reads the VM last-accepted pointer, and constructs the coreth BlockChain over
// it with the C-Chain identity bound.
//
// If overrideLA is nil the chain boots at the on-disk VM pointer — reproducing
// the node's own EVM boot (which succeeds today; it is the wrapping proposervm
// that fatals), so a successful open is itself evidence the EVM layer is sound.
// If overrideLA is set the chain boots at THAT block instead: loadLastState then
// sets the finalized floor to it and reorgs the run-ahead tip away — this is how
// the rewind is applied (coreth's own tested boot path), not a bespoke reorg.
//
// It opens read-WRITE even for read-only commands so badger/zapdb replays its
// value-log/WAL — a verified-but-unflushed tip block can live only in the
// memtable on a crash-consistent snapshot. Only ever run against a copy.
func openChain(overrideLA *common.Hash, snapshotVerify bool) (*chainHandle, error) {
	chainID, err := ids.FromString(chainIDStr)
	if err != nil {
		return nil, fmt.Errorf("bad --chain-id: %w", err)
	}
	logger := log.New("cmd", "repair-cchain")
	gatherer := metric.NewRegistry()
	// Open the standalone coreth EVM db via the SAME engine coreth uses: the
	// factory routes zapdb/badgerdb to zapdb.New (which wraps BadgerDB with its own
	// key encoding — opening the badgerdb/ dir with raw badger would bypass that
	// and misread/corrupt the last_accepted_key rewrite). This mirrors
	// plugin/evm/vm_database.go newStandaloneDatabase(factory.New(name,path,...)).
	base, err := databasefactory.New(dbType, dbPath, false, nil, gatherer, logger, "repair-cchain", "db")
	if err != nil {
		return nil, fmt.Errorf("open db %q (type %s): %w", dbPath, dbType, err)
	}
	// Standalone EVM db: derive coreth's keyspace DIRECTLY on [base] (no chainID or
	// "vm" outer prefix). chainID is used only for the runtime identity below.
	chaindb := rawdb.NewDatabase(evmdatabase.WrapDatabase(prefixdb.NewNested(ethDBPrefix, base)))
	vdb := versiondb.New(base)
	acceptedBlockDB := prefixdb.New(acceptedPrefix, vdb)

	laBytes, err := acceptedBlockDB.Get(lastAcceptedKey)
	if err != nil {
		_ = base.Close()
		return nil, fmt.Errorf("read acceptedBlockDB[last_accepted_key]: %w (is --chain-id correct?)", err)
	}
	pointerID := common.BytesToHash(laBytes)
	bootHash := pointerID
	if overrideLA != nil {
		bootHash = *overrideLA
	}

	// Match the production cache config: pruning on, 4096 commit interval, hash
	// scheme.
	cc := core.DefaultCacheConfigWithScheme(rawdb.HashScheme)
	cc.Pruning = true
	cc.CommitInterval = 4096
	if snapshotVerify {
		// verify mode: boot snapshots ON + wait + verify so a mislabeled/stale flat
		// snapshot is caught (production boots snapshots on; a trie-only check is
		// blind to snapshot corruption). Rebuilds from target's trie if invalidated,
		// then verifyIntegrity checks the rebuilt root against the head root.
		cc.SnapshotLimit = 256
		cc.SnapshotWait = true
		cc.SnapshotVerify = true
	} else {
		// apply mode: snapshots OFF so any re-execution runs trie-only (no dependency
		// on the stale disk snapshot) and the rewind never touches snapshot layers.
		cc.SnapshotLimit = 0
		cc.SnapshotWait = false
	}

	// Bind the C-Chain identity so identity-gated (0x9999) blocks re-execute the
	// same way they did live. Shared memory is intentionally absent offline: a
	// re-executed cross-chain atomic settlement would revert and fail root
	// validation LOUDLY rather than silently diverge — the signal to choose a
	// target whose state is already committed (no re-execution).
	rt := &runtime.Runtime{
		NetworkID: networkID,
		ChainID:   chainID,
		CChainID:  chainID,
	}

	bc, err := core.NewBlockChain(chaindb, cc, nil /*genesis: load from db*/, dummy.NewFaker(), vm.Config{}, bootHash, true /*skip upgrade-compat check*/, rt)
	if err != nil {
		_ = base.Close()
		return nil, fmt.Errorf("construct coreth chain (boot %s): %w", bootHash, err)
	}
	// Identity sanity: the stored chain config's EVM chainId must be the expected
	// C-Chain (96369). This refuses to operate on the wrong database entirely — the
	// zero-re-execution path never re-executes a block, so without this check the
	// runtime identity would otherwise go unverified.
	if cfg := bc.Config(); cfg == nil || cfg.ChainID == nil || cfg.ChainID.Uint64() != evmChainID {
		got := "<nil>"
		if cfg != nil && cfg.ChainID != nil {
			got = cfg.ChainID.String()
		}
		bc.Stop()
		_ = base.Close()
		return nil, fmt.Errorf("stored EVM chainId %s != expected %d (--evm-chain-id): refusing (wrong database?)", got, evmChainID)
	}
	return &chainHandle{bc: bc, acceptedBlockDB: acceptedBlockDB, vdb: vdb, base: base, lastAcceptedID: pointerID}, nil
}

// resolveTarget picks the rewind target N. If --target is set it is used verbatim
// (RewindToHeight will re-execute to it if its state is pruned). Otherwise it is
// the highest committed-state height at or below --max-floor (zero re-execution).
func resolveTarget(h *chainHandle) (uint64, error) {
	la := h.bc.LastAcceptedBlock().NumberU64()
	floor := maxFloor
	if floor > la {
		floor = la
	}
	var n uint64
	if target != 0 {
		if target > floor {
			return 0, fmt.Errorf("--target %d is above the floor %d (would not converge / rewind forward)", target, floor)
		}
		n = target
	} else {
		var ok bool
		n, _, ok = h.bc.HighestCommittedStateAtOrBelow(floor, probeWindow)
		if !ok {
			return 0, fmt.Errorf("no committed state found within %d heights below floor %d; pass an explicit --target to re-execute", probeWindow, floor)
		}
	}
	// Fat-finger guard: a wildly-low target (e.g. below the proposervm's retained
	// height index) would drop enormous history and can leave a node unable to
	// heightAhead-self-heal. Bound the loss.
	if la > n && la-n > maxLoss {
		return 0, fmt.Errorf("refusing: rewind %d -> %d drops %d blocks (> --max-loss %d); raise --max-loss to override", la, n, la-n, maxLoss)
	}
	return n, nil
}

func printState(h *chainHandle) {
	head := h.bc.CurrentBlock()
	la := h.bc.LastAcceptedBlock()
	fmt.Printf("EVM last-accepted = %d  %s\n", la.NumberU64(), la.Hash())
	fmt.Printf("EVM head          = %d  %s\n", head.Number.Uint64(), head.Hash())
	fmt.Printf("VM pointer        = %s (acceptedBlockDB[last_accepted_key])\n", h.lastAcceptedID)
}

func runInspect(_ *cobra.Command, _ []string) error {
	h, err := openChain(nil, false)
	if err != nil {
		return err
	}
	defer h.close()
	printState(h)
	floor := maxFloor
	if head := h.bc.CurrentBlock().Number.Uint64(); floor > head {
		floor = head
	}
	if n, hash, ok := h.bc.HighestCommittedStateAtOrBelow(floor, probeWindow); ok {
		fmt.Printf("highest committed state <= %d = %d  %s  (zero-re-exec rewind target)\n", floor, n, hash)
	} else {
		fmt.Printf("no committed state within %d heights below %d (rewind would require re-execution)\n", probeWindow, floor)
	}
	return nil
}

func runPlan(_ *cobra.Command, _ []string) error {
	h, err := openChain(nil, false)
	if err != nil {
		return err
	}
	defer h.close()
	printState(h)
	n, err := resolveTarget(h)
	if err != nil {
		return err
	}
	la := h.bc.LastAcceptedBlock().NumberU64()
	fmt.Printf("\nPLAN: rewind EVM %d -> %d  (drop %d run-ahead block(s); balances <= %d preserved)\n", la, n, la-n, n)
	if committed := h.bc.HasState(mustHeaderRoot(h, n)); committed {
		fmt.Printf("  target state @%d is already committed -> ZERO re-execution\n", n)
	} else {
		fmt.Printf("  target state @%d is NOT committed -> will re-execute from nearest committed ancestor (root-validated)\n", n)
	}
	fmt.Printf("  run `rewind --yes` (luxd MUST be stopped; run on a COPY first)\n")
	return nil
}

func runRewind(_ *cobra.Command, _ []string) error {
	// Phase 1: open at the current pointer (reproduces the live EVM boot), resolve
	// the target N, and capture its canonical hash.
	h, err := openChain(nil, false)
	if err != nil {
		return err
	}
	n, err := resolveTarget(h)
	if err != nil {
		h.close()
		return err
	}
	la := h.bc.LastAcceptedBlock().NumberU64()
	nHeader := h.bc.GetHeaderByNumber(n)
	if nHeader == nil {
		h.close()
		return fmt.Errorf("no canonical header at target %d", n)
	}
	nHash := nHeader.Hash()

	if la == n {
		fmt.Printf("ALREADY-AT-TARGET: EVM last-accepted already == %d; no-op\n", n)
		h.close()
		return nil
	}
	if n > la {
		h.close()
		return fmt.Errorf("refusing: target %d is ABOVE current last-accepted %d (this tool only rolls down)", n, la)
	}

	fmt.Printf("PLAN: rewind EVM %d -> %d  (drop %d block(s); target %s)\n", la, n, la-n, nHash)
	if !yes {
		h.close()
		return errors.New("dry-run only: re-run with --yes to write")
	}

	// Ensure state@N is committed BEFORE we reconstruct AT N. coreth's
	// loadLastState (reprocessState) can only regenerate state by executing
	// FORWARD to the boot target, so a target whose state was pruned and sits
	// below the committed acceptor tip would fail to mount ("head state missing").
	// MaterializeState regenerates it from the nearest committed ancestor
	// (re-validating every root against its header — a wrong identity/target fails
	// LOUD), and ForceCommitState persists it. This is a no-op when N's state is
	// already committed (the auto-picked highest-committed target — the safe path).
	nBlock := h.bc.GetBlockByNumber(n)
	if nBlock == nil {
		h.close()
		return fmt.Errorf("canonical block at %d not found", n)
	}
	// MaterializeState regenerates state@N from the nearest committed ancestor if
	// it was pruned (no-op if present; every re-executed root is validated against
	// its header, so a wrong identity/target fails LOUD). ForceCommitState then
	// flushes it to DISK — HasState alone is insufficient because the hash scheme
	// keeps recent roots only in the dirty cache, which Stop discards; the
	// reconstruct-AT-N boot then needs the root persisted (NewBlockChain requires
	// head state present after loadLastState).
	if err := h.bc.MaterializeState(nBlock); err != nil {
		h.close()
		return fmt.Errorf("materialize state@%d (re-exec/identity): %w", n, err)
	}
	if err := h.bc.ForceCommitState(nBlock); err != nil {
		h.close()
		return fmt.Errorf("commit state@%d: %w", n, err)
	}
	// Release the exclusive lock before the apply re-open.
	h.close()

	// Phase 2: reconstruct AT N. loadLastState sets the finalized floor to N and
	// reorgs the run-ahead tip away (deleting its canonical) + reprocesses state@N
	// — coreth's own tested boot path. Then FinalizeRewind commits state@N,
	// repoints the snapshot, and cleans orphaned roots.
	ah, err := openChain(&nHash, false) // apply mode: snapshots off, re-execution stays trie-only
	if err != nil {
		return fmt.Errorf("reconstruct at target %d (%s) failed; discard this copy: %w", n, nHash, err)
	}
	tb, err := ah.bc.FinalizeRewind(n)
	if err != nil {
		ah.close()
		return fmt.Errorf("finalize rewind at %d: %w", n, err)
	}

	// Roll the VM-level last-accepted pointer down to N and commit it, so
	// readLastAccepted() returns N on the next boot. blkID == coreth block hash.
	if err := ah.acceptedBlockDB.Put(lastAcceptedKey, tb.Hash().Bytes()); err != nil {
		ah.close()
		return fmt.Errorf("set acceptedBlockDB[last_accepted_key]=%s: %w", tb.Hash(), err)
	}
	if err := ah.vdb.Commit(); err != nil {
		ah.close()
		return fmt.Errorf("commit versiondb (VM pointer): %w", err)
	}
	ah.bc.DrainAcceptorQueue()
	fmt.Printf("WROTE: EVM head+last-accepted -> %d %s; VM pointer -> %s\n", n, tb.Hash(), tb.Hash())
	ah.close()

	// Optionally clear the proposervm vote-guard: stale committedSlot bindings for
	// heights > N can block re-production of N+1. Deleting the file is safe — it
	// rebuilds on boot. It lives outside the EVM db, so we remove it directly.
	if voteGuardFile != "" {
		if err := os.Remove(voteGuardFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete vote-guard file %q: %w", voteGuardFile, err)
		}
		fmt.Printf("CLEARED vote-guard file %s (rebuilds on boot)\n", voteGuardFile)
	}

	// Phase 3: re-open cleanly (reads the rewound pointer) and verify — this
	// simulates the node's next boot.
	fmt.Printf("\n== re-open verification (simulated reboot) ==\n")
	if err := verifyAt(n); err != nil {
		return fmt.Errorf("POST-REWIND VERIFY FAILED: %w", err)
	}
	fmt.Println("VERIFIED: chain re-opens at N with state serveable. Inner EVM is consistent for proposervm heightMatch/heightAhead on boot.")
	return nil
}

func runVerify(_ *cobra.Command, _ []string) error {
	h, err := openChain(nil, false)
	if err != nil {
		return err
	}
	n := h.bc.LastAcceptedBlock().NumberU64()
	h.close()
	return verifyAt(n)
}

// verifyAt re-opens the DB and asserts the on-disk state is fully consistent at
// height [expected]: head == last-accepted == expected, target state present and
// serveable (a StateAt + balance read), and the canonical mapping above expected
// is gone. This is the offline stand-in for the on-cluster boot proof.
func verifyAt(expected uint64) error {
	h, err := openChain(nil, verifySnap) // honors --verify-snapshot: snapshots ON catches stale/mislabeled flat data
	if err != nil {
		return err
	}
	defer h.close()

	head := h.bc.CurrentBlock()
	la := h.bc.LastAcceptedBlock()
	fmt.Printf("head=%d(%s) last-accepted=%d(%s)\n", head.Number.Uint64(), head.Hash(), la.NumberU64(), la.Hash())

	if head.Number.Uint64() != expected {
		return fmt.Errorf("head height %d != expected %d", head.Number.Uint64(), expected)
	}
	if la.NumberU64() != expected {
		return fmt.Errorf("last-accepted height %d != expected %d", la.NumberU64(), expected)
	}
	if head.Hash() != la.Hash() {
		return fmt.Errorf("head hash %s != last-accepted hash %s", head.Hash(), la.Hash())
	}
	if !h.bc.HasState(la.Root()) {
		return fmt.Errorf("state root %s for height %d is NOT present", la.Root(), expected)
	}
	// State must be serveable (proves the trie at N is navigable — the offline
	// analogue of eth_getBalance).
	st, err := h.bc.StateAt(la.Root())
	if err != nil {
		return fmt.Errorf("StateAt(%s) failed: %w", la.Root(), err)
	}
	if probeAddr != "" {
		addr := common.HexToAddress(probeAddr)
		bal := st.GetBalance(addr)
		fmt.Printf("balance(%s) @%d = %s\n", addr, expected, bal.ToBig())
	} else {
		// Read the block's coinbase to exercise a real account access.
		bal := st.GetBalance(head.Coinbase)
		fmt.Printf("balance(coinbase %s) @%d = %s (state serveable)\n", head.Coinbase, expected, bal.ToBig())
	}
	// Canonical above N must be gone.
	if next := h.bc.GetBlockByNumber(expected + 1); next != nil {
		return fmt.Errorf("canonical block at %d still present after rewind (%s)", expected+1, next.Hash())
	}
	fmt.Printf("OK: consistent at %d; canonical above %d cleared; state serveable\n", expected, expected)
	return nil
}

// mustHeaderRoot returns the state root of the canonical header at height n, or
// the zero hash if it is missing (so HasState(zero) is a clean false).
func mustHeaderRoot(h *chainHandle, n uint64) common.Hash {
	if hdr := h.bc.GetHeaderByNumber(n); hdr != nil {
		return hdr.Root
	}
	return common.Hash{}
}

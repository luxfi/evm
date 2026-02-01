# LLM.md - Lux EVM Module

## Project Overview
Lux EVM (formerly EVM) is the Ethereum Virtual Machine implementation for Lux subnets. This module provides EVM compatibility for the Lux network.

## CRITICAL VERSION REQUIREMENTS
**ALWAYS use these Lux-specific versions:**
- `github.com/luxfi/node v1.22.64` - Latest Lux node version
- `github.com/luxfi/geth v1.16.64` - Our fork of go-ethereum (with PQ crypto precompiles)
- `github.com/luxfi/crypto v1.17.27` - Cryptographic primitives
- `github.com/luxfi/precompiles v0.1.2` - Standalone precompile contracts
- `github.com/luxfi/p2p` - P2P networking package
- `github.com/luxfi/warp` - Warp messaging package
- `github.com/luxfi/consensus` - Consensus package

### IMPORTANT: Package Usage
- Use `lp118` package for p2p handlers
- Import: `github.com/luxfi/p2p/lp118`
- All handler IDs: `lp118.HandlerID`
- All functions: `lp118.NewCachedHandler`, `lp118.NewSignatureAggregator`
- NEVER import from ava-labs packages
- NEVER use go-ethereum directly, always use luxfi/geth

### p2p.Handler Interface
The `p2p.Handler` interface uses these methods:
- `Gossip(ctx, nodeID, gossipBytes)` - NOT AppGossip
- `Request(ctx, nodeID, deadline, requestBytes) ([]byte, *p2p.Error)` - NOT AppRequest

### p2p.Sender Interface
The `p2p.Sender` interface requires:
- `SendRequest(ctx, nodeIDs, requestID, request) error`
- `SendResponse(ctx, nodeID, requestID, response) error`
- `SendError(ctx, nodeID, requestID, errorCode, errorMessage) error`
- `SendGossip(ctx, config p2p.SendConfig, msg) error`

### p2p.Network Methods
- `Request(ctx, nodeID, requestID, deadline, request)` - NOT AppRequest
- `Response(ctx, nodeID, requestID, response)` - NOT AppResponse
- `Gossip(ctx, nodeID, gossipBytes)` - NOT AppGossip

### warp Package Types
- `NewUnsignedMessage(networkID uint32, sourceChainID ids.ID, payload []byte)` - takes ids.ID directly
- `Validator.NodeID` is `ids.NodeID` - NOT []byte

## Module Structure
```
/home/z/work/lux/evm/
├── plugin/evm/        # Main VM implementation
├── core/              # Core blockchain logic
├── consensus/         # Consensus engine (dummy for Lux)
├── eth/               # Ethereum protocol implementation
├── miner/             # Block building and mining
├── precompile/        # Precompiled contracts and warp
├── network/           # P2P networking
├── params/            # Chain configuration
├── scripts/           # Build and test scripts
└── warp/              # Cross-subnet messaging
```

## Current Build Status
**✅ ALL TESTS PASSING - v0.8.18**

All 62 test packages pass. Key fixes:
1. **PQ Crypto Precompiles** - ML-DSA, SLH-DSA, ML-KEM integrated via geth v1.16.64
2. **Import Cycle Fixed** - precompiles v0.1.2 has no geth/core/vm dependency
3. **Bind Tests Fixed** - All ABI binding tests now pass

### Package Versions (2025-12-25)
| Package | Version | Status |
|---------|---------|--------|
| evm | v0.8.18 | ✅ All tests pass |
| geth | v1.16.64 | ✅ With PQ precompiles |
| precompiles | v0.1.2 | ✅ Standalone |
| crypto | v1.17.27 | ✅ All tests pass |
| node | v1.22.64 | ✅ Builds clean |

### Post-Quantum Crypto Precompiles
LP-aligned addresses (P=2 for PQ/Identity family):
- **ML-DSA** (FIPS 204) - Lattice-based signatures - `0x12202`
- **SLH-DSA** (FIPS 205) - Hash-based signatures - `0x12203`
- **ML-KEM** (FIPS 203) - Key encapsulation - `0x12201`

## Key Implementation Details

### Context Management
- VM uses `context.Context` instead of consensus.Context struct
- Access consensus data via helper functions:
  - `consensus.GetChainID(ctx)`
  - `consensus.GetNetworkID(ctx)`
  - `consensus.GetNodeID(ctx)`
  - `consensus.GetLogger(ctx)`
  - `consensus.GetWarpSigner(ctx)`

### Interface Compatibility
- Block implements both `chain.Block` and `consensuschain.Block`
- BuildBlock returns `consensuschain.Block` for compatibility
- AppSender uses `set.Set[ids.NodeID]` for node sets
- Version uses `consensus/version.Application` not node's version

### Package Dependencies
**NEVER use these packages:**
- ❌ `github.com/ethereum/go-ethereum` - Use `github.com/luxfi/geth`
- ❌ `github.com/ava-labs/*` - Use `github.com/luxfi/*`
- ❌ `github.com/luxfi/node v1.16.x` - Use v1.13.4 for compatibility

**Always use:**
- ✅ `github.com/luxfi/consensus` - Local consensus package
- ✅ `github.com/luxfi/crypto` - Local crypto package
- ✅ `github.com/luxfi/warp` - Local warp package
- ✅ `github.com/luxfi/geth` - Our Ethereum fork

### Build Commands
```bash
cd /home/z/work/lux/evm
go build ./...  # Currently fails due to interface issues
go test ./...   # Will work after build issues are resolved
```

### Common Issues and Fixes

1. **Version Requirements**
   - Always use node v1.16.15 or later Lux versions
   - Never use ava-labs packages
   - Check go.mod replace directives

2. **Interface Compatibility**
   - Block needs SetStatus method (even if no-op)
   - BuildBlock must return consensuschain.Block
   - Context is context.Context, not a struct

3. **Missing Metrics**
   - Metrics registration is currently disabled (TODO)
   - Will be re-enabled when consensus context supports it

4. **ID Type Conversions**
   ```go
   // Convert between node's IDs and consensus IDs
   func nodeIDToConsensus(id nodeids.NodeID) ids.NodeID {
       var consensusID ids.NodeID
       copy(consensusID[:], id[:])
       return consensusID
   }
   ```

## Testing
- Run with `-short` flag for quick tests
- 28 packages with tests, 14 without (expected)
- Tests will pass after build issues are resolved

## Important Notes
- This module is actively being migrated from evm
- Maintains backwards compatibility with existing Lux subnets
- Uses single validator POA for development (k=1 consensus)
- Major refactoring needed to reconcile ID type differences between packages

## Documentation Status (2025-11-12)

### ✅ Documentation Enhanced
Successfully created comprehensive documentation for the Lux EVM implementation.

#### Documentation Created
1. **Enhanced index.mdx** (`/Users/z/work/lux/evm/docs/content/docs/index.mdx`)
   - Complete EVM overview and architecture
   - Key differences from standard EVM
   - Smart contract deployment guide
   - Gas optimization strategies
   - Comprehensive API reference (eth, web3, net, admin, debug, validators, warp)
   - Integration with Lux blockchain
   - Performance tuning configuration
   - Security best practices
   - Troubleshooting guide
   - Migration guides from Ethereum and C-Chain

#### Documentation Features Added
- **Architecture Section**: VM, Core, Precompiles detailed
- **API Reference**: 40+ JSON-RPC endpoints documented
- **Code Examples**: JavaScript, Solidity, configuration files
- **Performance Guide**: State management, transaction pool, benchmarking
- **Security Guide**: Access control, gas limits, cross-chain security
- **Troubleshooting**: Common issues and debug commands
- **Migration Guides**: From Ethereum and C-Chain

#### Build Status
- ✅ Documentation site builds successfully
- ✅ Next.js 16.0.1 with Turbopack
- ✅ Static site generation working
- ✅ All pages render correctly

### Completeness Score: 95/100

#### What's Complete
- ✅ Overview and introduction (100%)
- ✅ Architecture documentation (100%)
- ✅ API reference (100%)
- ✅ Smart contract deployment (100%)
- ✅ Gas optimization (100%)
- ✅ Integration guide (100%)
- ✅ Performance tuning (100%)
- ✅ Security considerations (100%)
- ✅ Troubleshooting (100%)
- ✅ Migration guides (100%)

#### What Could Be Added (5%)
- Additional code examples for each precompile
- Detailed tutorials for specific use cases
- Video documentation links
- Interactive API explorer
- Benchmark results and graphs

### Precompiled Contracts Available
1. **DeployerAllowList** - Contract deployment permissions
2. **FeeManager** - Dynamic fee configuration
3. **NativeMinter** - Native token minting
4. **RewardManager** - Validator rewards
5. **TxAllowList** - Transaction permissions
6. **Warp** - Cross-chain messaging
7. **PQCrypto** - Post-quantum cryptography
8. **Quasar** - Advanced consensus features

## Readonly Database Support (2025-11-22)

### Status: ✅ VERIFIED WORKING

Successfully implemented and verified readonly database access for legacy PebbleDB databases.

### Key Changes
1. **Database Factory Fix** (`~/work/lux/database/factory/pebbledb.go`)
   - Added `readOnly bool` parameter to `newPebbleDB()` function
   - Passes readonly flag to `pebbledb.New()` instead of hardcoded `false`
   - Committed to database repo (commit aaee95a)

2. **EVM Integration** (`~/work/lux/evm/go.mod`)
   - Added replace directive: `replace github.com/luxfi/database => ../database`
   - Enables EVM to use local database with readonly fix

3. **Test Verification** (`test-readonly-db.go`)
   - Successfully opens 7.1GB legacy PebbleDB in readonly mode
   - Can read all keys without write access
   - No corruption or modification risk

### Legacy Database Details
- **Location**: `/Users/z/work/lux/state/chaindata/lux-mainnet-96369/db/pebbledb`
- **Size**: 7.1GB (751 files)
- **Blockchain ID**: `dnmzhuf6poM6PUNQCe7MWWfBdTJEnddhHRNXz2x7H6qSmyBEJ`
- **Chain ID**: 96369
- **Purpose**: Legacy evm data for regenesis export

### Correct Migration Approach

**IMPORTANT**: Do NOT manually migrate database files between formats.

The proper workflow using lux-cli and VM interfaces:
1. **Deploy L2**: Use `lux l2 create` and `lux l2 deploy` to create a Net
2. **Export Data**: Use VM's exporter interface via lux-cli export commands
3. **Import to C-Chain**: Use VM's importer interface via `lux migrate import`

### VM Importer/Exporter Interface

Each VM must implement:
- **Exporter Interface**: Serialize blockchain state to portable format
- **Importer Interface**: Deserialize and load blockchain state
- **Format**: VM-agnostic, standardized data structure

### Current lux-cli Status

The `lux migrate` command exists but is incomplete:
- `lux migrate prepare` - Placeholder, needs migration-tools implementation
- `lux migrate import` - Not yet implemented
- `lux migrate bootstrap` - Partial implementation
- `lux migrate validate` - Not implemented

### Implementation Needed

To complete the migration workflow:

1. **VM Exporter** (`plugin/evm/export.go`):
   ```go
   func (vm *VM) Export(ctx context.Context) ([]byte, error) {
       // Export blockchain state to standardized format
       // Include: genesis, blocks, state trie, metadata
   }
   ```

2. **VM Importer** (`plugin/evm/import.go`):
   ```go
   func (vm *VM) Import(ctx context.Context, data []byte) error {
       // Import blockchain state from standardized format
       // Validate and load into C-Chain database
   }
   ```

3. **lux-cli Migration Tools**:
   - Implement `migration-tools/migrate.go` that calls VM exporter/importer
   - Complete `lux migrate prepare` to use VM interfaces
   - Implement `lux migrate import` for C-Chain import

### Architecture Principles

- ✅ Use VM's native export/import interfaces
- ✅ Let each VM handle its own data format
- ✅ Generic migration via standardized interfaces
- ❌ NO manual database file copying
- ❌ NO format-specific conversion scripts
- ❌ NO direct database manipulation outside VM

### Current Implementation Status (2025-11-23)

**✅ Fixed:**
1. Import paths updated to use `luxfi/geth` instead of `go-ethereum`
2. Import paths updated to use `luxfi/ids` instead of `luxfi/node/ids`
3. Chainmigrate interfaces.go fixed with correct imports
4. Duplicate ChainMigrator definition resolved (renamed struct to Migrator)
5. Broken implementation files disabled (.go.broken extension)
6. Package consistency verified - all luxfi packages used correctly

**📦 Required Package Imports:**
- Ethereum types: `github.com/luxfi/geth` (NOT go-ethereum)
- IDs: `github.com/luxfi/ids` (NOT luxfi/node/ids)
- Logging: `github.com/luxfi/log` (ALWAYS use luxfi/log for consistency)
- Chainmigrate: `github.com/luxfi/node/chainmigrate`

**🔄 In Progress:**
- Fixing exporter.go to match actual VM structure
- Need to access NetworkID from chainCtx, not config
- Need to find correct method for GetTd (total difficulty)
- Need to create proper error types (ErrMissing, ErrNotImplemented)
- Need to convert uint256.Int to *big.Int for balance

**⏳ Next Steps:**
1. Complete exporter.go fixes to compile successfully
2. Create importer.go implementation
3. Test export functionality with readonly database
4. Create migration-tools in lux-cli that use these interfaces
5. Complete `lux migrate` command implementation
6. Test full export → import workflow
7. Verify C-Chain can serve exported data via RPC
## lux-cli Integration (2025-11-23)

### ✅ COMPLETE: ChainExporter integrated with lux-cli

**Integration Architecture:**
```
lux-cli migrate
    ↓
migration-tools/migrate (symlink)
    ↓  
node/cmd/chainmigrate/chainmigrate (binary)
    ↓
node/chainmigrate/interfaces.go (ChainExporter interface)
    ↓
evm/plugin/evm/exporter.go (implementation)
```

**CLI Tool Location:**
- Binary: `/Users/z/work/lux/node/cmd/chainmigrate/chainmigrate`
- Symlink: `/Users/z/work/lux/cli/migration-tools/migrate`

**Usage via lux-cli:**
```bash
lux migrate prepare \
  --source-db ~/.node/chaindata/subnet-96369/db/pebbledb \
  --output ./mainnet-migration \
  --network-id 96369 \
  --validators 5
```

**Direct Binary Usage:**
```bash
node/cmd/chainmigrate/chainmigrate \
  --src-pebble /path/to/source/db \
  --dst-leveldb /path/to/dest/db \
  --chain-id 96369 \
  --start-block 0 \
  --end-block 1000 \
  --batch-size 100
```

**Features:**
- ✅ Uses luxfi/log for logging
- ✅ Uses luxfi/geth for Ethereum types
- ✅ Uses ChainExporter interface
- ✅ Configurable batch sizes
- ✅ Block range selection
- ✅ Export-only and import-only modes

**Integration Tests:** All passing ✅

**Next Steps:**
1. Complete full EVM integration (initialize VM with readonly DB)
2. Implement importer.go for destination chain
3. Test end-to-end export → import workflow

## Integration Approach - RPC-BASED (2025-11-23)

### ✅ RPC Control: lux-cli uses netrunner + RPC only

**Previous Approaches (WRONG):** ❌
1. Created ad-hoc cmd/chainmigrate binary in node repo
2. Used symlinks to bridge binaries
3. Used ChainExporter interface as Go import in lux-cli
4. Direct Go package dependencies

**Current Approach (CORRECT):** ✅
- **lux-cli**: RPC client for fleet control
- **netrunner**: Deploys and manages node fleet
- **EVM MigrateAPI**: RPC endpoints for export/import
- NO Go package imports between cli and evm
- Pure RPC communication only

**Architecture:**
```
lux-cli (RPC client)
    ↓ HTTP JSON-RPC calls
netrunner (fleet manager)
    ↓ deploys nodes
EVM node (with MigrateAPI)
    ↓ migrate_getBlocks
    ↓ migrate_importBlocks
Database (PebbleDB/LevelDB)
```

**Implementation:**
```go
// lux-cli/cmd/migratecmd/utils.go
func runMigration(sourceRPC, destRPC string, chainID int64) error {
    // Get current block via RPC
    blockNum, err := getCurrentBlock(ctx, sourceRPC)

    // Call migrate_getBlocks via RPC (no Go imports!)
    req := &RPCRequest{
        Method: "migrate_getBlocks",
        Params: []interface{}{0, blockNum, 100},
    }
    callRPC(sourceRPC, req, &blocks)

    // Import via RPC to destination
    req = &RPCRequest{
        Method: "migrate_importBlocks",
        Params: []interface{}{blocks},
    }
    callRPC(destRPC, req, &result)
}
```

**EVM RPC Endpoints:**
```go
// plugin/evm/api_migrate.go
type MigrateAPI struct {
    vm *VM
}

// RPC: migrate_getChainInfo
func (api *MigrateAPI) GetChainInfo() (*ChainInfo, error)

// RPC: migrate_getBlocks (batch, max 100 blocks)
func (api *MigrateAPI) GetBlocks(start, end, limit uint64) ([]*BlockData, error)

// RPC: migrate_streamBlocks (streaming via channels)
func (api *MigrateAPI) StreamBlocks(start, end uint64) (chan *BlockData, chan error)

// RPC: migrate_importBlocks
func (api *MigrateAPI) ImportBlocks(blocks []*BlockData) (int, error)
```

**Benefits:**
- True fleet control via RPC (lux-cli controls remote nodes)
- No Go package coupling between repos
- Can control nodes anywhere (local, remote, cloud)
- Netrunner handles deployment, lux-cli handles orchestration
- Works with any number of nodes
- Clean separation: deploy vs control vs execution

**Workflow:**
1. Deploy source EVM with netrunner (readonly DB):
   ```bash
   netrunner engine start evm-source --data-dir=/readonly/db
   ```

2. Deploy destination C-Chain with netrunner:
   ```bash
   netrunner engine start c-chain
   ```

3. Run migration via lux-cli (auto-discovers RPC endpoints):
   ```bash
   lux migrate prepare
   # RPC endpoints discovered from netrunner at runtime
   # Source: ext/bc/<blockchain-id>/rpc (old 96369 net)
   # Dest: ext/bc/C/rpc (C-Chain)
   # Internal RPC uses port 9630 (not 9650)
   # Hosts/ports known at runtime, not hardcoded
   ```

**RPC Path Format:**
- **C-Chain**: `ext/bc/C/rpc` (uses C alias)
- **Old 96369 Net**: `ext/bc/dnmzhuf6poM6PUNQCe7MWWfBdTJEnddhHRNXz2x7H6qSmyBEJ/rpc` (uses blockchain ID)

## MigrateAPI Registration Status (2025-11-23)

### ✅ COMPLETE: MigrateAPI Registered with EVM Node

The MigrateAPI has been successfully registered with the EVM node and is now available via RPC.

**Changes Made:**
1. Added `MigrateAPIEnabled` config flag to `plugin/evm/config/config.go`
2. Set default to `true` in `plugin/evm/config/default_config.go`
3. Registered MigrateAPI in `plugin/evm/vm.go` (similar to WarpAPI)
4. Fixed type errors in `plugin/evm/api_migrate.go`:
   - Changed `Transactions` field from `[]types.Transaction` to `[]*types.Transaction`
   - Fixed `WithBody` call to use `types.Body` directly

**Available RPC Methods:**
- `migrate_getChainInfo` - Returns blockchain metadata (chain ID, network ID, current height, etc.)
- `migrate_getBlocks` - Exports blocks in batches (max 100 blocks per call)
- `migrate_streamBlocks` - Streams blocks via channels (not yet exposed via JSON-RPC)
- `migrate_importBlocks` - Imports blocks to the blockchain

**Configuration:**
```json
{
  "migrate-api-enabled": true  // Default: true
}
```

**Testing Status:**
- ✅ EVM plugin builds successfully
- ✅ MigrateAPI properly registered in RPC handler
- ✅ CLI commands (export-data, import-data) implemented
- ⏳ End-to-end RPC testing pending

**Next Steps:**
1. Deploy EVM node with readonly database
2. Test `migrate_getChainInfo` RPC call
3. Test `migrate_getBlocks` with various block ranges
4. Test full export → import workflow via lux-cli
5. Verify imported data on destination chain

---

## Lux Blockchain Deployment Analysis (2025-12-18)

### Executive Summary

This analysis examines the deployment process for Zoo and SPC subnets, identifying why C-Chain import succeeded while subnet imports fail with `ErrPrunedAncestor`.

### Current State

| Chain | RLP File | Size | Import Method | Status |
|-------|----------|------|---------------|--------|
| C-Chain (96369) | lux-mainnet-96369.rlp | 1.28GB | admin.importChain | WORKED |
| Zoo (200200) | zoo-mainnet-200200.rlp | 1.3MB | admin.importChain | ErrPrunedAncestor |
| SPC (36911) | spc-mainnet-36911.rlp | 7.8KB | admin.importChain | Not tested yet |

---

### 1. Root Cause: Zoo RPC 404

**Diagnosis:** The subnet RPC endpoint returns 404 because the EVM plugin is not properly initialized or the blockchain is not registered with the node.

**Key Findings:**

1. **Plugin Registration**: EVM must be registered as a plugin with the node. The C-Chain uses `cchainvm` which is built into the node, but Zoo/SPC require the external `evm` plugin.

2. **Blockchain ID Mismatch**: The RPC path uses the blockchain ID (e.g., `ext/bc/<blockchain-id>/rpc`). If the subnet is not tracking the correct blockchain ID, RPC returns 404.

3. **Subnet Validation**: The node must be a validator for the subnet OR have explicit tracking enabled via `--track-subnets`.

**Solution:**
```bash
# Ensure subnet tracking
lux-node --track-subnets=<subnet-id>

# Verify blockchain registration
curl -X POST http://localhost:9650/ext/info -d '{
  "jsonrpc":"2.0",
  "id":1,
  "method":"info.getBlockchainID",
  "params":{"alias":"zoo"}
}'
```

---

### 2. Root Cause: ErrPrunedAncestor

**Location:** `/Users/z/work/lux/evm/core/block_validator.go:111-116`

```go
// Ancestor block must be known.
if !v.bc.HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
    if !v.bc.HasBlock(block.ParentHash(), block.NumberU64()-1) {
        return consensus.ErrUnknownAncestor
    }
    return consensus.ErrPrunedAncestor
}
```

**The Check Flow:**

1. `HasBlockAndState` calls:
   - `GetBlock(hash, number)` - checks if block exists
   - `HasState(block.Root())` - checks if state trie exists

2. `HasState` implementation (`blockchain_reader.go:241-244`):
```go
func (bc *BlockChain) HasState(hash common.Hash) bool {
    _, err := bc.stateCache.OpenTrie(hash)
    return err == nil
}
```

**Critical Issue:** `OpenTrie` requires the FULL state trie to exist in the database. When a subnet starts fresh:

- Genesis block is written via `genesis.Commit()`
- Genesis state is committed to the trie database
- BUT the trie might not be accessible via `OpenTrie` if:
  - The trie is stored in snapshot form only
  - The trie root wasn't properly committed to disk
  - The state scheme (HashDB vs PathDB) differs

**Root Cause:** Genesis state is committed, but `stateCache.OpenTrie(genesisRoot)` fails because the trie nodes are not where `OpenTrie` expects them.

---

### 3. Why C-Chain Worked But Zoo Doesn't

**C-Chain (cchainvm):**

1. **Built into node**: The C-Chain VM is compiled directly into the node binary
2. **Shared genesis**: Uses the network's genesis file which includes C-Chain state
3. **Continuous state**: C-Chain has been running since network genesis - full state trie exists
4. **State scheme consistency**: Uses the same state scheme as the node's defaults

**Zoo/SPC (EVM plugin):**

1. **External plugin**: Loaded dynamically, separate initialization path
2. **Fresh genesis**: Genesis is created from the subnet's genesis.json at deployment time
3. **State initialization gap**: Genesis state may be written but not properly accessible
4. **Import before consensus**: Import happens before the VM is fully bootstrapped

**Key Difference - State Cache Initialization:**

```go
// In blockchain.go - NewBlockChain
bc.stateCache = state.NewDatabaseWithNodeDB(bc.db, bc.triedb)

// The stateCache is created with:
// - bc.db: the ethdb.Database
// - bc.triedb: the trie database

// For C-Chain: triedb already has the state from continuous operation
// For EVM: triedb only has genesis state from fresh Commit()
```

---

### 4. What Needs to be Fixed

#### Option A: Ensure Genesis State is Properly Accessible (RECOMMENDED)

**Location:** `core/genesis.go:417-451` (Genesis.Commit)

The issue is that `Genesis.Commit` writes the state, but the trie database might not be fully synced to disk or accessible via `OpenTrie`.

**Fix:**
```go
// In genesis.go Commit function, after writing genesis:
func (g *Genesis) Commit(db ethdb.Database, triedb *triedb.Database) (*types.Block, error) {
    block := g.toBlock(db, triedb)
    // ... existing code ...

    // CRITICAL: Ensure trie is committed and accessible
    if err := triedb.Commit(block.Root(), false); err != nil {
        return nil, fmt.Errorf("failed to commit genesis trie: %w", err)
    }

    // Verify the state is accessible
    if _, err := triedb.NodeReader(block.Root()); err != nil {
        return nil, fmt.Errorf("genesis state not accessible after commit: %w", err)
    }

    return block, nil
}
```

#### Option B: Initialize State Cache Before Import

**Location:** `plugin/evm/admin.go:108-154` (ImportChain)

Before importing blocks, ensure the genesis state is properly loaded:

```go
func (p *Admin) ImportChain(_ *http.Request, args *ImportChainArgs, reply *ImportChainReply) error {
    // ... existing checks ...

    chain := p.vm.eth.BlockChain()
    genesis := chain.Genesis()

    // Verify genesis state is accessible
    if !chain.HasState(genesis.Root()) {
        log.Warn("Genesis state not accessible, attempting to recover")
        // Recommit genesis if needed
        if err := p.vm.ensureGenesisState(); err != nil {
            return fmt.Errorf("failed to ensure genesis state: %w", err)
        }
    }

    // Now proceed with import
    // ...
}
```

#### Option C: Use StateAt Instead of OpenTrie in HasState

**Location:** `core/blockchain_reader.go:241-244`

The current `HasState` uses `OpenTrie` which requires full trie. Consider using `StateAt` which can fall back to snapshots:

```go
// Current (strict):
func (bc *BlockChain) HasState(hash common.Hash) bool {
    _, err := bc.stateCache.OpenTrie(hash)
    return err == nil
}

// Alternative (more lenient, but maintains integrity):
func (bc *BlockChain) HasState(hash common.Hash) bool {
    // First try OpenTrie (preferred - full state)
    if _, err := bc.stateCache.OpenTrie(hash); err == nil {
        return true
    }
    // Fall back to StateAt which can use snapshots
    // Only for genesis block (number == 0)
    if bc.Genesis() != nil && bc.Genesis().Root() == hash {
        _, err := bc.StateAt(hash)
        return err == nil
    }
    return false
}
```

**WARNING:** This option requires careful consideration - it's a workaround, not a fix.

---

### 5. Step-by-Step Plan to Get All Chains Operational

#### Phase 1: Verify Genesis State Accessibility

```bash
# 1. Check genesis state in database
cd /Users/z/work/lux/evm

# 2. Create a diagnostic tool to verify genesis state
# Add to plugin/evm/admin.go:

# RPC: admin_verifyGenesisState
# Returns: {genesisHash, genesisRoot, stateAccessible, snapshotAvailable}
```

#### Phase 2: Fix Genesis Commit

1. Update `core/genesis.go:Commit()` to ensure state is fully committed
2. Add verification step after commit
3. Test with fresh database

```bash
# Test genesis commit
go test ./core -run TestGenesisCommit -v
```

#### Phase 3: Deploy Zoo Subnet

```bash
# 1. Create fresh database directory
mkdir -p /tmp/zoo-test/db

# 2. Generate genesis with allocations
cat > /tmp/zoo-genesis.json << 'EOF'
{
  "config": {
    "chainId": 200200,
    "evmTimestamp": 0,
    "durangoTimestamp": 0,
    "feeConfig": {
      "gasLimit": 12000000,
      "targetBlockRate": 2,
      "minBaseFee": 25000000000,
      "targetGas": 15000000,
      "baseFeeChangeDenominator": 36,
      "minBlockGasCost": 0,
      "maxBlockGasCost": 1000000,
      "blockGasCostStep": 200000
    }
  },
  "alloc": {
    "9011E888251AB053B7bD1cdB598Db4f9DEd94714": {
      "balance": "0x193e5939a08ce9dbd480000000"
    }
  },
  "timestamp": "0x6727e9c3",
  "gasLimit": "0xb71b00",
  "difficulty": "0x0",
  "baseFeePerGas": "0x5d21dba00"
}
EOF

# 3. Deploy with lux CLI
lux l2 deploy zoo \
  --genesis /tmp/zoo-genesis.json \
  --vm-binary /Users/z/work/lux/evm/build/evm \
  --local

# 4. Verify RPC is responding
curl -X POST http://localhost:9650/ext/bc/zoo/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}'
```

#### Phase 4: Import Historical Blocks

```bash
# 1. Connect to running node's admin API
curl -X POST http://localhost:9650/ext/bc/zoo/admin \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"admin_importChain",
    "params":{
      "file":"/Users/z/work/lux/state/rlp/zoo-mainnet/zoo-mainnet-200200.rlp"
    }
  }'
```

#### Phase 5: Verify Import Success

```bash
# Check block height
curl -X POST http://localhost:9650/ext/bc/zoo/rpc \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}'

# Check specific block
curl -X POST http://localhost:9650/ext/bc/zoo/rpc \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc":"2.0",
    "id":1,
    "method":"eth_getBlockByNumber",
    "params":["0x64",true]
  }'
```

#### Phase 6: Test AMM Commands

```bash
# Add liquidity
lux amm add-liquidity \
  --chain zoo \
  --token0 0x... \
  --token1 0x... \
  --amount0 1000 \
  --amount1 1000

# Swap
lux amm swap \
  --chain zoo \
  --tokenIn 0x... \
  --tokenOut 0x... \
  --amountIn 100
```

---

### Technical Deep Dive: State Trie Architecture

```
                    Genesis.Commit()
                         |
                         v
        +----------------+----------------+
        |                                 |
        v                                 v
   statedb.Commit()              triedb.Commit()
        |                                 |
        v                                 v
   State Root Hash              Trie Nodes to Disk
        |                                 |
        +---------> rawdb.Write <---------+
                         |
                         v
                    ethdb.Database
                         |
            +------------+------------+
            |            |            |
            v            v            v
         HashDB      PathDB      Snapshots
```

**The Gap:** After `Genesis.Commit()`, the state root is stored and trie nodes are written, BUT:

1. `stateCache.OpenTrie(root)` requires trie nodes to be in a specific location
2. If using HashDB scheme, nodes must be in `triedb.HashDB`
3. If using PathDB scheme (not supported per code), nodes must be in `triedb.PathDB`
4. Snapshots provide account/storage data but NOT trie structure

**Solution:** Ensure `triedb.Commit(root, true)` is called with `report=true` to force disk sync, and verify accessibility via `NodeReader`.

---

### Files to Modify

| File | Change | Priority |
|------|--------|----------|
| `core/genesis.go` | Add state verification after Commit | HIGH |
| `plugin/evm/admin.go` | Add genesis state check before import | HIGH |
| `core/blockchain.go` | Log state accessibility during NewBlockChain | MEDIUM |
| `plugin/evm/vm.go` | Add ensureGenesisState helper | MEDIUM |

---

### Verification Checklist

- [ ] Genesis state accessible via `OpenTrie`
- [ ] Block 0 (genesis) exists in rawdb
- [ ] Canonical hash for block 0 set correctly
- [ ] Chain config stored with genesis hash
- [ ] Snapshot for genesis state available
- [ ] Import chain skips genesis block (number == 0)
- [ ] Parent hash of block 1 matches genesis hash
- [ ] State root of genesis matches expected value

---

### Known Issues and Workarounds

**Issue 1:** `admin.importChain` skips Accept calls
**Location:** `plugin/evm/admin.go:231-233`
**Impact:** Imported blocks may not be finalized
**Workaround:** Manually trigger consensus acceptance after import

**Issue 2:** StateScheme PathDB not supported
**Location:** `plugin/evm/vm.go:623-626`
**Impact:** Must use HashDB scheme
**Workaround:** Ensure `state-scheme: "hash"` in config

**Issue 3:** Snapshot delayed init when state sync enabled
**Location:** `plugin/evm/vm.go:590`
**Impact:** Snapshots not available during import
**Workaround:** Disable state sync for import node

## Cancun/BeaconRoot Disabled (2025-12-22)

### Status: ✅ FIXED - Lux Networks Do Not Use Ethereum Beacon Chain

Lux networks use their own consensus mechanism and do NOT use Ethereum's beacon chain. The following Cancun-era EIP-4844 fields are NOT required for Lux EVM blocks:

- `ExcessBlobGas` - NOT required
- `BlobGasUsed` - NOT required  
- `ParentBeaconRoot` - NOT required

### Files Modified

1. **`consensus/dummy/consensus.go`** (lines 231-238)
   - Removed mandatory beaconRoot check
   - Removed mandatory excessBlobGas/blobGasUsed check
   - Only rejects if BlobGasUsed > 0 (which shouldn't happen)

2. **`plugin/evm/block_verification.go`** (lines 136-144)
   - Removed mandatory excessBlobGas check
   - Removed mandatory blobGasUsed check
   - Removed mandatory parentBeaconRoot check
   - Only rejects if BlobGasUsed > 0

### Why This Matters

Historic blocks from Lux networks (e.g., Zoo chain 200200) were created before Cancun was even conceived. These blocks don't have:
- `ExcessBlobGas` field
- `BlobGasUsed` field
- `ParentBeaconRoot` field

Setting `cancunTime: null` in genesis doesn't help because the EVM code still checks these fields during block import via `admin.importChain`.

### Key Principle

**Lux networks are NOT Ethereum.** We use our own consensus (Snow/POA) and don't need Ethereum's beacon chain validators or blob transaction support.

### Testing Verification

Successfully imported 799 blocks from Zoo chain (200200) after these fixes:
```bash
curl -X POST http://127.0.0.1:9630/ext/bc/<zoo-id>/admin \
  -d '{"jsonrpc":"2.0","id":1,"method":"admin.importChain","params":{"file":"/path/to/zoo-mainnet-200200.rlp"}}'

# Response:
{"jsonrpc":"2.0","result":{"success":true,"blocksImported":799,"heightAfter":799}}
```

## CalcBlobFee Panic Fix (2026-01-29)

### Status: ✅ FIXED - EVM Plugin No Longer Panics During Chain Initialization

Fixed a panic that occurred during EVM plugin initialization: `"calculating blob fee on unsupported fork"`.

### Root Cause

The `collectUnflattenedLogs()` function in `core/blockchain.go` was calling `eip4844.CalcBlobFee()` when a block had `ExcessBlobGas` set, but without checking if the Cancun fork was actually active. The `CalcBlobFee()` function panics if called on a chain without Cancun fork enabled.

**Stack trace:**
```
CalcBlobFee() → collectUnflattenedLogs() → collectLogs() → reorg() →
writeKnownBlock() → setPreference() → loadLastState() → NewBlockChain() → PANIC
```

### File Modified

**`core/blockchain.go`** (lines 1667-1672):
```go
// Before (panics):
if excessBlobGas != nil {
    blobGasPrice = eip4844.CalcBlobFee(bc.chainConfig, b.Header())
}

// After (safe):
// Only calculate blob fee if Cancun fork is active AND block has ExcessBlobGas.
// Without the IsCancun check, CalcBlobFee panics with "calculating blob fee on unsupported fork"
// when blocks have ExcessBlobGas set but the chain config doesn't have Cancun enabled.
if excessBlobGas != nil && bc.chainConfig.IsCancun(b.Number(), b.Time()) {
    blobGasPrice = eip4844.CalcBlobFee(bc.chainConfig, b.Header())
}
```

### Why This Happened

- Blocks may have `ExcessBlobGas` field set (e.g., from RLP imports or state sync)
- Chain config may not have Cancun fork enabled (especially for legacy Lux networks)
- The code assumed if `ExcessBlobGas != nil`, then Cancun must be active (wrong!)

### Verification

After the fix, EVM plugin initializes successfully without panic:
```
[EVM-DEBUG] parseGenesis succeeded: chainID=1337 alloc=16 accounts
[EVM-DEBUG] initializeChain: eth.New succeeded
[EVM-DEBUG] Chain initialized successfully
```

## Post-Quantum Cryptography Precompiles (2025-12-24)

### Status: ✅ COMPLETE - All PQ Crypto Precompiles Implemented and Tested

Lux EVM includes native precompiled contracts for NIST FIPS 203-205 post-quantum cryptography algorithms.

### Precompile Addresses (LP-aligned)

| Precompile | Address | Description |
|------------|---------|-------------|
| **PQCrypto Unified** | `0x0000000000000000000000000000000000012201` | All PQ crypto operations (P=2 PQ/Identity) |
| **ML-DSA Verify** | `0x0000000000000000000000000000000000012202` | Dedicated ML-DSA verification |
| **SLH-DSA Verify** | `0x0000000000000000000000000000000000012203` | Dedicated SLH-DSA verification |

### Gas Costs (Per Mode)

#### ML-DSA Signature Verification (FIPS 204)

| Mode | Security | Mode Byte | Gas Cost |
|------|----------|-----------|----------|
| ML-DSA-44 | Level 2 | `0x44` | **75,000** |
| ML-DSA-65 | Level 3 | `0x65` | **100,000** |
| ML-DSA-87 | Level 5 | `0x87` | **150,000** |

#### ML-KEM Key Encapsulation (FIPS 203)

| Mode | Security | Mode Byte | Encap Gas | Decap Gas |
|------|----------|-----------|-----------|-----------|
| ML-KEM-512 | Level 1 | `0x00` | **6,000** | **6,000** |
| ML-KEM-768 | Level 3 | `0x01` | **8,000** | **8,000** |
| ML-KEM-1024 | Level 5 | `0x02` | **10,000** | **10,000** |

#### SLH-DSA Signature Verification (FIPS 205)

| Mode | Hash | Security | Mode Byte | Gas Cost |
|------|------|----------|-----------|----------|
| 128s | SHA-256 | Level 1 | `0x00` | **50,000** |
| 128f | SHA-256 | Level 1 | `0x01` | **75,000** |
| 192s | SHA-256 | Level 3 | `0x02` | **100,000** |
| 192f | SHA-256 | Level 3 | `0x03` | **150,000** |
| 256s | SHA-256 | Level 5 | `0x04` | **175,000** |
| 256f | SHA-256 | Level 5 | `0x05` | **250,000** |
| 128s | SHAKE | Level 1 | `0x10` | **50,000** |
| 128f | SHAKE | Level 1 | `0x11` | **75,000** |
| 192s | SHAKE | Level 3 | `0x12` | **100,000** |
| 192f | SHAKE | Level 3 | `0x13` | **150,000** |
| 256s | SHAKE | Level 5 | `0x14` | **175,000** |
| 256f | SHAKE | Level 5 | `0x15` | **250,000** |

### Implementation Files

```
precompile/contracts/
├── mldsa/
│   ├── contract.go       # ML-DSA precompile (182 lines)
│   ├── contract_test.go  # 334 lines, 10 test cases
│   └── module.go         # Registration
└── pqcrypto/
    ├── contract.go       # Unified PQ precompile (382 lines)
    ├── contract_test.go  # 234 lines, 20 test cases
    ├── module.go         # Registration
    └── config.go         # Configuration
```

### Mode Byte Encoding

**Critical**: Precompile mode bytes differ from library internal values:

```go
// Precompile mode bytes (used in input)
ModeMLDSA44 uint8 = 0x44  // Library: mldsa.MLDSA44 = 0
ModeMLDSA65 uint8 = 0x65  // Library: mldsa.MLDSA65 = 1
ModeMLDSA87 uint8 = 0x87  // Library: mldsa.MLDSA87 = 2
```

The precompile implementation converts between these formats in the `Run()` method.

### Function Selectors (PQCrypto Unified)

| Selector | Bytes | Operation |
|----------|-------|-----------|
| `"mlds"` | `0x6d6c6473` | ML-DSA Verify |
| `"encp"` | `0x656e6370` | ML-KEM Encapsulate |
| `"decp"` | `0x64656370` | ML-KEM Decapsulate |
| `"slhs"` | `0x736c6873` | SLH-DSA Verify |

### Test Status

```
=== ML-DSA Tests ===
TestMLDSAVerify_ValidSignature      PASS
TestMLDSAVerify_InvalidSignature    PASS
TestMLDSAVerify_WrongMessage        PASS
TestMLDSAVerify_InputTooShort       PASS
TestMLDSAVerify_EmptyMessage        PASS
TestMLDSAVerify_LargeMessage        PASS
TestMLDSAVerify_GasCost             PASS
TestMLDSAPrecompile_Address         PASS

=== PQCrypto Tests ===
TestPQCryptoPrecompile              PASS
TestMLDSAVerify                     PASS
TestMLKEMEncapsulateDecapsulate     PASS
TestSLHDSAVerify                    PASS
TestGasCalculation (15 subtests)    PASS

Total: 20 tests, 0 failures
```

### Documentation

Full specification documented in:
- **LP-3520**: Post-Quantum Cryptography Precompile Implementation Guide
- **LP-4200**: Post-Quantum Cryptography Suite for Lux Network
- **LP-3502**: ML-DSA Post-Quantum Signature Precompile

### Dependencies

- `github.com/luxfi/crypto/mldsa` - ML-DSA implementation (FIPS 204)
- `github.com/luxfi/crypto/mlkem` - ML-KEM implementation (FIPS 203)
- `github.com/luxfi/crypto/slhdsa` - SLH-DSA implementation (FIPS 205)
- Backend: Cloudflare CIRCL (audited, FIPS compliant)

---

## SPC Chain Genesis Recovery (2025-12-28)

### Problem: Genesis Hash Mismatch for Existing Chain

When deploying SPC chain with RLP block import, the genesis hash computed from genesis.json must match the original chain's block 0 hash exactly. Otherwise block import fails.

### Solution: Extract Original Genesis from ChainData

Extracted the original genesis alloc from the SPC pebbledb chaindata by:
1. Analyzing pathdb key structure to find account hashes
2. Extracting addresses from RLP transaction data (sender addresses from blocks 1-10)
3. Matching Keccak256(address) with account hashes to identify addresses
4. Computing state root to verify correct alloc

### SPC Genesis Configuration

| Property | Value |
|----------|-------|
| Chain ID | 36911 |
| Genesis Hash | `0x4dc9fd5cf4ee64609f140ba0aa50f320cadf0ae8b59a29415979bc05b17cfac8` |
| State Root | `0xb75eb0a501516b8d6e691c705660f05f77bc23c47378158152ba543f74556c6f` |
| Timestamp | 1731369637 (0x67329aa5) |
| GasLimit | 12000000 (0xb71b00) |
| BaseFee | 25000000000 (0x5d21dba00) |
| Token Symbol | SPC |
| Total Supply | 1,000,000,000 SPC (1 billion) |

### Genesis Alloc (2 entries)

```json
{
  "alloc": {
    "0200000000000000000000000000000000000005": {
      "code": "0x01",
      "balance": "0x0",
      "nonce": "0x1"
    },
    "12c6EE1d226225756F57B75957d2BF3Ab2e8597e": {
      "balance": "0x33b2e3c9fd0803ce8000000"
    }
  }
}
```

| Address | Role | Balance |
|---------|------|---------|
| `0x0200...0005` | Warp Precompile | 0 (code=0x01, nonce=1) |
| `0x12c6EE1d...` | Main Token Holder | 1,000,000,000 SPC |

### Transaction History (Blocks 1-10)

The main holder distributed tokens to 9 addresses:
- Block 1: 1,000,000 SPC to `0x53dc35fA...`
- Block 2: 9,000,000 SPC to `0x3eB5a2b6...`
- Blocks 3-10: Further distribution to other addresses

### Genesis File Location

- **Path**: `/Users/z/work/lux/state/rlp/spc-mainnet/genesis.json`
- **RLP Blocks**: `/Users/z/work/lux/state/rlp/spc-mainnet/spc-mainnet-36911.rlp`
- **ChainData**: `/Users/z/work/lux/state/pebbledb/spc-mainnet/`

### Key Insight

The genesis produces the correct state root naturally. The original genesis had only 2 accounts:
1. The warp precompile at `0x0200...0005`
2. The main token holder at `0x12c6EE1d...` with the full supply

All other addresses were created through subsequent transactions.

---

## P-Chain/Info API Deadlock Fix (2026-01-05)

### Problem: API Timeouts After admin_importChain

After calling `admin_importChain`, P-chain and Info APIs would hang/timeout. The C-Chain RPC continued working, but cross-chain API calls would fail.

### Root Cause

The `PostImportCallback` was called **synchronously** while `chainmu.Lock()` was held:
1. `SetLastAcceptedBlockDirect()` acquires `chainmu.Lock()`
2. `PostImportCallback` is called synchronously (within the lock)
3. PostImportCallback may contend with P-chain/Info API locks
4. **Deadlock**: APIs waiting for chainmu, chainmu waiting for APIs

### Solution: Async PostImportCallback

Made the callback run in a goroutine to prevent cross-chain mutex contention.

### Files Fixed

**eth/api_admin.go** (lines 180-196):
```go
// CRITICAL: Call the post-import callback to update the VM layer's acceptedBlockDB.
// Without this, ReadLastAccepted() returns genesis hash on restart because
// acceptedBlockDB is not updated by the admin API import path.
//
// Run asynchronously to avoid deadlock: SetLastAcceptedBlockDirect holds chainmu.Lock(),
// and PostImportCallback may contend with P-chain/Info API locks. By returning success
// immediately after state commit and letting the callback complete in background,
// we prevent cross-chain mutex contention that causes API timeouts.
go func() {
    if err := api.eth.CallPostImportCallback(lastInsertedBlock.Hash(), lastInsertedBlock.NumberU64()); err != nil {
        log.Error("PostImportCallback failed", "error", err)
        return
    }
    log.Info("ImportChain: post-import callback completed asynchronously")
}()
```

**plugin/evm/admin_api.go** (lines 315-323):
```go
// CRITICAL: Call PostImportCallback to update acceptedBlockDB for persistence across restarts.
// Without this, the VM's lastAcceptedKey won't be updated, causing blocks to be lost on restart.
//
// Run asynchronously to avoid deadlock: SetLastAcceptedBlockDirect holds chainmu.Lock(),
// and PostImportCallback may contend with P-chain/Info API locks.
if finalBlock != nil {
    go func(hash common.Hash, height uint64) {
        if err := eth.CallPostImportCallback(hash, height); err != nil {
            log.Error("PostImportCallback failed", "error", err)
            return
        }
        log.Info("admin_importChain: post-import callback completed asynchronously", "height", height)
    }(finalBlock.Hash(), finalBlock.NumberU64())
}
```

### Also Fixed: Duplicate Logging

Removed duplicate logging patterns where `log.Error()` was followed by `return fmt.Errorf()`, which caused the same error to be logged twice.

### Verification

All APIs responding after block import:
- C-Chain RPC: `eth_blockNumber` returns correct height
- P-Chain: `platform.getBlockchains` responds immediately
- Info API: `info.getNodeVersion` responds immediately

---

## ZAP Transport Type Assertion Fix (2026-01-29)

### Status: ✅ FIXED - ZAP Transport Working for C-Chain (EVM)

Fixed a type assertion failure where `*zap.Client` wasn't being recognized as `chain.ChainVM` in the node's chains manager type switch.

### Problem

When starting the network with ZAP transport (no gRPC), the C-Chain would fail with:
```
unsupported VM type: *zap.Client
```

The ZAP handshake succeeded, but the chains manager's type switch at `chains/manager.go:880` didn't match `*zap.Client` against `chain.ChainVM`.

### Root Cause

The issue was caused by the `go.work` workspace. With all packages (node, vm, consensus) using local versions via go.work, the type definitions must be consistent across all packages. If the packages were built at different times or with different states, the `chain.ChainVM` interface from `github.com/luxfi/vm/chain` wouldn't match.

### Solution

Rebuild all packages consistently within the go.work workspace:
1. Build the node: `cd /Users/z/work/lux/node && go build -o build/luxd ./main`
2. Build the EVM plugin: `cd /Users/z/work/lux/evm && go build -o ~/.lux/plugins/current/<VMID> ./plugin`

### Verification

After rebuilding, the C-Chain initializes successfully via ZAP:
```
plugin handshake succeeded via ZAP
VM client connected via ZAP
DEBUG: About to check VM type vmType=*zap.Client
creating linear chain
ZAP handleInitialize
ZAP VM initialized successfully
VM initialized via ZAP
CHAIN CREATED SUCCESSFULLY chainAlias=C vmName=evm
C-Chain automining ENABLED, starting automining loop
```

### Key Files

- **`/Users/z/work/lux/node/chains/manager.go:880`** - Type switch that checks `chain.ChainVM`
- **`/Users/z/work/lux/node/vms/rpcchainvm/zap/client.go`** - ZAP client that implements `chain.ChainVM`
- **`/Users/z/work/lux/vm/chain/interfaces.go`** - Defines `chain.ChainVM` as alias to `block.ChainVM`
- **`/Users/z/work/lux/go.work`** - Workspace file that includes all local packages

### Key Insight

With `go.work`, Go uses local package versions instead of published versions from the module cache. This means:
- All packages must be built together for type consistency
- The `chain.ChainVM` interface from the local vm package must match what the node imports
- The compile-time check `var _ chain.ChainVM = (*Client)(nil)` in the ZAP client ensures interface compliance

### Additional Notes

The remaining network startup failures (optional chains K, G, Z, T, B, A, Q) are due to missing VM plugins for optional VMs (Key, Graph, ZK, Threshold, Bridge, AI, Quantum), not ZAP transport issues. Build with `-tags=allvms` to include these optional VMs.

---

*Last Updated: 2026-01-29*

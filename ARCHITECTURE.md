# Lux EVM Architecture Guidelines

This document outlines the architectural principles and implementation guidelines for the Lux EVM codebase.

## 1. Module Architecture

### Core Modules
```
evm/
├── core/           # State, consensus, block/tx formats (no external deps)
├── interfaces/     # Minimal interface definitions
├── common/         # Shared types and utilities
├── plugin/         # Pluggable components
│   ├── db/         # Database backends
│   ├── sync/       # State sync implementations
│   └── validator/  # Block validators
├── rpc/           # RPC layer
├── node/          # Node orchestration
└── cli/           # Command-line interface
```

### Dependency Rules
- **Core** → interfaces only
- **Plugin** → core, interfaces
- **Node/RPC/CLI** → core, interfaces, plugin
- **No upward imports**: Core must never import from plugin, node, rpc, or cli

## 2. Interface Design

### Example: ChainReader Interface
```go
// interfaces/blockchain.go
package interfaces

type ChainReader interface {
    Config() ChainConfig
    CurrentHeader() *types.Header
    GetHeader(hash common.Hash, number uint64) *types.Header
}

type ChainConfig interface {
    GetChainID() *big.Int
    IsActive(fork ForkID, blockNum *big.Int, timestamp uint64) bool
}
```

### Adapter Pattern
```go
// adapters/geth_adapter.go
package adapters

type GethChainConfigAdapter struct {
    *gethparams.ChainConfig
}

func (g *GethChainConfigAdapter) IsActive(fork ForkID, blockNum *big.Int, timestamp uint64) bool {
    switch fork {
    case ForkShanghai:
        return g.IsShanghai(blockNum, timestamp)
    // ... other forks
    }
}
```

## 3. Fork Management

### Centralized Fork Registry
```go
// common/forks/registry.go
package forks

type ForkID string

const (
    ForkHomestead ForkID = "homestead"
    ForkByzantium ForkID = "byzantium"
    ForkShanghai  ForkID = "shanghai"
    ForkCancun    ForkID = "cancun"
    // Lux-specific
    ForkEVM       ForkID = "evm"
    ForkDurango   ForkID = "durango"
    ForkEtna      ForkID = "etna"
    ForkFortuna   ForkID = "fortuna"
    ForkGranite   ForkID = "granite"
)

type Fork struct {
    ID          ForkID
    Block       *big.Int // nil for time-based
    Timestamp   *uint64  // nil for block-based
    AlwaysActive bool    // true for historical forks
}

var DefaultForks = []Fork{
    // All historical forks enabled from genesis
    {ID: ForkHomestead, AlwaysActive: true},
    {ID: ForkByzantium, AlwaysActive: true},
    {ID: ForkShanghai, AlwaysActive: true},
    {ID: ForkCancun, AlwaysActive: true},
    {ID: ForkEVM, AlwaysActive: true},
    {ID: ForkDurango, AlwaysActive: true},
    {ID: ForkEtna, AlwaysActive: true},
    {ID: ForkFortuna, AlwaysActive: true},
    {ID: ForkGranite, AlwaysActive: true},
}

// Usage throughout codebase:
// if cfg.IsActive(forks.ForkCancun, blockNum, timestamp) { ... }
```

## 4. Database Abstraction

### Clean DB Interface
```go
// interfaces/database.go
package interfaces

type Database interface {
    Get(key []byte) ([]byte, error)
    Put(key []byte, value []byte) error
    Delete(key []byte) error
    NewBatch() Batch
    Close() error
}

type Batch interface {
    Put(key []byte, value []byte) error
    Delete(key []byte) error
    Write() error
    Reset()
}
```

### Plugin Registration
```go
// plugin/db/registry.go
package db

var drivers = make(map[string]func(config.DB) (interfaces.Database, error))

func Register(name string, driver func(config.DB) (interfaces.Database, error)) {
    drivers[name] = driver
}

func Open(name string, config config.DB) (interfaces.Database, error) {
    driver, ok := drivers[name]
    if !ok {
        return nil, fmt.Errorf("unknown database driver: %s", name)
    }
    return driver(config)
}
```

## 5. Precompile Architecture

### Clean Precompile Interface
```go
// interfaces/precompile.go
package interfaces

type PrecompileContract interface {
    Address() common.Address
    RequiredGas(input []byte) uint64
    Run(input []byte, caller common.Address, state StateDB, readOnly bool) ([]byte, error)
}

type PrecompileRegistry interface {
    Register(contract PrecompileContract)
    Get(addr common.Address) (PrecompileContract, bool)
    IsActive(addr common.Address, rules ChainRules) bool
}
```

## 6. Build Configuration

### Single Build Command
```makefile
# Makefile
.DEFAULT_GOAL := build

# Single canonical build
build:
	go build -tags="sqlite,rocksdb" -ldflags "-X main.version=$(VERSION)" ./...

# Test with coverage
test:
	go test -v -cover ./...

# Lint
lint:
	golangci-lint run

# All CI checks
ci: lint test build
```

### CI Configuration
```yaml
# .github/workflows/ci.yml
name: CI
on: [push, pull_request]

jobs:
  test:
    strategy:
      matrix:
        go-version: [1.21.x, 1.22.x]
        db-backend: [sqlite, rocksdb, leveldb]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v4
        with:
          go-version: ${{ matrix.go-version }}
      - run: make ci DB_BACKEND=${{ matrix.db-backend }}
```

## 7. Code Generation

### Interface Mocks
```go
//go:generate moq -out mocks/chain_reader_mock.go -pkg mocks . ChainReader
//go:generate moq -out mocks/state_db_mock.go -pkg mocks . StateDB
```

### Delegate Pattern
```go
//go:generate delegate -type ChainConfig -from github.com/luxfi/geth/params.ChainConfig -to wrapped
```

## 8. Metrics & Observability

### Standardized Metrics
```go
// common/metrics/registry.go
package metrics

type Counter interface {
    Inc(delta int64)
    Get() int64
}

type Timer interface {
    UpdateSince(start time.Time)
}

var (
    blockImportTime = NewTimer("block_import_time")
    txProcessed     = NewCounter("tx_processed_total")
    stateReads      = NewCounter("state_reads_total")
)
```

## 9. Migration Path

### Phase 1: Structure (Week 1-2)
- [ ] Create interfaces/ package with minimal types
- [ ] Move shared types to common/
- [ ] Create plugin/ directory structure

### Phase 2: Decouple Core (Week 3-4)
- [ ] Remove precompile imports from core
- [ ] Move fork logic to centralized registry
- [ ] Create adapter layer for geth types

### Phase 3: Simplify Build (Week 5)
- [ ] Remove unused build tags
- [ ] Standardize on single build command
- [ ] Update CI configuration

### Phase 4: Enhance (Week 6+)
- [ ] Add comprehensive metrics
- [ ] Implement code generation
- [ ] Write architecture tests

## 10. Architecture Tests

### Enforce Module Boundaries
```go
// architecture_test.go
package evm_test

import (
    "go/build"
    "testing"
)

func TestNoCoreImportsFromPlugins(t *testing.T) {
    pkg, err := build.Import("github.com/luxfi/evm/core", "", 0)
    require.NoError(t, err)
    
    for _, imp := range pkg.Imports {
        assert.NotContains(t, imp, "plugin/")
        assert.NotContains(t, imp, "node/")
        assert.NotContains(t, imp, "rpc/")
    }
}
```

## Best Practices

1. **Interface Segregation**: Keep interfaces small and focused
2. **Dependency Injection**: Pass interfaces, not concrete types
3. **Feature Flags**: Use config, not code, for toggles
4. **Documentation**: Every package must have a README
5. **Benchmarks**: Track performance of critical paths
6. **Error Handling**: Wrap errors with context
7. **Logging**: Structured logging with context

## Conclusion

By following these guidelines, we achieve:
- Clean module boundaries
- No import cycles
- Easy testing and mocking
- Simple build process
- Clear upgrade path
- Better performance visibility

This architecture supports rapid iteration while maintaining code quality and performance.
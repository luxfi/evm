# Lux EVM

[![Releases](https://img.shields.io/github/v/tag/luxfi/evm.svg?sort=semver)](https://github.com/luxfi/evm/releases)
[![CI](https://github.com/luxfi/evm/actions/workflows/ci.yml/badge.svg)](https://github.com/luxfi/evm/actions/workflows/ci.yml)
[![CodeQL](https://github.com/luxfi/evm/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/luxfi/evm/actions/workflows/codeql-analysis.yml)
[![License](https://img.shields.io/github/license/luxfi/evm)](https://github.com/luxfi/evm/blob/master/LICENSE)

[Lux](https://docs.lux.network/lux-l1s) is a network composed of multiple blockchains.
Each blockchain is an instance of a Virtual Machine (VM), much like an object in an object-oriented language is an instance of a class.
That is, the VM defines the behavior of the blockchain.

Lux EVM is the unified [Virtual Machine (VM)](https://docs.lux.network/learn/virtual-machines) that powers both the C-Chain and L1/L2 Subnet EVM chains. This package consolidates the former Coreth (C-Chain) and Subnet EVM codebases into a single implementation.

> **Note**: The `github.com/luxfi/coreth` repository has been deprecated and merged here. See the [migration guide](#migrating-from-coreth) below.

This chain implements the Ethereum Virtual Machine and supports Solidity smart contracts as well as most other Ethereum client functionality.

## Building

The Subnet EVM runs in a separate process from the main Luxd process and communicates with it over a local gRPC connection.

### Luxd Compatibility

```text
[v0.7.0] Luxd@v1.12.0-v1.12.1 (Protocol Version: 38)
[v0.7.1] Luxd@v1.12.2 (Protocol Version: 39)
[v0.7.2] Luxd@v1.12.2/1.13.0-testnet (Protocol Version: 39)
[v0.7.3] Luxd@v1.12.2/1.13.0 (Protocol Version: 39)
[v0.7.4] Luxd@v1.13.1 (Protocol Version: 40)
[v0.7.5] Luxd@v1.13.2 (Protocol Version: 41)
[v0.7.6] Luxd@v1.13.3-rc.2 (Protocol Version: 42)
[v0.7.7] Luxd@v1.13.3 (Protocol Version: 42)
```

## API

The Subnet EVM supports the following API namespaces:

- `eth`
- `personal`
- `txpool`
- `debug`

Only the `eth` namespace is enabled by default.
Full documentation for the C-Chain's API can be found [here](https://build.lux.network/docs/api-reference/c-chain/api).

## Compatibility

The Subnet EVM is compatible with almost all Ethereum tooling, including [Remix](https://docs.lux.network/build/dapp/smart-contracts/remix-deploy), [Metamask](https://docs.lux.network/build/dapp/chain-settings), and [Foundry](https://docs.lux.network/build/dapp/smart-contracts/toolchains/foundry).

## C-Chain vs Subnet EVM Mode

This unified EVM package supports both modes:

**C-Chain Mode** (Primary Network):
- Atomic transactions for cross-chain transfers (X-Chain ↔ C-Chain)
- Shared memory integration
- Primary network consensus

**Subnet EVM Mode** (L1/L2 Subnets):
- Configurable fees and gas limits in genesis
- Simplified hardfork configuration
- No atomic transactions (cross-chain via Warp messaging)

## Migrating from Coreth

If you were using `github.com/luxfi/coreth`, update your imports:

```go
// Before
import "github.com/luxfi/coreth/core"

// After
import "github.com/luxfi/evm/core"
```

All coreth functionality is preserved in this package.

## Block Format

To support these changes, there have been a number of changes to the EVM block format compared to what exists on the C-Chain and Ethereum. Here we list the changes to the block format as compared to Ethereum.

### Block Header

- `BaseFee`: Added by EIP-1559 to represent the base fee of the block (present in Ethereum as of EIP-1559)
- `BlockGasCost`: surcharge for producing a block faster than the target rate

## Create an EVM Subnet on a Local Network

### Clone EVM

First install Go 1.23.9 or later. Follow the instructions [here](https://go.dev/doc/install). You can verify by running `go version`.

Set `$GOPATH` environment variable properly for Go to look for Go Workspaces. Please read [this](https://go.dev/doc/code) for details. You can verify by running `echo $GOPATH`.

As a few software will be installed into `$GOPATH/bin`, please make sure that `$GOPATH/bin` is in your `$PATH`, otherwise, you may get error running the commands below.

Download the `evm` repository into your `$GOPATH`:

```sh
cd $GOPATH
mkdir -p src/github.com/luxfi
cd src/github.com/luxfi
git clone git@github.com:luxfi/evm.git
cd evm
```

This will clone and checkout to `master` branch.

### Run Local Network

To run a local network, it is recommended to use the [lux-cli](https://github.com/luxfi/lux-cli#lux-cli) to set up an instance of EVM on a local Lux Network.

There are two options when using the Lux-CLI:

1. Use an official EVM release: <https://docs.lux.network/subnets/build-first-subnet>
2. Build and deploy a locally built (and optionally modified) version of EVM: <https://docs.lux.network/subnets/create-custom-subnet>

## Releasing

You can refer to the [`docs/releasing/README.md`](docs/releasing/README.md) file for the release process.

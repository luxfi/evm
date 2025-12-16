// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// evm-node is a standalone EVM node that can run without the Lux node infrastructure
package main

import (
	"fmt"
	"os"

	"github.com/luxfi/evm/cmd/evm-node/chaincmd"
	"github.com/luxfi/geth/cmd/utils"
	"github.com/luxfi/geth/log"
	"github.com/urfave/cli/v2"
)

const clientIdentifier = "evm-node"

var (
	app = &cli.App{
		Name:    clientIdentifier,
		Usage:   "Lux EVM node - blockchain import/export and initialization",
		Version: "1.0.0",
	}
)

func init() {
	app.Action = runNode
	app.Commands = []*cli.Command{
		chaincmd.InitCommand,
		chaincmd.ExportCommand,
		chaincmd.ImportCommand,
		chaincmd.CopyGenesisCommand,
		chaincmd.JSONLToRLPCommand,
		chaincmd.RegenesisCommand,
	}

	app.Flags = utils.DatabaseFlags

	app.Before = func(ctx *cli.Context) error {
		log.SetDefault(log.NewLogger(log.NewTerminalHandlerWithLevel(os.Stderr, log.LevelInfo, true)))
		return nil
	}
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runNode(ctx *cli.Context) error {
	fmt.Println("EVM node - Lux EVM blockchain tool")
	fmt.Println("")
	fmt.Println("Commands:")
	fmt.Println("  init         Initialize genesis block from SubnetEVM genesis file")
	fmt.Println("  export       Export blocks from SubnetEVM pebbledb to RLP file")
	fmt.Println("  import       Import blocks from RLP file with full transaction replay")
	fmt.Println("  copy-genesis Copy genesis state from SubnetEVM database")
	fmt.Println("  jsonl-to-rlp Convert JSONL block export to RLP format")
	fmt.Println("  regenesis    Disaster recovery: initialize genesis + replay transactions")
	fmt.Println("")
	fmt.Println("Workflow for Zoo chain disaster recovery:")
	fmt.Println("  1. evm-node jsonl-to-rlp blocks.jsonl blocks.rlp")
	fmt.Println("  2. evm-node regenesis --datadir /new/db genesis.json blocks.rlp")
	return nil
}

// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"

	"github.com/luxfi/evm/plugin/evm"
	"github.com/luxfi/evm/plugin/runner"
	"github.com/luxfi/node/version"
)

func main() {
	versionString := fmt.Sprintf("Lux-EVM/%s [node=%s, rpcchainvm=%d]", evm.Version, version.Current, version.RPCChainVMProtocol)
	runner.Run(versionString)
}

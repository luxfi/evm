// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"

	"github.com/luxfi/node/v2/version"
	"github.com/luxfi/evm/v2/plugin/evm"
	"github.com/luxfi/evm/v2/plugin/runner"
)

func main() {
	versionString := fmt.Sprintf("EVM/%s [Lux=%s, rpcchainvm=%d]", evm.Version, version.Current, version.RPCChainVMProtocol)
	runner.Run(versionString)
}

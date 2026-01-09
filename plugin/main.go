// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"os"

	"github.com/luxfi/evm/plugin/evm"
	"github.com/luxfi/evm/plugin/runner"
	"github.com/luxfi/version"
)

func init() {
	// Debug: log that init() is called - this runs before main()
	if f, err := os.OpenFile("/tmp/evm_init.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		fmt.Fprintf(f, "[%d] init() started\n", os.Getpid())
		f.Close()
	}
}

func main() {
	// Debug: log that main() is called
	if f, err := os.OpenFile("/tmp/evm_main.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		fmt.Fprintf(f, "[%d] main() started\n", os.Getpid())
		f.Close()
	}
	versionString := fmt.Sprintf("Lux-EVM/%s [node=%s, rpcchainvm=%d]", evm.Version, version.Current, version.RPCChainVMProtocol)
	runner.Run(versionString)
}

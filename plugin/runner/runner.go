// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/luxfi/log"
	"github.com/luxfi/vm/vms/rpcchainvm"
	"github.com/luxfi/vm/utils/ulimit"

	"github.com/luxfi/evm/plugin/evm"
)

func Run(versionStr string) {
	// Debug logging to file
	debugLog := func(msg string) {
		if f, err := os.OpenFile("/tmp/evm_runner.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			fmt.Fprintf(f, "[%d] %s\n", os.Getpid(), msg)
			f.Close()
		}
	}
	debugLog("Run() called with version: " + versionStr)

	printVersion, err := PrintVersion()
	if err != nil {
		debugLog("PrintVersion error: " + err.Error())
		fmt.Printf("couldn't get config: %s", err)
		os.Exit(1)
	}
	debugLog(fmt.Sprintf("printVersion=%v", printVersion))
	if printVersion && versionStr != "" {
		fmt.Println(versionStr)
		os.Exit(0)
	}
	debugLog("Setting ulimit")
	if err := ulimit.Set(ulimit.DefaultFDLimit, log.Root()); err != nil {
		debugLog("ulimit error: " + err.Error())
		fmt.Printf("failed to set fd limit correctly due to: %s", err)
		os.Exit(1)
	}
	debugLog("Calling rpcchainvm.Serve")
	if err := rpcchainvm.Serve(context.Background(), log.Root(), &evm.VM{}); err != nil {
		debugLog("rpcchainvm.Serve error: " + err.Error())
		fmt.Printf("rpcchainvm.Serve error: %s\n", err)
		os.Exit(1)
	}
	debugLog("rpcchainvm.Serve returned")
}

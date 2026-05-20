// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package runner

import (
	"context"
	"fmt"
	"os"

	log "github.com/luxfi/log"
	"github.com/luxfi/sys/ulimit"
	"github.com/luxfi/vm/rpc"

	"github.com/luxfi/evm/plugin/evm"
)

func Run(versionStr string) {
	printVersion, err := PrintVersion()
	if err != nil {
		fmt.Printf("couldn't get config: %s", err)
		os.Exit(1)
	}
	if printVersion && versionStr != "" {
		fmt.Println(versionStr)
		os.Exit(0)
	}
	if err := ulimit.Set(ulimit.DefaultFDLimit, log.Root()); err != nil {
		fmt.Printf("failed to set fd limit correctly due to: %s", err)
		os.Exit(1)
	}
	if err := rpc.Serve(context.Background(), log.Root(), &evm.VM{}); err != nil {
		fmt.Printf("rpc.Serve error: %s\n", err)
		os.Exit(1)
	}
}

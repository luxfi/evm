// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import "fmt"

var (
	// GitCommit is set by the build script
	GitCommit string
	// Version is the version of Lux EVM (set by build script via ldflags)
	Version string = "v0.16.9"
)

func init() {
	if len(GitCommit) != 0 {
		Version = fmt.Sprintf("%s@%s", Version, GitCommit)
	}
}

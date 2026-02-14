// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import "github.com/luxfi/vm"

// VMState is the canonical VM lifecycle state from github.com/luxfi/vm.
type VMState = vm.State

// Re-export canonical state constants from github.com/luxfi/vm.
// One and only one definition — these are aliases, not copies.
const (
	VMUnknown       = vm.Unknown       // 0
	VMStarting      = vm.Starting      // 1
	VMStateSyncing  = vm.Syncing       // 2
	VMBootstrapping = vm.Bootstrapping // 3
	VMNormalOp      = vm.Ready         // 4
	VMDegraded      = vm.Degraded      // 5
	VMStopping      = vm.Stopping      // 6
	VMStopped       = vm.Stopped       // 7
)

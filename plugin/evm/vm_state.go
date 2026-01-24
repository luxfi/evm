// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

// VMState represents the lifecycle state of the VM.
// Must match github.com/luxfi/vm State values:
//   - Unknown = 0
//   - Starting = 1
//   - Syncing = 2
//   - Bootstrapping = 3
//   - Ready = 4
//   - Degraded = 5
//   - Stopping = 6
//   - Stopped = 7
type VMState uint8

const (
	VMUnknown      VMState = iota // 0 - matches vm.Unknown
	VMStarting                    // 1 - matches vm.Starting
	VMStateSyncing                // 2 - matches vm.Syncing
	VMBootstrapping               // 3 - matches vm.Bootstrapping
	VMNormalOp                    // 4 - matches vm.Ready
	VMDegraded                    // 5 - matches vm.Degraded
	VMStopping                    // 6 - matches vm.Stopping
	VMStopped                     // 7 - matches vm.Stopped
)

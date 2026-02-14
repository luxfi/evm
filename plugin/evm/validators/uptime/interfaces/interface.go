// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package interfaces

import (
	validatorsstateinterfaces "github.com/luxfi/evm/plugin/evm/validators/state/interfaces"
	"github.com/luxfi/ids"
	"github.com/luxfi/validators/uptime"
)

type PausableManager interface {
	uptime.Calculator
	validatorsstateinterfaces.StateCallbackListener

	// Connection management
	Connect(nodeID ids.NodeID) error
	Disconnect(nodeID ids.NodeID) error
	IsConnected(nodeID ids.NodeID) bool
	StartTracking(nodeIDs []ids.NodeID) error
	StopTracking(nodeIDs []ids.NodeID) error

	// Pause management
	IsPaused(nodeID ids.NodeID) bool
}

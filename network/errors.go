// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package network

import "errors"

var (
	// ErrNoPeers is returned when there are no peers to send a request to
	ErrNoPeers = errors.New("no peers available")

	// ErrNoSender is returned when no sender is configured
	ErrNoSender = errors.New("no sender configured")

	// ErrNetworkClosed is returned when the network has been shut down
	ErrNetworkClosed = errors.New("network closed")

	// ErrRequestCancelled is returned when a request is cancelled
	ErrRequestCancelled = errors.New("request cancelled")

	// ErrRequestTimeout is returned when a request times out
	ErrRequestTimeout = errors.New("request timeout")
)

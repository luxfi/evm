// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	gethlog "github.com/luxfi/geth/log"
	"github.com/luxfi/log"
)

// Until we can make luxfi/log directly type-compatible with geth/log,
// we need this minimal compatibility layer for packages that expect geth/log

func asGethLogger(logger log.Logger) gethlog.Logger {
	// Create a geth logger that forwards to our logger
	// Both packages use slog under the hood, so we can share handlers
	return gethlog.NewLogger(logger.Handler())
}
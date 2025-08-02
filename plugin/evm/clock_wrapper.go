// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"time"

	"github.com/luxfi/evm/v2/utils"
	"github.com/luxfi/node/utils/timer/mockable"
)

// ClockWrapper wraps a mockable.Clock to implement utils.MockableTimer
type ClockWrapper struct {
	*mockable.Clock
}

// NewClockWrapper creates a new clock wrapper
func NewClockWrapper(clock *mockable.Clock) utils.MockableTimer {
	return &ClockWrapper{Clock: clock}
}

// Advance implements utils.MockableTimer
func (c *ClockWrapper) Advance(d time.Duration) {
	c.Clock.Set(c.Clock.Time().Add(d))
}

// (c) 2025-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testutils

import (
	"sync"
	"testing"

	"github.com/luxfi/geth/metrics"
)

var metricsLock sync.Mutex

// WithMetrics enables go-ethereum metrics globally for the test.
// If metrics are already enabled, nothing is done.
func WithMetrics(t *testing.T) {
	metricsLock.Lock()
	t.Cleanup(func() {
		metricsLock.Unlock()
	})
	if metrics.Enabled() {
		return
	}
	// Enable metrics for the test
	metrics.Enable()
}

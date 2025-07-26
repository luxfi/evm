// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package statesync

import (
	"testing"
	"time"

	"github.com/luxfi/geth/metrics"
	"github.com/stretchr/testify/require"
)

func TestETAShouldNotOverflow(t *testing.T) {
	require := require.New(t)
	now := time.Now()
	start := now.Add(-6 * time.Hour)

	stats := &trieSyncStats{
		triesStartTime: start,
		triesSynced:    100_000,
		triesRemaining: 450_000,
		leafsRateGauge: metrics.GetOrRegisterGauge("test_gauge", nil),
	}
	require.Positive(stats.updateETA(time.Minute, now))
}
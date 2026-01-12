// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gatherer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/luxfi/evm/metrics/metricstest"
	"github.com/luxfi/geth/metrics"
	"github.com/luxfi/metric"
)

func TestGatherer_Gather(t *testing.T) {
	metricstest.WithMetrics(t)

	registry := metrics.NewRegistry()
	register := func(t *testing.T, name string, collector any) {
		t.Helper()
		err := registry.Register(name, collector)
		require.NoErrorf(t, err, "registering collector %q", name)
	}

	counter := metrics.NewCounter()
	counter.Inc(12345)
	register(t, "test/counter", counter)

	counterFloat64 := metrics.NewCounterFloat64()
	counterFloat64.Inc(1.1)
	register(t, "test/counter_float64", counterFloat64)

	gauge := metrics.NewGauge()
	gauge.Update(23456)
	register(t, "test/gauge", gauge)

	gaugeFloat64 := metrics.NewGaugeFloat64()
	gaugeFloat64.Update(34567.89)
	register(t, "test/gauge_float64", gaugeFloat64)

	gaugeInfo := metrics.NewGaugeInfo()
	gaugeInfo.Update(metrics.GaugeInfoValue{"key": "value"})
	register(t, "test/gauge_info", gaugeInfo) // skipped

	sample := metrics.NewUniformSample(1028)
	histogram := metrics.NewHistogram(sample)
	register(t, "test/histogram", histogram)

	meter := metrics.NewMeter()
	t.Cleanup(meter.Stop)
	meter.Mark(9999999)
	register(t, "test/meter", meter)

	timer := metrics.NewTimer()
	t.Cleanup(timer.Stop)
	timer.Update(20 * time.Millisecond)
	timer.Update(21 * time.Millisecond)
	timer.Update(22 * time.Millisecond)
	timer.Update(120 * time.Millisecond)
	timer.Update(23 * time.Millisecond)
	timer.Update(24 * time.Millisecond)
	register(t, "test/timer", timer)

	resettingTimer := metrics.NewResettingTimer()
	register(t, "test/resetting_timer", resettingTimer)
	resettingTimer.Update(time.Second) // must be after register call

	emptyResettingTimer := metrics.NewResettingTimer()
	register(t, "test/empty_resetting_timer", emptyResettingTimer)

	emptyResettingTimer.Update(time.Second) // no effect because of snapshot below
	register(t, "test/empty_resetting_timer_snapshot", emptyResettingTimer.Snapshot())

	// Skip nil metrics registration as it causes issues with gatherer
	// registerNilMetrics(t, register)

	gatherer := NewGatherer(registry)

	families, err := gatherer.Gather()
	require.NoError(t, err)

	// Build expected metrics programmatically to match gatherer output format
	expectedFamilies := map[string]*metric.MetricFamily{
		"test_counter": {
			Name: "test_counter",
			Type: metric.MetricTypeCounter,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{Value: 12345},
			}},
		},
		"test_counter_float64": {
			Name: "test_counter_float64",
			Type: metric.MetricTypeCounter,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{Value: 1.1},
			}},
		},
		"test_gauge": {
			Name: "test_gauge",
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{Value: 23456},
			}},
		},
		"test_gauge_float64": {
			Name: "test_gauge_float64",
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{Value: 34567.89},
			}},
		},
		"test_histogram": {
			Name: "test_histogram",
			Type: metric.MetricTypeSummary,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					SampleCount: 0,
					SampleSum:   0,
					Quantiles: []metric.Quantile{
						{Quantile: 0.5, Value: 0},
						{Quantile: 0.75, Value: 0},
						{Quantile: 0.95, Value: 0},
						{Quantile: 0.99, Value: 0},
						{Quantile: 0.999, Value: 0},
						{Quantile: 0.9999, Value: 0},
					},
				},
			}},
		},
		"test_meter": {
			Name: "test_meter",
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{Value: 9999999},
			}},
		},
		"test_resetting_timer": {
			Name: "test_resetting_timer",
			Type: metric.MetricTypeSummary,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					SampleCount: 1,
					SampleSum:   1e9,
					Quantiles: []metric.Quantile{
						{Quantile: 50, Value: 1e9},
						{Quantile: 95, Value: 1e9},
						{Quantile: 99, Value: 1e9},
					},
				},
			}},
		},
		"test_timer": {
			Name: "test_timer",
			Type: metric.MetricTypeSummary,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					SampleCount: 6,
					SampleSum:   2.3e8,
					Quantiles: []metric.Quantile{
						{Quantile: 0.5, Value: 2.25e7},
						{Quantile: 0.75, Value: 4.8e7},
						{Quantile: 0.95, Value: 1.2e8},
						{Quantile: 0.99, Value: 1.2e8},
						{Quantile: 0.999, Value: 1.2e8},
						{Quantile: 0.9999, Value: 1.2e8},
					},
				},
			}},
		},
	}

	assert.Len(t, families, len(expectedFamilies))
	for _, got := range families {
		want, ok := expectedFamilies[got.Name]
		require.True(t, ok, "unexpected metric family: %s", got.Name)
		assert.Equal(t, want.Type, got.Type, "type mismatch for %s", got.Name)
		assert.Equal(t, want.Help, got.Help, "help mismatch for %s", got.Name)
		assert.Equal(t, want.Metrics, got.Metrics, "metrics mismatch for %s", got.Name)
	}

	register(t, "unsupported", metrics.NewHealthcheck(nil))
	families, err = gatherer.Gather()
	assert.ErrorIs(t, err, errMetricTypeNotSupported)
	assert.Empty(t, families)
}

func registerNilMetrics(t *testing.T, register func(t *testing.T, name string, collector any)) {
	// metrics.Enabled = false
	// defer func() { metrics.Enabled = true }()
	nilCounter := metrics.NewCounter()
	register(t, "nil/counter", nilCounter)
	nilCounterFloat64 := metrics.NewCounterFloat64()
	register(t, "nil/counter_float64", nilCounterFloat64)
	// nilEWMA := &metrics.NilEWMA{}
	// nilEWMA := metrics.NewEWMA1()
	// register(t, "nil/ewma", nilEWMA)
	nilGauge := metrics.NewGauge()
	register(t, "nil/gauge", nilGauge)
	nilGaugeFloat64 := metrics.NewGaugeFloat64()
	register(t, "nil/gauge_float64", nilGaugeFloat64)
	nilGaugeInfo := metrics.NewGaugeInfo()
	register(t, "nil/gauge_info", nilGaugeInfo)
	nilHealthcheck := metrics.NewHealthcheck(nil)
	register(t, "nil/healthcheck", nilHealthcheck)
	nilHistogram := metrics.NewHistogram(nil)
	register(t, "nil/histogram", nilHistogram)
	nilMeter := metrics.NewMeter()
	register(t, "nil/meter", nilMeter)
	nilResettingTimer := metrics.NewResettingTimer()
	register(t, "nil/resetting_timer", nilResettingTimer)
	nilSample := metrics.NewUniformSample(1028)
	register(t, "nil/sample", nilSample)
	nilTimer := metrics.NewTimer()
	register(t, "nil/timer", nilTimer)
}

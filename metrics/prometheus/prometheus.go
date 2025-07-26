// (c) 2021-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prometheus

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	
	"github.com/luxfi/geth/metrics"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Gatherer implements [prometheus.Gatherer] interface by
// gathering all metrics from the given Prometheus registry.
type Gatherer struct {
	registry Registry
}

var _ prometheus.Gatherer = (*Gatherer)(nil)

// NewGatherer returns a [Gatherer] using the given registry.
func NewGatherer(registry Registry) *Gatherer {
	return &Gatherer{
		registry: registry,
	}
}

// Gather gathers metrics from the registry and converts them to
// a slice of metric families.
func (g *Gatherer) Gather() (mfs []*dto.MetricFamily, err error) {
	// Gather and pre-sort the metrics to avoid random listings
	var names []string
	g.registry.Each(func(name string, i any) {
		names = append(names, name)
	})
	sort.Strings(names)

	mfs = make([]*dto.MetricFamily, 0, len(names))
	for _, name := range names {
		mf, err := metricFamily(g.registry, name)
		switch {
		case errors.Is(err, errMetricSkip):
			continue
		case err != nil:
			return nil, err
		}
		mfs = append(mfs, mf)
	}

	return mfs, nil
}

var (
	errMetricSkip             = errors.New("metric skipped")
	errMetricTypeNotSupported = errors.New("metric type is not supported")
)

func ptrTo[T any](x T) *T { return &x }

func metricFamily(registry Registry, name string) (mf *dto.MetricFamily, err error) {
    metric := registry.Get(name)
    name = strings.ReplaceAll(name, "/", "_")

    // Check for nil metric
    if metric == nil {
        return nil, fmt.Errorf("%w: %q metric is nil", errMetricSkip, name)
    }

    // Handle types - both pointer and non-pointer versions
	switch m := metric.(type) {
	case *metrics.Counter:
		snapshot := m.Snapshot()
		// Skip nil metrics (they have zero values and are from registerNilMetrics)
		if strings.HasPrefix(name, "nil_") && snapshot.Count() == 0 {
			return nil, fmt.Errorf("%w: %q counter is nil", errMetricSkip, name)
		}
		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_COUNTER.Enum(),
			Metric: []*dto.Metric{{
				Counter: &dto.Counter{
					Value: ptrTo(float64(snapshot.Count())),
				},
			}},
		}, nil
		
	case *metrics.CounterFloat64:
		snapshot := m.Snapshot()
		if strings.HasPrefix(name, "nil_") && snapshot.Count() == 0 {
			return nil, fmt.Errorf("%w: %q counter_float64 is nil", errMetricSkip, name)
		}
		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_COUNTER.Enum(),
			Metric: []*dto.Metric{{
				Counter: &dto.Counter{
					Value: ptrTo(snapshot.Count()),
				},
			}},
		}, nil
		
	case *metrics.Gauge:
		if strings.HasPrefix(name, "nil_") && m.Snapshot().Value() == 0 {
			return nil, fmt.Errorf("%w: %q gauge is nil", errMetricSkip, name)
		}
		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_GAUGE.Enum(),
			Metric: []*dto.Metric{{
				Gauge: &dto.Gauge{
					Value: ptrTo(float64(m.Snapshot().Value())),
				},
			}},
		}, nil
		
	case *metrics.GaugeFloat64:
		if strings.HasPrefix(name, "nil_") && m.Snapshot().Value() == 0 {
			return nil, fmt.Errorf("%w: %q gauge_float64 is nil", errMetricSkip, name)
		}
		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_GAUGE.Enum(),
			Metric: []*dto.Metric{{
				Gauge: &dto.Gauge{
					Value: ptrTo(m.Snapshot().Value()),
				},
			}},
		}, nil
		
	case *metrics.GaugeInfo:
		// Always skip GaugeInfo
		return nil, fmt.Errorf("%w: %q is a gauge_info", errMetricSkip, name)
		
	case metrics.Histogram:
		snapshot := m.Snapshot()
		if snapshot.Count() == 0 || strings.HasPrefix(name, "nil_") {
			return nil, fmt.Errorf("%w: %q histogram has no data", errMetricSkip, name) 
		}

		quantiles := []float64{.5, .75, .95, .99, .999, .9999}
		thresholds := snapshot.Percentiles(quantiles)
		dtoQuantiles := make([]*dto.Quantile, len(quantiles))
		for i, quantile := range quantiles {
			dtoQuantiles[i] = &dto.Quantile{
				Quantile: ptrTo(quantile),
				Value:    ptrTo(thresholds[i]),
			}
		}

		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_SUMMARY.Enum(),
			Metric: []*dto.Metric{{
				Summary: &dto.Summary{
					SampleCount: ptrTo(uint64(snapshot.Count())),
					SampleSum:   ptrTo(float64(snapshot.Sum())),
					Quantile:    dtoQuantiles,
				},
			}},
		}, nil
		
	case *metrics.Meter:
		snapshot := m.Snapshot()
		if strings.HasPrefix(name, "nil_") && snapshot.Count() == 0 {
			return nil, fmt.Errorf("%w: %q meter is nil", errMetricSkip, name)
		}
		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_GAUGE.Enum(),
			Metric: []*dto.Metric{{
				Gauge: &dto.Gauge{
					Value: ptrTo(float64(snapshot.Count())),
				},
			}},
		}, nil
		
	case *metrics.Timer:
		snapshot := m.Snapshot()
		if snapshot.Count() == 0 || strings.HasPrefix(name, "nil_") {
			return nil, fmt.Errorf("%w: %q timer has no data", errMetricSkip, name)
		}

		quantiles := []float64{.5, .75, .95, .99, .999, .9999}
		thresholds := snapshot.Percentiles(quantiles)
		dtoQuantiles := make([]*dto.Quantile, len(quantiles))
		for i, quantile := range quantiles {
			dtoQuantiles[i] = &dto.Quantile{
				Quantile: ptrTo(quantile),
				Value:    ptrTo(thresholds[i] / float64(time.Millisecond)),
			}
		}

		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_SUMMARY.Enum(),
			Metric: []*dto.Metric{{
				Summary: &dto.Summary{
					SampleCount: ptrTo(uint64(snapshot.Count())),
					SampleSum:   ptrTo(float64(snapshot.Sum())),
					Quantile:    dtoQuantiles,
				},
			}},
		}, nil
		
	case *metrics.ResettingTimer:
		snapshot := m.Snapshot()
		if snapshot.Count() == 0 || strings.HasPrefix(name, "nil_") {
			return nil, fmt.Errorf("%w: %q resetting timer has no data", errMetricSkip, name)
		}

		pvShortPercent := []float64{50, 95, 99}
		thresholds := snapshot.Percentiles(pvShortPercent)
		dtoQuantiles := make([]*dto.Quantile, len(pvShortPercent))
		for i, p := range pvShortPercent {
			dtoQuantiles[i] = &dto.Quantile{
				Quantile: ptrTo(p / 100.0),
				Value:    ptrTo(thresholds[i] / float64(time.Millisecond)),
			}
		}

		return &dto.MetricFamily{
			Name: &name,
			Type: dto.MetricType_SUMMARY.Enum(),
			Metric: []*dto.Metric{{
				Summary: &dto.Summary{
					SampleCount: ptrTo(uint64(snapshot.Count())),
					// ResettingTimer doesn't have Sum, calculate from mean * count
					SampleSum:   ptrTo(snapshot.Mean() * float64(snapshot.Count()) / float64(time.Millisecond)),
					Quantile:    dtoQuantiles,
				},
			}},
		}, nil

	default:
        // Skip or error on unsupported types
        switch metric.(type) {
        case *metrics.UniformSample, *metrics.ResettingTimerSnapshot:
            // Skip samples and snapshots
            return nil, fmt.Errorf("%w: %q is a sample/snapshot", errMetricSkip, name)
        case *metrics.Healthcheck:
            // Skip healthchecks from nil metrics, error for others
            if strings.HasPrefix(name, "nil_") {
                return nil, fmt.Errorf("%w: %q is a nil healthcheck", errMetricSkip, name)
            }
            return nil, fmt.Errorf("%w: %q is a healthcheck", errMetricTypeNotSupported, name)
        case *metrics.EWMA:
            if strings.HasPrefix(name, "nil_") {
                return nil, fmt.Errorf("%w: %q is a nil EWMA", errMetricSkip, name)
            }
            return nil, fmt.Errorf("%w: %q is an EWMA", errMetricTypeNotSupported, name)
        default:
            return nil, fmt.Errorf("%w: metric %q type %T", errMetricTypeNotSupported, name, metric)
        }
	}
}
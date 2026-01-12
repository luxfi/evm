// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package gatherer

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/luxfi/geth/metrics"
	"github.com/luxfi/metric"
)

// Gatherer implements [metric.Gatherer] interface by
// gathering all metrics from the given registry.
type Gatherer struct {
	registry Registry
}

var _ metric.Gatherer = (*Gatherer)(nil)

// NewGatherer returns a [Gatherer] using the given registry.
func NewGatherer(registry Registry) *Gatherer {
	return &Gatherer{
		registry: registry,
	}
}

// Gather gathers metrics from the registry and converts them to
// a slice of metric families.
func (g *Gatherer) Gather() (mfs []*metric.MetricFamily, err error) {
	// Gather and pre-sort the metrics to avoid random listings
	var names []string
	g.registry.Each(func(name string, i any) {
		names = append(names, name)
	})
	sort.Strings(names)

	mfs = make([]*metric.MetricFamily, 0, len(names))
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

func metricFamily(registry Registry, name string) (mf *metric.MetricFamily, err error) {
	metricValue := registry.Get(name)
	name = strings.ReplaceAll(name, "/", "_")

	if metricValue == nil {
		return nil, fmt.Errorf("%w: %q metric is nil", errMetricSkip, name)
	}

	switch m := metricValue.(type) {
	case metrics.NilCounter, metrics.NilCounterFloat64, metrics.NilEWMA,
		metrics.NilGauge, metrics.NilGaugeFloat64, metrics.NilGaugeInfo,
		metrics.NilHealthcheck, metrics.NilHistogram, metrics.NilMeter,
		metrics.NilResettingTimer, metrics.NilSample, metrics.NilTimer:
		return nil, fmt.Errorf("%w: %q metric is nil", errMetricSkip, name)

	case *metrics.Counter:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeCounter,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: float64((*m).Snapshot().Count()),
				},
			}},
		}, nil
	case metrics.Counter:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeCounter,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: float64(m.Snapshot().Count()),
				},
			}},
		}, nil
	case *metrics.CounterFloat64:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeCounter,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: (*m).Snapshot().Count(),
				},
			}},
		}, nil
	case metrics.CounterFloat64:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeCounter,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: m.Snapshot().Count(),
				},
			}},
		}, nil
	case *metrics.Gauge:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: float64((*m).Snapshot().Value()),
				},
			}},
		}, nil
	case metrics.Gauge:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: float64(m.Snapshot().Value()),
				},
			}},
		}, nil
	case *metrics.GaugeFloat64:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: (*m).Snapshot().Value(),
				},
			}},
		}, nil
	case metrics.GaugeFloat64:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: m.Snapshot().Value(),
				},
			}},
		}, nil
	case *metrics.GaugeInfo:
		return nil, fmt.Errorf("%w: %q is a %T", errMetricSkip, name, metricValue)
	case metrics.GaugeInfo:
		return nil, fmt.Errorf("%w: %q is a %T", errMetricSkip, name, metricValue)
	case *metrics.Histogram:
		return metricFamilyFromHistogram(name, (*m).Snapshot()), nil
	case metrics.Histogram:
		return metricFamilyFromHistogram(name, m.Snapshot()), nil
	case *metrics.Meter:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: float64((*m).Snapshot().Count()),
				},
			}},
		}, nil
	case metrics.Meter:
		return &metric.MetricFamily{
			Name: name,
			Type: metric.MetricTypeGauge,
			Metrics: []metric.Metric{{
				Value: metric.MetricValue{
					Value: float64(m.Snapshot().Count()),
				},
			}},
		}, nil
	case *metrics.Timer:
		return metricFamilyFromTimer(name, (*m).Snapshot()), nil
	case metrics.Timer:
		return metricFamilyFromTimer(name, m.Snapshot()), nil
	case *metrics.ResettingTimer:
		return metricFamilyFromResettingTimer(name, (*m).Snapshot())
	case metrics.ResettingTimer:
		return metricFamilyFromResettingTimer(name, m.Snapshot())
	case metrics.Healthcheck:
		return nil, fmt.Errorf("%w: %q is a %T", errMetricSkip, name, metricValue)
	default:
		return nil, fmt.Errorf("%w: metric %q type %T", errMetricTypeNotSupported, name, metricValue)
	}
}

func metricFamilyFromHistogram(name string, snapshot metrics.HistogramSnapshot) *metric.MetricFamily {
	quantiles := []float64{.5, .75, .95, .99, .999, .9999}
	thresholds := snapshot.Percentiles(quantiles)
	metricQuantiles := make([]metric.Quantile, len(quantiles))
	for i := range thresholds {
		metricQuantiles[i] = metric.Quantile{
			Quantile: quantiles[i],
			Value:    thresholds[i],
		}
	}

	return &metric.MetricFamily{
		Name: name,
		Type: metric.MetricTypeSummary,
		Metrics: []metric.Metric{{
			Value: metric.MetricValue{
				SampleCount: uint64(snapshot.Count()), //nolint:gosec
				SampleSum:   float64(snapshot.Sum()),
				Quantiles:   metricQuantiles,
			},
		}},
	}
}

func metricFamilyFromTimer(name string, snapshot *metrics.TimerSnapshot) *metric.MetricFamily {
	quantiles := []float64{.5, .75, .95, .99, .999, .9999}
	thresholds := snapshot.Percentiles(quantiles)
	metricQuantiles := make([]metric.Quantile, len(quantiles))
	for i := range thresholds {
		metricQuantiles[i] = metric.Quantile{
			Quantile: quantiles[i],
			Value:    thresholds[i],
		}
	}

	return &metric.MetricFamily{
		Name: name,
		Type: metric.MetricTypeSummary,
		Metrics: []metric.Metric{{
			Value: metric.MetricValue{
				SampleCount: uint64(snapshot.Count()), //nolint:gosec
				SampleSum:   float64(snapshot.Sum()),
				Quantiles:   metricQuantiles,
			},
		}},
	}
}

func metricFamilyFromResettingTimer(name string, snapshot *metrics.ResettingTimerSnapshot) (*metric.MetricFamily, error) {
	count := snapshot.Count()
	if count == 0 {
		return nil, fmt.Errorf("%w: %q resetting timer metric count is zero", errMetricSkip, name)
	}

	pvShortPercent := []float64{50, 95, 99}
	thresholds := snapshot.Percentiles(pvShortPercent)
	metricQuantiles := make([]metric.Quantile, len(pvShortPercent))
	for i := range pvShortPercent {
		metricQuantiles[i] = metric.Quantile{
			Quantile: pvShortPercent[i],
			Value:    thresholds[i],
		}
	}

	return &metric.MetricFamily{
		Name: name,
		Type: metric.MetricTypeSummary,
		Metrics: []metric.Metric{{
			Value: metric.MetricValue{
				SampleCount: uint64(count), //nolint:gosec
				SampleSum:   float64(count) * snapshot.Mean(),
				Quantiles:   metricQuantiles,
			},
		}},
	}, nil
}

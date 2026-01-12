// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/luxfi/cache/bytecache"
	"github.com/luxfi/metric"
)

// MeteredCache wraps *bytecache.Cache and periodically pulls stats from it.
type MeteredCache struct {
	cache     *bytecache.Cache
	namespace string

	// stats to be surfaced
	entriesCount metric.Gauge
	bytesSize    metric.Gauge
	collisions   metric.Gauge
	gets         metric.Gauge
	sets         metric.Gauge
	misses       metric.Gauge
	statsTime    metric.Counter

	// count all operations to decide when to update stats
	ops             uint64
	updateFrequency uint64
}

// NewMeteredCache returns a new MeteredCache that will update stats to the
// provided namespace once per each [updateFrequency] operations.
// Note: if [updateFrequency] is passed as 0, it will be treated as 1.
func NewMeteredCache(size int, namespace string, updateFrequency uint64) *MeteredCache {
	if updateFrequency == 0 {
		updateFrequency = 1 // avoid division by zero
	}
	mc := &MeteredCache{
		cache:           bytecache.New(size),
		namespace:       namespace,
		updateFrequency: updateFrequency,
	}
	if namespace != "" {
		// only register stats if a namespace is provided.
		mc.entriesCount = metric.NewGauge(metric.GaugeOpts{Name: fmt.Sprintf("%s/entriesCount", namespace), Help: "cache entries"})
		mc.bytesSize = metric.NewGauge(metric.GaugeOpts{Name: fmt.Sprintf("%s/bytesSize", namespace), Help: "cache size in bytes"})
		mc.collisions = metric.NewGauge(metric.GaugeOpts{Name: fmt.Sprintf("%s/collisions", namespace), Help: "cache collisions"})
		mc.gets = metric.NewGauge(metric.GaugeOpts{Name: fmt.Sprintf("%s/gets", namespace), Help: "cache gets"})
		mc.sets = metric.NewGauge(metric.GaugeOpts{Name: fmt.Sprintf("%s/sets", namespace), Help: "cache sets"})
		mc.misses = metric.NewGauge(metric.GaugeOpts{Name: fmt.Sprintf("%s/misses", namespace), Help: "cache misses"})
		mc.statsTime = metric.NewCounter(metric.CounterOpts{Name: fmt.Sprintf("%s/statsTime", namespace), Help: "time spent updating cache stats"})
	}
	return mc
}

// updateStatsIfNeeded updates metrics from cache
func (mc *MeteredCache) updateStatsIfNeeded() {
	if mc.namespace == "" {
		return
	}
	ops := atomic.AddUint64(&mc.ops, 1)
	if ops%mc.updateFrequency != 0 {
		return
	}

	start := time.Now()
	s := bytecache.Stats{}
	mc.cache.UpdateStats(&s)
	if mc.entriesCount != nil {
		mc.entriesCount.Set(float64(s.EntriesCount))
		mc.bytesSize.Set(float64(s.BytesSize))
		mc.collisions.Set(float64(s.Collisions))
		mc.gets.Set(float64(s.GetCalls))
		mc.sets.Set(float64(s.SetCalls))
		mc.misses.Set(float64(s.Misses))
		mc.statsTime.Add(float64(time.Since(start).Nanoseconds()))
	}
}

func (mc *MeteredCache) Del(k []byte) {
	mc.updateStatsIfNeeded()
	mc.cache.Del(k)
}

func (mc *MeteredCache) Get(dst, k []byte) []byte {
	mc.updateStatsIfNeeded()
	return mc.cache.Get(dst, k)
}

func (mc *MeteredCache) GetBig(dst, k []byte) []byte {
	mc.updateStatsIfNeeded()
	return mc.cache.GetBig(dst, k)
}

func (mc *MeteredCache) Has(k []byte) bool {
	mc.updateStatsIfNeeded()
	return mc.cache.Has(k)
}

func (mc *MeteredCache) HasGet(dst, k []byte) ([]byte, bool) {
	mc.updateStatsIfNeeded()
	return mc.cache.HasGet(dst, k)
}

func (mc *MeteredCache) Set(k, v []byte) {
	mc.updateStatsIfNeeded()
	mc.cache.Set(k, v)
}

func (mc *MeteredCache) SetBig(k, v []byte) {
	mc.updateStatsIfNeeded()
	mc.cache.SetBig(k, v)
}

func (mc *MeteredCache) Reset() {
	mc.cache.Reset()
}

func (mc *MeteredCache) SaveToFileConcurrent(filePath string, concurrency int) error {
	return mc.cache.SaveToFileConcurrent(filePath, concurrency)
}

func (mc *MeteredCache) LoadFromFile(filePath string) error {
	return mc.cache.LoadFromFile(filePath)
}

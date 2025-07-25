package trie

import "github.com/luxfi/geth/metrics"

type StackTrieOptions struct {
    SkipLeftBoundary  bool
    SkipRightBoundary bool
    boundaryGauge     metrics.Gauge
}
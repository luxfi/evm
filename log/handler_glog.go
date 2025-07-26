package log

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// GlogHandler is a log handler that mimics the filtering features of Google's
// glog logger: setting global log levels; overriding with callsite pattern
// matches; and requesting backtraces at certain positions.
type GlogHandler struct {
	handler slog.Handler // The wrapped handler

	level   atomic.Int32 // Current log level
	lock    sync.Mutex   // Lock protecting the filter lists
	patterns []pattern   // Current list of callsite filters to apply
}

// pattern contains a filter for the Vmodule option
type pattern struct {
	pattern *regexp.Regexp
	level   int32
}

// NewGlogHandler creates a new glog handler wrapping the given handler.
func NewGlogHandler(h slog.Handler) *GlogHandler {
	return &GlogHandler{
		handler: h,
	}
}

// Handle implements slog.Handler
func (h *GlogHandler) Handle(ctx context.Context, r slog.Record) error {
	// Check if this level is enabled
	if !h.Enabled(ctx, r.Level) {
		return nil
	}
	return h.handler.Handle(ctx, r)
}

// Enabled implements slog.Handler
func (h *GlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= slog.Level(h.level.Load())
}

// WithAttrs implements slog.Handler
func (h *GlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &GlogHandler{
		handler: h.handler.WithAttrs(attrs),
		level:   h.level,
	}
}

// WithGroup implements slog.Handler
func (h *GlogHandler) WithGroup(name string) slog.Handler {
	return &GlogHandler{
		handler: h.handler.WithGroup(name),
		level:   h.level,
	}
}

// Verbosity sets the glog verbosity ceiling
func (h *GlogHandler) Verbosity(level slog.Level) {
	h.level.Store(int32(level))
}

// Vmodule sets the glog verbosity pattern
func (h *GlogHandler) Vmodule(ruleset string) error {
	h.lock.Lock()
	defer h.lock.Unlock()

	if ruleset == "" {
		h.patterns = h.patterns[:0]
		return nil
	}

	rules := strings.Split(ruleset, ",")
	for _, rule := range rules {
		if len(rule) == 0 {
			continue
		}

		parts := strings.Split(rule, "=")
		if len(parts) != 2 {
			return fmt.Errorf("invalid vmodule pattern %s", rule)
		}

		parts[0] = strings.TrimSpace(parts[0])
		parts[1] = strings.TrimSpace(parts[1])
		if len(parts[0]) == 0 || len(parts[1]) == 0 {
			return fmt.Errorf("invalid vmodule pattern %s", rule)
		}

		level, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid vmodule pattern %s", rule)
		}

		patterns := []string{parts[0]}
		if strings.Contains(parts[0], "/") {
			patterns = append(patterns, parts[0]+".*")
		}

		var filter *regexp.Regexp
		for _, pat := range patterns {
			if f, err := regexp.Compile(pat); err == nil {
				filter = f
				break
			}
		}
		if filter == nil {
			return fmt.Errorf("invalid vmodule pattern %s", rule)
		}

		h.patterns = append(h.patterns, pattern{filter, int32(level)})
	}
	return nil
}
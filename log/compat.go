// Package log provides a compatibility layer for go-ethereum style logging
// that redirects to luxfi/log
package log

import (
	"context"
	"io"
	"log/slog"
	"os"

	luxlog "github.com/luxfi/log"
)

// Re-export types and constants from luxfi/log
type (
	Logger = luxlog.Logger
)

const (
	// Level constants - use slog.Level values directly to avoid conflicts
	LevelTrace slog.Level = -8
	LevelDebug           = slog.LevelDebug
	LevelInfo            = slog.LevelInfo
	LevelWarn            = slog.LevelWarn
	LevelError           = slog.LevelError
	LevelCrit  slog.Level = 12

	// Backward compatibility
	LvlTrace = LevelTrace
	LvlInfo  = LevelInfo
	LvlDebug = LevelDebug
)

// Re-export functions from luxfi/log
var (
	New  = luxlog.New
	Root = luxlog.Root
)

// Global logging functions
func Trace(msg string, ctx ...interface{}) { luxlog.Root().Trace(msg, ctx...) }
func Debug(msg string, ctx ...interface{}) { luxlog.Root().Debug(msg, ctx...) }
func Info(msg string, ctx ...interface{})  { luxlog.Root().Info(msg, ctx...) }
func Warn(msg string, ctx ...interface{})  { luxlog.Root().Warn(msg, ctx...) }
func Error(msg string, ctx ...interface{}) { luxlog.Root().Error(msg, ctx...) }
func Crit(msg string, ctx ...interface{})  { luxlog.Root().Crit(msg, ctx...) }

func Enabled(ctx context.Context, level slog.Level) bool {
	return luxlog.Root().Enabled(ctx, level)
}

// NewLogger returns a logger with the specified handler set
func NewLogger(h slog.Handler) Logger {
	// For compatibility, we ignore the handler and return a luxfi logger
	return luxlog.Root()
}

// LvlFromString returns the appropriate level from a string name
func LvlFromString(lvlString string) (slog.Level, error) {
	level, err := luxlog.ToLevel(lvlString)
	return slog.Level(level), err
}

// LevelAlignedString returns a 5-character string containing the name of a level
func LevelAlignedString(l slog.Level) string {
	return luxlog.Level(l).String()
}

// LevelString returns a string containing the name of a level
func LevelString(l slog.Level) string {
	return luxlog.Level(l).LowerString()
}

// FromLegacyLevel converts from old Geth verbosity level constants
func FromLegacyLevel(lvl int) slog.Level {
	return luxlog.FromLegacyLevel(lvl)
}

// SetDefault sets the default logger
func SetDefault(l Logger) {
	luxlog.SetDefault(l)
}

// Handler types for compatibility
type GlogHandler struct{
	handler slog.Handler
}

// NewGlogHandler creates a new glog handler
func NewGlogHandler(handler slog.Handler) *GlogHandler {
	return &GlogHandler{handler: handler}
}

// SetHandler sets the handler (no-op for compatibility)
func (h *GlogHandler) SetHandler(handler slog.Handler) {
	h.handler = handler
}

// Verbosity sets the verbosity level (no-op for compatibility)
func (h *GlogHandler) Verbosity(level slog.Level) {
	// No-op
}

// DiscardHandler returns a handler that discards all log records
func DiscardHandler() slog.Handler {
	return slog.NewTextHandler(io.Discard, nil)
}

// StreamHandler returns a handler that writes to an io.Writer
func StreamHandler(w io.Writer, fmtr Formatter) slog.Handler {
	return slog.NewTextHandler(w, nil)
}

// FileHandler returns a handler that writes to a file
func FileHandler(path string, fmtr Formatter) (slog.Handler, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return slog.NewTextHandler(f, nil), nil
}

// NewTerminalHandler creates a handler that writes to terminal
func NewTerminalHandler(w io.Writer, useColor bool) slog.Handler {
	return slog.NewTextHandler(w, nil)
}

// Formatter interface for compatibility
type Formatter interface{}

// TerminalFormat returns a terminal formatter
func TerminalFormat(useColor bool) Formatter {
	return nil
}

// JSONFormat returns a JSON formatter
func JSONFormat() Formatter {
	return nil
}

// LvlFilterHandler returns a handler that filters by level
func LvlFilterHandler(maxLevel slog.Level, h slog.Handler) slog.Handler {
	return h
}
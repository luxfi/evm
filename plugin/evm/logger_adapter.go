// (c) 2020-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"log/slog"

	"go.uber.org/zap"
	luxlog "github.com/luxfi/log"
	"github.com/luxfi/node/utils/logging"
	gethlog "github.com/luxfi/geth/log"
)

// createLoggerAdapter creates an adapter that converts node's logging.Logger to luxfi/log.Logger
// This is a temporary solution until node/utils/logging is extracted to luxfi/log
func createLoggerAdapter(nodeLogger logging.Logger, chainAlias string) luxlog.Logger {
	return newZapToLuxLogAdapter(nodeLogger, chainAlias)
}

// zapToLuxLogAdapter adapts a zap-based logger to the luxfi/log.Logger interface
type zapToLuxLogAdapter struct {
	zapLogger logging.Logger
	chainAlias string
}

func newZapToLuxLogAdapter(zapLogger logging.Logger, chainAlias string) luxlog.Logger {
	return &zapToLuxLogAdapter{
		zapLogger: zapLogger,
		chainAlias: chainAlias,
	}
}

func (a *zapToLuxLogAdapter) With(ctx ...interface{}) luxlog.Logger {
	// Convert ctx pairs to zap fields
	fields := make([]zap.Field, 0, len(ctx)/2)
	for i := 0; i < len(ctx)-1; i += 2 {
		if key, ok := ctx[i].(string); ok {
			fields = append(fields, zap.Any(key, ctx[i+1]))
		}
	}
	return &zapToLuxLogAdapter{
		zapLogger: a.zapLogger.With(fields...),
		chainAlias: a.chainAlias,
	}
}

func (a *zapToLuxLogAdapter) New(ctx ...interface{}) luxlog.Logger {
	return a.With(ctx...)
}

func (a *zapToLuxLogAdapter) Log(level slog.Level, msg string, ctx ...interface{}) {
	// This method is not used in the adapter
}

func (a *zapToLuxLogAdapter) Trace(msg string, ctx ...interface{}) {
	fields := ctxToZapFields(ctx)
	a.zapLogger.Trace(msg, fields...)
}

func (a *zapToLuxLogAdapter) Debug(msg string, ctx ...interface{}) {
	fields := ctxToZapFields(ctx)
	a.zapLogger.Debug(msg, fields...)
}

func (a *zapToLuxLogAdapter) Info(msg string, ctx ...interface{}) {
	fields := ctxToZapFields(ctx)
	a.zapLogger.Info(msg, fields...)
}

func (a *zapToLuxLogAdapter) Warn(msg string, ctx ...interface{}) {
	fields := ctxToZapFields(ctx)
	a.zapLogger.Warn(msg, fields...)
}

func (a *zapToLuxLogAdapter) Error(msg string, ctx ...interface{}) {
	fields := ctxToZapFields(ctx)
	a.zapLogger.Error(msg, fields...)
}

func (a *zapToLuxLogAdapter) Crit(msg string, ctx ...interface{}) {
	fields := ctxToZapFields(ctx)
	a.zapLogger.Fatal(msg, fields...)
}

func (a *zapToLuxLogAdapter) Write(level slog.Level, msg string, attrs ...any) {
	// Convert to appropriate log level
	switch level {
	case luxlog.LevelTrace:
		a.Trace(msg, attrs...)
	case luxlog.LevelDebug:
		a.Debug(msg, attrs...)
	case luxlog.LevelInfo:
		a.Info(msg, attrs...)
	case luxlog.LevelWarn:
		a.Warn(msg, attrs...)
	case luxlog.LevelError:
		a.Error(msg, attrs...)
	case luxlog.LevelCrit:
		a.Crit(msg, attrs...)
	}
}

func (a *zapToLuxLogAdapter) Enabled(ctx context.Context, level slog.Level) bool {
	// Since we don't have access to the zap logger's level checking,
	// we'll assume all levels are enabled
	return true
}

func (a *zapToLuxLogAdapter) Handler() slog.Handler {
	// This is not implemented in the adapter
	return nil
}

// ctxToZapFields converts context key-value pairs to zap fields
func ctxToZapFields(ctx []interface{}) []zap.Field {
	fields := make([]zap.Field, 0, len(ctx)/2)
	for i := 0; i < len(ctx)-1; i += 2 {
		if key, ok := ctx[i].(string); ok {
			fields = append(fields, zap.Any(key, ctx[i+1]))
		}
	}
	return fields
}

// createGethLoggerFromLuxLog creates a geth logger that forwards to luxfi/log.Logger
// This is a temporary solution until geth/log is replaced with luxfi/log
func createGethLoggerFromLuxLog(luxLogger luxlog.Logger) gethlog.Logger {
	// Create a handler that forwards to the luxfi logger
	handler := &luxToGethHandler{luxLogger: luxLogger}
	return gethlog.NewLogger(handler)
}

// luxToGethHandler implements slog.Handler to forward logs to luxfi/log.Logger
type luxToGethHandler struct {
	luxLogger luxlog.Logger
}

func (h *luxToGethHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.luxLogger.Enabled(ctx, level)
}

func (h *luxToGethHandler) Handle(ctx context.Context, r slog.Record) error {
	// Convert slog attributes to luxfi log context
	var ctxPairs []interface{}
	r.Attrs(func(a slog.Attr) bool {
		ctxPairs = append(ctxPairs, a.Key, a.Value.Any())
		return true
	})
	
	// Forward to luxfi logger
	h.luxLogger.Write(r.Level, r.Message, ctxPairs...)
	return nil
}

func (h *luxToGethHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Convert attrs to context pairs
	var ctxPairs []interface{}
	for _, attr := range attrs {
		ctxPairs = append(ctxPairs, attr.Key, attr.Value.Any())
	}
	return &luxToGethHandler{
		luxLogger: h.luxLogger.With(ctxPairs...),
	}
}

func (h *luxToGethHandler) WithGroup(name string) slog.Handler {
	// Groups are not directly supported, so we'll just add it as a prefix
	return &luxToGethHandler{
		luxLogger: h.luxLogger.With("group", name),
	}
}
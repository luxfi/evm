// (c) 2019-2020, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"

	"log/slog"

	"github.com/luxfi/evm/log"
)

type EVMLogger struct {
	log.Logger

	logLevel *slog.LevelVar
}

// InitLogger initializes logger with alias and sets the log level and format with the original [os.StdErr] interface
// along with the context logger.
func InitLogger(alias string, level string, jsonFormat bool, writer io.Writer) (EVMLogger, error) {
	logLevel := &slog.LevelVar{}

	var handler slog.Handler
	if jsonFormat {
		chainStr := fmt.Sprintf("%s Chain", alias)
		handler = log.JSONHandlerWithLevel(writer, logLevel)
		handler = &addContext{Handler: handler, logger: chainStr}
	} else {
		useColor := false
		chainStr := fmt.Sprintf("<%s Chain> ", alias)
		termHandler := log.NewTerminalHandlerWithLevel(writer, logLevel, useColor)
		// The new TerminalHandler doesn't have a Prefix field anymore.
		// We'll add the chain prefix through a wrapper handler.
		handler = &prefixHandler{Handler: termHandler, prefix: chainStr}
	}

	// Create handler
	c := EVMLogger{
		Logger:   log.NewLogger(handler),
		logLevel: logLevel,
	}

	if err := c.SetLogLevel(level); err != nil {
		return EVMLogger{}, err
	}
	// log.SetDefault is not available in the current version
	// The logger is returned for the caller to use
	return c, nil
}

// SetLogLevel sets the log level of initialized log handler.
func (s *EVMLogger) SetLogLevel(level string) error {
	// Set log level
	logLevel, err := log.LvlFromString(level)
	if err != nil {
		return err
	}
	s.logLevel.Set(slog.Level(logLevel))
	return nil
}

// locationTrims are trimmed for display to avoid unwieldy log lines.
var locationTrims = []string{
	"evm/",
}

func trimPrefixes(s string) string {
	for _, prefix := range locationTrims {
		idx := strings.LastIndex(s, prefix)
		if idx >= 0 {
			s = s[idx+len(prefix):]
		}
	}
	return s
}

func getSource(r slog.Record) (string, int) {
	frames := runtime.CallersFrames([]uintptr{r.PC})
	frame, _ := frames.Next()
	return trimPrefixes(frame.File), frame.Line
}

type addContext struct {
	slog.Handler

	logger string
}

func (a *addContext) Handle(ctx context.Context, r slog.Record) error {
	r.Add(slog.String("logger", a.logger))
	file, line := getSource(r)
	if file != "" {
		r.Add(slog.String("caller", fmt.Sprintf("%s:%d", file, line)))
	}
	return a.Handler.Handle(ctx, r)
}

type prefixHandler struct {
	slog.Handler
	prefix string
}

func (p *prefixHandler) Handle(ctx context.Context, r slog.Record) error {
	file, line := getSource(r)
	if file != "" {
		r.AddAttrs(slog.String("location", fmt.Sprintf("%s%s:%d", p.prefix, file, line)))
	}
	return p.Handler.Handle(ctx, r)
}

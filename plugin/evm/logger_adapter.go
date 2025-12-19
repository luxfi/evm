// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package evm

import (
	"io"
	"os"
)

// loggerWriter wraps a consensus.Logger to implement io.Writer
type loggerWriter struct {
	logger interface{}
	writer io.Writer
}

// Write implements io.Writer - writes to stderr for plugin subprocess logging
func (w *loggerWriter) Write(p []byte) (n int, err error) {
	return w.writer.Write(p)
}

// newLoggerWriter creates a new loggerWriter that writes to stderr
func newLoggerWriter(logger interface{}) io.Writer {
	return &loggerWriter{
		logger: logger,
		writer: os.Stderr,
	}
}

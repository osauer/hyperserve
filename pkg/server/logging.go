package server

import "log/slog"

var logger = slog.Default()

// DefaultLogger returns the logger used by the server package.
func DefaultLogger() *slog.Logger {
	return logger
}

// SetDefaultLogger overrides the logger used by the server package.
func SetDefaultLogger(l *slog.Logger) {
	if l == nil {
		logger = slog.Default()
		return
	}
	logger = l
}

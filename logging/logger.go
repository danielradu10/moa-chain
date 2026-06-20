package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// NodeLogger owns a slog logger and the file receiving the same records as stdout.
type NodeLogger struct {
	logger *slog.Logger
	file   *os.File
	path   string
}

// NewNodeLogger creates a logger that writes every record to stdout and to filePath.
func NewNodeLogger(nodeID string, filePath string) (*NodeLogger, error) {
	return NewNodeLoggerWithLevel(nodeID, filePath, slog.LevelInfo)
}

// NewNodeLoggerWithLevel creates a logger with a caller-selected minimum level.
func NewNodeLoggerWithLevel(nodeID string, filePath string, level slog.Level) (*NodeLogger, error) {
	err := os.MkdirAll(filepath.Dir(filePath), 0o755)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, err
	}

	writer := io.MultiWriter(os.Stdout, file)
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: level,
	})

	return &NodeLogger{
		logger: slog.New(handler).With("node", nodeID),
		file:   file,
		path:   filePath,
	}, nil
}

// ParseLevel converts a user-facing level string into a slog level.
func ParseLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ParseTestLevel returns the requested test log level, defaulting tests to debug.
func ParseTestLevel(value string) slog.Level {
	if strings.TrimSpace(value) == "" {
		return slog.LevelDebug
	}

	return ParseLevel(value)
}

// NewNopLogger returns a logger that discards all records.
func NewNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Logger returns the underlying structured logger.
func (nl *NodeLogger) Logger() *slog.Logger {
	if nl == nil || nl.logger == nil {
		return NewNopLogger()
	}

	return nl.logger
}

// Path returns the path of the file receiving this logger's records.
func (nl *NodeLogger) Path() string {
	if nl == nil {
		return ""
	}

	return nl.path
}

// Close closes the log file.
func (nl *NodeLogger) Close() error {
	if nl == nil || nl.file == nil {
		return nil
	}

	return nl.file.Close()
}

// FromOptional returns the first non-nil logger or a no-op logger.
func FromOptional(loggers ...*slog.Logger) *slog.Logger {
	for _, logger := range loggers {
		if logger != nil {
			return logger
		}
	}

	return NewNopLogger()
}

package logging

import (
	"context"
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
	return NewNodeLoggerWithLevels(nodeID, filePath, level, level)
}

// NewNodeLoggerWithLevels creates a logger with separate file and stdout levels.
func NewNodeLoggerWithLevels(
	nodeID string,
	filePath string,
	fileLevel slog.Level,
	consoleLevel slog.Level,
) (*NodeLogger, error) {
	err := os.MkdirAll(filepath.Dir(filePath), 0o755)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	fileHandler := slog.NewTextHandler(file, &slog.HandlerOptions{
		Level: fileLevel,
	})
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: consoleLevel,
	})
	handler := newMultiHandler(consoleHandler, fileHandler)

	return &NodeLogger{
		logger: slog.New(handler).With("node", nodeID),
		file:   file,
		path:   filePath,
	}, nil
}

type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{
		handlers: handlers,
	}
}

func (mh *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range mh.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

func (mh *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range mh.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}

		err := handler.Handle(ctx, record)
		if err != nil {
			return err
		}
	}

	return nil
}

func (mh *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(mh.handlers))
	for _, handler := range mh.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}

	return newMultiHandler(handlers...)
}

func (mh *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(mh.handlers))
	for _, handler := range mh.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}

	return newMultiHandler(handlers...)
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

// ParseIntegrationTestLevel returns the requested integration-test log level, defaulting to debug.
func ParseIntegrationTestLevel(value string) slog.Level {
	if strings.TrimSpace(value) == "" {
		return slog.LevelDebug
	}

	return ParseLevel(value)
}

// ParseIntegrationTestConsoleLevel returns the requested console log level, defaulting to errors only.
func ParseIntegrationTestConsoleLevel(value string) slog.Level {
	if strings.TrimSpace(value) == "" {
		return slog.LevelError
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

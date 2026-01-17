package internal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
)

// ConsoleHandler handles log records and outputs them to the console with colors.
type ConsoleHandler struct {
	w    io.Writer
	mu   sync.Mutex
	opts slog.HandlerOptions
}

// NewConsoleHandler creates a new ConsoleHandler.
func NewConsoleHandler(w io.Writer, opts *slog.HandlerOptions) *ConsoleHandler {
	if opts == nil {
		opts = &slog.HandlerOptions{}
	}
	return &ConsoleHandler{w: w, opts: *opts}
}

// Enabled checks if the log level is enabled.
func (h *ConsoleHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.opts.Level.Level()
}

// Handle outputs the log record.
func (h *ConsoleHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	timestamp := r.Time.Format("2006-01-02 15:04:05-07:00")
	levelColor := ""
	resetColor := "\033[0m"
	levelStr := ""

	switch r.Level {
	case slog.LevelDebug:
		levelColor = "\033[35m" // Magenta
		levelStr = "DEBUG"
	case slog.LevelInfo:
		levelColor = "\033[32m" // Green
		levelStr = "INFO "
	case slog.LevelWarn:
		levelColor = "\033[33m" // Yellow
		levelStr = "WARN "
	case slog.LevelError:
		levelColor = "\033[31m" // Red
		levelStr = "ERROR"
	default:
		levelStr = r.Level.String()
	}

	// Format: TIME [LEVEL] MESSAGE
	fmt.Fprintf(h.w, "%s %s[%s]%s %s", timestamp, levelColor, levelStr, resetColor, r.Message)

	// Append attributes if present, formatted as "key: value"
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(h.w, " %s: %v", a.Key, a.Value.Any())
		return true
	})

	fmt.Fprintln(h.w)
	return nil
}

// WithAttrs returns a new handler with the given attributes.
// For simplicity in this specific task, we're not implementing full attribute carrying
// because the usage in this project is simple direct calls.
func (h *ConsoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

// WithGroup returns a new handler with the given group.
func (h *ConsoleHandler) WithGroup(name string) slog.Handler {
	return h
}

// Log level constants for external configuration
const (
	LevelDebug = slog.LevelDebug
	LevelInfo  = slog.LevelInfo
	LevelWarn  = slog.LevelWarn
	LevelError = slog.LevelError
)

var defaultHandler *ConsoleHandler

func InitLogger(verbose bool) {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	defaultHandler = NewConsoleHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})

	logger := slog.New(defaultHandler)
	slog.SetDefault(logger)
}

// SetLogLevel dynamically changes the log level
func SetLogLevel(level slog.Level) {
	if defaultHandler != nil {
		defaultHandler.opts.Level = level
	}
}

func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}

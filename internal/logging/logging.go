// Package logging manages application logging using the log/slog package.
//
// This package provides a structured logging facility for applications. It
// allows the creation of a Logger instance that can log messages at different
// severity levels such as Debug, Info, Warn, Error, and Fatal. The logging
// configuration is flexible and supports different output destinations (such as
// standard output or files) and formats (such as JSON or text).
//
// The Logger uses the slog package for structured logging and can be configured
// to determine the logging output and format based on user-defined settings.
//
// Use the New function to create a Logger instance with specified logging
// configuration. Various methods are provided to log messages at different
// severity levels with additional context.
package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/peng-mj/scproxy/internal/config"
)

// nopWriteCloser wraps an io.Writer with a no-op Close method.
// This is useful for standard streams like os.Stdout and os.Stderr
// when an io.WriteCloser interface is required.
type nopWriteCloser struct {
	io.Writer
}

// Close implements the io.Closer interface for nopWriteCloser. It does nothing and returns nil.
func (nwc nopWriteCloser) Close() error { return nil }

// Logger represents an instance of the logging system.
type Logger struct {
	slogger  *slog.Logger // The slogger instance
	exitFunc func(int)    // Function to call on Fatal, defaults to os.Exit
	closer   io.Closer    // The underlying writer that might need to be closed (e.g., a file)
}

// New creates a new Logger instance with the specified logging configuration.
// It returns an error if the configuration is invalid or setup fails.
func New(cfg *config.LoggingConfig) (*Logger, error) {
	// Set the output based on the configuration
	output, err := parseOutput(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not parse log output: %v", err)
	}

	// Setup the handler based on the format
	handler, err := parseFormat(output, cfg)
	if err != nil {
		var errs []error
		errs = append(errs, fmt.Errorf("could not set up log handler: %v", err))

		// Close output
		if closeErr := output.Close(); closeErr != nil {
			errs = append(errs, fmt.Errorf("failed to close output writer: %v", closeErr))
		}

		if len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
	}

	return &Logger{
		slogger:  slog.New(handler),
		exitFunc: os.Exit,
		closer:   output,
	}, nil
}

// Close closes the logger's underlying output writer, if it is closable
// (e.g., a file). It should be called when the logger is no longer needed
// to release resources.
func (l *Logger) Close() error {
	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// Debug logs a message at the debug level.
func (l *Logger) Debug(msg string, args ...any) {
	l.slogger.Debug(msg, args...)
}

// Info logs a message at the info level.
func (l *Logger) Info(msg string, args ...any) {
	l.slogger.Info(msg, args...)
}

// Warn logs a message at the warn level.
func (l *Logger) Warn(msg string, args ...any) {
	l.slogger.Warn(msg, args...)
}

// Error logs a message at the error level.
func (l *Logger) Error(msg string, args ...any) {
	l.slogger.Error(msg, args...)
}

// Fatal logs a message at the error level and then exits the program.
// The exit behavior can be overridden for testing.
func (l *Logger) Fatal(msg string, args ...any) {
	l.slogger.Error(msg, args...)
	if l.exitFunc != nil {
		l.exitFunc(1)
	} else {
		os.Exit(1)
	}
}

// parseOutput determines the io.Writer for logging based on the configuration.
// Note: If a file is opened, the caller is responsible for closing it.
func parseOutput(cfg *config.LoggingConfig) (io.WriteCloser, error) {
	var output io.WriteCloser

	switch cfg.Output {
	case config.LogOutputStderr:
		output = nopWriteCloser{os.Stderr}
	case config.LogOutputFile:
		if cfg.Path == "" {
			return nil, errors.New("internal error: file output mode requires a non-empty path, but path is empty")
		} else {
			file, err := os.OpenFile(cfg.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
			if err != nil {
				return nil, fmt.Errorf("failed to open log file %q: %v", cfg.Path, err)
			}
			output = file
		}
	case config.LogOutputStdout:
		fallthrough
	default:
		output = nopWriteCloser{os.Stdout}
	}

	return output, nil
}

// parseLevel converts a string representation of a log level from config.LogLevel
// to slog.Level. Defaults to slog.LevelInfo on parsing failure.
func parseLevel(logLevel config.LogLevel) slog.Level {
	var level slog.Level
	if err := level.UnmarshalText([]byte(string(logLevel))); err != nil {
		return slog.LevelInfo
	}
	return level
}

// parseFormat creates an slog.Handler with the compact format.
func parseFormat(output io.Writer, cfg *config.LoggingConfig) (slog.Handler, error) {
	opts := &slog.HandlerOptions{
		Level: parseLevel(cfg.Level),
	}
	return newCompactTextHandler(output, opts), nil
}

// compactTextHandler is a custom slog.Handler that outputs logs in a compact format.
// Format: MM-DD HH:MM:SS.mmm [LEVEL] message key=value key=value...
type compactTextHandler struct {
	mu    sync.Mutex
	out   io.Writer
	level slog.Level
}

// newCompactTextHandler creates a new compactTextHandler.
func newCompactTextHandler(output io.Writer, opts *slog.HandlerOptions) *compactTextHandler {
	h := &compactTextHandler{
		out:   output,
		level: slog.LevelInfo,
	}
	if opts != nil && opts.Level != nil {
		h.level = opts.Level.Level()
	}
	return h
}

// Enabled reports whether the handler handles records at the given level.
func (h *compactTextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle handles the Record.
func (h *compactTextHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Format: MM-DD HH:MM:SS.mmm [LEVEL] message key=value key=value...
	t := r.Time
	buf := make([]byte, 0, 200)
	buf = append(buf, fmt.Sprintf("%02d-%02d %02d:%02d:%02d.%03d ",
		t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond()/1_000_000)...)
	buf = append(buf, levelToString(r.Level)...)
	buf = append(buf, ' ')
	buf = append(buf, r.Message...)

	// Append args as key=value pairs
	r.Attrs(func(a slog.Attr) bool {
		buf = append(buf, ' ')
		buf = append(buf, a.Key...)
		buf = append(buf, '=')
		buf = append(buf, a.Value.String()...)
		return true
	})

	buf = append(buf, '\n')

	_, err := h.out.Write(buf)
	return err
}

// WithAttrs returns a new Handler whose attributes are the union of h's attributes
// and attrs. Not implemented for this simple handler.
func (h *compactTextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

// WithGroup returns a new Handler with the given group appended to the receiver's
// existing groups. Not implemented for this simple handler.
func (h *compactTextHandler) WithGroup(name string) slog.Handler {
	return h
}

// levelToString converts slog.Level to string with brackets.
func levelToString(level slog.Level) string {
	switch {
	case level < slog.LevelInfo:
		return "[DEBUG]"
	case level < slog.LevelWarn:
		return "[INFO]"
	case level < slog.LevelError:
		return "[WARN]"
	default:
		return "[ERROR]"
	}
}

// logging_test.go
package logging

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/peng-mj/scproxy/internal/config"
)

// newTestDefaultConfig is a helper to create a default valid config.LoggingConfig for tests.
// Assumes config package defines these constants/types (e.g., config.InfoLevel = "info").
func newTestDefaultConfig() *config.LoggingConfig {
	return &config.LoggingConfig{
		Level:  config.LogLevelInfo,
		Output: config.LogOutputStdout,
		Path:   "",
	}
}

// TestNew checks the Logging creation.
func TestNew(t *testing.T) {
	t.Run("should_create_logger_with_default_config", func(t *testing.T) {
		cfg := newTestDefaultConfig() // Uses StdoutOutput by default from helper

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("New() with default config failed: %v", err)
		}
		if logger == nil {
			t.Fatal("New() returned nil logger for default config")
		}
		if logger.slogger == nil {
			t.Fatal("New() logger.slogger is nil")
		}
	})

	t.Run("should_create_logger_for_file_output", func(t *testing.T) {
		tempDir := t.TempDir()
		logFilePath := filepath.Join(tempDir, "test_new.log")

		cfg := newTestDefaultConfig()
		cfg.Output = config.LogOutputFile
		cfg.Path = logFilePath

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("New() with file output config failed: %v", err)
		}
		if logger == nil {
			t.Fatal("New() returned nil logger for file output")
		}
		defer func() {
			if err := logger.Close(); err != nil {
				t.Fatalf("Failed to close log file %q: %v", logFilePath, err)
			}
		}()

		logger.Info("test message for file output")

		content, readErr := os.ReadFile(logFilePath)
		if readErr != nil {
			t.Fatalf("Failed to read log file %q: %v", logFilePath, readErr)
		}
		if !strings.Contains(string(content), "test message for file output") {
			t.Errorf("Log file content does not contain expected message. Got: %s", string(content))
		}
	})

	t.Run("should_fail_if_parseOutput_fails_due_to_empty_path_for_file", func(t *testing.T) {
		cfg := newTestDefaultConfig()
		cfg.Output = config.LogOutputFile
		cfg.Path = "" // Invalid: empty path for file output

		_, err := New(cfg)
		if err == nil {
			t.Fatal("New() succeeded with empty path for file output, but expected error")
		}
		if !strings.Contains(err.Error(), "could not parse log output") || !strings.Contains(err.Error(), "internal error: file output mode requires a non-empty path") {
			t.Errorf("Expected error about parsing output or internal error for empty path, got: %v", err)
		}
	})

	t.Run("should_fail_if_parseOutput_fails_to_open_file", func(t *testing.T) {
		cfg := newTestDefaultConfig()
		cfg.Output = config.LogOutputFile
		cfg.Path = "/this/path/should/not/exist/or/be/writable/test.log" // Invalid path

		_, err := New(cfg)
		if err == nil {
			t.Fatal("New() with invalid file path succeeded, but expected error")
		}
		if !strings.Contains(err.Error(), "could not parse log output") || !strings.Contains(err.Error(), "failed to open log file") {
			t.Errorf("Expected error about failing to open log file, got: %v", err)
		}
	})
}

// TestLogger_Close checks if the Close method correctly closes the file writer.
func TestLogger_Close(t *testing.T) {
	t.Run("should_close_file_writer_successfully", func(t *testing.T) {
		tempDir := t.TempDir()
		logFilePath := filepath.Join(tempDir, "test_close.log")
		cfg := &config.LoggingConfig{Output: config.LogOutputFile, Path: logFilePath, Level: config.LogLevelInfo}

		logger, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create logger for TestLogger_Close: %v", err)
		}

		err = logger.Close()
		if err != nil {
			t.Errorf("logger.Close() failed for file writer: %v", err)
		}

		// Try to close again (for *os.File, this should error, testing robustness of app logic around it)
		// However, our Close() method doesn't prevent this internally. The os.File.Close() will error.
		// This tests that the error from the underlying closer is propagated.
		err = logger.Close()
		if err == nil {
			t.Error("logger.Close() on an already closed file writer did not return an error")
		}
	})

	t.Run("should_return_nil_when_closing_nopWriteCloser_for_stdout", func(t *testing.T) {
		cfg := newTestDefaultConfig()
		logger, err := New(cfg) // This will use nopWriteCloser{os.Stdout} due to default in parseOutput
		if err != nil {
			t.Fatalf("Failed to create logger: %v", err)
		}
		err = logger.Close()
		if err != nil {
			t.Errorf("logger.Close() failed for NopWriteCloser (stdout): %v", err)
		}
	})

	t.Run("should_return_nil_when_closer_is_nil_in_logger_struct", func(t *testing.T) {
		// This case is unlikely if New always initializes closer, but tests defensiveness.
		logger := &Logger{closer: nil}
		err := logger.Close()
		if err != nil {
			t.Errorf("logger.Close() with nil closer failed: %v", err)
		}
	})
}

// TestParseOutput checks the Logging Output configuration.
func TestParseOutput(t *testing.T) {
	// Test cases
	tests := []struct {
		name          string                // Name of the test case
		cfg           *config.LoggingConfig // The Logging configuration
		expectError   bool                  // true if an error is expected, false otherwise
		errorContains string                // A substring expected to be in the error message if expectError is true
		expectedOut   io.Writer             // Specific os.File instance for stdout/stderr to compare against
	}{
		{
			name:        "output_stdout_should_use_stdout",
			cfg:         &config.LoggingConfig{Output: config.LogOutputStdout},
			expectError: false,
			expectedOut: os.Stdout,
		},
		{
			name:        "output_stderr_should_use_stderr",
			cfg:         &config.LoggingConfig{Output: config.LogOutputStderr},
			expectError: false,
			expectedOut: os.Stderr,
		},
		{
			name: "output_file_with_valid_path",
			cfg: &config.LoggingConfig{
				Output: config.LogOutputFile,
				Path:   filepath.Join(t.TempDir(), "test_parseoutput.log"),
			},
			expectError: false,
			// expectedOut cannot be directly os.File as it's a new file. Check type and path.
		},
		{
			name:          "output_file_with_empty_path_should_error",
			cfg:           &config.LoggingConfig{Output: config.LogOutputFile, Path: ""},
			expectError:   true,
			errorContains: "internal error: file output mode requires a non-empty path",
		},
		{
			name: "output_file_with_unwritable_path_should_error",
			cfg: &config.LoggingConfig{
				Output: config.LogOutputFile,
				Path:   "/this/path/is/likely/unwritable/test.log",
			},
			expectError:   true,
			errorContains: "failed to open log file",
		},
		{
			name:        "output_default_should_use_stdout",
			cfg:         &config.LoggingConfig{Output: "some_other_value"}, // Default case
			expectError: false,
			expectedOut: os.Stdout,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer, err := parseOutput(tt.cfg)

			hasError := (err != nil)
			if hasError != tt.expectError {
				t.Fatalf("parseOutput() error = %v, expectError %v", err, tt.expectError)
			}

			if tt.expectError {
				if err != nil && tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("parseOutput() error = %q, want error to contain %q", err.Error(), tt.errorContains)
				}
				return // Stop if error was expected and occurred (or not as expected)
			}

			// If no error expected, check the writer type
			if fileCase := strings.Contains(tt.name, "output_file_with_valid_path"); fileCase {
				f, ok := writer.(*os.File)
				if !ok {
					t.Fatalf("Expected *os.File for file output, got %T", writer)
				}
				if f.Name() != tt.cfg.Path {
					t.Errorf("Expected file path %q, got %q", tt.cfg.Path, f.Name())
				}
				// Clean up by closing the file; parseOutput doesn't close, the caller (New->Logger) does.
				// Here, the test itself needs to close what parseOutput returned for this test case.
				closeErr := writer.Close()
				if closeErr != nil {
					t.Errorf("Failed to close file opened by parseOutput: %v", closeErr)
				}
			} else {
				// For stdout/stderr, check if it's a nopWriteCloser wrapping the expected stream
				nwc, ok := writer.(nopWriteCloser)
				if !ok {
					t.Fatalf("Expected nopWriteCloser for stdout/stderr, got %T", writer)
				}
				if nwc.Writer != tt.expectedOut {
					t.Errorf("Expected nopWriteCloser to wrap %v, but wrapped %v", tt.expectedOut, nwc.Writer)
				}
			}
		})
	}
}

// TestParseLevel checks the Logging Level configuration.
func TestParseLevel(t *testing.T) {
	// Tests cases
	tests := []struct {
		name            string          // Name of the subtest
		inputConfigLvl  config.LogLevel // The providedlogging level
		expectedSlogLvl slog.Level      // The expected logging level
	}{
		{"level_debug", config.LogLevel("debug"), slog.LevelDebug},
		{"level_info", config.LogLevel("info"), slog.LevelInfo},
		{"level_warn", config.LogLevel("warn"), slog.LevelWarn},
		{"level_error", config.LogLevel("error"), slog.LevelError},
		{"level_invalid_string", config.LogLevel("verbose"), slog.LevelInfo}, // Default
		{"level_empty_string", config.LogLevel(""), slog.LevelInfo},          // Default
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLevel(tt.inputConfigLvl)
			if got != tt.expectedSlogLvl {
				t.Errorf("parseLevel(%q) = %s, want %s", tt.inputConfigLvl, got, tt.expectedSlogLvl)
			}
		})
	}
}

// TestLogger_LoggingMethods checks individual logger methods (Debug, Info, Warn, Error).
func TestLogger_LoggingMethods(t *testing.T) {
	// Tests cases
	tests := []struct {
		name          string
		logMethod     func(l *Logger, msg string, args ...any)
		handlerLevel  slog.Level // Level to set the test handler to
		logEntryLevel slog.Level // Level of the log entry being made
		expectedMsg   string     // The expected message
		args          []any      // Arbitrary args
		shouldLog     bool       // Whether the message is expected to be logged based on handlerLevel
	}{
		{"text_info_level_log_info", (*Logger).Info, slog.LevelInfo, slog.LevelInfo, "info test", []any{"key", "val"}, true},
		{"text_info_level_log_debug", (*Logger).Debug, slog.LevelInfo, slog.LevelDebug, "debug test", []any{"key", "val"}, false},
		{"text_debug_level_log_debug", (*Logger).Debug, slog.LevelDebug, slog.LevelDebug, "debug test", []any{"key", "val"}, true},
		{"text_warn_level_log_warn", (*Logger).Warn, slog.LevelWarn, slog.LevelWarn, "warn test", []any{"key", "val"}, true},
		{"text_error_level_log_error", (*Logger).Error, slog.LevelError, slog.LevelError, "error test", []any{"key", "val"}, true},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			opts := &slog.HandlerOptions{Level: tt.handlerLevel}
			handler := newCompactTextHandler(&buf, opts)

			logger := &Logger{slogger: slog.New(handler), exitFunc: func(int) {}} // Mock exit

			tt.logMethod(logger, tt.expectedMsg, tt.args...)
			output := buf.String()

			if tt.shouldLog {
				if output == "" {
					t.Fatalf("Expected log output for %q with level %s, but got empty string", tt.expectedMsg, tt.handlerLevel)
				}
				if !strings.Contains(output, tt.expectedMsg) {
					t.Errorf("Output %q does not contain expected message %q", output, tt.expectedMsg)
				}
				if len(tt.args) > 0 {
					if !strings.Contains(output, fmt.Sprintf("%s=%s", tt.args[0], tt.args[1])) &&
						!strings.Contains(output, fmt.Sprintf("%s=%q", tt.args[0], tt.args[1])) {
						t.Errorf("Text output %q does not seem to contain args %v", output, tt.args)
					}
				}

				// Check for level string
				var expectedLevelStr string
				switch tt.logEntryLevel {
				case slog.LevelDebug:
					expectedLevelStr = "DEBUG"
				case slog.LevelInfo:
					expectedLevelStr = "INFO"
				case slog.LevelWarn:
					expectedLevelStr = "WARN"
				case slog.LevelError:
					expectedLevelStr = "ERROR"
				}
				if !strings.Contains(output, "["+expectedLevelStr+"]") {
					t.Errorf("Output %q does not contain expected level string for %s", output, expectedLevelStr)
				}

			} else { // Message should not have been logged
				if output != "" {
					t.Errorf("Expected no log output for %q with handler level %s and entry level %s, but got: %s", tt.expectedMsg, tt.handlerLevel, tt.logEntryLevel, output)
				}
			}
		})
	}
}

// TestLogger_Fatal checks the Fatal logger method.
func TestLogger_Fatal(t *testing.T) {
	var buf bytes.Buffer
	handler := newCompactTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})

	var exitCode = -1
	var exited = false
	var exitMutex sync.Mutex

	mockExitFunc := func(code int) {
		exitMutex.Lock()
		defer exitMutex.Unlock()
		exitCode = code
		exited = true
	}

	logger := &Logger{
		slogger:  slog.New(handler),
		exitFunc: mockExitFunc,
		closer:   nopWriteCloser{&buf}, // Dummy closer
	}

	testMsg := "critical failure happened"
	args := []any{"code", 123, "component", "test"}
	logger.Fatal(testMsg, args...)

	output := buf.String()

	// Check that the message was logged at Error level
	if !strings.Contains(output, "[ERROR]") {
		t.Errorf("Fatal log output %q does not contain '[ERROR]'", output)
	}
	if !strings.Contains(output, testMsg) {
		t.Errorf("Fatal log output %q does not contain message %q", output, testMsg)
	}
	if !strings.Contains(output, "code=123") || !strings.Contains(output, "component=test") {
		t.Errorf("Fatal log output %q does not contain all args", output)
	}

	// Check that the exit function was called
	exitMutex.Lock()
	defer exitMutex.Unlock()
	finalExited := exited
	finalExitCode := exitCode

	if !finalExited {
		t.Error("logger.Fatal() did not call the exit function")
	}
	if finalExitCode != 1 {
		t.Errorf("logger.Fatal() called exit function with code %d, want 1", finalExitCode)
	}
}

package config

import (
	"testing"
)

// TestLoggingConfigValidate checks the Logging Config validation.
func TestLoggingConfigValidate(t *testing.T) {
	// Test cases
	tests := []struct {
		name        string        // Name of the test case
		config      LoggingConfig // The Logging configuration
		expectError bool          // true if an error is expected, false otherwise
	}{
		// Valid tests cases
		{
			name: "valid_file_output_with_path",
			config: LoggingConfig{
				Level:  LogLevelInfo,
				Output: LogOutputFile,
				Path:   "/tmp/test.log",
			},
			expectError: false,
		},
		{
			name: "valid_stdout_output",
			config: LoggingConfig{
				Level:  LogLevelInfo,
				Output: LogOutputStdout,
			},
			expectError: false,
		},
		// Invalid test cases
		{
			name:        "empty_config_struct_should_fail",
			config:      LoggingConfig{},
			expectError: true,
		},
		{
			name:        "invalid_level",
			config:      LoggingConfig{Level: "invalid"},
			expectError: true,
		},
		{
			name:        "invalid_output_destination",
			config:      LoggingConfig{Output: "invalid"},
			expectError: true,
		},
		{
			name: "invalid_file_output_missing_path",
			config: LoggingConfig{
				Output: LogOutputFile,
			},
			expectError: true,
		},
	}

	// Run tests
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.Validate()
			if (got != nil) != tt.expectError {
				if tt.expectError {
					t.Errorf("Config: %+v\nExpected error, but got: %v", tt.config, got)
				} else {
					t.Errorf("Config: %+v\nExpected no error, but got: %v", tt.config, got)
				}
			}
		})
	}
}

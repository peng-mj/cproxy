// Package config provides application configuration management.
//
// This package handles loading and saving application configuration from
// JSON files, as well as creating default configurations when needed.

package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/peng-mj/scproxy/internal/stats"
	"github.com/peng-mj/scproxy/internal/validation"
)

// AppConfig represents the application configuration loaded from JSON file.
type AppConfig struct {
	Proxy   string                   `json:"proxy"`   // Outbound proxy URL
	Host    string                   `json:"host"`    // Server listening host
	Cache   CacheConfig              `json:"cache"`   // Cache configuration
	Logging LoggingConfig            `json:"logging"` // Logging configuration
	DNS     DNSConfig                `json:"dns"`     // DNS proxy configuration
	VHost   VHostConfig              `json:"vhost"`   // Virtual host reverse proxy configuration
	Routes  []validation.RouteConfig `json:"routes"`  // Routes configuration for batch mode
	Summary *stats.AllStats          `json:"summary"` // Statistics summary
}

// DNSConfig holds DNS proxy server configuration.
type DNSConfig struct {
	Enabled  bool     `json:"enabled"`  // Enable DNS server
	Addr     string   `json:"addr"`     // Listen address (e.g. ":53")
	Upstream []string `json:"upstream"` // Upstream DNS servers (e.g. ["8.8.8.8:53"])
	ProxyIP  string   `json:"proxyIP"`  // IP to return for proxied domains (default "127.0.0.1")
}

// VHostConfig holds virtual host reverse proxy configuration.
type VHostConfig struct {
	Enabled bool `json:"enabled"` // Enable virtual host mode
	Port    int  `json:"port"`    // Port to listen on (default 80)
}

// CacheConfig represents cache-specific configuration.
type CacheConfig struct {
	Enabled           bool     `json:"enabled"`           // Enable caching
	Directory         string   `json:"directory"`         // Cache directory path
	MaxTotalSizeMB    int      `json:"maxTotalSizeMB"`    // Maximum total cache size in MB
	MinFileSizeKB     int      `json:"minFileSizeKB"`     // Minimum response size to cache in KB
	MaxFileSizeKB     int      `json:"maxFileSizeKB"`     // Maximum response size to cache in KB
	CacheAuth         bool     `json:"cacheAuth"`         // Cache authenticated requests
	ExcludeExtensions []string `json:"excludeExtensions"` // File extensions to exclude from caching
	ExcludePaths      []string `json:"excludePaths"`      // URL path prefixes to exclude from caching
}

// defaultAppConfig returns the default application configuration.
func defaultAppConfig() *AppConfig {
	return &AppConfig{
		Proxy: "",
		Host:  "0.0.0.0",
		Cache: CacheConfig{
			Enabled:           true,
			Directory:         "./cache",
			MaxTotalSizeMB:    0,
			MinFileSizeKB:     0,
			MaxFileSizeKB:     0,
			CacheAuth:         false,
			ExcludeExtensions: []string{"html", "js", "css", "json", "xml"},
			ExcludePaths:      []string{},
		},
		Logging: LoggingConfig{
			Level:  LogLevelInfo,
			Output: LogOutputStdout,
		},
		DNS: DNSConfig{
			Enabled:  true,
			Addr:     ":53",
			Upstream: []string{"8.8.8.8:53"},
			ProxyIP:  "127.0.0.1",
		},
		VHost: VHostConfig{
			Enabled: true,
			Port:    80,
		},
		Routes: []validation.RouteConfig{},
	}
}

// GetDefaultConfigPath returns the default configuration file path.
func GetDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./config.json"
	}
	return filepath.Join(homeDir, ".scproxy", "config.json")
}

// expandPath expands ~ to user's home directory.
func expandPath(path string) (string, error) {
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %v", err)
		}
		return filepath.Join(homeDir, path[1:]), nil
	}
	return path, nil
}

// LoadAppConfig loads application configuration from a JSON file.
func LoadAppConfig(configPath string) (*AppConfig, error) {
	// Expand ~ in path
	expandedPath, err := expandPath(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to expand config path: %v", err)
	}

	// Read the configuration file
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	// Parse JSON
	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	return &config, nil
}

// SaveAppConfig saves application configuration to a JSON file.
func SaveAppConfig(config *AppConfig, configPath string) error {
	// Expand ~ in path
	expandedPath, err := expandPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to expand config path: %v", err)
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(expandedPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %v", err)
	}

	// Marshal config to JSON with indentation
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %v", err)
	}

	// Write to file with restrictive permissions
	if err := os.WriteFile(expandedPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %v", err)
	}

	return nil
}

// InitDefaultAppConfig creates a default configuration file at the specified path.
func InitDefaultAppConfig(configPath string) error {
	// Check if file already exists
	expandedPath, err := expandPath(configPath)
	if err != nil {
		return fmt.Errorf("failed to expand config path: %v", err)
	}

	if _, err := os.Stat(expandedPath); err == nil {
		// File already exists
		return fmt.Errorf("config file already exists: %s", expandedPath)
	} else if !os.IsNotExist(err) {
		// Some other error
		return fmt.Errorf("failed to check config file: %v", err)
	}

	// Create and save default config
	defaultConfig := defaultAppConfig()
	if err := SaveAppConfig(defaultConfig, configPath); err != nil {
		return fmt.Errorf("failed to save default config: %v", err)
	}

	return nil
}

// Validate checks the validity of the cache configuration.
func (c *CacheConfig) Validate() error {
	if c.Directory == "" {
		return fmt.Errorf("cache directory cannot be empty")
	}

	if c.MaxTotalSizeMB < 0 {
		return fmt.Errorf("max total cache size cannot be negative")
	}

	if c.MinFileSizeKB < 0 {
		return fmt.Errorf("min file size cannot be negative")
	}

	if c.MaxFileSizeKB < 0 {
		return fmt.Errorf("max file size cannot be negative")
	}

	if c.MinFileSizeKB > 0 && c.MaxFileSizeKB > 0 && c.MinFileSizeKB > c.MaxFileSizeKB {
		return fmt.Errorf("min file size (%d KB) cannot be greater than max file size (%d KB)", c.MinFileSizeKB, c.MaxFileSizeKB)
	}

	for _, ext := range c.ExcludeExtensions {
		if ext == "" {
			return fmt.Errorf("exclude extensions cannot contain empty strings")
		}
	}

	for _, pattern := range c.ExcludePaths {
		if pattern == "" {
			return fmt.Errorf("exclude paths cannot contain empty strings")
		}
		if !strings.HasPrefix(pattern, "/") {
			return fmt.Errorf("exclude path must start with '/': %q", pattern)
		}
	}

	return nil
}

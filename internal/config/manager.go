package config

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/peng-mj/scproxy/internal/stats"
)

// ConfigManager manages configuration with thread-safe statistics updates.
type ConfigManager struct {
	configPath string
	mu         sync.RWMutex
}

// NewConfigManager creates a new configuration manager.
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
	}
}

// UpdateStats updates the statistics in the configuration file.
// This uses a read-write lock to ensure thread-safe access.
func (cm *ConfigManager) UpdateStats(allStats *stats.AllStats) error {
	// Lock for writing
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Read current config
	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return err
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}

	// Update statistics
	config.Summary = allStats

	// Marshal back to JSON
	newData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	// Write to temporary file first
	tmpPath := cm.configPath + ".tmp"
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return err
	}

	// Atomic rename
	return os.Rename(tmpPath, cm.configPath)
}

// GetConfig reads the current configuration.
func (cm *ConfigManager) GetConfig() (*AppConfig, error) {
	// Lock for reading
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	return LoadAppConfig(cm.configPath)
}

// GetConfigPath returns the configuration file path.
func (cm *ConfigManager) GetConfigPath() string {
	return cm.configPath
}

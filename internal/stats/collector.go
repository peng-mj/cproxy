package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// ConfigWriter interface for writing statistics to configuration.
type ConfigWriter interface {
	UpdateStats(allStats *AllStats) error
}

// Collector collects and manages statistics.
type Collector struct {
	allStats       AllStats
	mu             sync.RWMutex
	cancelFunc     context.CancelFunc
	configWriter   ConfigWriter
	statsFilePath  string                 // For --summary functionality (separate from config)
	configFilePath string                 // Path to config file for in-place updates
	configData     map[string]interface{} // Cached config content in memory
	stopOnce       sync.Once
}

// NewCollector creates a new statistics collector.
func NewCollector() *Collector {
	return &Collector{
		allStats: AllStats{
			ByDomain: make(map[string]*Stats),
		},
	}
}

// RecordCacheHit records a cache hit and bytes saved for a domain.
func (c *Collector) RecordCacheHit(domain string, size uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Update domain stats
	domainStats := c.allStats.GetOrCreateDomainStats(domain)
	domainStats.CacheHits++
	domainStats.BytesSaved += size
	domainStats.LastUpdated = now

	// Update total stats
	c.allStats.Total.CacheHits++
	c.allStats.Total.BytesSaved += size
	c.allStats.Total.LastUpdated = now
}

// RecordCacheMiss records a cache miss for a domain.
func (c *Collector) RecordCacheMiss(domain string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Update domain stats
	domainStats := c.allStats.GetOrCreateDomainStats(domain)
	domainStats.CacheMisses++
	domainStats.LastUpdated = now

	// Update total stats
	c.allStats.Total.CacheMisses++
	c.allStats.Total.LastUpdated = now
}

// RecordResponse records a response status code for a domain.
func (c *Collector) RecordResponse(domain string, statusCode int) {
	// Only track error responses (4xx and 5xx)
	if statusCode < 400 || statusCode >= 600 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Update domain stats
	domainStats := c.allStats.GetOrCreateDomainStats(domain)
	domainStats.ErrorResponses++
	domainStats.LastUpdated = now

	// Update total stats
	c.allStats.Total.ErrorResponses++
	c.allStats.Total.LastUpdated = now
}

// UpdateCacheSize updates the cache size for a domain.
func (c *Collector) UpdateCacheSize(domain string, size uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	// Update domain stats
	domainStats := c.allStats.GetOrCreateDomainStats(domain)
	domainStats.CacheSizeBytes = size
	domainStats.LastUpdated = now

	// Recalculate total cache size
	var totalSize uint64
	for _, s := range c.allStats.ByDomain {
		totalSize += s.CacheSizeBytes
	}
	c.allStats.Total.CacheSizeBytes = totalSize
	c.allStats.Total.LastUpdated = now
}

// GetSnapshot returns a copy of current statistics.
func (c *Collector) GetSnapshot() AllStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Deep copy to avoid race conditions
	result := AllStats{
		Total:    c.allStats.Total,
		ByDomain: make(map[string]*Stats, len(c.allStats.ByDomain)),
	}

	for domain, stats := range c.allStats.ByDomain {
		statsCopy := *stats
		result.ByDomain[domain] = &statsCopy
	}

	return result
}

// StartPeriodicWrite starts a goroutine that writes stats to file periodically.
func (c *Collector) StartPeriodicWrite(configFilePath string, interval time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.configFilePath = configFilePath

	// Load config file into memory once
	data, err := os.ReadFile(configFilePath)
	if err == nil {
		var config map[string]interface{}
		if json.Unmarshal(data, &config) == nil {
			c.configData = config
		}
	}
	if c.configData == nil {
		c.configData = make(map[string]interface{})
	}

	ctx, cancel := context.WithCancel(context.Background())
	c.cancelFunc = cancel

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		c.syncToDisk()

		for {
			select {
			case <-ticker.C:
				c.syncToDisk()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop stops the periodic writer.
func (c *Collector) Stop() {
	c.stopOnce.Do(func() {
		if c.cancelFunc != nil {
			c.cancelFunc()
		}
		c.syncToDisk()
	})
}

// syncToDisk writes current in-memory stats to the config file.
func (c *Collector) syncToDisk() {
	var configData map[string]interface{}
	var configFilePath string

	c.mu.Lock()

	snapshot := AllStats{
		Total:    c.allStats.Total,
		ByDomain: make(map[string]*Stats, len(c.allStats.ByDomain)),
	}

	for domain, stats := range c.allStats.ByDomain {
		statsCopy := *stats
		snapshot.ByDomain[domain] = &statsCopy
	}

	configData = c.configData
	configFilePath = c.configFilePath
	c.mu.Unlock()

	if configFilePath == "" {
		return
	}

	// Update summary field in memory
	configData["summary"] = snapshot

	// Marshal to JSON
	newData, err := json.MarshalIndent(configData, "", "  ")
	if err != nil {
		return
	}

	// Write to temporary file first, then rename for atomicity
	tmpPath := fmt.Sprintf("%s.tmp.%d", configFilePath, time.Now().UnixNano())
	if err := os.WriteFile(tmpPath, newData, 0644); err != nil {
		return
	}

	if err := os.Rename(tmpPath, configFilePath); err != nil {
		_ = os.Remove(tmpPath)
		return
	}
}

// SetStatsFilePath sets the path for separate stats file (used for --summary).
func (c *Collector) SetStatsFilePath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.statsFilePath = path
}

// GetStatsFilePath returns the path for separate stats file.
func (c *Collector) GetStatsFilePath() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.statsFilePath
}

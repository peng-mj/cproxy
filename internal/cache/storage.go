package cache

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const (
	dataDir = "data"
)

// calculateFileCRC32 calculates the CRC32 checksum of a file.
func calculateFileCRC32(filePath string) (uint32, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	hasher := crc32.NewIEEE()
	if _, err := io.Copy(hasher, file); err != nil {
		return 0, err
	}

	return hasher.Sum32(), nil
}

// calculateDataCRC32 calculates the CRC32 checksum of data bytes.
func calculateDataCRC32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// Storage manages cache storage on the file system.
// Uses SHA256 hash as cache key, stores files using URL path, and maintains a single index file.
type Storage struct {
	cacheDir string
	mu       sync.RWMutex
	index    *CacheIndex // Index manages hash -> file path mapping
}

// NewStorage creates a new cache storage and loads the cache index.
func NewStorage(cacheDir string) (*Storage, error) {
	// Create cache directory structure
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %v", err)
	}

	dataPath := filepath.Join(cacheDir, dataDir)
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	// Create or load cache index
	cacheIndex, err := NewCacheIndex(cacheDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache index: %v", err)
	}

	storage := &Storage{
		cacheDir: cacheDir,
		index:    cacheIndex,
	}

	// Validate index and cleanup invalid entries
	if err := storage.validateAndCleanupIndex(); err != nil {
		// Log warning but don't fail - storage can still work
		fmt.Printf("Warning: index validation failed: %v\n", err)
	}

	return storage, nil
}

// validateAndCleanupIndex validates the index and removes invalid entries.
func (s *Storage) validateAndCleanupIndex() error {
	// Validate index against actual files
	invalidHashes, err := s.index.Validate(s.cacheDir)
	if err != nil {
		return err
	}

	// Remove invalid entries from index
	if len(invalidHashes) > 0 {
		fmt.Printf("Found %d invalid cache entries, cleaning up...\n", len(invalidHashes))
		if err := s.index.Cleanup(invalidHashes); err != nil {
			return fmt.Errorf("failed to cleanup index: %v", err)
		}
	}

	return nil
}

// getFilePath returns the file path for cached data based on host and URL path.
// Host is used as the top-level folder for cache isolation.
func (s *Storage) getFilePath(host string, urlPath string) string {
	// Sanitize host for use as directory name
	safeHost := sanitizePath(host)
	// Generate safe file path from URL path
	safePath := GenerateFilePath(urlPath)
	return filepath.Join(s.cacheDir, dataDir, safeHost, safePath)
}

// Get reads cached data from storage using hash as key.
func (s *Storage) Get(hash string) (*CachedResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entry, exists := s.index.Get(hash)
	if !exists {
		return nil, nil
	}

	dataPath := s.getFilePath(entry.Host, entry.URLPath)

	// Validate file integrity using CRC32
	currentCRC32, err := calculateFileCRC32(dataPath)
	if err != nil {
		return nil, nil
	}
	if currentCRC32 != entry.CRC32 {
		return nil, nil
	}

	headers := make(map[string][]string)
	for k, v := range entry.Metadata.Headers {
		headers[k] = []string{v}
	}

	return &CachedResponse{
		StatusCode: entry.Metadata.StatusCode,
		Headers:    headers,
		Body:       nil,
		FilePath:   dataPath,
	}, nil
}

// Put writes cached data to storage using hash as key and URL path as file location.
func (s *Storage) Put(hash string, host string, urlPath string, response *CachedResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prepare headers map
	headers := make(map[string]string)
	for k, v := range response.Headers {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	// Generate relative file path from URL path, including host for proper validation
	safeHost := sanitizePath(host)
	safePath := GenerateFilePath(urlPath)
	filePath := filepath.Join(safeHost, safePath)

	// Create full file path
	dataPath := s.getFilePath(host, urlPath)

	// Create subdirectories if needed
	if err := os.MkdirAll(filepath.Dir(dataPath), 0755); err != nil {
		return fmt.Errorf("failed to create data subdirectory: %v", err)
	}

	// Write data file
	if err := os.WriteFile(dataPath, response.Body, 0644); err != nil {
		return fmt.Errorf("failed to write data file: %v", err)
	}

	// Calculate CRC32 for integrity check
	crc32Value := calculateDataCRC32(response.Body)

	// Add entry to index
	if err := s.index.Add(hash, host, urlPath, filePath, response.StatusCode, headers, int64(len(response.Body)), crc32Value); err != nil {
		// Clean up data file if index update fails
		os.Remove(dataPath)
		return fmt.Errorf("failed to update index: %v", err)
	}

	return nil
}

// PutFromDisk adds a cache entry for a file that's already on disk.
// This is used for streaming cache writes where the file is written directly.
func (s *Storage) PutFromDisk(hash string, host string, urlPath string, statusCode int, headers map[string][]string, filePath string, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prepare headers map
	headersMap := make(map[string]string)
	for k, v := range headers {
		if len(v) > 0 {
			headersMap[k] = v[0]
		}
	}

	// Calculate CRC32 for integrity check
	dataPath := s.getFilePath(host, urlPath)
	crc32Value, err := calculateFileCRC32(dataPath)
	if err != nil {
		return fmt.Errorf("failed to calculate CRC32: %v", err)
	}

	// Add entry to index
	if err := s.index.Add(hash, host, urlPath, filePath, statusCode, headersMap, size, crc32Value); err != nil {
		return fmt.Errorf("failed to update index: %v", err)
	}

	return nil
}

// Delete removes cached data from storage.
func (s *Storage) Delete(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get entry from index to find file path
	entry, exists := s.index.Get(hash)
	if !exists {
		return nil // Nothing to delete
	}

	// Remove data file
	dataPath := s.getFilePath(entry.Host, entry.URLPath)
	if err := os.Remove(dataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove data file: %v", err)
	}

	// Remove from index
	if err := s.index.Delete(hash); err != nil {
		return fmt.Errorf("failed to update index: %v", err)
	}

	return nil
}

// GetStats returns cache statistics.
func GetStats(cacheDir string) (*CacheStats, error) {
	stats := &CacheStats{}

	index, err := NewCacheIndex(cacheDir)
	if err != nil {
		dataPath := filepath.Join(cacheDir, dataDir)
		err := filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			stats.TotalFiles++
			stats.TotalSizeMB += float64(info.Size()) / (1024 * 1024)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to walk cache directory: %v", err)
		}
		return stats, nil
	}
	defer index.Close()

	stats.TotalFiles = int64(index.Count())
	stats.TotalSizeMB = float64(index.TotalSize()) / (1024 * 1024)

	return stats, nil
}

// GetStats returns cache statistics using the existing index.
func (s *Storage) GetStats() *CacheStats {
	stats := &CacheStats{}
	stats.TotalFiles = int64(s.index.Count())
	stats.TotalSizeMB = float64(s.index.TotalSize()) / (1024 * 1024)
	return stats
}

// GetCacheSize returns the total size of the cache in bytes using the existing index.
func (s *Storage) GetCacheSize() int64 {
	return s.index.TotalSize()
}

// GetSizeForHost returns the total cache size for a specific host in bytes using the existing index.
func (s *Storage) GetSizeForHost(host string) int64 {
	return s.index.TotalSizeForHost(host)
}

// Close closes the storage and releases resources.
func (s *Storage) Close() error {
	if s.index != nil {
		return s.index.Close()
	}
	return nil
}

// Clear removes all cached data.
func Clear(cacheDir string) error {
	dataPath := filepath.Join(cacheDir, dataDir)
	dbPath := filepath.Join(cacheDir, dbFileName)

	// Remove data directory
	if err := os.RemoveAll(dataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove data directory: %v", err)
	}

	// Remove index database
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove index database: %v", err)
	}

	// Recreate empty data directory
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return fmt.Errorf("failed to recreate data directory: %v", err)
	}

	return nil
}

// GetCacheSize returns the total size of the cache in bytes.
func GetCacheSize(cacheDir string) (int64, error) {
	index, err := NewCacheIndex(cacheDir)
	if err != nil {
		var size int64
		dataPath := filepath.Join(cacheDir, dataDir)
		err := filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		return size, nil
	}
	defer index.Close()

	return index.TotalSize(), nil
}

// GetSizeForHost returns the total cache size for a specific host in bytes.
func GetSizeForHost(cacheDir string, host string) (int64, error) {
	index, err := NewCacheIndex(cacheDir)
	if err != nil {
		var size int64
		dataPath := filepath.Join(cacheDir, dataDir, sanitizePath(host))
		err := filepath.Walk(dataPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				size += info.Size()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		return size, nil
	}
	defer index.Close()

	return index.TotalSizeForHost(host), nil
}

// GetIndex returns a copy of the cache index.
func (s *Storage) GetIndex() map[string]*IndexEntry {
	return s.index.GetAll()
}

// rebuildIndexLocked validates and cleans up the index.
// Must be called with mu.Lock() held.
func (s *Storage) rebuildIndexLocked() error {
	// Validate index against actual files and remove invalid entries
	invalidHashes, err := s.index.Validate(s.cacheDir)
	if err != nil {
		return err
	}

	// Remove invalid entries from index
	if len(invalidHashes) > 0 {
		s.index.Cleanup(invalidHashes)
	}

	return nil
}

// RebuildIndex rebuilds the cache index by validating all entries.
func (s *Storage) RebuildIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rebuildIndexLocked()
}

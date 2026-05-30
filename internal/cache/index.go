package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	indexFileName = "cache_index.json"
)

// CacheIndex maintains the mapping between cache keys and cached files.
type CacheIndex struct {
	mu       sync.RWMutex
	filePath string
	entries  map[string]*IndexEntry // hash -> entry
}

// IndexEntry represents a single cache entry in the index.
type IndexEntry struct {
	Hash     string           `json:"hash"`     // SHA256 hash key
	Host     string           `json:"host"`     // Host for cache isolation (not in hash)
	URLPath  string           `json:"urlPath"`  // Original URL path
	FilePath string           `json:"filePath"` // Relative path to cached file
	Created  int64            `json:"created"`  // Creation timestamp
	Accessed int64            `json:"accessed"` // Last access timestamp
	Size     int64            `json:"size"`     // File size in bytes
	Expires  int64            `json:"expires"`  // Expiration timestamp (0 = never)
	Metadata ResponseMetadata `json:"metadata"` // Response metadata
}

// ResponseMetadata contains HTTP response metadata.
type ResponseMetadata struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
}

// NewCacheIndex creates a new cache index.
func NewCacheIndex(cacheDir string) (*CacheIndex, error) {
	indexFilePath := filepath.Join(cacheDir, indexFileName)

	index := &CacheIndex{
		filePath: indexFilePath,
		entries:  make(map[string]*IndexEntry),
	}

	// Load existing index if it exists
	if err := index.Load(); err != nil {
		// If index doesn't exist, create empty index
		if os.IsNotExist(err) {
			return index, nil
		}
		return nil, fmt.Errorf("failed to load cache index: %v", err)
	}

	return index, nil
}

// Load loads the index from disk.
func (idx *CacheIndex) Load() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	file, err := os.Open(idx.filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	var data struct {
		Entries []IndexEntry `json:"entries"`
		Version int          `json:"version"`
	}

	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode index: %v", err)
	}

	// Rebuild entries map
	idx.entries = make(map[string]*IndexEntry)
	for _, entry := range data.Entries {
		idx.entries[entry.Hash] = &entry
	}

	return nil
}

// Save saves the index to disk.
func (idx *CacheIndex) Save() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.saveLocked()
}

// saveLocked saves the index to disk without locking (must be called with lock held).
func (idx *CacheIndex) saveLocked() error {
	// Create temporary file with unique ID to avoid conflicts in parallel scenarios
	tempPath := idx.filePath + ".tmp"
	file, err := os.Create(tempPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}

	// Prepare data
	data := struct {
		Entries []IndexEntry `json:"entries"`
		Version int          `json:"version"`
	}{
		Version: 1,
		Entries: make([]IndexEntry, 0, len(idx.entries)),
	}

	for _, entry := range idx.entries {
		data.Entries = append(data.Entries, *entry)
	}

	// Write to file
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		file.Close()
		// Ignore remove errors - file might not exist or be locked by another process
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to encode index: %v", err)
	}

	// Sync to disk before closing
	if err := file.Sync(); err != nil {
		file.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to sync temp file: %v", err)
	}

	file.Close()

	// Atomic rename
	if err := os.Rename(tempPath, idx.filePath); err != nil {
		// Clean up temp file if rename fails
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %v", err)
	}

	return nil
}

// Add adds a new entry to the index.
func (idx *CacheIndex) Add(hash string, host string, urlPath string, filePath string, statusCode int, headers map[string]string, size int64) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	now := time.Now().Unix()

	idx.entries[hash] = &IndexEntry{
		Hash:     hash,
		Host:     host,
		URLPath:  urlPath,
		FilePath: filePath,
		Created:  now,
		Accessed: now,
		Size:     size,
		Expires:  0, // No expiration by default
		Metadata: ResponseMetadata{
			StatusCode: statusCode,
			Headers:    headers,
		},
	}

	return idx.saveLocked()
}

// Get retrieves an entry from the index.
func (idx *CacheIndex) Get(hash string) (*IndexEntry, bool) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	entry, exists := idx.entries[hash]
	if !exists {
		return nil, false
	}

	entry.Accessed = time.Now().Unix()

	return entry, true
}

// Delete removes an entry from the index.
func (idx *CacheIndex) Delete(hash string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, exists := idx.entries[hash]; !exists {
		return nil // Entry doesn't exist
	}

	delete(idx.entries, hash)
	return idx.saveLocked()
}

// Validate validates all entries in the index against actual files.
func (idx *CacheIndex) Validate(cacheDir string) ([]string, error) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var invalidHashes []string
	dataDir := filepath.Join(cacheDir, "data")

	for hash, entry := range idx.entries {
		// Check if file exists
		filePath := filepath.Join(dataDir, entry.FilePath)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			// File doesn't exist, mark as invalid
			invalidHashes = append(invalidHashes, hash)
			continue
		}

		// Check if file size matches
		info, err := os.Stat(filePath)
		if err != nil {
			invalidHashes = append(invalidHashes, hash)
			continue
		}

		if info.Size() != entry.Size {
			// File size mismatch, mark as invalid
			invalidHashes = append(invalidHashes, hash)
		}
	}

	return invalidHashes, nil
}

// Cleanup removes invalid entries from the index.
func (idx *CacheIndex) Cleanup(invalidHashes []string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, hash := range invalidHashes {
		delete(idx.entries, hash)
	}

	return idx.saveLocked()
}

// GetAll returns all entries in the index.
func (idx *CacheIndex) GetAll() map[string]*IndexEntry {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	result := make(map[string]*IndexEntry, len(idx.entries))
	for k, v := range idx.entries {
		result[k] = v
	}

	return result
}

// Count returns the number of entries in the index.
func (idx *CacheIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	return len(idx.entries)
}

// TotalSize returns the total size of all cached files.
func (idx *CacheIndex) TotalSize() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var total int64
	for _, entry := range idx.entries {
		total += entry.Size
	}

	return total
}

// TotalSizeForHost returns the total size of cached files for a specific host.
func (idx *CacheIndex) TotalSizeForHost(host string) int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var total int64
	for _, entry := range idx.entries {
		if entry.Host == host {
			total += entry.Size
		}
	}

	return total
}

// GetLRUEntry returns the least recently used entry.
func (idx *CacheIndex) GetLRUEntry() (*IndexEntry, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(idx.entries) == 0 {
		return nil, false
	}

	var oldest *IndexEntry
	for _, entry := range idx.entries {
		if oldest == nil || entry.Accessed < oldest.Accessed {
			oldest = entry
		}
	}

	return oldest, true
}

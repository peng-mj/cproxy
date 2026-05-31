package cache

import (
	"io"
	"net/http"
	"sync/atomic"

	"github.com/Madh93/prxy/internal/config"
)

// Cache represents an HTTP cache.
type Cache struct {
	storage *Storage
	config  config.CacheConfig
	stats   CacheStats
}

// New creates a new cache instance.
func New(config config.CacheConfig) (*Cache, error) {
	storage, err := NewStorage(config.Directory)
	if err != nil {
		return nil, err
	}

	return &Cache{
		storage: storage,
		config:  config,
		stats: CacheStats{
			Hits:   0,
			Misses: 0,
		},
	}, nil
}

// Get retrieves a cached response.
func (c *Cache) Get(key string) (*CachedResponse, error) {
	response, err := c.storage.Get(key)
	if err != nil {
		return nil, err
	}

	if response == nil {
		// Cache miss
		atomic.AddInt64(&c.stats.Misses, 1)
		return nil, nil
	}

	// Cache hit
	atomic.AddInt64(&c.stats.Hits, 1)

	return response, nil
}

// Put stores a response in the cache.
func (c *Cache) Put(hash string, host string, urlPath string, response *CachedResponse) error {
	return c.storage.Put(hash, host, urlPath, response)
}

// PutFromDisk stores a reference to a file that's already on disk.
// Used for streaming cache writes where the file is written directly.
func (c *Cache) PutFromDisk(hash string, host string, urlPath string, statusCode int, headers map[string][]string, filePath string, size int64) error {
	return c.storage.PutFromDisk(hash, host, urlPath, statusCode, headers, filePath, size)
}

// Delete removes a response from the cache.
func (c *Cache) Delete(key string) error {
	return c.storage.Delete(key)
}

// GetStats returns cache statistics.
func (c *Cache) GetStats() *CacheStats {
	fileStats := c.storage.GetStats()
	c.stats.TotalFiles = fileStats.TotalFiles
	c.stats.TotalSizeMB = fileStats.TotalSizeMB

	return &CacheStats{
		Hits:        atomic.LoadInt64(&c.stats.Hits),
		Misses:      atomic.LoadInt64(&c.stats.Misses),
		TotalFiles:  c.stats.TotalFiles,
		TotalSizeMB: c.stats.TotalSizeMB,
	}
}

// Close performs cleanup operations.
func (c *Cache) Close() error {
	if c.storage != nil {
		return c.storage.Close()
	}
	return nil
}

// WriteResponse writes an HTTP response to a CachedResponse structure.
func WriteResponse(resp *http.Response) (*CachedResponse, error) {
	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Copy headers
	headers := make(map[string][]string)
	for k, v := range resp.Header {
		headers[k] = v
	}

	return &CachedResponse{
		StatusCode: resp.StatusCode,
		Headers:    headers,
		Body:       body,
	}, nil
}

// ResponseToHTTP converts a CachedResponse to an HTTP response.
func ResponseToHTTP(cached *CachedResponse, w http.ResponseWriter) {
	// Set headers
	for k, v := range cached.Headers {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	// Set cache status header
	w.Header().Set("X-Cache", "HIT")

	// Write status code
	w.WriteHeader(cached.StatusCode)

	// Write body
	w.Write(cached.Body)
}

// StreamResponse streams an HTTP response to both the client and cache.
func StreamResponse(resp *http.Response, w http.ResponseWriter, hash string, host string, urlPath string, cache *Cache) error {
	// Create a tee reader to stream to both client and cache
	bodyReader := io.TeeReader(resp.Body, w)

	// Read all data
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return err
	}

	if cache != nil && ShouldCacheResponse(resp, cache.config) {
		size := int64(len(body))
		minSize := int64(cache.config.MinFileSizeKB) * 1024
		maxSize := int64(cache.config.MaxFileSizeKB) * 1024

		// Check file size limits (0 means no limit)
		if (minSize == 0 || size >= minSize) && (maxSize == 0 || size <= maxSize) {
			cached := &CachedResponse{
				StatusCode: resp.StatusCode,
				Headers:    make(map[string][]string),
				Body:       body,
			}

			for k, v := range resp.Header {
				cached.Headers[k] = v
			}

			if err := cache.Put(hash, host, urlPath, cached); err != nil {
				return err
			}

			// Check and evict if cache exceeds limit after adding new entry
			if cache.config.MaxTotalSizeMB > 0 {
				if err := cache.CheckAndEvict(); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// GetSize returns the total cache size in bytes.
func (c *Cache) GetSize() (int64, error) {
	return c.storage.GetCacheSize(), nil
}

// GetSizeForHost returns the cache size for a specific host in bytes.
func (c *Cache) GetSizeForHost(host string) (int64, error) {
	return c.storage.GetSizeForHost(host), nil
}

// EvictLRU removes least recently used items to free up space.
func (c *Cache) EvictLRU(requiredSpace int64) error {
	storage := c.storage

	for requiredSpace > 0 {
		// Get LRU entry from index
		index := storage.GetIndex()
		if len(index) == 0 {
			// No more items to evict
			break
		}

		// Find LRU entry
		var oldestKey string
		var oldestAccessed int64
		for key, entry := range index {
			if oldestKey == "" || entry.Accessed < oldestAccessed {
				oldestKey = key
				oldestAccessed = entry.Accessed
			}
		}

		if oldestKey == "" {
			break
		}

		// Get entry to know its size
		entry, exists := index[oldestKey]
		if !exists {
			continue
		}

		// Delete the entry
		if err := c.Delete(oldestKey); err != nil {
			return err
		}

		// Reduce required space
		requiredSpace -= entry.Size
	}

	return nil
}

// CheckAndEvict checks if the cache exceeds the maximum size
// and evicts items if necessary.
func (c *Cache) CheckAndEvict() error {
	// No limit set
	if c.config.MaxTotalSizeMB <= 0 {
		return nil
	}

	currentSize, err := c.GetSize()
	if err != nil {
		return err
	}

	maxSize := int64(c.config.MaxTotalSizeMB) * 1024 * 1024

	if currentSize > maxSize {
		requiredSpace := currentSize - maxSize
		return c.EvictLRU(requiredSpace)
	}

	return nil
}

// CacheConfig returns the cache configuration.
func (c *Cache) CacheConfig() config.CacheConfig {
	return c.config
}

// Directory returns the cache directory.
func (c *Cache) Directory() string {
	return c.config.Directory
}

// Exists checks if a cache key exists.
func (c *Cache) Exists(key string) bool {
	resp, err := c.Get(key)
	return err == nil && resp != nil
}

// RebuildIndex rebuilds the cache index by validating all entries.
func (c *Cache) RebuildIndex() error {
	return c.storage.RebuildIndex()
}

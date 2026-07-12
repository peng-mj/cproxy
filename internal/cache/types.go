// Package cache provides HTTP caching functionality for the scproxy proxy server.
//
// This package implements a file-based HTTP cache with support for cache policies,
// concurrent access, and cache invalidation strategies.

package cache

// CacheStats represents cache statistics.
type CacheStats struct {
	TotalFiles  int64   // Total number of cached files
	TotalSizeMB float64 // Total cache size in MB
	Hits        int64   // Number of cache hits
	Misses      int64   // Number of cache misses
}

// CachedResponse represents a cached HTTP response.
type CachedResponse struct {
	StatusCode int                 // HTTP status code
	Headers    map[string][]string // HTTP headers
	Body       []byte              // Response body (for small files in memory)
	FilePath   string              // File path (for large files on disk)
}

// CacheKey represents a cache key.
type CacheKey struct {
	Method        string // HTTP method
	Host          string // Request host (included in hash for domain isolation)
	Path          string // Request path (without domain)
	Query         string // Query string
	Authorization string // Authorization header (if caching auth requests)
}

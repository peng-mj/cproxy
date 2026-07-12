package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
)

// GenerateCacheKey generates a SHA256 cache key from an HTTP request.
// The key is based on the method, host, path, query, and authorization header (if enabled).
// Host is included in the hash generation to ensure unique keys across different domains.
// It returns a SHA256 hash that will be used as the cache key.
func GenerateCacheKey(req *http.Request, host string, cacheAuth bool) (string, error) {
	// Create cache key structure
	key := CacheKey{
		Method:        req.Method,
		Host:          host,
		Path:          normalizePath(req.URL.Path),
		Query:         req.URL.RawQuery,
		Authorization: "",
	}

	// Include authorization header if caching auth requests
	if cacheAuth {
		key.Authorization = req.Header.Get("Authorization")
	}

	// Generate hash from cache key (host is included in hash for domain isolation)
	hash, err := hashCacheKey(key)
	if err != nil {
		return "", err
	}

	return hash, nil
}

// hashCacheKey generates a SHA256 hash from a CacheKey.
// Host is included in the hash to ensure unique keys across different domains.
func hashCacheKey(key CacheKey) (string, error) {
	h := sha256.New()

	// Write key components to hash (including Host for domain isolation)
	io.WriteString(h, key.Method)
	io.WriteString(h, key.Host)
	io.WriteString(h, key.Path)
	io.WriteString(h, key.Query)
	io.WriteString(h, key.Authorization)

	// Return hex encoded hash
	return hex.EncodeToString(h.Sum(nil)), nil
}

// GenerateFilePath generates a safe file path from URL path for storage.
func GenerateFilePath(urlPath string) string {
	// Normalize path to collapse multiple consecutive slashes
	path := normalizePath(urlPath)

	// Remove leading slash
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}

	// Handle empty path (root)
	if path == "" || path == "/" {
		path = "root"
	}

	// Sanitize the path to make it safe for file systems
	path = sanitizePath(path)

	// If path ends with a slash (directory), add index.html as the file name
	// This prevents issues on Windows where renaming a file to an existing directory fails
	if len(path) > 0 && (path[len(path)-1] == '/' || path[len(path)-1] == '\\') {
		path = path + "index.html"
	}

	return path
}

// sanitizePath makes a string safe for use as a file path.
func sanitizePath(path string) string {
	return SanitizePath(path)
}

// SanitizePath makes a string safe for use as a file path (exported version).
func SanitizePath(path string) string {
	// Replace potentially dangerous characters
	replacer := strings.NewReplacer(
		"..", "", // Remove parent directory references
		"\\", "", // Remove backslashes
		"\x00", "", // Remove null bytes
		"|", "_", // Replace pipes with underscores
		"*", "_", // Replace asterisks with underscores
		"?", "_", // Replace question marks with underscores
		"<", "_", // Replace less than with underscores
		">", "_", // Replace greater than with underscores
		":", "_", // Replace colons with underscores
		"\"", "_", // Replace quotes with underscores
	)

	result := replacer.Replace(path)

	// Ensure the result is not empty
	if result == "" {
		return "root"
	}

	return result
}

// normalizePath normalizes a URL path by collapsing multiple consecutive slashes into a single slash.
// This ensures that paths like "/path//to///resource" and "/path/to/resource" generate the same cache key.
func normalizePath(path string) string {
	// Use a loop to replace multiple consecutive slashes with a single slash
	for strings.Contains(path, "//") {
		path = strings.ReplaceAll(path, "//", "/")
	}
	return path
}

// IsExcludedExtension checks if a file extension is in the exclusion list.
func IsExcludedExtension(path string, excludeExtensions []string) bool {
	// Extract extension from path
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return false
	}

	ext := strings.ToLower(parts[len(parts)-1])

	// Check against exclusion list
	for _, excluded := range excludeExtensions {
		if ext == strings.ToLower(excluded) {
			return true
		}
	}

	return false
}

// HasFileExtension checks if the path ends with any of the specified file extensions.
// It supports both simple extensions (e.g., ".zip") and compound extensions (e.g., ".tar.gz").
// Compound extensions should be listed first to ensure correct matching.
// The comparison is case-insensitive.
func HasFileExtension(path string, extensions []string) bool {
	lowerPath := strings.ToLower(path)

	for _, ext := range extensions {
		if strings.HasSuffix(lowerPath, strings.ToLower(ext)) {
			return true
		}
	}

	return false
}

// ParseCacheKey parses a cache key back into its components.
// This is useful for debugging and displaying cache information.
func ParseCacheKey(hash string) (*CacheKeyInfo, error) {
	info := &CacheKeyInfo{
		Hash: hash,
	}

	// We can't easily parse the hash back to components,
	// but we can provide metadata about the hash
	info.Method = "GET"   // Default assumption
	info.Path = "unknown" // Can't be recovered from hash

	return info, nil
}

// CacheKeyInfo represents parsed cache key information.
type CacheKeyInfo struct {
	Hash   string
	Method string
	Path   string
}

// URL returns the URL information (limited for hash-based keys).
func (cki *CacheKeyInfo) URL() string {
	return cki.Path
}

// ShortKey returns a shortened version of the cache key for logging (first 16 chars).
func ShortKey(key string) string {
	if len(key) <= 16 {
		return key
	}
	return key[:16]
}

package cache

import (
	"net/http"
	"strings"

	"github.com/peng-mj/scproxy/internal/config"
)

// ShouldCacheRequest determines if a request should be cached.
func ShouldCacheRequest(req *http.Request, cacheConfig config.CacheConfig) bool {
	// Only cache GET and HEAD requests by default
	if req.Method != "GET" && req.Method != "HEAD" {
		return false
	}

	// Check URL path exclusion (suffix match against the end of the path);
	// IsExcludedLastPfx normalizes directory paths and patterns to their
	// index document form ("/foo/" -> "/foo/index.html") before matching.
	if IsExcludedLastPfx(req.URL.Path, cacheConfig.ExcludeLastPfx) {
		return false
	}

	// Don't cache requests with Range header
	if req.Header.Get("Range") != "" {
		return false
	}

	return true
}

// IsExcludedLastPfx checks if a URL path is excluded.
// Every pattern is matched case-insensitively against the end of the path,
// e.g. "index.html" matches "/aa/bb/cc/index.html", ".html" matches any
// path ending with ".html", and "/index.html" matches any path whose
// trailing segments end with "/index.html".
// Paths and patterns are normalized to their index document form first
// ("/foo/" and "/foo/index.html" are equivalent), so a directory path is
// excluded by "index.html" and a directory pattern ("/ubuntu/dists/")
// keeps matching directory requests.
func IsExcludedLastPfx(path string, patterns []string) bool {
	lowerPath := strings.ToLower(normalizePath(path))

	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		if strings.HasSuffix(lowerPath, strings.ToLower(normalizePath(pattern))) {
			return true
		}
	}

	return false
}

// ShouldCacheResponse determines if a response should be cached.
func ShouldCacheResponse(resp *http.Response, cacheConfig config.CacheConfig) bool {
	// Handle 304 Not Modified responses
	// 304 responses should not be cached as they don't contain a body
	// The browser will use its cached version if it receives 304
	if resp.StatusCode == http.StatusNotModified {
		return false
	}

	// Only cache successful responses (2xx)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}

	// Check Cache-Control header
	cacheControl := resp.Header.Get("Cache-Control")
	if cacheControl != "" {
		// Don't cache if no-store or no-cache directives are present
		directives := strings.Split(cacheControl, ",")
		for _, directive := range directives {
			directive = strings.TrimSpace(strings.ToLower(directive))
			if strings.HasPrefix(directive, "no-store") ||
				strings.HasPrefix(directive, "no-cache") ||
				strings.HasPrefix(directive, "private") {
				return false
			}
		}
	}

	// Don't cache partial content
	if resp.StatusCode == http.StatusPartialContent {
		return false
	}

	return true
}

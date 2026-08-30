package cache

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/peng-mj/scproxy/internal/config"
)

// TestGenerateCacheKey_HostInHash verifies that different hosts generate different hashes
// for the same request path/query/method (host is included in hash for domain isolation).
func TestGenerateCacheKey_HostInHash(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		path      string
		query     string
		auth      string
		cacheAuth bool
	}{
		{
			name:      "simple GET request",
			method:    "GET",
			path:      "/path/to/resource",
			query:     "",
			auth:      "",
			cacheAuth: false,
		},
		{
			name:      "GET with query",
			method:    "GET",
			path:      "/search",
			query:     "q=test&page=1",
			auth:      "",
			cacheAuth: false,
		},
		{
			name:      "POST request",
			method:    "POST",
			path:      "/api/data",
			query:     "",
			auth:      "",
			cacheAuth: false,
		},
		{
			name:      "with authorization",
			method:    "GET",
			path:      "/private/data",
			query:     "",
			auth:      "Bearer token123",
			cacheAuth: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts := []string{
				"example.com",
				"mirrors.aliyun.com",
				"api.github.com",
				"localhost:8080",
			}

			hashes := make(map[string]string)
			// Generate hash for each host
			for _, host := range hosts {
				req := newRequest(t, tt.method, "https://"+host+tt.path, tt.query, tt.auth)
				hash, err := GenerateCacheKey(req, host, tt.cacheAuth)
				if err != nil {
					t.Fatalf("GenerateCacheKey failed for host %s: %v", host, err)
				}
				hashes[host] = hash
			}

			// All hosts should produce different hashes
			for i, h1 := range hosts {
				for _, h2 := range hosts[i+1:] {
					if hashes[h1] == hashes[h2] {
						t.Errorf("Hosts %s and %s should generate different hashes, both got %s",
							h1, h2, hashes[h1])
					}
				}
			}
		})
	}
}

// TestGenerateCacheKey_DifferentPathsDifferentHashes verifies that different paths
// generate different hashes.
func TestGenerateCacheKey_DifferentPathsDifferentHashes(t *testing.T) {
	req1 := newRequest(t, "GET", "https://example.com/path1", "", "")
	req2 := newRequest(t, "GET", "https://example.com/path2", "", "")

	hash1, err := GenerateCacheKey(req1, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	hash2, err := GenerateCacheKey(req2, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	if hash1 == hash2 {
		t.Errorf("Different paths should generate different hashes: both got %s", hash1)
	}
}

// TestGenerateCacheKey_DifferentMethodsDifferentHashes verifies that different methods
// generate different hashes.
func TestGenerateCacheKey_DifferentMethodsDifferentHashes(t *testing.T) {
	req1 := newRequest(t, "GET", "https://example.com/path", "", "")
	req2 := newRequest(t, "POST", "https://example.com/path", "", "")

	hash1, err := GenerateCacheKey(req1, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	hash2, err := GenerateCacheKey(req2, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	if hash1 == hash2 {
		t.Errorf("Different methods should generate different hashes: both got %s", hash1)
	}
}

// TestGenerateCacheKey_DifferentQueriesDifferentHashes verifies that different queries
// generate different hashes.
func TestGenerateCacheKey_DifferentQueriesDifferentHashes(t *testing.T) {
	req1 := newRequest(t, "GET", "https://example.com/path", "a=1", "")
	req2 := newRequest(t, "GET", "https://example.com/path", "a=2", "")

	hash1, err := GenerateCacheKey(req1, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	hash2, err := GenerateCacheKey(req2, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	if hash1 == hash2 {
		t.Errorf("Different queries should generate different hashes: both got %s", hash1)
	}
}

// TestGenerateCacheKey_CacheAuthEnabled verifies that authorization header is included
// when cacheAuth is enabled.
func TestGenerateCacheKey_CacheAuthEnabled(t *testing.T) {
	req1 := newRequest(t, "GET", "https://example.com/path", "", "Bearer token1")
	req2 := newRequest(t, "GET", "https://example.com/path", "", "Bearer token2")

	hash1, err := GenerateCacheKey(req1, "example.com", true)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	hash2, err := GenerateCacheKey(req2, "example.com", true)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	if hash1 == hash2 {
		t.Errorf("Different auth headers should generate different hashes when cacheAuth is enabled: both got %s", hash1)
	}
}

// TestGenerateCacheKey_CacheAuthDisabled verifies that authorization header is NOT included
// when cacheAuth is disabled.
func TestGenerateCacheKey_CacheAuthDisabled(t *testing.T) {
	req1 := newRequest(t, "GET", "https://example.com/path", "", "Bearer token1")
	req2 := newRequest(t, "GET", "https://example.com/path", "", "Bearer token2")

	hash1, err := GenerateCacheKey(req1, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	hash2, err := GenerateCacheKey(req2, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Different auth headers should generate same hash when cacheAuth is disabled: got %s and %s", hash1, hash2)
	}
}

// TestGenerateCacheKey_PathNormalization verifies that paths are normalized before hashing.
func TestGenerateCacheKey_PathNormalization(t *testing.T) {
	req1 := newRequest(t, "GET", "https://example.com/path//to///resource", "", "")
	req2 := newRequest(t, "GET", "https://example.com/path/to/resource", "", "")

	hash1, err := GenerateCacheKey(req1, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	hash2, err := GenerateCacheKey(req2, "example.com", false)
	if err != nil {
		t.Fatalf("GenerateCacheKey failed: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Normalized paths should generate same hash: got %s and %s", hash1, hash2)
	}
}

// newRequest creates a test HTTP request.
func newRequest(t *testing.T, method, rawURL, query, auth string) *http.Request {
	t.Helper()

	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Failed to parse URL %s: %v", rawURL, err)
	}

	u.RawQuery = query

	req := &http.Request{
		Method: method,
		URL:    u,
		Header: make(http.Header),
	}

	if auth != "" {
		req.Header.Set("Authorization", auth)
	}

	return req
}

// TestGetFilePath_HostIsolation verifies that different hosts create different directory paths.
func TestGetFilePath_HostIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	path := "/ubuntu/pool/main/f/file.tar.xz"

	hosts := []string{
		"mirrors.aliyun.com",
		"archive.ubuntu.com",
		"example.com",
	}

	paths := make(map[string]string)
	for _, host := range hosts {
		filePath := storage.getFilePath(host, path)
		paths[host] = filePath
	}

	// All paths should be different
	for i, h1 := range hosts {
		for _, h2 := range hosts[i+1:] {
			if paths[h1] == paths[h2] {
				t.Errorf("Hosts %s and %s should have different paths, both got %s", h1, h2, paths[h1])
			}
		}
	}

	// Verify that host appears in the path and paths are properly structured
	for host, filePath := range paths {
		// The full path should be: tmpDir/data/sanitized_host/ubuntu/pool/main/f/file.tar.xz
		expectedPath := filepath.Join(tmpDir, "data", sanitizePath(host), "ubuntu", "pool", "main", "f", "file.tar.xz")
		if filePath != expectedPath {
			t.Errorf("For host %s: expected path %s, got %s", host, expectedPath, filePath)
		}
	}
}

// TestGetFilePath_SameHostSamePath verifies that same host and path produce same file path.
func TestGetFilePath_SameHostSamePath(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	host := "mirrors.aliyun.com"
	path := "/ubuntu/pool/main/f/file.tar.xz"

	path1 := storage.getFilePath(host, path)
	path2 := storage.getFilePath(host, path)

	if path1 != path2 {
		t.Errorf("Same host and path should produce same file path: got %s and %s", path1, path2)
	}
}

// TestGetFilePath_HostSanitization verifies that hosts are properly sanitized.
func TestGetFilePath_HostSanitization(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	tests := []struct {
		host           string
		expectedInPath string // String that should appear in the path
	}{
		{
			host:           "example.com",
			expectedInPath: "example.com",
		},
		{
			host:           "mirrors.aliyun.com",
			expectedInPath: "mirrors.aliyun.com",
		},
		{
			host:           "localhost:8080",
			expectedInPath: "localhost_8080", // colon should be replaced
		},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			path := storage.getFilePath(tt.host, "/test/path")
			if !contains(path, tt.expectedInPath) {
				t.Errorf("Expected %q to contain %q", path, tt.expectedInPath)
			}
		})
	}
}

// TestStorage_Put_HostIsolation verifies that Put stores files in host-isolated directories.
func TestStorage_Put_HostIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	response := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"application/octet-stream"}},
		Body:       []byte("test content"),
	}

	// Put same path with different hosts
	hosts := []string{"example.com", "mirrors.aliyun.com"}
	urlPath := "/test/file.bin"

	for i, host := range hosts {
		hash := "hash" + string(rune('0'+i))
		err := storage.Put(hash, host, urlPath, response)
		if err != nil {
			t.Fatalf("Put failed for host %s: %v", host, err)
		}
	}

	// Verify files are in different directories
	for _, host := range hosts {
		expectedPath := storage.getFilePath(host, urlPath)
		if _, err := os.Stat(expectedPath); err != nil {
			t.Errorf("File for host %s should exist at %s: %v", host, expectedPath, err)
		}
	}
}

// TestStorage_Get_HostIsolation verifies that Get retrieves files from host-isolated directories.
func TestStorage_Get_HostIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	host1 := "example.com"
	host2 := "mirrors.aliyun.com"
	urlPath := "/greeting.txt"

	// Put same path with different hosts but different content
	hash1 := "hash1"
	hash2 := "hash2"

	response1 := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       []byte("content from " + host1),
	}

	response2 := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       []byte("content from " + host2),
	}

	if err := storage.Put(hash1, host1, urlPath, response1); err != nil {
		t.Fatalf("Put failed for host1: %v", err)
	}

	if err := storage.Put(hash2, host2, urlPath, response2); err != nil {
		t.Fatalf("Put failed for host2: %v", err)
	}

	// Get entries
	cached1, err := storage.Get(hash1)
	if err != nil {
		t.Fatalf("Get failed for hash1: %v", err)
	}

	cached2, err := storage.Get(hash2)
	if err != nil {
		t.Fatalf("Get failed for hash2: %v", err)
	}

	// Read file contents since Body is nil for file-based responses
	body1, err := os.ReadFile(cached1.FilePath)
	if err != nil {
		t.Fatalf("Failed to read file for hash1: %v", err)
	}

	body2, err := os.ReadFile(cached2.FilePath)
	if err != nil {
		t.Fatalf("Failed to read file for hash2: %v", err)
	}

	if string(body1) != string(response1.Body) {
		t.Errorf("Expected body %q, got %q", string(response1.Body), string(body1))
	}

	if string(body2) != string(response2.Body) {
		t.Errorf("Expected body %q, got %q", string(response2.Body), string(body2))
	}

	// Verify the files are in different host directories
	if cached1.FilePath == cached2.FilePath {
		t.Errorf("Files should be in different paths, both got %s", cached1.FilePath)
	}
}

// TestStorage_Delete_HostIsolation verifies that Delete removes only the specific host's file.
func TestStorage_Delete_HostIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	storage, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	response := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       []byte("test"),
	}

	host1 := "example.com"
	host2 := "mirrors.aliyun.com"
	urlPath := "/shared/path.txt"

	hash1 := "hash1"
	hash2 := "hash2"

	if err := storage.Put(hash1, host1, urlPath, response); err != nil {
		t.Fatalf("Put failed for host1: %v", err)
	}

	if err := storage.Put(hash2, host2, urlPath, response); err != nil {
		t.Fatalf("Put failed for host2: %v", err)
	}

	// Delete host1's entry
	if err := storage.Delete(hash1); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// host1's file should be gone
	if _, err := storage.Get(hash1); err != nil {
		t.Logf("Expected error after delete: %v", err)
	}

	// host2's file should still exist
	cached2, err := storage.Get(hash2)
	if err != nil {
		t.Fatalf("Get failed for hash2: %v", err)
	}
	if cached2 == nil {
		t.Error("host2's entry should still exist")
	}

	// Verify host2's file still exists on disk
	host2Path := storage.getFilePath(host2, urlPath)
	if _, err := os.Stat(host2Path); err != nil {
		t.Errorf("host2's file should still exist at %s: %v", host2Path, err)
	}
}

// TestCache_Put_HostIsolation verifies that Cache.Put uses host for isolation.
func TestCache_Put_HostIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.CacheConfig{
		Directory: tmpDir,
	}

	cache, err := New(cfg)
	if err != nil {
		t.Fatalf("New cache failed: %v", err)
	}

	response := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       []byte("test"),
	}

	host1 := "example.com"
	urlPath := "/test.txt"
	hash := "testhash"

	// Put with host1
	if err := cache.Put(hash, host1, urlPath, response); err != nil {
		t.Fatalf("Put failed with host1: %v", err)
	}

	// Get should return the entry
	cached, err := cache.Get(hash)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if cached == nil {
		t.Fatal("Expected cached entry, got nil")
	}

	// Verify the file exists at the correct path
	expectedPath := filepath.Join(tmpDir, "data", sanitizePath(host1), "test.txt")
	if _, err := os.Stat(expectedPath); err != nil {
		t.Errorf("File should exist at %s: %v", expectedPath, err)
	}
}

// TestWriteResponse_ReadResponse verifies WriteResponse and ResponseToHTTP work correctly.
func TestWriteResponse_ReadResponse(t *testing.T) {
	originalHeaders := map[string][]string{
		"Content-Type":  {"text/plain"},
		"Cache-Control": {"max-age=3600"},
	}

	originalBody := []byte("test content")

	resp := &http.Response{
		StatusCode: 200,
		Header:     originalHeaders,
		Body:       io.NopCloser(bytes.NewReader(originalBody)),
	}

	cached, err := WriteResponse(resp)
	if err != nil {
		t.Fatalf("WriteResponse failed: %v", err)
	}

	if cached.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", cached.StatusCode)
	}

	if !bytes.Equal(cached.Body, originalBody) {
		t.Errorf("Body mismatch: got %q, want %q", string(cached.Body), string(originalBody))
	}

	for k, expectedValues := range originalHeaders {
		actualValues, ok := cached.Headers[k]
		if !ok {
			t.Errorf("Missing header %s", k)
			continue
		}
		if len(actualValues) != len(expectedValues) {
			t.Errorf("Header %s: expected %d values, got %d", k, len(expectedValues), len(actualValues))
		}
		for i, v := range expectedValues {
			if i >= len(actualValues) || actualValues[i] != v {
				t.Errorf("Header %s[%d]: expected %q, got %q", k, i, v, actualValues[i])
			}
		}
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestIsExcludedLastPfx(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		patterns []string
		want     bool
	}{
		{
			name:     "empty exclusion list",
			path:     "/ubuntu/dists/focal/Release",
			patterns: []string{},
			want:     false,
		},
		{
			name:     "nil exclusion list",
			path:     "/ubuntu/dists/focal/Release",
			patterns: nil,
			want:     false,
		},
		{
			name:     "suffix match - full file name",
			path:     "/aa/bb/cc/index.html",
			patterns: []string{"index.html"},
			want:     true,
		},
		{
			name:     "suffix match - partial file name",
			path:     "/aa/bb/cc/index.html",
			patterns: []string{"dex.html"},
			want:     true,
		},
		{
			name:     "suffix match - extension",
			path:     "/aa/bb/cc/index.html",
			patterns: []string{".html"},
			want:     true,
		},
		{
			name:     "suffix match - extension only",
			path:     "/aa/bb/cc/index.html",
			patterns: []string{"tml"},
			want:     true,
		},
		{
			name:     "suffix match - crosses segment boundary",
			path:     "/aa/bb/cc/index.html",
			patterns: []string{"cc/index.html"},
			want:     true,
		},
		{
			name:     "suffix match - leading slash file name",
			path:     "/aa/bb/index.html",
			patterns: []string{"/index.html"},
			want:     true,
		},
		{
			name:     "suffix match - trailing slash directory",
			path:     "/mirror/ubuntu/dists/",
			patterns: []string{"/ubuntu/dists/"},
			want:     true,
		},
		{
			name:     "deeper path not matched",
			path:     "/ubuntu/dists/focal/Release",
			patterns: []string{"/ubuntu/dists/"},
			want:     false,
		},
		{
			name:     "suffix match - full path equals pattern",
			path:     "/etc/resolv.conf",
			patterns: []string{"/etc/resolv.conf"},
			want:     true,
		},
		{
			name:     "no match - different file",
			path:     "/etc/hosts",
			patterns: []string{"/etc/resolv.conf"},
			want:     false,
		},
		{
			name:     "no match - longer path",
			path:     "/etc/resolv.conf.bak",
			patterns: []string{"/etc/resolv.conf"},
			want:     false,
		},
		{
			name:     "suffix match - extension case-insensitive",
			path:     "/ubuntu/pool/main/f/File.HTML",
			patterns: []string{"html"},
			want:     true,
		},
		{
			name:     "suffix no match",
			path:     "/packages/requests-2.28.0.whl",
			patterns: []string{"html"},
			want:     false,
		},
		{
			name:     "root path equals pattern",
			path:     "/",
			patterns: []string{"/"},
			want:     true,
		},
		{
			name:     "root pattern does not match deeper paths",
			path:     "/a/b",
			patterns: []string{"/"},
			want:     false,
		},
		{
			name:     "empty pattern is ignored",
			path:     "/a/b/c.txt",
			patterns: []string{""},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsExcludedLastPfx(tt.path, tt.patterns)
			if got != tt.want {
				t.Errorf("IsExcludedLastPfx(%q, %v) = %v, want %v", tt.path, tt.patterns, got, tt.want)
			}
		})
	}
}

func TestShouldCacheRequest_ExcludeLastPfx(t *testing.T) {
	cfg := config.CacheConfig{
		ExcludeLastPfx: []string{"/ubuntu/dists/", "/etc/resolv.conf", "index.html"},
	}

	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{
			name:   "path not excluded",
			method: "GET",
			path:   "/ubuntu/pool/main/f/file.tar.xz",
			want:   true,
		},
		{
			name:   "directory suffix excluded",
			method: "GET",
			path:   "/mirror/ubuntu/dists/",
			want:   false,
		},
		{
			name:   "deeper path still cached",
			method: "GET",
			path:   "/ubuntu/dists/focal/Release",
			want:   true,
		},
		{
			name:   "full path excluded",
			method: "GET",
			path:   "/etc/resolv.conf",
			want:   false,
		},
		{
			name:   "longer path not excluded",
			method: "GET",
			path:   "/etc/resolv.conf.backup",
			want:   true,
		},
		{
			name:   "file name suffix excluded",
			method: "GET",
			path:   "/mirror/site/index.html",
			want:   false,
		},
		{
			name:   "POST not cached regardless",
			method: "POST",
			path:   "/ubuntu/pool/main/f/file.tar.xz",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Method: tt.method,
				URL:    &url.URL{Path: tt.path},
			}
			got := ShouldCacheRequest(req, cfg)
			if got != tt.want {
				t.Errorf("ShouldCacheRequest(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

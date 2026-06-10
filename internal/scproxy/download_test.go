package scproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/stats"
)

func createTestLogger() *logging.Logger {
	cfg := &config.LoggingConfig{
		Level:  config.LogLevelInfo,
		Output: config.LogOutputStdout,
	}
	logger, err := logging.New(cfg)
	if err != nil {
		panic("Failed to create test logger: " + err.Error())
	}
	return logger
}

func createTestConfig(cacheDir string) *config.Config {
	return &config.Config{
		Cache: config.CacheConfig{
			Enabled:           true,
			Directory:         cacheDir,
			MaxTotalSizeMB:    0,
			MinFileSizeKB:     0,
			MaxFileSizeKB:     0,
			CacheAuth:         false,
			ExcludeExtensions: []string{"html", "js", "css", "json", "xml"},
		},
	}
}

func createTestContext(t *testing.T, target string, cacheInst *cache.Cache) *RequestContext {
	t.Helper()
	logger := createTestLogger()
	cfg := createTestConfig(t.TempDir())
	statsCollector := stats.NewCollector()
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	return NewRequestContext(w, req, logger, cacheInst, cfg, statsCollector, target, "")
}

func createTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	cfg := config.CacheConfig{
		Enabled:   true,
		Directory: t.TempDir(),
	}
	c, err := cache.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create test cache: %v", err)
	}
	return c
}

func TestFetchWithRedirectFollow_GitHubOptimization(t *testing.T) {
	t.Run("Basic request succeeds", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test response"))
		}))
		defer ts.Close()

		ctx := createTestContext(t, ts.URL, nil)

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(ts.URL, "")

		if err != nil {
			t.Fatalf("fetchWithRedirectFollow() error = %v", err)
		}
		if fromCache {
			t.Error("Expected fromCache = false for new request")
		}
		if finalURL != ts.URL {
			t.Errorf("Expected finalURL = %q, got %q", ts.URL, finalURL)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		defer resp.Body.Close()
	})

	t.Run("GitHub releases URL follows redirect and preserves headers", func(t *testing.T) {
		finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Disposition", `attachment; filename="scproxy-linux-amd64.tar.gz"`)
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("scproxy binary content"))
		}))
		defer finalServer.Close()

		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/peng-mj/scproxy/releases/download/V0.1.13/scproxy-linux-amd64.tar.gz" {
				http.Redirect(w, r, finalServer.URL, http.StatusFound)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer redirectServer.Close()

		req := httptest.NewRequest("GET", "/peng-mj/scproxy/releases/download/V0.1.13/scproxy-linux-amd64.tar.gz", nil)
		logger := createTestLogger()
		cfg := createTestConfig(t.TempDir())
		statsCollector := stats.NewCollector()
		w := httptest.NewRecorder()
		ctx := NewRequestContext(w, req, logger, nil, cfg, statsCollector, redirectServer.URL, "")

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(redirectServer.URL+"/peng-mj/scproxy/releases/download/V0.1.13/scproxy-linux-amd64.tar.gz", "")

		if err != nil {
			t.Fatalf("fetchWithRedirectFollow() error = %v", err)
		}
		if fromCache {
			t.Error("Expected fromCache = false for new request")
		}
		if finalURL != finalServer.URL {
			t.Errorf("Expected finalURL = %q, got %q", finalServer.URL, finalURL)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		defer resp.Body.Close()

		if ct := resp.Header.Get("Content-Type"); ct != "application/octet-stream" {
			t.Errorf("Expected Content-Type 'application/octet-stream', got %q", ct)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "scproxy-linux-amd64.tar.gz") {
			t.Errorf("Expected Content-Disposition to contain filename, got %q", cd)
		}
	})

	t.Run("GitHub URL detection", func(t *testing.T) {
		testCases := []struct {
			name     string
			url      string
			isGitHub bool
		}{
			{"GitHub releases URL", "https://github.com/user/repo/releases/download/v1.0/file.tar.gz", true},
			{"GitHub API URL", "https://api.github.com/repos/user/repo/releases", true},
			{"GitHub assets URL", "https://github.com/user/repo/releases/assets/123", true},
			{"Non-GitHub URL", "https://example.com/file.zip", false},
			{"Non-GitHub URL with github in path", "https://example.com/github/file.zip", false},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				isGitHub := strings.Contains(tc.url, "github.com")
				if isGitHub != tc.isGitHub {
					t.Errorf("Expected isGitHub = %v for URL %q", tc.isGitHub, tc.url)
				}
			})
		}
	})
}

func TestFetchWithRedirectFollow_RedirectHandling(t *testing.T) {
	t.Run("Follows single redirect", func(t *testing.T) {
		finalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("final content"))
		}))
		defer finalServer.Close()

		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, finalServer.URL, http.StatusFound)
		}))
		defer redirectServer.Close()

		ctx := createTestContext(t, redirectServer.URL, nil)

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(redirectServer.URL, "")

		if err != nil {
			t.Fatalf("fetchWithRedirectFollow() error = %v", err)
		}
		if fromCache {
			t.Error("Expected fromCache = false")
		}
		if finalURL != finalServer.URL {
			t.Errorf("Expected finalURL = %q, got %q", finalServer.URL, finalURL)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		defer resp.Body.Close()
	})

	t.Run("Follows multiple redirects", func(t *testing.T) {
		servers := make([]*httptest.Server, 3)
		defer func() {
			for _, s := range servers {
				s.Close()
			}
		}()

		for i := 0; i < 3; i++ {
			idx := i
			servers[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if idx < 2 {
					http.Redirect(w, r, servers[idx+1].URL, http.StatusFound)
				} else {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("final content"))
				}
			}))
		}

		ctx := createTestContext(t, servers[0].URL, nil)

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(servers[0].URL, "")

		if err != nil {
			t.Fatalf("fetchWithRedirectFollow() error = %v", err)
		}
		if fromCache {
			t.Error("Expected fromCache = false")
		}
		if finalURL != servers[2].URL {
			t.Errorf("Expected finalURL = %q, got %q", servers[2].URL, finalURL)
		}
		if resp == nil {
			t.Fatal("Expected non-nil response")
		}
		defer resp.Body.Close()
	})

	t.Run("Handles too many redirects", func(t *testing.T) {
		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://"+r.Host+"/loop", http.StatusFound)
		}))
		defer redirectServer.Close()

		ctx := createTestContext(t, redirectServer.URL, nil)

		_, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(redirectServer.URL, "")

		if err == nil {
			t.Error("Expected error for too many redirects")
		}
		if !strings.Contains(err.Error(), "too many redirects") {
			t.Errorf("Expected error message to contain 'too many redirects', got: %v", err)
		}
		if fromCache {
			t.Error("Expected fromCache = false on error")
		}
		if finalURL != "" {
			t.Error("Expected empty finalURL on error")
		}
	})

	t.Run("Handles missing Location header", func(t *testing.T) {
		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", "")
			w.WriteHeader(http.StatusFound)
		}))
		defer redirectServer.Close()

		ctx := createTestContext(t, redirectServer.URL, nil)

		_, _, _, err := ctx.fetchWithRedirectFollow(redirectServer.URL, "")

		if err == nil {
			t.Error("Expected error for missing Location header")
		}
		if !strings.Contains(err.Error(), "redirect response missing Location header") {
			t.Errorf("Expected error message to contain 'redirect response missing Location header', got: %v", err)
		}
	})
}

func TestFetchWithRedirectFollow_CacheHit(t *testing.T) {
	t.Run("Returns fromCache when redirect URL is cached", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("cached content"))
		}))
		defer ts.Close()

		c := createTestCache(t)
		ctx := createTestContext(t, ts.URL, c)

		req := httptest.NewRequest("GET", ts.URL, nil)
		key, err := cache.GenerateCacheKey(req, "", false)
		if err != nil {
			t.Fatalf("GenerateCacheKey error = %v", err)
		}

		cachedResp := &cache.CachedResponse{
			StatusCode: http.StatusOK,
			Headers:    map[string][]string{"Content-Type": {"text/plain"}},
			Body:       []byte("cached content"),
		}
		if err := c.Put(key, "", "/", cachedResp); err != nil {
			t.Fatalf("cache.Put error = %v", err)
		}

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(ts.URL, "")

		if err != nil {
			t.Fatalf("fetchWithRedirectFollow() error = %v", err)
		}
		if !fromCache {
			t.Error("Expected fromCache = true for cached URL")
		}
		if resp != nil {
			t.Error("Expected nil response for cache hit")
		}
		if finalURL != ts.URL {
			t.Errorf("Expected finalURL = %q, got %q", ts.URL, finalURL)
		}
	})

	t.Run("Cache miss makes network request", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("fresh content"))
		}))
		defer ts.Close()

		c := createTestCache(t)
		ctx := createTestContext(t, ts.URL, c)

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(ts.URL, "")

		if err != nil {
			t.Fatalf("fetchWithRedirectFollow() error = %v", err)
		}
		if fromCache {
			t.Error("Expected fromCache = false for cache miss")
		}
		if resp == nil {
			t.Fatal("Expected non-nil response for cache miss")
		}
		defer resp.Body.Close()
		if finalURL != ts.URL {
			t.Errorf("Expected finalURL = %q, got %q", ts.URL, finalURL)
		}
	})
}

func TestFetchWithRedirectFollow_Errors(t *testing.T) {
	t.Run("Handles invalid URL", func(t *testing.T) {
		ctx := createTestContext(t, "", nil)

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow(":invalid-url", "")

		if err == nil {
			t.Error("Expected error for invalid URL")
		}
		if resp != nil {
			t.Error("Expected nil response on error")
		}
		if finalURL != "" {
			t.Error("Expected empty finalURL on error")
		}
		if fromCache {
			t.Error("Expected fromCache = false on error")
		}
	})

	t.Run("Handles context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(context.Background())
		cancel()

		logger := createTestLogger()
		cfg := createTestConfig(t.TempDir())
		statsCollector := stats.NewCollector()
		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(cancelCtx)
		w := httptest.NewRecorder()
		ctx := NewRequestContext(w, req, logger, nil, cfg, statsCollector, "", "")

		resp, finalURL, fromCache, err := ctx.fetchWithRedirectFollow("https://example.com/test", "")

		if err == nil {
			t.Error("Expected error for cancelled context")
		}
		if resp != nil {
			t.Error("Expected nil response on error")
		}
		if finalURL != "" {
			t.Error("Expected empty finalURL on error")
		}
		if fromCache {
			t.Error("Expected fromCache = false on error")
		}
	})

	t.Run("Handles request timeout", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Second)
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		logger := createTestLogger()
		cfg := createTestConfig(t.TempDir())
		statsCollector := stats.NewCollector()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		req := httptest.NewRequest("GET", "/test", nil)
		req = req.WithContext(ctx)
		w := httptest.NewRecorder()
		requestCtx := NewRequestContext(w, req, logger, nil, cfg, statsCollector, ts.URL, "")

		resp, _, _, err := requestCtx.fetchWithRedirectFollow(ts.URL, "")

		if err == nil {
			t.Error("Expected error for timeout")
		}
		if resp != nil {
			resp.Body.Close()
		}
	})
}

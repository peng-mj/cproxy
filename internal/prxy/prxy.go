package prxy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Madh93/prxy/internal/cache"
	"github.com/Madh93/prxy/internal/config"
	"github.com/Madh93/prxy/internal/logging"
	"github.com/Madh93/prxy/internal/stats"
)

// Prxy holds all the dependencies for the HTTP server.
type Prxy struct {
	logger         *logging.Logger
	server         *http.Server
	cache          *cache.Cache
	cfg            *config.Config
	target         string // Target URL for this server instance
	targetHost     string // Parsed target host for stats tracking
	port           int    // Port for this server instance
	statsCollector *stats.Collector
}

// New creates and configures a new Prxy instance.
func New(cfg *config.Config, target string, port int, logger *logging.Logger, statsCollector *stats.Collector, sharedCache *cache.Cache) (*Prxy, error) {
	// 0. Ensure to parse URLs
	parsedTargetURL, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("invalid target URL %q: %w", target, err)
	}

	var c *cache.Cache
	if sharedCache != nil {
		c = sharedCache
	} else if cfg.Cache.Enabled {
		c, err = cache.New(cfg.Cache)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize cache: %v", err)
		}
		logger.Info("Cache initialized", "directory",
			cfg.Cache.Directory, "maxTotalSizeMB", cfg.Cache.MaxTotalSizeMB)
	}

	// 2. Creates Reverse Proxy Handler
	reverseProxyHandler := httputil.NewSingleHostReverseProxy(parsedTargetURL)

	// 2.1 Use the outbound HTTP Proxy for the transport (if provided)
	transport := &http.Transport{
		// Configure timeouts to prevent hanging on slow or broken connections
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second, // Connection timeout
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second, // Timeout waiting for response headers
		// Disable HTTP/2 for better compatibility with mirror servers
		ForceAttemptHTTP2: false,
	}
	if cfg.Proxy != "" {
		parsedProxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy URL %q: %w", cfg.Proxy, err)
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)
		logger.Info("Using outbound proxy", "proxy", cfg.Proxy)
	} else {
		logger.Info("No outbound proxy configured, using direct connection")
	}
	reverseProxyHandler.Transport = transport

	// 2.2 Ensure the Host header is rewritten to the target's host.
	originalDirector := reverseProxyHandler.Director
	reverseProxyHandler.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = parsedTargetURL.Host
	}

	// 2.3 Custom error handler for better logging and response.
	reverseProxyHandler.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		logger.Error("Reverse proxy error", "url", req.URL.String(), "error", err)

		// If this is a streaming cache writer, mark it as aborted
		if scw, ok := rw.(*streamingCacheWriter); ok {
			scw.cleanup()
		}

		http.Error(rw, "Proxy Error: "+err.Error(), http.StatusBadGateway)
	}

	// 3. Wrap with caching handler if enabled
	var handler http.Handler
	if c != nil {
		handler = newCachingHandler(reverseProxyHandler, c, logger, cfg, target, parsedTargetURL.Host, statsCollector)
		logger.Info("Caching enabled")
	} else {
		handler = reverseProxyHandler
		logger.Info("Caching disabled")
	}

	// 4. Creates HTTP httpServer
	httpServer := &http.Server{
		Addr:    net.JoinHostPort(cfg.Host, strconv.Itoa(port)),
		Handler: handler,
	}

	// Create main Prxy struct.
	prxy := &Prxy{
		logger:         logger,
		server:         httpServer,
		cache:          c,
		cfg:            cfg,
		target:         target,
		targetHost:     parsedTargetURL.Host,
		port:           port,
		statsCollector: statsCollector,
	}

	return prxy, nil
}

// Run starts the HTTP server and blocks until it exits.
func (s Prxy) Run() error {
	// This method always returns a non-nil error. When Shutdown() is called,
	// it returns http.ErrServerClosed.
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server.
func (s Prxy) Shutdown(ctx context.Context) error {
	s.logger.Debug("Shutting down HTTP server...")
	return s.server.Shutdown(ctx)
}

// Addr returns the network address the server is listening on.
// Returns an empty string if the server is not running.
func (s Prxy) Addr() string {
	return s.server.Addr
}

// formatBytes formats bytes to human-readable size.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)

	if bytes < MB {
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	} else if bytes < GB {
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	} else {
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	}
}

// parseContentLength parses the Content-Length header value.
func parseContentLength(contentLength string) (int64, error) {
	var length int64
	_, err := fmt.Sscanf(contentLength, "%d", &length)
	return length, err
}

// streamingCacheWriter handles streaming cache writes.
type streamingCacheWriter struct {
	http.ResponseWriter
	request               *http.Request
	key                   string
	cache                 *cache.Cache
	logger                *logging.Logger
	cfg                   *config.Config
	host                  string
	targetHost            string
	statsCollector        *stats.Collector
	statusCode            int
	headers               map[string][]string
	tempFile              *os.File
	tempFilePath          string
	bytesWritten          int64
	expectedContentLength int64
	complete              bool
	aborted               bool
}

func newStreamingCacheWriter(w http.ResponseWriter, r *http.Request, key string, cache *cache.Cache, logger *logging.Logger, cfg *config.Config, host string, targetHost string, statsCollector *stats.Collector) *streamingCacheWriter {
	return &streamingCacheWriter{
		ResponseWriter: w,
		request:        r,
		key:            key,
		cache:          cache,
		logger:         logger,
		cfg:            cfg,
		host:           host,
		targetHost:     targetHost,
		statsCollector: statsCollector,
		headers:        make(map[string][]string),
	}
}

func (sw *streamingCacheWriter) WriteHeader(statusCode int) {
	sw.statusCode = statusCode
	sw.ResponseWriter.WriteHeader(statusCode)

	// Record error response statistics (4xx and 5xx)
	if sw.statsCollector != nil && sw.targetHost != "" && statusCode >= 400 && statusCode < 600 {
		sw.statsCollector.RecordResponse(sw.targetHost, statusCode)
	}

	// Check if we should cache this response
	mockResp := &http.Response{
		StatusCode: statusCode,
		Header:     sw.Header(),
	}
	if !cache.ShouldCacheResponse(mockResp, sw.cfg.Cache) {
		sw.logger.Debug("Response not cacheable", "host", sw.host, "status", statusCode)
		return
	}

	// Capture headers
	for k, v := range sw.Header() {
		if len(sw.headers[k]) == 0 {
			sw.headers[k] = []string{}
		}
		sw.headers[k] = append(sw.headers[k], v...)
	}

	// Parse Content-Length for validation
	if contentLength := sw.Header().Get("Content-Length"); contentLength != "" {
		if length, err := parseContentLength(contentLength); err == nil {
			sw.expectedContentLength = length
			sw.logger.Debug("Expected content length", "key", cache.ShortKey(sw.key), "length", length)
		}
	}

	// Create temporary file for caching
	cacheDir := sw.cfg.Cache.Directory
	safeHost := cache.SanitizePath(sw.host)
	safePath := cache.GenerateFilePath(sw.request.URL.Path)
	tempFilePath := filepath.Join(cacheDir, "data", safeHost, safePath+".tmp")

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(tempFilePath), 0755); err != nil {
		sw.logger.Error("Failed to create cache directory", "error", err)
		return
	}

	// Create temp file
	tempFile, err := os.Create(tempFilePath)
	if err != nil {
		sw.logger.Error("Failed to create temp file", "error", err)
		return
	}

	sw.tempFile = tempFile
	sw.tempFilePath = tempFilePath
}

func (sw *streamingCacheWriter) Write(data []byte) (int, error) {
	// Write to client
	n, err := sw.ResponseWriter.Write(data)
	if err != nil {
		// Client disconnected, mark as aborted
		sw.aborted = true
		sw.logger.Warn("Client disconnected during transfer", "key", cache.ShortKey(sw.key), "written", formatBytes(sw.bytesWritten), "error", err)
		sw.cleanup()
		return n, err
	}

	// Write to temp file if caching is enabled
	if sw.tempFile != nil {
		if _, err := sw.tempFile.Write(data); err != nil {
			sw.logger.Error("Failed to write to temp file", "error", err)
			sw.cleanup()
			sw.tempFile = nil
			return n, err
		}
		sw.bytesWritten += int64(len(data))

		// Sync to disk periodically to ensure data persistence
		if sw.bytesWritten%(1024*1024) == 0 { // Every 1MB
			if err := sw.tempFile.Sync(); err != nil {
				sw.logger.Error("Failed to sync temp file", "error", err)
			}
		}
	}

	// Flush data to client immediately for streaming
	if flusher, ok := sw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}

	return n, err
}

// Flush implements the http.Flusher interface to support streaming
func (sw *streamingCacheWriter) Flush() {
	if flusher, ok := sw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack implements the http.Hijacker interface for WebSocket support
func (sw *streamingCacheWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := sw.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not support hijacking")
}

// cleanup removes incomplete cache files
func (sw *streamingCacheWriter) cleanup() {
	if sw.tempFile != nil {
		sw.tempFile.Close()
	}
	if sw.tempFilePath != "" {
		if err := os.Remove(sw.tempFilePath); err != nil && !os.IsNotExist(err) {
			sw.logger.Error("Failed to remove incomplete cache file", "error", err)
		} else {
			sw.logger.Debug("Removed incomplete cache file", "path", sw.tempFilePath)
		}
	}
	sw.tempFile = nil
	sw.tempFilePath = ""
	sw.aborted = true
}

func (sw *streamingCacheWriter) Close() {
	sw.logger.Debug("Close() called", "key", cache.ShortKey(sw.key), "aborted", sw.aborted, "tempFile", sw.tempFile != nil)

	if sw.aborted || sw.tempFile == nil {
		sw.logger.Debug("Transfer aborted or no temp file, cleaning up")
		sw.cleanup()
		return
	}

	sw.tempFile.Close()
	sw.logger.Debug("Temp file closed", "bytesWritten", sw.bytesWritten, "expected", sw.expectedContentLength)

	if sw.expectedContentLength > 0 {
		if sw.bytesWritten != sw.expectedContentLength {
			sw.logger.Warn("Incomplete download detected, removing cache",
				"key", cache.ShortKey(sw.key),
				"expected", formatBytes(sw.expectedContentLength),
				"received", formatBytes(sw.bytesWritten),
				"missing", formatBytes(sw.expectedContentLength-sw.bytesWritten))
			sw.cleanup()
			return
		}
		sw.complete = true
		sw.logger.Debug("Download validated as complete")
	}

	finalPath := strings.TrimSuffix(sw.tempFilePath, ".tmp")

	if err := os.Rename(sw.tempFilePath, finalPath); err != nil {
		fileInfo, statErr := os.Stat(finalPath)
		if statErr == nil && fileInfo.IsDir() {
			sw.logger.Warn("Final path is a directory, removing and retrying", "path", finalPath)
			if rmErr := os.RemoveAll(finalPath); rmErr != nil {
				sw.logger.Error("Failed to remove existing directory", "path", finalPath, "error", rmErr)
				sw.cleanup()
				return
			}
			if renameErr := os.Rename(sw.tempFilePath, finalPath); renameErr != nil {
				sw.logger.Error("Failed to rename temp file after removing directory", "error", renameErr)
				sw.cleanup()
				return
			}
		} else {
			sw.logger.Error("Failed to rename temp file", "error", err)
			sw.cleanup()
			return
		}
	}

	fileInfo, err := os.Stat(finalPath)
	if err != nil {
		sw.logger.Error("Failed to get file size", "error", err)
		os.Remove(finalPath)
		return
	}

	if sw.expectedContentLength > 0 && fileInfo.Size() != sw.expectedContentLength {
		sw.logger.Warn("File size mismatch after rename, removing cache",
			"key", cache.ShortKey(sw.key),
			"expected", sw.expectedContentLength,
			"actual", fileInfo.Size())
		os.Remove(finalPath)
		return
	}

	safeHost := cache.SanitizePath(sw.host)
	safePath := cache.GenerateFilePath(sw.request.URL.Path)
	relativePath := filepath.Join(safeHost, safePath)
	sw.logger.Debug("About to store in cache index", "key", cache.ShortKey(sw.key), "path", sw.request.URL.Path, "relativePath", relativePath, "size", fileInfo.Size())

	if err := sw.cache.PutFromDisk(sw.key, sw.host, sw.request.URL.Path, sw.statusCode, sw.headers, relativePath, fileInfo.Size()); err != nil {
		sw.logger.Error("Failed to store cache index", "error", err)
		os.Remove(finalPath)
	} else {
		sw.logger.Info("Stored in cache", "host", sw.host, "key", cache.ShortKey(sw.key), "size", formatBytes(fileInfo.Size()), "path", sw.request.URL.Path)

		// Update cache size statistics
		if sw.statsCollector != nil && sw.targetHost != "" {
			// Get current total cache size for this domain
			var totalSize uint64
			if cacheSize, err := sw.cache.GetSizeForHost(sw.targetHost); err == nil {
				totalSize = uint64(cacheSize)
			}
			sw.statsCollector.UpdateCacheSize(sw.targetHost, totalSize)
		}

		// Check and evict if cache exceeds limit after adding new entry
		if sw.cache != nil && sw.cache.CacheConfig().MaxTotalSizeMB > 0 {
			if err := sw.cache.CheckAndEvict(); err != nil {
				sw.logger.Warn("Failed to evict cache entries", "error", err)
			}
		}
	}

	sw.tempFile = nil
	sw.tempFilePath = ""
}

// newCachingHandler creates a caching middleware handler with streaming support.
func newCachingHandler(handler http.Handler, c *cache.Cache, logger *logging.Logger, cfg *config.Config, target string, targetHost string, statsCollector *stats.Collector) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := NewRequestContext(w, r, logger, c, cfg, statsCollector, target, targetHost)

		if !cache.ShouldCacheRequest(r, cfg.Cache) {
			ctx.SetCacheStatusHeader("BYPASS")
			handler.ServeHTTP(w, r)
			return
		}

		key, err := ctx.GenerateCacheKey()
		if err != nil {
			logger.Error("Failed to generate cache key", "error", err)
			handler.ServeHTTP(w, r)
			return
		}

		cached, err := c.Get(key)
		if err != nil {
			logger.Error("Cache get error", "error", err)
			handler.ServeHTTP(w, r)
			return
		}

		if cached != nil {
			logger.Info("Cache hit", "host", targetHost, "key", cache.ShortKey(key), "path", r.URL.Path)
			ctx.handleCachedResponse(cached)
			return
		}

		if statsCollector != nil && targetHost != "" {
			statsCollector.RecordCacheMiss(targetHost)
		}

		if r.Header.Get("Range") != "" {
			ctx.SetCacheStatusHeader("MISS")
			handler.ServeHTTP(w, r)
			return
		}

		if target != "" {
			fullURL := ctx.BuildFullURL(r.URL.Path)

			logger.Debug("Fetching with redirect follow", "url", fullURL, "path", r.URL.Path)

			resp, finalURL, cachedHit, err := ctx.fetchWithRedirectFollow(fullURL, cfg.Proxy)
			if err != nil {
				logger.Error("Failed to fetch with redirect follow", "error", err)
				ctx.SetCacheStatusHeader("MISS")
				ctx.WriteError("Failed to fetch: "+err.Error(), http.StatusBadGateway)
				return
			}

			if cachedHit {
				parsedURL, err := url.Parse(finalURL)
				if err == nil {
					cacheReq := &http.Request{
						Method: r.Method,
						URL:    parsedURL,
						Header: r.Header,
						Host:   parsedURL.Host,
					}
					redirectKey, err := cache.GenerateCacheKey(cacheReq, parsedURL.Host, cfg.Cache.CacheAuth)
					if err == nil {
						cached, err := c.Get(redirectKey)
						if err == nil && cached != nil {
							logger.Debug("Returning cached redirect response", "url", finalURL, "key", cache.ShortKey(redirectKey))
							ctx.handleCachedResponse(cached)
							return
						}
					}
				}
			}

			defer resp.Body.Close()

			if finalURL != fullURL {
				logger.Info("Redirect followed", "original", fullURL, "final", finalURL)
			}

			streamingWriter := newStreamingCacheWriter(w, r, key, c, logger, cfg, targetHost, targetHost, statsCollector)
			defer streamingWriter.Close()

			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("Panic during streaming, cleaning up cache", "error", rec)
					streamingWriter.cleanup()
				}
			}()

			for k, v := range resp.Header {
				if k != "Transfer-Encoding" && k != "Connection" {
					streamingWriter.Header()[k] = v
				}
			}

			streamingWriter.WriteHeader(resp.StatusCode)

			if _, err := io.Copy(streamingWriter, resp.Body); err != nil {
				logger.Error("Failed to stream response", "error", err)
				streamingWriter.cleanup()
				return
			}

			logger.Debug("Response cached after redirect follow", "host", targetHost, "key", cache.ShortKey(key), "status", resp.StatusCode)
			return
		}

		logger.Debug("Cache miss", "host", targetHost, "key", cache.ShortKey(key), "path", r.URL.Path)
		streamingWriter := newStreamingCacheWriter(w, r, key, c, logger, cfg, targetHost, targetHost, statsCollector)
		defer streamingWriter.Close()

		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic during streaming, cleaning up cache", "error", r)
				streamingWriter.cleanup()
			}
		}()

		handler.ServeHTTP(streamingWriter, r)
	})
}

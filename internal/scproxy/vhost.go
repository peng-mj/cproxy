package scproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/stats"
)

// VHostServer implements a virtual host reverse proxy that routes HTTP
// requests to different upstream targets based on the Host header.
type VHostServer struct {
	cfg            *config.Config
	port           int
	logger         *logging.Logger
	cache          *cache.Cache
	statsCollector *stats.Collector
	handlers       map[string]http.Handler // host → proxy handler (with caching)
	hostnames      map[string]string       // host → target URL
	server         *http.Server
	mu             sync.RWMutex
}

// NewVHostServer creates a new virtual host reverse proxy server.
func NewVHostServer(cfg *config.Config, port int, logger *logging.Logger, statsCollector *stats.Collector, c *cache.Cache) (*VHostServer, error) {
	s := &VHostServer{
		cfg:            cfg,
		port:           port,
		logger:         logger,
		cache:          c,
		statsCollector: statsCollector,
		handlers:       make(map[string]http.Handler),
		hostnames:      make(map[string]string),
	}

	for _, route := range cfg.Routes {
		parsedURL, err := url.Parse(route.Target)
		if err != nil {
			return nil, fmt.Errorf("failed to parse route target %q: %w", route.Target, err)
		}

		hostname := strings.ToLower(parsedURL.Hostname())
		if hostname == "" {
			continue
		}

		if _, exists := s.handlers[hostname]; exists {
			logger.Warn("Duplicate hostname in routes, skipping", "hostname", hostname, "target", route.Target)
			continue
		}

		handler, _, err := NewProxyHandler(cfg, route.Target, logger, statsCollector, c)
		if err != nil {
			return nil, fmt.Errorf("failed to create proxy handler for %q: %w", route.Target, err)
		}

		s.handlers[hostname] = handler
		s.hostnames[hostname] = route.Target

		logger.Info("VHost route registered", "hostname", hostname, "target", route.Target)
	}

	return s, nil
}

// Start starts the virtual host reverse proxy server.
func (s *VHostServer) Start() error {
	addr := fmt.Sprintf(":%d", s.port)

	// Pre-bind listener to detect port conflicts immediately
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind VHost %s: %w", addr, err)
	}

	s.server = &http.Server{
		Handler: s,
	}

	go func() {
		s.logger.Info("VHost server starting", "addr", addr, "routes", len(s.handlers))
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			s.logger.Error("VHost server error", "error", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the virtual host server.
func (s *VHostServer) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down VHost server...")
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

// ServeHTTP routes the request to the appropriate handler based on the Host header.
func (s *VHostServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := extractVHost(r)

	s.mu.RLock()
	handler, ok := s.handlers[host]
	s.mu.RUnlock()

	if !ok {
		s.logger.Warn("No VHost route for host", "host", host, "path", r.URL.Path)
		http.Error(w, fmt.Sprintf("No route configured for host: %s", host), http.StatusNotFound)
		return
	}

	s.logger.Debug("VHost request routed", "host", host, "method", r.Method, "path", r.URL.Path)
	handler.ServeHTTP(w, r)
}

// extractVHost extracts the hostname from the Host header or X-Forwarded-Host,
// stripping any port number.
func extractVHost(r *http.Request) string {
	host := r.Host
	if forwarded := r.Header.Get("X-Forwarded-Host"); forwarded != "" {
		host = forwarded
	}

	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	return strings.ToLower(host)
}

// Hostnames returns all configured hostnames for DNS registration.
func (s *VHostServer) Hostnames() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.hostnames))
	for h := range s.hostnames {
		names = append(names, h)
	}
	return names
}

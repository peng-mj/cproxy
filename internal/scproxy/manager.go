package scproxy

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/cert"
	"github.com/peng-mj/scproxy/internal/config"
	dnsserver "github.com/peng-mj/scproxy/internal/dns"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/stats"
	"github.com/peng-mj/scproxy/internal/validation"
)

// scproxyManager manages multiple proxy server instances.
type scproxyManager struct {
	logger         *logging.Logger
	cache          *cache.Cache
	servers        []*scproxy     // All per-port server instances
	cfg            *config.Config // Shared configuration
	statsCollector *stats.Collector
	dnsServer      *dnsserver.Server // DNS proxy server (nil if disabled)
	vhostServer    *VHostServer      // VHost reverse proxy server (nil if disabled)
	certMgr        *cert.Manager     // Certificate manager (nil if TLS disabled)
}

// NewscproxyManager creates a new server manager.
func NewscproxyManager(cfg *config.Config, logger *logging.Logger, configPath string) (*scproxyManager, error) {
	// Initialize cache (shared by all servers)
	var c *cache.Cache
	var err error
	if cfg.Cache.Enabled {
		c, err = cache.New(cfg.Cache)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize cache: %v", err)
		}
		logger.Info("Cache initialized", "directory", cfg.Cache.Directory)
	}

	// Initialize statistics collector with config file path
	statsCollector := stats.NewCollector()
	if configPath != "" {
		statsCollector.StartPeriodicWrite(configPath, 1*time.Second)
	}

	pm := &scproxyManager{
		logger:         logger,
		cache:          c,
		cfg:            cfg,
		statsCollector: statsCollector,
	}

	// Create a server for each route
	for _, route := range cfg.Routes {
		server, err := pm.createServerForRoute(route, c)
		if err != nil {
			// Cleanup already created servers
			pm.Shutdown(context.Background())
			return nil, err
		}
		pm.servers = append(pm.servers, server)
	}

	// Initialize certificate manager if TLS is enabled
	var certMgr *cert.Manager
	if cfg.TLS.Enabled {
		cm, err := cert.NewManager(cfg.TLS.CertDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize certificate manager: %v", err)
		}
		certMgr = cm
		// Pre-generate certificates for all known domains
		certMgr.PreGenerate(extractDomains(cfg.Routes))
		logger.Info("Certificate manager ready",
			"certDir", cfg.TLS.CertDir,
			"caCert", cm.CACertPath(),
			"tlsPort", cfg.TLS.Port)
	}

	// Initialize VHost server if enabled
	if cfg.VHost.Enabled {
		vs, err := NewVHostServer(cfg, cfg.VHost.Port, logger, statsCollector, c, certMgr)
		if err != nil {
			pm.Shutdown(context.Background())
			return nil, fmt.Errorf("failed to create VHost server: %v", err)
		}
		pm.vhostServer = vs
		pm.certMgr = certMgr
	}

	// Initialize DNS server if enabled
	if cfg.DNS.Enabled {
		domains := extractDomains(cfg.Routes)
		ds, err := dnsserver.New(dnsserver.Config(cfg.DNS), domains, logger)
		if err != nil {
			pm.Shutdown(context.Background())
			return nil, fmt.Errorf("failed to create DNS server: %v", err)
		}
		if ds != nil {
			pm.dnsServer = ds
		}
	}

	return pm, nil
}

// createServerForRoute creates a server for a single route.
func (pm *scproxyManager) createServerForRoute(route validation.RouteConfig, cache *cache.Cache) (*scproxy, error) {
	server, err := New(pm.cfg, route.Target, route.Port, pm.logger, pm.statsCollector, cache)
	if err != nil {
		return nil, fmt.Errorf("failed to create server for port %d: %v", route.Port, err)
	}

	pm.logger.Info("Server created", "target", route.Target, "port", route.Port)
	return server, nil
}

// Start starts all servers.
func (pm *scproxyManager) Start() error {
	pm.logger.Info("Starting all servers...", "perPort", len(pm.servers))

	for _, server := range pm.servers {
		go func(s *scproxy) {
			if err := s.Run(); err != nil {
				pm.logger.Error("Server stopped", "port", s.port, "error", err)
			}
		}(server)
	}

	if pm.vhostServer != nil {
		if err := pm.vhostServer.Start(); err != nil {
			pm.logger.Warn("VHost HTTP server failed to start, continuing without it", "error", err)
			pm.vhostServer = nil
		} else if pm.certMgr != nil && pm.cfg.TLS.Enabled {
			if err := pm.vhostServer.StartTLS(pm.cfg.TLS.Port); err != nil {
				pm.logger.Warn("VHost HTTPS server failed to start, continuing without it", "error", err)
			}
		}
	}

	if pm.dnsServer != nil {
		if err := pm.dnsServer.Start(); err != nil {
			pm.logger.Warn("DNS server failed to start, continuing without it", "error", err)
			pm.dnsServer = nil
		}
	}

	return nil
}

// Shutdown gracefully shuts down all servers.
func (pm *scproxyManager) Shutdown(ctx context.Context) error {
	pm.logger.Info("Shutting down all servers...", "perPort", len(pm.servers))

	// Stop statistics collector
	if pm.statsCollector != nil {
		pm.statsCollector.Stop()
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(pm.servers)+2)

	for _, server := range pm.servers {
		wg.Add(1)
		go func(s *scproxy) {
			defer wg.Done()
			if err := s.Shutdown(ctx); err != nil {
				errs <- err
			}
		}(server)
	}

	if pm.vhostServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pm.vhostServer.Shutdown(ctx); err != nil {
				errs <- err
			}
		}()
	}

	if pm.dnsServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := pm.dnsServer.Shutdown(ctx); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	// Collect errors
	var errors []error
	for err := range errs {
		errors = append(errors, err)
	}

	// Close cache after all servers are shut down
	if pm.cache != nil {
		if cerr := pm.cache.Close(); cerr != nil {
			pm.logger.Error("Failed to close cache", "error", cerr)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	return nil
}

// extractDomains extracts unique hostnames from route targets.
func extractDomains(routes []validation.RouteConfig) []string {
	seen := make(map[string]bool)
	domains := make([]string, 0)

	for _, route := range routes {
		parsedURL, err := url.Parse(route.Target)
		if err != nil {
			continue
		}
		hostname := strings.ToLower(parsedURL.Hostname())
		if hostname != "" && !seen[hostname] {
			seen[hostname] = true
			domains = append(domains, hostname)
		}
	}

	return domains
}

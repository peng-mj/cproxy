package scproxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/stats"
	"github.com/peng-mj/scproxy/internal/validation"
)

// scproxyManager manages multiple proxy server instances.
type scproxyManager struct {
	logger         *logging.Logger
	cache          *cache.Cache
	servers        []*scproxy     // All server instances
	cfg            *config.Config // Shared configuration
	statsCollector *stats.Collector
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

	return pm, nil
}

// createServerForRoute creates a server for a single route.
func (pm *scproxyManager) createServerForRoute(route validation.RouteConfig, cache *cache.Cache) (*scproxy, error) {
	// Create server for this route
	server, err := New(pm.cfg, route.Target, route.Port, pm.logger, pm.statsCollector, cache)
	if err != nil {
		return nil, fmt.Errorf("failed to create server for port %d: %v", route.Port, err)
	}

	pm.logger.Info("Server created", "target", route.Target, "port", route.Port)
	return server, nil
}

// Start starts all servers.
func (pm *scproxyManager) Start() error {
	pm.logger.Info("Starting all servers...", "count", len(pm.servers))

	for _, server := range pm.servers {
		go func(s *scproxy) {
			if err := s.Run(); err != nil {
				pm.logger.Error("Server stopped", "port", s.port, "error", err)
			}
		}(server)
	}

	return nil
}

// Shutdown gracefully shuts down all servers.
func (pm *scproxyManager) Shutdown(ctx context.Context) error {
	pm.logger.Info("Shutting down all servers...", "count", len(pm.servers))

	// Stop statistics collector
	if pm.statsCollector != nil {
		pm.statsCollector.Stop()
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(pm.servers))

	for _, server := range pm.servers {
		wg.Add(1)
		go func(s *scproxy) {
			defer wg.Done()
			if err := s.Shutdown(ctx); err != nil {
				errs <- err
			}
		}(server)
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

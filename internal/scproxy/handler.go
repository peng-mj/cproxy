package scproxy

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/stats"
)

// NewProxyHandler creates an HTTP handler chain (reverse proxy + optional caching)
// for a single target URL. This is used by both per-port proxy servers and the
// virtual host (vhost) server.
func NewProxyHandler(
	cfg *config.Config,
	target string,
	logger *logging.Logger,
	statsCollector *stats.Collector,
	c *cache.Cache,
) (http.Handler, string, error) {
	parsedTargetURL, err := url.Parse(target)
	if err != nil {
		return nil, "", fmt.Errorf("invalid target URL %q: %w", target, err)
	}

	targetHost := parsedTargetURL.Host

	reverseProxyHandler := httputil.NewSingleHostReverseProxy(parsedTargetURL)

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		ForceAttemptHTTP2:     false,
	}
	if cfg.Proxy != "" {
		parsedProxyURL, err := url.Parse(cfg.Proxy)
		if err != nil {
			return nil, "", fmt.Errorf("invalid proxy URL %q: %w", cfg.Proxy, err)
		}
		transport.Proxy = http.ProxyURL(parsedProxyURL)
	}
	reverseProxyHandler.Transport = transport

	originalDirector := reverseProxyHandler.Director
	reverseProxyHandler.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = parsedTargetURL.Host
	}

	reverseProxyHandler.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		logger.Error("Reverse proxy error", "url", req.URL.String(), "error", err)

		if scw, ok := rw.(*streamingCacheWriter); ok {
			scw.cleanup()
		}

		http.Error(rw, "Proxy Error: "+err.Error(), http.StatusBadGateway)
	}

	var handler http.Handler
	if c != nil {
		handler = newCachingHandler(reverseProxyHandler, c, logger, cfg, target, targetHost, statsCollector)
		logger.Info("Caching enabled", "target", target)
	} else {
		handler = reverseProxyHandler
		logger.Info("Caching disabled", "target", target)
	}

	return handler, targetHost, nil
}

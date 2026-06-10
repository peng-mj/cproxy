package scproxy

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/stats"
)

type RequestContext struct {
	w              http.ResponseWriter
	r              *http.Request
	logger         *logging.Logger
	cache          *cache.Cache
	cfg            *config.Config
	statsCollector *stats.Collector
	target         string
	targetHost     string
}

func NewRequestContext(w http.ResponseWriter, r *http.Request, logger *logging.Logger, cache *cache.Cache, cfg *config.Config, statsCollector *stats.Collector, target string, targetHost string) *RequestContext {
	return &RequestContext{
		w:              w,
		r:              r,
		logger:         logger,
		cache:          cache,
		cfg:            cfg,
		statsCollector: statsCollector,
		target:         target,
		targetHost:     targetHost,
	}
}

func (ctx *RequestContext) SetCacheStatusHeader(status string) {
	ctx.w.Header().Set("X-Cache", status)
}

func (ctx *RequestContext) WriteError(message string, statusCode int) {
	http.Error(ctx.w, message, statusCode)
}

func (ctx *RequestContext) BuildFullURL(path string) string {
	fullURL := ctx.target + path
	if ctx.r.URL.RawQuery != "" {
		fullURL += "?" + ctx.r.URL.RawQuery
	}
	return fullURL
}

func (ctx *RequestContext) ParseTargetURL() (*url.URL, error) {
	if ctx.target == "" {
		return nil, fmt.Errorf("target URL is empty")
	}
	return url.Parse(ctx.target)
}

func (ctx *RequestContext) GenerateCacheKey() (string, error) {
	if ctx.targetHost == "" {
		return "", fmt.Errorf("targetHost is empty")
	}
	return cache.GenerateCacheKey(ctx.r, ctx.targetHost, ctx.cfg.Cache.CacheAuth)
}

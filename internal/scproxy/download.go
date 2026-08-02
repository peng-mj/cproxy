package scproxy

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
)

func (ctx *RequestContext) fetchWithRedirectFollow(targetURL string, proxyURL string) (*http.Response, string, bool, error) {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Resolver:  upstreamResolver(ctx.cfg),
		}).DialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		// Optimize for GitHub CDN connections
		DisableCompression:  false,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 10,
	}

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if proxyURL != "" {
		if parsedProxy, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsedProxy)
		}
	}

	maxRedirects := 10
	currentURL := targetURL
	var finalURL string

	for i := 0; i < maxRedirects; i++ {
		finalURL = currentURL

		parsedURL, err := url.Parse(currentURL)
		if err != nil {
			return nil, "", false, err
		}

		if ctx.cache != nil {
			cacheReq := &http.Request{
				Method: ctx.r.Method,
				URL:    parsedURL,
				Header: ctx.r.Header,
				Host:   parsedURL.Host,
			}

			key, err := cache.GenerateCacheKey(cacheReq, parsedURL.Host, ctx.cfg.Cache.CacheAuth)
			if err == nil {
				cached, err := ctx.cache.Get(key)
				if err == nil && cached != nil {
					ctx.logger.Debug("Redirect URL cache hit", "url", currentURL, "key", cache.ShortKey(key))
					return nil, currentURL, true, nil
				}
			}
		}

		req, err := http.NewRequestWithContext(ctx.r.Context(), ctx.r.Method, currentURL, nil)
		if err != nil {
			return nil, "", false, err
		}

		for k, v := range ctx.r.Header {
			if k != "Host" && k != "Content-Length" {
				req.Header[k] = v
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", false, err
		}

		if resp.StatusCode == http.StatusMovedPermanently ||
			resp.StatusCode == http.StatusFound ||
			resp.StatusCode == http.StatusSeeOther ||
			resp.StatusCode == http.StatusTemporaryRedirect ||
			resp.StatusCode == http.StatusPermanentRedirect {
			location := resp.Header.Get("Location")
			if location == "" {
				resp.Body.Close()
				return nil, "", false, fmt.Errorf("redirect response missing Location header")
			}

			resp.Body.Close()

			if parsedLocation, err := url.Parse(location); err == nil {
				if !parsedLocation.IsAbs() {
					if base, err := url.Parse(currentURL); err == nil {
						currentURL = base.ResolveReference(parsedLocation).String()
					} else {
						currentURL = location
					}
				} else {
					currentURL = location
				}
			} else {
				currentURL = location
			}

			ctx.logger.Debug("Following redirect", "from", finalURL, "to", currentURL, "status", resp.StatusCode)
			continue
		}

		return resp, finalURL, false, nil
	}

	return nil, "", false, fmt.Errorf("too many redirects (max %d)", maxRedirects)
}

package scproxy

import (
	"context"
	"net"
	"time"

	"github.com/peng-mj/scproxy/internal/config"
)

// upstreamResolver returns a *net.Resolver that bypasses the system resolver
// and always dials the configured DNS upstream(s). It returns nil (=> default
// system resolver) when gateway/DNS mode is off, preserving per-port behavior.
//
// This breaks the self-loop in gateway mode: the proxy's own upstream fetches
// no longer consult the hijacking DNS (which maps the target domain back to
// 127.0.0.1), so they resolve to the real upstream IP instead of looping into
// the proxy itself.
func upstreamResolver(cfg *config.Config) *net.Resolver {
	if cfg == nil || !cfg.DNS.Enabled || len(cfg.DNS.Upstream) == 0 {
		return nil
	}
	upstream := cfg.DNS.Upstream[0]
	if _, _, err := net.SplitHostPort(upstream); err != nil {
		upstream = upstream + ":53"
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, upstream)
		},
	}
}

// Package dns provides a DNS proxy server that resolves configured domains
// to a specified IP address (typically the local proxy server), while
// forwarding all other queries to upstream DNS servers.
package dns

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/peng-mj/scproxy/internal/logging"
)

// Config holds DNS server configuration.
type Config struct {
	Enabled  bool     `json:"enabled"`  // Enable DNS server
	Addr     string   `json:"addr"`     // Listen address (e.g. ":53")
	Upstream []string `json:"upstream"` // Upstream DNS servers (e.g. ["8.8.8.8:53"])
	ProxyIP  string   `json:"proxyIP"`  // IP to return for proxied domains
}

// DefaultConfig returns default DNS configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:  false,
		Addr:     ":53",
		Upstream: []string{"8.8.8.8:53"},
		ProxyIP:  "127.0.0.1",
	}
}

// Server implements a DNS proxy server.
type Server struct {
	cfg     Config
	domains map[string]net.IP // domain → resolved IP
	proxyIP net.IP
	logger  *logging.Logger
	udpSrv  *dns.Server
	tcpSrv  *dns.Server
	mu      sync.RWMutex
}

// New creates a new DNS server.
func New(cfg Config, domains []string, logger *logging.Logger) (*Server, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	if cfg.Addr == "" {
		cfg.Addr = ":53"
	}
	if len(cfg.Upstream) == 0 {
		cfg.Upstream = []string{"8.8.8.8:53"}
	}
	if cfg.ProxyIP == "" {
		cfg.ProxyIP = "127.0.0.1"
	}

	proxyIP := net.ParseIP(cfg.ProxyIP)
	if proxyIP == nil {
		return nil, fmt.Errorf("invalid proxy IP: %s", cfg.ProxyIP)
	}

	for i, upstream := range cfg.Upstream {
		if !strings.Contains(upstream, ":") {
			cfg.Upstream[i] = upstream + ":53"
		}
	}

	domainMap := make(map[string]net.IP, len(domains))
	for _, d := range domains {
		domainMap[strings.ToLower(strings.TrimSuffix(d, "."))] = proxyIP
	}

	s := &Server{
		cfg:     cfg,
		domains: domainMap,
		proxyIP: proxyIP,
		logger:  logger,
	}

	return s, nil
}

// Start starts the DNS server (UDP and TCP).
func (s *Server) Start() error {
	dnsMux := dns.NewServeMux()
	dnsMux.HandleFunc(".", s.handleQuery)

	// Pre-bind listeners to detect port conflicts immediately
	udpListener, err := net.ListenPacket("udp", s.cfg.Addr)
	if err != nil {
		return fmt.Errorf("failed to bind DNS UDP %s: %w", s.cfg.Addr, err)
	}

	tcpListener, err := net.Listen("tcp", s.cfg.Addr)
	if err != nil {
		udpListener.Close()
		return fmt.Errorf("failed to bind DNS TCP %s: %w", s.cfg.Addr, err)
	}

	s.udpSrv = &dns.Server{
		PacketConn: udpListener,
		Handler:    dnsMux,
	}

	s.tcpSrv = &dns.Server{
		Listener: tcpListener,
		Handler:  dnsMux,
	}

	go s.udpSrv.ActivateAndServe()
	go s.tcpSrv.ActivateAndServe()

	s.logger.Info("DNS server started", "addr", s.cfg.Addr, "proxyIP", s.cfg.ProxyIP, "domains", len(s.domains))
	for d := range s.domains {
		s.logger.Info("DNS domain mapping", "domain", d, "ip", s.cfg.ProxyIP)
	}

	return nil
}

// Shutdown gracefully shuts down the DNS server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down DNS server...")

	var errs []error

	if s.udpSrv != nil {
		if err := s.udpSrv.ShutdownContext(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if s.tcpSrv != nil {
		if err := s.tcpSrv.ShutdownContext(ctx); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("DNS shutdown errors: %v", errs)
	}
	return nil
}

// handleQuery processes incoming DNS queries.
func (s *Server) handleQuery(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		dns.HandleFailed(w, r)
		return
	}

	question := r.Question[0]
	domain := strings.ToLower(strings.TrimSuffix(question.Name, "."))

	switch question.Qtype {
	case dns.TypeA, dns.TypeAAAA:
		if s.isProxiedDomain(domain) {
			s.handleProxiedDomain(w, r, question)
			return
		}
	case dns.TypeCNAME:
		if s.isProxiedDomain(domain) {
			s.handleProxiedDomain(w, r, question)
			return
		}
	}

	s.forwardQuery(w, r)
}

// isProxiedDomain checks if the domain is in the proxied domain map.
func (s *Server) isProxiedDomain(domain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.domains[domain]
	return ok
}

// handleProxiedDomain responds with the proxy IP for a proxied domain.
func (s *Server) handleProxiedDomain(w dns.ResponseWriter, r *dns.Msg, question dns.Question) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true

	domain := question.Name

	switch question.Qtype {
	case dns.TypeA:
		rr := &dns.A{
			Hdr: dns.RR_Header{
				Name:   domain,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    300,
			},
			A: s.proxyIP.To4(),
		}
		if rr.A != nil {
			msg.Answer = append(msg.Answer, rr)
		}

	case dns.TypeAAAA:
		ip := s.proxyIP.To16()
		if ip != nil && ip.To4() == nil {
			rr := &dns.AAAA{
				Hdr: dns.RR_Header{
					Name:   domain,
					Rrtype: dns.TypeAAAA,
					Class:  dns.ClassINET,
					Ttl:    300,
				},
				AAAA: ip,
			}
			msg.Answer = append(msg.Answer, rr)
		}

	case dns.TypeCNAME:
		msg.Rcode = dns.RcodeNameError
	}

	s.logger.Debug("DNS query resolved", "domain", domain, "type", dns.TypeToString[question.Qtype], "ip", s.cfg.ProxyIP)

	if err := w.WriteMsg(msg); err != nil {
		s.logger.Error("Failed to write DNS response", "error", err)
	}
}

// forwardQuery forwards a DNS query to upstream servers.
func (s *Server) forwardQuery(w dns.ResponseWriter, r *dns.Msg) {
	c := &dns.Client{
		Timeout: 5 * time.Second,
		Net:     "udp",
	}

	for _, upstream := range s.cfg.Upstream {
		resp, _, err := c.Exchange(r, upstream)
		if err == nil && resp != nil {
			if err := w.WriteMsg(resp); err != nil {
				s.logger.Error("Failed to write forwarded DNS response", "error", err)
			}
			return
		}
	}

	c.Net = "tcp"
	for _, upstream := range s.cfg.Upstream {
		resp, _, err := c.Exchange(r, upstream)
		if err == nil && resp != nil {
			if err := w.WriteMsg(resp); err != nil {
				s.logger.Error("Failed to write forwarded DNS response (TCP)", "error", err)
			}
			return
		}
	}

	s.logger.Debug("DNS forward failed for all upstreams", "query", r.Question[0].Name)
	dns.HandleFailed(w, r)
}

// AddDomain adds a domain to the proxied domain map at runtime.
func (s *Server) AddDomain(domain string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.domains[strings.ToLower(strings.TrimSuffix(domain, "."))] = s.proxyIP
}

// RemoveDomain removes a domain from the proxied domain map.
func (s *Server) RemoveDomain(domain string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.domains, strings.ToLower(strings.TrimSuffix(domain, ".")))
}

package config

import (
	"reflect"
	"strings"
	"testing"
)

// TestMergeDNSVHost checks the --dns merge semantics and the DNS/VHost single switch.
func TestMergeDNSVHost(t *testing.T) {
	defaults := DNSConfig{
		Addr:     DNSDefaultAddr,
		Upstream: []string{DNSDefaultUpstream},
		ProxyIP:  DNSDefaultProxyIP,
	}

	tests := []struct {
		name        string
		dnsIn       DNSConfig
		vhostIn     VHostConfig
		flag        string
		wantDNS     DNSConfig
		wantVHost   VHostConfig
		expectError bool
	}{
		{
			name:      "no flag and empty config stays disabled with defaults",
			dnsIn:     DNSConfig{},
			vhostIn:   VHostConfig{},
			wantDNS:   defaults,
			wantVHost: VHostConfig{Port: VHostDefaultPort},
		},
		{
			name:    "no flag keeps enabled config unchanged",
			dnsIn:   DNSConfig{Enabled: true, Addr: ":5353", Upstream: []string{"1.1.1.1:53"}, ProxyIP: "10.0.0.1"},
			vhostIn: VHostConfig{Enabled: true, Port: 8080},
			wantDNS: DNSConfig{Enabled: true, Addr: ":5353", Upstream: []string{"1.1.1.1:53"}, ProxyIP: "10.0.0.1"}, wantVHost: VHostConfig{Enabled: true, Port: 8080},
		},
		{
			name:      "legacy vhost-only config gets disabled",
			dnsIn:     defaults,
			vhostIn:   VHostConfig{Enabled: true, Port: 80},
			wantDNS:   defaults,
			wantVHost: VHostConfig{Enabled: false, Port: 80},
		},
		{
			name:      "dns enabled forces vhost on",
			dnsIn:     DNSConfig{Enabled: true},
			vhostIn:   VHostConfig{},
			wantDNS:   DNSConfig{Enabled: true, Addr: DNSDefaultAddr, Upstream: []string{DNSDefaultUpstream}, ProxyIP: DNSDefaultProxyIP},
			wantVHost: VHostConfig{Enabled: true, Port: VHostDefaultPort},
		},
		{
			name:      "flag enables both and sets proxy ip",
			flag:      "192.168.1.100",
			wantDNS:   DNSConfig{Enabled: true, Addr: DNSDefaultAddr, Upstream: []string{DNSDefaultUpstream}, ProxyIP: "192.168.1.100"},
			wantVHost: VHostConfig{Enabled: true, Port: VHostDefaultPort},
		},
		{
			name:      "flag overrides configured proxy ip",
			dnsIn:     DNSConfig{Enabled: true, Addr: ":53", Upstream: []string{"8.8.8.8:53"}, ProxyIP: "127.0.0.1"},
			vhostIn:   VHostConfig{Enabled: true, Port: 80},
			flag:      "10.20.30.40",
			wantDNS:   DNSConfig{Enabled: true, Addr: ":53", Upstream: []string{"8.8.8.8:53"}, ProxyIP: "10.20.30.40"},
			wantVHost: VHostConfig{Enabled: true, Port: 80},
		},
		{
			name:      "flag ipv6 accepted",
			flag:      "::1",
			wantDNS:   DNSConfig{Enabled: true, Addr: DNSDefaultAddr, Upstream: []string{DNSDefaultUpstream}, ProxyIP: "::1"},
			wantVHost: VHostConfig{Enabled: true, Port: VHostDefaultPort},
		},
		{
			name:        "flag hostname rejected",
			flag:        "example.com",
			expectError: true,
		},
		{
			name:        "flag boolean rejected",
			flag:        "false",
			expectError: true,
		},
		{
			name:        "flag garbage rejected",
			flag:        "not-an-ip!",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dns, vhost, err := mergeDNSVHost(tt.dnsIn, tt.vhostIn, tt.flag)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), "--dns") {
					t.Errorf("error should mention --dns, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(dns, tt.wantDNS) {
				t.Errorf("dns = %+v, want %+v", dns, tt.wantDNS)
			}
			if !reflect.DeepEqual(vhost, tt.wantVHost) {
				t.Errorf("vhost = %+v, want %+v", vhost, tt.wantVHost)
			}
		})
	}
}

// TestDefaultAppConfigDNSVHostDisabled checks that DNS and VHost default to disabled.
func TestDefaultAppConfigDNSVHostDisabled(t *testing.T) {
	cfg := defaultAppConfig()
	if cfg.DNS.Enabled {
		t.Error("default DNS should be disabled")
	}
	if cfg.VHost.Enabled {
		t.Error("default VHost should be disabled")
	}
	if cfg.DNS.Addr != DNSDefaultAddr {
		t.Errorf("default DNS addr = %q, want %q", cfg.DNS.Addr, DNSDefaultAddr)
	}
	if len(cfg.DNS.Upstream) != 1 || cfg.DNS.Upstream[0] != DNSDefaultUpstream {
		t.Errorf("default DNS upstream = %v, want [%s]", cfg.DNS.Upstream, DNSDefaultUpstream)
	}
	if cfg.DNS.ProxyIP != DNSDefaultProxyIP {
		t.Errorf("default DNS proxyIP = %q, want %q", cfg.DNS.ProxyIP, DNSDefaultProxyIP)
	}
	if cfg.VHost.Port != VHostDefaultPort {
		t.Errorf("default VHost port = %d, want %d", cfg.VHost.Port, VHostDefaultPort)
	}
}

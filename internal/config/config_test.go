package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestMergeDNSVHost checks the --dns merge semantics and the DNS/VHost single switch.
func TestMergeDNSVHost(t *testing.T) {
	defaults := DNSConfig{
		Addr:     DNSDefaultAddr,
		Upstream: DNSDefaultUpstreams,
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
			wantDNS:   DNSConfig{Enabled: true, Addr: DNSDefaultAddr, Upstream: DNSDefaultUpstreams, ProxyIP: DNSDefaultProxyIP},
			wantVHost: VHostConfig{Enabled: true, Port: VHostDefaultPort},
		},
		{
			name:      "flag enables both and sets proxy ip",
			flag:      "192.168.1.100",
			wantDNS:   DNSConfig{Enabled: true, Addr: DNSDefaultAddr, Upstream: DNSDefaultUpstreams, ProxyIP: "192.168.1.100"},
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
			wantDNS:   DNSConfig{Enabled: true, Addr: DNSDefaultAddr, Upstream: DNSDefaultUpstreams, ProxyIP: "::1"},
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
	if len(cfg.DNS.Upstream) != 3 || !reflect.DeepEqual(cfg.DNS.Upstream, DNSDefaultUpstreams) {
		t.Errorf("default DNS upstream = %v, want %v", cfg.DNS.Upstream, DNSDefaultUpstreams)
	}
	if cfg.DNS.ProxyIP != DNSDefaultProxyIP {
		t.Errorf("default DNS proxyIP = %q, want %q", cfg.DNS.ProxyIP, DNSDefaultProxyIP)
	}
	if cfg.VHost.Port != VHostDefaultPort {
		t.Errorf("default VHost port = %d, want %d", cfg.VHost.Port, VHostDefaultPort)
	}
	if cfg.Cache.Directory != DefaultCacheDir {
		t.Errorf("default cache directory = %q, want %q", cfg.Cache.Directory, DefaultCacheDir)
	}
}

// TestResolveCacheDir checks the preferred/fallback cache directory resolution.
func TestResolveCacheDir(t *testing.T) {
	tmpDir := t.TempDir()

	preferred := filepath.Join(tmpDir, "preferred")
	if got := resolveCacheDir(preferred, FallbackCacheDir); got != preferred {
		t.Errorf("resolveCacheDir(creatable) = %q, want %q", got, preferred)
	}

	blocker := filepath.Join(tmpDir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("failed to create blocker file: %v", err)
	}
	fallback := filepath.Join(tmpDir, "fallback")
	if got := resolveCacheDir(filepath.Join(blocker, "sub"), fallback); got != fallback {
		t.Errorf("resolveCacheDir(non-creatable) = %q, want %q", got, fallback)
	}
}

// TestLoadAppConfigLegacyExcludeMigration checks that deprecated exclusion keys
// are converted to excludeLastPfx when it is not set, and ignored when it is.
func TestLoadAppConfigLegacyExcludeMigration(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.json")

	legacyJSON := `{
		"cache": {
			"enabled": true,
			"directory": "./cache",
			"excludeExtensions": ["html", "js"],
			"excludePaths": ["/ubuntu/dists/"]
		}
	}`
	if err := os.WriteFile(path, []byte(legacyJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err := LoadAppConfig(path)
	if err != nil {
		t.Fatalf("LoadAppConfig failed: %v", err)
	}
	want := []string{"/ubuntu/dists/", ".html", ".js"}
	if !reflect.DeepEqual(cfg.Cache.ExcludeLastPfx, want) {
		t.Errorf("migrated excludeLastPfx = %v, want %v", cfg.Cache.ExcludeLastPfx, want)
	}

	bothJSON := `{
		"cache": {
			"enabled": true,
			"directory": "./cache",
			"excludeLastPfx": ["index.html"],
			"excludeExtensions": ["html"],
			"excludePaths": ["/ubuntu/dists/"]
		}
	}`
	if err := os.WriteFile(path, []byte(bothJSON), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
	cfg, err = LoadAppConfig(path)
	if err != nil {
		t.Fatalf("LoadAppConfig failed: %v", err)
	}
	if !reflect.DeepEqual(cfg.Cache.ExcludeLastPfx, []string{"index.html"}) {
		t.Errorf("excludeLastPfx = %v, want [index.html] (legacy keys should be ignored)", cfg.Cache.ExcludeLastPfx)
	}
}

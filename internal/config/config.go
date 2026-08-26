package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/peng-mj/scproxy/internal/validation"
)

const (
	AppName            = "scproxy"
	LocalConfig        = "./cache/config.json"
	EtcConfig          = "/etc/scproxy/config.json"
	DNSDefaultAddr     = ":53"
	DNSDefaultUpstream = "8.8.8.8:53"
	DNSDefaultProxyIP  = "127.0.0.1"
	VHostDefaultPort   = 80
	TLSDefaultPort     = 443
	TLSCertDirDefault  = "./certs"
)

type Config struct {
	Proxy   string
	Host    string
	Cache   CacheConfig
	Logging LoggingConfig
	DNS     DNSConfig
	VHost   VHostConfig
	TLS     TLSConfig
	Routes  []validation.RouteConfig
}

func New(flags *CLIFlags) (*Config, string, error) {
	configPath := flags.ConfigPath

	if configPath == "" {
		if _, err := os.Stat(LocalConfig); err == nil {
			configPath = LocalConfig
			fmt.Printf("Using existing configuration at %s\n", configPath)
		} else if _, err := os.Stat(EtcConfig); err == nil {
			configPath = EtcConfig
			fmt.Printf("Using existing configuration at %s\n", configPath)
		} else {
			configPath = EtcConfig
		}
	}

	var appConfig *AppConfig
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("Creating default configuration at %s\n", configPath)
		if err := InitDefaultAppConfig(configPath); err != nil {
			fmt.Printf("Failed to create config at %s (error: %v), falling back to %s\n", configPath, err, LocalConfig)
			configPath = LocalConfig
			fmt.Printf("Creating default configuration at %s\n", configPath)
			if err := InitDefaultAppConfig(configPath); err != nil {
				return nil, "", fmt.Errorf("failed to create default config: %v", err)
			}
		}
		appConfig, err = LoadAppConfig(configPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load default config: %v", err)
		}
	} else {
		var err error
		appConfig, err = LoadAppConfig(configPath)
		if err != nil {
			return nil, "", fmt.Errorf("failed to load config: %v", err)
		}
	}

	proxy := flags.Proxy
	target := flags.Target
	port := flags.Port
	host := flags.Host
	logLevel := flags.LogLevel
	logOutput := flags.LogOutput
	enableCache := flags.EnableCache

	if proxy == "" {
		proxy = appConfig.Proxy
	}

	if target != "" && port > 0 {
		portExists := false
		for _, route := range appConfig.Routes {
			if route.Port == port {
				portExists = true
				break
			}
		}
		if !portExists {
			additionalRoute := validation.RouteConfig{
				Target: target,
				Port:   port,
			}
			appConfig.Routes = append(appConfig.Routes, additionalRoute)
			fmt.Printf("Added route from CLI: %s -> port %d\n", target, port)
		} else {
			fmt.Printf("Warning: port %d already exists in config, skipping CLI route\n", port)
		}
	}

	loggingConfig := appConfig.Logging
	if logLevel != "" {
		loggingConfig.Level = LogLevel(logLevel)
	}
	if logOutput != "" {
		loggingConfig.Output = LogOutput(logOutput)
	}

	cacheConfig := appConfig.Cache
	if enableCache {
		cacheConfig.Enabled = true
	}

	hostConfig := appConfig.Host
	if host != "" {
		hostConfig = host
	}

	dnsConfig := appConfig.DNS
	if !dnsConfig.Enabled && dnsConfig.Addr == "" && len(dnsConfig.Upstream) == 0 && dnsConfig.ProxyIP == "" {
		dnsConfig.Enabled = true
		dnsConfig.Addr = DNSDefaultAddr
		dnsConfig.Upstream = []string{DNSDefaultUpstream}
		dnsConfig.ProxyIP = DNSDefaultProxyIP
	}
	if dnsConfig.Addr == "" {
		dnsConfig.Addr = DNSDefaultAddr
	}
	if len(dnsConfig.Upstream) == 0 {
		dnsConfig.Upstream = []string{DNSDefaultUpstream}
	}
	if dnsConfig.ProxyIP == "" {
		dnsConfig.ProxyIP = DNSDefaultProxyIP
	}
	if flags.DNSDisable {
		dnsConfig.Enabled = false
	}
	if flags.DNSEnable {
		dnsConfig.Enabled = true
	}
	if flags.DNSAddr != "" {
		dnsConfig.Addr = flags.DNSAddr
	}
	if flags.DNSUpstream != "" {
		dnsConfig.Upstream = splitUpstream(flags.DNSUpstream)
	}
	if flags.DNSProxyIP != "" {
		dnsConfig.ProxyIP = flags.DNSProxyIP
	}

	vhostConfig := appConfig.VHost
	if !vhostConfig.Enabled && vhostConfig.Port == 0 {
		vhostConfig.Enabled = true
		vhostConfig.Port = VHostDefaultPort
	}
	if vhostConfig.Port == 0 {
		vhostConfig.Port = VHostDefaultPort
	}
	if flags.VHostDisable {
		vhostConfig.Enabled = false
	}
	if flags.VHostEnable {
		vhostConfig.Enabled = true
	}
	if flags.VHostPort > 0 {
		vhostConfig.Port = flags.VHostPort
	}

	tlsConfig := appConfig.TLS
	if !tlsConfig.Enabled && tlsConfig.Port == 0 && tlsConfig.CertDir == "" {
		tlsConfig.Enabled = true
		tlsConfig.Port = TLSDefaultPort
		tlsConfig.CertDir = TLSCertDirDefault
		tlsConfig.SkipUpstreamVerify = true
		tlsConfig.RedirectHTTP = true
	}
	if tlsConfig.Port == 0 {
		tlsConfig.Port = TLSDefaultPort
	}
	if tlsConfig.CertDir == "" {
		tlsConfig.CertDir = TLSCertDirDefault
	}
	if flags.TLSDisable {
		tlsConfig.Enabled = false
	}
	if flags.TLSEnable {
		tlsConfig.Enabled = true
	}
	if flags.TLSPort > 0 {
		tlsConfig.Port = flags.TLSPort
	}
	if flags.TLSCertDir != "" {
		tlsConfig.CertDir = flags.TLSCertDir
	}
	if flags.TLSVerifyUpstream {
		tlsConfig.SkipUpstreamVerify = false
	}
	if flags.TLSNoRedirectHTTP {
		tlsConfig.RedirectHTTP = false
	}

	cfg := &Config{
		Proxy:   proxy,
		Host:    hostConfig,
		Cache:   cacheConfig,
		Logging: loggingConfig,
		DNS:     dnsConfig,
		VHost:   vhostConfig,
		TLS:     tlsConfig,
		Routes:  appConfig.Routes,
	}

	if err := validateConfig(proxy, appConfig.Routes, vhostConfig.Port, vhostConfig.Enabled, tlsConfig); err != nil {
		return nil, "", fmt.Errorf("invalid configuration: %v", err)
	}

	return cfg, configPath, nil
}

func splitUpstream(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func validateConfig(proxy string, routes []validation.RouteConfig, vhostPort int, vhostEnabled bool, tlsCfg TLSConfig) error {
	if err := validation.ValidateRoutes(routes); err != nil {
		return err
	}

	if proxy != "" {
		if err := validation.ValidateURL(proxy); err != nil {
			return fmt.Errorf("invalid proxy URL: %v", err)
		}
	}

	if len(routes) == 0 {
		return fmt.Errorf("no routes configured. Please add routes to config file or use --target and --port")
	}

	if vhostEnabled {
		for _, route := range routes {
			if route.Port == vhostPort {
				return fmt.Errorf("vhost port %d conflicts with a route port", vhostPort)
			}
		}
	}

	if tlsCfg.Enabled {
		for _, route := range routes {
			if route.Port == tlsCfg.Port {
				return fmt.Errorf("TLS port %d conflicts with a route port", tlsCfg.Port)
			}
		}
		if vhostEnabled && tlsCfg.Port == vhostPort {
			return fmt.Errorf("TLS port %d conflicts with vhost port", tlsCfg.Port)
		}
	}

	return nil
}

// UpdateAndSaveAppConfig updates the app config file with CLI parameters and saves it.
func UpdateAndSaveAppConfig(configPath string, flags *CLIFlags, cfg *Config) error {
	appConfig, err := LoadAppConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load app config: %v", err)
	}

	logLevel := flags.LogLevel
	logOutput := flags.LogOutput

	if logLevel != "" {
		appConfig.Logging.Level = LogLevel(logLevel)
	}
	if logOutput != "" {
		appConfig.Logging.Output = LogOutput(logOutput)
	}

	enableCache := flags.EnableCache
	if enableCache {
		appConfig.Cache.Enabled = true
	}

	appConfig.Host = cfg.Host
	appConfig.Cache = cfg.Cache
	appConfig.DNS = cfg.DNS
	appConfig.VHost = cfg.VHost
	appConfig.TLS = cfg.TLS

	if cfg.Proxy != "" {
		appConfig.Proxy = cfg.Proxy
	}

	target := flags.Target
	port := flags.Port
	if target != "" && port > 0 {
		portExists := false
		for _, route := range appConfig.Routes {
			if route.Port == port {
				portExists = true
				break
			}
		}
		if !portExists {
			appConfig.Routes = append(appConfig.Routes, validation.RouteConfig{
				Target: target,
				Port:   port,
			})
		}
	}

	if err := SaveAppConfig(appConfig, configPath); err != nil {
		return fmt.Errorf("failed to save app config: %v", err)
	}

	return nil
}

// Package config manages application configuration using the Koanf library.
//
// This package provides structures and functions for handling application
// configuration settings. It uses the Koanf library to facilitate loading
// configuration from various sources.
//
// The main structures include:
//
//   - Config: Represents the overall configuration object, containing nested
//     configurations for Host, Port, Proxy URL and Target URL, Cache settings,
//     and Logging settings.
//
//   - LoggingConfig: Holds logging configuration settings, including the log level,
//     format, output destination, and path for log files. It includes validation
//     to ensure the logging settings are correct and conform to allowed values.
//
// The package also provides a New function to create a new configuration
// instance, initializing it with default values, loading settings from environment
// variables and processing command line flags. It ensures that settings are
// validated before they are used, enhancing the reliability of the application.
package config

import (
	"fmt"
	"os"

	"github.com/peng-mj/scproxy/internal/validation"
)

// Config represents a configuration object. This type is
// designed to hold server and other configurations.
type Config struct {
	Proxy   string                   // Outbound Proxy URL (from CLI or JSON config)
	Host    string                   // Server listening host (from JSON config)
	Cache   CacheConfig              // Cache configuration (from JSON config)
	Logging LoggingConfig            // Logging configuration (from JSON config)
	DNS     DNSConfig                // DNS proxy configuration (from JSON config)
	VHost   VHostConfig              // Virtual host configuration (from JSON config)
	TLS     TLSConfig                // TLS/HTTPS configuration (from JSON config)
	Routes  []validation.RouteConfig // Routes configuration (from JSON config)
}

// AppName is the name of the application.
const AppName = "scproxy"

// New loads the application configuration from various sources:
//   - JSON configuration file (created with defaults if missing)
//   - Command line flags (target, proxy, port)
func New(flags *CLIFlags) (*Config, error) {
	// 1. Get configuration file path
	configPath := flags.ConfigPath
	if configPath == "" {
		configPath = "./cache/scproxy.json"
	}

	// 2. Load JSON configuration
	var appConfig *AppConfig
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Config file doesn't exist, create default config
		fmt.Printf("Creating default configuration at %s\n", configPath)
		if err := InitDefaultAppConfig(configPath); err != nil {
			return nil, fmt.Errorf("failed to create default config: %v", err)
		}
		appConfig, err = LoadAppConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load default config: %v", err)
		}
	} else {
		// Config file exists, load it
		var err error
		appConfig, err = LoadAppConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config: %v", err)
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

	// DNS config: apply defaults for missing fields
	dnsConfig := appConfig.DNS
	if dnsConfig.Addr == "" {
		dnsConfig.Addr = ":53"
	}
	if len(dnsConfig.Upstream) == 0 {
		dnsConfig.Upstream = []string{"8.8.8.8:53"}
	}
	if dnsConfig.ProxyIP == "" {
		dnsConfig.ProxyIP = "127.0.0.1"
	}

	// VHost config: apply defaults for missing fields
	vhostConfig := appConfig.VHost
	if vhostConfig.Port == 0 {
		vhostConfig.Port = 80
	}

	// TLS config: apply defaults for missing fields
	tlsConfig := appConfig.TLS
	if tlsConfig.Port == 0 {
		tlsConfig.Port = 443
	}
	if tlsConfig.CertDir == "" {
		tlsConfig.CertDir = "./certs"
	}

	// Gateway mode: enable DNS(:53) + VHost(:80) + TLS(:443) together
	if flags.Gateway {
		dnsConfig.Enabled = true
		vhostConfig.Enabled = true
		tlsConfig.Enabled = true
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

	// 9. Validate the configuration
	if err := validateConfig(proxy, appConfig.Routes, vhostConfig.Port, vhostConfig.Enabled, tlsConfig); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return cfg, nil
}

// validateConfig checks the validity of the configuration.
func validateConfig(proxy string, routes []validation.RouteConfig, vhostPort int, vhostEnabled bool, tlsCfg TLSConfig) error {
	// Validate routes
	if err := validation.ValidateRoutes(routes); err != nil {
		return err
	}

	// Validate proxy URL (optional)
	if proxy != "" {
		if err := validation.ValidateURL(proxy); err != nil {
			return fmt.Errorf("invalid proxy URL: %v", err)
		}
	}

	// Check if we have at least one route
	if len(routes) == 0 {
		return fmt.Errorf("no routes configured. Please add routes to config file or use --target and --port")
	}

	// Check VHost port doesn't conflict with route ports
	if vhostEnabled {
		for _, route := range routes {
			if route.Port == vhostPort {
				return fmt.Errorf("vhost port %d conflicts with a route port", vhostPort)
			}
		}
	}

	// Check TLS port doesn't conflict with route or vhost ports
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

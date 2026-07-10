// Package main is the entry point for the scproxy application.
//
// It defines the command-line interface (CLI) using the pflag library,
// handles configuration loading from flags, sets up structured logging, and
// manages the lifecycle of the proxy server, including graceful shutdown.

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/logging"
	"github.com/peng-mj/scproxy/internal/scproxy"
	"github.com/peng-mj/scproxy/internal/stats"
	"github.com/peng-mj/scproxy/internal/version"
	"github.com/spf13/pflag"
)

var (
	// Target/Proxy flags
	targetFlag = pflag.StringP("target", "t", "", "target service URL (required for single-target mode, optional for batch mode)")
	proxyFlag  = pflag.StringP("proxy", "x", "", "outbound HTTP proxy URL (empty = direct connection)")
	portFlag   = pflag.IntP("port", "P", 0, "port to listen on (required for single-target mode, optional for batch mode)")

	// Server config
	hostFlag = pflag.StringP("host", "H", "", "host to listen on (default: 0.0.0.0)")

	// Config file
	configFlag = pflag.StringP("config", "c", "./cache/scproxy.json", "config file path")

	// Cache flags
	cacheFlag      = pflag.Bool("cache", false, "enable caching")
	clearCacheFlag = pflag.Bool("clear-cache", false, "clear all cached data and exit")
	yesFlag        = pflag.Bool("yes", false, "auto-confirm cache clearing (skip prompt)")

	// Logging flags
	logLevelFlag  = pflag.StringP("log-level", "l", "", "set log level")
	logOutputFlag = pflag.StringP("log-output", "o", "", "set log output")

	// DNS proxy flags
	dnsEnableFlag   = pflag.Bool("dns", false, "force enable DNS proxy server")
	dnsDisableFlag  = pflag.Bool("no-dns", false, "disable DNS proxy server (overrides config)")
	dnsAddrFlag     = pflag.String("dns-addr", "", "DNS server listen address (default: \":53\")")
	dnsUpstreamFlag = pflag.String("dns-upstream", "", "upstream DNS servers, comma-separated (default: \"8.8.8.8:53\")")
	dnsProxyIPFlag  = pflag.String("dns-proxy-ip", "", "IP returned by DNS for proxied domains (default: \"127.0.0.1\")")

	// VHost reverse proxy flags
	vhostEnableFlag  = pflag.Bool("vhost", false, "force enable virtual host reverse proxy")
	vhostDisableFlag = pflag.Bool("no-vhost", false, "disable virtual host reverse proxy (overrides config)")
	vhostPortFlag    = pflag.Int("vhost-port", 0, "virtual host proxy listen port (default: 80)")

	// Version flag
	showVersion = pflag.BoolP("version", "v", false, "show version information and exit")

	// Summary flag
	showSummary = pflag.BoolP("summary", "s", false, "show statistics summary and exit")
)

func init() {
	// Set custom usage function
	pflag.Usage = printUsage
}

// printUsage displays custom help text with examples
func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", config.AppName)
	fmt.Fprintln(os.Stderr, "Forwards HTTP requests to a target (optionally via an external proxy)")
	fmt.Fprintln(os.Stderr, "\nOptions:")
	pflag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\nExamples:")
	fmt.Fprintln(os.Stderr, "  # Single target mode")
	fmt.Fprintln(os.Stderr, "  scproxy --target https://example.com --proxy http://proxy:8080 --port 8080")
	fmt.Fprintln(os.Stderr, "\n  # Batch mode (from config file)")
	fmt.Fprintln(os.Stderr, "  scproxy --proxy http://proxy:8080")
	fmt.Fprintln(os.Stderr, "\n  # DNS + VHost mode (one-stop proxy)")
	fmt.Fprintln(os.Stderr, "  scproxy --dns --vhost --dns-proxy-ip 192.168.1.100")
	fmt.Fprintln(os.Stderr, "\n  # Clear cache")
	fmt.Fprintln(os.Stderr, "  scproxy --clear-cache --yes")
}

// printVersion displays version information
func printVersion() {
	fmt.Printf("%s %s\n", config.AppName, version.Get().String())
}

// printSummary displays statistics summary
func printSummary() {
	// Load config to get statistics
	configPath := "./cache/scproxy.json"
	if *configFlag != "" {
		configPath = *configFlag
	}

	// Load config to get statistics
	appConfig, err := config.LoadAppConfig(configPath)
	if err != nil {
		// Try loading from stats file as fallback
		statsFile := "./scproxy_stats.json"
		if err := stats.PrintSummary(statsFile); err != nil {
			fmt.Fprintf(os.Stderr, "No statistics found. Run scproxy with cache enabled first.\n")
			os.Exit(1)
		}
		return
	}

	// Check if summary exists
	if appConfig.Summary == nil {
		fmt.Fprintf(os.Stderr, "No statistics found. Run scproxy with cache enabled first.\n")
		os.Exit(1)
	}

	// Print summary from config
	stats.PrintAllStats(appConfig.Summary)
}

// main defines the CLI, initializes the configuration, sets up logging,
// and starts the application.
func main() {
	// Parse flags
	pflag.Parse()

	// Handle --version (early exit)
	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	// Handle --summary (early exit)
	if *showSummary {
		printSummary()
		os.Exit(0)
	}

	// Populate CLIFlags struct
	flags := &config.CLIFlags{
		Target:       *targetFlag,
		Proxy:        *proxyFlag,
		Port:         *portFlag,
		Host:         *hostFlag,
		ConfigPath:   *configFlag,
		EnableCache:  *cacheFlag,
		LogLevel:     *logLevelFlag,
		LogOutput:    *logOutputFlag,
		ClearCache:   *clearCacheFlag,
		Yes:          *yesFlag,
		DNSEnable:    *dnsEnableFlag,
		DNSDisable:   *dnsDisableFlag,
		DNSAddr:      *dnsAddrFlag,
		DNSUpstream:  *dnsUpstreamFlag,
		DNSProxyIP:   *dnsProxyIPFlag,
		VHostEnable:  *vhostEnableFlag,
		VHostDisable: *vhostDisableFlag,
		VHostPort:    *vhostPortFlag,
	}

	// Handle --clear-cache (early exit)
	if flags.ClearCache {
		if err := clearCache(flags); err != nil {
			log.Fatal(err)
		}
		return
	}

	// Load configuration
	cfg, err := config.New(flags)
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Save CLI parameters to config file
	configPath := flags.ConfigPath
	if configPath == "" {
		configPath = "./cache/scproxy.json"
	}
	if err := config.UpdateAndSaveAppConfig(configPath, flags, cfg); err != nil {
		// Log warning but don't fail - the server can still run
		fmt.Printf("Warning: failed to save config file: %v\n", err)
	}

	// Setup logger
	logger, err := logging.New(&cfg.Logging)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	logger.Debug("Configuration loaded successfully", "config", cfg)

	// Start scproxy in batch mode (always use scproxyManager now)
	logger.Info("Starting scproxy...", "routes", len(cfg.Routes))
	manager, err := scproxy.NewscproxyManager(cfg, logger, configPath)
	if err != nil {
		log.Fatalf("Failed to create proxy manager: %v", err)
	}

	// Start all servers
	if err := manager.Start(); err != nil {
		log.Fatalf("Failed to start servers: %v", err)
	}

	logger.Info("All servers started successfully")

	// Wait for signal
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-signalCtx.Done()
	stop()

	logger.Info("Shutdown signal received. Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := manager.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Error during graceful shutdown: %v", err)
	}

	logger.Info("All done! scproxy has been shut down.")

	// Print statistics summary
	printSummary()

	// Cleanly close the logger before exiting.
	if cerr := logger.Close(); cerr != nil {
		log.Fatalf("Failed to close log file (%s): %v", cfg.Logging.Path, cerr)
	}
}

// clearCache handles the cache clearing operation.
func clearCache(flags *config.CLIFlags) error {
	// Get configuration file path
	configPath := flags.ConfigPath
	if configPath == "" {
		configPath = "./cache/scproxy.json"
	}

	// Load configuration
	appConfig, err := config.LoadAppConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Check if cache is enabled
	if !appConfig.Cache.Enabled {
		return fmt.Errorf("cache is not enabled in configuration")
	}

	fmt.Printf("Preparing to clear cache...\n")
	fmt.Printf("Cache directory: %s\n", appConfig.Cache.Directory)

	// Get cache statistics
	stats, err := cache.GetStats(appConfig.Cache.Directory)
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %v", err)
	}

	fmt.Printf("Cache information:\n")
	fmt.Printf("  Total files: %d\n", stats.TotalFiles)
	fmt.Printf("  Total size: %.2f MB\n", stats.TotalSizeMB)

	// Confirm unless --yes flag is provided
	if !flags.Yes {
		fmt.Printf("Are you sure you want to clear the cache? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cache clearing cancelled.")
			return nil
		}
	}

	// Clear the cache
	fmt.Println("Clearing cache...")
	if err := cache.Clear(appConfig.Cache.Directory); err != nil {
		return fmt.Errorf("failed to clear cache: %v", err)
	}

	fmt.Println("Cache cleared successfully.")
	return nil
}

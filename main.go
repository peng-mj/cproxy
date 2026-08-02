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
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/peng-mj/scproxy/internal/cache"
	"github.com/peng-mj/scproxy/internal/config"
	"github.com/peng-mj/scproxy/internal/install"
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

	// Gateway mode flag: enables DNS(:53) + HTTP(:80) + HTTPS(:443) together
	gatewayFlag = pflag.Bool("gateway", false, "enable gateway mode: DNS + HTTP(:80) + HTTPS(:443), serving both in parallel")

	// Install/uninstall local DNS wiring (Linux): point system resolver at
	// scproxy and trust its CA so gateway mode works on a single host.
	installFlag   = pflag.Bool("install", false, "wire local DNS to scproxy (free :53, set resolver to proxyIP, trust CA) and exit")
	uninstallFlag = pflag.Bool("uninstall", false, "undo --install (restore previous resolver) and exit")

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
	fmt.Fprintln(os.Stderr, "\n  # Gateway mode (DNS:53 + HTTP:80 + HTTPS:443, both serve in parallel)")
	fmt.Fprintln(os.Stderr, "  scproxy --target https://example.com --gateway")
	fmt.Fprintln(os.Stderr, "\n  # Single-host gateway: install local DNS + CA first, then run")
	fmt.Fprintln(os.Stderr, "  sudo scproxy --install && sudo scproxy --target https://example.com --gateway")
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

	// Handle --install/--uninstall (early exit)
	if *installFlag {
		if err := install.Install(*configFlag); err != nil {
			fmt.Fprintf(os.Stderr, "install failed: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *uninstallFlag {
		if err := install.Uninstall(*configFlag); err != nil {
			fmt.Fprintf(os.Stderr, "uninstall failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Populate CLIFlags struct
	flags := &config.CLIFlags{
		Target:      *targetFlag,
		Proxy:       *proxyFlag,
		Port:        *portFlag,
		Host:        *hostFlag,
		ConfigPath:  *configFlag,
		EnableCache: *cacheFlag,
		LogLevel:    *logLevelFlag,
		LogOutput:   *logOutputFlag,
		ClearCache:  *clearCacheFlag,
		Yes:         *yesFlag,
		Gateway:     *gatewayFlag,
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

	// Check root privileges for privileged ports (DNS :53, VHost :80, TLS :443)
	if err := checkPrivileges(cfg); err != nil {
		log.Fatalf("%v", err)
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

	// Set as default slog handler for packages using slog directly
	logger.SetDefault()

	logger.Debug("Configuration loaded successfully", "config", cfg)

	// Gateway mode: ensure proxyIP is reachable; if not, add it to lo.
	if cfg.DNS.Enabled && cfg.DNS.ProxyIP != "" {
		if err := install.EnsureLoopback(cfg.DNS.ProxyIP); err != nil {
			logger.Warn("Failed to ensure loopback alias — scproxy may be unreachable at proxyIP",
				"proxyIP", cfg.DNS.ProxyIP, "err", err)
		}
	}

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

// checkPrivileges verifies that the current process has sufficient privileges
// to bind any privileged ports (< 1024) required by the configuration.
// On Unix, binding ports below 1024 requires root or CAP_NET_BIND_SERVICE.
// If a privileged port is needed but the process is not root, it returns an
// error instructing the user to re-run with sudo.
func checkPrivileges(cfg *config.Config) error {
	type listener struct {
		name string
		port int
	}
	var privileged []listener

	if cfg.DNS.Enabled {
		if port := portFromAddr(cfg.DNS.Addr); port > 0 && port < 1024 {
			privileged = append(privileged, listener{"DNS", port})
		}
	}
	if cfg.VHost.Enabled && cfg.VHost.Port > 0 && cfg.VHost.Port < 1024 {
		privileged = append(privileged, listener{"VHost HTTP", cfg.VHost.Port})
	}
	if cfg.TLS.Enabled && cfg.TLS.Port > 0 && cfg.TLS.Port < 1024 {
		privileged = append(privileged, listener{"VHost HTTPS", cfg.TLS.Port})
	}

	if len(privileged) == 0 {
		return nil
	}

	if os.Getuid() != 0 {
		var parts []string
		for _, l := range privileged {
			parts = append(parts, fmt.Sprintf("%s(:%d)", l.name, l.port))
		}
		return fmt.Errorf(
			"the following listeners require root privileges: %s\n"+
				"re-run with: sudo %s",
			strings.Join(parts, ", "),
			strings.Join(os.Args, " "),
		)
	}
	return nil
}

// portFromAddr extracts the numeric port from a listen address like ":53" or
// "0.0.0.0:53". Returns 0 if the port cannot be parsed.
func portFromAddr(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

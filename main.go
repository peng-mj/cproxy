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

const (
	LocalConfig   = "./cache/config.json"
	EtcConfig     = "/etc/scproxy/config.json"
	StatsFilePath = "./scproxy_stats.json"
)

var (
	targetFlag            = pflag.StringP("target", "t", "", "target service URL (required for single-target mode, optional for batch mode)")
	proxyFlag             = pflag.StringP("proxy", "x", "", "outbound HTTP proxy URL (empty = direct connection)")
	portFlag              = pflag.IntP("port", "P", 0, "port to listen on (required for single-target mode, optional for batch mode)")
	hostFlag              = pflag.StringP("host", "H", "", "host to listen on (default: 0.0.0.0)")
	configFlag            = pflag.StringP("config", "c", "", "config file path (default: auto-detect from ./cache/config.json or /etc/scproxy/config.json)")
	cacheFlag             = pflag.Bool("cache", false, "enable caching")
	clearCacheFlag        = pflag.Bool("clear-cache", false, "clear all cached data and exit")
	yesFlag               = pflag.Bool("yes", false, "auto-confirm cache clearing (skip prompt)")
	logLevelFlag          = pflag.StringP("log-level", "l", "", "set log level")
	logOutputFlag         = pflag.StringP("log-output", "o", "", "set log output")
	dnsFlag               = pflag.String("dns", "", "enable DNS + VHost mode; value sets the IP returned by DNS for proxied domains (e.g. --dns 192.168.1.100, default: \"127.0.0.1\")")
	tlsEnableFlag         = pflag.Bool("tls", false, "force enable HTTPS TLS listener")
	tlsDisableFlag        = pflag.Bool("no-tls", false, "disable HTTPS TLS listener (overrides config)")
	tlsPortFlag           = pflag.Int("tls-port", 0, "HTTPS TLS listen port (default: 443)")
	tlsCertDirFlag        = pflag.String("tls-cert-dir", "", "certificate storage directory (default: ./certs)")
	tlsVerifyUpstreamFlag = pflag.Bool("tls-verify-upstream", false, "verify upstream TLS certificates (default: skip)")
	tlsNoRedirectFlag     = pflag.Bool("tls-no-redirect", false, "disable HTTP→HTTPS redirect")
	showVersion           = pflag.BoolP("version", "v", false, "show version information and exit")
	showSummary           = pflag.BoolP("summary", "s", false, "show statistics summary and exit")
)

func init() {
	pflag.Usage = printUsage
}

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
	fmt.Fprintln(os.Stderr, "  scproxy --dns 192.168.1.100")
	fmt.Fprintln(os.Stderr, "\n  # HTTPS mode (auto-generate root CA, trust certs/ca.crt on client)")
	fmt.Fprintln(os.Stderr, "  scproxy --tls --tls-cert-dir ./certs")
	fmt.Fprintln(os.Stderr, "\n  # Clear cache")
	fmt.Fprintln(os.Stderr, "  scproxy --clear-cache --yes")
}

// printVersion displays version information
func printVersion() {
	fmt.Printf("%s %s\n", config.AppName, version.Get().String())
}

func printSummary() {
	configPath := *configFlag
	if configPath == "" {
		if _, err := os.Stat(LocalConfig); err == nil {
			configPath = LocalConfig
		} else if _, err := os.Stat(EtcConfig); err == nil {
			configPath = EtcConfig
		} else {
			configPath = LocalConfig
		}
	}

	appConfig, err := config.LoadAppConfig(configPath)
	if err != nil {
		if err := stats.PrintSummary(StatsFilePath); err != nil {
			fmt.Fprintf(os.Stderr, "No statistics found. Run scproxy with cache enabled first.\n")
			os.Exit(1)
		}
		return
	}

	if appConfig.Summary == nil {
		fmt.Fprintf(os.Stderr, "No statistics found. Run scproxy with cache enabled first.\n")
		os.Exit(1)
	}

	stats.PrintAllStats(appConfig.Summary)
}

func main() {
	pflag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	if *showSummary {
		printSummary()
		os.Exit(0)
	}

	flags := &config.CLIFlags{
		Target:            *targetFlag,
		Proxy:             *proxyFlag,
		Port:              *portFlag,
		Host:              *hostFlag,
		ConfigPath:        *configFlag,
		EnableCache:       *cacheFlag,
		LogLevel:          *logLevelFlag,
		LogOutput:         *logOutputFlag,
		ClearCache:        *clearCacheFlag,
		Yes:               *yesFlag,
		DNS:               *dnsFlag,
		TLSEnable:         *tlsEnableFlag,
		TLSDisable:        *tlsDisableFlag,
		TLSPort:           *tlsPortFlag,
		TLSCertDir:        *tlsCertDirFlag,
		TLSVerifyUpstream: *tlsVerifyUpstreamFlag,
		TLSNoRedirectHTTP: *tlsNoRedirectFlag,
	}

	if flags.ClearCache {
		if err := clearCache(flags); err != nil {
			log.Fatal(err)
		}
		return
	}

	cfg, configPath, err := config.New(flags)
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	if err := config.UpdateAndSaveAppConfig(configPath, flags, cfg); err != nil {
		fmt.Printf("Warning: failed to save config file: %v\n", err)
	}

	logger, err := logging.New(&cfg.Logging)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	logger.SetDefault()
	logger.Debug("Configuration loaded successfully", "config", cfg)
	logger.Info("Starting scproxy...", "routes", len(cfg.Routes))
	manager, err := scproxy.NewscproxyManager(cfg, logger, configPath)
	if err != nil {
		log.Fatalf("Failed to create proxy manager: %v", err)
	}

	if err := manager.Start(); err != nil {
		log.Fatalf("Failed to start servers: %v", err)
	}

	logger.Info("All servers started successfully")

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
	printSummary()

	if cerr := logger.Close(); cerr != nil {
		log.Fatalf("Failed to close log file (%s): %v", cfg.Logging.Path, cerr)
	}
}

func clearCache(flags *config.CLIFlags) error {
	configPath := flags.ConfigPath
	if configPath == "" {
		if _, err := os.Stat(LocalConfig); err == nil {
			configPath = LocalConfig
		} else if _, err := os.Stat(EtcConfig); err == nil {
			configPath = EtcConfig
		} else {
			configPath = LocalConfig
		}
	}

	appConfig, err := config.LoadAppConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	if !appConfig.Cache.Enabled {
		return fmt.Errorf("cache is not enabled in configuration")
	}

	fmt.Printf("Preparing to clear cache...\n")
	fmt.Printf("Cache directory: %s\n", appConfig.Cache.Directory)

	stats, err := cache.GetStats(appConfig.Cache.Directory)
	if err != nil {
		return fmt.Errorf("failed to get cache stats: %v", err)
	}

	fmt.Printf("Cache information:\n")
	fmt.Printf("  Total files: %d\n", stats.TotalFiles)
	fmt.Printf("  Total size: %.2f MB\n", stats.TotalSizeMB)

	if !flags.Yes {
		fmt.Printf("Are you sure you want to clear the cache? [y/N]: ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Cache clearing cancelled.")
			return nil
		}
	}

	fmt.Println("Clearing cache...")
	if err := cache.Clear(appConfig.Cache.Directory); err != nil {
		return fmt.Errorf("failed to clear cache: %v", err)
	}

	fmt.Println("Cache cleared successfully.")
	return nil
}

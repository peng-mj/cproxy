package config

// CLIFlags holds all command-line flag values.
// This struct replaces the *cli.Command parameter pattern from urfave/cli
// with a simple struct that pflag can populate.
type CLIFlags struct {
	Target      string
	Proxy       string
	Port        int
	Host        string
	ConfigPath  string
	EnableCache bool
	LogLevel    string
	LogOutput   string
	ClearCache  bool
	Yes         bool

	// DNS + VHost mode: proxy IP address, empty = use config file
	DNS string

	// TLS/HTTPS flags
	TLSEnable         bool
	TLSDisable        bool
	TLSPort           int
	TLSCertDir        string
	TLSVerifyUpstream bool
	TLSNoRedirectHTTP bool
}

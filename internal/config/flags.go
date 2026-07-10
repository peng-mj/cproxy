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

	// DNS proxy flags
	DNSEnable   bool
	DNSDisable  bool
	DNSAddr     string
	DNSUpstream string
	DNSProxyIP  string

	// VHost reverse proxy flags
	VHostEnable  bool
	VHostDisable bool
	VHostPort    int

	// TLS/HTTPS flags
	TLSEnable         bool
	TLSDisable        bool
	TLSPort           int
	TLSCertDir        string
	TLSVerifyUpstream bool
	TLSNoRedirectHTTP bool
}

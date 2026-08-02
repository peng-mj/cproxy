package install

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"
)

// dialTimeout is how long to wait when probing proxyIP reachability.
const dialTimeout = 2 * time.Second

// EnsureLoopback checks whether proxyIP is usable on the network. If it is
// neither assigned to a local interface nor reachable via TCP probe, the IP
// is added as a /32 alias on the loopback interface (lo) so that traffic to
// proxyIP is delivered locally where scproxy is listening.
//
// This makes gateway mode work out-of-the-box on hosts where proxyIP (e.g.
// 10.255.255.254) is not a real network address — common on standalone Linux
// machines outside WSL2.
func EnsureLoopback(proxyIP string) error {
	if proxyIP == "" {
		return nil
	}

	// Already on a local interface (lo, eth0, …)? Nothing to do.
	if isLocalIP(proxyIP) {
		return nil
	}

	// Reachable on the network? Nothing to do.
	if isReachable(proxyIP) {
		return nil
	}

	// Not reachable — add to lo so scproxy receives the traffic locally.
	if err := addLoopbackAlias(proxyIP); err != nil {
		return fmt.Errorf("add %s/32 to lo: %w", proxyIP, err)
	}
	fmt.Printf("    [loopback] %s was unreachable — added as alias on lo\n", proxyIP)
	return nil
}

// RemoveLoopback removes a /32 alias from lo that was previously added by
// EnsureLoopback. Best-effort: ignores errors if the address doesn't exist.
func RemoveLoopback(proxyIP string) error {
	if proxyIP == "" {
		return nil
	}
	if !commandExists("ip") {
		return nil
	}
	cmd := exec.Command("ip", "addr", "del", proxyIP+"/32", "dev", "lo")
	if out, err := cmd.CombinedOutput(); err != nil {
		// "Cannot assign requested address" / "File exists" are harmless.
		return fmt.Errorf("ip addr del %s/32 dev lo: %s: %w", proxyIP, string(out), err)
	}
	return nil
}

// isLocalIP reports whether ip is assigned to any local network interface.
func isLocalIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	ifaces, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, addr := range ifaces {
		if ipNet, ok := addr.(*net.IPNet); ok {
			if ipNet.IP.Equal(parsed) {
				return true
			}
		}
	}
	return false
}

// isReachable reports whether a TCP connection attempt to ip:80 gets any
// response (connection accepted OR refused). A timeout means the host is not
// reachable on the network.
func isReachable(ip string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), dialTimeout)
	if err != nil {
		// A timeout means no route to host; any other error (connection
		// refused, reset) means the host IS there — just nothing on :80.
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			return false
		}
		return true
	}
	conn.Close()
	return true
}

// addLoopbackAlias adds ip/32 to the loopback interface via `ip addr add`.
func addLoopbackAlias(ip string) error {
	if !commandExists("ip") {
		return fmt.Errorf("'ip' command not found (requires iproute2)")
	}
	cmd := exec.Command("ip", "addr", "add", ip+"/32", "dev", "lo")
	if out, err := cmd.CombinedOutput(); err != nil {
		// "File exists" is harmless — the alias was already added.
		if len(out) > 0 && strings.Contains(string(out), "File exists") {
			return nil
		}
		return fmt.Errorf("%s: %w", string(out), err)
	}
	return nil
}

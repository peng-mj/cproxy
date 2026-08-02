// Package install manages host-level DNS wiring so that gateway mode works
// out of the box on a single Linux host.
//
// On Linux (Debian/Ubuntu with systemd-resolved), scproxy's DNS server needs
// to own :53. By default systemd-resolved already binds :53 as a stub
// listener, so scproxy cannot start. --install frees :53, points the system
// resolver at 127.0.0.1 (so hijacked domains resolve to the proxy), and trusts
// the proxy's root CA (so HTTPS clients accept the on-the-fly certs).
// --uninstall reverses all of it, restoring the original files from backup.
package install

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/peng-mj/scproxy/internal/config"
)

var (
	resolvConfPath   = "/etc/resolv.conf"
	resolvConfBackup = "/etc/resolv.conf.scproxy.bak"

	resolvedConfPath   = "/etc/systemd/resolved.conf"
	resolvedConfBackup = "/etc/systemd/resolved.conf.scproxy.bak"

	caTrustPath = "/usr/local/share/ca-certificates/scproxy-root-ca.crt"
)

const (
	defaultProxyIP = "10.255.255.254"
)

var stubListenerLine = regexp.MustCompile(`(?m)^[\s#]*DNSStubListener\s*=`)

// Install wires the local system to use scproxy as its DNS resolver and trusts
// scproxy's root CA. Intended to be run with sudo before `scproxy --gateway`.
//
// configPath is used only to locate the TLS cert directory; pass "" to use the
// default ("./certs").
func Install(configPath string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("--install is only supported on Linux (detected %s); configure DNS manually", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		return fmt.Errorf("--install must be run as root (try: sudo scproxy --install)")
	}

	fmt.Println("==> Installing scproxy local DNS configuration...")

	// 1. Free port 53 from systemd-resolved so scproxy's DNS server can bind it.
	if err := freePort53(); err != nil {
		return fmt.Errorf("free port 53: %w", err)
	}

	// 2. Point the system resolver at scproxy.
	proxyIP := proxyIPFromConfig(configPath)
	if err := installResolvConf(proxyIP); err != nil {
		return fmt.Errorf("configure resolver: %w", err)
	}

	// 3. Trust scproxy's root CA so HTTPS clients accept proxy-signed certs.
	if err := installCA(certDirFromConfig(configPath)); err != nil {
		return fmt.Errorf("install CA: %w", err)
	}

	fmt.Println()
	fmt.Printf("==> Done. Local DNS now points at scproxy (%s).\n", proxyIP)
	fmt.Println("    Start the proxy with:  sudo scproxy --gateway -t https://<target> -P <port>")
	fmt.Println("    Remove with:          sudo scproxy --uninstall")
	return nil
}

// Uninstall reverses --install, restoring the original resolver and resolved
// configuration from the backups taken during install.
func Uninstall(configPath string) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("--uninstall is only supported on Linux (detected %s)", runtime.GOOS)
	}
	if os.Getuid() != 0 {
		return fmt.Errorf("--uninstall must be run as root (try: sudo scproxy --uninstall)")
	}

	fmt.Println("==> Uninstalling scproxy local DNS configuration...")

	// 1. Restore /etc/resolv.conf.
	if err := restoreFile(resolvConfBackup, resolvConfPath); err != nil {
		fmt.Fprintf(os.Stderr, "    [warn] could not restore %s: %v\n", resolvConfPath, err)
	} else {
		fmt.Printf("    restored %s\n", resolvConfPath)
	}

	// 2. Restore systemd-resolved config (or re-enable its stub listener).
	if err := restoreResolved(); err != nil {
		fmt.Fprintf(os.Stderr, "    [warn] could not restore systemd-resolved: %v\n", err)
	}

	// 3. Remove the trusted CA.
	if err := uninstallCA(); err != nil {
		fmt.Fprintf(os.Stderr, "    [warn] could not remove CA: %v\n", err)
	}

	// 4. Remove loopback alias if it was auto-added.
	proxyIP := proxyIPFromConfig(configPath)
	if err := RemoveLoopback(proxyIP); err != nil {
		fmt.Fprintf(os.Stderr, "    [warn] could not remove loopback alias %s: %v\n", proxyIP, err)
	}

	fmt.Println()
	fmt.Println("==> Done. Local DNS configuration removed.")
	return nil
}

// freePort53 disables systemd-resolved's stub listener (which binds :53 by
// default) so scproxy can bind :53 instead. No-op if systemd-resolved is not
// active or not present.
func freePort53() error {
	if !commandExists("systemctl") {
		fmt.Println("    [skip] systemctl not found; assuming :53 is free")
		return nil
	}
	if !systemdResolvedActive() {
		fmt.Println("    [skip] systemd-resolved not active; :53 should be free")
		return nil
	}

	// Back up the original resolved.conf once.
	if err := backupOnce(resolvedConfPath, resolvedConfBackup); err != nil {
		return fmt.Errorf("back up %s: %w", resolvedConfPath, err)
	}
	fmt.Printf("    backed up %s -> %s\n", resolvedConfPath, resolvedConfBackup)

	// Set DNSStubListener=no.
	if err := setStubListener(false); err != nil {
		return fmt.Errorf("disable stub listener: %w", err)
	}
	fmt.Println("    disabled systemd-resolved stub listener (DNSStubListener=no)")

	if err := run("systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("restart systemd-resolved: %w", err)
	}
	fmt.Println("    restarted systemd-resolved")
	return nil
}

// restoreResolved restores the original systemd-resolved config from backup, or
// re-enables the stub listener if no backup exists.
func restoreResolved() error {
	if !commandExists("systemctl") {
		return nil
	}
	if _, err := os.Stat(resolvedConfBackup); err == nil {
		if err := restoreFile(resolvedConfBackup, resolvedConfPath); err != nil {
			return err
		}
		fmt.Printf("    restored %s\n", resolvedConfPath)
	} else if systemdResolvedActive() {
		// No backup but service is active: flip the listener back on.
		if err := setStubListener(true); err != nil {
			return err
		}
		fmt.Println("    re-enabled systemd-resolved stub listener")
	}
	return run("systemctl", "restart", "systemd-resolved")
}

// installResolvConf backs up the current resolver config and writes one that
// points at the given proxy IP. Handles the WSL2 case where /etc/resolv.conf
// is a symlink by replacing it with a plain file.
func installResolvConf(nameserver string) error {
	if err := backupOnce(resolvConfPath, resolvConfBackup); err != nil {
		return fmt.Errorf("back up %s: %w", resolvConfPath, err)
	}
	fmt.Printf("    backed up %s -> %s\n", resolvConfPath, resolvConfBackup)

	content := fmt.Sprintf("# Generated by scproxy --install\nnameserver %s\n", nameserver)

	// Remove any existing file/symlink, then write a plain file so WSL/systemd
	// does not silently overwrite it.
	if err := os.Remove(resolvConfPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", resolvConfPath, err)
	}
	if err := os.WriteFile(resolvConfPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write %s: %w", resolvConfPath, err)
	}
	fmt.Printf("    wrote %s (nameserver %s)\n", resolvConfPath, nameserver)
	return nil
}

// installCA copies scproxy's root CA into the system trust store. Best-effort:
// if the CA has not been generated yet (scproxy never ran), it skips with a
// hint rather than failing.
func installCA(certDir string) error {
	caPath := filepath.Join(certDir, "ca.crt")
	data, err := os.ReadFile(caPath)
	if err != nil {
		fmt.Printf("    [skip] CA not found at %s — run `scproxy --gateway` once to generate it, then re-run --install\n", caPath)
		return nil
	}
	if err := os.WriteFile(caTrustPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", caTrustPath, err)
	}
	fmt.Printf("    installed CA -> %s\n", caTrustPath)

	if commandExists("update-ca-certificates") {
		if err := run("update-ca-certificates"); err != nil {
			return fmt.Errorf("update-ca-certificates: %w", err)
		}
		fmt.Println("    updated CA bundle (update-ca-certificates)")
	} else {
		fmt.Printf("    [skip] update-ca-certificates not found; trust %s manually\n", caTrustPath)
	}
	return nil
}

// uninstallCA removes the proxy CA from the trust store.
func uninstallCA() error {
	if _, err := os.Stat(caTrustPath); err != nil {
		return nil // nothing to remove
	}
	if err := os.Remove(caTrustPath); err != nil {
		return err
	}
	fmt.Printf("    removed %s\n", caTrustPath)
	if commandExists("update-ca-certificates") {
		if err := run("update-ca-certificates"); err != nil {
			return err
		}
		fmt.Println("    updated CA bundle (update-ca-certificates)")
	}
	return nil
}

// setStubListener rewrites resolved.conf so DNSStubListener is enabled/disabled.
// Preserves all other content; adds the directive if absent.
func setStubListener(enable bool) error {
	value := "no"
	if enable {
		value = "yes"
	}

	data, err := os.ReadFile(resolvedConfPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(resolvedConfPath, []byte("DNSStubListener="+value+"\n"), 0644)
		}
		return err
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		if stubListenerLine.MatchString(line) {
			lines[i] = "DNSStubListener=" + value
			found = true
		}
	}
	if !found {
		lines = append(lines, "DNSStubListener="+value)
	}
	return os.WriteFile(resolvedConfPath, []byte(strings.Join(lines, "\n")), 0644)
}

// backupOnce copies src to bak only if bak does not already exist, so repeated
// installs never overwrite the pristine original. src is read through any
// symlink (e.g. WSL2's /etc/resolv.conf).
func backupOnce(src, bak string) error {
	if _, err := os.Stat(bak); err == nil {
		return nil // backup already exists
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(bak, data, 0644)
}

// restoreFile moves bak to dst, replacing dst (which may be a symlink) with a
// plain file containing the backup content.
func restoreFile(bak, dst string) error {
	data, err := os.ReadFile(bak)
	if err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func systemdResolvedActive() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", "systemd-resolved")
	return cmd.Run() == nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// proxyIPFromConfig reads the DNS proxy IP from the config file, falling back
// to the WSL2 host gateway (10.255.255.254) if the file is missing or unset.
func proxyIPFromConfig(configPath string) string {
	if configPath == "" {
		return defaultProxyIP
	}
	if app, err := config.LoadAppConfig(configPath); err == nil && app.DNS.ProxyIP != "" {
		return app.DNS.ProxyIP
	}
	return defaultProxyIP
}

// certDirFromConfig reads the TLS cert directory from the config file, falling
// back to the default ("./certs") if the file is missing or unset.
func certDirFromConfig(configPath string) string {
	if configPath == "" {
		return "./certs"
	}
	if app, err := config.LoadAppConfig(configPath); err == nil && app.TLS.CertDir != "" {
		return app.TLS.CertDir
	}
	return "./certs"
}

package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupOnce(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src.txt")
	bak := filepath.Join(tmp, "src.bak")

	// Create source file
	if err := os.WriteFile(src, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}

	// First backup should succeed
	if err := backupOnce(src, bak); err != nil {
		t.Fatalf("backupOnce: %v", err)
	}
	data, _ := os.ReadFile(bak)
	if string(data) != "original" {
		t.Fatalf("backup content = %q, want %q", data, "original")
	}

	// Modify source and backup again — should NOT overwrite
	os.WriteFile(src, []byte("modified"), 0644)
	backupOnce(src, bak)
	data, _ = os.ReadFile(bak)
	if string(data) != "original" {
		t.Fatalf("backup was overwritten: got %q, want %q", data, "original")
	}
}

func TestRestoreFile(t *testing.T) {
	tmp := t.TempDir()
	bak := filepath.Join(tmp, "bak.txt")
	dst := filepath.Join(tmp, "dst.txt")

	os.WriteFile(bak, []byte("restored content"), 0644)
	os.WriteFile(dst, []byte("current"), 0644)

	if err := restoreFile(bak, dst); err != nil {
		t.Fatalf("restoreFile: %v", err)
	}
	data, _ := os.ReadFile(dst)
	if string(data) != "restored content" {
		t.Fatalf("restore content = %q, want %q", data, "restored content")
	}
}

func TestRestoreFileOverwritesSymlink(t *testing.T) {
	tmp := t.TempDir()
	bak := filepath.Join(tmp, "bak.txt")
	target := filepath.Join(tmp, "target.txt")
	link := filepath.Join(tmp, "link.txt")

	os.WriteFile(bak, []byte("from backup"), 0644)
	os.WriteFile(target, []byte("symlink target"), 0644)
	os.Symlink(target, link)

	// restoreFile should replace the symlink with a plain file
	if err := restoreFile(bak, link); err != nil {
		t.Fatalf("restoreFile: %v", err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dst is still a symlink after restore")
	}
	data, _ := os.ReadFile(link)
	if string(data) != "from backup" {
		t.Fatalf("restore content = %q, want %q", data, "from backup")
	}
}

func TestSetStubListenerDisable(t *testing.T) {
	tmp := t.TempDir()
	origPath := resolvedConfPath
	resolvedConfPath = filepath.Join(tmp, "resolved.conf")
	defer func() { resolvedConfPath = origPath }()

	// Write a config with stub listener commented out
	content := "[Resolve]\n#DNSStubListener=yes\nDNS=8.8.8.8\n"
	os.WriteFile(resolvedConfPath, []byte(content), 0644)

	if err := setStubListener(false); err != nil {
		t.Fatalf("setStubListener: %v", err)
	}

	data, _ := os.ReadFile(resolvedConfPath)
	s := string(data)
	if !contains(s, "DNSStubListener=no") {
		t.Fatalf("expected DNSStubListener=no, got:\n%s", s)
	}
	if contains(s, "DNSStubListener=yes") {
		t.Fatalf("old yes line should be gone, got:\n%s", s)
	}
	// Other content preserved
	if !contains(s, "DNS=8.8.8.8") {
		t.Fatalf("DNS line should be preserved, got:\n%s", s)
	}
}

func TestSetStubListenerEnable(t *testing.T) {
	tmp := t.TempDir()
	origPath := resolvedConfPath
	resolvedConfPath = filepath.Join(tmp, "resolved.conf")
	defer func() { resolvedConfPath = origPath }()

	content := "[Resolve]\nDNSStubListener=no\n"
	os.WriteFile(resolvedConfPath, []byte(content), 0644)

	if err := setStubListener(true); err != nil {
		t.Fatalf("setStubListener: %v", err)
	}

	data, _ := os.ReadFile(resolvedConfPath)
	s := string(data)
	if !contains(s, "DNSStubListener=yes") {
		t.Fatalf("expected DNSStubListener=yes, got:\n%s", s)
	}
}

func TestSetStubListenerNoExistingDirective(t *testing.T) {
	tmp := t.TempDir()
	origPath := resolvedConfPath
	resolvedConfPath = filepath.Join(tmp, "resolved.conf")
	defer func() { resolvedConfPath = origPath }()

	content := "[Resolve]\nDNS=8.8.8.8\n"
	os.WriteFile(resolvedConfPath, []byte(content), 0644)

	if err := setStubListener(false); err != nil {
		t.Fatalf("setStubListener: %v", err)
	}

	data, _ := os.ReadFile(resolvedConfPath)
	s := string(data)
	if !contains(s, "DNSStubListener=no") {
		t.Fatalf("expected DNSStubListener=no appended, got:\n%s", s)
	}
}

func TestSetStubListenerNonExistentFile(t *testing.T) {
	tmp := t.TempDir()
	origPath := resolvedConfPath
	resolvedConfPath = filepath.Join(tmp, "resolved.conf")
	defer func() { resolvedConfPath = origPath }()

	// File doesn't exist — should create it
	if err := setStubListener(false); err != nil {
		t.Fatalf("setStubListener on missing file: %v", err)
	}
	data, _ := os.ReadFile(resolvedConfPath)
	if string(data) != "DNSStubListener=no\n" {
		t.Fatalf("got %q", data)
	}
}

func TestCertDirFromConfig(t *testing.T) {
	// Empty path → default
	if d := certDirFromConfig(""); d != "./certs" {
		t.Fatalf("empty path: got %q, want ./certs", d)
	}
	// Non-existent file → default
	if d := certDirFromConfig("/nonexistent/path.json"); d != "./certs" {
		t.Fatalf("nonexistent: got %q, want ./certs", d)
	}
}

func TestProxyIPFromConfig(t *testing.T) {
	// Empty path → default
	if ip := proxyIPFromConfig(""); ip != defaultProxyIP {
		t.Fatalf("empty path: got %q, want %q", ip, defaultProxyIP)
	}
	// Non-existent file → default
	if ip := proxyIPFromConfig("/nonexistent/path.json"); ip != defaultProxyIP {
		t.Fatalf("nonexistent: got %q, want %q", ip, defaultProxyIP)
	}
}

func TestProxyIPFromConfigWithConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	customIP := "192.168.1.100"
	cfgContent := `{"dns": {"proxyIP": "` + customIP + `"}}`
	os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	if ip := proxyIPFromConfig(cfgPath); ip != customIP {
		t.Fatalf("got %q, want %q", ip, customIP)
	}
}

func TestCertDirFromConfigWithConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	customDir := filepath.Join(tmp, "mycerts")
	cfgContent := `{"tls": {"certDir": "` + customDir + `"}}`
	os.WriteFile(cfgPath, []byte(cfgContent), 0644)

	if d := certDirFromConfig(cfgPath); d != customDir {
		t.Fatalf("got %q, want %q", d, customDir)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

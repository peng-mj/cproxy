package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func mustHashCacheKey(t *testing.T, key CacheKey) string {
	t.Helper()
	hash, err := hashCacheKey(key)
	if err != nil {
		t.Fatalf("hashCacheKey failed: %v", err)
	}
	return hash
}

func TestPurgeLegacyDirectoryEntries(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}
	defer s.Close()

	body := []byte("legacy content")
	resp := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       body,
	}

	// Legacy entry for "/foo/": keyed by the raw directory path, data file
	// stored (by the legacy scheme) at foo/index.html.
	legacyHash := mustHashCacheKey(t, CacheKey{Method: "GET", Host: "example.com", Path: "/foo/"})
	if err := s.Put(legacyHash, "example.com", "/foo/", resp); err != nil {
		t.Fatalf("Put legacy /foo/ failed: %v", err)
	}

	// Legacy entry for "/": data file stored (by the legacy scheme) as a
	// literal "root" file.
	rootHash := mustHashCacheKey(t, CacheKey{Method: "GET", Host: "example.com", Path: "/"})
	if err := s.Put(rootHash, "example.com", "/", resp); err != nil {
		t.Fatalf("Put legacy / failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "example.com", "root"), body, 0644); err != nil {
		t.Fatalf("failed to write legacy root file: %v", err)
	}

	// Live entry with interior double slashes (no trailing slash after
	// collapsing): still reachable via the normalized key, must survive.
	liveHash := mustHashCacheKey(t, CacheKey{Method: "GET", Host: "example.com", Path: "/a/b"})
	if err := s.Put(liveHash, "example.com", "/a//b", resp); err != nil {
		t.Fatalf("Put live /a//b failed: %v", err)
	}

	s.purgeLegacyDirectoryEntries()

	if _, ok := s.index.Get(legacyHash); ok {
		t.Error("legacy /foo/ entry should be purged")
	}
	if _, ok := s.index.Get(rootHash); ok {
		t.Error("legacy / entry should be purged")
	}
	if _, ok := s.index.Get(liveHash); !ok {
		t.Error("live /a//b entry should survive")
	}

	if _, err := os.Stat(filepath.Join(dir, "data", "example.com", "foo", "index.html")); !os.IsNotExist(err) {
		t.Error("legacy /foo/ data file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "example.com", "root")); !os.IsNotExist(err) {
		t.Error("legacy root data file should be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "example.com", "a", "b")); err != nil {
		t.Error("live /a//b data file should survive")
	}
}

func TestNewStorage_PurgesLegacyEntriesOnStartup(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	body := []byte("legacy content")
	resp := &CachedResponse{
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		Body:       body,
	}
	legacyHash := mustHashCacheKey(t, CacheKey{Method: "GET", Host: "example.com", Path: "/foo/"})
	if err := s.Put(legacyHash, "example.com", "/foo/", resp); err != nil {
		t.Fatalf("Put legacy /foo/ failed: %v", err)
	}
	s.Close()

	// Reopening the storage must purge the legacy entry automatically.
	s2, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage reopen failed: %v", err)
	}
	defer s2.Close()

	if _, ok := s2.index.Get(legacyHash); ok {
		t.Error("legacy /foo/ entry should be purged on startup")
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "example.com", "foo", "index.html")); !os.IsNotExist(err) {
		t.Error("legacy /foo/ data file should be removed on startup")
	}
}

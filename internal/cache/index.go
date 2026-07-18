package cache

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	dbFileName    = "cache_index.db"
	entriesBucket = "entries"
)

// CacheIndex maintains the mapping between cache keys and cached files.
type CacheIndex struct {
	db *bolt.DB
}

// IndexEntry represents a single cache entry in the index.
type IndexEntry struct {
	Hash     string           `json:"hash"`
	Host     string           `json:"host"`
	URLPath  string           `json:"urlPath"`
	FilePath string           `json:"filePath"`
	Created  int64            `json:"created"`
	Accessed int64            `json:"accessed"`
	Size     int64            `json:"size"`
	CRC32    uint32           `json:"crc32"`
	Expires  int64            `json:"expires"`
	Metadata ResponseMetadata `json:"metadata"`
}

// ResponseMetadata contains HTTP response metadata.
type ResponseMetadata struct {
	StatusCode int               `json:"statusCode"`
	Headers    map[string]string `json:"headers"`
}

func init() {
	gob.Register(IndexEntry{})
	gob.Register(ResponseMetadata{})
}

// NewCacheIndex creates a new cache index backed by bbolt.
func NewCacheIndex(cacheDir string) (*CacheIndex, error) {
	dbPath := filepath.Join(cacheDir, dbFileName)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(entriesBucket))
		return err
	}); err != nil {
		// Best effort cleanup - log error but don't override original error
		if closeErr := db.Close(); closeErr != nil {
			slog.Warn("failed to close database during cleanup", "error", closeErr)
		}
		return nil, fmt.Errorf("failed to create bucket: %v", err)
	}

	return &CacheIndex{db: db}, nil
}

// Close closes the underlying bbolt database.
func (idx *CacheIndex) Close() error {
	if idx.db != nil {
		return idx.db.Close()
	}
	return nil
}

// Load is a no-op for bbolt (data is loaded on demand).
func (idx *CacheIndex) Load() error {
	return nil
}

// Save is a no-op for bbolt (data is persisted on each write).
func (idx *CacheIndex) Save() error {
	return nil
}

// Add adds a new entry to the index.
func (idx *CacheIndex) Add(hash string, host string, urlPath string, filePath string, statusCode int, headers map[string]string, size int64, crc32 uint32) error {
	now := time.Now().Unix()

	entry := &IndexEntry{
		Hash:     hash,
		Host:     host,
		URLPath:  urlPath,
		FilePath: filePath,
		Created:  now,
		Accessed: now,
		Size:     size,
		CRC32:    crc32,
		Expires:  0,
		Metadata: ResponseMetadata{
			StatusCode: statusCode,
			Headers:    headers,
		},
	}

	data, err := encodeEntry(entry)
	if err != nil {
		return fmt.Errorf("failed to encode entry: %v", err)
	}

	return idx.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		return bucket.Put([]byte(hash), data)
	})
}

// Get retrieves an entry from the index and updates its access time.
func (idx *CacheIndex) Get(hash string) (*IndexEntry, bool) {
	var entry *IndexEntry
	var found bool

	// Try to update access time - log errors but don't fail the Get operation
	if updateErr := idx.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		data := bucket.Get([]byte(hash))
		if data == nil {
			return nil
		}

		decoded, decErr := decodeEntry(data)
		if decErr != nil {
			// Delete corrupted entry - log error
			if delErr := bucket.Delete([]byte(hash)); delErr != nil {
				slog.Warn("failed to delete corrupted entry %s", "error", hash, delErr)
			}
			return nil
		}

		found = true
		decoded.Accessed = time.Now().Unix()

		encoded, encErr := encodeEntry(decoded)
		if encErr != nil {
			entry = decoded
			slog.Warn("failed to encode entry %s for access time update", "error", hash, encErr)
			return nil
		}

		if putErr := bucket.Put([]byte(hash), encoded); putErr != nil {
			entry = decoded
			slog.Warn("failed to update access time for entry %s", "error", hash, putErr)
			return nil
		}

		entry = decoded
		return nil
	}); updateErr != nil {
		slog.Warn("failed to update access time for entry %s", "error", hash, updateErr)
	}

	return entry, found
}

// Delete removes an entry from the index.
func (idx *CacheIndex) Delete(hash string) error {
	return idx.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		return bucket.Delete([]byte(hash))
	})
}

// Validate validates all entries in the index against actual files.
func (idx *CacheIndex) Validate(cacheDir string) ([]string, error) {
	var invalidHashes []string
	dataDir := filepath.Join(cacheDir, "data")

	err := idx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			entry, decErr := decodeEntry(v)
			if decErr != nil {
				invalidHashes = append(invalidHashes, string(k))
				continue
			}

			filePath := filepath.Join(dataDir, entry.FilePath)
			if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
				invalidHashes = append(invalidHashes, string(k))
				continue
			}
			if _, statErr := os.Stat(filePath); statErr != nil {
				invalidHashes = append(invalidHashes, string(k))
				continue
			}
			// Validate CRC32 instead of size
			crc, crcErr := calculateFileCRC32(filePath)
			if crcErr != nil || crc != entry.CRC32 {
				invalidHashes = append(invalidHashes, string(k))
			}
		}
		return nil
	})

	return invalidHashes, err
}

// Cleanup removes invalid entries from the index.
func (idx *CacheIndex) Cleanup(invalidHashes []string) error {
	return idx.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		for _, hash := range invalidHashes {
			if err := bucket.Delete([]byte(hash)); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetAll returns all entries in the index.
func (idx *CacheIndex) GetAll() map[string]*IndexEntry {
	result := make(map[string]*IndexEntry)

	// Read all entries - log error but return what we have
	if viewErr := idx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			entry, err := decodeEntry(v)
			if err != nil {
				continue
			}
			result[string(k)] = entry
		}
		return nil
	}); viewErr != nil {
		slog.Warn("failed to read all cache entries", "error", viewErr)
	}

	return result
}

// Count returns the number of entries in the index.
func (idx *CacheIndex) Count() int {
	var count int
	idx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		c := bucket.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			count++
		}
		return nil
	})
	return count
}

// TotalSize returns the total size of all cached files.
func (idx *CacheIndex) TotalSize() int64 {
	var total int64
	idx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			entry, err := decodeEntry(v)
			if err != nil {
				continue
			}
			total += entry.Size
		}
		return nil
	})
	return total
}

// TotalSizeForHost returns the total size of cached files for a specific host.
func (idx *CacheIndex) TotalSizeForHost(host string) int64 {
	var total int64
	idx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			entry, err := decodeEntry(v)
			if err != nil {
				continue
			}
			if entry.Host == host {
				total += entry.Size
			}
		}
		return nil
	})
	return total
}

// GetLRUEntry returns the least recently used entry.
func (idx *CacheIndex) GetLRUEntry() (*IndexEntry, bool) {
	var oldest *IndexEntry

	idx.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(entriesBucket))
		c := bucket.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			entry, err := decodeEntry(v)
			if err != nil {
				continue
			}
			if oldest == nil || entry.Accessed < oldest.Accessed {
				oldest = entry
			}
		}
		return nil
	})

	if oldest == nil {
		return nil, false
	}
	return oldest, true
}

func encodeEntry(entry *IndexEntry) ([]byte, error) {
	var buf bytes.Buffer
	err := gob.NewEncoder(&buf).Encode(entry)
	return buf.Bytes(), err
}

func decodeEntry(data []byte) (*IndexEntry, error) {
	var entry IndexEntry
	err := gob.NewDecoder(bytes.NewReader(data)).Decode(&entry)
	return &entry, err
}

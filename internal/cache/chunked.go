package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// ChunkSize is the size of each cache chunk (1MB)
	ChunkSize = 1024 * 1024
	// MetaFileName is the name of the chunk metadata file
	MetaFileName = "_chunks.json"
)

// ChunkMetadata stores metadata for chunked files.
type ChunkMetadata struct {
	TotalSize    int64    `json:"totalSize"`    // Total file size
	ChunkCount   int      `json:"chunkCount"`   // Number of chunks
	ETag         string   `json:"etag"`         // ETag header
	LastModified string   `json:"lastModified"` // Last-Modified header
	ContentType  string   `json:"contentType"`  // Content-Type header
	ChunkHashes  []string `json:"chunkHashes"`  // SHA256 hash of each chunk
	Created      int64    `json:"created"`      // Creation timestamp
	Accessed     int64    `json:"accessed"`     // Last access timestamp
}

// ChunkedCache handles chunked file caching.
type ChunkedCache struct {
	cacheDir string
	mu       sync.RWMutex
	metadata map[string]*ChunkMetadata // hash -> chunk metadata
}

// NewChunkedCache creates a new chunked cache.
func NewChunkedCache(cacheDir string) (*ChunkedCache, error) {
	cc := &ChunkedCache{
		cacheDir: cacheDir,
		metadata: make(map[string]*ChunkMetadata),
	}

	// Load existing metadata
	if err := cc.loadMetadata(); err != nil {
		return nil, fmt.Errorf("failed to load chunk metadata: %v", err)
	}

	return cc, nil
}

// loadMetadata loads chunk metadata from disk.
func (cc *ChunkedCache) loadMetadata() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	metaPath := filepath.Join(cc.cacheDir, dataDir)

	err := filepath.Walk(metaPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-meta files
		if info.IsDir() || !strings.HasSuffix(path, MetaFileName) {
			return nil
		}

		// Read metadata file
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		var metadata ChunkMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			return err
		}

		// Extract hash from file path (parent directory name)
		hash := filepath.Base(filepath.Dir(path))
		cc.metadata[hash] = &metadata

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// GetChunk returns a specific chunk of a file.
func (cc *ChunkedCache) GetChunk(hash string, chunkIndex int) ([]byte, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	meta, exists := cc.metadata[hash]
	if !exists {
		return nil, fmt.Errorf("chunk metadata not found")
	}

	if chunkIndex < 0 || chunkIndex >= meta.ChunkCount {
		return nil, fmt.Errorf("invalid chunk index: %d", chunkIndex)
	}

	// Read chunk file
	chunkPath := cc.getChunkPath(hash, chunkIndex)
	data, err := os.ReadFile(chunkPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read chunk: %v", err)
	}

	// Verify chunk hash
	if len(meta.ChunkHashes) > chunkIndex {
		hasher := sha256.New()
		hasher.Write(data)
		chunkHash := hex.EncodeToString(hasher.Sum(nil))
		if chunkHash != meta.ChunkHashes[chunkIndex] {
			return nil, fmt.Errorf("chunk hash mismatch: expected %s, got %s", meta.ChunkHashes[chunkIndex], chunkHash)
		}
	}

	// Update access time
	meta.Accessed = time.Now().Unix()
	cc.saveMetadata(hash, meta)

	return data, nil
}

// PutChunk stores a chunk of a file.
func (cc *ChunkedCache) PutChunk(hash string, chunkIndex int, data []byte) (string, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	// Calculate chunk hash
	hasher := sha256.New()
	hasher.Write(data)
	chunkHash := hex.EncodeToString(hasher.Sum(nil))

	// Ensure metadata exists
	meta, exists := cc.metadata[hash]
	if !exists {
		meta = &ChunkMetadata{
			Created: time.Now().Unix(),
		}
		cc.metadata[hash] = meta
	}

	// Update metadata
	if chunkIndex >= len(meta.ChunkHashes) {
		// Extend slice if needed
		newHashes := make([]string, chunkIndex+1)
		copy(newHashes, meta.ChunkHashes)
		meta.ChunkHashes = newHashes
	}
	meta.ChunkHashes[chunkIndex] = chunkHash
	meta.Accessed = time.Now().Unix()

	// Create chunk directory
	chunkPath := cc.getChunkPath(hash, chunkIndex)
	if err := os.MkdirAll(filepath.Dir(chunkPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create chunk directory: %v", err)
	}

	// Write chunk data
	if err := os.WriteFile(chunkPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write chunk: %v", err)
	}

	// Save metadata
	if err := cc.saveMetadata(hash, meta); err != nil {
		os.Remove(chunkPath)
		return "", fmt.Errorf("failed to save metadata: %v", err)
	}

	return chunkHash, nil
}

// SetMetadata sets the metadata for a chunked file.
func (cc *ChunkedCache) SetMetadata(hash string, meta *ChunkMetadata) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	meta.Created = time.Now().Unix()
	meta.Accessed = time.Now().Unix()
	cc.metadata[hash] = meta

	return cc.saveMetadata(hash, meta)
}

// GetMetadata retrieves metadata for a chunked file.
func (cc *ChunkedCache) GetMetadata(hash string) (*ChunkMetadata, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	meta, exists := cc.metadata[hash]
	if !exists {
		return nil, false
	}

	// Update access time
	meta.Accessed = time.Now().Unix()

	return meta, true
}

// Delete removes all chunks for a file.
func (cc *ChunkedCache) Delete(hash string) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	meta, exists := cc.metadata[hash]
	if !exists {
		return nil // Nothing to delete
	}

	// Remove all chunk files
	for i := 0; i < meta.ChunkCount; i++ {
		chunkPath := cc.getChunkPath(hash, i)
		if err := os.Remove(chunkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove chunk %d: %v", i, err)
		}
	}

	// Remove metadata file
	metaPath := cc.getChunkMetadataPath(hash)
	if err := os.Remove(metaPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove metadata: %v", err)
	}

	// Remove from memory
	delete(cc.metadata, hash)

	return nil
}

// Validate validates all chunks for a file.
func (cc *ChunkedCache) Validate(hash string) error {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	meta, exists := cc.metadata[hash]
	if !exists {
		return fmt.Errorf("metadata not found")
	}

	// Validate each chunk
	for i := 0; i < meta.ChunkCount; i++ {
		chunkPath := cc.getChunkPath(hash, i)
		data, err := os.ReadFile(chunkPath)
		if err != nil {
			return fmt.Errorf("chunk %d missing or corrupted: %v", i, err)
		}

		// Verify hash
		hasher := sha256.New()
		hasher.Write(data)
		chunkHash := hex.EncodeToString(hasher.Sum(nil))
		if chunkHash != meta.ChunkHashes[i] {
			return fmt.Errorf("chunk %d hash mismatch: expected %s, got %s", i, meta.ChunkHashes[i], chunkHash)
		}
	}

	return nil
}

// getChunkPath returns the path for a specific chunk.
func (cc *ChunkedCache) getChunkPath(hash string, chunkIndex int) string {
	return filepath.Join(cc.cacheDir, dataDir, hash, fmt.Sprintf("chunk_%05d", chunkIndex))
}

// getChunkMetadataPath returns the path for chunk metadata.
func (cc *ChunkedCache) getChunkMetadataPath(hash string) string {
	return filepath.Join(cc.cacheDir, dataDir, hash, MetaFileName)
}

// saveMetadata saves chunk metadata to disk.
func (cc *ChunkedCache) saveMetadata(hash string, meta *ChunkMetadata) error {
	metaPath := cc.getChunkMetadataPath(hash)

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(metaPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
}

// Cleanup removes invalid chunks and metadata.
func (cc *ChunkedCache) Cleanup() error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	var invalidHashes []string

	for hash, meta := range cc.metadata {
		// Validate all chunks
		valid := true
		for i := 0; i < meta.ChunkCount; i++ {
			chunkPath := cc.getChunkPath(hash, i)
			if _, err := os.Stat(chunkPath); os.IsNotExist(err) {
				valid = false
				break
			}
		}

		if !valid {
			invalidHashes = append(invalidHashes, hash)
		}
	}

	// Remove invalid entries
	for _, hash := range invalidHashes {
		meta := cc.metadata[hash]
		// Remove chunk files
		for i := 0; i < meta.ChunkCount; i++ {
			chunkPath := cc.getChunkPath(hash, i)
			os.Remove(chunkPath)
		}
		// Remove metadata
		metaPath := cc.getChunkMetadataPath(hash)
		os.Remove(metaPath)
		// Remove from memory
		delete(cc.metadata, hash)
	}

	return nil
}

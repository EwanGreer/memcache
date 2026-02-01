package memcache

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SyncStrategy determines when cache data is persisted to disk.
type SyncStrategy int

const (
	// SyncDefault uses SyncPeriodic when FilePath is set.
	SyncDefault SyncStrategy = iota
	// SyncNone disables automatic syncing. Use Flush() manually.
	SyncNone
	// SyncImmediate writes to disk on every mutation (debounced).
	SyncImmediate
	// SyncPeriodic writes to disk at regular intervals.
	SyncPeriodic
)

const (
	fileVersion     = 1
	defaultInterval = 30 * time.Second
)

// fileHeader contains metadata about the cache file.
type fileHeader struct {
	Version   uint32
	CreatedAt time.Time
	ItemCount int
}

// fileItem represents a single cache entry for serialization.
type fileItem struct {
	Key       string
	Value     []byte
	ExpiresAt time.Time
}

// loadFromFile reads cache data from disk, filtering out expired items.
func loadFromFile(path string) (map[string]*fileItem, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := gob.NewDecoder(f)

	var header fileHeader
	if err := dec.Decode(&header); err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	if header.Version != fileVersion {
		return nil, fmt.Errorf("unsupported file version: %d (expected %d)", header.Version, fileVersion)
	}

	items := make(map[string]*fileItem, header.ItemCount)
	now := time.Now()

	for i := range header.ItemCount {
		var it fileItem
		if err := dec.Decode(&it); err != nil {
			return nil, fmt.Errorf("decode item %d: %w", i, err)
		}

		// Skip expired items
		if !it.ExpiresAt.IsZero() && now.After(it.ExpiresAt) {
			continue
		}

		items[it.Key] = &it
	}

	return items, nil
}

// saveToFile writes cache data to disk using atomic write (temp file + rename).
func saveToFile(path string, items []*fileItem) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write to temp file first
	tmp, err := os.CreateTemp(dir, ".memcache-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	// Set restrictive permissions (owner read/write only)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("set file permissions: %w", err)
	}

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	enc := gob.NewEncoder(tmp)

	header := fileHeader{
		Version:   fileVersion,
		CreatedAt: time.Now(),
		ItemCount: len(items),
	}

	if err := enc.Encode(&header); err != nil {
		return fmt.Errorf("encode header: %w", err)
	}

	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return fmt.Errorf("encode item: %w", err)
		}
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	success = true
	return nil
}

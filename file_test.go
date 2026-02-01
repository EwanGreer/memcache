package memcache

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFlushAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	// Create cache with persistence
	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	c.Set("key1", []byte("value1"))
	c.SetWithTTL("key2", []byte("value2"), time.Hour)

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("cache file was not created")
	}

	// Create new cache and load
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify data
	if v := c2.Get("key1"); !bytes.Equal(v, []byte("value1")) {
		t.Errorf("key1 = %q, want %q", v, "value1")
	}
	if v := c2.Get("key2"); !bytes.Equal(v, []byte("value2")) {
		t.Errorf("key2 = %q, want %q", v, "value2")
	}
}

func TestLoadOnInit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	// Create and populate cache
	c1 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	c1.Set("preloaded", []byte("data"))
	if err := c1.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Create new cache with default LoadOnInit (should load automatically)
	c2 := NewWithOptions(Options{
		FilePath:     path,
		SyncStrategy: SyncNone,
	})

	if v := c2.Get("preloaded"); !bytes.Equal(v, []byte("data")) {
		t.Errorf("preloaded = %q, want %q (LoadOnInit should load automatically)", v, "data")
	}
}

func TestExpiredItemsFilteredOnLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	c.Set("permanent", []byte("stays"))
	c.SetWithTTL("temporary", []byte("expires"), 50*time.Millisecond)

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Load into new cache
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if v := c2.Get("permanent"); !bytes.Equal(v, []byte("stays")) {
		t.Errorf("permanent = %q, want %q", v, "stays")
	}
	if v := c2.Get("temporary"); v != nil {
		t.Errorf("temporary = %q, want nil (should be filtered on load)", v)
	}
}

func TestSyncImmediate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncImmediate,
		SkipLoadOnInit: true,
	})

	c.Set("key", []byte("value"))

	// Give time for async flush
	time.Sleep(50 * time.Millisecond)

	// Verify file exists and has data
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if v := c2.Get("key"); !bytes.Equal(v, []byte("value")) {
		t.Errorf("key = %q, want %q (SyncImmediate should have flushed)", v, "value")
	}
}

func TestSyncPeriodic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncPeriodic,
		SyncInterval:   50 * time.Millisecond,
		SkipLoadOnInit: true,
	})
	defer c.Close()

	c.Set("key", []byte("value"))

	// Wait for periodic sync
	time.Sleep(100 * time.Millisecond)

	// Verify data was persisted
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if v := c2.Get("key"); !bytes.Equal(v, []byte("value")) {
		t.Errorf("key = %q, want %q (SyncPeriodic should have flushed)", v, "value")
	}
}

func TestConcurrentFlushProtection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	// Add data
	for i := range 100 {
		c.Set(string(rune('a'+i)), []byte("value"))
	}

	// Concurrent flushes should not panic or cause issues
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Flush()
		}()
	}
	wg.Wait()

	// Verify data is still loadable
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load after concurrent flushes failed: %v", err)
	}
}

func TestCloseStopsSyncAndFlushes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncPeriodic,
		SyncInterval:   time.Hour, // Long interval - shouldn't auto-flush
		SkipLoadOnInit: true,
	})

	c.Set("key", []byte("value"))

	// Close should flush
	if err := c.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify data was flushed
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if v := c2.Get("key"); !bytes.Equal(v, []byte("value")) {
		t.Errorf("key = %q, want %q (Close should have flushed)", v, "value")
	}
}

func TestLoadNonExistentFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	// Load should not error for non-existent file
	if err := c.Load(); err != nil {
		t.Errorf("Load on non-existent file should not error, got: %v", err)
	}

	// Cache should be empty
	if c.Len() != 0 {
		t.Errorf("Len = %d, want 0", c.Len())
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupted.db")

	// Write garbage to file
	if err := os.WriteFile(path, []byte("not a valid gob file"), 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	// Load should error for corrupted file
	if err := c.Load(); err == nil {
		t.Error("Load on corrupted file should error")
	}
}

func TestLoadVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "oldversion.db")

	// Write a file with wrong version
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	enc := gob.NewEncoder(f)
	header := fileHeader{
		Version:   999, // Invalid version
		CreatedAt: time.Now(),
		ItemCount: 0,
	}
	if err := enc.Encode(&header); err != nil {
		f.Close()
		t.Fatalf("failed to encode header: %v", err)
	}
	f.Close()

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	// Load should error for version mismatch
	err = c.Load()
	if err == nil {
		t.Error("Load with version mismatch should error")
	}
}

func TestFlushNoFilePath(t *testing.T) {
	c := New()

	// Flush should be no-op when no file path is set
	if err := c.Flush(); err != nil {
		t.Errorf("Flush with no file path should return nil, got: %v", err)
	}
}

func TestLoadNoFilePath(t *testing.T) {
	c := New()

	// Load should be no-op when no file path is set
	if err := c.Load(); err != nil {
		t.Errorf("Load with no file path should return nil, got: %v", err)
	}
}

func TestCloseNoFilePath(t *testing.T) {
	c := New()

	// Close should be no-op when no file path is set
	if err := c.Close(); err != nil {
		t.Errorf("Close with no file path should return nil, got: %v", err)
	}
}

func TestTTLPreservedAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	// Set item with TTL
	c.SetWithTTL("key", []byte("value"), 200*time.Millisecond)
	if err := c.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Reload immediately - should have data
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if v := c2.Get("key"); !bytes.Equal(v, []byte("value")) {
		t.Errorf("key = %q, want %q", v, "value")
	}

	// Wait for TTL to expire
	time.Sleep(250 * time.Millisecond)

	if v := c2.Get("key"); v != nil {
		t.Errorf("key after TTL = %q, want nil", v)
	}
}

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	c.Set("key", []byte("value"))
	if err := c.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Verify no temp files left behind
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Errorf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestLoadReplacesContents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	// Create initial cache
	c1 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	c1.Set("fromfile", []byte("filedata"))
	if err := c1.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Create cache with existing data
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	c2.Set("inmemory", []byte("memdata"))

	// Load should replace contents
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if v := c2.Get("fromfile"); !bytes.Equal(v, []byte("filedata")) {
		t.Errorf("fromfile = %q, want %q", v, "filedata")
	}
	if v := c2.Get("inmemory"); v != nil {
		t.Errorf("inmemory = %q, want nil (Load should replace contents)", v)
	}
}

func TestDirtyFlagReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})

	// Initially not dirty
	if c.dirty.Load() {
		t.Error("new cache should not be dirty")
	}

	c.Set("key", []byte("value"))

	// Should be dirty after Set
	if !c.dirty.Load() {
		t.Error("cache should be dirty after Set")
	}

	if err := c.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Should not be dirty after Flush
	if c.dirty.Load() {
		t.Error("cache should not be dirty after Flush")
	}
}

func TestLoadRespectsMaxItems(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	// Create cache with no limit and save many items
	c1 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	for i := range 10 {
		c1.Set(string(rune('a'+i)), []byte("value"))
	}
	if err := c1.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Load into cache with MaxItems limit
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		MaxItems:       5,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c2.Len() > 5 {
		t.Errorf("Len = %d, want <= 5 (should respect MaxItems)", c2.Len())
	}
}

func TestLoadRespectsMaxBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	// Create cache with no limit
	c1 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		SkipLoadOnInit: true,
	})
	// Each item is ~10 bytes (1 byte key + 9 byte value)
	for i := range 10 {
		c1.Set(string(rune('a'+i)), []byte("123456789"))
	}
	if err := c1.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Load into cache with MaxBytes limit (~50 bytes = ~5 items)
	c2 := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone,
		MaxBytes:       50,
		SkipLoadOnInit: true,
	})
	if err := c2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if c2.Bytes() > 50 {
		t.Errorf("Bytes = %d, want <= 50 (should respect MaxBytes)", c2.Bytes())
	}
}

func TestOnErrorCallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupted.db")

	// Write garbage to file
	if err := os.WriteFile(path, []byte("not a valid gob file"), 0644); err != nil {
		t.Fatalf("failed to write corrupted file: %v", err)
	}

	var capturedOp string
	var capturedErr error

	// Create cache with OnError callback - load will fail
	_ = NewWithOptions(Options{
		FilePath:     path,
		SyncStrategy: SyncNone,
		OnError: func(op string, err error) {
			capturedOp = op
			capturedErr = err
		},
	})

	if capturedOp != "load" {
		t.Errorf("OnError op = %q, want %q", capturedOp, "load")
	}
	if capturedErr == nil {
		t.Error("OnError should have received an error")
	}
}

func TestSyncNoneExplicit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")

	c := NewWithOptions(Options{
		FilePath:       path,
		SyncStrategy:   SyncNone, // Explicitly set to SyncNone
		SkipLoadOnInit: true,
	})
	defer c.Close()

	c.Set("key", []byte("value"))

	// Wait a bit - should NOT auto-flush
	time.Sleep(50 * time.Millisecond)

	// File should not exist yet (no auto-sync)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should not exist with SyncNone until explicit Flush")
	}

	// Manual flush should work
	if err := c.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("file should exist after explicit Flush")
	}
}

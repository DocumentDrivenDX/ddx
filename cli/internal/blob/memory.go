package blob

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"time"
)

// MemoryBlob implements Store as an in-memory map. Intended for tests.
// All operations are safe for concurrent use.
type MemoryBlob struct {
	mu    sync.RWMutex
	store map[Key]memEntry
}

type memEntry struct {
	data    []byte
	modTime time.Time
}

// NewMemory returns an empty MemoryBlob ready for use.
func NewMemory() *MemoryBlob {
	return &MemoryBlob{store: make(map[Key]memEntry)}
}

// Put stores a snapshot of r at key, replacing any prior value.
func (m *MemoryBlob) Put(ctx context.Context, key Key, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = memEntry{data: append([]byte(nil), data...), modTime: time.Now()}
	return nil
}

// Get returns a reader over the stored bytes. The reader is backed by an
// in-memory copy; the caller must still call Close when done.
// Returns ErrNotFound when the key does not exist.
func (m *MemoryBlob) Get(ctx context.Context, key Key) (io.ReadCloser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.store[key]
	if !ok {
		return nil, ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(e.data)), nil
}

// Stat returns metadata without reading the blob body.
// Returns ErrNotFound when the key does not exist.
func (m *MemoryBlob) Stat(ctx context.Context, key Key) (Info, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.store[key]
	if !ok {
		return Info{}, ErrNotFound
	}
	return Info{
		Key:     key,
		Size:    int64(len(e.data)),
		ModTime: e.modTime,
	}, nil
}

// List returns all keys that start with prefix. Order is unspecified.
func (m *MemoryBlob) List(ctx context.Context, prefix Key) ([]Key, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var keys []Key
	for k := range m.store {
		if strings.HasPrefix(string(k), string(prefix)) {
			keys = append(keys, k)
		}
	}
	return keys, nil
}

// Delete removes the blob at key. Deleting a missing key is not an error.
func (m *MemoryBlob) Delete(ctx context.Context, key Key) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

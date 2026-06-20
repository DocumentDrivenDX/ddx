package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalFSBlob implements Store against a local filesystem directory.
// Keys map directly to file paths relative to Root:
//
//	key "executions/20260511T.../result.json" with Root "/path/to/.ddx"
//	→ file "/path/to/.ddx/executions/20260511T.../result.json"
//
// Put is atomic: it writes to a sibling temp file, syncs, then renames.
// Put also syncs the parent directory so the rename is durable on return.
type LocalFSBlob struct {
	Root string
}

// NewLocalFS returns a LocalFSBlob backed by the given root directory.
// Root must be an existing directory; subdirectories are created on demand.
func NewLocalFS(root string) *LocalFSBlob {
	return &LocalFSBlob{Root: root}
}

func (l *LocalFSBlob) absPath(key Key) (string, error) {
	k := string(key)
	if strings.Contains(k, "..") {
		return "", fmt.Errorf("blob: key contains '..': %q", key)
	}
	if strings.HasPrefix(k, "/") {
		return "", fmt.Errorf("blob: key must not start with '/': %q", key)
	}
	return filepath.Join(l.Root, filepath.FromSlash(k)), nil
}

// Put writes r atomically to the path derived from key.
// It creates parent directories as needed. On return with a nil error,
// the data is guaranteed durable (file fsynced, parent dir fsynced, rename done).
func (l *LocalFSBlob) Put(ctx context.Context, key Key, r io.Reader) error {
	path, err := l.absPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("blob put %q: mkdir: %w", key, err)
	}
	// Use os.CreateTemp so concurrent Puts to the same key each get a unique
	// temp file. The atomic rename means the last writer wins, which is the
	// correct last-write-wins semantics for an overwrite-capable store.
	f, err := os.CreateTemp(filepath.Dir(path), ".blobtmp-*")
	if err != nil {
		return fmt.Errorf("blob put %q: create temp: %w", key, err)
	}
	tmp := f.Name()
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("blob put %q: write: %w", key, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("blob put %q: sync: %w", key, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("blob put %q: close: %w", key, err)
	}
	// Sync parent directory so the rename is durable.
	if d, openErr := os.Open(filepath.Dir(path)); openErr == nil {
		_ = d.Sync()
		d.Close()
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("blob put %q: rename: %w", key, err)
	}
	return nil
}

// Get opens the blob at key for reading. The caller must Close the result.
// Returns ErrNotFound if the key does not exist.
func (l *LocalFSBlob) Get(ctx context.Context, key Key) (io.ReadCloser, error) {
	path, err := l.absPath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("blob get %q: %w", key, err)
	}
	return f, nil
}

// Stat returns metadata for key. Returns ErrNotFound if the key does not exist.
func (l *LocalFSBlob) Stat(ctx context.Context, key Key) (Info, error) {
	path, err := l.absPath(key)
	if err != nil {
		return Info{}, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Info{}, ErrNotFound
		}
		return Info{}, fmt.Errorf("blob stat %q: %w", key, err)
	}
	return Info{
		Key:     key,
		Size:    fi.Size(),
		ModTime: fi.ModTime(),
	}, nil
}

// List returns all keys whose string representation starts with prefix.
// Walks the directory rooted at Root/prefix recursively. Returns an empty
// slice (not an error) when the prefix path does not exist.
func (l *LocalFSBlob) List(ctx context.Context, prefix Key) ([]Key, error) {
	walkRoot, err := l.absPath(prefix)
	if err != nil {
		return nil, err
	}
	var keys []Key
	walkErr := filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(l.Root, path)
		if relErr != nil {
			return relErr
		}
		keys = append(keys, Key(filepath.ToSlash(rel)))
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, os.ErrNotExist) {
		return nil, fmt.Errorf("blob list %q: %w", prefix, walkErr)
	}
	return keys, nil
}

// Delete removes the blob at key. A missing key is not an error.
func (l *LocalFSBlob) Delete(ctx context.Context, key Key) error {
	path, err := l.absPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("blob delete %q: %w", key, err)
	}
	return nil
}

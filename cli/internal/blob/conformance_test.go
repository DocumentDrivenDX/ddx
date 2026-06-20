package blob_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/blob"
)

// TestBlobStoreConformance_LocalFS runs the shared conformance suite against
// a LocalFSBlob backed by a temp directory.
func TestBlobStoreConformance_LocalFS(t *testing.T) {
	runConformance(t, func(t *testing.T) blob.Store {
		t.Helper()
		return blob.NewLocalFS(t.TempDir())
	})
}

// TestBlobStoreConformance_Memory runs the shared conformance suite against
// an in-memory MemoryBlob implementation.
func TestBlobStoreConformance_Memory(t *testing.T) {
	runConformance(t, func(t *testing.T) blob.Store {
		t.Helper()
		return blob.NewMemory()
	})
}

// runConformance runs the full conformance suite against any Store. All
// sub-test functions are named TestBlobStoreConformance_* as required by
// FEAT-028 §Acceptance Criteria #4.
func runConformance(t *testing.T, makeStore func(*testing.T) blob.Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("PutGet", func(t *testing.T) {
		s := makeStore(t)
		data := []byte("hello blob world")
		if err := s.Put(ctx, "col/id/file.txt", bytes.NewReader(data)); err != nil {
			t.Fatalf("Put: %v", err)
		}
		rc, err := s.Get(ctx, "col/id/file.txt")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer rc.Close()
		got, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if !bytes.Equal(got, data) {
			t.Errorf("Get: got %q, want %q", got, data)
		}
	})

	t.Run("PutOverwrite", func(t *testing.T) {
		s := makeStore(t)
		if err := s.Put(ctx, "k", strings.NewReader("v1")); err != nil {
			t.Fatal(err)
		}
		if err := s.Put(ctx, "k", strings.NewReader("v2")); err != nil {
			t.Fatal(err)
		}
		rc, err := s.Get(ctx, "k")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer rc.Close()
		got, _ := io.ReadAll(rc)
		if string(got) != "v2" {
			t.Errorf("overwrite: got %q, want %q", got, "v2")
		}
	})

	t.Run("Stat", func(t *testing.T) {
		s := makeStore(t)
		data := []byte("stat me")
		before := time.Now().Add(-time.Second)
		if err := s.Put(ctx, "col/id/stat.bin", bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		after := time.Now().Add(time.Second)
		info, err := s.Stat(ctx, "col/id/stat.bin")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if info.Key != "col/id/stat.bin" {
			t.Errorf("Stat Key: got %q, want %q", info.Key, "col/id/stat.bin")
		}
		if info.Size != int64(len(data)) {
			t.Errorf("Stat Size: got %d, want %d", info.Size, len(data))
		}
		if info.ModTime.Before(before) || info.ModTime.After(after) {
			t.Errorf("Stat ModTime %v not in [%v, %v]", info.ModTime, before, after)
		}
	})

	t.Run("ListPrefix", func(t *testing.T) {
		s := makeStore(t)
		keys := []blob.Key{
			"executions/attempt-1/prompt.md",
			"executions/attempt-1/result.json",
			"executions/attempt-2/result.json",
			"attachments/bead-1/events.jsonl",
		}
		for _, k := range keys {
			if err := s.Put(ctx, k, strings.NewReader("data")); err != nil {
				t.Fatalf("Put %q: %v", k, err)
			}
		}
		got, err := s.List(ctx, "executions/attempt-1")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		want := []blob.Key{
			"executions/attempt-1/prompt.md",
			"executions/attempt-1/result.json",
		}
		sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
		sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
		if len(got) != len(want) {
			t.Fatalf("List count: got %v, want %v", got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("List[%d]: got %q, want %q", i, got[i], want[i])
			}
		}
	})

	t.Run("ListMissingPrefix", func(t *testing.T) {
		s := makeStore(t)
		got, err := s.List(ctx, "nonexistent/prefix")
		if err != nil {
			t.Fatalf("List missing prefix: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("List missing prefix: got %v, want empty", got)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		s := makeStore(t)
		if err := s.Put(ctx, "del/key", strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, "del/key"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Get(ctx, "del/key"); !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("after Delete, Get: got err %v, want ErrNotFound", err)
		}
	})

	t.Run("DeleteIdempotent", func(t *testing.T) {
		s := makeStore(t)
		// Deleting a key that was never Put must be a no-op.
		if err := s.Delete(ctx, "never/existed"); err != nil {
			t.Errorf("Delete missing key: got err %v, want nil", err)
		}
		// Deleting the same key twice must also be a no-op.
		if err := s.Put(ctx, "twice/del", strings.NewReader("y")); err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, "twice/del"); err != nil {
			t.Fatalf("first Delete: %v", err)
		}
		if err := s.Delete(ctx, "twice/del"); err != nil {
			t.Errorf("second Delete: got err %v, want nil", err)
		}
	})

	t.Run("ErrNotFound", func(t *testing.T) {
		s := makeStore(t)
		_, err := s.Get(ctx, "missing/key")
		if !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("Get missing key: got err %v, want ErrNotFound", err)
		}
		_, err = s.Stat(ctx, "missing/key")
		if !errors.Is(err, blob.ErrNotFound) {
			t.Errorf("Stat missing key: got err %v, want ErrNotFound", err)
		}
	})

	t.Run("ConcurrentPut", func(t *testing.T) {
		s := makeStore(t)
		const workers = 8
		const key blob.Key = "concurrent/key"
		var wg sync.WaitGroup
		errs := make([]error, workers)
		for i := range workers {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				errs[n] = s.Put(ctx, key, strings.NewReader("value"))
			}(i)
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Errorf("worker %d: Put: %v", i, err)
			}
		}
		// After all concurrent puts, the key must be readable.
		rc, err := s.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get after concurrent puts: %v", err)
		}
		rc.Close()
	})
}

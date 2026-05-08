package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TestBeadsByProject_UpdateVisibleOnNextRequest covers ddx-9ce6842a AC §6:
// write a bead via the store, then query through the GraphQL list resolver,
// and assert the new value is visible on the very next request.
func TestBeadsByProject_UpdateVisibleOnNextRequest(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "stale-read-list")

	projectPath := filepath.Join(t.TempDir(), "project")
	store := bead.NewStore(filepath.Join(projectPath, ".ddx"))
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beads := make([]bead.Bead, 0, 30)
	for i := 0; i < 30; i++ {
		beads = append(beads, bead.Bead{
			ID:        fmt.Sprintf("ddx-stale-%03d", i),
			Title:     fmt.Sprintf("original title %03d", i),
			Status:    bead.StatusOpen,
			Priority:  bead.DefaultPriority,
			IssueType: bead.DefaultType,
			CreatedAt: base,
			UpdatedAt: base,
		})
	}
	if err := store.WriteAll(beads); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := New(":0", projectPath)
	t.Cleanup(func() { _ = srv.Shutdown() })
	projects := srv.state.GetProjects()
	if len(projects) != 1 {
		t.Fatalf("setup: want 1 project, got %d", len(projects))
	}
	projectID := projects[0].ID
	handler := srv.Handler()

	// Warm the handler so any first-request caches are seeded.
	warmQuery := fmt.Sprintf(`{ beadsByProject(projectID: %q, first: 50) { edges { node { id title } } } }`, projectID)
	mustPostStaleList(t, handler, warmQuery)

	// Update one bead on disk via the bead.Store — this is the same write
	// path `ddx bead update` uses.
	updatedTitle := "mutated via store.Update"
	if err := store.Update("ddx-stale-017", func(b *bead.Bead) {
		b.Title = updatedTitle
	}); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Next GraphQL request must see the new value without any explicit
	// invalidation or sleep — the store is the source of truth.
	resp := mustPostStaleList(t, handler, warmQuery)
	var body struct {
		Data struct {
			BeadsByProject struct {
				Edges []struct {
					Node struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"beadsByProject"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp, &body); err != nil {
		t.Fatalf("decode: %v\n%s", err, resp)
	}
	found := false
	for _, edge := range body.Data.BeadsByProject.Edges {
		if edge.Node.ID == "ddx-stale-017" {
			if edge.Node.Title != updatedTitle {
				t.Fatalf("stale read: title=%q, want %q", edge.Node.Title, updatedTitle)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("updated bead not present in response")
	}
}

// TestBeadsByProject_ConcurrentQueriesAndUpdates covers ddx-9ce6842a AC §6:
// N concurrent readers + writers interleaved without torn reads or panics.
func TestBeadsByProject_ConcurrentQueriesAndUpdates(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("DDX_NODE_NAME", "stale-read-concurrent")

	projectPath := filepath.Join(t.TempDir(), "project")
	store := bead.NewStore(filepath.Join(projectPath, ".ddx"))
	if err := store.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beads := make([]bead.Bead, 0, 20)
	for i := 0; i < 20; i++ {
		beads = append(beads, bead.Bead{
			ID:        fmt.Sprintf("ddx-concur-%03d", i),
			Title:     "concurrent fixture",
			Status:    bead.StatusOpen,
			Priority:  bead.DefaultPriority,
			IssueType: bead.DefaultType,
			CreatedAt: base,
			UpdatedAt: base,
		})
	}
	if err := store.WriteAll(beads); err != nil {
		t.Fatalf("write: %v", err)
	}

	srv := New(":0", projectPath)
	t.Cleanup(func() { _ = srv.Shutdown() })
	projects := srv.state.GetProjects()
	if len(projects) != 1 {
		t.Fatalf("setup: want 1 project, got %d", len(projects))
	}
	projectID := projects[0].ID
	handler := srv.Handler()

	const readers = 8
	const iterations = 15
	errs := make(chan error, readers*iterations*2)
	var wg sync.WaitGroup
	// Writers: keep renaming bead 000 through a known sequence.
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				title := fmt.Sprintf("w%02d-i%03d", w, i)
				if err := store.Update("ddx-concur-000", func(b *bead.Bead) {
					b.Title = title
				}); err != nil {
					errs <- fmt.Errorf("writer %d iter %d: %w", w, i, err)
					return
				}
			}
		}(w)
	}
	// Readers: hammer the list resolver and parse every response.
	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func(r int) {
			defer wg.Done()
			query := fmt.Sprintf(`{ beadsByProject(projectID: %q, first: 50) { edges { node { id title } } } }`, projectID)
			for i := 0; i < iterations; i++ {
				raw, err := postStaleList(handler, query)
				if err != nil {
					errs <- fmt.Errorf("reader %d iter %d: %w", r, i, err)
					return
				}
				// Shape sanity check — the whole beads list must be present
				// even while a writer is mutating bead 000. A torn read would
				// manifest as either a missing "edges" key or a 500.
				if !bytes.Contains(raw, []byte(`"edges"`)) {
					errs <- fmt.Errorf("reader %d iter %d: malformed body: %s", r, i, string(raw))
					return
				}
				if !bytes.Contains(raw, []byte("ddx-concur-000")) {
					errs <- fmt.Errorf("reader %d iter %d: bead 000 missing from response", r, i)
					return
				}
			}
		}(r)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	_ = strings.TrimSpace // keep import in case reader evolves
}

func mustPostStaleList(t *testing.T, h http.Handler, query string) []byte {
	t.Helper()
	raw, err := postStaleList(h, query)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func postStaleList(h http.Handler, query string) ([]byte, error) {
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, err
	}
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", w.Code, w.Body.String())
	}
	return w.Body.Bytes(), nil
}

package graphql

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
)

func providerModelsTestContext(t *testing.T, workDir, name string, listModels func(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error)) context.Context {
	t.Helper()
	stub := &graphqlInventoryStub{
		providers:    []agentlib.ProviderInfo{{Name: name, BaseURL: "https://api.example.com"}},
		listModelsFn: listModels,
	}
	return graphqlInventoryContext(t, workDir, stub)
}

func TestSanitizeBaseURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://api.example.com/v1", "https://api.example.com/v1"},
		{"https://user:pass@api.example.com/v1?key=secret#frag", "https://api.example.com/v1"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"  https://host.tld  ", "https://host.tld"},
		{"", ""},
		{"not a url", ""},
		{"file:///etc/passwd", ""}, // no host
	}
	for _, tc := range cases {
		if got := sanitizeBaseURL(tc.in); got != tc.want {
			t.Errorf("sanitizeBaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestProviderModelsCacheServesFromCacheWithinTTL verifies that a second
// query within the TTL does not invoke the live fetcher and surfaces
// fromCache=true without mutating the cached fetchedAt timestamp.
func TestProviderModelsCacheServesFromCacheWithinTTL(t *testing.T) {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
	t.Cleanup(func() {
		providerModelsCache.Lock()
		providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
		providerModelsCache.inFlight = make(map[string]chan struct{})
		providerModelsCache.Unlock()
	})

	var calls int32
	workDir := t.TempDir()
	ctx := providerModelsTestContext(t, workDir, "openai", func(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
		atomic.AddInt32(&calls, 1)
		return []agentlib.ModelInfo{}, nil
	})

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}

	first, err := r.ProviderModels(ctx, "openai", ProviderKindEndpoint)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first.FromCache {
		t.Fatalf("first call should be a live fetch, got fromCache=true")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("first call: want 1 fetcher invocation, got %d", got)
	}

	second, err := r.ProviderModels(ctx, "openai", ProviderKindEndpoint)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !second.FromCache {
		t.Fatalf("second call should be cached, got fromCache=false")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("second call: fetcher should not be invoked again, got %d total calls", got)
	}
	if first.FetchedAt != second.FetchedAt {
		t.Fatalf("cached fetchedAt should not change between calls: first=%s second=%s", first.FetchedAt, second.FetchedAt)
	}
}

// TestProviderModelsCacheRevalidatesCurrentFizeauIdentity proves a model
// payload cached for one request seam cannot preserve membership or canonical
// identity across later Fizeau listings for the same project.
func TestProviderModelsCacheRevalidatesCurrentFizeauIdentity(t *testing.T) {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
	t.Cleanup(func() {
		providerModelsCache.Lock()
		providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
		providerModelsCache.inFlight = make(map[string]chan struct{})
		providerModelsCache.Unlock()
	})

	workDir := t.TempDir()
	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}
	seed := &graphqlInventoryStub{
		providers: []agentlib.ProviderInfo{{Name: "provider", BaseURL: "https://old.example/v1"}},
		models:    []agentlib.ModelInfo{{ID: "old-model"}},
	}
	seedCtx := graphqlInventoryContext(t, workDir, seed)
	first, err := r.ProviderModels(seedCtx, "provider", ProviderKindEndpoint)
	if err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if first.FromCache || first.Name != "provider" || len(first.Models) != 1 || first.Models[0].ID != "old-model" {
		t.Fatalf("unexpected seed result: %#v", first)
	}

	removed := &graphqlInventoryStub{}
	removedCtx := graphqlInventoryContext(t, workDir, removed)
	if got, err := r.ProviderModels(removedCtx, "provider", ProviderKindEndpoint); err == nil {
		t.Fatalf("removed provider returned stale cached row: %#v", got)
	}
	if got := removed.modelCalls.Load(); got != 0 {
		t.Fatalf("removed provider must fail membership before ListModels, got %d model calls", got)
	}

	// EqualFold still matches this request, but the canonical Fizeau identity
	// changed. Exact canonical cache keys force a fresh model payload.
	changed := &graphqlInventoryStub{
		providers: []agentlib.ProviderInfo{{Name: "PROVIDER", BaseURL: "https://new.example/v1"}},
		models:    []agentlib.ModelInfo{{ID: "new-model"}},
	}
	changedCtx := graphqlInventoryContext(t, workDir, changed)
	current, err := r.ProviderModels(changedCtx, "provider", ProviderKindEndpoint)
	if err != nil {
		t.Fatalf("changed canonical identity: %v", err)
	}
	if current.FromCache {
		t.Fatalf("changed canonical identity returned the prior cached row: %#v", current)
	}
	if current.Name != "PROVIDER" || current.BaseURL != "https://new.example/v1" {
		t.Fatalf("current Fizeau identity not preserved: %#v", current)
	}
	if len(current.Models) != 1 || current.Models[0].ID != "new-model" {
		t.Fatalf("changed identity returned stale models: %#v", current.Models)
	}
	if got := changed.lastFilter.Provider; got != "PROVIDER" {
		t.Fatalf("ListModels filter = %q, want canonical current identity", got)
	}
}

// TestProviderModelsCacheExpiresAfterTTL verifies expired entries trigger a
// fresh live fetch.
func TestProviderModelsCacheExpiresAfterTTL(t *testing.T) {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
	t.Cleanup(func() {
		providerModelsCache.Lock()
		providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
		providerModelsCache.inFlight = make(map[string]chan struct{})
		providerModelsCache.Unlock()
	})

	prevTTL := providerModelsCacheTTL
	providerModelsCacheTTL = 10 * time.Millisecond
	t.Cleanup(func() { providerModelsCacheTTL = prevTTL })

	var calls int32
	workDir := t.TempDir()
	ctx := providerModelsTestContext(t, workDir, "p", func(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
		atomic.AddInt32(&calls, 1)
		return []agentlib.ModelInfo{}, nil
	})

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}

	if _, err := r.ProviderModels(ctx, "p", ProviderKindEndpoint); err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(25 * time.Millisecond)
	if _, err := r.ProviderModels(ctx, "p", ProviderKindEndpoint); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("want 2 fetcher invocations after TTL expiry, got %d", got)
	}
}

// TestProviderModelsInFlightGuard verifies concurrent queries for the same
// (workDir, name, kind) collapse into a single underlying fetch.
func TestProviderModelsInFlightGuard(t *testing.T) {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
	t.Cleanup(func() {
		providerModelsCache.Lock()
		providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
		providerModelsCache.inFlight = make(map[string]chan struct{})
		providerModelsCache.Unlock()
	})

	var calls int32
	release := make(chan struct{})
	workDir := t.TempDir()
	ctx := providerModelsTestContext(t, workDir, "p", func(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return []agentlib.ModelInfo{}, nil
	})

	r := &queryResolver{Resolver: &Resolver{WorkingDir: workDir}}

	const N = 8
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = r.ProviderModels(ctx, "p", ProviderKindEndpoint)
		}()
	}

	// Give goroutines a moment to all park on the in-flight wait.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("in-flight guard should coalesce concurrent fetches: want 1 invocation, got %d", got)
	}
}

// TestRefreshProviderModelsBypassesCache verifies the mutation always invokes
// a live fetch regardless of cache state.
func TestRefreshProviderModelsBypassesCache(t *testing.T) {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
	t.Cleanup(func() {
		providerModelsCache.Lock()
		providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
		providerModelsCache.inFlight = make(map[string]chan struct{})
		providerModelsCache.Unlock()
	})

	var calls int32
	workDir := t.TempDir()
	ctx := providerModelsTestContext(t, workDir, "p", func(context.Context, agentlib.ModelFilter) ([]agentlib.ModelInfo, error) {
		atomic.AddInt32(&calls, 1)
		return []agentlib.ModelInfo{}, nil
	})

	resolver := &Resolver{WorkingDir: workDir}
	q := &queryResolver{Resolver: resolver}
	m := &mutationResolver{Resolver: resolver}

	if _, err := q.ProviderModels(ctx, "p", ProviderKindEndpoint); err != nil {
		t.Fatalf("seed query: %v", err)
	}
	if _, err := m.RefreshProviderModels(ctx, "p", ProviderKindEndpoint); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("refresh should always trigger a live fetch: want 2 invocations, got %d", got)
	}

	cached, err := q.ProviderModels(ctx, "p", ProviderKindEndpoint)
	if err != nil {
		t.Fatalf("post-refresh query: %v", err)
	}
	if !cached.FromCache {
		t.Fatalf("query after refresh should come from cache, got fromCache=false")
	}
}

func TestProviderModelsRejectsEmptyName(t *testing.T) {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
	t.Cleanup(func() {
		providerModelsCache.Lock()
		providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
		providerModelsCache.inFlight = make(map[string]chan struct{})
		providerModelsCache.Unlock()
	})
	r := &queryResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	if _, err := r.ProviderModels(context.Background(), "  ", ProviderKindEndpoint); err == nil {
		t.Fatalf("expected error for empty name")
	}
	m := &mutationResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	if _, err := m.RefreshProviderModels(context.Background(), "", ProviderKindEndpoint); err == nil {
		t.Fatalf("expected error for empty name")
	}
}

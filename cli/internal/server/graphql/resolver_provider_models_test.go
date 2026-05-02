package graphql

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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
	resetProviderModelsCacheForTest()
	t.Cleanup(resetProviderModelsCacheForTest)

	var calls int32
	prev := providerModelsFetcher
	providerModelsFetcher = func(ctx context.Context, workDir, name string, kind ProviderKind) (*ProviderModelsResult, error) {
		atomic.AddInt32(&calls, 1)
		return &ProviderModelsResult{
			Name:      name,
			Kind:      kind,
			BaseURL:   "https://api.example.com",
			FetchedAt: time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
	t.Cleanup(func() { providerModelsFetcher = prev })

	r := &queryResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	ctx := context.Background()

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

// TestProviderModelsCacheExpiresAfterTTL verifies expired entries trigger a
// fresh live fetch.
func TestProviderModelsCacheExpiresAfterTTL(t *testing.T) {
	resetProviderModelsCacheForTest()
	t.Cleanup(resetProviderModelsCacheForTest)

	prevTTL := providerModelsCacheTTL
	providerModelsCacheTTL = 10 * time.Millisecond
	t.Cleanup(func() { providerModelsCacheTTL = prevTTL })

	var calls int32
	prev := providerModelsFetcher
	providerModelsFetcher = func(ctx context.Context, workDir, name string, kind ProviderKind) (*ProviderModelsResult, error) {
		atomic.AddInt32(&calls, 1)
		return &ProviderModelsResult{Name: name, Kind: kind, FetchedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
	}
	t.Cleanup(func() { providerModelsFetcher = prev })

	r := &queryResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	ctx := context.Background()

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
	resetProviderModelsCacheForTest()
	t.Cleanup(resetProviderModelsCacheForTest)

	var calls int32
	release := make(chan struct{})
	prev := providerModelsFetcher
	providerModelsFetcher = func(ctx context.Context, workDir, name string, kind ProviderKind) (*ProviderModelsResult, error) {
		atomic.AddInt32(&calls, 1)
		<-release
		return &ProviderModelsResult{Name: name, Kind: kind, FetchedAt: time.Now().UTC().Format(time.RFC3339)}, nil
	}
	t.Cleanup(func() { providerModelsFetcher = prev })

	r := &queryResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	ctx := context.Background()

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
	resetProviderModelsCacheForTest()
	t.Cleanup(resetProviderModelsCacheForTest)

	var calls int32
	prev := providerModelsFetcher
	providerModelsFetcher = func(ctx context.Context, workDir, name string, kind ProviderKind) (*ProviderModelsResult, error) {
		atomic.AddInt32(&calls, 1)
		return &ProviderModelsResult{Name: name, Kind: kind, FetchedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
	}
	t.Cleanup(func() { providerModelsFetcher = prev })

	resolver := &Resolver{WorkingDir: "/tmp/wd"}
	q := &queryResolver{Resolver: resolver}
	m := &mutationResolver{Resolver: resolver}
	ctx := context.Background()

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
	resetProviderModelsCacheForTest()
	t.Cleanup(resetProviderModelsCacheForTest)
	r := &queryResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	if _, err := r.ProviderModels(context.Background(), "  ", ProviderKindEndpoint); err == nil {
		t.Fatalf("expected error for empty name")
	}
	m := &mutationResolver{Resolver: &Resolver{WorkingDir: "/tmp/wd"}}
	if _, err := m.RefreshProviderModels(context.Background(), "", ProviderKindEndpoint); err == nil {
		t.Fatalf("expected error for empty name")
	}
}

package graphql

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	agentlib "github.com/DocumentDrivenDX/fizeau"
)

// providerModelsCacheTTL is how long cached provider model snapshots live
// before a query falls through to a live discovery call. The bead spec calls
// for a 60s TTL, surfaced as a package var so tests can lower it.
var providerModelsCacheTTL = 60 * time.Second

// providerModelsFetcher is the live discovery fetcher used by the cache. It
// is a package var so tests can replace it without standing up a real
// fizeau service. Production code uses fetchProviderModelsLive.
var providerModelsFetcher = fetchProviderModelsLive

type providerModelsCacheEntry struct {
	result    *ProviderModelsResult
	fetchedAt time.Time
}

var providerModelsCache = struct {
	sync.Mutex
	entries  map[string]providerModelsCacheEntry
	inFlight map[string]chan struct{}
}{
	entries:  make(map[string]providerModelsCacheEntry),
	inFlight: make(map[string]chan struct{}),
}

func providerModelsCacheKey(workDir, name string, kind ProviderKind) string {
	return workDir + "\x00" + strings.ToLower(strings.TrimSpace(name)) + "\x00" + string(kind)
}

// ProviderModels is the resolver for the providerModels query.
// Returns a cached snapshot when one is fresh (within TTL); otherwise issues
// a single coalesced live fetch (in-flight guard prevents duplicate concurrent
// discovery calls for the same key).
func (r *queryResolver) ProviderModels(ctx context.Context, name string, kind ProviderKind) (*ProviderModelsResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if !kind.IsValid() {
		return nil, fmt.Errorf("invalid kind: %s", kind)
	}

	key := providerModelsCacheKey(r.WorkingDir, name, kind)

	if cached, ok := lookupProviderModelsCache(key); ok {
		// Serve cached copy with fromCache=true; do NOT alter fetchedAt.
		out := cloneProviderModelsResult(cached.result)
		out.FromCache = true
		return out, nil
	}

	res, err := loadProviderModels(ctx, r.WorkingDir, name, kind, key)
	if err != nil {
		return nil, err
	}
	out := cloneProviderModelsResult(res)
	out.FromCache = false
	return out, nil
}

// RefreshProviderModels is the resolver for the refreshProviderModels mutation.
// Always performs a live discovery, ignoring TTL, but still routes through the
// in-flight guard so concurrent refreshes for the same key are coalesced.
// Auth-gating is provided by the requireTrusted wrapper on the /graphql HTTP
// route (see cli/internal/server/server.go); this resolver runs only for
// trusted callers.
func (r *mutationResolver) RefreshProviderModels(ctx context.Context, name string, kind ProviderKind) (*ProviderModelsResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	if !kind.IsValid() {
		return nil, fmt.Errorf("invalid kind: %s", kind)
	}

	key := providerModelsCacheKey(r.WorkingDir, name, kind)
	res, err := refreshProviderModels(ctx, r.WorkingDir, name, kind, key)
	if err != nil {
		return nil, err
	}
	out := cloneProviderModelsResult(res)
	out.FromCache = false
	return out, nil
}

// lookupProviderModelsCache returns the cached entry when it is still fresh.
func lookupProviderModelsCache(key string) (providerModelsCacheEntry, bool) {
	providerModelsCache.Lock()
	defer providerModelsCache.Unlock()
	entry, ok := providerModelsCache.entries[key]
	if !ok || entry.result == nil {
		return providerModelsCacheEntry{}, false
	}
	if time.Since(entry.fetchedAt) > providerModelsCacheTTL {
		return providerModelsCacheEntry{}, false
	}
	return entry, true
}

// loadProviderModels returns a fresh result, performing a live discovery only
// when no other goroutine is already doing so. When an in-flight fetch exists
// for this key, the caller waits on its completion and then reads the cache.
func loadProviderModels(ctx context.Context, workDir, name string, kind ProviderKind, key string) (*ProviderModelsResult, error) {
	return doProviderModelsFetch(ctx, workDir, name, kind, key, false)
}

// refreshProviderModels forces a live discovery (no TTL check), still
// coalescing concurrent calls for the same key.
func refreshProviderModels(ctx context.Context, workDir, name string, kind ProviderKind, key string) (*ProviderModelsResult, error) {
	return doProviderModelsFetch(ctx, workDir, name, kind, key, true)
}

func doProviderModelsFetch(ctx context.Context, workDir, name string, kind ProviderKind, key string, force bool) (*ProviderModelsResult, error) {
	for {
		providerModelsCache.Lock()
		// If a fetch is in flight for this key, wait for it and re-evaluate.
		if ch, ok := providerModelsCache.inFlight[key]; ok {
			providerModelsCache.Unlock()
			select {
			case <-ch:
				if !force {
					if entry, ok := lookupProviderModelsCache(key); ok {
						return entry.result, nil
					}
				}
				// Fall through to retry — we either need a fresh fetch or the
				// previous attempt failed.
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// We own the in-flight slot.
		ch := make(chan struct{})
		providerModelsCache.inFlight[key] = ch
		providerModelsCache.Unlock()

		result, err := providerModelsFetcher(ctx, workDir, name, kind)

		providerModelsCache.Lock()
		delete(providerModelsCache.inFlight, key)
		if err == nil && result != nil {
			providerModelsCache.entries[key] = providerModelsCacheEntry{
				result:    result,
				fetchedAt: time.Now().UTC(),
			}
		}
		providerModelsCache.Unlock()
		close(ch)

		if err != nil {
			return nil, err
		}
		return result, nil
	}
}

// fetchProviderModelsLive performs the live discovery via the agent service.
// For ENDPOINT kinds it filters by provider name; for HARNESS kinds it filters
// by harness name. The returned baseURL is sanitized (credentials, query, and
// fragment stripped) for ts-net display safety; harness rows return "".
func fetchProviderModelsLive(ctx context.Context, workDir, name string, kind ProviderKind) (*ProviderModelsResult, error) {
	svc, err := agent.NewServiceFromWorkDir(workDir)
	if err != nil {
		return nil, fmt.Errorf("agent service: %w", err)
	}

	filter := agentlib.ModelFilter{}
	switch kind {
	case ProviderKindEndpoint:
		filter.Provider = name
	case ProviderKindHarness:
		filter.Harness = name
	default:
		return nil, fmt.Errorf("invalid kind: %s", kind)
	}

	models, err := svc.ListModels(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	baseURL := ""
	if kind == ProviderKindEndpoint {
		// Pick up the provider's base URL from ListProviders, fall back to
		// the first model's recorded EndpointBaseURL when discovery returned
		// it.
		if providers, perr := svc.ListProviders(ctx); perr == nil {
			for _, p := range providers {
				if strings.EqualFold(p.Name, name) {
					baseURL = sanitizeBaseURL(p.BaseURL)
					break
				}
			}
		}
		if baseURL == "" {
			for _, m := range models {
				if m.EndpointBaseURL != "" {
					baseURL = sanitizeBaseURL(m.EndpointBaseURL)
					break
				}
			}
		}
	}

	entries := make([]*ProviderModelEntry, 0, len(models))
	for _, m := range models {
		entry := &ProviderModelEntry{
			ID:        m.ID,
			Available: m.Available,
		}
		if m.ContextLength > 0 {
			cl := m.ContextLength
			entry.ContextLength = &cl
		}
		entries = append(entries, entry)
	}

	return &ProviderModelsResult{
		Name:      name,
		Kind:      kind,
		Models:    entries,
		BaseURL:   baseURL,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		FromCache: false,
	}, nil
}

// sanitizeBaseURL returns a base URL safe to surface over a ts-net trusted
// channel: credentials, query string, and fragment are dropped, the scheme
// and host are preserved, and any path is kept verbatim. Inputs that fail to
// parse, or lack a host, return "" — never the original raw string — so a
// malformed config cannot leak credentials downstream.
func sanitizeBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if u.Scheme == "" || u.Host == "" {
		return ""
	}
	clean := &url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}
	return clean.String()
}

// cloneProviderModelsResult deep-copies a result so cached entries are not
// mutated when callers flip FromCache or a future field.
func cloneProviderModelsResult(in *ProviderModelsResult) *ProviderModelsResult {
	if in == nil {
		return nil
	}
	out := *in
	if in.Models != nil {
		out.Models = make([]*ProviderModelEntry, 0, len(in.Models))
		for _, m := range in.Models {
			if m == nil {
				continue
			}
			mc := *m
			if m.ContextLength != nil {
				v := *m.ContextLength
				mc.ContextLength = &v
			}
			out.Models = append(out.Models, &mc)
		}
	}
	return &out
}

// resetProviderModelsCacheForTest is a test seam.
func resetProviderModelsCacheForTest() {
	providerModelsCache.Lock()
	providerModelsCache.entries = make(map[string]providerModelsCacheEntry)
	providerModelsCache.inFlight = make(map[string]chan struct{})
	providerModelsCache.Unlock()
}

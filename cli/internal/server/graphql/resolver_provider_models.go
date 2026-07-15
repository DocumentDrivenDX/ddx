package graphql

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	agentlib "github.com/easel/fizeau"
)

// providerModelsCacheTTL is how long cached provider model snapshots live
// before a query falls through to a live discovery call. The bead spec calls
// for a 60s TTL, surfaced as a package var so tests can lower it.
var providerModelsCacheTTL = 60 * time.Second

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
	// The canonical Fizeau identity is intentionally case-sensitive here. A
	// listing that changes only canonical casing is still a new current
	// identity and must not inherit a prior row's cached model payload.
	return workDir + "\x00" + name + "\x00" + string(kind)
}

type providerModelsIdentity struct {
	name    string
	baseURL string
	kind    ProviderKind
	service inventoryService
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

	workDir := r.workingDir(ctx)
	identity, err := resolveProviderModelsIdentity(ctx, workDir, name, kind)
	if err != nil {
		return nil, err
	}
	key := providerModelsCacheKey(workDir, identity.name, kind)

	if cached, ok := lookupProviderModelsCache(key); ok {
		// Membership and canonical identity were revalidated above. Only the
		// model payload is cached; current identity facts always win.
		out := cloneProviderModelsResult(cached.result)
		applyProviderModelsIdentity(out, identity)
		out.FromCache = true
		return out, nil
	}

	res, err := loadProviderModels(ctx, identity, key)
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

	workDir := r.workingDir(ctx)
	identity, err := resolveProviderModelsIdentity(ctx, workDir, name, kind)
	if err != nil {
		return nil, err
	}
	key := providerModelsCacheKey(workDir, identity.name, kind)
	res, err := refreshProviderModels(ctx, identity, key)
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
func loadProviderModels(ctx context.Context, identity providerModelsIdentity, key string) (*ProviderModelsResult, error) {
	return doProviderModelsFetch(ctx, identity, key, false)
}

// refreshProviderModels forces a live discovery (no TTL check), still
// coalescing concurrent calls for the same key.
func refreshProviderModels(ctx context.Context, identity providerModelsIdentity, key string) (*ProviderModelsResult, error) {
	return doProviderModelsFetch(ctx, identity, key, true)
}

func doProviderModelsFetch(ctx context.Context, identity providerModelsIdentity, key string, force bool) (*ProviderModelsResult, error) {
	for {
		providerModelsCache.Lock()
		// If a fetch is in flight for this key, wait for it and re-evaluate.
		if ch, ok := providerModelsCache.inFlight[key]; ok {
			providerModelsCache.Unlock()
			select {
			case <-ch:
				if !force {
					if entry, ok := lookupProviderModelsCache(key); ok {
						out := cloneProviderModelsResult(entry.result)
						applyProviderModelsIdentity(out, identity)
						return out, nil
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

		result, err := fetchProviderModelsLive(ctx, identity)

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

// resolveProviderModelsIdentity validates current Fizeau membership before
// any model-cache access and returns the canonical listed identity plus the
// same request-scoped service to use for a cache miss.
func resolveProviderModelsIdentity(ctx context.Context, workDir, name string, kind ProviderKind) (providerModelsIdentity, error) {
	svc, err := inventoryServiceForRequest(ctx, workDir)
	if err != nil {
		return providerModelsIdentity{}, fmt.Errorf("Fizeau inventory service: %w", err)
	}

	switch kind {
	case ProviderKindEndpoint:
		providers, err := svc.ListProviders(ctx)
		if err != nil {
			return providerModelsIdentity{}, fmt.Errorf("list providers: %w", err)
		}
		for _, provider := range providers {
			if strings.EqualFold(provider.Name, name) {
				return providerModelsIdentity{
					name:    provider.Name,
					baseURL: sanitizeBaseURL(provider.BaseURL),
					kind:    kind,
					service: svc,
				}, nil
			}
		}
		return providerModelsIdentity{}, fmt.Errorf("provider not found: %s", name)
	case ProviderKindHarness:
		harnesses, err := svc.ListHarnesses(ctx)
		if err != nil {
			return providerModelsIdentity{}, fmt.Errorf("list harnesses: %w", err)
		}
		for _, harness := range harnesses {
			if strings.EqualFold(harness.Name, name) {
				return providerModelsIdentity{name: harness.Name, kind: kind, service: svc}, nil
			}
		}
		return providerModelsIdentity{}, fmt.Errorf("harness not found: %s", name)
	default:
		return providerModelsIdentity{}, fmt.Errorf("invalid kind: %s", kind)
	}
}

// fetchProviderModelsLive performs model discovery only after current Fizeau
// membership has been validated. The request-scoped service and canonical
// identity are reused so a cache miss cannot observe a different registry.
func fetchProviderModelsLive(ctx context.Context, identity providerModelsIdentity) (*ProviderModelsResult, error) {
	filter := agentlib.ModelFilter{}
	if identity.kind == ProviderKindEndpoint {
		filter.Provider = identity.name
	} else {
		filter.Harness = identity.name
	}

	models, err := identity.service.ListModels(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	entries := make([]*ProviderModelEntry, 0, len(models))
	for _, m := range models {
		entry := &ProviderModelEntry{
			ID:           m.ID,
			Available:    m.Available,
			AutoRoutable: m.AutoRoutable,
		}
		if m.ContextLength > 0 {
			cl := m.ContextLength
			entry.ContextLength = &cl
		}
		entries = append(entries, entry)
	}

	return &ProviderModelsResult{
		Name:      identity.name,
		Kind:      identity.kind,
		Models:    entries,
		BaseURL:   identity.baseURL,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		FromCache: false,
	}, nil
}

func applyProviderModelsIdentity(result *ProviderModelsResult, identity providerModelsIdentity) {
	if result == nil {
		return
	}
	result.Name = identity.name
	result.Kind = identity.kind
	result.BaseURL = identity.baseURL
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

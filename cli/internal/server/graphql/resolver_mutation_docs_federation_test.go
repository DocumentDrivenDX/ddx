package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	"github.com/DocumentDrivenDX/ddx/internal/federation"
	"github.com/DocumentDrivenDX/ddx/internal/testutils"
)

// docWriteProjectFixture is a minimal initialized project for documentWrite
// federation tests. The library is rooted at the project root (library.path: ".").
type docWriteProjectFixture struct {
	root      string
	libPath   string
	projectID string
}

func newDocWriteProjectFixture(t *testing.T, projectID string) docWriteProjectFixture {
	t.Helper()
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	cfg := "version: \"1.0\"\nlibrary:\n  path: \".\"\n"
	if err := os.WriteFile(filepath.Join(root, ddxroot.DirName, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return docWriteProjectFixture{root: root, libPath: root, projectID: projectID}
}

func newDocWriteState(nodeID string, fixtures ...docWriteProjectFixture) *mutationTestStateProvider {
	projects := make([]*Project, 0, len(fixtures))
	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range fixtures {
		projects = append(projects, &Project{
			ID:           f.projectID,
			Name:         f.projectID,
			Path:         f.root,
			RegisteredAt: now,
			LastSeen:     now,
		})
	}
	return &mutationTestStateProvider{
		node: NodeStateSnapshot{
			ID:        nodeID,
			Name:      nodeID,
			StartedAt: time.Now().Add(-time.Minute),
			LastSeen:  time.Now(),
		},
		projects: projects,
	}
}

// docWriteFederation is an in-process FederationProvider that routes
// documentWrite ForwardMutation calls to spoke resolvers without real HTTP.
type docWriteFederation struct {
	spokes        []federation.SpokeRecord
	runtimes      map[string]*docWriteSpokeRuntime
	forwardCalls  int
	lastForwarded *federation.ForwardMutationRequest
}

type docWriteSpokeRuntime struct {
	resolver *mutationResolver
	fixture  docWriteProjectFixture
}

func newDocWriteSpokeRuntime(nodeID string, state *mutationTestStateProvider, fixture docWriteProjectFixture) *docWriteSpokeRuntime {
	return &docWriteSpokeRuntime{
		resolver: &mutationResolver{&Resolver{
			State:      state,
			WorkingDir: fixture.root,
			NodeID:     nodeID,
		}},
		fixture: fixture,
	}
}

func (f *docWriteFederation) Spokes() []federation.SpokeRecord {
	out := make([]federation.SpokeRecord, len(f.spokes))
	copy(out, f.spokes)
	return out
}

func (f *docWriteFederation) FanOut(_ context.Context, _ *federation.FanOutRequest) (*federation.FanOutResult, error) {
	return &federation.FanOutResult{
		Responses:     map[string]json.RawMessage{},
		Errors:        map[string]error{},
		Skipped:       map[string]federation.SkipReason{},
		StatusUpdates: map[string]federation.SpokeStatus{},
		Nodes:         []federation.NodeResult{},
	}, nil
}

func (f *docWriteFederation) ForwardMutation(ctx context.Context, req *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	if f == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}
	f.forwardCalls++
	if req != nil {
		cp := *req
		cp.ForwardingPath = append([]string(nil), req.ForwardingPath...)
		cp.Body = append([]byte(nil), req.Body...)
		if req.Headers != nil {
			cp.Headers = make(map[string]string, len(req.Headers))
			for k, v := range req.Headers {
				cp.Headers[k] = v
			}
		}
		f.lastForwarded = &cp
	}

	registry := federation.NewRegistry()
	for _, spoke := range f.spokes {
		if err := registry.UpsertSpoke(spoke); err != nil {
			return nil, err
		}
	}

	owner, err := federation.RouteMutationToProjectOwner(registry, req.TargetProjectID)
	if err != nil {
		if strings.Contains(err.Error(), "multiple registered owners") {
			return nil, federation.ErrForwardMutationBroadcastLike
		}
		return nil, federation.ErrForwardMutationMissingOwner
	}
	if owner.NodeID != req.TargetNodeID {
		return nil, federation.ErrForwardMutationBroadcastLike
	}
	if owner.Status == federation.StatusOffline {
		return nil, federation.ErrForwardMutationOffline
	}
	if owner.Status == federation.StatusStale {
		return nil, federation.ErrForwardMutationStale
	}
	if !beadWriteCapability(owner.Capabilities) {
		return nil, federation.ErrForwardMutationReadOnly
	}

	runtime := f.runtimes[owner.NodeID]
	if runtime == nil {
		return nil, federation.ErrForwardMutationMissingOwner
	}

	var envelope struct {
		Query     string                     `json:"query"`
		Variables map[string]json.RawMessage `json:"variables"`
	}
	if err := json.Unmarshal(req.Body, &envelope); err != nil {
		return nil, err
	}
	if !strings.Contains(envelope.Query, "documentWrite") {
		return nil, fmt.Errorf("unexpected mutation envelope query: %s", envelope.Query)
	}

	var path, content string
	if raw, ok := envelope.Variables["path"]; ok {
		if err := json.Unmarshal(raw, &path); err != nil {
			return nil, err
		}
	}
	if raw, ok := envelope.Variables["content"]; ok {
		if err := json.Unmarshal(raw, &content); err != nil {
			return nil, err
		}
	}

	// Inject loop-prevention header so the spoke writes locally.
	httpReq, _ := http.NewRequest(http.MethodPost, "/graphql", nil)
	if originID := req.Headers[beadMutationOriginServerHeader]; originID != "" {
		httpReq.Header.Set(beadMutationOriginServerHeader, originID)
	}
	spokeCtx := WithHTTPRequest(context.Background(), httpReq)
	spokeCtx = WithWorkingDir(spokeCtx, runtime.fixture.root)

	doc, writeErr := runtime.resolver.DocumentWrite(spokeCtx, path, content)
	if writeErr != nil {
		body, err := json.Marshal(map[string]any{
			"errors": []map[string]any{{"message": writeErr.Error()}},
		})
		if err != nil {
			return nil, err
		}
		return &federation.ForwardMutationResponse{
			OriginIdentity:  req.OriginIdentity,
			ForwardingPath:  append([]string(nil), req.ForwardingPath...),
			RequestID:       req.RequestID,
			IdempotencyKey:  req.IdempotencyKey,
			TargetNodeID:    req.TargetNodeID,
			TargetProjectID: req.TargetProjectID,
			StatusCode:      http.StatusOK,
			Headers:         http.Header{"Content-Type": []string{"application/json"}},
			Body:            body,
		}, nil
	}

	body, err := json.Marshal(map[string]any{
		"data": map[string]any{"documentWrite": doc},
	})
	if err != nil {
		return nil, err
	}
	return &federation.ForwardMutationResponse{
		OriginIdentity:  req.OriginIdentity,
		ForwardingPath:  append([]string(nil), req.ForwardingPath...),
		RequestID:       req.RequestID,
		IdempotencyKey:  req.IdempotencyKey,
		TargetNodeID:    req.TargetNodeID,
		TargetProjectID: req.TargetProjectID,
		StatusCode:      http.StatusOK,
		Headers:         http.Header{"Content-Type": []string{"application/json"}},
		Body:            body,
	}, nil
}

func hubLibraryDocPath(libPath, rel string) string {
	return filepath.Join(libPath, filepath.FromSlash(rel))
}

// TestFederatedDocumentWrite_ForwardsToOwner proves hub-mode documentWrite for
// a spoke-owned project routes through Federation.ForwardMutation to the
// owning spoke and never creates a hub-local phantom file. It also covers
// offline and missing-owner refusals (no hub-local write).
func TestFederatedDocumentWrite_ForwardsToOwner(t *testing.T) {
	t.Run("forwards to owner", func(t *testing.T) {
		hubFix := newDocWriteProjectFixture(t, "proj-a")
		spokeFix := newDocWriteProjectFixture(t, "proj-a")

		hubState := newDocWriteState("hub-node", hubFix)
		spokeState := newDocWriteState("spoke-node", spokeFix)

		fed := &docWriteFederation{
			spokes: []federation.SpokeRecord{
				{
					NodeID:       "spoke-node",
					Name:         "spoke-a",
					ProjectIDs:   []string{"proj-a"},
					Capabilities: []string{"read", "write"},
					Status:       federation.StatusActive,
				},
			},
			runtimes: map[string]*docWriteSpokeRuntime{
				"spoke-node": newDocWriteSpokeRuntime("spoke-node", spokeState, spokeFix),
			},
		}

		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubFix.root,
			Federation: fed,
			NodeID:     "hub-node",
		}}

		const relPath = "docs/helix/test.md"
		const content = "# Forwarded Write\n"

		doc, err := resolver.DocumentWrite(WithWorkingDir(context.Background(), hubFix.root), relPath, content)
		if err != nil {
			t.Fatalf("DocumentWrite: %v", err)
		}
		if doc == nil {
			t.Fatal("expected document payload")
		}
		if doc.Path != relPath && doc.ID != relPath {
			// Accept either path or id matching the relative path.
			if !strings.Contains(doc.Path, "test.md") && !strings.Contains(doc.ID, "test.md") {
				t.Fatalf("unexpected document path/id: path=%q id=%q", doc.Path, doc.ID)
			}
		}

		if fed.forwardCalls != 1 {
			t.Fatalf("ForwardMutation calls = %d, want 1", fed.forwardCalls)
		}
		if fed.lastForwarded == nil {
			t.Fatal("expected lastForwarded request")
		}
		if fed.lastForwarded.TargetNodeID != "spoke-node" {
			t.Fatalf("TargetNodeID = %q, want spoke-node", fed.lastForwarded.TargetNodeID)
		}
		if fed.lastForwarded.TargetProjectID != "proj-a" {
			t.Fatalf("TargetProjectID = %q, want proj-a", fed.lastForwarded.TargetProjectID)
		}
		if len(fed.lastForwarded.ForwardingPath) < 2 {
			t.Fatalf("ForwardingPath = %v, want hub + spoke", fed.lastForwarded.ForwardingPath)
		}
		if got := fed.lastForwarded.Headers[beadMutationOriginServerHeader]; got != "hub-node" {
			t.Fatalf("origin server header = %q, want hub-node", got)
		}
		if !strings.Contains(string(fed.lastForwarded.Body), "documentWrite") {
			t.Fatalf("forward body missing documentWrite: %s", fed.lastForwarded.Body)
		}

		// Spoke received the write under its library path.
		spokePath := hubLibraryDocPath(spokeFix.libPath, relPath)
		spokeBytes, readErr := os.ReadFile(spokePath)
		if readErr != nil {
			t.Fatalf("spoke library file missing: %v", readErr)
		}
		if string(spokeBytes) != content {
			t.Fatalf("spoke content = %q, want %q", string(spokeBytes), content)
		}

		// Hub library path must remain unchanged (no phantom local file).
		hubPath := hubLibraryDocPath(hubFix.libPath, relPath)
		if _, statErr := os.Stat(hubPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hub library path must stay unchanged, stat(%s)=%v", hubPath, statErr)
		}
	})

	t.Run("offline refuses without hub-local write", func(t *testing.T) {
		hubFix := newDocWriteProjectFixture(t, "proj-a")
		spokeFix := newDocWriteProjectFixture(t, "proj-a")
		hubState := newDocWriteState("hub-node", hubFix)
		spokeState := newDocWriteState("spoke-node", spokeFix)

		fed := &docWriteFederation{
			spokes: []federation.SpokeRecord{
				{
					NodeID:       "spoke-node",
					Name:         "spoke-a",
					ProjectIDs:   []string{"proj-a"},
					Capabilities: []string{"read", "write"},
					Status:       federation.StatusOffline,
				},
			},
			runtimes: map[string]*docWriteSpokeRuntime{
				"spoke-node": newDocWriteSpokeRuntime("spoke-node", spokeState, spokeFix),
			},
		}
		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubFix.root,
			Federation: fed,
			NodeID:     "hub-node",
		}}

		const relPath = "docs/helix/offline.md"
		_, err := resolver.DocumentWrite(WithWorkingDir(context.Background(), hubFix.root), relPath, "# offline\n")
		if !errors.Is(err, federation.ErrForwardMutationOffline) {
			t.Fatalf("offline error = %v, want offline", err)
		}
		hubPath := hubLibraryDocPath(hubFix.libPath, relPath)
		if _, statErr := os.Stat(hubPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hub-local phantom on offline: %v", statErr)
		}
		spokePath := hubLibraryDocPath(spokeFix.libPath, relPath)
		if _, statErr := os.Stat(spokePath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("spoke must not write on offline: %v", statErr)
		}
	})

	t.Run("missing owner refuses without hub-local write", func(t *testing.T) {
		hubFix := newDocWriteProjectFixture(t, "proj-a")
		hubState := newDocWriteState("hub-node", hubFix)
		resolver := &mutationResolver{&Resolver{
			State:      hubState,
			WorkingDir: hubFix.root,
			Federation: &docWriteFederation{},
			NodeID:     "hub-node",
		}}

		const relPath = "docs/helix/missing-owner.md"
		_, err := resolver.DocumentWrite(WithWorkingDir(context.Background(), hubFix.root), relPath, "# missing\n")
		if !errors.Is(err, federation.ErrForwardMutationMissingOwner) {
			t.Fatalf("missing-owner error = %v, want missing-owner", err)
		}
		hubPath := hubLibraryDocPath(hubFix.libPath, relPath)
		if _, statErr := os.Stat(hubPath); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("hub-local phantom on missing-owner: %v", statErr)
		}
	})
}

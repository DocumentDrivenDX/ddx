package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/stretchr/testify/require"
)

// docWriteProjectFixture is a minimal initialized project for document write
// federation tests. The library is rooted at the project root (library.path: ".").
type docWriteProjectFixture struct {
	root      string
	projectID string
}

func newDocWriteProjectFixture(t *testing.T, projectID string) docWriteProjectFixture {
	t.Helper()
	root := t.TempDir()
	testutils.MakeInitializedDDxRoot(t, root)
	cfg := "version: \"1.0\"\nlibrary:\n  path: \".\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, ddxroot.DirName, "config.yaml"), []byte(cfg), 0o644))
	return docWriteProjectFixture{root: root, projectID: projectID}
}

func newDocWriteState(nodeID string, fixtures ...docWriteProjectFixture) *mutationTestStateProvider {
	projects := make([]*Project, len(fixtures))
	now := time.Now().UTC().Format(time.RFC3339)
	for i, f := range fixtures {
		projects[i] = &Project{
			ID:           f.projectID,
			Name:         f.projectID,
			Path:         f.root,
			RegisteredAt: now,
			LastSeen:     now,
		}
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

// docWriteFederation is a test-only FederationProvider for document write
// federation tests. It routes documentWrite mutations to the appropriate spoke
// resolver in-process.
type docWriteFederation struct {
	spokes   []federation.SpokeRecord
	runtimes map[string]*docWriteSpokeRuntime
}

type docWriteSpokeRuntime struct {
	resolver *mutationResolver
	fixture  docWriteProjectFixture
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

	registry := federation.NewRegistry()
	registry.Spokes = append(registry.Spokes, f.spokes...)

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
	if !hasWriteCapability(owner.Capabilities) {
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
		return nil, fmt.Errorf("docWriteFederation: decode body: %w", err)
	}

	var path, content string
	var expectedHash *string
	if raw, ok := envelope.Variables["path"]; ok {
		_ = json.Unmarshal(raw, &path)
	}
	if raw, ok := envelope.Variables["content"]; ok {
		_ = json.Unmarshal(raw, &content)
	}
	if raw, ok := envelope.Variables["expectedHash"]; ok {
		var h string
		if err := json.Unmarshal(raw, &h); err == nil {
			expectedHash = &h
		}
	}

	// Inject the origin server ID header so the spoke detects a forwarded request
	// and writes locally rather than re-routing (loop prevention).
	httpReq, _ := http.NewRequest("POST", "/graphql", nil)
	if originID := req.Headers["X-DDx-Origin-Server-ID"]; originID != "" {
		httpReq.Header.Set("X-DDx-Origin-Server-ID", originID)
	}
	spokeCtx := WithHTTPRequest(context.Background(), httpReq)
	spokeCtx = WithWorkingDir(spokeCtx, runtime.fixture.root)

	doc, writeErr := runtime.resolver.DocumentWrite(spokeCtx, path, content, expectedHash)

	var body []byte
	if writeErr != nil {
		ext := map[string]any{}
		if strings.Contains(writeErr.Error(), "conflict") {
			ext["code"] = "DOCUMENT_WRITE_CONFLICT"
			ext["status"] = 409
		}
		body, _ = json.Marshal(map[string]any{
			"errors": []map[string]any{
				{"message": writeErr.Error(), "extensions": ext},
			},
		})
	} else {
		body, _ = json.Marshal(map[string]any{
			"data": map[string]any{"documentWrite": doc},
		})
	}

	return &federation.ForwardMutationResponse{
		OriginIdentity:  req.OriginIdentity,
		ForwardingPath:  append([]string(nil), req.ForwardingPath...),
		TargetNodeID:    req.TargetNodeID,
		TargetProjectID: req.TargetProjectID,
		StatusCode:      http.StatusOK,
		Headers:         http.Header{"Content-Type": []string{"application/json"}},
		Body:            body,
	}, nil
}

// trackingDocWriteFederation records whether ForwardMutation was called.
// Used to assert that loop-prevention prevents re-forwarding.
type trackingDocWriteFederation struct {
	spokes          []federation.SpokeRecord
	onForwardCalled func()
}

func (f *trackingDocWriteFederation) Spokes() []federation.SpokeRecord { return f.spokes }
func (f *trackingDocWriteFederation) FanOut(_ context.Context, _ *federation.FanOutRequest) (*federation.FanOutResult, error) {
	return &federation.FanOutResult{}, nil
}
func (f *trackingDocWriteFederation) ForwardMutation(_ context.Context, _ *federation.ForwardMutationRequest) (*federation.ForwardMutationResponse, error) {
	if f.onForwardCalled != nil {
		f.onForwardCalled()
	}
	return nil, fmt.Errorf("unexpected forward mutation call — forwarding loop not prevented")
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestFederatedDocumentWrite_ForwardsToOwner proves hub documentWrite(path,
// content, expectedHash) for a spoke-owned registered project persists on the
// owning spoke and does not create or overwrite a hub-local file.
func TestFederatedDocumentWrite_ForwardsToOwner(t *testing.T) {
	hubFix := newDocWriteProjectFixture(t, "proj-a")
	spokeFix := newDocWriteProjectFixture(t, "proj-a")

	hubState := newDocWriteState("hub-node", hubFix)
	spokeState := newDocWriteState("spoke-node", spokeFix)

	spokeResolver := &mutationResolver{&Resolver{
		State:      spokeState,
		WorkingDir: spokeFix.root,
		NodeID:     "spoke-node",
	}}

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
			"spoke-node": {resolver: spokeResolver, fixture: spokeFix},
		},
	}

	hubResolver := &mutationResolver{&Resolver{
		State:      hubState,
		WorkingDir: hubFix.root,
		Federation: fed,
		NodeID:     "hub-node",
	}}

	ctx := WithWorkingDir(context.Background(), hubFix.root)
	doc, err := hubResolver.DocumentWrite(ctx, "docs/test.md", "# Forwarded Write", nil)
	require.NoError(t, err)
	require.NotNil(t, doc)

	// File persists on the spoke.
	spokePath := filepath.Join(spokeFix.root, "docs", "test.md")
	spokeContent, readErr := os.ReadFile(spokePath)
	require.NoError(t, readErr)
	require.Equal(t, "# Forwarded Write", string(spokeContent))

	// Hub must NOT have a local copy.
	hubPath := filepath.Join(hubFix.root, "docs", "test.md")
	_, statErr := os.Stat(hubPath)
	require.True(t, errors.Is(statErr, os.ErrNotExist), "hub should not have local file for spoke-owned project")
}

// TestFederatedDocumentWrite_PreservesExpectedHashConflict proves stale
// expectedHash failures are returned from the owning spoke without overwriting
// spoke content.
func TestFederatedDocumentWrite_PreservesExpectedHashConflict(t *testing.T) {
	hubFix := newDocWriteProjectFixture(t, "proj-a")
	spokeFix := newDocWriteProjectFixture(t, "proj-a")

	// Pre-populate the spoke with a known file.
	origContent := "# Original Content\n"
	spokePath := filepath.Join(spokeFix.root, "docs", "test.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(spokePath), 0o755))
	require.NoError(t, os.WriteFile(spokePath, []byte(origContent), 0o644))

	// Compute hash of the original content.
	origHash := sha256Hex([]byte(origContent))

	// Concurrently update the spoke's file so the hash is now stale.
	newContent := "# Updated by concurrent writer\n"
	require.NoError(t, os.WriteFile(spokePath, []byte(newContent), 0o644))

	hubState := newDocWriteState("hub-node", hubFix)
	spokeState := newDocWriteState("spoke-node", spokeFix)

	spokeResolver := &mutationResolver{&Resolver{
		State:      spokeState,
		WorkingDir: spokeFix.root,
		NodeID:     "spoke-node",
	}}

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
			"spoke-node": {resolver: spokeResolver, fixture: spokeFix},
		},
	}

	hubResolver := &mutationResolver{&Resolver{
		State:      hubState,
		WorkingDir: hubFix.root,
		Federation: fed,
		NodeID:     "hub-node",
	}}

	ctx := WithWorkingDir(context.Background(), hubFix.root)
	_, err := hubResolver.DocumentWrite(ctx, "docs/test.md", "# My Overwrite", &origHash)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "conflict")

	// Spoke content must be unchanged.
	body, readErr := os.ReadFile(spokePath)
	require.NoError(t, readErr)
	require.Equal(t, newContent, string(body), "spoke content must not be overwritten on conflict")
}

// TestFederatedDocumentWrite_RejectsForwardingLoop proves originServerID/
// forwarded metadata prevents recursive hub-spoke forwarding.
func TestFederatedDocumentWrite_RejectsForwardingLoop(t *testing.T) {
	spokeFix := newDocWriteProjectFixture(t, "proj-loop")
	spokeState := newDocWriteState("spoke-node", spokeFix)

	// Wire the spoke with a federation that would loop back toward a "meta-hub".
	// Without loop prevention, the spoke would try to re-forward to meta-hub-node.
	forwardCalled := false
	loopFed := &trackingDocWriteFederation{
		spokes: []federation.SpokeRecord{
			{
				NodeID:       "meta-hub-node",
				ProjectIDs:   []string{"proj-loop"},
				Capabilities: []string{"write"},
				Status:       federation.StatusActive,
			},
		},
		onForwardCalled: func() { forwardCalled = true },
	}

	spokeResolver := &mutationResolver{&Resolver{
		State:      spokeState,
		WorkingDir: spokeFix.root,
		Federation: loopFed,
		NodeID:     "spoke-node",
	}}

	// Simulate a request that was already forwarded from hub-node.
	// The X-DDx-Origin-Server-ID header is the loop-prevention signal.
	httpReq, _ := http.NewRequest("POST", "/graphql", nil)
	httpReq.Header.Set("X-DDx-Origin-Server-ID", "hub-node")
	ctx := WithHTTPRequest(context.Background(), httpReq)
	ctx = WithWorkingDir(ctx, spokeFix.root)

	_, err := spokeResolver.DocumentWrite(ctx, "docs/test.md", "# Local Write", nil)
	require.NoError(t, err)

	// Spoke must have written locally — ForwardMutation must NOT have been called.
	require.False(t, forwardCalled, "forwarding loop was not prevented: ForwardMutation was called on the spoke")

	spokePath := filepath.Join(spokeFix.root, "docs", "test.md")
	body, readErr := os.ReadFile(spokePath)
	require.NoError(t, readErr)
	require.Equal(t, "# Local Write", string(body))
}

// TestFederatedDocumentWrite_UsesRegisteredProjectOwner proves forwarding
// chooses the owner from the federation/project registry rather than from a
// filesystem path heuristic.
func TestFederatedDocumentWrite_UsesRegisteredProjectOwner(t *testing.T) {
	hubFix := newDocWriteProjectFixture(t, "proj-a")
	spoke1Fix := newDocWriteProjectFixture(t, "proj-b") // not the owner of proj-a
	spoke2Fix := newDocWriteProjectFixture(t, "proj-a") // registry-registered owner of proj-a

	hubState := newDocWriteState("hub-node", hubFix)
	spoke1State := newDocWriteState("spoke-1", spoke1Fix)
	spoke2State := newDocWriteState("spoke-2", spoke2Fix)

	spoke1Resolver := &mutationResolver{&Resolver{State: spoke1State, WorkingDir: spoke1Fix.root, NodeID: "spoke-1"}}
	spoke2Resolver := &mutationResolver{&Resolver{State: spoke2State, WorkingDir: spoke2Fix.root, NodeID: "spoke-2"}}

	fed := &docWriteFederation{
		spokes: []federation.SpokeRecord{
			{
				NodeID:       "spoke-1",
				Name:         "spoke-one",
				ProjectIDs:   []string{"proj-b"},
				Capabilities: []string{"read", "write"},
				Status:       federation.StatusActive,
			},
			{
				NodeID:       "spoke-2",
				Name:         "spoke-two",
				ProjectIDs:   []string{"proj-a"},
				Capabilities: []string{"read", "write"},
				Status:       federation.StatusActive,
			},
		},
		runtimes: map[string]*docWriteSpokeRuntime{
			"spoke-1": {resolver: spoke1Resolver, fixture: spoke1Fix},
			"spoke-2": {resolver: spoke2Resolver, fixture: spoke2Fix},
		},
	}

	hubResolver := &mutationResolver{&Resolver{
		State:      hubState,
		WorkingDir: hubFix.root,
		Federation: fed,
		NodeID:     "hub-node",
	}}

	ctx := WithWorkingDir(context.Background(), hubFix.root)
	_, err := hubResolver.DocumentWrite(ctx, "docs/test.md", "# Write via registry", nil)
	require.NoError(t, err)

	// The write must land on spoke-2 (registry-registered owner of proj-a).
	spoke2Path := filepath.Join(spoke2Fix.root, "docs", "test.md")
	content, readErr := os.ReadFile(spoke2Path)
	require.NoError(t, readErr)
	require.Equal(t, "# Write via registry", string(content))

	// The write must NOT land on spoke-1.
	spoke1Path := filepath.Join(spoke1Fix.root, "docs", "test.md")
	_, statErr := os.Stat(spoke1Path)
	require.True(t, errors.Is(statErr, os.ErrNotExist),
		"spoke-1 must not receive writes intended for proj-a (spoke-2 is the registry owner)")
}

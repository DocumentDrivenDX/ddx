package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// newArtifactGQLHandler builds a GQL handler whose workDir is the project root.
func newArtifactGQLHandler(workDir string, store *bead.Store) http.Handler {
	state := newTestStateProvider(workDir, store)
	return newGQLHandler(state, workDir, nil)
}

func TestArtifacts_ListAll(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\") { totalCount edges { node { id path title mediaType staleness } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}

	data := result["data"].(map[string]interface{})
	artifacts := data["artifacts"].(map[string]interface{})
	total := int(artifacts["totalCount"].(float64))
	if total < 3 {
		t.Errorf("expected at least 3 artifacts (1 markdown + 2 sidecar), got %d", total)
	}

	edges := artifacts["edges"].([]interface{})
	mediaTypes := map[string]bool{}
	for _, e := range edges {
		edge := e.(map[string]interface{})
		node := edge["node"].(map[string]interface{})
		mt := node["mediaType"].(string)
		mediaTypes[mt] = true
	}
	if !mediaTypes["text/markdown"] {
		t.Error("expected at least one text/markdown artifact")
	}
	if !mediaTypes["image/png"] {
		t.Error("expected at least one image/png artifact")
	}
	if !mediaTypes["application/octet-stream"] {
		t.Error("expected at least one application/octet-stream artifact")
	}
}

func TestArtifacts_MediaTypeFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", mediaType: \"image/png\") { totalCount edges { node { id mediaType } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}

	data := result["data"].(map[string]interface{})
	artifacts := data["artifacts"].(map[string]interface{})
	total := int(artifacts["totalCount"].(float64))
	if total != 1 {
		t.Errorf("expected 1 image/png artifact, got %d", total)
	}

	edges := artifacts["edges"].([]interface{})
	for _, e := range edges {
		edge := e.(map[string]interface{})
		node := edge["node"].(map[string]interface{})
		mt := node["mediaType"].(string)
		if mt != "image/png" {
			t.Errorf("mediaType filter returned non-image artifact: %s", mt)
		}
	}
}

func TestArtifacts_SearchFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	// Search for "diagram" — should match the diagram.png artifact.
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", search: \"diagram\") { totalCount edges { node { id title path } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}

	data := result["data"].(map[string]interface{})
	artifacts := data["artifacts"].(map[string]interface{})
	total := int(artifacts["totalCount"].(float64))
	if total == 0 {
		t.Error("search for 'diagram' returned no results")
	}
	edges := artifacts["edges"].([]interface{})
	found := false
	for _, e := range edges {
		edge := e.(map[string]interface{})
		node := edge["node"].(map[string]interface{})
		title := node["title"].(string)
		path := node["path"].(string)
		if title == "Architecture Diagram" || containsCI(path, "diagram") {
			found = true
		}
	}
	if !found {
		t.Error("search for 'diagram' did not return the diagram artifact")
	}
}

func TestArtifacts_MediaTypeWildcard(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", mediaType: \"image/*\") { totalCount edges { node { mediaType } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	data := result["data"].(map[string]interface{})
	artifacts := data["artifacts"].(map[string]interface{})
	total := int(artifacts["totalCount"].(float64))
	if total < 1 {
		t.Errorf("expected wildcard image/* to match at least 1 artifact, got %d", total)
	}
	for _, e := range artifacts["edges"].([]interface{}) {
		mt := e.(map[string]interface{})["node"].(map[string]interface{})["mediaType"].(string)
		if !bytes.HasPrefix([]byte(mt), []byte("image/")) {
			t.Errorf("wildcard image/* matched unrelated mediaType: %s", mt)
		}
	}
}

func TestArtifacts_StalenessFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", staleness: \"fresh\") { totalCount edges { node { staleness } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	data := result["data"].(map[string]interface{})
	artifacts := data["artifacts"].(map[string]interface{})
	for _, e := range artifacts["edges"].([]interface{}) {
		st := e.(map[string]interface{})["node"].(map[string]interface{})["staleness"].(string)
		if st != "fresh" {
			t.Errorf("staleness filter returned non-fresh artifact: %s", st)
		}
	}
}

func TestArtifacts_SortByTitleStable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", sort: TITLE) { edges { node { id title } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	data := result["data"].(map[string]interface{})
	edges := data["artifacts"].(map[string]interface{})["edges"].([]interface{})
	var prevTitle, prevID string
	for i, e := range edges {
		node := e.(map[string]interface{})["node"].(map[string]interface{})
		title := node["title"].(string)
		id := node["id"].(string)
		if i > 0 {
			if title < prevTitle {
				t.Errorf("titles not sorted ascending at index %d: %q before %q", i, prevTitle, title)
			}
			if title == prevTitle && id < prevID {
				t.Errorf("tie-breaker by id violated at index %d", i)
			}
		}
		prevTitle, prevID = title, id
	}
}

// TestArtifacts_SortAllValuesTieBreak runs every ArtifactSort enum value
// through the resolver and asserts that ties on the primary key fall back to
// ascending id ordering — the contract that makes cursor pagination
// deterministic when filters or sorts change.
func TestArtifacts_SortAllValuesTieBreak(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	type sortKey struct {
		name   string
		field  string // node field used as the primary sort key for the assertion
		gqlVal string
	}
	keys := []sortKey{
		{"ID", "id", "ID"},
		{"PATH", "path", "PATH"},
		{"TITLE", "title", "TITLE"},
		{"MODIFIED", "updatedAt", "MODIFIED"},
		{"DEPS_COUNT", "id", "DEPS_COUNT"}, // depsCount is not exposed; verify only the id tie-breaker
	}

	for _, k := range keys {
		t.Run(k.name, func(t *testing.T) {
			body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", sort: ` + k.gqlVal + `) { edges { node { id path title updatedAt } } } }"}`)
			resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if errs, ok := result["errors"]; ok {
				t.Fatalf("GraphQL errors: %v", errs)
			}
			edges := result["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["edges"].([]interface{})
			if len(edges) < 2 {
				t.Fatalf("need ≥2 artifacts to verify ordering, got %d", len(edges))
			}

			var prevPrimary, prevID string
			for i, e := range edges {
				node := e.(map[string]interface{})["node"].(map[string]interface{})
				id := node["id"].(string)
				var primary string
				if v, ok := node[k.field]; ok && v != nil {
					primary, _ = v.(string)
				}
				if i > 0 {
					if primary < prevPrimary {
						t.Errorf("sort=%s: primary key not ascending at index %d: %q before %q", k.name, i, prevPrimary, primary)
					}
					if primary == prevPrimary && id < prevID {
						t.Errorf("sort=%s: id tie-breaker violated at index %d: %q before %q", k.name, i, prevID, id)
					}
				}
				prevPrimary, prevID = primary, id
			}
		})
	}
}

func TestArtifacts_MissingSidecarGraceful(t *testing.T) {
	// Verifies the resolver does not crash when the sidecar directory is absent.
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	// No fixture files — .ddx/plugins does not exist.

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\") { totalCount } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("resolver crashed on missing sidecars: %v", errs)
	}
}

func TestArtifacts_Pagination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	// first: 1 should return exactly one artifact.
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", first: 1) { totalCount pageInfo { hasNextPage } edges { node { id } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}

	data := result["data"].(map[string]interface{})
	artifacts := data["artifacts"].(map[string]interface{})
	edges := artifacts["edges"].([]interface{})
	if len(edges) != 1 {
		t.Errorf("expected 1 edge with first:1, got %d", len(edges))
	}
	pi := artifacts["pageInfo"].(map[string]interface{})
	if !pi["hasNextPage"].(bool) {
		t.Error("expected hasNextPage=true with first:1 and 3+ artifacts")
	}
}

// setupArtifactFixtureInDir populates the given workDir with the artifact fixture.
// It returns the same workDir (for call chaining).
func setupArtifactFixtureInDir(t *testing.T, workDir string) string {
	t.Helper()

	// Markdown document with DDx frontmatter.
	docsDir := filepath.Join(workDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "spec.md"), []byte(
		"---\nddx:\n  id: spec-001\n  title: Project Spec\n---\n# Project Spec\n\nContent here.\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	// Image artifact with .ddx.yaml sidecar.
	assetsDir := filepath.Join(workDir, ".ddx", "plugins", "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pngData := []byte("\x89PNG\r\n\x1a\n")
	if err := os.WriteFile(filepath.Join(assetsDir, "diagram.png"), pngData, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "diagram.png.ddx.yaml"), []byte(
		"ddx:\n  id: diagram-001\n  title: Architecture Diagram\n  media_type: image/png\n  description: System architecture\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	// Unknown-type artifact (no media_type → application/octet-stream).
	if err := os.WriteFile(filepath.Join(assetsDir, "data.bin"), []byte("binary data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "data.bin.ddx.yaml"), []byte(
		"ddx:\n  id: data-001\n  title: Binary Data\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	return workDir
}

// setupPhasePrefixFixtureInDir adds HELIX-style phase docs to exercise the
// phase + prefix filters. Returns the workDir for chaining.
func setupPhasePrefixFixtureInDir(t *testing.T, workDir string) string {
	t.Helper()

	frame := filepath.Join(workDir, "docs", "helix", "01-frame")
	design := filepath.Join(workDir, "docs", "helix", "02-design")
	for _, d := range []string{frame, design} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(frame, "PRD.md"), []byte(
		"---\nddx:\n  id: PRD-100\n  title: Product Requirements\n---\n# PRD\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(frame, "FEAT-200.md"), []byte(
		"---\nddx:\n  id: FEAT-200\n  title: Search feature\n---\n# Feature\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(design, "ADR-300.md"), []byte(
		"---\nddx:\n  id: ADR-300\n  title: Storage decision\n---\n# ADR\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	return workDir
}

func TestArtifacts_PhaseFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupPhasePrefixFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", phase: \"01-frame\") { totalCount edges { node { id path } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	artifacts := result["data"].(map[string]interface{})["artifacts"].(map[string]interface{})
	if total := int(artifacts["totalCount"].(float64)); total != 2 {
		t.Errorf("phase=01-frame: expected 2, got %d", total)
	}
	for _, e := range artifacts["edges"].([]interface{}) {
		path := e.(map[string]interface{})["node"].(map[string]interface{})["path"].(string)
		if !containsCI(path, "docs/helix/01-frame/") {
			t.Errorf("phase filter returned out-of-phase path: %s", path)
		}
	}

	// Numeric prefix form: phase=01 should also match docs/helix/01-frame/.
	body = bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", phase: \"01\") { totalCount } }"}`)
	resp2, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var r2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&r2); err != nil {
		t.Fatal(err)
	}
	if total := int(r2["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["totalCount"].(float64)); total != 2 {
		t.Errorf("phase=01: expected 2, got %d", total)
	}
}

func TestArtifacts_PrefixFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupPhasePrefixFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	// Single prefix.
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", prefix: [\"ADR\"]) { totalCount edges { node { id } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var r map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if errs, ok := r["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	artifacts := r["data"].(map[string]interface{})["artifacts"].(map[string]interface{})
	if total := int(artifacts["totalCount"].(float64)); total != 1 {
		t.Errorf("prefix=[ADR]: expected 1, got %d", total)
	}
	for _, e := range artifacts["edges"].([]interface{}) {
		id := e.(map[string]interface{})["node"].(map[string]interface{})["id"].(string)
		if !containsCI(id, "ADR-") {
			t.Errorf("prefix filter returned non-ADR id: %s", id)
		}
	}

	// Multi prefix OR.
	body = bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", prefix: [\"ADR\",\"FEAT\"]) { totalCount } }"}`)
	resp2, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	var r2 map[string]interface{}
	if err := json.NewDecoder(resp2.Body).Decode(&r2); err != nil {
		t.Fatal(err)
	}
	if total := int(r2["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["totalCount"].(float64)); total != 2 {
		t.Errorf("prefix=[ADR,FEAT]: expected 2, got %d", total)
	}
}

func TestArtifacts_PhasePrefixComposeWithOtherFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupPhasePrefixFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	// phase=01-frame AND prefix=[FEAT] should narrow to one.
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", phase: \"01-frame\", prefix: [\"FEAT\"], mediaType: \"text/markdown\", staleness: \"fresh\", sort: TITLE) { totalCount edges { node { id } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var r map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		t.Fatal(err)
	}
	if errs, ok := r["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}
	artifacts := r["data"].(map[string]interface{})["artifacts"].(map[string]interface{})
	if total := int(artifacts["totalCount"].(float64)); total != 1 {
		t.Errorf("composed filter: expected 1, got %d", total)
	}
	edges := artifacts["edges"].([]interface{})
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	id := edges[0].(map[string]interface{})["node"].(map[string]interface{})["id"].(string)
	if !containsCI(id, "FEAT-") {
		t.Errorf("expected FEAT artifact, got %s", id)
	}
}

func containsCI(s, sub string) bool {
	return bytes.Contains(
		bytes.ToLower([]byte(s)),
		bytes.ToLower([]byte(sub)),
	)
}

func TestArtifact_DetailWithContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	// Query the markdown artifact by fetching the list first to get its ID.
	listBody := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", mediaType: \"text/markdown\") { edges { node { id } } } }"}`)
	listResp, err := http.Post(srv.URL+"/graphql", "application/json", listBody)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	var listResult map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatal(err)
	}
	if errs, ok := listResult["errors"]; ok {
		t.Fatalf("list GraphQL errors: %v", errs)
	}
	listData := listResult["data"].(map[string]interface{})
	edges := listData["artifacts"].(map[string]interface{})["edges"].([]interface{})
	if len(edges) == 0 {
		t.Fatal("expected at least one markdown artifact")
	}
	artifactID := edges[0].(map[string]interface{})["node"].(map[string]interface{})["id"].(string)

	// Now fetch detail via artifact(projectID, id).
	detailBody := bytes.NewBufferString(`{"query":"{ artifact(projectID: \"` + projID + `\", id: \"` + artifactID + `\") { id path mediaType content ddxFrontmatter } }"}`)
	detailResp, err := http.Post(srv.URL+"/graphql", "application/json", detailBody)
	if err != nil {
		t.Fatal(err)
	}
	defer detailResp.Body.Close()

	var detailResult map[string]interface{}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailResult); err != nil {
		t.Fatal(err)
	}
	if errs, ok := detailResult["errors"]; ok {
		t.Fatalf("detail GraphQL errors: %v", errs)
	}

	detail := detailResult["data"].(map[string]interface{})["artifact"].(map[string]interface{})
	if detail["mediaType"].(string) != "text/markdown" {
		t.Errorf("expected text/markdown, got %s", detail["mediaType"])
	}
	content, hasContent := detail["content"]
	if !hasContent || content == nil {
		t.Error("expected content field to be populated for text/markdown artifact")
	}
	if ddxFM, ok := detail["ddxFrontmatter"]; !ok || ddxFM == nil {
		t.Error("expected ddxFrontmatter field to be populated")
	}
}

func TestArtifact_DetailBinaryNoContent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	// Get the image/png artifact ID.
	listBody := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", mediaType: \"image/png\") { edges { node { id } } } }"}`)
	listResp, err := http.Post(srv.URL+"/graphql", "application/json", listBody)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()

	var listResult map[string]interface{}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatal(err)
	}
	if errs, ok := listResult["errors"]; ok {
		t.Fatalf("list GraphQL errors: %v", errs)
	}
	edges := listResult["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["edges"].([]interface{})
	if len(edges) == 0 {
		t.Fatal("expected at least one image/png artifact")
	}
	artifactID := edges[0].(map[string]interface{})["node"].(map[string]interface{})["id"].(string)

	detailBody := bytes.NewBufferString(`{"query":"{ artifact(projectID: \"` + projID + `\", id: \"` + artifactID + `\") { id mediaType content } }"}`)
	detailResp, err := http.Post(srv.URL+"/graphql", "application/json", detailBody)
	if err != nil {
		t.Fatal(err)
	}
	defer detailResp.Body.Close()

	var detailResult map[string]interface{}
	if err := json.NewDecoder(detailResp.Body).Decode(&detailResult); err != nil {
		t.Fatal(err)
	}
	if errs, ok := detailResult["errors"]; ok {
		t.Fatalf("detail GraphQL errors: %v", errs)
	}

	detail := detailResult["data"].(map[string]interface{})["artifact"].(map[string]interface{})
	if content := detail["content"]; content != nil {
		t.Errorf("expected content to be null for binary artifact, got %v", content)
	}
}

func TestArtifact_DetailNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"{ artifact(projectID: \"` + projID + `\", id: \"nonexistent-id\") { id } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("GraphQL errors: %v", errs)
	}

	if artifact := result["data"].(map[string]interface{})["artifact"]; artifact != nil {
		t.Errorf("expected null for unknown artifact, got %v", artifact)
	}
}

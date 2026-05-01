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

package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestArtifactRegenerate_NoGeneratorMetadata asserts that artifacts without a
// generated_by sidecar block return a typed NO_GENERATOR_METADATA error rather
// than crashing or silently succeeding. This is what lets the UI keep its
// Regenerate button disabled with an explanatory tooltip (FEAT-008 US-081b).
func TestArtifactRegenerate_NoGeneratorMetadata(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)

	// data-001 is a binary sidecar artifact with no generated_by block.
	listBody := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\") { edges { node { id generatedBy { runId } } } } }"}`)
	listResp, err := http.Post(srv.URL+"/graphql", "application/json", listBody)
	if err != nil {
		t.Fatal(err)
	}
	defer listResp.Body.Close()
	var listResult map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatal(err)
	}
	var targetID string
	for _, e := range listResult["data"].(map[string]any)["artifacts"].(map[string]any)["edges"].([]any) {
		node := e.(map[string]any)["node"].(map[string]any)
		if node["generatedBy"] == nil {
			targetID = node["id"].(string)
			break
		}
	}
	if targetID == "" {
		t.Fatal("expected at least one artifact without generatedBy")
	}

	body := bytes.NewBufferString(`{"query":"mutation { artifactRegenerate(artifactId: \"` + targetID + `\") { runId status } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	errs, ok := result["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected typed error, got result=%v", result)
	}
	first := errs[0].(map[string]any)
	exts, _ := first["extensions"].(map[string]any)
	if exts["code"] != "NO_GENERATOR_METADATA" {
		t.Errorf("expected extensions.code=NO_GENERATOR_METADATA, got %v", exts)
	}
}

// TestArtifactRegenerate_SuccessDispatches asserts that when an artifact has a
// generated_by sidecar block, the mutation returns a runId and a queued status
// and persists a regeneration record under .ddx/artifact-regenerations/.
func TestArtifactRegenerate_SuccessDispatches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupArtifactFixtureInDir(t, workDir)

	// Add an artifact with a generated_by block so the resolver can dispatch.
	assetsDir := filepath.Join(workDir, ".ddx", "plugins", "assets")
	if err := os.WriteFile(filepath.Join(assetsDir, "report.md"), []byte("# Report\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "report.md.ddx.yaml"), []byte(
		"ddx:\n  id: report-001\n  title: Generated Report\n  media_type: text/markdown\n  generated_by:\n    run_id: run-abc\n    prompt_summary: synthesize report\n    source_hash: deadbeef\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	body := bytes.NewBufferString(`{"query":"mutation { artifactRegenerate(artifactId: \"report-001\") { runId status } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if errs, ok := result["errors"]; ok {
		t.Fatalf("unexpected GraphQL errors: %v", errs)
	}
	out := result["data"].(map[string]any)["artifactRegenerate"].(map[string]any)
	runID, _ := out["runId"].(string)
	status, _ := out["status"].(string)
	if runID == "" {
		t.Error("expected non-empty runId")
	}
	if status != "queued" {
		t.Errorf("expected status=queued, got %q", status)
	}

	// The dispatch record should have been persisted.
	matches, err := filepath.Glob(filepath.Join(workDir, ".ddx", "artifact-regenerations", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Error("expected at least one persisted regeneration record under .ddx/artifact-regenerations/")
	}
	_ = projID
}

// TestArtifactRegenerate_ArtifactNotFound asserts that an unknown artifact ID
// returns a typed ARTIFACT_NOT_FOUND error rather than panicking.
func TestArtifactRegenerate_ArtifactNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	body := bytes.NewBufferString(`{"query":"mutation { artifactRegenerate(artifactId: \"nope\") { runId status } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	errs, ok := result["errors"].([]any)
	if !ok || len(errs) == 0 {
		t.Fatalf("expected an error, got %v", result)
	}
	exts, _ := errs[0].(map[string]any)["extensions"].(map[string]any)
	if exts["code"] != "ARTIFACT_NOT_FOUND" {
		t.Errorf("expected ARTIFACT_NOT_FOUND, got %v", exts)
	}
}

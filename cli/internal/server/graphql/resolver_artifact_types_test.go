package graphql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

type gqlArtifactTypeFile struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	IsTruncated bool   `json:"isTruncated"`
	SizeBytes   int    `json:"sizeBytes"`
}

type gqlArtifactTypeExample struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Content     string `json:"content"`
	IsTruncated bool   `json:"isTruncated"`
	SizeBytes   int    `json:"sizeBytes"`
}

type gqlArtifactTypeDefinition struct {
	Plugin         string                   `json:"plugin"`
	TypeID         string                   `json:"typeId"`
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	Prefix         string                   `json:"prefix"`
	Pattern        string                   `json:"pattern"`
	Phase          string                   `json:"phase"`
	SourceMetaPath string                   `json:"sourceMetaPath"`
	Template       gqlArtifactTypeFile      `json:"template"`
	Prompt         gqlArtifactTypeFile      `json:"prompt"`
	Examples       []gqlArtifactTypeExample `json:"examples"`
}

type gqlArtifactTypeDefinitionResponse struct {
	Data struct {
		Artifact struct {
			ID              string                      `json:"id"`
			TypeDefinitions []gqlArtifactTypeDefinition `json:"typeDefinitions"`
		} `json:"artifact"`
	} `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

func writeArtifactTypeFixture(t *testing.T, root, pluginName, typeID, prefix, templateContent, promptContent string, exampleContent *string) {
	t.Helper()

	typeDir := filepath.Join(root, ddxroot.DirName, "plugins", pluginName, "workflows", "phases", "01-frame", "artifacts", typeID)
	if err := os.MkdirAll(typeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := "artifact:\n" +
		"  name: " + typeID + "\n" +
		"  id: " + typeID + "\n" +
		"  type: document\n" +
		"  prefix: " + prefix + "\n" +
		"  phase: frame\n" +
		"description: " + typeID + " definition\n"
	if err := os.WriteFile(filepath.Join(typeDir, "meta.yml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(typeDir, "template.md"), []byte(templateContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(typeDir, "prompt.md"), []byte(promptContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if exampleContent != nil {
		if err := os.WriteFile(filepath.Join(typeDir, "example.md"), []byte(*exampleContent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func writeArtifactFixture(t *testing.T, root, id, relPath, body string) {
	t.Helper()

	fullPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nddx:\n  id: " + id + "\n---\n" + body + "\n"
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fetchArtifactTypeDefinitions(t *testing.T, srv *httptest.Server, projID, artifactID string) gqlArtifactTypeDefinitionResponse {
	t.Helper()

	body := bytes.NewBufferString(`{"query":"{ artifact(projectID: \"` + projID + `\", id: \"` + artifactID + `\") { id typeDefinitions { plugin typeId name description prefix pattern phase sourceMetaPath template { path content isTruncated sizeBytes } prompt { path content isTruncated sizeBytes } examples { path description content isTruncated sizeBytes } } } }"}`)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var out gqlArtifactTypeDefinitionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Errors) > 0 {
		t.Fatalf("GraphQL errors: %s", string(out.Errors))
	}
	return out
}

func TestTypeDefinitions_SingleMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	writeArtifactFixture(t, workDir, "DOC-001", "docs/helix/01-frame/single.md", "# single")
	writeArtifactTypeFixture(t, workDir, "ddx", "docs-sample", "docs", "# template one", "# prompt one", nil)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	out := fetchArtifactTypeDefinitions(t, srv, projID, "doc:DOC-001")
	defs := out.Data.Artifact.TypeDefinitions
	if len(defs) != 1 {
		t.Fatalf("expected 1 artifact type definition, got %d", len(defs))
	}
	if defs[0].Plugin != "ddx" || defs[0].TypeID != "docs-sample" {
		t.Fatalf("unexpected definition: %+v", defs[0])
	}
	if defs[0].Template.Content != "# template one" || defs[0].Prompt.Content != "# prompt one" {
		t.Fatalf("expected inline template/prompt content, got template=%q prompt=%q", defs[0].Template.Content, defs[0].Prompt.Content)
	}
	if defs[0].Template.IsTruncated || defs[0].Prompt.IsTruncated {
		t.Fatalf("expected untruncated small files, got template=%v prompt=%v", defs[0].Template.IsTruncated, defs[0].Prompt.IsTruncated)
	}
}

func TestTypeDefinitions_PrefixCollision_MultiMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	writeArtifactFixture(t, workDir, "DOC-002", "docs/helix/01-frame/collision.md", "# collision")
	writeArtifactTypeFixture(t, workDir, "alpha", "docs-alpha", "docs", "# alpha template", "# alpha prompt", nil)
	writeArtifactTypeFixture(t, workDir, "beta", "docs-beta", "docs", "# beta template", "# beta prompt", nil)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	out := fetchArtifactTypeDefinitions(t, srv, projID, "doc:DOC-002")
	defs := out.Data.Artifact.TypeDefinitions
	if len(defs) != 2 {
		t.Fatalf("expected 2 artifact type definitions, got %d", len(defs))
	}
	if defs[0].Plugin != "alpha" || defs[1].Plugin != "beta" {
		t.Fatalf("expected deterministic plugin ordering, got %+v", defs)
	}
	for _, def := range defs {
		if def.Prefix != "docs" {
			t.Fatalf("expected docs prefix, got %+v", def)
		}
	}
}

func TestTypeDefinitions_LargeFile_Truncated(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	writeArtifactFixture(t, workDir, "DOC-003", "docs/helix/01-frame/large.md", "# large")
	large := strings.Repeat("x", 70*1024)
	writeArtifactTypeFixture(t, workDir, "ddx", "docs-large", "docs", large, "# small prompt", nil)

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	out := fetchArtifactTypeDefinitions(t, srv, projID, "doc:DOC-003")
	defs := out.Data.Artifact.TypeDefinitions
	if len(defs) != 1 {
		t.Fatalf("expected 1 artifact type definition, got %d", len(defs))
	}
	if !defs[0].Template.IsTruncated {
		t.Fatalf("expected template to be truncated, got %+v", defs[0].Template)
	}
	if defs[0].Template.SizeBytes <= 64*1024 {
		t.Fatalf("expected template size > 64KB, got %d", defs[0].Template.SizeBytes)
	}
	if len(defs[0].Template.Content) == 0 {
		t.Fatal("expected truncated template content to be returned")
	}
}

func TestTypeDefinitions_NoArtifactTypes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	writeArtifactFixture(t, workDir, "DOC-004", "docs/helix/01-frame/none.md", "# none")

	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()

	projID := "proj-integration-" + filepath.Base(workDir)
	out := fetchArtifactTypeDefinitions(t, srv, projID, "doc:DOC-004")
	defs := out.Data.Artifact.TypeDefinitions
	if len(defs) != 0 {
		t.Fatalf("expected 0 artifact type definitions, got %d", len(defs))
	}
}

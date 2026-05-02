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
)

// setupSearchFixtureInDir builds a fixture for body/metadata search tests.
// Layout (under <workDir>):
//
//	.ddx/plugins/search/title-hit.txt           — match in sidecar title
//	.ddx/plugins/search/path-hit.txt            — match in path segment
//	.ddx/plugins/search/desc-hit.txt            — match in sidecar description
//	.ddx/plugins/search/fm-hit.txt              — match in frontmatter (generated_by.run_id)
//	.ddx/plugins/search/body-hit.txt            — match only in body
//	.ddx/plugins/search/oversize.txt            — cap-only test: needle past 256 KiB
//	.ddx/plugins/search/binary-ext.png          — binary by extension
//	.ddx/plugins/search/binary-nul.dat          — binary by NUL sniff
//
// All sidecars use the unique token "needlexyz" only in their intended
// match field; this lets each AC field be probed in isolation.
func setupSearchFixtureInDir(t *testing.T, workDir string) string {
	t.Helper()
	dir := filepath.Join(workDir, ".ddx", "plugins", "search")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	type fixture struct {
		artifact     string // file under dir
		artifactBody []byte
		sidecarBody  string
	}

	fixtures := []fixture{
		{
			artifact:     "neutral-1.txt",
			artifactBody: []byte("noise"),
			sidecarBody:  "ddx:\n  id: NEUTRAL-1\n  title: NeedlEXYZ Title Hit\n  media_type: text/plain\n",
		},
		{
			artifact:     "needlexyz-path-hit.txt",
			artifactBody: []byte("noise"),
			sidecarBody:  "ddx:\n  id: PATHHIT-1\n  title: Plain\n  media_type: text/plain\n",
		},
		{
			artifact:     "neutral-2.txt",
			artifactBody: []byte("noise"),
			sidecarBody:  "ddx:\n  id: DESCHIT-1\n  title: Plain\n  media_type: text/plain\n  description: contains needlexyz in description\n",
		},
		{
			artifact:     "neutral-3.txt",
			artifactBody: []byte("noise"),
			sidecarBody: "ddx:\n  id: FMHIT-1\n  title: Plain\n  media_type: text/plain\n" +
				"  generated_by:\n    run_id: needlexyz-run-99\n    prompt_summary: pp\n    source_hash: ''\n",
		},
		{
			artifact:     "neutral-4.txt",
			artifactBody: []byte("body content has the needlexyz token within text"),
			sidecarBody:  "ddx:\n  id: BODYHIT-1\n  title: Plain\n  media_type: text/plain\n",
		},
		{
			artifact:     "neutral-5.txt",
			artifactBody: append(bytes.Repeat([]byte("a"), 300*1024), []byte("needlexyz")...),
			sidecarBody:  "ddx:\n  id: OVERSIZE-1\n  title: Plain\n  media_type: text/plain\n",
		},
		{
			artifact:     "binary-ext.png",
			artifactBody: []byte("needlexyz inside but png extension"),
			sidecarBody:  "ddx:\n  id: BINEXT-1\n  title: Plain\n  media_type: image/png\n",
		},
		{
			artifact:     "binary-nul.dat",
			artifactBody: append([]byte("needlexyz\x00 in first bytes"), []byte("ignored")...),
			sidecarBody:  "ddx:\n  id: BINNUL-1\n  title: Plain\n  media_type: text/plain\n",
		},
	}

	for _, f := range fixtures {
		artifactPath := filepath.Join(dir, f.artifact)
		if err := os.WriteFile(artifactPath, f.artifactBody, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(artifactPath+".ddx.yaml", []byte(f.sidecarBody), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return workDir
}

// gqlSearchEdges runs an artifacts query and returns the edges.
func gqlSearchEdges(t *testing.T, srv *httptest.Server, projID, query string) []map[string]interface{} {
	t.Helper()
	body := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", search: \"` + query + `\") { totalCount edges { node { id title path } snippet } } }"}`)
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
	rawEdges := result["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["edges"].([]interface{})
	out := make([]map[string]interface{}, len(rawEdges))
	for i, e := range rawEdges {
		out[i] = e.(map[string]interface{})
	}
	return out
}

func edgeIDs(edges []map[string]interface{}) []string {
	ids := make([]string, len(edges))
	for i, e := range edges {
		ids[i] = e["node"].(map[string]interface{})["id"].(string)
	}
	return ids
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestArtifacts_Search_TitleMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	edges := gqlSearchEdges(t, srv, projID, "needlexyz")
	ids := edgeIDs(edges)
	// All 5 metadata + 1 body fixture should match. Binaries + oversize
	// should be excluded.
	for _, want := range []string{"NEUTRAL-1", "PATHHIT-1", "DESCHIT-1", "FMHIT-1", "BODYHIT-1"} {
		if !containsID(ids, want) {
			t.Errorf("expected id %q in search results: %v", want, ids)
		}
	}
}

func TestArtifacts_Search_PathMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	edges := gqlSearchEdges(t, srv, projID, "needlexyz-path-hit")
	ids := edgeIDs(edges)
	if !containsID(ids, "PATHHIT-1") {
		t.Errorf("expected PATHHIT-1 in path-token search results: %v", ids)
	}
}

func TestArtifacts_Search_DescriptionMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	edges := gqlSearchEdges(t, srv, projID, "in description")
	ids := edgeIDs(edges)
	if !containsID(ids, "DESCHIT-1") {
		t.Errorf("expected DESCHIT-1 in description-token search: %v", ids)
	}
}

func TestArtifacts_Search_FrontmatterMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	// "needlexyz-run-99" appears only inside the JSON-stringified frontmatter.
	edges := gqlSearchEdges(t, srv, projID, "needlexyz-run-99")
	ids := edgeIDs(edges)
	if !containsID(ids, "FMHIT-1") {
		t.Errorf("expected FMHIT-1 in frontmatter-token search: %v", ids)
	}
}

func TestArtifacts_Search_BodyMatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	// "token within text" appears only in BODYHIT-1's body, not in any
	// metadata field of any artifact.
	edges := gqlSearchEdges(t, srv, projID, "token within text")
	ids := edgeIDs(edges)
	if len(ids) != 1 || ids[0] != "BODYHIT-1" {
		t.Fatalf("expected exactly BODYHIT-1 from body match, got %v", ids)
	}
	// Snippet must be populated for a body-only match.
	sn, ok := edges[0]["snippet"].(string)
	if !ok || sn == "" {
		t.Fatalf("expected snippet on body match, got %v", edges[0]["snippet"])
	}
	if !strings.Contains(sn, "**token within text**") {
		t.Errorf("snippet did not mark match terms: %q", sn)
	}
}

func TestArtifacts_Search_BodySizeCap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	// The needle in OVERSIZE-1's body is past the 256 KiB cap, so a body
	// search for that needle must not return OVERSIZE-1.
	// We use a needle that does NOT appear in any metadata field anywhere.
	edges := gqlSearchEdges(t, srv, projID, "needlexyz")
	ids := edgeIDs(edges)
	if containsID(ids, "OVERSIZE-1") {
		t.Errorf("expected OVERSIZE-1 to be excluded by size cap, got %v", ids)
	}
}

func TestArtifacts_Search_BinarySkip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	edges := gqlSearchEdges(t, srv, projID, "needlexyz")
	ids := edgeIDs(edges)
	if containsID(ids, "BINEXT-1") {
		t.Errorf("expected BINEXT-1 to be excluded by extension allowlist, got %v", ids)
	}
	if containsID(ids, "BINNUL-1") {
		t.Errorf("expected BINNUL-1 to be excluded by NUL-byte sniff, got %v", ids)
	}
}

func TestArtifacts_Search_NonBodyMatchHasNoSnippet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	// Title-only match: snippet should still render and mark the match.
	edges := gqlSearchEdges(t, srv, projID, "needlexyz")
	for _, e := range edges {
		id := e["node"].(map[string]interface{})["id"].(string)
		if id != "NEUTRAL-1" {
			continue
		}
		sn, ok := e["snippet"].(string)
		if !ok || sn == "" {
			t.Fatalf("expected snippet on title match for NEUTRAL-1, got %v", e["snippet"])
		}
		// The match is case-insensitive but the snippet preserves source casing
		// and wraps the matched substring with markers.
		if !strings.Contains(strings.ToLower(sn), "**needlexyz**") {
			t.Errorf("title snippet did not mark match: %q", sn)
		}
		return
	}
	t.Fatal("NEUTRAL-1 not present in search edges")
}

func TestArtifacts_Search_PaginationDeterministic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir, store := setupIntegrationDir(t)
	workDir = setupSearchFixtureInDir(t, workDir)
	srv := httptest.NewServer(newArtifactGQLHandler(workDir, store))
	defer srv.Close()
	projID := "proj-integration-" + filepath.Base(workDir)

	// Compose search + sort + paginate. Use TITLE sort to exercise the
	// (sortKey, id) tie-breaker on titles like "Plain".
	q := `{ artifacts(projectID: \"` + projID + `\", search: \"needlexyz\", sort: TITLE) { edges { node { id title } cursor } } }`
	body := bytes.NewBufferString(`{"query":"` + q + `"}`)
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

	var prevTitle, prevID string
	for i, e := range edges {
		node := e.(map[string]interface{})["node"].(map[string]interface{})
		title := node["title"].(string)
		id := node["id"].(string)
		if i > 0 {
			if title < prevTitle {
				t.Errorf("titles not ascending at %d: %q before %q", i, prevTitle, title)
			}
			if title == prevTitle && id < prevID {
				t.Errorf("id tie-breaker violated at %d: %q before %q", i, prevID, id)
			}
		}
		prevTitle, prevID = title, id
	}

	// Paginate first:2, then after the second cursor; the union must equal
	// the un-paginated set in the same order.
	if len(edges) < 4 {
		t.Fatalf("need at least 4 matching artifacts for pagination test, got %d", len(edges))
	}
	expectedIDs := make([]string, len(edges))
	for i, e := range edges {
		expectedIDs[i] = e.(map[string]interface{})["node"].(map[string]interface{})["id"].(string)
	}

	// Page 1: first:2
	page1Cursor := edges[1].(map[string]interface{})["cursor"].(string)
	body1 := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", search: \"needlexyz\", sort: TITLE, first: 2) { edges { node { id } } } }"}`)
	resp1, _ := http.Post(srv.URL+"/graphql", "application/json", body1)
	var r1 map[string]interface{}
	json.NewDecoder(resp1.Body).Decode(&r1)
	resp1.Body.Close()
	got1 := r1["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["edges"].([]interface{})
	if len(got1) != 2 || got1[0].(map[string]interface{})["node"].(map[string]interface{})["id"].(string) != expectedIDs[0] {
		t.Errorf("page1 mismatch: %v vs %v", got1, expectedIDs[:2])
	}

	// Page 2: after page1Cursor
	body2 := bytes.NewBufferString(`{"query":"{ artifacts(projectID: \"` + projID + `\", search: \"needlexyz\", sort: TITLE, after: \"` + page1Cursor + `\") { edges { node { id } } } }"}`)
	resp2, _ := http.Post(srv.URL+"/graphql", "application/json", body2)
	var r2 map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&r2)
	resp2.Body.Close()
	got2 := r2["data"].(map[string]interface{})["artifacts"].(map[string]interface{})["edges"].([]interface{})
	if len(got2) != len(expectedIDs)-2 {
		t.Errorf("page2 length: got %d want %d", len(got2), len(expectedIDs)-2)
	}
	for i, e := range got2 {
		got := e.(map[string]interface{})["node"].(map[string]interface{})["id"].(string)
		if got != expectedIDs[i+2] {
			t.Errorf("page2[%d]: got %q want %q", i, got, expectedIDs[i+2])
		}
	}
}

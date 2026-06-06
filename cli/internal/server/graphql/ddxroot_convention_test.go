package graphql_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	ddxgraphql "github.com/DocumentDrivenDX/ddx/internal/server/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ddxRootRunsProvider struct {
	*testStateProvider
	run *ddxgraphql.Run
}

func (p *ddxRootRunsProvider) GetRunsGraphQL(projectID string, filter ddxgraphql.RunFilter) []*ddxgraphql.Run {
	if p.run == nil {
		return nil
	}
	if projectID != "" && (p.run.ProjectID == nil || *p.run.ProjectID != projectID) {
		return nil
	}
	return ddxgraphql.ApplyRunFilter([]*ddxgraphql.Run{p.run}, filter)
}

func (p *ddxRootRunsProvider) GetRunGraphQL(id string) (*ddxgraphql.Run, bool) {
	if p.run != nil && p.run.ID == id {
		return p.run, true
	}
	return nil, false
}

func setupConventionRootGraphQLProject(t *testing.T) (string, *bead.Store) {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	projectRoot := t.TempDir()
	stateRoot := ddxroot.Path(context.Background(), projectRoot)
	require.NotEqual(t, filepath.Join(projectRoot, ddxroot.DirName), stateRoot, "test must run in convention-root mode")
	require.NoError(t, os.MkdirAll(stateRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stateRoot, "config.yaml"), []byte("version: \"1.0\"\nbead:\n  id_prefix: \"it\"\n"), 0o644))

	store := bead.NewStore(stateRoot)
	require.NoError(t, store.Create(context.Background(), &bead.Bead{
		ID:         "ddx-root-gql",
		Title:      "GraphQL DDx root coverage",
		Status:     bead.StatusOpen,
		IssueType:  bead.DefaultType,
		Acceptance: "graphql should resolve state via ddxroot",
	}))

	pluginDir := ddxroot.JoinProject(projectRoot, "plugins", "search")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "report.md"), []byte("# DDx root artifact\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "report.md.ddx.yaml"), []byte(
		"ddx:\n  id: ART-DDXROOT\n  title: DDx Root Artifact\n  media_type: text/markdown\n",
	), 0o644))

	return projectRoot, store
}

func TestGraphQLResolversUseDDxRoot(t *testing.T) {
	projectRoot, store := setupConventionRootGraphQLProject(t)
	base := newTestStateProvider(projectRoot, store)
	projectID := base.projects[0].ID
	beadID := "ddx-root-gql"

	run := &ddxgraphql.Run{
		ID:          "run-ddxroot",
		Layer:       ddxgraphql.RunLayerRun,
		Status:      "success",
		ChildRunIds: []string{},
		ProjectID:   &projectID,
		BeadID:      &beadID,
	}
	provider := &ddxRootRunsProvider{
		testStateProvider: base,
		run:               run,
	}
	h := newGQLHandler(provider, projectRoot, nil)

	resp := gqlPost(t, h, `{ beadsReady { totalCount edges { node { id } } } }`)
	var beadsData struct {
		BeadsReady struct {
			TotalCount int `json:"totalCount"`
			Edges      []struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"beadsReady"`
	}
	require.NoError(t, json.Unmarshal(resp["data"], &beadsData))
	require.Equal(t, 1, beadsData.BeadsReady.TotalCount)
	require.Len(t, beadsData.BeadsReady.Edges, 1)
	assert.Equal(t, beadID, beadsData.BeadsReady.Edges[0].Node.ID)

	resp = gqlPost(t, h, `{
		run(id: "run-ddxroot") { id }
	}`)
	var runData struct {
		Run struct {
			ID string `json:"id"`
		} `json:"run"`
	}
	require.NoError(t, json.Unmarshal(resp["data"], &runData))
	assert.Equal(t, "run-ddxroot", runData.Run.ID)

	events, err := store.EventsByKind(beadID, "run_detail_view")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Body, "run_id=run-ddxroot")

	resp = gqlPost(t, h, `{
		artifacts(projectID: "`+projectID+`") { edges { node { id } } }
	}`)
	var artifactsData struct {
		Artifacts struct {
			Edges []struct {
				Node struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"artifacts"`
	}
	require.NoError(t, json.Unmarshal(resp["data"], &artifactsData))

	var ids []string
	for _, edge := range artifactsData.Artifacts.Edges {
		ids = append(ids, edge.Node.ID)
	}
	assert.Contains(t, ids, "ART-DDXROOT")

	_, err = os.Stat(filepath.Join(projectRoot, ddxroot.DirName))
	assert.True(t, os.IsNotExist(err), "test must prove resolver access without an in-tree .ddx directory")
}

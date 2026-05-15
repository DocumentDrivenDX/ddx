package graphql

import (
	"context"
	"fmt"
	"strings"

	gqlgraphql "github.com/99designs/gqlgen/graphql"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// RunsStateProvider is the optional sub-interface a StateProvider may
// implement to back the runs/run queries.
// Kept separate from StateProvider so test stubs that don't need runs
// continue to compile.
type RunsStateProvider interface {
	GetRunsGraphQL(projectID string, filter RunFilter) []*Run
	GetRunGraphQL(id string) (*Run, bool)
}

// RunDetailStateProvider is the optional sub-interface for the Story 16
// run-detail surfaces (tool calls + bundle file resolver). Kept separate from
// RunsStateProvider so legacy stubs continue to compile.
type RunDetailStateProvider interface {
	GetRunToolCallsGraphQL(id string) []*RunToolCall
	GetRunBundleFileGraphQL(id, path string) (*RunBundleFileContent, bool)
}

const runDetailViewEventKind = "run_detail_view"
const rawTranscriptViewedEventKind = "raw_transcript_viewed"

// RunFilter holds the optional filter args for Query.runs.
type RunFilter struct {
	Layer   *RunLayer
	BeadID  string
	Status  string
	Harness string
}

// Runs is the resolver for the runs field.
func (r *queryResolver) Runs(ctx context.Context, projectID *string, layer *RunLayer, beadID *string, status *string, harness *string, first *int, after *string) (*RunConnection, error) {
	provider, ok := r.State.(RunsStateProvider)
	if !ok {
		return emptyRunConnection(), nil
	}
	pid := ""
	if projectID != nil {
		pid = *projectID
	}
	filter := RunFilter{Layer: layer}
	if beadID != nil {
		filter.BeadID = *beadID
	}
	if status != nil {
		filter.Status = *status
	}
	if harness != nil {
		filter.Harness = *harness
	}
	all := provider.GetRunsGraphQL(pid, filter)
	return runConnectionFrom(all, first, after), nil
}

// Run is the resolver for the run field.
func (r *queryResolver) Run(ctx context.Context, id string) (*Run, error) {
	provider, ok := r.State.(RunsStateProvider)
	if !ok {
		return nil, nil
	}
	wd := r.workingDir(ctx)
	if wd == "" {
		return nil, nil
	}
	projectID, ok := r.projectIDForWorkingDir(wd)
	if !ok {
		return nil, nil
	}
	run, ok := provider.GetRunGraphQL(id)
	if !ok {
		return nil, nil
	}
	if run.ProjectID == nil || strings.TrimSpace(*run.ProjectID) != projectID {
		return nil, nil
	}
	r.recordRunDetailView(ctx, run)
	if runHasRawTranscriptSelection(ctx) {
		r.recordRawTranscriptViewed(ctx, run)
	}
	return run, nil
}

func (r *queryResolver) projectIDForWorkingDir(workingDir string) (string, bool) {
	if r.State == nil || workingDir == "" {
		return "", false
	}
	for _, proj := range r.State.GetProjectSnapshots(false) {
		if proj.Path == workingDir {
			return proj.ID, true
		}
	}
	return "", false
}

// recordRunDetailView appends the canonical run_detail_view bead audit event
// the first time a project-scoped run detail is resolved for a specific run.
// Duplicate loads of the same run id are ignored, but distinct runs on the
// same bead remain auditable.
func (r *queryResolver) recordRunDetailView(ctx context.Context, run *Run) {
	if run == nil || run.BeadID == nil || strings.TrimSpace(*run.BeadID) == "" {
		return
	}
	if run.ProjectID == nil || strings.TrimSpace(*run.ProjectID) == "" {
		return
	}
	wd := r.workingDir(ctx)
	if wd == "" {
		return
	}
	if projectID, ok := r.projectIDForWorkingDir(wd); !ok || projectID != strings.TrimSpace(*run.ProjectID) {
		return
	}

	store := projectBeadStore(wd)
	if existing, err := store.EventsByKind(*run.BeadID, runDetailViewEventKind); err == nil {
		for _, ev := range existing {
			if strings.Contains(ev.Body, "run_id="+run.ID) {
				return
			}
		}
	}

	identity := operatorPromptIdentityInfo{kind: "unknown", actor: "anonymous"}
	if httpReq := httpRequestFromContext(ctx); httpReq != nil {
		identity = operatorPromptIdentity(httpReq)
	}
	body := fmt.Sprintf(
		"project_id=%s run_id=%s layer=%s visibility=project_membership",
		*run.ProjectID,
		run.ID,
		run.Layer,
	)
	_ = store.AppendEvent(*run.BeadID, bead.BeadEvent{
		Kind:    runDetailViewEventKind,
		Summary: "run detail viewed",
		Body:    body,
		Actor:   identity.actor,
		Source:  "graphql:run",
	})
}

func runHasRawTranscriptSelection(ctx context.Context) bool {
	if !gqlgraphql.HasOperationContext(ctx) {
		return false
	}
	for _, field := range gqlgraphql.CollectAllFields(ctx) {
		switch field {
		case "prompt", "response", "stderr":
			return true
		}
	}
	return false
}

func rawTranscriptSelectionSummary(ctx context.Context) string {
	if !gqlgraphql.HasOperationContext(ctx) {
		return ""
	}
	selected := gqlgraphql.CollectAllFields(ctx)
	ordered := make([]string, 0, 3)
	for _, name := range []string{"prompt", "response", "stderr"} {
		for _, field := range selected {
			if field == name {
				ordered = append(ordered, name)
				break
			}
		}
	}
	return strings.Join(ordered, ",")
}

func (r *queryResolver) recordRawTranscriptViewed(ctx context.Context, run *Run) {
	if run == nil || run.BeadID == nil || strings.TrimSpace(*run.BeadID) == "" {
		return
	}
	if run.ProjectID == nil || strings.TrimSpace(*run.ProjectID) == "" {
		return
	}
	wd := r.workingDir(ctx)
	if wd == "" {
		return
	}
	if projectID, ok := r.projectIDForWorkingDir(wd); !ok || projectID != strings.TrimSpace(*run.ProjectID) {
		return
	}

	store := projectBeadStore(wd)
	if existing, err := store.EventsByKind(*run.BeadID, rawTranscriptViewedEventKind); err == nil {
		for _, ev := range existing {
			if strings.Contains(ev.Body, "run_id="+run.ID) {
				return
			}
		}
	}

	identity := operatorPromptIdentityInfo{kind: "unknown", actor: "anonymous"}
	if httpReq := httpRequestFromContext(ctx); httpReq != nil {
		identity = operatorPromptIdentity(httpReq)
	}
	fields := rawTranscriptSelectionSummary(ctx)
	if fields == "" {
		fields = "prompt,response,stderr"
	}
	body := fmt.Sprintf(
		"project_id=%s run_id=%s fields=%s visibility=project_membership",
		*run.ProjectID,
		run.ID,
		fields,
	)
	_ = store.AppendEvent(*run.BeadID, bead.BeadEvent{
		Kind:    rawTranscriptViewedEventKind,
		Summary: "raw transcript viewed",
		Body:    body,
		Actor:   identity.actor,
		Source:  "graphql:run",
	})
}

// RunToolCalls is the resolver for the runToolCalls field. Returns the
// normalized tool-call entries persisted at drain time, paginated by
// sequence. Returns an empty connection (not an error) for unknown ids or
// when the backing state provider does not implement RunDetailStateProvider.
func (r *queryResolver) RunToolCalls(ctx context.Context, id string, first *int, after *string) (*RunToolCallConnection, error) {
	provider, ok := r.State.(RunDetailStateProvider)
	if !ok {
		return emptyRunToolCallConnection(), nil
	}
	calls := provider.GetRunToolCallsGraphQL(id)
	return runToolCallConnectionFrom(calls, first, after), nil
}

// RunBundleFile is the resolver for the runBundleFile field. Returns nil
// (not an error) when the path fails canonical-path / whitelist checks or
// the file is not present — the GraphQL response treats nil as 404.
func (r *queryResolver) RunBundleFile(ctx context.Context, id string, path string) (*RunBundleFileContent, error) {
	provider, ok := r.State.(RunDetailStateProvider)
	if !ok {
		return nil, nil
	}
	out, ok := provider.GetRunBundleFileGraphQL(id, path)
	if !ok {
		return nil, nil
	}
	return out, nil
}

func emptyRunToolCallConnection() *RunToolCallConnection {
	return &RunToolCallConnection{Edges: []*RunToolCallEdge{}, PageInfo: &PageInfo{}, TotalCount: 0}
}

func runToolCallConnectionFrom(calls []*RunToolCall, first *int, after *string) *RunToolCallConnection {
	all := make([]*RunToolCallEdge, len(calls))
	ids := make([]string, len(calls))
	for i, c := range calls {
		all[i] = &RunToolCallEdge{Node: c, Cursor: encodeStableCursor(c.ID)}
		ids[i] = c.ID
	}
	startIdx, _ := stablePageBounds(ids, after, nil)
	slice := all[startIdx:]
	truncByFirst := false
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
		truncByFirst = true
	}
	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0,
		HasNextPage:     truncByFirst,
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}
	return &RunToolCallConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

func emptyRunConnection() *RunConnection {
	return &RunConnection{Edges: []*RunEdge{}, PageInfo: &PageInfo{}, TotalCount: 0}
}

func runConnectionFrom(runs []*Run, first *int, after *string) *RunConnection {
	all := make([]*RunEdge, len(runs))
	ids := make([]string, len(runs))
	for i, r := range runs {
		all[i] = &RunEdge{Node: r, Cursor: encodeStableCursor(r.ID)}
		ids[i] = r.ID
	}
	startIdx, _ := stablePageBounds(ids, after, nil)
	slice := all[startIdx:]
	truncByFirst := false
	if first != nil && *first >= 0 && *first < len(slice) {
		slice = slice[:*first]
		truncByFirst = true
	}
	pageInfo := &PageInfo{
		HasPreviousPage: startIdx > 0,
		HasNextPage:     truncByFirst,
	}
	if len(slice) > 0 {
		pageInfo.StartCursor = &slice[0].Cursor
		pageInfo.EndCursor = &slice[len(slice)-1].Cursor
	}
	return &RunConnection{Edges: slice, PageInfo: pageInfo, TotalCount: len(all)}
}

// ApplyRunFilter applies the filter to a list of runs. Exported so state_runs.go can call it.
func ApplyRunFilter(in []*Run, filter RunFilter) []*Run {
	return applyRunFilter(in, filter)
}

// applyRunFilter applies the filter to a list of runs.
func applyRunFilter(in []*Run, filter RunFilter) []*Run {
	if len(in) == 0 {
		return in
	}
	if filter.Layer == nil && filter.BeadID == "" && filter.Status == "" && filter.Harness == "" {
		return in
	}
	out := in[:0:0]
	for _, r := range in {
		if filter.Layer != nil && r.Layer != *filter.Layer {
			continue
		}
		if filter.BeadID != "" {
			if r.BeadID == nil || *r.BeadID != filter.BeadID {
				continue
			}
		}
		if filter.Status != "" {
			if !strings.EqualFold(r.Status, filter.Status) {
				continue
			}
		}
		if filter.Harness != "" {
			if r.Harness == nil || *r.Harness != filter.Harness {
				continue
			}
		}
		out = append(out, r)
	}
	return out
}

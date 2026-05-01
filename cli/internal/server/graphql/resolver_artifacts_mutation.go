package graphql

import (
	"context"
	"time"

	"github.com/vektah/gqlparser/v2/gqlerror"
)

// ArtifactRegenerate is the resolver for the artifactRegenerate mutation.
//
// It looks up the artifact by ID and dispatches a server-side regeneration
// using the artifact's recorded generator metadata (generatedBy.runId and
// promptSummary). When the artifact has no generator metadata it returns a
// typed GraphQL error (extension code NO_GENERATOR_METADATA) so the UI can
// keep its Regenerate button disabled with an explanatory tooltip.
func (r *mutationResolver) ArtifactRegenerate(ctx context.Context, artifactID string) (*ArtifactRegenerateResult, error) {
	if r.WorkingDir == "" {
		return nil, &gqlerror.Error{
			Message:    "working directory not configured",
			Extensions: map[string]any{"code": "NOT_CONFIGURED"},
		}
	}

	// Resolve the artifact across all known projects so the mutation works
	// even when the caller passed only the artifact ID. We re-use the same
	// collection logic used by the Artifact and Artifacts resolvers.
	artifact, err := findArtifactByID(r.Resolver, artifactID)
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, &gqlerror.Error{
			Message:    "artifact not found",
			Extensions: map[string]any{"code": "ARTIFACT_NOT_FOUND"},
		}
	}
	if artifact.GeneratedBy == nil {
		return nil, &gqlerror.Error{
			Message: "artifact has no generator metadata; nothing to regenerate",
			Extensions: map[string]any{
				"code": "NO_GENERATOR_METADATA",
			},
		}
	}

	runID := newDispatchID("regen", artifactID)
	record := artifactRegenerateRecord{
		ID:            runID,
		ArtifactID:    artifactID,
		ArtifactPath:  artifact.Path,
		SourceRunID:   artifact.GeneratedBy.RunID,
		PromptSummary: artifact.GeneratedBy.PromptSummary,
		Status:        queuedPlaceholderState,
		CreatedAt:     time.Now().UTC(),
	}
	if err := writeJSONRecord(r.WorkingDir, "artifact-regenerations", runID, record); err != nil {
		return nil, err
	}

	return &ArtifactRegenerateResult{
		RunID:  runID,
		Status: queuedPlaceholderState,
	}, nil
}

type artifactRegenerateRecord struct {
	ID            string    `json:"id"`
	ArtifactID    string    `json:"artifact_id"`
	ArtifactPath  string    `json:"artifact_path"`
	SourceRunID   string    `json:"source_run_id"`
	PromptSummary string    `json:"prompt_summary"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

// findArtifactByID locates an artifact across the resolver's projects by ID.
// Falls back to scanning the resolver working directory when no project state
// is configured. Returns (nil, nil) when no artifact matches.
func findArtifactByID(r *Resolver, id string) (*Artifact, error) {
	roots := []string{}
	if r.State != nil {
		for _, p := range r.State.GetProjectSnapshots(false) {
			if p.Path != "" {
				roots = append(roots, p.Path)
			}
		}
	}
	if len(roots) == 0 && r.WorkingDir != "" {
		roots = append(roots, r.WorkingDir)
	}
	for _, root := range roots {
		artifacts, err := collectArtifacts(root)
		if err != nil {
			continue
		}
		for _, a := range artifacts {
			if a.ID == id {
				return a, nil
			}
		}
	}
	return nil, nil
}

package exec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/docgraph"
)

// loadDefinitions returns all definitions and the doc graph built during loading.
// The graph may be nil if graph loading failed (soft error treated as empty).
func (s *Store) loadDefinitions() ([]Definition, *docgraph.Graph, error) {
	defs := make(map[string]Definition)

	// Graph-authored definitions take precedence over runtime-managed ones.
	// The graph is returned so callers can reuse it without a second build.
	graph, graphDefs, _ := s.loadGraphDefinitions()
	for _, def := range graphDefs {
		if def.ID == "" || !def.Active {
			continue
		}
		defs[def.ID] = def
	}

	if s.DefinitionCollection != nil {
		beads, err := s.DefinitionCollection.ReadAll()
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range beads {
			def, err := definitionFromBead(entry)
			if err != nil {
				return nil, nil, err
			}
			if def.ID == "" || !def.Active {
				continue
			}
			if _, ok := defs[def.ID]; ok {
				continue // graph-authored takes precedence
			}
			defs[def.ID] = def
		}
	}

	legacy, err := s.readLegacyDefinitions()
	if err != nil {
		return nil, nil, err
	}
	for _, def := range legacy {
		if def.ID == "" || !def.Active {
			continue
		}
		if _, ok := defs[def.ID]; ok {
			continue
		}
		defs[def.ID] = def
	}

	out := make([]Definition, 0, len(defs))
	for _, def := range defs {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, graph, nil
}

// loadGraphDefinitions builds the doc graph and extracts exec definitions
// embedded in document frontmatter via the exec: block.
// Returns the built graph so callers can reuse it without a second walk.
func (s *Store) loadGraphDefinitions() (*docgraph.Graph, []Definition, error) {
	graph, err := s.buildGraph()
	if err != nil {
		return nil, nil, err
	}
	var defs []Definition
	for _, doc := range graph.Documents {
		if doc.ExecDef == nil {
			continue
		}
		ed := doc.ExecDef
		def := Definition{
			ID:          doc.ID,
			ArtifactIDs: ed.ArtifactIDs,
			Executor: ExecutorSpec{
				Kind:      ed.Executor.Kind,
				Command:   ed.Executor.Command,
				Cwd:       ed.Executor.Cwd,
				Env:       ed.Executor.Env,
				TimeoutMS: ed.Executor.TimeoutMS,
			},
			Required:    ed.Required,
			Active:      ed.Active,
			GraphSource: true,
		}
		defs = append(defs, def)
	}
	return graph, defs, nil
}

func (s *Store) readLegacyDefinitions() ([]Definition, error) {
	entries, err := os.ReadDir(s.DefinitionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	defs := make([]Definition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(s.DefinitionsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		var def Definition
		if err := json.Unmarshal(raw, &def); err != nil {
			return nil, fmt.Errorf("parse exec definition %q: %w", entry.Name(), err)
		}
		defs = append(defs, def)
	}
	return defs, nil
}

func (s *Store) loadRuns() ([]RunRecord, error) {
	runs := make(map[string]RunRecord)

	if s.RunCollection != nil {
		beads, err := s.RunCollection.ReadAll()
		if err != nil {
			return nil, err
		}
		for _, entry := range beads {
			rec, err := s.runRecordFromBead(entry)
			if err != nil {
				return nil, err
			}
			if rec.RunID == "" {
				continue
			}
			runs[rec.RunID] = rec
		}
	}

	legacy, err := s.readLegacyRuns()
	if err != nil {
		return nil, err
	}
	for _, rec := range legacy {
		if rec.RunID == "" {
			continue
		}
		if _, ok := runs[rec.RunID]; ok {
			continue
		}
		runs[rec.RunID] = rec
	}

	out := make([]RunRecord, 0, len(runs))
	for _, rec := range runs {
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].RunID < out[j].RunID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out, nil
}

func (s *Store) readLegacyRuns() ([]RunRecord, error) {
	entries, err := os.ReadDir(s.RunsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	records := make([]RunRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		rec, err := s.readRunBundle(filepath.Join(s.RunsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, nil
}

func (s *Store) loadRunByID(runID string) (RunRecord, error) {
	if s.RunCollection != nil {
		beads, err := s.RunCollection.ReadAll()
		if err != nil {
			return RunRecord{}, err
		}
		for _, entry := range beads {
			if entry.ID != runID {
				continue
			}
			return s.runRecordFromBead(entry)
		}
	}
	return s.readRunBundle(filepath.Join(s.RunsDir, runID))
}

func definitionBead(def Definition) bead.Bead {
	title := def.ID
	if len(def.ArtifactIDs) > 0 {
		title = fmt.Sprintf("Execution definition for %s", strings.Join(def.ArtifactIDs, ", "))
	}
	status := bead.StatusClosed
	if def.Active {
		status = bead.StatusOpen
	}
	return bead.Bead{
		ID:        def.ID,
		Title:     title,
		Status:    status,
		Priority:  2,
		IssueType: "exec_definition",
		CreatedAt: def.CreatedAt,
		UpdatedAt: def.CreatedAt,
		Labels:    definitionLabels(def),
		Extra: map[string]any{
			"definition": def,
		},
	}
}

func definitionFromBead(b bead.Bead) (Definition, error) {
	var def Definition
	if raw, ok := b.Extra["definition"]; ok {
		if err := decodeAny(raw, &def); err != nil {
			return Definition{}, err
		}
	}
	if def.ID == "" {
		def.ID = b.ID
	}
	if def.CreatedAt.IsZero() {
		def.CreatedAt = b.CreatedAt
	}
	if !def.Active {
		def.Active = b.Status == bead.StatusOpen
	}
	if len(def.ArtifactIDs) == 0 {
		def.ArtifactIDs = beadArtifactIDs(b)
	}
	return def, nil
}

func runBead(rec RunRecord) bead.Bead {
	title := rec.RunID
	if len(rec.ArtifactIDs) > 0 {
		title = fmt.Sprintf("Execution run for %s", strings.Join(rec.ArtifactIDs, ", "))
	}
	return bead.Bead{
		ID:        rec.RunID,
		Title:     title,
		Status:    bead.StatusClosed,
		Priority:  2,
		IssueType: "exec_run",
		CreatedAt: rec.StartedAt,
		UpdatedAt: rec.FinishedAt,
		Labels:    runLabels(rec),
		Extra: map[string]any{
			"manifest": rec.RunManifest,
		},
	}
}

func (s *Store) runRecordFromBead(b bead.Bead) (RunRecord, error) {
	var manifest RunManifest
	if raw, ok := b.Extra["manifest"]; ok {
		if err := decodeAny(raw, &manifest); err != nil {
			return RunRecord{}, err
		}
	}
	if manifest.RunID == "" {
		manifest.RunID = b.ID
	}
	if len(manifest.ArtifactIDs) == 0 {
		manifest.ArtifactIDs = beadArtifactIDs(b)
	}
	if manifest.StartedAt.IsZero() {
		manifest.StartedAt = b.CreatedAt
	}
	if manifest.FinishedAt.IsZero() {
		manifest.FinishedAt = b.UpdatedAt
	}
	if manifest.Status == "" {
		manifest.Status = beadRunStatus(b.Status)
	}
	if manifest.Attachments == nil {
		manifest.Attachments = runAttachmentRefs(s.WorkingDir, manifest.RunID)
	}

	result, err := s.readRunResult(manifest)
	if err != nil {
		return RunRecord{}, err
	}
	return RunRecord{RunManifest: manifest, Result: result}, nil
}

func (s *Store) readRunResult(manifest RunManifest) (RunResult, error) {
	resultPath := attachmentPath(s.WorkingDir, manifest.Attachments["result"])
	if resultPath != "" {
		if raw, err := os.ReadFile(resultPath); err == nil {
			var result RunResult
			if err := json.Unmarshal(raw, &result); err != nil {
				return RunResult{}, err
			}
			return result, nil
		}
	}
	resultPath = filepath.Join(s.runBundleDir(manifest.RunID), "result.json")
	raw, err := os.ReadFile(resultPath)
	if err != nil {
		return RunResult{}, err
	}
	var result RunResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return RunResult{}, err
	}
	return result, nil
}

func (s *Store) saveDefinitionBead(def Definition) error {
	if s.DefinitionCollection == nil {
		return fmt.Errorf("exec definition collection not initialized")
	}
	return s.DefinitionCollection.WithLock(func() error {
		beads, err := s.DefinitionCollection.ReadAll()
		if err != nil {
			return err
		}
		current := definitionBead(def)
		replaced := false
		for i := range beads {
			if beads[i].ID == current.ID {
				beads[i] = current
				replaced = true
				break
			}
		}
		if !replaced {
			beads = append(beads, current)
		}
		return s.DefinitionCollection.WriteAll(beads)
	})
}

func (s *Store) saveRunRecord(rec RunRecord) error {
	if s.RunCollection == nil {
		return fmt.Errorf("exec run collection not initialized")
	}
	if rec.RunID == "" {
		return fmt.Errorf("run_id is required")
	}
	if rec.DefinitionID == "" {
		return fmt.Errorf("definition_id is required")
	}
	if rec.Attachments == nil {
		rec.Attachments = make(map[string]string)
	}
	rec.Attachments["stdout"] = runAttachmentRef(s.WorkingDir, rec.RunID, "stdout.log")
	rec.Attachments["stderr"] = runAttachmentRef(s.WorkingDir, rec.RunID, "stderr.log")
	rec.Attachments["result"] = runAttachmentRef(s.WorkingDir, rec.RunID, "result.json")

	attachmentRoot := filepath.Join(s.WorkingDir, ".ddx", execRunAttachmentDir)
	if err := os.MkdirAll(attachmentRoot, 0o755); err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp(attachmentRoot, ".tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)

	manifestRaw, err := json.MarshalIndent(rec.RunManifest, "", "  ")
	if err != nil {
		return err
	}
	resultRaw, err := json.MarshalIndent(rec.Result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "manifest.json"), manifestRaw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "result.json"), resultRaw, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "stdout.log"), []byte(rec.Result.Stdout), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tempDir, "stderr.log"), []byte(rec.Result.Stderr), 0o644); err != nil {
		return err
	}
	if err := syncPath(tempDir); err != nil {
		return err
	}

	return s.RunCollection.WithLock(func() error {
		finalDir := filepath.Join(attachmentRoot, rec.RunID)
		if err := os.Rename(tempDir, finalDir); err != nil {
			return err
		}
		if err := syncPath(attachmentRoot); err != nil {
			return err
		}
		beads, err := s.RunCollection.ReadAll()
		if err != nil {
			return err
		}
		current := runBead(rec)
		replaced := false
		for i := range beads {
			if beads[i].ID == current.ID {
				beads[i] = current
				replaced = true
				break
			}
		}
		if !replaced {
			beads = append(beads, current)
		}
		return s.RunCollection.WriteAll(beads)
	})
}

func (s *Store) runBundleDir(runID string) string {
	return filepath.Join(s.WorkingDir, ".ddx", execRunAttachmentDir, runID)
}

func runAttachmentRef(workingDir, runID, name string) string {
	return filepath.Join(execRunAttachmentDir, runID, name)
}

func runAttachmentRefs(workingDir, runID string) map[string]string {
	return map[string]string{
		"stdout": runAttachmentRef(workingDir, runID, "stdout.log"),
		"stderr": runAttachmentRef(workingDir, runID, "stderr.log"),
		"result": runAttachmentRef(workingDir, runID, "result.json"),
	}
}

func attachmentPath(workingDir, ref string) string {
	if ref == "" {
		return ""
	}
	if filepath.IsAbs(ref) {
		return ref
	}
	return filepath.Join(workingDir, ".ddx", filepath.FromSlash(ref))
}

func beadRunStatus(status string) string {
	switch status {
	case StatusSuccess:
		return "success"
	case StatusTimedOut:
		return "timed_out"
	case StatusErrored:
		return "errored"
	default:
		return "failed"
	}
}

func beadArtifactIDs(b bead.Bead) []string {
	if raw, ok := b.Extra["artifact_ids"]; ok {
		var ids []string
		if err := decodeAny(raw, &ids); err == nil {
			return ids
		}
	}
	return nil
}

func definitionLabels(def Definition) []string {
	labels := make([]string, 0, len(def.ArtifactIDs)+2)
	for _, artifactID := range def.ArtifactIDs {
		labels = append(labels, "artifact:"+artifactID)
	}
	if def.Executor.Kind != "" {
		labels = append(labels, "executor:"+def.Executor.Kind)
	}
	return labels
}

func runLabels(rec RunRecord) []string {
	labels := make([]string, 0, len(rec.ArtifactIDs)+2)
	for _, artifactID := range rec.ArtifactIDs {
		labels = append(labels, "artifact:"+artifactID)
	}
	if rec.Status != "" {
		labels = append(labels, "status:"+rec.Status)
	}
	if rec.DefinitionID != "" {
		labels = append(labels, "definition:"+rec.DefinitionID)
	}
	return labels
}

func decodeAny(raw any, target any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

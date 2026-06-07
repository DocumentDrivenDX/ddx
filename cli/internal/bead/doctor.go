package bead

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	doctorFindingKindOversizedField       = "oversized_field"
	doctorFindingKindParentAncestorInDeps = "parent_ancestor_in_deps"
	doctorFindingKindUnparseableLine      = "unparseable_line"
)

// DoctorFinding reports a single bead doctor issue. Findings cover oversized
// field values, dependency edges that point back into the bead's parent chain,
// and line-level parse failures.
type DoctorFinding struct {
	Kind         string   `json:"kind,omitempty"`
	BeadID       string   `json:"bead_id"`
	FieldPath    string   `json:"field_path"`
	TargetID     string   `json:"target_id,omitempty"`
	DependencyID string   `json:"dependency_id,omitempty"`
	AncestorID   string   `json:"ancestor_id,omitempty"`
	ParentChain  []string `json:"parent_chain,omitempty"`
	SizeBytes    int      `json:"size_bytes,omitempty"`
	SampleHead   string   `json:"sample_head,omitempty"`
}

// DoctorReport is the output of BeadDoctor — an ordered list of findings.
type DoctorReport struct {
	Path     string
	Findings []DoctorFinding
}

// Clean is true when there are no findings.
func (r DoctorReport) Clean() bool { return len(r.Findings) == 0 }

type doctorRow struct {
	lineNo   int
	rawLine  []byte
	raw      map[string]any
	bead     *Bead
	parseErr error
}

func loadDoctorRows(path string) ([]doctorRow, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	lines := bytes.Split(src, []byte{'\n'})
	rows := make([]doctorRow, 0, len(lines))
	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		row := doctorRow{
			lineNo:  i + 1,
			rawLine: append([]byte(nil), line...),
		}
		if err := json.Unmarshal(trimmed, &row.raw); err != nil {
			row.parseErr = err
			rows = append(rows, row)
			continue
		}
		bead, err := unmarshalBead(trimmed)
		if err != nil {
			row.parseErr = err
			rows = append(rows, row)
			continue
		}
		row.bead = &bead
		rows = append(rows, row)
	}
	return rows, nil
}

// BeadDoctor scans a beads.jsonl file and returns every oversized field and
// every dependency edge that points back into the bead's parent chain.
// Lines that fail to parse are reported as a single finding with
// Kind=unparseable_line and the raw line as the sample.
func BeadDoctor(path string) (DoctorReport, error) {
	report := DoctorReport{Path: path}
	rows, err := loadDoctorRows(path)
	if err != nil {
		return report, err
	}

	byID := make(map[string]*Bead, len(rows))
	for _, row := range rows {
		if row.bead == nil || row.bead.ID == "" {
			continue
		}
		byID[row.bead.ID] = row.bead
	}

	for _, row := range rows {
		if row.parseErr != nil {
			report.Findings = append(report.Findings, DoctorFinding{
				Kind:       doctorFindingKindUnparseableLine,
				BeadID:     fmt.Sprintf("line %d (unparseable)", row.lineNo),
				FieldPath:  "line",
				SizeBytes:  len(bytes.TrimSpace(row.rawLine)),
				SampleHead: firstN(string(bytes.TrimSpace(row.rawLine)), 80),
			})
			continue
		}
		if row.bead == nil {
			continue
		}
		report.Findings = append(report.Findings, oversizedFieldFindings(row.bead)...)
		report.Findings = append(report.Findings, parentAncestorDepFindings(row.bead, byID)...)
	}

	sortDoctorFindings(report.Findings)
	return report, nil
}

func oversizedFieldFindings(b *Bead) []DoctorFinding {
	findings := make([]DoctorFinding, 0, 5)
	if b == nil {
		return findings
	}
	for _, field := range []string{"description", "acceptance", "notes"} {
		if s := beadFieldString(b, field); len(s) > MaxFieldBytes {
			findings = append(findings, DoctorFinding{
				Kind:       doctorFindingKindOversizedField,
				BeadID:     b.ID,
				FieldPath:  field,
				SizeBytes:  len(s),
				SampleHead: firstN(s, 80),
			})
		}
	}
	for i, ev := range decodeBeadEvents(b.Extra["events"]) {
		for _, field := range []string{"body", "summary"} {
			if s := beadEventString(ev, field); len(s) > MaxFieldBytes {
				findings = append(findings, DoctorFinding{
					Kind:       doctorFindingKindOversizedField,
					BeadID:     b.ID,
					FieldPath:  fmt.Sprintf("events[%d].%s", i, field),
					SizeBytes:  len(s),
					SampleHead: firstN(s, 80),
				})
			}
		}
	}
	return findings
}

func beadFieldString(b *Bead, field string) string {
	switch field {
	case "description":
		return b.Description
	case "acceptance":
		return b.Acceptance
	case "notes":
		return b.Notes
	default:
		return ""
	}
}

func beadEventString(ev BeadEvent, field string) string {
	switch field {
	case "body":
		return ev.Body
	case "summary":
		return ev.Summary
	default:
		return ""
	}
}

func parentAncestorDepFindings(b *Bead, byID map[string]*Bead) []DoctorFinding {
	if b == nil {
		return nil
	}
	chain, _ := beadParentChain(byID, b.Parent)
	if len(chain) == 0 || len(b.Dependencies) == 0 {
		return nil
	}
	ancestorSet := make(map[string]struct{}, len(chain))
	for _, ancestorID := range chain {
		ancestorSet[ancestorID] = struct{}{}
	}

	findings := make([]DoctorFinding, 0, len(b.Dependencies))
	for i, dep := range b.Dependencies {
		if _, ok := ancestorSet[dep.DependsOnID]; !ok {
			continue
		}
		findings = append(findings, DoctorFinding{
			Kind:         doctorFindingKindParentAncestorInDeps,
			BeadID:       b.ID,
			FieldPath:    fmt.Sprintf("dependencies[%d].depends_on_id", i),
			TargetID:     dep.DependsOnID,
			DependencyID: dep.DependsOnID,
			AncestorID:   dep.DependsOnID,
			ParentChain:  append([]string(nil), chain...),
			SampleHead:   strings.Join(chain, " -> "),
		})
	}
	return findings
}

func sortDoctorFindings(findings []DoctorFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].BeadID != findings[j].BeadID {
			return findings[i].BeadID < findings[j].BeadID
		}
		if doctorFindingKindRank(findings[i].Kind) != doctorFindingKindRank(findings[j].Kind) {
			return doctorFindingKindRank(findings[i].Kind) < doctorFindingKindRank(findings[j].Kind)
		}
		if findings[i].AncestorID != findings[j].AncestorID {
			return findings[i].AncestorID < findings[j].AncestorID
		}
		if findings[i].DependencyID != findings[j].DependencyID {
			return findings[i].DependencyID < findings[j].DependencyID
		}
		if findings[i].FieldPath != findings[j].FieldPath {
			return findings[i].FieldPath < findings[j].FieldPath
		}
		if findings[i].TargetID != findings[j].TargetID {
			return findings[i].TargetID < findings[j].TargetID
		}
		return findings[i].SampleHead < findings[j].SampleHead
	})
}

func doctorFindingKindRank(kind string) int {
	switch kind {
	case doctorFindingKindOversizedField:
		return 0
	case doctorFindingKindParentAncestorInDeps:
		return 1
	case doctorFindingKindUnparseableLine:
		return 2
	default:
		return 9
	}
}

// BeadDoctorFix rewrites oversized fields on disk and removes dependency edges
// that point back into the bead's parent chain.
//
//  1. If the file is already clean, returns the empty report without
//     touching anything.
//  2. Otherwise writes a timestamped backup under .ddx/backups/ before any
//     mutation; errors if the backup write fails.
//  3. Truncates oversized fields via capFieldBytes (head+tail+marker), then
//     writes full overflow content to
//     .ddx/executions/<bead-id>/repair-<timestamp>/<field>.log so the
//     original payload remains auditable.
//  4. Removes dependency edges whose targets appear in the bead's parent
//     chain.
//  5. Appends a repair event to each rewritten bead (kind="repair", actor=
//     "ddx bead doctor").
//  6. Returns the report of findings that were remediated.
//
// Idempotent: a second call finds no offending fields and returns a clean
// report without writing anything.
func BeadDoctorFix(path string, now func() time.Time) (DoctorReport, error) {
	if now == nil {
		now = time.Now
	}
	report, err := BeadDoctor(path)
	if err != nil {
		return report, err
	}
	if report.Clean() {
		return report, nil
	}

	ddxDir := filepath.Dir(path)
	ts := now().UTC().Format("20060102T150405")

	backupDir := filepath.Join(ddxDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return report, fmt.Errorf("bead doctor: mkdir backup dir: %w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("beads-%s.jsonl", ts))
	src, err := os.ReadFile(path)
	if err != nil {
		return report, fmt.Errorf("bead doctor: read source: %w", err)
	}
	if err := os.WriteFile(backupPath, src, 0o644); err != nil {
		return report, fmt.Errorf("bead doctor: write backup %s: %w", backupPath, err)
	}

	findingsByBead := make(map[string][]DoctorFinding)
	for _, f := range report.Findings {
		findingsByBead[f.BeadID] = append(findingsByBead[f.BeadID], f)
	}

	lines := bytes.Split(src, []byte{'\n'})
	var out bytes.Buffer
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			out.Write(line)
			out.WriteByte('\n')
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(trimmed, &raw); err != nil {
			// Leave unparseable lines alone; the doctor report flagged them
			// but we cannot safely rewrite what we cannot parse.
			out.Write(line)
			out.WriteByte('\n')
			continue
		}
		id, _ := raw["id"].(string)
		beadFindings := findingsByBead[id]
		if len(beadFindings) == 0 {
			out.Write(trimmed)
			out.WriteByte('\n')
			continue
		}

		repairDir := filepath.Join(ddxDir, "executions", id, "repair-"+ts)
		repaired, err := repairBead(raw, beadFindings, repairDir, ddxDir, now().UTC())
		if err != nil {
			return report, err
		}
		if repaired {
			encoded, err := json.Marshal(raw)
			if err != nil {
				return report, fmt.Errorf("bead doctor: re-encode %s: %w", id, err)
			}
			out.Write(encoded)
		} else {
			out.Write(trimmed)
		}
		out.WriteByte('\n')
	}

	written := out.Bytes()
	if !bytes.HasSuffix(src, []byte{'\n'}) && bytes.HasSuffix(written, []byte{'\n'}) {
		written = written[:len(written)-1]
	}

	if err := os.WriteFile(path, written, 0o644); err != nil {
		return report, fmt.Errorf("bead doctor: write repaired %s: %w", path, err)
	}
	return report, nil
}

// repairBead applies per-field truncation and dependency-edge removals. It
// updates raw in place and returns whether any changes were made plus a list of
// audit references to include in the repair event body.
func repairBead(raw map[string]any, findings []DoctorFinding, repairDir, ddxDir string, ts time.Time) (bool, error) {
	artifactRefs := make([]string, 0, len(findings))
	changed := false

	needsFieldRepair := false
	for _, f := range findings {
		if f.Kind == doctorFindingKindOversizedField {
			needsFieldRepair = true
			break
		}
	}
	if needsFieldRepair {
		if err := os.MkdirAll(repairDir, 0o755); err != nil {
			return false, fmt.Errorf("bead doctor: mkdir repair dir: %w", err)
		}
	}

	for _, f := range findings {
		switch f.Kind {
		case doctorFindingKindOversizedField:
			ref := applyFieldRepair(raw, f, repairDir, ddxDir)
			if ref != "" {
				artifactRefs = append(artifactRefs, f.FieldPath+"→"+ref)
				changed = true
			}
		}
	}

	if depRefs := applyParentAncestorDepRepair(raw, findings); len(depRefs) > 0 {
		artifactRefs = append(artifactRefs, depRefs...)
		changed = true
	}

	if !changed {
		return false, nil
	}

	events, _ := raw["events"].([]any)
	events = append(events, map[string]any{
		"kind":       "repair",
		"summary":    fmt.Sprintf("doctor repair applied: %d change(s)", len(artifactRefs)),
		"body":       strings.Join(artifactRefs, "\n"),
		"actor":      "ddx bead doctor",
		"source":     "ddx bead doctor --fix",
		"created_at": ts.Format(time.RFC3339Nano),
	})
	raw["events"] = events
	raw["updated_at"] = ts.Format(time.RFC3339Nano)
	return true, nil
}

// applyFieldRepair replaces raw[field] with a capped version and writes the
// full content to an artifact path. Returns the repair-relative artifact path
// (for the repair event summary) or "" if the path cannot be resolved.
func applyFieldRepair(raw map[string]any, f DoctorFinding, repairDir, ddxDir string) string {
	original, ok := pickField(raw, f.FieldPath)
	if !ok {
		return ""
	}
	artifactName := strings.ReplaceAll(f.FieldPath, "[", "_")
	artifactName = strings.ReplaceAll(artifactName, "]", "")
	artifactName = strings.ReplaceAll(artifactName, ".", "_") + ".log"
	artifactPath := filepath.Join(repairDir, artifactName)
	if err := os.WriteFile(artifactPath, []byte(original), 0o644); err != nil {
		return ""
	}
	rel, err := filepath.Rel(filepath.Dir(ddxDir), artifactPath)
	if err != nil {
		rel = artifactPath
	}
	// capFieldBytes already yields a value at or below MaxFieldBytes. Don't
	// append the artifact reference to the field itself — that would push it
	// back over the cap and defeat idempotency. The repair event body records
	// the field→artifact mapping for audit.
	setField(raw, f.FieldPath, capFieldBytes(original))
	return rel
}

// applyParentAncestorDepRepair removes dependency edges whose targets appear
// in the bead's parent chain. Returns audit references for the removed edges.
func applyParentAncestorDepRepair(raw map[string]any, findings []DoctorFinding) []string {
	targetOrder := make([]string, 0, len(findings))
	seenTargets := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		if f.Kind != doctorFindingKindParentAncestorInDeps || f.TargetID == "" {
			continue
		}
		if _, ok := seenTargets[f.TargetID]; ok {
			continue
		}
		seenTargets[f.TargetID] = struct{}{}
		targetOrder = append(targetOrder, f.TargetID)
	}
	if len(targetOrder) == 0 {
		return nil
	}

	deps, ok := raw["dependencies"].([]any)
	if !ok || len(deps) == 0 {
		return nil
	}

	removedCounts := make(map[string]int, len(targetOrder))
	filtered := make([]any, 0, len(deps))
	for _, depRaw := range deps {
		dep, _ := depRaw.(map[string]any)
		if dep == nil {
			filtered = append(filtered, depRaw)
			continue
		}
		target, _ := dep["depends_on_id"].(string)
		if target == "" {
			filtered = append(filtered, depRaw)
			continue
		}
		if _, ok := seenTargets[target]; ok {
			removedCounts[target]++
			continue
		}
		filtered = append(filtered, depRaw)
	}
	if len(removedCounts) == 0 {
		return nil
	}
	raw["dependencies"] = filtered

	refs := make([]string, 0, len(removedCounts))
	for _, target := range targetOrder {
		if n := removedCounts[target]; n > 0 {
			refs = append(refs, fmt.Sprintf("dependencies→removed %s (%d edge(s))", target, n))
		}
	}
	return refs
}

// pickField reads raw[path] where path is either a flat key or
// "events[N].field".
func pickField(raw map[string]any, path string) (string, bool) {
	if !strings.Contains(path, "events[") {
		s, ok := raw[path].(string)
		return s, ok
	}
	// events[N].field
	openIdx := strings.Index(path, "[")
	closeIdx := strings.Index(path, "]")
	if openIdx < 0 || closeIdx < 0 {
		return "", false
	}
	var n int
	if _, err := fmt.Sscanf(path[openIdx+1:closeIdx], "%d", &n); err != nil {
		return "", false
	}
	field := strings.TrimPrefix(path[closeIdx+1:], ".")
	events, ok := raw["events"].([]any)
	if !ok || n >= len(events) {
		return "", false
	}
	ev, _ := events[n].(map[string]any)
	if ev == nil {
		return "", false
	}
	s, ok := ev[field].(string)
	return s, ok
}

// setField mirrors pickField for writes.
func setField(raw map[string]any, path string, value string) {
	if !strings.Contains(path, "events[") {
		raw[path] = value
		return
	}
	openIdx := strings.Index(path, "[")
	closeIdx := strings.Index(path, "]")
	if openIdx < 0 || closeIdx < 0 {
		return
	}
	var n int
	if _, err := fmt.Sscanf(path[openIdx+1:closeIdx], "%d", &n); err != nil {
		return
	}
	field := strings.TrimPrefix(path[closeIdx+1:], ".")
	events, ok := raw["events"].([]any)
	if !ok || n >= len(events) {
		return
	}
	ev, _ := events[n].(map[string]any)
	if ev == nil {
		return
	}
	ev[field] = value
	events[n] = ev
	raw["events"] = events
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

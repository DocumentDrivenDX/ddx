package bead

import (
	"bufio"
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
	doctorFindingFieldOverflow      = "field_overflow"
	doctorFindingLineUnparseable    = "line_unparseable"
	doctorFindingParentAncestorDeps = "parent_ancestor_in_deps"
)

// DoctorFinding reports a single doctor finding on a single bead row. A bead
// can produce multiple findings (description + acceptance + event bodies +
// back-edge dependencies, etc.).
type DoctorFinding struct {
	Code       string `json:"code,omitempty"`
	BeadID     string `json:"bead_id,omitempty"`
	FieldPath  string `json:"field_path,omitempty"` // "description", "acceptance", "notes", "events[N].body", "events[N].summary", "dependencies[N].depends_on_id"
	DepIndex   int    `json:"dep_index,omitempty"`
	SizeBytes  int    `json:"size_bytes,omitempty"`
	SampleHead string `json:"sample_head,omitempty"` // first 80 bytes for visual identification
}

// DoctorReport is the output of BeadDoctor — an ordered list of findings.
type DoctorReport struct {
	Path     string
	Findings []DoctorFinding
}

// Clean is true when there are no findings (i.e. all bead fields fit).
func (r DoctorReport) Clean() bool { return len(r.Findings) == 0 }

type doctorRow struct {
	LineNo       int
	Raw          map[string]any
	BeadID       string
	Parent       string
	Dependencies []doctorDependency
}

type doctorDependency struct {
	Index       int
	DependsOnID string
}

// BeadDoctor scans a beads.jsonl file and returns every field that exceeds
// MaxFieldBytes, plus any dependency edge that points at the bead's parent
// chain. Parses each line best-effort; lines that fail to parse are reported
// as a single finding with FieldPath="line" and the raw line as the sample.
func BeadDoctor(path string) (DoctorReport, error) {
	rows, report, err := loadDoctorRows(path)
	if err != nil {
		return report, err
	}
	report.Findings = append(report.Findings, detectDoctorFindings(rows)...)
	sortDoctorFindings(report.Findings)
	return report, nil
}

func loadDoctorRows(path string) ([]doctorRow, DoctorReport, error) {
	report := DoctorReport{Path: path}
	f, err := os.Open(path)
	if err != nil {
		return nil, report, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	rows := make([]doctorRow, 0, 64)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			report.Findings = append(report.Findings, DoctorFinding{
				Code:       doctorFindingLineUnparseable,
				BeadID:     fmt.Sprintf("line %d (unparseable)", lineNo),
				FieldPath:  "line",
				SizeBytes:  len(line),
				SampleHead: firstN(string(line), 80),
			})
			continue
		}
		row := doctorRow{LineNo: lineNo, Raw: raw}
		row.BeadID, _ = raw["id"].(string)
		row.Parent, _ = raw["parent"].(string)
		if deps, ok := raw["dependencies"].([]any); ok {
			row.Dependencies = make([]doctorDependency, 0, len(deps))
			for i, depRaw := range deps {
				dep, _ := depRaw.(map[string]any)
				if dep == nil {
					continue
				}
				target, _ := dep["depends_on_id"].(string)
				if target == "" {
					continue
				}
				row.Dependencies = append(row.Dependencies, doctorDependency{
					Index:       i,
					DependsOnID: target,
				})
			}
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, report, fmt.Errorf("bead doctor: scanner: %w", err)
	}
	return rows, report, nil
}

func detectDoctorFindings(rows []doctorRow) []DoctorFinding {
	byID := make(map[string]*Bead, len(rows))
	for i := range rows {
		row := &rows[i]
		byID[row.BeadID] = &Bead{
			ID:     row.BeadID,
			Parent: row.Parent,
		}
	}

	findings := make([]DoctorFinding, 0, len(rows))
	for _, row := range rows {
		for _, field := range []string{"description", "acceptance", "notes"} {
			if s, ok := row.Raw[field].(string); ok && len(s) > MaxFieldBytes {
				findings = append(findings, DoctorFinding{
					Code:       doctorFindingFieldOverflow,
					BeadID:     row.BeadID,
					FieldPath:  field,
					SizeBytes:  len(s),
					SampleHead: firstN(s, 80),
				})
			}
		}
		if events, ok := row.Raw["events"].([]any); ok {
			for i, evRaw := range events {
				ev, _ := evRaw.(map[string]any)
				if ev == nil {
					continue
				}
				for _, field := range []string{"body", "summary"} {
					if s, ok := ev[field].(string); ok && len(s) > MaxFieldBytes {
						findings = append(findings, DoctorFinding{
							Code:       doctorFindingFieldOverflow,
							BeadID:     row.BeadID,
							FieldPath:  fmt.Sprintf("events[%d].%s", i, field),
							SizeBytes:  len(s),
							SampleHead: firstN(s, 80),
						})
					}
				}
			}
		}
		parentChain, err := beadParentChain(byID, row.Parent)
		if err != nil && len(parentChain) == 0 {
			continue
		}
		for _, dep := range row.Dependencies {
			if containsStringDoctor(parentChain, dep.DependsOnID) {
				findings = append(findings, DoctorFinding{
					Code:       doctorFindingParentAncestorDeps,
					BeadID:     row.BeadID,
					FieldPath:  fmt.Sprintf("dependencies[%d].depends_on_id", dep.Index),
					DepIndex:   dep.Index,
					SizeBytes:  len(dep.DependsOnID),
					SampleHead: firstN(dep.DependsOnID, 80),
				})
			}
		}
	}
	return findings
}

func sortDoctorFindings(findings []DoctorFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].BeadID != findings[j].BeadID {
			return findings[i].BeadID < findings[j].BeadID
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		if findings[i].DepIndex != findings[j].DepIndex && findings[i].DepIndex >= 0 && findings[j].DepIndex >= 0 {
			return findings[i].DepIndex < findings[j].DepIndex
		}
		if findings[i].FieldPath != findings[j].FieldPath {
			return findings[i].FieldPath < findings[j].FieldPath
		}
		if findings[i].SizeBytes != findings[j].SizeBytes {
			return findings[i].SizeBytes < findings[j].SizeBytes
		}
		return findings[i].SampleHead < findings[j].SampleHead
	})
}

// BeadDoctorFix rewrites oversized fields on disk. Behavior:
//
//  1. If the file is already clean, returns the empty report without
//     touching anything.
//  2. Otherwise writes a timestamped backup under .ddx/backups/ before any
//     mutation; errors if the backup write fails.
//  3. Truncates oversized fields via capFieldBytes (head+tail+marker), then
//     writes full overflow content to
//     .ddx/executions/<bead-id>/repair-<timestamp>/<field>.log so the
//     original payload remains auditable.
//  4. Removes dependency edges whose target is the bead's parent chain.
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

	// Per-bead rewrite: parse each line, repair it, write it back.
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
		if err := os.MkdirAll(repairDir, 0o755); err != nil {
			return report, fmt.Errorf("bead doctor: mkdir repair dir for %s: %w", id, err)
		}
		repaired := repairBead(raw, beadFindings, repairDir, ddxDir, now().UTC())
		encoded, err := json.Marshal(repaired)
		if err != nil {
			return report, fmt.Errorf("bead doctor: re-encode %s: %w", id, err)
		}
		out.Write(encoded)
		out.WriteByte('\n')
	}

	// Trim trailing newline if the source didn't have one.
	written := out.Bytes()
	if !bytes.HasSuffix(src, []byte{'\n'}) && bytes.HasSuffix(written, []byte{'\n'}) {
		written = written[:len(written)-1]
	}

	if err := os.WriteFile(path, written, 0o644); err != nil {
		return report, fmt.Errorf("bead doctor: write repaired %s: %w", path, err)
	}
	return report, nil
}

// repairBead applies per-field truncation + dependency edge removal. Updates
// raw in place and returns it for convenience.
func repairBead(raw map[string]any, findings []DoctorFinding, repairDir, ddxDir string, ts time.Time) map[string]any {
	repairRefs := make([]string, 0, len(findings))
	hasDepRepair := false
	for _, f := range findings {
		switch f.Code {
		case doctorFindingParentAncestorDeps:
			hasDepRepair = true
		case doctorFindingFieldOverflow, doctorFindingLineUnparseable, "":
			if ref := applyFieldRepair(raw, f, repairDir, ddxDir); ref != "" {
				repairRefs = append(repairRefs, f.FieldPath+"→"+ref)
			}
		default:
			if ref := applyFieldRepair(raw, f, repairDir, ddxDir); ref != "" {
				repairRefs = append(repairRefs, f.FieldPath+"→"+ref)
			}
		}
	}
	if hasDepRepair {
		if refs := applyDependencyRepairs(raw, findings); len(refs) > 0 {
			repairRefs = append(repairRefs, refs...)
		}
	}

	if len(repairRefs) == 0 {
		return raw
	}

	events, _ := raw["events"].([]any)
	events = append(events, map[string]any{
		"kind":       "repair",
		"summary":    fmt.Sprintf("doctor repair applied: %d finding(s) remediated", len(repairRefs)),
		"body":       strings.Join(repairRefs, "\n"),
		"actor":      "ddx bead doctor",
		"source":     "ddx bead doctor --fix",
		"created_at": ts.Format(time.RFC3339Nano),
	})
	raw["events"] = events
	raw["updated_at"] = ts.Format(time.RFC3339Nano)
	return raw
}

// applyFieldRepair replaces raw[field] with a capped version and writes the
// full content to an artifact path. Returns the repair-relative artifact
// path (for the repair event summary) or "" if the path cannot be resolved.
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
	// back over the cap and defeat idempotency. The repair event body
	// records the field→artifact mapping for audit.
	setField(raw, f.FieldPath, capFieldBytes(original))
	return rel
}

// applyDependencyRepairs removes dependency edges identified by the supplied
// findings. The original ordering of the remaining edges is preserved.
func applyDependencyRepairs(raw map[string]any, findings []DoctorFinding) []string {
	deps, ok := raw["dependencies"].([]any)
	if !ok || len(deps) == 0 {
		return nil
	}

	remove := make(map[int]DoctorFinding)
	for _, f := range findings {
		if f.Code != doctorFindingParentAncestorDeps {
			continue
		}
		if idx, ok := dependencyIndexFromFieldPath(f.FieldPath); ok && idx >= 0 && idx < len(deps) {
			remove[idx] = f
		}
	}
	if len(remove) == 0 {
		return nil
	}

	newDeps := make([]any, 0, len(deps)-len(remove))
	refs := make([]string, 0, len(remove))
	for i, dep := range deps {
		if f, ok := remove[i]; ok {
			refs = append(refs, f.FieldPath+"→removed("+f.SampleHead+")")
			continue
		}
		newDeps = append(newDeps, dep)
	}
	raw["dependencies"] = newDeps
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

func dependencyIndexFromFieldPath(path string) (int, bool) {
	if !strings.HasPrefix(path, "dependencies[") {
		return 0, false
	}
	openIdx := strings.Index(path, "[")
	closeIdx := strings.Index(path, "]")
	if openIdx < 0 || closeIdx < 0 {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(path[openIdx+1:closeIdx], "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

func containsStringDoctor(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

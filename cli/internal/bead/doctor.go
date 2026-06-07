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
	DoctorFindingCodeFieldTooLarge        = "field_too_large"
	DoctorFindingCodeParentAncestorInDeps = "parent_ancestor_in_deps"
	DoctorFindingCodeUnparseableLine      = "unparseable_line"
)

// DoctorFinding reports a single doctor finding on a single bead row. One
// bead can produce multiple findings (oversized fields, ancestor back-edges,
// etc.).
type DoctorFinding struct {
	BeadID     string `json:"bead_id"`
	Code       string `json:"code"`       // e.g. field_too_large or parent_ancestor_in_deps
	FieldPath  string `json:"field_path"` // "description", "acceptance", "notes", "events[N].body", "events[N].summary", "dependencies[N].depends_on_id"
	SizeBytes  int    `json:"size_bytes"`
	SampleHead string `json:"sample_head"` // first 80 bytes for visual identification
}

// DoctorReport is the output of BeadDoctor — an ordered list of findings.
type DoctorReport struct {
	Path     string
	Findings []DoctorFinding
}

// Clean is true when there are no findings (i.e. all bead fields fit).
func (r DoctorReport) Clean() bool { return len(r.Findings) == 0 }

// BeadDoctor scans a beads.jsonl file and returns every field that exceeds
// MaxFieldBytes plus any dependency edges that point back to the bead's
// parent chain. Parses each line best-effort; lines that fail to parse are
// reported as a single finding with FieldPath="line" and the raw line as the
// sample.
func BeadDoctor(path string) (DoctorReport, error) {
	report := DoctorReport{Path: path}
	src, err := os.ReadFile(path)
	if err != nil {
		return report, err
	}

	records, err := parseDoctorRecords(src)
	if err != nil {
		return report, err
	}
	byID := make(map[string]*Bead, len(records))
	for i := range records {
		if records[i].bead.ID == "" {
			continue
		}
		byID[records[i].bead.ID] = &records[i].bead
	}

	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNo := 0
	recIdx := 0
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		rec := records[recIdx]
		recIdx++
		if rec.parseErr != nil {
			report.Findings = append(report.Findings, DoctorFinding{
				BeadID:     fmt.Sprintf("line %d (unparseable)", lineNo),
				Code:       DoctorFindingCodeUnparseableLine,
				FieldPath:  "line",
				SizeBytes:  len(line),
				SampleHead: firstN(string(line), 80),
			})
			continue
		}

		id := rec.bead.ID
		raw := rec.raw
		for _, field := range []string{"description", "acceptance", "notes"} {
			if s, ok := raw[field].(string); ok && len(s) > MaxFieldBytes {
				report.Findings = append(report.Findings, DoctorFinding{
					BeadID:     id,
					Code:       DoctorFindingCodeFieldTooLarge,
					FieldPath:  field,
					SizeBytes:  len(s),
					SampleHead: firstN(s, 80),
				})
			}
		}
		if events, ok := raw["events"].([]any); ok {
			for i, evRaw := range events {
				ev, _ := evRaw.(map[string]any)
				if ev == nil {
					continue
				}
				for _, field := range []string{"body", "summary"} {
					if s, ok := ev[field].(string); ok && len(s) > MaxFieldBytes {
						report.Findings = append(report.Findings, DoctorFinding{
							BeadID:     id,
							Code:       DoctorFindingCodeFieldTooLarge,
							FieldPath:  fmt.Sprintf("events[%d].%s", i, field),
							SizeBytes:  len(s),
							SampleHead: firstN(s, 80),
						})
					}
				}
			}
		}
		report.Findings = append(report.Findings, ancestorDependencyFindings(rec.bead, raw, byID)...)
	}
	if err := scanner.Err(); err != nil {
		return report, fmt.Errorf("bead doctor: scanner: %w", err)
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		if report.Findings[i].BeadID == report.Findings[j].BeadID {
			if report.Findings[i].Code == report.Findings[j].Code {
				if report.Findings[i].FieldPath == report.Findings[j].FieldPath {
					return report.Findings[i].SampleHead < report.Findings[j].SampleHead
				}
				return report.Findings[i].FieldPath < report.Findings[j].FieldPath
			}
			return report.Findings[i].Code < report.Findings[j].Code
		}
		return report.Findings[i].BeadID < report.Findings[j].BeadID
	})
	return report, nil
}

type doctorRecord struct {
	lineNo   int
	raw      map[string]any
	bead     Bead
	parseErr error
}

func parseDoctorRecords(src []byte) ([]doctorRecord, error) {
	scanner := bufio.NewScanner(bytes.NewReader(src))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	lineNo := 0
	var records []doctorRecord
	for scanner.Scan() {
		lineNo++
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		rec := doctorRecord{lineNo: lineNo}
		if err := json.Unmarshal(line, &rec.raw); err != nil {
			rec.parseErr = err
			records = append(records, rec)
			continue
		}
		bead, err := unmarshalBead(line)
		if err != nil {
			rec.parseErr = err
			records = append(records, rec)
			continue
		}
		rec.bead = bead
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bead doctor: scanner: %w", err)
	}
	return records, nil
}

type dependencyRef struct {
	target string
	mapRef bool
}

func (r dependencyRef) fieldPath(fieldName string, idx int) string {
	if r.mapRef {
		return fmt.Sprintf("%s[%d].depends_on_id", fieldName, idx)
	}
	return fmt.Sprintf("%s[%d]", fieldName, idx)
}

func dependencyRefs(raw map[string]any) (string, []dependencyRef) {
	if deps, ok := raw["dependencies"].([]any); ok {
		return "dependencies", dependencyRefsFromArray(deps, true)
	}
	if deps, ok := raw["deps"].([]any); ok {
		return "deps", dependencyRefsFromArray(deps, false)
	}
	return "", nil
}

func dependencyRefsFromArray(items []any, mapRefs bool) []dependencyRef {
	refs := make([]dependencyRef, 0, len(items))
	for _, item := range items {
		if mapRefs {
			ev, _ := item.(map[string]any)
			if ev == nil {
				refs = append(refs, dependencyRef{mapRef: true})
				continue
			}
			target, _ := ev["depends_on_id"].(string)
			refs = append(refs, dependencyRef{target: target, mapRef: true})
			continue
		}
		target, _ := item.(string)
		refs = append(refs, dependencyRef{target: target})
	}
	return refs
}

func ancestorDependencyFindings(b Bead, raw map[string]any, byID map[string]*Bead) []DoctorFinding {
	parentChain, _ := beadParentChain(byID, b.Parent)
	if len(parentChain) == 0 {
		return nil
	}
	fieldName, refs := dependencyRefs(raw)
	if fieldName == "" {
		return nil
	}
	findings := make([]DoctorFinding, 0, len(refs))
	for i, ref := range refs {
		if ref.target == "" || !containsString(parentChain, ref.target) {
			continue
		}
		findings = append(findings, DoctorFinding{
			BeadID:     b.ID,
			Code:       DoctorFindingCodeParentAncestorInDeps,
			FieldPath:  ref.fieldPath(fieldName, i),
			SizeBytes:  len(ref.target),
			SampleHead: firstN(ref.target, 80),
		})
	}
	return findings
}

// BeadDoctorFix rewrites oversized fields on disk and removes dependency
// edges that point back to the bead's parent chain. Behavior:
//
//  1. If the file is already clean, returns the empty report without
//     touching anything.
//  2. Otherwise writes a timestamped backup under .ddx/backups/ before any
//     mutation; errors if the backup write fails.
//  3. Truncates oversized fields via capFieldBytes (head+tail+marker), then
//     writes full overflow content to
//     .ddx/executions/<bead-id>/repair-<timestamp>/<field>.log so the
//     original payload remains auditable.
//  4. Removes dependency edges that point to the bead's parent or ancestor.
//  5. Appends a repair event to each rewritten bead.
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

	src, err := os.ReadFile(path)
	if err != nil {
		return report, fmt.Errorf("bead doctor: read source: %w", err)
	}

	records, err := parseDoctorRecords(src)
	if err != nil {
		return report, err
	}
	byID := make(map[string]*Bead, len(records))
	for i := range records {
		if records[i].bead.ID == "" {
			continue
		}
		byID[records[i].bead.ID] = &records[i].bead
	}

	ddxDir := filepath.Dir(path)
	ts := now().UTC().Format("20060102T150405")

	backupDir := filepath.Join(ddxDir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return report, fmt.Errorf("bead doctor: mkdir backup dir: %w", err)
	}
	backupPath := filepath.Join(backupDir, fmt.Sprintf("beads-%s.jsonl", ts))
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
		if err := os.MkdirAll(repairDir, 0o755); err != nil {
			return report, fmt.Errorf("bead doctor: mkdir repair dir for %s: %w", id, err)
		}
		repaired, repairRefs := repairBead(raw, beadFindings, byID, id, repairDir, ddxDir, now().UTC())
		if len(repairRefs) == 0 {
			out.Write(trimmed)
			out.WriteByte('\n')
			continue
		}
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

// repairBead applies per-field truncation + dependency pruning. Updates raw in
// place and returns it plus the repair references that should be recorded.
func repairBead(raw map[string]any, findings []DoctorFinding, byID map[string]*Bead, beadID, repairDir, ddxDir string, ts time.Time) (map[string]any, []string) {
	repairRefs := make([]string, 0, len(findings))
	for _, f := range findings {
		if f.Code != DoctorFindingCodeFieldTooLarge {
			continue
		}
		ref := applyFieldRepair(raw, f, repairDir, ddxDir)
		if ref != "" {
			repairRefs = append(repairRefs, f.FieldPath+"→"+ref)
		}
	}

	if bead, ok := byID[beadID]; ok {
		if removed := applyAncestorDependencyRepair(raw, *bead, byID); len(removed) > 0 {
			repairRefs = append(repairRefs, removed...)
		}
	}

	if len(repairRefs) == 0 {
		return raw, nil
	}

	events, _ := raw["events"].([]any)
	events = append(events, map[string]any{
		"kind":       "repair",
		"summary":    fmt.Sprintf("doctor repairs applied: %d change(s)", len(repairRefs)),
		"body":       strings.Join(repairRefs, "\n"),
		"actor":      "ddx bead doctor",
		"source":     "ddx bead doctor --fix",
		"created_at": ts.Format(time.RFC3339Nano),
	})
	raw["events"] = events
	raw["updated_at"] = ts.Format(time.RFC3339Nano)
	return raw, repairRefs
}

func applyAncestorDependencyRepair(raw map[string]any, b Bead, byID map[string]*Bead) []string {
	parentChain, _ := beadParentChain(byID, b.Parent)
	if len(parentChain) == 0 {
		return nil
	}

	fieldName, refs := dependencyRefs(raw)
	if fieldName == "" || len(refs) == 0 {
		return nil
	}

	removed := make([]string, 0, len(refs))
	kept := make([]any, 0, len(refs))
	changed := false
	for idx, ref := range refs {
		item, ok := dependencyRawItem(raw[fieldName], idx)
		if ref.target != "" && containsString(parentChain, ref.target) {
			changed = true
			removed = append(removed, fmt.Sprintf("%s→removed(%s)", ref.fieldPath(fieldName, idx), ref.target))
			continue
		}
		if ok {
			kept = append(kept, item)
		}
	}
	if !changed {
		return nil
	}
	if len(kept) == 0 {
		delete(raw, fieldName)
		return removed
	}
	raw[fieldName] = kept
	return removed
}

func dependencyRawItem(fieldValue any, idx int) (any, bool) {
	switch v := fieldValue.(type) {
	case []any:
		if idx >= 0 && idx < len(v) {
			return v[idx], true
		}
	case []string:
		if idx >= 0 && idx < len(v) {
			return v[idx], true
		}
	}
	return nil, false
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

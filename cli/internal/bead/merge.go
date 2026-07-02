package bead

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// TrackerMergeReport describes a JSONL three-way tracker merge.
type TrackerMergeReport struct {
	TotalRecords    int
	PreservedOurs   int
	PreservedTheirs int
	MergedRecords   int
	ScalarConflicts []TrackerScalarConflict
}

// TrackerScalarConflict records a same-field conflict resolved by the merge
// policy. The command intentionally reports these choices instead of hiding
// them behind a silent latest-wins fold.
type TrackerScalarConflict struct {
	ID     string
	Field  string
	Choice string
	Reason string
}

type trackerRecord map[string]any

// MergeTrackerConflictJSONL merges base/ours/theirs bead JSONL content.
// Records are keyed by id. Append-only event and dependency lists are unioned;
// scalar fields use a three-way merge with deterministic timestamp/ours
// tie-breaking for true conflicts.
func MergeTrackerConflictJSONL(baseData, oursData, theirsData []byte) ([]byte, TrackerMergeReport, error) {
	base, err := parseTrackerRecords(baseData, "base")
	if err != nil {
		return nil, TrackerMergeReport{}, err
	}
	ours, err := parseTrackerRecords(oursData, "ours")
	if err != nil {
		return nil, TrackerMergeReport{}, err
	}
	theirs, err := parseTrackerRecords(theirsData, "theirs")
	if err != nil {
		return nil, TrackerMergeReport{}, err
	}

	report := TrackerMergeReport{}
	ids := mergeTrackerIDs(base, ours, theirs)
	merged := make([]trackerRecord, 0, len(ids))
	for _, id := range ids {
		b, hasBase := base[id]
		o, hasOurs := ours[id]
		t, hasTheirs := theirs[id]

		switch {
		case hasOurs && !hasTheirs:
			merged = append(merged, cloneTrackerRecord(o))
			if !hasBase {
				report.PreservedOurs++
			}
		case hasTheirs && !hasOurs:
			merged = append(merged, cloneTrackerRecord(t))
			if !hasBase {
				report.PreservedTheirs++
			}
		case hasOurs && hasTheirs:
			if trackerRecordEqual(o, t) {
				merged = append(merged, cloneTrackerRecord(o))
				continue
			}
			rec, conflicts := mergeTrackerRecord(id, b, o, t, hasBase)
			merged = append(merged, rec)
			report.MergedRecords++
			report.ScalarConflicts = append(report.ScalarConflicts, conflicts...)
		default:
			merged = append(merged, cloneTrackerRecord(b))
		}
	}

	out, err := encodeTrackerJSONL(merged)
	if err != nil {
		return nil, TrackerMergeReport{}, err
	}
	if err := ValidateTrackerJSONLUnique(out); err != nil {
		return nil, TrackerMergeReport{}, err
	}
	report.TotalRecords = len(merged)
	return out, report, nil
}

// ValidateTrackerJSONLUnique checks that content is valid bead JSONL with one
// record per id.
func ValidateTrackerJSONLUnique(data []byte) error {
	_, err := parseTrackerRecords(data, "merged")
	return err
}

func parseTrackerRecords(data []byte, side string) (map[string]trackerRecord, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	records := make(map[string]trackerRecord)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec trackerRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return nil, fmt.Errorf("bead merge: parse %s line %d: %w", side, lineNo, err)
		}
		id, _ := rec["id"].(string)
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("bead merge: parse %s line %d: missing id", side, lineNo)
		}
		if _, exists := records[id]; exists {
			return nil, fmt.Errorf("bead merge: parse %s line %d: duplicate id %s", side, lineNo, id)
		}
		records[id] = rec
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("bead merge: scan %s: %w", side, err)
	}
	return records, nil
}

func mergeTrackerIDs(recordSets ...map[string]trackerRecord) []string {
	seen := make(map[string]bool)
	for _, records := range recordSets {
		for id := range records {
			seen[id] = true
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func mergeTrackerRecord(id string, base, ours, theirs trackerRecord, hasBase bool) (trackerRecord, []TrackerScalarConflict) {
	out := cloneTrackerRecord(ours)
	keys := mergeTrackerFieldKeys(base, ours, theirs)
	oursUpdated := trackerUpdatedAt(ours)
	theirsUpdated := trackerUpdatedAt(theirs)
	var conflicts []TrackerScalarConflict

	for _, key := range keys {
		switch key {
		case "labels":
			out[key] = mergeStringLists(valueSlice(base[key]), valueSlice(ours[key]), valueSlice(theirs[key]))
			continue
		case "dependencies":
			out[key] = mergeObjectLists(dependencyMergeKey, valueSlice(base[key]), valueSlice(ours[key]), valueSlice(theirs[key]))
			continue
		case "events":
			out[key] = mergeObjectLists(stableJSONKey, valueSlice(base[key]), valueSlice(ours[key]), valueSlice(theirs[key]))
			continue
		}

		bv, bok := base[key]
		ov, ook := ours[key]
		tv, tok := theirs[key]

		switch {
		case !ook && tok:
			if hasBase && bok && trackerValueEqual(tv, bv) {
				delete(out, key)
				continue
			}
			if hasBase && bok {
				choice, reason, value := chooseTrackerConflictValue(oursUpdated, theirsUpdated, nil, tv)
				if choice == "ours" {
					delete(out, key)
				} else {
					out[key] = value
				}
				conflicts = append(conflicts, TrackerScalarConflict{ID: id, Field: key, Choice: choice, Reason: reason})
				continue
			}
			out[key] = tv
		case ook && !tok:
			if hasBase && bok && trackerValueEqual(ov, bv) {
				delete(out, key)
				continue
			}
			if hasBase && bok {
				choice, reason, value := chooseTrackerConflictValue(oursUpdated, theirsUpdated, ov, nil)
				if choice == "theirs" {
					delete(out, key)
				} else {
					out[key] = value
				}
				conflicts = append(conflicts, TrackerScalarConflict{ID: id, Field: key, Choice: choice, Reason: reason})
				continue
			}
			out[key] = ov
		case !ook && !tok:
			delete(out, key)
		case trackerValueEqual(ov, tv):
			out[key] = ov
		case hasBase && bok && trackerValueEqual(ov, bv):
			out[key] = tv
		case hasBase && bok && trackerValueEqual(tv, bv):
			out[key] = ov
		default:
			choice, reason, value := chooseTrackerConflictValue(oursUpdated, theirsUpdated, ov, tv)
			out[key] = value
			conflicts = append(conflicts, TrackerScalarConflict{
				ID:     id,
				Field:  key,
				Choice: choice,
				Reason: reason,
			})
		}
	}
	out["id"] = id
	return out, conflicts
}

func chooseTrackerConflictValue(oursUpdated, theirsUpdated time.Time, oursValue, theirsValue any) (choice string, reason string, value any) {
	if theirsUpdated.After(oursUpdated) {
		return "theirs", "theirs has newer updated_at", theirsValue
	}
	if oursUpdated.After(theirsUpdated) {
		return "ours", "ours has newer updated_at", oursValue
	}
	return "ours", "equal updated_at; chose ours", oursValue
}

func mergeTrackerFieldKeys(recordSets ...trackerRecord) []string {
	seen := make(map[string]bool)
	for _, rec := range recordSets {
		for key := range rec {
			seen[key] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func cloneTrackerRecord(rec trackerRecord) trackerRecord {
	out := make(trackerRecord, len(rec))
	for key, value := range rec {
		out[key] = cloneTrackerValue(value)
	}
	return out
}

func cloneTrackerValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return value
	}
	return out
}

func trackerRecordEqual(a, b trackerRecord) bool {
	return trackerValueEqual(a, b)
}

func trackerValueEqual(a, b any) bool {
	return reflect.DeepEqual(canonicalTrackerValue(a), canonicalTrackerValue(b))
}

func canonicalTrackerValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return value
	}
	return out
}

func trackerUpdatedAt(rec trackerRecord) time.Time {
	raw, _ := rec["updated_at"].(string)
	if raw == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t
	}
	return time.Time{}
}

func valueSlice(value any) []any {
	switch v := value.(type) {
	case nil:
		return nil
	case []any:
		return v
	default:
		return []any{v}
	}
}

func mergeStringLists(lists ...[]any) []any {
	seen := make(map[string]bool)
	for _, list := range lists {
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				continue
			}
			seen[s] = true
		}
	}
	out := make([]string, 0, len(seen))
	for item := range seen {
		out = append(out, item)
	}
	sort.Strings(out)
	anyOut := make([]any, 0, len(out))
	for _, item := range out {
		anyOut = append(anyOut, item)
	}
	return anyOut
}

func mergeObjectLists(keyFn func(any) string, lists ...[]any) []any {
	seen := make(map[string]bool)
	out := make([]any, 0)
	for _, list := range lists {
		for _, item := range list {
			key := keyFn(item)
			if key == "" {
				key = stableJSONKey(item)
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, cloneTrackerValue(item))
		}
	}
	return out
}

func dependencyMergeKey(value any) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	issueID, _ := m["issue_id"].(string)
	dependsOnID, _ := m["depends_on_id"].(string)
	depType, _ := m["type"].(string)
	if issueID == "" || dependsOnID == "" || depType == "" {
		return ""
	}
	return issueID + "\x00" + dependsOnID + "\x00" + depType
}

func stableJSONKey(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%#v", value)
	}
	return string(data)
}

func encodeTrackerJSONL(records []trackerRecord) ([]byte, error) {
	var buf bytes.Buffer
	for _, rec := range records {
		data, err := json.Marshal(rec)
		if err != nil {
			return nil, fmt.Errorf("bead merge: marshal %s: %w", rec["id"], err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

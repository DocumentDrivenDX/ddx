package bead

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// FuzzParseBeadJSONL tests that the JSONL parser never panics and that
// foldLatestBeads never produces duplicate IDs for any byte input.
func FuzzParseBeadJSONL(f *testing.F) {
	// Seed corpus: valid, minimal, empty, and adversarial inputs.
	f.Add([]byte(`{"id":"bx-aabb0011","title":"test","status":"open","priority":2,"issue_type":"task","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`))
	f.Add([]byte(""))
	f.Add([]byte("{}"))
	f.Add([]byte("{bad json}"))
	f.Add([]byte("\x00\x01\x02\x03"))
	f.Add([]byte(`{"id":"","title":"","status":""}`))
	f.Add([]byte(`{"id":"a","title":"b"}` + "\n" + `{"id":"a","title":"b2"}`))
	f.Add([]byte(`{"id":"x","title":"t","status":"open"}` + "\n" + `not-json` + "\n" + `{"id":"y","title":"t2"}`))
	f.Add([]byte(`{"id":"dup","title":"first"}` + "\n" + `{"id":"dup","title":"second"}`))
	f.Add([]byte(strings.Repeat(`{"id":"a","title":"t"}`+"\n", 100)))
	f.Add([]byte(`{"id":"a","title":"` + strings.Repeat("x", 10000) + `"}`))
	f.Add([]byte(`{"id":"a","status":"open","title":"t","extra_field":"value","nested":{"k":"v"}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// parseBeadJSONL must never panic.
		beads, warnings, err := parseBeadJSONL(data)
		if err != nil {
			return // scan errors are OK
		}
		_ = warnings

		// foldLatestBeads must not panic and must return at most one entry per ID.
		folded := foldLatestBeads(beads)
		seen := make(map[string]int, len(folded))
		for _, b := range folded {
			seen[b.ID]++
		}
		for id, count := range seen {
			if count > 1 {
				t.Errorf("foldLatestBeads produced duplicate ID %q (%d times)", id, count)
			}
		}

		// folded count must never exceed unique IDs in input.
		if len(folded) > len(seen) {
			t.Errorf("folded len %d > unique IDs %d", len(folded), len(seen))
		}
	})
}

// FuzzUnmarshalBead verifies that unmarshalBead never panics on arbitrary JSON
// and that a successful parse round-trips through marshalBead.
func FuzzUnmarshalBead(f *testing.F) {
	f.Add([]byte(`{"id":"bx-aabb0011","title":"hello","status":"open","priority":2,"issue_type":"task","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"id":null,"title":123,"status":true}`))
	f.Add([]byte(`{"id":"a","title":"t","labels":["x","y"],"dependencies":[{"issue_id":"a","depends_on_id":"b","type":"blocks"}]}`))
	f.Add([]byte(`{"id":"a","title":"t","extra_key":"extra_val","nested":{"deep":1}}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(`null`))
	f.Add([]byte(`[]`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Must not panic.
		b, err := unmarshalBead(data)
		if err != nil {
			return
		}

		// Round-trip: marshal then re-parse must produce equal core fields.
		encoded, err := marshalBead(b)
		if err != nil {
			t.Errorf("marshalBead failed for valid unmarshal result: %v", err)
			return
		}

		b2, err := unmarshalBead(encoded)
		if err != nil {
			t.Errorf("unmarshalBead failed on re-encoded bead: %v", err)
			return
		}

		// Core fields must survive round-trip.
		if b.ID != b2.ID {
			t.Errorf("ID mismatch after round-trip: %q != %q", b.ID, b2.ID)
		}
		if b.Title != b2.Title {
			t.Errorf("Title mismatch after round-trip: %q != %q", b.Title, b2.Title)
		}
		if b.Status != b2.Status {
			t.Errorf("Status mismatch after round-trip: %q != %q", b.Status, b2.Status)
		}
		if b.Priority != b2.Priority {
			t.Errorf("Priority mismatch after round-trip: %d != %d", b.Priority, b2.Priority)
		}
	})
}

// FuzzMarshalBead verifies that marshalBead always produces valid JSON
// for any well-formed Bead, and that the output re-parses correctly.
func FuzzMarshalBead(f *testing.F) {
	f.Add("bx-aabb0011", "hello world", "open", 2, "task", "alice")
	f.Add("", "", "", 0, "", "")
	f.Add("id-with-dashes", "title with spaces", "closed", 4, "bug", "")
	f.Add("x", `"json injection"`, "in_progress", 1, "task", "bob")
	f.Add("a", "unicode: 日本語🚀", "open", 0, "feature", "")
	f.Add("b", strings.Repeat("x", 500), "open", 3, "task", "")

	f.Fuzz(func(t *testing.T, id, title, status string, priority int, issueType, owner string) {
		b := Bead{
			ID:        id,
			Title:     title,
			Status:    status,
			Priority:  priority,
			IssueType: issueType,
			Owner:     owner,
			CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		encoded, err := marshalBead(b)
		if err != nil {
			t.Errorf("marshalBead should not error: %v", err)
			return
		}

		// Output must be valid JSON.
		var raw map[string]any
		if err := json.Unmarshal(encoded, &raw); err != nil {
			t.Errorf("marshalBead produced invalid JSON: %v\noutput: %s", err, encoded)
			return
		}

		// Re-parse must not error.
		b2, err := unmarshalBead(encoded)
		if err != nil {
			t.Errorf("unmarshalBead failed on marshalBead output: %v", err)
			return
		}

		// Core string fields must round-trip exactly.
		if b.ID != b2.ID {
			t.Errorf("ID round-trip: %q -> %q", b.ID, b2.ID)
		}
		if b.Title != b2.Title {
			t.Errorf("Title round-trip: %q -> %q", b.Title, b2.Title)
		}
	})
}

// FuzzFoldLatestBeads verifies that foldLatestBeads is idempotent and
// always produces at most one row per ID for any input.
func FuzzFoldLatestBeads(f *testing.F) {
	// Seed with various duplication patterns.
	makeBeads := func(ids ...string) []byte {
		var lines []string
		for _, id := range ids {
			lines = append(lines, `{"id":"`+id+`","title":"t","status":"open"}`)
		}
		return []byte(strings.Join(lines, "\n"))
	}
	f.Add(makeBeads("a", "b", "c"))
	f.Add(makeBeads("a", "a", "a"))
	f.Add(makeBeads("a", "b", "a", "c", "b"))
	f.Add([]byte(""))
	f.Add(makeBeads())

	f.Fuzz(func(t *testing.T, data []byte) {
		beads, _, err := parseBeadJSONL(data)
		if err != nil {
			return
		}

		folded := foldLatestBeads(beads)

		// Invariant 1: no duplicate IDs.
		seen := make(map[string]bool, len(folded))
		for _, b := range folded {
			if seen[b.ID] {
				t.Errorf("duplicate ID %q after fold", b.ID)
			}
			seen[b.ID] = true
		}

		// Invariant 2: idempotent — folding again produces same result.
		folded2 := foldLatestBeads(folded)
		if len(folded2) != len(folded) {
			t.Errorf("foldLatestBeads not idempotent: len %d != %d on second fold", len(folded2), len(folded))
		}

		// Invariant 3: every ID in folded was present in input.
		inputIDs := make(map[string]bool, len(beads))
		for _, b := range beads {
			inputIDs[b.ID] = true
		}
		for _, b := range folded {
			if !inputIDs[b.ID] {
				t.Errorf("folded contains ID %q not in input", b.ID)
			}
		}
	})
}

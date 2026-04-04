package bead

import (
	"encoding/json"
	"fmt"
	"time"
)

// Known field names that map to Bead struct fields (bd-compatible names).
var knownFields = map[string]bool{
	"id": true, "title": true, "issue_type": true, "status": true,
	"priority": true, "owner": true, "created_at": true, "created_by": true,
	"updated_at": true, "labels": true, "parent": true, "description": true,
	"acceptance": true, "dependencies": true, "notes": true,
	// Note: bd computed fields (dependency_count, dependent_count, comment_count)
	// are NOT in knownFields — they land in Extra and round-trip through it.
}

// unmarshalBead parses JSON into a Bead, preserving unknown fields in Extra.
func unmarshalBead(data []byte) (Bead, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Bead{}, fmt.Errorf("bead: unmarshal: %w", err)
	}

	var b Bead

	if v, ok := raw["id"]; ok {
		json.Unmarshal(v, &b.ID)
	}
	if v, ok := raw["title"]; ok {
		json.Unmarshal(v, &b.Title)
	}
	if v, ok := raw["issue_type"]; ok {
		json.Unmarshal(v, &b.IssueType)
	}
	if v, ok := raw["status"]; ok {
		json.Unmarshal(v, &b.Status)
	}
	if v, ok := raw["priority"]; ok {
		json.Unmarshal(v, &b.Priority)
	}
	if v, ok := raw["owner"]; ok {
		json.Unmarshal(v, &b.Owner)
	}
	if v, ok := raw["created_at"]; ok {
		var t time.Time
		if err := json.Unmarshal(v, &t); err == nil {
			b.CreatedAt = t
		}
	}
	if v, ok := raw["created_by"]; ok {
		json.Unmarshal(v, &b.CreatedBy)
	}
	if v, ok := raw["updated_at"]; ok {
		var t time.Time
		if err := json.Unmarshal(v, &t); err == nil {
			b.UpdatedAt = t
		}
	}
	if v, ok := raw["labels"]; ok {
		json.Unmarshal(v, &b.Labels)
	}
	if v, ok := raw["parent"]; ok {
		json.Unmarshal(v, &b.Parent)
	}
	if v, ok := raw["description"]; ok {
		json.Unmarshal(v, &b.Description)
	}
	if v, ok := raw["acceptance"]; ok {
		json.Unmarshal(v, &b.Acceptance)
	}
	if v, ok := raw["notes"]; ok {
		json.Unmarshal(v, &b.Notes)
	}
	if v, ok := raw["dependencies"]; ok {
		json.Unmarshal(v, &b.Dependencies)
	}

	// Defaults
	if b.IssueType == "" {
		b.IssueType = DefaultType
	}
	if b.Status == "" {
		b.Status = DefaultStatus
	}

	// Collect unknown fields into Extra
	for k, v := range raw {
		if knownFields[k] {
			continue
		}
		if b.Extra == nil {
			b.Extra = make(map[string]any)
		}
		var val any
		json.Unmarshal(v, &val)
		b.Extra[k] = val
	}

	return b, nil
}

// MarshalBead serializes a Bead to JSON, merging Extra fields back in.
// The output matches bd/br JSONL format.
func MarshalBead(b Bead) ([]byte, error) {
	return marshalBead(b)
}

func marshalBead(b Bead) ([]byte, error) {
	m := map[string]any{
		"id":         b.ID,
		"title":      b.Title,
		"status":     b.Status,
		"priority":   b.Priority,
		"issue_type": b.IssueType,
		"created_at": b.CreatedAt,
		"updated_at": b.UpdatedAt,
	}

	// Optional fields — only include if non-empty
	if b.Owner != "" {
		m["owner"] = b.Owner
	}
	if b.CreatedBy != "" {
		m["created_by"] = b.CreatedBy
	}
	if len(b.Labels) > 0 {
		m["labels"] = b.Labels
	}
	if b.Parent != "" {
		m["parent"] = b.Parent
	}
	if b.Description != "" {
		m["description"] = b.Description
	}
	if b.Acceptance != "" {
		m["acceptance"] = b.Acceptance
	}
	if b.Notes != "" {
		m["notes"] = b.Notes
	}
	if len(b.Dependencies) > 0 {
		m["dependencies"] = b.Dependencies
	}

	// Merge extra fields (workflow-specific)
	for k, v := range b.Extra {
		if !knownFields[k] {
			m[k] = v
		}
	}

	return json.Marshal(m)
}

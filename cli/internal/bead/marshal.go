package bead

import (
	"encoding/json"
	"fmt"
	"time"
)

// Known field names that map to Bead struct fields.
var knownFields = map[string]bool{
	"id": true, "title": true, "type": true, "status": true,
	"priority": true, "labels": true, "parent": true,
	"description": true, "acceptance": true, "deps": true,
	"assignee": true, "notes": true, "created": true, "updated": true,
}

// unmarshalBead parses JSON into a Bead, preserving unknown fields in Extra.
func unmarshalBead(data []byte) (Bead, error) {
	// First unmarshal into a generic map to capture everything.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return Bead{}, fmt.Errorf("bead: unmarshal: %w", err)
	}

	var b Bead

	// Unmarshal known fields
	if v, ok := raw["id"]; ok {
		json.Unmarshal(v, &b.ID)
	}
	if v, ok := raw["title"]; ok {
		json.Unmarshal(v, &b.Title)
	}
	if v, ok := raw["type"]; ok {
		json.Unmarshal(v, &b.Type)
	}
	if v, ok := raw["status"]; ok {
		json.Unmarshal(v, &b.Status)
	}
	if v, ok := raw["priority"]; ok {
		json.Unmarshal(v, &b.Priority)
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
	if v, ok := raw["deps"]; ok {
		json.Unmarshal(v, &b.Deps)
	}
	if v, ok := raw["assignee"]; ok {
		json.Unmarshal(v, &b.Assignee)
	}
	if v, ok := raw["notes"]; ok {
		json.Unmarshal(v, &b.Notes)
	}
	if v, ok := raw["created"]; ok {
		var t time.Time
		if err := json.Unmarshal(v, &t); err == nil {
			b.Created = t
		}
	}
	if v, ok := raw["updated"]; ok {
		var t time.Time
		if err := json.Unmarshal(v, &t); err == nil {
			b.Updated = t
		}
	}

	// Defaults for nil slices
	if b.Labels == nil {
		b.Labels = []string{}
	}
	if b.Deps == nil {
		b.Deps = []string{}
	}
	if b.Type == "" {
		b.Type = DefaultType
	}
	if b.Status == "" {
		b.Status = DefaultStatus
	}

	// Collect unknown fields
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

// marshalBead serializes a Bead to JSON, merging Extra fields back in.
func marshalBead(b Bead) ([]byte, error) {
	// Build an ordered map with known fields first, then extras.
	m := map[string]any{
		"id":       b.ID,
		"title":    b.Title,
		"type":     b.Type,
		"status":   b.Status,
		"priority": b.Priority,
		"labels":   b.Labels,
		"deps":     b.Deps,
		"created":  b.Created,
		"updated":  b.Updated,
	}

	// Only include optional fields if non-empty
	if b.Parent != "" {
		m["parent"] = b.Parent
	}
	if b.Description != "" {
		m["description"] = b.Description
	}
	if b.Acceptance != "" {
		m["acceptance"] = b.Acceptance
	}
	if b.Assignee != "" {
		m["assignee"] = b.Assignee
	}
	if b.Notes != "" {
		m["notes"] = b.Notes
	}

	// Merge extra fields (workflow-specific)
	for k, v := range b.Extra {
		if !knownFields[k] {
			m[k] = v
		}
	}

	return json.Marshal(m)
}

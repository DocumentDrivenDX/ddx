package bead

import "github.com/DocumentDrivenDX/ddx/internal/bead/axon"

func axonBeadFromLocal(src Bead) axon.Bead {
	dst := axon.Bead{
		Version:     0,
		ID:          src.ID,
		Title:       src.Title,
		Status:      src.Status,
		Priority:    src.Priority,
		IssueType:   src.IssueType,
		Owner:       src.Owner,
		CreatedAt:   src.CreatedAt,
		CreatedBy:   src.CreatedBy,
		UpdatedAt:   src.UpdatedAt,
		Labels:      append([]string(nil), src.Labels...),
		Parent:      src.Parent,
		Description: src.Description,
		Acceptance:  src.Acceptance,
		Notes:       src.Notes,
		Extra:       cloneStringAnyMap(src.Extra),
	}
	if len(src.Dependencies) > 0 {
		dst.Dependencies = make([]axon.Dependency, 0, len(src.Dependencies))
		for _, dep := range src.Dependencies {
			dst.Dependencies = append(dst.Dependencies, axon.Dependency{
				IssueID:     dep.IssueID,
				DependsOnID: dep.DependsOnID,
				Type:        dep.Type,
				CreatedAt:   dep.CreatedAt,
				CreatedBy:   dep.CreatedBy,
				Metadata:    dep.Metadata,
			})
		}
	}
	return dst
}

func axonBeadInputFromLocal(src Bead) axon.BeadInput {
	dst := axon.BeadInput{
		ID:          src.ID,
		Title:       src.Title,
		Status:      src.Status,
		Priority:    src.Priority,
		IssueType:   src.IssueType,
		Owner:       src.Owner,
		CreatedAt:   src.CreatedAt,
		CreatedBy:   src.CreatedBy,
		UpdatedAt:   src.UpdatedAt,
		Labels:      append([]string(nil), src.Labels...),
		Parent:      src.Parent,
		Description: src.Description,
		Acceptance:  src.Acceptance,
		Notes:       src.Notes,
		Extra:       cloneStringAnyMap(src.Extra),
	}
	if len(src.Dependencies) > 0 {
		dst.Dependencies = make([]axon.DependencyInput, 0, len(src.Dependencies))
		for _, dep := range src.Dependencies {
			dst.Dependencies = append(dst.Dependencies, axon.DependencyInput{
				IssueID:     dep.IssueID,
				DependsOnID: dep.DependsOnID,
				Type:        dep.Type,
				CreatedAt:   dep.CreatedAt,
				CreatedBy:   dep.CreatedBy,
				Metadata:    dep.Metadata,
			})
		}
	}
	return dst
}

func axonBeadToLocal(src axon.Bead) Bead {
	dst := Bead{
		ID:          src.ID,
		Title:       src.Title,
		Status:      src.Status,
		Priority:    src.Priority,
		IssueType:   src.IssueType,
		Owner:       src.Owner,
		CreatedAt:   src.CreatedAt,
		CreatedBy:   src.CreatedBy,
		UpdatedAt:   src.UpdatedAt,
		Labels:      append([]string(nil), src.Labels...),
		Parent:      src.Parent,
		Description: src.Description,
		Acceptance:  src.Acceptance,
		Notes:       src.Notes,
		Extra:       cloneStringAnyMap(src.Extra),
	}
	if len(src.Dependencies) > 0 {
		dst.Dependencies = make([]Dependency, 0, len(src.Dependencies))
		for _, dep := range src.Dependencies {
			dst.Dependencies = append(dst.Dependencies, Dependency{
				IssueID:     dep.IssueID,
				DependsOnID: dep.DependsOnID,
				Type:        dep.Type,
				CreatedAt:   dep.CreatedAt,
				CreatedBy:   dep.CreatedBy,
				Metadata:    dep.Metadata,
			})
		}
	}
	return dst
}

func axonEventFromLocal(beadID string, index int, event BeadEvent) axon.BeadEvent {
	return axon.BeadEvent{
		EventOf:   beadID,
		Index:     index,
		Kind:      event.Kind,
		Summary:   event.Summary,
		Body:      event.Body,
		Actor:     event.Actor,
		Source:    event.Source,
		CreatedAt: event.CreatedAt,
	}
}

func axonEventToLocal(src axon.BeadEvent) BeadEvent {
	return BeadEvent{
		Kind:      src.Kind,
		Summary:   src.Summary,
		Body:      src.Body,
		Actor:     src.Actor,
		CreatedAt: src.CreatedAt,
		Source:    src.Source,
	}
}

func localEventsFromBead(b Bead) []BeadEvent {
	if b.Extra == nil {
		return nil
	}
	return decodeBeadEvents(b.Extra["events"])
}

func dependencySet(deps []Dependency) map[string]Dependency {
	out := make(map[string]Dependency, len(deps))
	for _, dep := range deps {
		out[dep.DependsOnID] = dep
	}
	return out
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

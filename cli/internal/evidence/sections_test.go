package evidence

import (
	"strings"
	"testing"
)

func TestFitSectionsAllFit(t *testing.T) {
	got := FitSections([]SectionInput{
		{Name: "a", Content: "aaa", MinFloor: true},
		{Name: "b", Content: "bbb"},
	}, 100)
	if len(got.Included) != 2 {
		t.Errorf("included = %d, want 2", len(got.Included))
	}
	if len(got.Omitted) != 0 {
		t.Errorf("expected no omissions, got %d", len(got.Omitted))
	}
	if got.TotalBytes != 6 {
		t.Errorf("TotalBytes = %d, want 6", got.TotalBytes)
	}
}

func TestFitSectionsOmitsNonFloorOverBudget(t *testing.T) {
	long := strings.Repeat("y", 1000)
	got := FitSections([]SectionInput{
		{Name: "floor", Content: "floor", MinFloor: true},
		{Name: "big", Content: long},
	}, 10)
	// Floor must be included regardless.
	if len(got.Included) < 1 || got.Included[0].Name != "floor" {
		t.Fatalf("floor not first included: %+v", got.Included)
	}
	// Big should be omitted because it can't fit even line-trimmed (no \n).
	for _, s := range got.Sections {
		if s.Name == "big" && s.BytesIncluded != 0 {
			t.Errorf("big should be omitted, got %d bytes", s.BytesIncluded)
		}
	}
}

func TestFitSectionsTelemetryShape(t *testing.T) {
	got := FitSections([]SectionInput{
		{Name: "x", Content: "hello"},
	}, 100)
	if len(got.Sections) != 1 {
		t.Fatalf("sections = %d", len(got.Sections))
	}
	s := got.Sections[0]
	if s.Name != "x" || s.BytesIncluded != 5 || s.BytesOmitted != 0 {
		t.Errorf("section telemetry wrong: %+v", s)
	}
	if len(s.SelectedItems) != 1 || s.SelectedItems[0] != "x" {
		t.Errorf("SelectedItems = %v", s.SelectedItems)
	}
}

func TestFitSectionsPerSectionCap(t *testing.T) {
	long := strings.Repeat("y", 1000)
	got := FitSections([]SectionInput{
		{Name: "g", Content: long, PerSectionCap: 100},
	}, 10000)
	if len(got.Sections) != 1 {
		t.Fatalf("expected 1 section")
	}
	if got.Sections[0].BytesIncluded > 100 {
		t.Errorf("section not clamped to PerSectionCap: %d", got.Sections[0].BytesIncluded)
	}
	if got.Sections[0].TruncationReason != "per_section_cap" {
		t.Errorf("TruncationReason = %q", got.Sections[0].TruncationReason)
	}
}

func TestEvidenceAssemblySectionExported(t *testing.T) {
	// Compile-time contract: the struct has the fields downstream stages
	// rely on.
	var s EvidenceAssemblySection
	s.Name = "x"
	s.BytesIncluded = 1
	s.BytesOmitted = 0
	s.TruncationReason = ""
	s.SelectedItems = []string{"a"}
	s.OmittedItems = []string{"b"}
	if s.Name != "x" {
		t.Fatal("struct shape broken")
	}
}

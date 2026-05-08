package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

// TestExecuteBead_ContextBudgetMinimal verifies that when ContextBudget is
// set to "minimal", the prompt omits large governing refs and only includes
// the bead description, acceptance, and minimal metadata.
func TestExecuteBead_ContextBudgetMinimal(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	b := &bead.Bead{
		ID:          "ddx-ctx-budget",
		Title:       "Context budget test",
		Description: "Test minimal context budget.",
		Acceptance:  "Prompt is smaller when context-budget=minimal",
		Labels:      []string{"test"},
	}
	refs := []executeBeadGoverningRef{
		{ID: "FEAT-LARGE", Path: "docs/helix/FEAT-SPEC.md", Title: "Large Spec Document"},
	}

	arts, err := createArtifactBundle(root, wt, "20260101T000000-ctx00001")
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}

	const baseRev = "1234abcd"

	// Generate prompt without minimal budget (full)
	promptFull, srcFull, err := buildPrompt(root, b, refs, arts, baseRev, "", "claude", "")
	if err != nil {
		t.Fatalf("buildPrompt (full): %v", err)
	}
	if srcFull != "synthesized" {
		t.Errorf("expected prompt source=synthesized, got %q", srcFull)
	}

	// Generate prompt with minimal budget
	promptMinimal, srcMinimal, err := buildPrompt(root, b, refs, arts, baseRev, "", "claude", "minimal")
	if err != nil {
		t.Fatalf("buildPrompt (minimal): %v", err)
	}
	if srcMinimal != "synthesized" {
		t.Errorf("expected prompt source=synthesized, got %q", srcMinimal)
	}

	// The minimal budget prompt should be smaller than the full one
	if len(promptMinimal) >= len(promptFull) {
		t.Errorf("minimal budget prompt (%d bytes) should be smaller than full (%d bytes)", len(promptMinimal), len(promptFull))
	}

	// Both prompts should have the bead description and acceptance
	fullStr := string(promptFull)
	minimalStr := string(promptMinimal)

	for _, s := range []string{fullStr, minimalStr} {
		if !strings.Contains(s, "<description>") || !strings.Contains(s, b.Description) {
			t.Errorf("prompt missing description: %q", s)
		}
		if !strings.Contains(s, "<acceptance>") || !strings.Contains(s, b.Acceptance) {
			t.Errorf("prompt missing acceptance: %q", s)
		}
	}

	// Full prompt should contain governing refs
	if !strings.Contains(fullStr, "<governing>") || !strings.Contains(fullStr, "FEAT-LARGE") {
		t.Errorf("full prompt should contain governing refs")
	}

	// Minimal budget prompt should NOT contain the full governing refs
	minimalHasRefs := strings.Contains(minimalStr, "FEAT-LARGE") || strings.Contains(minimalStr, "<governing>") && !strings.Contains(minimalStr, "No governing references")
	if minimalHasRefs {
		t.Errorf("minimal budget prompt should omit large governing refs, but found references")
	}

	// Minimal budget should have a note that governing refs are omitted
	if !strings.Contains(minimalStr, "No governing references") {
		t.Errorf("minimal budget prompt should indicate no governing refs")
	}

	// Both should have instructions
	for _, s := range []string{fullStr, minimalStr} {
		if !strings.Contains(s, "<instructions>") {
			t.Errorf("prompt missing instructions")
		}
	}
}

// TestExecuteBead_ContextBudgetDefaultBehavior verifies that empty context
// budget (default) produces the same prompt as before.
func TestExecuteBead_ContextBudgetDefaultBehavior(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	b := &bead.Bead{
		ID:          "ddx-ctx-default",
		Title:       "Default behavior test",
		Description: "Test default context budget.",
		Acceptance:  "Prompt includes full content when no budget is set",
		Labels:      []string{"test"},
	}
	refs := []executeBeadGoverningRef{
		{ID: "FEAT-DEFAULT", Path: "docs/helix/FEAT-TEST.md", Title: "Test Spec"},
	}

	arts, err := createArtifactBundle(root, wt, "20260101T000000-def00001")
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}

	const baseRev = "1234abcd"

	// Generate prompt with empty context budget (default)
	prompt, src, err := buildPrompt(root, b, refs, arts, baseRev, "", "claude", "")
	if err != nil {
		t.Fatalf("buildPrompt (default): %v", err)
	}
	if src != "synthesized" {
		t.Errorf("expected prompt source=synthesized, got %q", src)
	}

	s := string(prompt)

	// Should include description and acceptance
	if !strings.Contains(s, b.Description) {
		t.Errorf("default prompt should include description")
	}
	if !strings.Contains(s, b.Acceptance) {
		t.Errorf("default prompt should include acceptance")
	}

	// Should include governing refs
	if !strings.Contains(s, "FEAT-DEFAULT") {
		t.Errorf("default prompt should include governing refs")
	}
	if !strings.Contains(s, "<governing>") {
		t.Errorf("default prompt should have <governing> section")
	}

	// Should include missing governing text when refs empty
	arts2, err := createArtifactBundle(root, wt, "20260101T000000-def00002")
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}
	prompt2, _, err := buildPrompt(root, b, []executeBeadGoverningRef{}, arts2, baseRev, "", "claude", "")
	if err != nil {
		t.Fatalf("buildPrompt (no refs): %v", err)
	}
	if !strings.Contains(string(prompt2), executeBeadMissingGoverningText) {
		t.Errorf("prompt with no refs should include missing governing text")
	}
}

// TestExecuteBead_ContextBudgetEmptyRefsWithMinimal verifies that minimal
// budget with empty refs produces a consistent prompt.
func TestExecuteBead_ContextBudgetEmptyRefsWithMinimal(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	b := &bead.Bead{
		ID:          "ddx-ctx-empty-min",
		Title:       "Empty refs minimal test",
		Description: "No governing refs with minimal budget.",
		Acceptance:  "Minimal budget works with empty refs",
		Labels:      []string{"test"},
	}

	arts, err := createArtifactBundle(root, wt, "20260101T000000-empty00001")
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}

	const baseRev = "1234abcd"

	// Generate prompt with empty refs and minimal budget
	prompt, src, err := buildPrompt(root, b, []executeBeadGoverningRef{}, arts, baseRev, "", "claude", "minimal")
	if err != nil {
		t.Fatalf("buildPrompt (empty refs minimal): %v", err)
	}
	if src != "synthesized" {
		t.Errorf("expected prompt source=synthesized, got %q", src)
	}

	s := string(prompt)

	// Should include description and acceptance
	if !strings.Contains(s, b.Description) {
		t.Errorf("prompt should include description")
	}
	if !strings.Contains(s, b.Acceptance) {
		t.Errorf("prompt should include acceptance")
	}

	// Should have minimal governing note, not missing refs text
	if strings.Contains(s, executeBeadMissingGoverningText) {
		t.Errorf("minimal budget should not use missing refs text")
	}
	if !strings.Contains(s, "No governing references") {
		t.Errorf("minimal budget should have its own note")
	}
}

// TestExecuteBead_ContextBudgetOmitSpecDoc verifies that when spec-id is set
// but context-budget=minimal, the full spec document content is not included.
func TestExecuteBead_ContextBudgetOmitSpecDoc(t *testing.T) {
	root := t.TempDir()
	wt := t.TempDir()

	// Create a spec document that would normally be included
	specPath := filepath.Join(wt, "docs", "helix", "FEAT-TEST.md")
	if err := os.MkdirAll(filepath.Dir(specPath), 0o755); err != nil {
		t.Fatal(err)
	}
	specContent := `# FEAT-TEST Test Specification

This is a large specification document with many details that we want to omit
for cheap-tier attempts with minimal context budget.
`
	if err := os.WriteFile(specPath, []byte(specContent), 0o644); err != nil {
		t.Fatal(err)
	}

	b := &bead.Bead{
		ID:          "ddx-ctx-spec",
		Title:       "Spec test with minimal budget",
		Description: "Test spec-id inclusion.",
		Acceptance:  "Spec content omitted with minimal budget",
		Labels:      []string{"test"},
	}
	b.Extra = map[string]any{"spec-id": "FEAT-TEST"}

	arts, err := createArtifactBundle(root, wt, "20260101T000000-spec00001")
	if err != nil {
		t.Fatalf("createArtifactBundle: %v", err)
	}

	const baseRev = "1234abcd"

	// Full prompt (should resolve spec)
	promptFull, _, err := buildPrompt(root, b, []executeBeadGoverningRef{}, arts, baseRev, "", "claude", "")
	if err != nil {
		t.Fatalf("buildPrompt (full): %v", err)
	}

	// Minimal prompt (should NOT include spec content)
	promptMinimal, _, err := buildPrompt(root, b, []executeBeadGoverningRef{}, arts, baseRev, "", "claude", "minimal")
	if err != nil {
		t.Fatalf("buildPrompt (minimal): %v", err)
	}

	fullStr := string(promptFull)
	minimalStr := string(promptMinimal)

	// Check that minimal is smaller
	if len(minimalStr) >= len(fullStr) {
		t.Errorf("minimal budget prompt (%d bytes) should be smaller than full (%d bytes)", len(minimalStr), len(fullStr))
	}

	// Both should have the spec-id metadata
	if !strings.Contains(minimalStr, "FEAT-TEST") {
		t.Errorf("prompt should include spec-id in metadata")
	}

	// Full prompt may or may not include the spec content (depends on ref resolution)
	// but minimal should definitely NOT include it
	if strings.Contains(minimalStr, "large specification document") {
		t.Errorf("minimal budget prompt should not include spec content")
	}
}

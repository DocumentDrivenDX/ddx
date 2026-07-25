package spechonesty

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestVerificationMappingParser(t *testing.T) {
	fixtureRoot := filepath.Join("testdata", "docs")
	before := snapshotFixtures(t, fixtureRoot)
	if len(before) == 0 {
		t.Fatalf("expected markdown fixtures under %s", fixtureRoot)
	}

	t.Run("requirement_ids_in_order", func(t *testing.T) {
		path := filepath.Join(fixtureRoot, "with_requirement_ids.md")
		model, err := ParseVerificationDocument(path)
		if err != nil {
			t.Fatalf("ParseVerificationDocument: %v", err)
		}
		wantInv := []string{"REQ-001", "REQ-002", "REQ-010"}
		if !reflect.DeepEqual(model.Inventory, wantInv) {
			t.Fatalf("inventory = %#v, want %#v", model.Inventory, wantInv)
		}
		if model.InventoryKind != InventoryRequirementIDs {
			t.Fatalf("InventoryKind = %q, want %q", model.InventoryKind, InventoryRequirementIDs)
		}
	})

	t.Run("section_anchors_when_no_requirement_ids", func(t *testing.T) {
		path := filepath.Join(fixtureRoot, "section_anchors_only.md")
		model, err := ParseVerificationDocument(path)
		if err != nil {
			t.Fatalf("ParseVerificationDocument: %v", err)
		}
		wantInv := []string{"overview", "architecture", "data-model"}
		if !reflect.DeepEqual(model.Inventory, wantInv) {
			t.Fatalf("inventory = %#v, want %#v", model.Inventory, wantInv)
		}
		if model.InventoryKind != InventorySectionAnchors {
			t.Fatalf("InventoryKind = %q, want %q", model.InventoryKind, InventorySectionAnchors)
		}
	})

	t.Run("well_formed_verification_rows", func(t *testing.T) {
		path := filepath.Join(fixtureRoot, "with_requirement_ids.md")
		model, err := ParseVerificationDocument(path)
		if err != nil {
			t.Fatalf("ParseVerificationDocument: %v", err)
		}
		if len(model.Findings) != 0 {
			t.Fatalf("unexpected findings: %+v", model.Findings)
		}
		if len(model.Rows) != 3 {
			t.Fatalf("len(Rows) = %d, want 3; rows=%+v", len(model.Rows), model.Rows)
		}
		want := []VerificationRow{
			{
				RequirementRef: "REQ-001",
				EvidenceTarget: "TestCreateResource",
				Command:        "cd cli && go test ./pkg -run TestCreateResource",
			},
			{
				RequirementRef: "REQ-002",
				EvidenceTarget: "TestListResources",
				Command:        "cd cli && go test ./pkg -run TestListResources",
			},
			{
				RequirementRef: "REQ-010",
				EvidenceTarget: "check:static-delete",
				Command:        "go run ./tools/lint/deletecheck",
			},
		}
		for i, got := range model.Rows {
			if got.RequirementRef != want[i].RequirementRef ||
				got.EvidenceTarget != want[i].EvidenceTarget ||
				got.Command != want[i].Command {
				t.Fatalf("row[%d] = {%q, %q, %q}, want {%q, %q, %q}",
					i,
					got.RequirementRef, got.EvidenceTarget, got.Command,
					want[i].RequirementRef, want[i].EvidenceTarget, want[i].Command,
				)
			}
			if got.Line <= 0 {
				t.Fatalf("row[%d].Line = %d, want > 0", i, got.Line)
			}
		}

		// Section-anchor fixture rows also expose all three fields.
		anchorPath := filepath.Join(fixtureRoot, "section_anchors_only.md")
		anchorModel, err := ParseVerificationDocument(anchorPath)
		if err != nil {
			t.Fatalf("ParseVerificationDocument(anchors): %v", err)
		}
		if len(anchorModel.Rows) != 3 {
			t.Fatalf("anchor rows = %d, want 3", len(anchorModel.Rows))
		}
		if anchorModel.Rows[2].EvidenceTarget != ".ddx/executions/fixture/report.json" {
			t.Fatalf("artifact evidence = %q", anchorModel.Rows[2].EvidenceTarget)
		}
	})

	t.Run("malformed_rows_produce_parse_findings", func(t *testing.T) {
		path := filepath.Join(fixtureRoot, "malformed_rows.md")
		model, err := ParseVerificationDocument(path)
		if err != nil {
			t.Fatalf("ParseVerificationDocument: %v", err)
		}

		// One well-formed row should still be present.
		if len(model.Rows) != 1 {
			t.Fatalf("len(Rows) = %d, want 1 (only the well-formed row); rows=%+v", len(model.Rows), model.Rows)
		}
		if model.Rows[0].RequirementRef != "REQ-100" {
			t.Fatalf("well-formed row ref = %q, want REQ-100", model.Rows[0].RequirementRef)
		}

		kinds := map[ParseFindingKind]int{}
		for _, f := range model.Findings {
			kinds[f.Kind]++
			if f.Path != path {
				t.Fatalf("finding path = %q, want %q", f.Path, path)
			}
			if f.Line <= 0 {
				t.Fatalf("finding line = %d, want > 0", f.Line)
			}
			if f.Message == "" {
				t.Fatalf("finding missing message: %+v", f)
			}
		}
		if kinds[FindingMissingRequirementRef] != 1 {
			t.Fatalf("missing_requirement_ref findings = %d, want 1; all=%+v", kinds[FindingMissingRequirementRef], model.Findings)
		}
		if kinds[FindingMissingEvidenceTarget] != 1 {
			t.Fatalf("missing_evidence_target findings = %d, want 1; all=%+v", kinds[FindingMissingEvidenceTarget], model.Findings)
		}
		if kinds[FindingMissingCommand] != 1 {
			t.Fatalf("missing_command findings = %d, want 1; all=%+v", kinds[FindingMissingCommand], model.Findings)
		}
	})

	// AC2: parser is read-only — no fixture file may be modified.
	after := snapshotFixtures(t, fixtureRoot)
	if diffs := diffFixtures(before, after); len(diffs) > 0 {
		t.Fatalf("verification mapping parser mutated fixtures:\n%s", joinLines(diffs))
	}
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}
		out += l
	}
	return out
}

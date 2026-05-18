package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFEAT010DescribesForwardProgressPolicy(t *testing.T) {
	path := filepath.Join("..", "..", "docs/helix/01-frame/features/FEAT-010-task-execution.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read FEAT-010")
	feat010 := string(data)

	// Verify the section exists and contains key concepts
	for _, want := range []string{
		"Forward-Progress Readiness Policy",
		"Open is the execution lane",
		"Proposed is the operator-decision escape",
		"Operator-promoted beads are durable",
		"operator promotion",
		"operator-promotion non-regression",
		"prevents readiness from recreating a proposed→open→proposed",
	} {
		if !strings.Contains(feat010, want) {
			t.Errorf("FEAT-010 missing required phrase: %q", want)
		}
	}

	// Verify the section forbids readiness downgrade
	if !strings.Contains(feat010, "durable") {
		t.Error("FEAT-010 must clearly state that operator-promoted beads are durable")
	}
}

func TestADR023CodifiesOperatorAcceptanceSignal(t *testing.T) {
	path := filepath.Join("..", "..", "docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md")
	data, err := os.ReadFile(path)
	require.NoError(t, err, "read ADR-023")
	adr023 := string(data)

	// Verify WARN-ONLY default is documented
	for _, want := range []string{
		"WARN-ONLY",
		"default",
		"proceed",
	} {
		if !strings.Contains(adr023, want) {
			t.Errorf("ADR-023 missing WARN-ONLY default documentation: %q", want)
		}
	}

	// Verify operator-acceptance signal is codified as durable
	for _, want := range []string{
		"Operator-Promotion Non-Regression Clause",
		"durable override",
		"must honor",
		"operator-promoted",
		"status=open",
		"status=proposed",
		"prevents readiness from recreating a proposed→open→proposed",
	} {
		if !strings.Contains(adr023, want) {
			t.Errorf("ADR-023 missing operator-acceptance signal specification: %q", want)
		}
	}

	// Verify the non-regression clause is explicit
	if !strings.Contains(adr023, "Non-regression requirement") {
		t.Error("ADR-023 must explicitly state the non-regression requirement")
	}
}

func TestFEAT010ADR023CrossLink(t *testing.T) {
	feat010Path := filepath.Join("..", "..", "docs/helix/01-frame/features/FEAT-010-task-execution.md")
	feat010Data, err := os.ReadFile(feat010Path)
	require.NoError(t, err, "read FEAT-010")
	feat010 := string(feat010Data)

	adr023Path := filepath.Join("..", "..", "docs/helix/02-design/adr/ADR-023-bead-lifecycle-quality-policy.md")
	adr023Data, err := os.ReadFile(adr023Path)
	require.NoError(t, err, "read ADR-023")
	adr023 := string(adr023Data)

	// Verify FEAT-010 references ADR-023
	if !strings.Contains(feat010, "ADR-023") {
		t.Error("FEAT-010 must reference ADR-023 for the forward-progress readiness policy")
	}

	// Verify ADR-023 references FEAT-010
	if !strings.Contains(adr023, "FEAT-010") {
		t.Error("ADR-023 must reference FEAT-010 for the forward-progress readiness policy")
	}

	// Verify both docs mention the key concepts together
	if !strings.Contains(feat010, "Forward-Progress Readiness Policy") {
		t.Error("FEAT-010 must contain the 'Forward-Progress Readiness Policy' section")
	}
	if !strings.Contains(adr023, "Non-Regression") {
		t.Error("ADR-023 must contain 'Non-Regression' clause documentation")
	}
}

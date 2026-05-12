package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDocCommandHelpIncludesProse(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()

	output, err := env.RunCommand("doc", "--help")
	require.NoError(t, err)
	if !strings.Contains(output, "prose") {
		t.Fatalf("expected doc help to mention prose subcommand, got: %q", output)
	}
}

func TestDocProseCommandChangedAdvisoryDefault(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: file:///tmp/ddx-library
    branch: master
persona_bindings: {}
`)
	env.CreateFile("docs/helix/guide.md", "# Guide\n\nThis is robust and comprehensive.\n")

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)

	if !strings.Contains(output, "docs/helix/guide.md:3 [advisory] prose.claim.unsupported") {
		t.Fatalf("expected advisory prose finding, got: %q", output)
	}
	if !strings.Contains(output, "rationale:") || !strings.Contains(output, "suggested edit:") {
		t.Fatalf("expected rationale and suggested edit fields, got: %q", output)
	}
}

func TestDocProseCommandModeAffectsFindings(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: file:///tmp/ddx-library
    branch: master
persona_bindings: {}
prose:
  mode: planning
`)
	env.CreateFile("docs/helix/roadmap.md", `# Planning Notes

We need a robust roadmap that creates a seamless transition and keeps momentum high.

First, we should align stakeholders. First, we should align stakeholders.

In the next phase, the team will focus on the most important work and other strategic items.
`)

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)

	if !strings.Contains(output, "prose.planning.claims") {
		t.Fatalf("expected planning claims finding, got: %q", output)
	}
	if !strings.Contains(output, "prose.planning.vagueness") {
		t.Fatalf("expected planning vagueness finding, got: %q", output)
	}
	if strings.Contains(output, "prose.claim.unsupported") {
		t.Fatalf("did not expect technical-mode generic claims finding, got: %q", output)
	}
}

func TestDocProseCommandVocabularyConfigAffectsFindings(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: file:///tmp/ddx-library
    branch: master
persona_bindings: {}
prose:
  vocabulary:
    accept:
      - Quartz
    reject:
      - system
`)
	env.CreateFile("docs/helix/vocab.md", "# Vocab\n\nQuartz keeps the system solution honest.\n")

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)

	if !strings.Contains(output, "prose.vocabulary.reject") {
		t.Fatalf("expected vocabulary reject finding, got: %q", output)
	}
	if strings.Count(output, "prose.vocabulary.reject") != 1 {
		t.Fatalf("expected exactly one vocabulary finding, got: %q", output)
	}
	if strings.Contains(output, "Quartz") {
		t.Fatalf("accepted vocabulary term should not be flagged, got: %q", output)
	}
}

func TestDocProseCommandBlockingPolicyExitsNonZero(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: file:///tmp/ddx-library
    branch: master
persona_bindings: {}
prose:
  policy: blocking
`)
	env.CreateFile("docs/helix/blocking.md", "# Guide\n\nThis is robust and comprehensive.\n")

	output, err := env.RunCommand("doc", "prose", "--changed")
	if err == nil {
		t.Fatal("expected blocking prose policy to return an exit error")
	}
	exitErr, ok := err.(*ExitError)
	if !ok {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitCodeGeneralError {
		t.Fatalf("expected exit code %d, got %d", ExitCodeGeneralError, exitErr.Code)
	}
	if !strings.Contains(output, "docs/helix/blocking.md:3") {
		t.Fatalf("expected blocking finding output, got: %q", output)
	}
}

func TestDocProseCommandMissingRunnerReportsDiagnostic(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	gitPath := mustLookPath(t, "git")
	binDir := t.TempDir()
	require.NoError(t, os.Symlink(gitPath, filepath.Join(binDir, "git")))
	t.Setenv("PATH", binDir)

	env := NewTestEnvironment(t)
	env.CreateConfig(`version: "1.0"
library:
  path: .ddx/plugins/ddx
  repository:
    url: file:///tmp/ddx-library
    branch: master
persona_bindings: {}
prose:
  runner: vale
`)
	env.CreateFile("docs/helix/runner.md", "# Guide\n\nThis is robust and comprehensive.\n")

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)
	if !strings.Contains(output, `warning: optional prose runner "vale" is unavailable; using embedded checker`) {
		t.Fatalf("expected missing-runner diagnostic, got: %q", output)
	}
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	found, err := exec.LookPath(name)
	require.NoError(t, err)
	return found
}

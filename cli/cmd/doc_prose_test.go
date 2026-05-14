package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// installFakeVale writes a shell script named "vale" into a temp bin dir that
// returns the given JSON output (for the linting invocation) and reports a
// supported version when invoked with --version. The directory also exposes
// the real git binary so that ddx subcommands that shell out to git keep
// working. The returned binDir is set as PATH for the test.
func installFakeVale(t *testing.T, lintJSON string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture uses sh")
	}
	binDir := t.TempDir()

	// Re-expose the real git binary on the constrained PATH so git status
	// inside the changed-paths helper still works.
	gitPath := mustLookPath(t, "git")
	require.NoError(t, os.Symlink(gitPath, filepath.Join(binDir, "git")))

	scriptPath := filepath.Join(binDir, "vale")
	// The script must rely only on shell built-ins because tests set PATH
	// to binDir, hiding coreutils like cat.
	escaped := strings.ReplaceAll(lintJSON, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `%`, `%%`)
	escaped = strings.ReplaceAll(escaped, `'`, `'\''`)
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  printf '%s\\n' 'vale version 3.13.0'\n" +
		"  exit 0\n" +
		"fi\n" +
		"printf '%s\\n' '" + escaped + "'\n" +
		"exit 1\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return binDir
}

func TestDocProseCommand_ChangedUsesValeBackedEngine(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	relPath := "docs/helix/guide.md"
	env.CreateFile(relPath, "# Guide\n\nThis is robust and comprehensive.\n")

	absPath := filepath.Join(env.Dir, relPath)
	alertJSON := fmt.Sprintf(`{%q:[{"Check":"DDx.UnsupportedClaim","Line":3,"Span":[9,15],"Severity":"warning","Message":"raw vale message","Match":"robust"}]}`, absPath)
	t.Setenv("PATH", installFakeVale(t, alertJSON))

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)

	if !strings.Contains(output, "docs/helix/guide.md:3") {
		t.Fatalf("expected vale-backed finding to be reported with the relative file path, got: %q", output)
	}
	if !strings.Contains(output, "prose.claim.unsupported") {
		t.Fatalf("expected DDx rule id from normalized Vale alert, got: %q", output)
	}
	if strings.Contains(output, "using embedded checker") {
		t.Fatalf("expected vale-backed engine, but embedded fallback warning appeared: %q", output)
	}
}

func TestDocProseCommand_PathModeUsesValeBackedEngine(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	relPath := "docs/helix/explicit.md"
	env.CreateFile(relPath, "# Explicit\n\nThis is robust and comprehensive.\n")

	absPath := filepath.Join(env.Dir, relPath)
	alertJSON := fmt.Sprintf(`{%q:[{"Check":"DDx.TokenCost","Line":3,"Span":[1,14],"Severity":"warning","Message":"raw vale message","Match":"very important"}]}`, absPath)
	t.Setenv("PATH", installFakeVale(t, alertJSON))

	output, err := env.RunCommand("doc", "prose", relPath)
	require.NoError(t, err)

	if !strings.Contains(output, "docs/helix/explicit.md:3") {
		t.Fatalf("expected vale-backed finding for explicit path, got: %q", output)
	}
	if !strings.Contains(output, "prose.cost.filler") {
		t.Fatalf("expected DDx rule id from normalized Vale alert, got: %q", output)
	}
	if strings.Contains(output, "using embedded checker") {
		t.Fatalf("expected vale-backed engine, but embedded fallback warning appeared: %q", output)
	}
}

func TestDocProseCommand_NoProjectValeINIRequired(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	relPath := "docs/helix/no-ini.md"
	env.CreateFile(relPath, "# No INI\n\nThis is robust and comprehensive.\n")

	// Sanity check: the test environment must not ship a project .vale.ini.
	if _, err := os.Stat(filepath.Join(env.Dir, ".vale.ini")); !os.IsNotExist(err) {
		t.Fatalf("expected no project .vale.ini in test environment, stat err = %v", err)
	}

	absPath := filepath.Join(env.Dir, relPath)
	alertJSON := fmt.Sprintf(`{%q:[{"Check":"DDx.UnsupportedClaim","Line":3,"Span":[9,15],"Severity":"warning","Message":"raw vale message","Match":"robust"}]}`, absPath)
	t.Setenv("PATH", installFakeVale(t, alertJSON))

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)

	if !strings.Contains(output, "prose.claim.unsupported") {
		t.Fatalf("expected DDx rule id from normalized Vale alert without project .vale.ini, got: %q", output)
	}
	if strings.Contains(output, "using embedded checker") {
		t.Fatalf("expected vale-backed engine to run without project config, got fallback: %q", output)
	}
	if _, err := os.Stat(filepath.Join(env.Dir, ".vale.ini")); !os.IsNotExist(err) {
		t.Fatalf("doc prose must not write a project .vale.ini: %v", err)
	}
}

func TestDocProseCommand_OutputUsesDDxRuleIDs(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")

	env := NewTestEnvironment(t)
	env.CreateDefaultConfig()
	relPath := "docs/helix/rule-ids.md"
	env.CreateFile(relPath, "# Rule IDs\n\nThis is robust.\nWe must streamline things.\n")

	absPath := filepath.Join(env.Dir, relPath)
	alertJSON := fmt.Sprintf(`{%q:[`+
		`{"Check":"DDx.UnsupportedClaim","Line":3,"Span":[9,15],"Severity":"warning","Message":"raw vale message","Match":"robust"},`+
		`{"Check":"DDx.MissingActorAction","Line":4,"Span":[9,19],"Severity":"warning","Message":"raw vale message","Match":"streamline"}`+
		`]}`, absPath)
	t.Setenv("PATH", installFakeVale(t, alertJSON))

	output, err := env.RunCommand("doc", "prose", "--changed")
	require.NoError(t, err)

	if !strings.Contains(output, "prose.claim.unsupported") {
		t.Fatalf("expected DDx rule id prose.claim.unsupported, got: %q", output)
	}
	if !strings.Contains(output, "prose.specificity.actor_action") {
		t.Fatalf("expected DDx rule id prose.specificity.actor_action, got: %q", output)
	}
	for _, raw := range []string{"DDx.UnsupportedClaim", "DDx.MissingActorAction", "raw vale message"} {
		if strings.Contains(output, raw) {
			t.Fatalf("raw Vale name %q leaked into user-facing output: %q", raw, output)
		}
	}
}

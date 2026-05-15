package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
	gitpkg "github.com/DocumentDrivenDX/ddx/internal/git"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheckVersionGateBlocksOldBinaryAgainstNewerProject is the ddx-9655497c
// acceptance test for FEAT-015 AC-007: a project that records a newer
// required DDx version in .ddx/versions.yaml must refuse to run against an
// older binary, with actionable guidance.
func TestCheckVersionGateBlocksOldBinaryAgainstNewerProject(t *testing.T) {
	workDir := t.TempDir()
	// Project insists on v0.9.0; binary reports v0.6.0.
	writeProjectVersion(t, workDir, "0.9.0")

	factory := NewCommandFactory(workDir)
	factory.Version = "0.6.0"

	err := factory.checkVersionGate(&cobra.Command{Use: "list"}) // exempt list isn't one of the recovery names
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0.9.0")
	assert.Contains(t, err.Error(), "0.6.0")
	assert.Contains(t, err.Error(), "ddx upgrade")
}

// TestCheckVersionGateAllowsNewerBinary covers the common case: the
// operator upgraded first. Binary >= project = no block.
func TestCheckVersionGateAllowsNewerBinary(t *testing.T) {
	workDir := t.TempDir()
	writeProjectVersion(t, workDir, "0.6.0")

	factory := NewCommandFactory(workDir)
	factory.Version = "0.9.0"

	require.NoError(t, factory.checkVersionGate(&cobra.Command{Use: "list"}))
}

// TestCheckVersionGateSkipsWhenNoVersionsFile covers a fresh project: no
// .ddx/versions.yaml, no gate.
func TestCheckVersionGateSkipsWhenNoVersionsFile(t *testing.T) {
	workDir := t.TempDir()
	factory := NewCommandFactory(workDir)
	factory.Version = "0.6.0"
	require.NoError(t, factory.checkVersionGate(&cobra.Command{Use: "list"}))
}

// TestCheckVersionGateBypassedForDevBinary keeps dev builds unblocked so
// DDx engineers never trip the gate on in-flight work.
func TestCheckVersionGateBypassedForDevBinary(t *testing.T) {
	workDir := t.TempDir()
	writeProjectVersion(t, workDir, "0.9.0")

	factory := NewCommandFactory(workDir)
	factory.Version = "dev"
	require.NoError(t, factory.checkVersionGate(&cobra.Command{Use: "list"}))

	factory.Version = ""
	require.NoError(t, factory.checkVersionGate(&cobra.Command{Use: "list"}))
}

// TestCheckVersionGateExemptsRecoveryCommands lists the commands that must
// work against an old binary in a new project so the operator can recover
// ('upgrade' itself, 'doctor' to diagnose, 'version', etc.).
func TestCheckVersionGateExemptsRecoveryCommands(t *testing.T) {
	workDir := t.TempDir()
	writeProjectVersion(t, workDir, "0.9.0")

	factory := NewCommandFactory(workDir)
	factory.Version = "0.6.0"

	for _, name := range []string{"upgrade", "version", "doctor", "init", "help", "completion"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{Use: name}
			require.NoError(t, factory.checkVersionGate(cmd),
				"recovery command %q must be exempt from the gate", name)
		})
	}
}

func TestVersionWarnsStaleSourceBinary(t *testing.T) {
	t.Setenv("DDX_DISABLE_UPDATE_CHECK", "1")
	projectRoot, buildSHA, headSHA := seedStaleSourceCheckout(t)

	factory := NewCommandFactory(projectRoot)
	factory.Version = "0.9.0"
	factory.Commit = buildSHA

	out, err := executeCommand(factory.NewRootCommand(), "version")
	require.NoError(t, err)
	assert.Contains(t, out, "installed ddx binary is behind this DDx source checkout.")
	assert.Contains(t, out, "project root: "+projectRoot)
	assert.Contains(t, out, "binary commit: "+buildSHA)
	assert.Contains(t, out, "source HEAD: "+headSHA)
	assert.Contains(t, out, "recovery: cd "+projectRoot+" && make install")
}

func writeProjectVersion(t *testing.T, workDir, version string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(workDir, ddxroot.DirName), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(workDir, ddxroot.DirName, "versions.yaml"),
		[]byte("ddx_version: \""+version+"\"\n"),
		0o644,
	))
}

func seedStaleSourceCheckout(t *testing.T) (projectRoot, buildSHA, headSHA string) {
	t.Helper()

	projectRoot = minimalProjectDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# ddx\n"), 0o644))

	runStaleSourceGit(t, projectRoot, "init")
	runStaleSourceGit(t, projectRoot, "config", "user.email", "test@example.com")
	runStaleSourceGit(t, projectRoot, "config", "user.name", "Test User")
	runStaleSourceGit(t, projectRoot, "remote", "add", "origin", "git@github.com:DocumentDrivenDX/ddx.git")
	runStaleSourceGit(t, projectRoot, "add", ".")
	runStaleSourceGit(t, projectRoot, "commit", "-m", "initial source state")
	buildSHA = staleSourceGitOutput(t, projectRoot, "rev-parse", "HEAD")

	require.NoError(t, os.WriteFile(filepath.Join(projectRoot, "README.md"), []byte("# ddx\n\nsource ahead\n"), 0o644))
	runStaleSourceGit(t, projectRoot, "add", "README.md")
	runStaleSourceGit(t, projectRoot, "commit", "-m", "source ahead of installed binary")
	headSHA = staleSourceGitOutput(t, projectRoot, "rev-parse", "HEAD")

	return projectRoot, buildSHA, headSHA
}

func runStaleSourceGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func staleSourceGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitpkg.CleanEnv()
	out, err := cmd.Output()
	require.NoError(t, err, "git %v failed", args)
	return strings.TrimSpace(string(out))
}

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	agentlib "github.com/easel/fizeau"
)

// TestFizeauV015ModuleContract proves a helper binary that imports only the
// public github.com/easel/fizeau surface resolves that module at v0.15.0 with
// no replacement or pseudo-version.
func TestFizeauV015ModuleContract(t *testing.T) {
	t.Parallel()

	// Keep the public module linked through exported symbols only.
	var _ agentlib.FizeauService
	_ = agentlib.PortableRuntimeGuestRoot()
	_ = agentlib.ErrPortableRuntimeRequestInvalid

	helperBin := buildFizeauModuleContractHelper(t)
	output := goVersionM(t, helperBin)

	line, found := findModuleBuildInfoLine(output, "github.com/easel/fizeau")
	if !found {
		t.Fatalf("github.com/easel/fizeau missing from build info:\n%s", output)
	}
	if strings.Contains(line, "=>") {
		t.Fatalf("github.com/easel/fizeau resolved via replacement: %s", line)
	}
	if !strings.Contains(line, "\tv0.15.0\t") {
		t.Fatalf("github.com/easel/fizeau version = %q, want v0.15.0\nfull output:\n%s", line, output)
	}
	if strings.Contains(line, "v0.0.0-") {
		t.Fatalf("github.com/easel/fizeau resolved to pseudo-version: %s", line)
	}
}

func buildFizeauModuleContractHelper(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	const helperSource = `package main

import _ "github.com/easel/fizeau"

func main() {}
`
	if err := os.WriteFile(src, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	bin := filepath.Join(dir, "helper")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin
}

func goVersionM(t *testing.T, bin string) string {
	t.Helper()

	cmd := exec.Command("go", "version", "-m", bin)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go version -m %s: %v\n%s", bin, err, out)
	}
	return string(out)
}

func findModuleBuildInfoLine(output, modulePath string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, modulePath) {
			return line, true
		}
	}
	return "", false
}

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	agentlib "github.com/easel/fizeau"
)

const pinnedFizeauModuleVersion = "v0.17.3"

// TestFizeauV015ModuleContract proves a helper binary that imports only the
// public github.com/easel/fizeau surface resolves that module at v0.17.3 with
// no replacement or pseudo-version. v0.17.2+ includes the claude-tui Stop-hook
// flush-race wait (waitForAuthoritativeTranscript); workers on v0.15.2 fail
// completed turns with "no assistant final event".
func TestFizeauV015ModuleContract(t *testing.T) {
	t.Parallel()

	// Keep the public module linked through exported symbols only.
	var _ agentlib.FizeauService
	_ = agentlib.PortableRuntimeGuestRoot()
	_ = agentlib.ErrPortableRuntimeRequestInvalid

	helperBin := buildFizeauModuleContractHelper(t)
	output := goVersionM(t, helperBin)

	line, nextLine, found := findModuleBuildInfoLine(output, "github.com/easel/fizeau")
	if !found {
		t.Fatalf("github.com/easel/fizeau missing from build info:\n%s", output)
	}
	if replacementLine := findReplacementLine(line, nextLine); replacementLine != "" {
		t.Fatalf("github.com/easel/fizeau resolved via replacement: %s", replacementLine)
	}
	if version, ok := moduleVersion(line, "github.com/easel/fizeau"); !ok {
		t.Fatalf("github.com/easel/fizeau version field missing: %q\nfull output:\n%s", line, output)
	} else if version != pinnedFizeauModuleVersion {
		t.Fatalf("github.com/easel/fizeau version = %q, want %s\nfull output:\n%s", version, pinnedFizeauModuleVersion, output)
	} else if strings.HasPrefix(version, "v0.0.0-") {
		t.Fatalf("github.com/easel/fizeau resolved to pseudo-version: %s", line)
	}
}

func buildFizeauModuleContractHelper(t *testing.T) string {
	t.Helper()

	modRoot := cliModuleRoot(t)
	dir := t.TempDir()
	if _, err := copyFile(filepath.Join(modRoot, "go.mod"), filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("copy go.mod: %v", err)
	}
	if _, err := copyFile(filepath.Join(modRoot, "go.sum"), filepath.Join(dir, "go.sum")); err != nil {
		t.Fatalf("copy go.sum: %v", err)
	}

	src := filepath.Join(dir, "main.go")
	const helperSource = `package main

import _ "github.com/easel/fizeau"

func main() {}
`
	if err := os.WriteFile(src, []byte(helperSource), 0o600); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	bin := filepath.Join(dir, "helper")
	cmd := exec.Command("go", "build", "-mod=readonly", "-buildvcs=false", "-o", bin, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}
	return bin
}

func cliModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(b), "module github.com/DocumentDrivenDX/ddx") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("cli module root not found")
		}
		dir = parent
	}
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

func findModuleBuildInfoLine(output, modulePath string) (string, string, bool) {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		if strings.Contains(line, modulePath) {
			nextLine := ""
			if i+1 < len(lines) {
				nextLine = lines[i+1]
			}
			return line, nextLine, true
		}
	}
	return "", "", false
}

func findReplacementLine(line, nextLine string) string {
	if strings.Contains(line, "=>") {
		return line
	}
	if strings.HasPrefix(strings.TrimSpace(nextLine), "=>") {
		return nextLine
	}
	return ""
}

func moduleVersion(line, modulePath string) (string, bool) {
	fields := strings.Fields(line)
	for i := 0; i < len(fields); i++ {
		if fields[i] == modulePath && i+1 < len(fields) {
			return fields[i+1], true
		}
	}
	return "", false
}

package spechonesty_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type lefthookCommand struct {
	Run string `yaml:"run"`
}

type lefthookHook struct {
	Commands map[string]lefthookCommand `yaml:"commands"`
}

type lefthookConfig struct {
	PreCommit lefthookHook `yaml:"pre-commit"`
	CI        lefthookHook `yaml:"ci"`
}

func readLefthookConfig(t *testing.T) lefthookConfig {
	t.Helper()
	root := filepath.Join(moduleRoot(t), "..")
	data, err := os.ReadFile(filepath.Join(root, "lefthook.yml"))
	if err != nil {
		t.Fatalf("read lefthook.yml: %v", err)
	}
	var cfg lefthookConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal lefthook.yml: %v", err)
	}
	return cfg
}

func findRunContaining(t *testing.T, commands map[string]lefthookCommand, needle string) string {
	t.Helper()
	for name, cmd := range commands {
		if strings.Contains(cmd.Run, needle) {
			return cmd.Run
		}
		t.Logf("hook %q run does not contain %q", name, needle)
	}
	t.Fatalf("no hook run contained %q", needle)
	return ""
}

// TestSpecHonestyLefthookWiringPresent verifies lefthook wires the
// spec-honesty gate into both pre-commit and CI and targets the docs/helix
// corpus via the spechonesty command.
func TestSpecHonestyLefthookWiringPresent(t *testing.T) {
	cfg := readLefthookConfig(t)
	if len(cfg.PreCommit.Commands) == 0 {
		t.Fatal("pre-commit commands missing from lefthook.yml")
	}
	if len(cfg.CI.Commands) == 0 {
		t.Fatal("ci commands missing from lefthook.yml")
	}

	preCommitRun := findRunContaining(t, cfg.PreCommit.Commands, "go run ./tools/lint/spechonesty")
	if !strings.Contains(preCommitRun, "../docs/helix") {
		t.Fatalf("pre-commit spechonesty run must target ../docs/helix; got:\n%s", preCommitRun)
	}
	if !strings.Contains(preCommitRun, "git diff --cached") {
		t.Fatalf("pre-commit spechonesty run must be scoped to staged docs; got:\n%s", preCommitRun)
	}

	ciRun := findRunContaining(t, cfg.CI.Commands, "go run ./tools/lint/spechonesty")
	if !strings.Contains(ciRun, "../docs/helix") {
		t.Fatalf("ci spechonesty run must target ../docs/helix; got:\n%s", ciRun)
	}
}

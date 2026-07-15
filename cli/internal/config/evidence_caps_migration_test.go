package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

func TestLoadConfigHardErrorsOnHarnessKeyedEvidenceCaps(t *testing.T) {
	for _, value := range []string{"{claude: {max_prompt_bytes: 100}}", "{}", "null"} {
		value := value
		t.Run(strings.ReplaceAll(value, " ", "_"), func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			configDir := filepath.Join(root, ddxroot.DirName)
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(configDir, "config.yaml")
			contents := `version: "1.0"
library:
  path: ./library
  repository:
    url: https://example.invalid/library
    branch: main
evidence_caps:
  per_harness: ` + value + "\n"
			if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}

			loader, err := NewConfigLoaderWithWorkingDir(root)
			if err != nil {
				t.Fatal(err)
			}
			entrypoints := map[string]func() error{
				"Load":       func() error { _, err := Load(); return err },
				"LoadConfig": func() error { _, err := loader.LoadConfig(); return err },
				"LoadConfigFromPath": func() error {
					_, err := loader.LoadConfigFromPath(filepath.Join(ddxroot.DirName, "config.yaml"))
					return err
				},
				"LoadFromFileRelative": func() error {
					_, err := LoadFromFile(filepath.Join(ddxroot.DirName, "config.yaml"))
					return err
				},
				"LoadFromFileAbsolute": func() error { _, err := LoadFromFile(configPath); return err },
				"LoadWithWorkingDir":   func() error { _, err := LoadWithWorkingDir(root); return err },
				"LoadAndResolve": func() error {
					_, err := LoadAndResolve(root, CLIOverrides{})
					return err
				},
			}
			for name, load := range entrypoints {
				t.Run(name, func(t *testing.T) {
					err := load()
					if err == nil {
						t.Fatal("expected migration error")
					}
					var migrationErr *EvidenceCapsMigrationError
					if !errors.As(err, &migrationErr) {
						t.Fatalf("expected EvidenceCapsMigrationError, got %T: %v", err, err)
					}
					if migrationErr.Field != "evidence_caps.per_harness" || migrationErr.Path != configPath {
						t.Fatalf("migration error = %+v, want field and absolute path %s", migrationErr, configPath)
					}
					for _, want := range []string{"per_role", "implementer", "reviewer", "lifecycle", "Fizeau"} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q does not contain %q", err, want)
						}
					}
				})
			}
		})
	}
}

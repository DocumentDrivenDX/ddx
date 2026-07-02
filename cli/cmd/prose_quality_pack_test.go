package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/docprose"
	"github.com/DocumentDrivenDX/ddx/internal/registry"
	"gopkg.in/yaml.v3"
)

func TestProseQualityCheckMetadata_LoadsValeStylePack(t *testing.T) {
	settings, err := docprose.DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	if settings.Vale.Version != "3.13.0" || settings.Vale.StylesPath != "styles" || settings.Vale.Style != "DDx" {
		t.Fatalf("unexpected Vale metadata: %+v", settings.Vale)
	}
	if settings.StylePack.Name != "DDx" || settings.StylePack.Version == "" || settings.StylePack.Path != "styles/DDx" {
		t.Fatalf("unexpected style-pack metadata: %+v", settings.StylePack)
	}

	wantRules := map[string]string{
		"DDx/UnsupportedClaim.yml":   "prose.claim.unsupported",
		"DDx/AISlop.yml":             "prose.ai_slop.polish",
		"DDx/FillerTransition.yml":   "prose.filler.transition",
		"DDx/MissingActorAction.yml": "prose.specificity.actor_action",
		"DDx/TokenCost.yml":          "prose.cost.filler",
		"DDx/RepeatedOpening.yml":    "prose.structure.repeated_opening",
		"DDx/Vocabulary.yml":         "prose.vocabulary.generic_substitute",
	}
	gotRules := make(map[string]string, len(settings.StylePack.Rules))
	for _, rule := range settings.StylePack.Rules {
		gotRules[rule.File] = rule.RuleID
	}
	for file, ruleID := range wantRules {
		if gotRules[file] != ruleID {
			t.Fatalf("style rule %s = %q, want %q (all rules: %+v)", file, gotRules[file], ruleID, gotRules)
		}
	}

	if settings.Vocabulary.DefaultPath != "vocabulary/default.yaml" || settings.Vocabulary.ValeStyle != "DDx/Vocabulary.yml" {
		t.Fatalf("unexpected vocabulary plumbing metadata: %+v", settings.Vocabulary)
	}
	if !containsAllStrings(settings.Vocabulary.Accept, []string{"DDx", "bead", "execution", "Helix", "Fizeau"}) {
		t.Fatalf("default accept vocabulary missing DDx terms: %+v", settings.Vocabulary.Accept)
	}
}

func TestProseQualityStylePack_PackagedPathsExist(t *testing.T) {
	root := repoRootForProseQualityTest(t)
	settings, err := docprose.DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}

	for _, rule := range settings.StylePack.Rules {
		path := filepath.Join(root, "library", "checks", "prose-quality", "styles", filepath.FromSlash(rule.File))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected packaged style file %s: %v", path, err)
		}
		var parsed struct {
			Extends string            `yaml:"extends"`
			Message string            `yaml:"message"`
			Level   string            `yaml:"level"`
			Tokens  []string          `yaml:"tokens"`
			Swap    map[string]string `yaml:"swap"`
		}
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("parse style file %s: %v", path, err)
		}
		if parsed.Extends == "" || parsed.Message == "" || parsed.Level == "" {
			t.Fatalf("style file %s missing skeleton fields: %+v", path, parsed)
		}
		if len(parsed.Tokens) == 0 && len(parsed.Swap) == 0 {
			t.Fatalf("style file %s must define placeholder tokens or substitutions: %+v", path, parsed)
		}
	}
}

func TestProseQualityStylePack_ShipsThroughDefaultLibrary(t *testing.T) {
	root := repoRootForProseQualityTest(t)

	builtin, err := registry.BuiltinRegistry().Find("ddx")
	if err != nil {
		t.Fatal(err)
	}
	if builtin.Install.Root == nil || builtin.Install.Root.Source != "library" || builtin.Install.Root.Target != ".ddx/plugins/ddx" {
		t.Fatalf("built-in ddx package does not ship the library root: %+v", builtin.Install.Root)
	}

	manifest, issues, err := registry.LoadPackageManifest(filepath.Join(root, "library"))
	if err != nil {
		t.Fatalf("load library manifest: %v", err)
	}
	if len(issues) > 0 {
		t.Fatalf("library manifest validation issues: %+v", issues)
	}
	// The canonical library/package.yaml is package-rooted: install.root.source
	// is "." relative to the library/ package root. Resolving the rule paths
	// therefore goes through library/<source>.
	if manifest.Install.Root == nil || manifest.Install.Root.Source != "." || manifest.Install.Root.Target != ".ddx/plugins/ddx" {
		t.Fatalf("library manifest does not ship the package root: %+v", manifest.Install.Root)
	}

	settings, err := docprose.DefaultSettings()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range settings.StylePack.Rules {
		path := filepath.Join(root, "library", manifest.Install.Root.Source, "checks", "prose-quality", "styles", filepath.FromSlash(rule.File))
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("style file %s is outside the default library install root: %v", path, err)
		}
	}
}

func repoRootForProseQualityTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime caller unavailable")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func containsAllStrings(got, want []string) bool {
	seen := make(map[string]bool, len(got))
	for _, value := range got {
		seen[value] = true
	}
	for _, value := range want {
		if !seen[value] {
			return false
		}
	}
	return true
}

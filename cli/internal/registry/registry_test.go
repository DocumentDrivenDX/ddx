package registry

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuiltinRegistry(t *testing.T) {
	r := BuiltinRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	pkg, err := r.Find("helix")
	if err != nil {
		t.Fatalf("expected helix package: %v", err)
	}

	if pkg.Name != "helix" {
		t.Errorf("expected name=helix, got %q", pkg.Name)
	}
	if pkg.Type != PackageTypeWorkflow {
		t.Errorf("expected type=workflow, got %q", pkg.Type)
	}
	if pkg.Version == "" {
		t.Error("expected non-empty version")
	}
	if pkg.Description == "" {
		t.Error("expected non-empty description")
	}
	if pkg.Source == "" {
		t.Error("expected non-empty source")
	}
	if pkg.Install.Skills == nil {
		t.Error("expected install.skills to be set")
	}
}

func TestBuiltinRegistry_DDxPackage(t *testing.T) {
	r := BuiltinRegistry()

	pkg, err := r.Find("ddx")
	if err != nil {
		t.Fatalf("expected ddx package: %v", err)
	}

	if pkg.Name != "ddx" {
		t.Errorf("expected name=ddx, got %q", pkg.Name)
	}
	if pkg.Type != PackageTypePlugin {
		t.Errorf("expected type=plugin, got %q", pkg.Type)
	}
	if pkg.Install.Root == nil {
		t.Fatal("expected install.root to be set")
	}
	if pkg.Install.Root.Source != "library" {
		t.Errorf("expected root source=library, got %q", pkg.Install.Root.Source)
	}
	if pkg.Install.Root.Target != ".ddx/plugins/ddx" {
		t.Errorf("expected root target=.ddx/plugins/ddx, got %q", pkg.Install.Root.Target)
	}
	// ddx plugin has no skills or scripts — it's just library resources
	if len(pkg.Install.Skills) != 0 {
		t.Errorf("expected no skills, got %d", len(pkg.Install.Skills))
	}
	if pkg.Install.Scripts != nil {
		t.Error("expected no scripts")
	}
}

func TestFind(t *testing.T) {
	r := BuiltinRegistry()

	pkg, err := r.Find("helix")
	if err != nil {
		t.Fatalf("expected to find helix: %v", err)
	}
	if pkg.Name != "helix" {
		t.Errorf("expected helix, got %q", pkg.Name)
	}

	ddxPkg, err := r.Find("ddx")
	if err != nil {
		t.Fatalf("expected to find ddx: %v", err)
	}
	if ddxPkg.Name != "ddx" {
		t.Errorf("expected ddx, got %q", ddxPkg.Name)
	}

	_, err = r.Find("nonexistent-package")
	if err == nil {
		t.Error("expected error for nonexistent package")
	}
	if !strings.Contains(err.Error(), "nonexistent-package") {
		t.Errorf("expected error to mention package name, got: %v", err)
	}
}

func TestSearch(t *testing.T) {
	r := BuiltinRegistry()

	// Match by name
	results := r.Search("helix")
	if len(results) == 0 {
		t.Error("expected results for 'helix' name search")
	}
	found := false
	for _, p := range results {
		if p.Name == "helix" {
			found = true
		}
	}
	if !found {
		t.Error("expected helix in name search results")
	}

	// Match by description
	results = r.Search("workflow")
	if len(results) == 0 {
		t.Error("expected results for 'workflow' description/type search")
	}

	// Match by keyword
	results = r.Search("methodology")
	if len(results) == 0 {
		t.Error("expected results for 'methodology' keyword search")
	}

	// No match
	results = r.Search("zzz-does-not-exist-zzz")
	if len(results) != 0 {
		t.Errorf("expected no results for non-matching query, got %d", len(results))
	}
}

func TestIsResourcePath(t *testing.T) {
	if !IsResourcePath("persona/foo") {
		t.Error("expected persona/foo to be a resource path")
	}
	if !IsResourcePath("template/my-template") {
		t.Error("expected template/my-template to be a resource path")
	}
	if IsResourcePath("helix") {
		t.Error("expected helix to NOT be a resource path")
	}
	if IsResourcePath("some-package") {
		t.Error("expected some-package to NOT be a resource path")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	result := ExpandHome("~/.agents/skills/")
	if !strings.HasPrefix(result, home) {
		t.Errorf("expected expanded path to start with %q, got %q", home, result)
	}
	if strings.HasPrefix(result, "~") {
		t.Error("expected ~ to be expanded")
	}

	// Non-~ path should be returned unchanged
	plain := "/absolute/path"
	if ExpandHome(plain) != plain {
		t.Errorf("expected unchanged absolute path, got %q", ExpandHome(plain))
	}

	relative := "relative/path"
	if ExpandHome(relative) != relative {
		t.Errorf("expected unchanged relative path, got %q", ExpandHome(relative))
	}
}

func TestLoadSaveState(t *testing.T) {
	tmpDir := t.TempDir()

	// Override HOME so LoadState/SaveState use our temp dir.
	orig := os.Getenv("HOME")
	t.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", orig) }()

	entry := InstalledEntry{
		Name:        "helix",
		Version:     "0.1.0",
		Type:        PackageTypeWorkflow,
		Source:      "https://github.com/easel/helix",
		InstalledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Files:       []string{"~/.agents/skills/helix.md"},
	}

	state := &InstalledState{
		Installed: []InstalledEntry{entry},
	}

	if err := SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	loaded, err := LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(loaded.Installed) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(loaded.Installed))
	}

	got := loaded.Installed[0]
	if got.Name != entry.Name {
		t.Errorf("expected name=%q, got %q", entry.Name, got.Name)
	}
	if got.Version != entry.Version {
		t.Errorf("expected version=%q, got %q", entry.Version, got.Version)
	}
	if got.Type != entry.Type {
		t.Errorf("expected type=%q, got %q", entry.Type, got.Type)
	}
	if got.Source != entry.Source {
		t.Errorf("expected source=%q, got %q", entry.Source, got.Source)
	}
	if len(got.Files) != 1 || got.Files[0] != entry.Files[0] {
		t.Errorf("expected files=%v, got %v", entry.Files, got.Files)
	}

	// Verify round-trip of time (truncated to second precision by YAML)
	if got.InstalledAt.IsZero() {
		t.Error("expected non-zero InstalledAt")
	}
}

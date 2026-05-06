package docprose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ddxconfig "github.com/DocumentDrivenDX/ddx/internal/config"
)

func TestProseCheckerFindingSchema(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	findings := checker.Findings("input.md", "This is robust.\n")
	if len(findings) == 0 {
		t.Fatal("expected at least one finding")
	}
	f := findings[0]
	if f.File != "input.md" || f.Line != 1 || f.RuleID == "" || f.Severity == "" || f.Rationale == "" || f.SuggestedEdit == "" {
		t.Fatalf("finding missing required field(s): %+v", f)
	}
}

func TestProseCheckerVocabularyOverrides(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{
		Accept: []string{"Quartz"},
		Reject: []string{"system", "solution"},
	})
	if err != nil {
		t.Fatal(err)
	}

	findings := checker.Findings("input.md", "Quartz keeps the system solution honest.\n")
	if len(findings) != 2 {
		t.Fatalf("expected 2 reject findings, got %+v", findings)
	}
	for _, finding := range findings {
		if finding.RuleID != "prose.vocabulary.reject" {
			t.Fatalf("unexpected rule id: %+v", finding)
		}
	}
}

func TestProseCheckerVocabularyPreservation(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{
		Accept: []string{"Quartz"},
		Reject: []string{"system"},
	})
	if err != nil {
		t.Fatal(err)
	}

	findings := checker.Findings("input.md", "Quartz keeps the system honest.\n")
	if len(findings) != 1 {
		t.Fatalf("expected one reject finding, got %+v", findings)
	}
	if findings[0].RuleID != "prose.vocabulary.reject" {
		t.Fatalf("unexpected rule id: %+v", findings[0])
	}
	if strings.Contains(findings[0].Rationale, "Quartz") {
		t.Fatalf("accepted term should not be flagged, got %+v", findings[0])
	}
}

func TestDefaultAssetLayout(t *testing.T) {
	root, err := defaultAssetRoot()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "check.yaml"),
		filepath.Join(root, "rules", "technical.yaml"),
		filepath.Join(root, "rules", "planning.yaml"),
		filepath.Join(root, "rules", "public.yaml"),
		filepath.Join(root, "vocabulary", "default.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected asset %s: %v", path, err)
		}
	}

	cfg, err := loadDefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != string(ModeTechnical) || cfg.Policy != "advisory" || cfg.Runner != "embedded" {
		t.Fatalf("unexpected default config: %+v", cfg)
	}

	vocab, err := loadDefaultVocabulary()
	if err != nil {
		t.Fatal(err)
	}
	if !containsAll(vocab.Accept, []string{"DDx", "bead", "execution"}) {
		t.Fatalf("default accept vocabulary missing DDx terms: %+v", vocab.Accept)
	}
	if len(vocab.Reject) != 0 {
		t.Fatalf("default reject vocabulary should be empty; got %+v", vocab.Reject)
	}
}

func TestResolveSettingsLayersConfig(t *testing.T) {
	settings, err := ResolveSettings(&ddxconfig.Config{
		Prose: &ddxconfig.ProseConfig{
			Mode:     "planning",
			Severity: "warning",
			Policy:   "blocking",
			Runner:   "vale",
			Vocabulary: &ddxconfig.ProseVocabularyConfig{
				Accept: []string{"Quartz"},
				Reject: []string{"system"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != ModePlanning || settings.Severity != "warning" || settings.Policy != "blocking" || settings.Runner != "vale" {
		t.Fatalf("unexpected layered settings: %+v", settings)
	}
	if !containsAll(settings.Vocabulary.Accept, []string{"Quartz"}) || !containsAll(settings.Vocabulary.Reject, []string{"system"}) {
		t.Fatalf("expected layered vocabulary overrides: %+v", settings.Vocabulary)
	}
}

func TestProseCheckerChangedMode(t *testing.T) {
	cases := listFixtureCases(t, "testdata/fixtures")
	for _, tc := range cases {
		t.Run(tc.Mode+"/"+tc.Name, func(t *testing.T) {
			checker, err := NewChecker(Mode(tc.Mode), Vocabulary{})
			if err != nil {
				t.Fatal(err)
			}
			got := checker.Findings("input.md", tc.Input)
			if !sameFindings(got, tc.Findings) {
				t.Fatalf("fixture mismatch for %s/%s\n--- got ---\n%v\n--- want ---\n%v", tc.Mode, tc.Name, got, tc.Findings)
			}
		})
	}
}

func TestProseCheckerPathMode(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"## Technical Context",
		"",
		"This is robust and comprehensive.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %+v", findings)
	}
	if findings[0].File != "docs/helix/example.md" {
		t.Fatalf("unexpected file in finding: %+v", findings[0])
	}
}

func TestProseCheckerDoesNotFlagUsefulTechnicalContext(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"The effect is robust and reproduced across model families.",
		"| Bead CRUD, claims, evidence | Comprehensive unit + integration | `cli/internal/bead/*_test.go` |",
		"`docs/helix/02-design/solution-designs/SD-007-release-readiness.md` documents release gating.",
		"`--once` processes at most one ready bead, then exits.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for contextual technical prose, got %+v", findings)
	}
}

func TestProseCheckerFlagsUnsupportedBenefitClaims(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"The framework exposes powerful commands for better alignment.",
		"Teams can use sophisticated multi-agent systems to solve complex problems.",
		"The pattern enables cutting edge automation in productive ways.",
		"Benchmarks showed significantly better output on the prompt suite.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 3 {
		t.Fatalf("expected three unsupported benefit findings, got %+v", findings)
	}
	for _, finding := range findings {
		if finding.RuleID != "prose.generic.claims" {
			t.Fatalf("unexpected rule id: %+v", finding)
		}
	}
}

func sameFindings(got, want []Finding) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func containsAll(got []string, want []string) bool {
	m := make(map[string]bool, len(got))
	for _, s := range got {
		m[s] = true
	}
	for _, s := range want {
		if !m[s] {
			return false
		}
	}
	return true
}

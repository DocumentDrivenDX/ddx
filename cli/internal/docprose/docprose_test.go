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
		"This is world-class and best-in-class.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %+v", findings)
	}
	if findings[0].File != "docs/helix/example.md" {
		t.Fatalf("unexpected file in finding: %+v", findings[0])
	}
}

func TestDocProseValeRules_StructureIgnores(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"---",
		"title: DDx prose quality plan",
		"description: world-class release notes should stay in front matter.",
		"---",
		"",
		"# Technical Context",
		"",
		"Use `world-class` as a code span, not prose.",
		"```markdown",
		"This is robust and seamless inside a fenced block.",
		"```",
		"",
		"Parent bead: ddx-ccda7a32.",
		"Governing spec: PLAN-2026-05-06-prose-quality-integration.",
		"`docs/helix/02-design/solution-designs/SD-007-release-readiness.md` documents release gating.",
		"`--once` processes at most one ready bead, then exits.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 0 {
		t.Fatalf("expected no findings for contextual technical prose, got %+v", findings)
	}
}

func TestDocProseValeRules_UnsupportedClaim(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"This implementation delivers world-class reliability and industry-leading performance characteristics.",
		"The approach represents a best-in-class solution for document-driven development workflows.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 2 {
		t.Fatalf("expected two unsupported-claim findings, got %+v", findings)
	}
	for _, finding := range findings {
		if finding.RuleID != "prose.claim.unsupported" {
			t.Fatalf("unexpected rule id: %+v", finding)
		}
	}
}

func TestDocProseValeRules_AISlop(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"Sophisticated autonomous workflows handle complex problems at scale.",
		"The toolkit provides true power for development teams through sophisticated multi-agent coordination.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 2 {
		t.Fatalf("expected two ai-slop findings, got %+v", findings)
	}
	for _, finding := range findings {
		if finding.RuleID != "prose.ai_slop.polish" {
			t.Fatalf("unexpected rule id: %+v", finding)
		}
	}
}

func TestDocProseValeRules_TokenCost(t *testing.T) {
	checker, err := NewChecker(ModeTechnical, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"It is very important to run the full test suite before merging any changes.",
		"This validation step is very important because it catches integration failures early.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 2 {
		t.Fatalf("expected two token-cost findings, got %+v", findings)
	}
	for _, finding := range findings {
		if finding.RuleID != "prose.cost.filler" {
			t.Fatalf("unexpected rule id: %+v", finding)
		}
	}
}

func TestDocProseValeRules_NegationPredicate(t *testing.T) {
	checker, err := NewChecker(ModePlanning, Vocabulary{})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		"HELIX is a control loop, not a pipeline.",
		"HELIX is a methodology with seven activities; it is not a pipeline.",
		"## What HELIX Is Not",
		"",
		"HELIX is not a CLI.",
	}, "\n")
	findings := checker.Findings("docs/helix/example.md", input)
	if len(findings) != 1 {
		t.Fatalf("expected one negation-predicate finding, got %+v", findings)
	}
	if findings[0].RuleID != "prose.definition_by_negation" {
		t.Fatalf("unexpected rule id: %+v", findings[0])
	}
	if findings[0].Line != 1 {
		t.Fatalf("unexpected finding line: %+v", findings[0])
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

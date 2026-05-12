package docprose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type corpusExpect struct {
	Mode          string   `json:"mode"`
	ExpectRuleIDs []string `json:"expect_rule_ids"`
}

type corpusCase struct {
	Group  string
	Name   string
	Input  string
	Expect corpusExpect
}

const corpusRoot = "testdata/corpus"

var requiredCorpusGroups = []string{
	"positive/ai-slop",
	"positive/unsupported-claim",
	"positive/token-cost",
	"positive/missing-actor-action",
	"negative/technical-density",
	"negative/markdown-structure",
	"negative/evidence-backed-claim",
	"real/ddx-docs",
}

func loadCorpusCases(t *testing.T) []corpusCase {
	t.Helper()
	var cases []corpusCase
	for _, group := range requiredCorpusGroups {
		groupDir := filepath.Join(corpusRoot, group)
		entries, err := os.ReadDir(groupDir)
		if err != nil {
			t.Fatalf("read corpus group %s: %v", group, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			caseDir := filepath.Join(groupDir, entry.Name())
			inputPath := filepath.Join(caseDir, "input.md")
			expectPath := filepath.Join(caseDir, "expect.json")

			input, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("read %s: %v", inputPath, err)
			}
			expectData, err := os.ReadFile(expectPath)
			if err != nil {
				t.Fatalf("read %s: %v", expectPath, err)
			}
			var expect corpusExpect
			if err := json.Unmarshal(expectData, &expect); err != nil {
				t.Fatalf("unmarshal %s: %v", expectPath, err)
			}
			cases = append(cases, corpusCase{
				Group:  group,
				Name:   entry.Name(),
				Input:  string(input),
				Expect: expect,
			})
		}
	}
	return cases
}

func TestDocProseCorpus_LoadsGroups(t *testing.T) {
	cases := loadCorpusCases(t)

	groupCounts := map[string]int{}
	for _, c := range cases {
		groupCounts[c.Group]++
	}

	for _, group := range requiredCorpusGroups {
		if groupCounts[group] == 0 {
			t.Errorf("corpus group %q has no cases", group)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	t.Logf("loaded %d corpus cases across %d groups", len(cases), len(groupCounts))
}

func TestDocProseCorpus_ExpectedFindingSchema(t *testing.T) {
	cases := loadCorpusCases(t)

	validModes := map[string]bool{
		"technical": true,
		"planning":  true,
		"public":    true,
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.Group+"/"+tc.Name, func(t *testing.T) {
			if !validModes[tc.Expect.Mode] {
				t.Errorf("expect.json mode %q is not valid (want technical/planning/public)", tc.Expect.Mode)
			}
			if tc.Expect.ExpectRuleIDs == nil {
				t.Errorf("expect.json missing expect_rule_ids field")
			}
			for _, rid := range tc.Expect.ExpectRuleIDs {
				if rid == "" {
					t.Errorf("expect.json contains empty rule_id")
				}
				if !strings.HasPrefix(rid, "prose.") {
					t.Errorf("expect.json rule_id %q must start with prose.", rid)
				}
			}
		})
	}
}

func TestDocProseCorpus_NegativeStructureCasesStayQuiet(t *testing.T) {
	const group = "negative/markdown-structure"
	groupDir := filepath.Join(corpusRoot, group)
	entries, err := os.ReadDir(groupDir)
	if err != nil {
		t.Fatalf("read %s: %v", groupDir, err)
	}

	seenCategories := map[string]bool{}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		caseDir := filepath.Join(groupDir, entry.Name())
		inputPath := filepath.Join(caseDir, "input.md")
		expectPath := filepath.Join(caseDir, "expect.json")

		input, err := os.ReadFile(inputPath)
		if err != nil {
			t.Fatalf("read %s: %v", inputPath, err)
		}
		expectData, err := os.ReadFile(expectPath)
		if err != nil {
			t.Fatalf("read %s: %v", expectPath, err)
		}
		var expect corpusExpect
		if err := json.Unmarshal(expectData, &expect); err != nil {
			t.Fatalf("unmarshal %s: %v", expectPath, err)
		}

		seenCategories[entry.Name()] = true
		name := entry.Name()

		t.Run(name, func(t *testing.T) {
			checker, err := NewChecker(Mode(expect.Mode), Vocabulary{})
			if err != nil {
				t.Fatal(err)
			}
			findings := checker.Findings("input.md", string(input))
			if len(findings) != 0 {
				data, _ := json.MarshalIndent(findings, "", "  ")
				t.Errorf("negative structure case %q produced %d finding(s), expected 0:\n%s",
					name, len(findings), data)
			}
		})
	}

	required := []string{"path", "command", "table", "code-span", "frontmatter", "fenced-block", "id"}
	for _, cat := range required {
		found := false
		for name := range seenCategories {
			if strings.Contains(name, cat) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("negative/markdown-structure corpus missing a case covering category %q", cat)
		}
	}
}

func TestDocProseCorpus_PositiveCasesCanAssertRuleIDs(t *testing.T) {
	positiveGroups := []string{
		"positive/ai-slop",
		"positive/unsupported-claim",
		"positive/token-cost",
		"positive/missing-actor-action",
	}

	for _, group := range positiveGroups {
		groupDir := filepath.Join(corpusRoot, group)
		entries, err := os.ReadDir(groupDir)
		if err != nil {
			t.Fatalf("read %s: %v", groupDir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			caseDir := filepath.Join(groupDir, entry.Name())
			inputPath := filepath.Join(caseDir, "input.md")
			expectPath := filepath.Join(caseDir, "expect.json")

			input, err := os.ReadFile(inputPath)
			if err != nil {
				t.Fatalf("read %s: %v", inputPath, err)
			}
			expectData, err := os.ReadFile(expectPath)
			if err != nil {
				t.Fatalf("read %s: %v", expectPath, err)
			}
			var expect corpusExpect
			if err := json.Unmarshal(expectData, &expect); err != nil {
				t.Fatalf("unmarshal %s: %v", expectPath, err)
			}

			group, entryName := group, entry.Name()
			t.Run(group+"/"+entryName, func(t *testing.T) {
				if len(expect.ExpectRuleIDs) == 0 {
					t.Skipf("positive case %s/%s has no expected rule_ids (no assertion)", group, entryName)
				}

				checker, err := NewChecker(Mode(expect.Mode), Vocabulary{})
				if err != nil {
					t.Fatal(err)
				}
				findings := checker.Findings("input.md", string(input))

				actualRuleIDs := map[string]bool{}
				for _, f := range findings {
					actualRuleIDs[f.RuleID] = true
				}

				for _, expectedRuleID := range expect.ExpectRuleIDs {
					if !actualRuleIDs[expectedRuleID] {
						data, _ := json.MarshalIndent(findings, "", "  ")
						t.Errorf("expected rule_id %q not found in findings for %s/%s:\n%s",
							expectedRuleID, group, entryName, data)
					}
				}
			})
		}
	}
}

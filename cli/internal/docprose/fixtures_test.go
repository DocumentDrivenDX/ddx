package docprose

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type fixtureCase struct {
	Name     string
	Mode     string
	Input    string
	Golden   string
	Findings []Finding
}

func loadFixtureCase(t *testing.T, root, mode, name string) fixtureCase {
	t.Helper()

	dir := filepath.Join(root, mode, name)
	inputPath := filepath.Join(dir, "input.md")
	goldenPath := filepath.Join(dir, "findings.golden.json")

	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("read %s: %v", inputPath, err)
	}
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read %s: %v", goldenPath, err)
	}
	var findings []Finding
	if err := json.Unmarshal(golden, &findings); err != nil {
		t.Fatalf("unmarshal %s: %v", goldenPath, err)
	}
	return fixtureCase{
		Name:     name,
		Mode:     mode,
		Input:    string(input),
		Golden:   strings.TrimSpace(string(golden)),
		Findings: findings,
	}
}

func listFixtureCases(t *testing.T, root string) []fixtureCase {
	t.Helper()

	modes, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read fixture root %s: %v", root, err)
	}

	var cases []fixtureCase
	for _, modeEntry := range modes {
		if !modeEntry.IsDir() {
			continue
		}
		mode := modeEntry.Name()
		modeDir := filepath.Join(root, mode)
		names, err := os.ReadDir(modeDir)
		if err != nil {
			t.Fatalf("read mode dir %s: %v", modeDir, err)
		}
		for _, nameEntry := range names {
			if !nameEntry.IsDir() {
				continue
			}
			cases = append(cases, loadFixtureCase(t, root, mode, nameEntry.Name()))
		}
	}

	sort.Slice(cases, func(i, j int) bool {
		if cases[i].Mode == cases[j].Mode {
			return cases[i].Name < cases[j].Name
		}
		return cases[i].Mode < cases[j].Mode
	})
	return cases
}

func TestFixtures(t *testing.T) {
	const root = "testdata/fixtures"
	cases := listFixtureCases(t, root)
	if len(cases) == 0 {
		t.Fatal("expected at least one docprose fixture case")
	}

	seenModes := map[string]bool{}
	for _, tc := range cases {
		t.Run(tc.Mode+"/"+tc.Name, func(t *testing.T) {
			seenModes[tc.Mode] = true
			if strings.TrimSpace(tc.Input) == "" {
				t.Fatalf("fixture input is empty")
			}
			if !strings.Contains(tc.Input, "\n") {
				t.Fatalf("fixture input must be markdown with multiple lines")
			}
			if !strings.HasSuffix(tc.Golden, "]") {
				t.Fatalf("golden must be a JSON array: %s", tc.Golden)
			}

			checker, err := NewChecker(Mode(tc.Mode), Vocabulary{})
			if err != nil {
				t.Fatal(err)
			}
			got := checker.Findings("input.md", tc.Input)
			if !sameFindings(got, tc.Findings) {
				gotJSON, err := json.MarshalIndent(got, "", "  ")
				if err != nil {
					t.Fatalf("marshal got findings: %v", err)
				}
				t.Fatalf("fixture golden mismatch for %s/%s\n--- got ---\n%s\n--- want ---\n%s", tc.Mode, tc.Name, gotJSON, tc.Golden)
			}

			for i, finding := range got {
				if finding.File == "" || finding.Line == 0 || finding.RuleID == "" || finding.Severity == "" || finding.Rationale == "" || finding.SuggestedEdit == "" {
					t.Fatalf("finding %d missing required field: %+v", i, finding)
				}
			}
		})
	}

	for _, mode := range []string{"technical", "planning", "public"} {
		if !seenModes[mode] {
			t.Fatalf("expected at least one fixture in %q mode", mode)
		}
	}
}

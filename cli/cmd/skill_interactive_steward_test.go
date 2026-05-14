package cmd

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// interactiveStewardRepoRoot returns the repo root derived from this file's
// location (cli/cmd/ → up two levels).
func interactiveStewardRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// file = <repo>/cli/cmd/skill_interactive_steward_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

type routeFixture struct {
	Phrase                 string   `json:"phrase"`
	Mode                   string   `json:"mode"`
	References             []string `json:"references"`
	QueueCommands          []string `json:"queue_commands"`
	TrackerMutationAllowed bool     `json:"tracker_mutation_allowed"`
	CodeEditsAllowed       bool     `json:"code_edits_allowed"`
	ExpectedNextAction     string   `json:"expected_next_action"`
}

func loadRouteFixtures(t *testing.T, root string) []routeFixture {
	t.Helper()
	path := filepath.Join(root, "library", "skills", "ddx", "evals", "routing.jsonl")
	f, err := os.Open(path)
	require.NoError(t, err, "open routing.jsonl")
	defer f.Close()

	var fixtures []routeFixture
	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		lineNum++
		if line == "" {
			continue
		}
		var fix routeFixture
		require.NoError(t, json.Unmarshal([]byte(line), &fix), "invalid JSON at line %d: %s", lineNum, line)
		fixtures = append(fixtures, fix)
	}
	require.NoError(t, scanner.Err())
	return fixtures
}

// TestDDxInteractiveStewardRouteFixtures validates the route fixture schema,
// required phrases, negative cases, phase output contract names, and expected
// queue/code/tracker permissions.
func TestDDxInteractiveStewardRouteFixtures(t *testing.T) {
	root := interactiveStewardRepoRoot(t)
	fixtures := loadRouteFixtures(t, root)

	assert.GreaterOrEqual(t, len(fixtures), 15, "routing.jsonl must have at least 15 rows")

	// Build phrase index for targeted assertions.
	phraseIndex := make(map[string]routeFixture, len(fixtures))
	for _, fix := range fixtures {
		phraseIndex[fix.Phrase] = fix
	}

	// --- Required schema fields on every row ---
	for _, fix := range fixtures {
		assert.NotEmpty(t, fix.Phrase, "fixture has empty phrase")
		assert.NotEmpty(t, fix.Mode, "phrase %q: empty mode", fix.Phrase)
		assert.NotNil(t, fix.References, "phrase %q: nil references", fix.Phrase)
		assert.NotNil(t, fix.QueueCommands, "phrase %q: nil queue_commands", fix.Phrase)
		assert.NotEmpty(t, fix.ExpectedNextAction, "phrase %q: empty expected_next_action", fix.Phrase)
	}

	// --- Required phrases (AC 5) ---
	requiredPhrases := []string{
		"what should I work on next",
		"what's blocking the queue now",
		"review with fresh eyes",
		"fold in this guidance and review again",
		"break this down into specs and beads",
		"make this testable",
		"implement the top ready bead",
		"ddx work --once",
	}
	for _, phrase := range requiredPhrases {
		_, ok := phraseIndex[phrase]
		assert.True(t, ok, "missing required phrase: %q", phrase)
	}

	// --- Negative cases: broad interactive-steward prompts must not allow code edits ---
	interactivePhrases := []string{
		"what should I work on next",
		"what's blocking the queue now",
		"review with fresh eyes",
		"fold in this guidance and review again",
	}
	for _, phrase := range interactivePhrases {
		fix, ok := phraseIndex[phrase]
		if !ok {
			continue
		}
		assert.Equal(t, "interactive-steward", fix.Mode,
			"phrase %q should route to interactive-steward mode", phrase)
		assert.False(t, fix.CodeEditsAllowed,
			"phrase %q should not allow code edits in interactive-steward mode", phrase)
	}

	// --- Positive case: explicit worker commands route to bead_execution ---
	if fix, ok := phraseIndex["ddx work --once"]; ok {
		assert.Equal(t, "bead_execution", fix.Mode,
			"'ddx work --once' should be bead_execution mode")
	}
	if fix, ok := phraseIndex["drain the queue"]; ok {
		assert.Equal(t, "bead_execution", fix.Mode,
			"'drain the queue' should be bead_execution mode")
	}

	// --- "implement the top ready bead" is direct implementation only ---
	if fix, ok := phraseIndex["implement the top ready bead"]; ok {
		assert.Equal(t, "direct_user_implementation", fix.Mode,
			"explicit implementation verb must be direct_user_implementation")
		assert.True(t, fix.CodeEditsAllowed,
			"'implement the top ready bead' must allow code edits")
	}

	// --- Phase output contract names must appear in expected_next_action values ---
	actionValues := make(map[string]bool, len(fixtures))
	for _, fix := range fixtures {
		actionValues[fix.ExpectedNextAction] = true
	}
	for _, phase := range []string{"orient", "plan"} {
		assert.True(t, actionValues[phase],
			"phase output contract %q not represented in any fixture's expected_next_action", phase)
	}

	// --- interactive-steward fixtures must reference interactive.md ---
	for _, fix := range fixtures {
		if fix.Mode != "interactive-steward" {
			continue
		}
		hasInteractive := false
		for _, ref := range fix.References {
			if ref == "reference/interactive.md" {
				hasInteractive = true
				break
			}
		}
		assert.True(t, hasInteractive,
			"phrase %q (interactive-steward) must reference reference/interactive.md", fix.Phrase)
	}
}

// TestDDxSkillCopiedTreesMatchSource compares the touched ddx skill files
// across all shipped skill paths so a stale project copy cannot pass.
func TestDDxSkillCopiedTreesMatchSource(t *testing.T) {
	root := interactiveStewardRepoRoot(t)
	srcDir := filepath.Join(root, "library", "skills", "ddx")

	copyDirs := []string{
		filepath.Join(root, "cli", "internal", "skills", "ddx"),
		filepath.Join(root, "cli", "internal", "registry", "defaultplugin", "library", "skills", "ddx"),
		filepath.Join(root, ".agents", "skills", "ddx"),
		filepath.Join(root, ".claude", "skills", "ddx"),
		filepath.Join(root, ".ddx", "skills", "ddx"),
	}

	touchedFiles := []string{
		"SKILL.md",
		"reference/interactive.md",
		"reference/status.md",
		"reference/work.md",
		"reference/review.md",
		"evals/routing.jsonl",
	}

	for _, rel := range touchedFiles {
		srcPath := filepath.Join(srcDir, rel)
		srcContent, err := os.ReadFile(srcPath)
		require.NoError(t, err, "source file missing: %s", rel)

		for _, copyDir := range copyDirs {
			copyPath := filepath.Join(copyDir, rel)
			copyContent, err := os.ReadFile(copyPath)
			require.NoError(t, err, "copy missing at %s", copyPath)
			assert.Equal(t, string(srcContent), string(copyContent),
				"stale copy: %s does not match source %s", copyPath, srcPath)
		}
	}
}

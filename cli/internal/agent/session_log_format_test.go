package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentlib "github.com/easel/fizeau"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readProgressCorpusFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", "progress_corpus", name)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}

func readProgressCorpusEntries(t *testing.T, name string) []agentlib.ServiceProgressData {
	t.Helper()
	path := filepath.Join("testdata", "progress_corpus", name)
	var entries []agentlib.ServiceProgressData
	err := ForEachJSONL[agentlib.ServiceProgressData](path, func(entry agentlib.ServiceProgressData) error {
		entries = append(entries, entry)
		return nil
	})
	require.NoError(t, err)
	return entries
}

// TestFormatSessionLogLines_Corpus covers the sanitized corpus used to guard
// canonical and legacy progress rendering. The corpus includes Claude,
// Codex/Fizeau, native, and secondary-harness samples while preserving long
// paths, turn counters, output summaries, and tok/sec rendering.
func TestFormatSessionLogLines_Corpus(t *testing.T) {
	cases := []struct {
		name            string
		file            string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "claude stream",
			file: "claude_stream.jsonl",
			wantContains: []string{
				"ddx-claude 21",
				"session_log_format.go",
				"out=1.8KB 73 lines",
				"func FormatServiceProgress",
			},
		},
		{
			name: "codex fizeau",
			file: "codex_fizeau.jsonl",
			wantContains: []string{
				"ddx-codex 22",
				"go test ./internal/agent/...",
				"out=2.4KB 88 lines",
				"tok/s",
			},
		},
		{
			name: "native agent",
			file: "native_agent.jsonl",
			wantContains: []string{
				"ddx-native 23",
				"out=512B 14 lines",
				"session_log_format.go",
			},
			wantNotContains: []string{"tok/s"},
		},
		{
			name: "secondary harness",
			file: "secondary_harness.jsonl",
			wantContains: []string{
				"ddx-secondary 24",
				"secondary harness response complete",
				"tok/s",
			},
		},
		{
			name: "legacy summary",
			file: "legacy_summary.jsonl",
			wantContains: []string{
				"ddx-legacy 25",
				"legacy output summary preserved from stream logs",
				"tok/s",
			},
			wantNotContains: []string{"out="},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			raw := readProgressCorpusFile(t, tc.file)
			got := FormatSessionLogLines(raw)
			require.NotEmpty(t, got)
			require.Equal(t, FormatServiceProgressEntries(readProgressCorpusEntries(t, tc.file)), got)

			for _, want := range tc.wantContains {
				assert.Contains(t, got, want)
			}
			for _, want := range tc.wantNotContains {
				assert.NotContains(t, got, want)
			}

			if tc.file == "codex_fizeau.jsonl" {
				line := renderedLineContaining(t, got, "go test ./internal/agent/...")
				assert.GreaterOrEqual(t, len(line), 72)
				assert.LessOrEqual(t, len(line), 120)
			}

			if tc.file == "secondary_harness.jsonl" {
				line := renderedLineContaining(t, got, "secondary harness response complete")
				assert.GreaterOrEqual(t, len(line), 72)
				assert.LessOrEqual(t, len(line), 80)
			}
		})
	}
}

// TestProgressCorpus_TurnIndexCountsUp verifies the corpus preserves the
// visible turn counter order across the canonical Claude, Codex/Fizeau, and
// native samples.
func TestProgressCorpus_TurnIndexCountsUp(t *testing.T) {
	raw := strings.Join([]string{
		readProgressCorpusFile(t, "claude_stream.jsonl"),
		readProgressCorpusFile(t, "codex_fizeau.jsonl"),
		readProgressCorpusFile(t, "native_agent.jsonl"),
	}, "\n")
	got := FormatSessionLogLines(raw)

	idx21 := strings.Index(got, "ddx-claude 21")
	idx22 := strings.Index(got, "ddx-codex 22")
	idx23 := strings.Index(got, "ddx-native 23")
	require.NotEqual(t, -1, idx21)
	require.NotEqual(t, -1, idx22)
	require.NotEqual(t, -1, idx23)
	assert.Less(t, idx21, idx22)
	assert.Less(t, idx22, idx23)
}

func TestFormatServiceProgressEntries_UsesToolCallIndexAndSummary(t *testing.T) {
	got := FormatServiceProgressEntries([]agentlib.ServiceProgressData{
		{
			Phase:         "tool",
			State:         "complete",
			TaskID:        "ddx-live",
			TurnIndex:     1,
			ToolName:      "go",
			Command:       "go test ./internal/agent/...",
			Action:        "run go test ./internal/agent/...",
			ToolCallID:    "call-2",
			ToolCallIndex: 2,
			OutputSummary: "command finished successfully",
		},
	})

	require.Equal(t, "  ok ddx-live 2 run go test ./internal/agent/... < command finished successfully\n", got)
	assert.NotContains(t, got, "\n  >")
}

func TestFormatServiceProgressEntries_WithScopedWorkPhaseSuppressesCurrentBead(t *testing.T) {
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now:           fixedProgressFormatTime,
		CurrentBeadID: "ddx-live",
		WorkPhase:     "do",
	})

	got := renderer.FormatServiceProgressEntries([]agentlib.ServiceProgressData{
		{
			Phase:       "tool",
			State:       "complete",
			TaskID:      "ddx-live",
			TurnIndex:   2,
			Action:      "run tests",
			Target:      "cli/internal/bead",
			OutputBytes: 42,
			OutputLines: 3,
		},
	})

	require.Equal(t, "12:34:56 do ok 2 run tests to cli/internal/bead < out=42B 3 lines\n", got)
	assert.NotContains(t, strings.TrimSpace(got), "ddx-live")
}

func TestFormatServiceProgressEntries_PreservesDifferentReferencedBead(t *testing.T) {
	renderer := NewWorkLogRenderer(WorkLogRendererOptions{
		Now:           fixedProgressFormatTime,
		CurrentBeadID: "ddx-current",
		WorkPhase:     "do",
	})

	got := renderer.FormatServiceProgressEntries([]agentlib.ServiceProgressData{
		{
			Phase:       "tool",
			State:       "complete",
			TaskID:      "ddx-other",
			TurnIndex:   3,
			Action:      "inspect related bead",
			Target:      "cli/internal/agent",
			OutputBytes: 128,
			OutputLines: 4,
		},
	})

	require.Equal(t, "12:34:56 do ok ddx-other 3 inspect related bead to cli/internal/agent < out=128B 4 lines\n", got)
	assert.Contains(t, got, "ddx-other")
	assert.NotContains(t, got, "ddx-current")
}

func fixedProgressFormatTime() time.Time {
	return time.Date(2026, 5, 9, 12, 34, 56, 789000000, time.UTC)
}

func renderedLineContaining(t *testing.T, rendered, want string) string {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(rendered), "\n") {
		if strings.Contains(line, want) {
			return line
		}
	}
	t.Fatalf("rendered output did not contain %q:\n%s", want, rendered)
	return ""
}

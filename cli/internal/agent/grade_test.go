package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// G-01: Grading prompt includes original task, arm outputs, and diffs.
func TestGradeConstructsPrompt(t *testing.T) {
	mock := &mockExecutor{output: `{"arms":[{"arm":"agent","score":8,"max_score":10,"pass":true,"rationale":"Good"}]}`}
	r := newTestRunner(mock)

	record := &ComparisonRecord{
		ID:        "cmp-test",
		Timestamp: time.Now(),
		Prompt:    "implement feature X",
		Arms: []ComparisonArm{
			{Harness: "agent", Output: "agent output", Diff: "diff --git a/file.go"},
			{Harness: "claude", Output: "claude output", Diff: "diff --git b/file.go"},
		},
	}

	_, err := r.Grade(record, GradeOptions{Grader: "codex"})
	require.NoError(t, err)

	// The grading prompt sent to the harness should contain the task and arm data
	sentPrompt := mock.lastArgs[len(mock.lastArgs)-1] // codex is arg mode
	assert.Contains(t, sentPrompt, "implement feature X")
	assert.Contains(t, sentPrompt, "agent output")
	assert.Contains(t, sentPrompt, "claude output")
	assert.Contains(t, sentPrompt, "diff --git a/file.go")
}

// G-02: Virtual harness returns JSON grade → parsed into per-arm scores.
func TestGradeParsesResponse(t *testing.T) {
	gradeJSON := `{"arms":[{"arm":"agent","score":9,"max_score":10,"pass":true,"rationale":"Excellent"},{"arm":"claude","score":7,"max_score":10,"pass":true,"rationale":"Good but verbose"}]}`
	mock := &mockExecutor{output: gradeJSON}
	r := newTestRunner(mock)

	record := &ComparisonRecord{
		ID:     "cmp-test",
		Prompt: "test task",
		Arms: []ComparisonArm{
			{Harness: "agent", Output: "ok"},
			{Harness: "claude", Output: "ok"},
		},
	}

	grades, err := r.Grade(record, GradeOptions{Grader: "codex"})
	require.NoError(t, err)
	require.Len(t, grades, 2)
	assert.Equal(t, "agent", grades[0].Arm)
	assert.Equal(t, 9, grades[0].Score)
	assert.True(t, grades[0].Pass)
	assert.Equal(t, "claude", grades[1].Arm)
	assert.Equal(t, 7, grades[1].Score)
}

// G-03: Grades are attached to the comparison record.
func TestGradeAttachesToRecord(t *testing.T) {
	gradeJSON := `{"arms":[{"arm":"agent","score":8,"max_score":10,"pass":true,"rationale":"Good"}]}`
	mock := &mockExecutor{output: gradeJSON}
	r := newTestRunner(mock)

	record := &ComparisonRecord{
		ID:     "cmp-test",
		Prompt: "test",
		Arms:   []ComparisonArm{{Harness: "agent", Output: "ok"}},
	}

	grades, err := r.Grade(record, GradeOptions{Grader: "codex"})
	require.NoError(t, err)

	record.Grades = grades
	assert.Len(t, record.Grades, 1)
	assert.Equal(t, 8, record.Grades[0].Score)
}

// G-04: Custom rubric replaces the default grading template.
func TestGradeCustomRubric(t *testing.T) {
	gradeJSON := `{"arms":[{"arm":"agent","score":5,"max_score":10,"pass":false,"rationale":"Failed custom criteria"}]}`
	mock := &mockExecutor{output: gradeJSON}
	r := newTestRunner(mock)

	record := &ComparisonRecord{
		ID:     "cmp-test",
		Prompt: "test",
		Arms:   []ComparisonArm{{Harness: "agent", Output: "ok"}},
	}

	customRubric := "Grade ONLY on security. Ignore functionality."
	_, err := r.Grade(record, GradeOptions{Grader: "codex", Rubric: customRubric})
	require.NoError(t, err)

	sentPrompt := mock.lastArgs[len(mock.lastArgs)-1]
	assert.Contains(t, sentPrompt, "Grade ONLY on security")
}

// G-05: Non-JSON grader output → graceful error.
func TestGradeMalformedResponse(t *testing.T) {
	mock := &mockExecutor{output: "This is not JSON at all"}
	r := newTestRunner(mock)

	record := &ComparisonRecord{
		ID:     "cmp-test",
		Prompt: "test",
		Arms:   []ComparisonArm{{Harness: "agent", Output: "ok"}},
	}

	_, err := r.Grade(record, GradeOptions{Grader: "codex"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

// G-06: Grading harness returns exit_code=1 → error.
func TestGradeGraderFailure(t *testing.T) {
	mock := &mockExecutor{output: "error", exitCode: 1}
	r := newTestRunner(mock)

	record := &ComparisonRecord{
		ID:     "cmp-test",
		Prompt: "test",
		Arms:   []ComparisonArm{{Harness: "agent", Output: "ok"}},
	}

	_, err := r.Grade(record, GradeOptions{Grader: "codex"})
	assert.Error(t, err)
}

// Verify grade JSON round-trips correctly.
func TestGradeJSONRoundTrip(t *testing.T) {
	grade := ComparisonGrade{
		Arm:       "agent",
		Score:     8,
		MaxScore:  10,
		Pass:      true,
		Rationale: "Correct implementation",
	}
	data, err := json.Marshal(grade)
	require.NoError(t, err)
	var decoded ComparisonGrade
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, grade, decoded)
}

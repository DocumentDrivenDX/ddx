package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureClaudeUsageOutput is a representative sample of tmux-captured /usage output
// (post-ANSI-strip) matching the real claude TUI format.
const fixtureClaudeUsageOutput = `
Sonnet 4.6 · Claude Max
Status   Config   Usage   Stats

Current session
██                                                 4% used
Resets 4pm (America/New_York)

Current week (all models)
                                                   0% used
Resets Apr 19, 11am (America/New_York)

Current week (Sonnet only)
▌                                                  1% used
Resets Apr 19, 11am (America/New_York)

Extra usage
██████████████████████████████████████████████████ 100% used
$200.10 / $200.00 spent · Resets May 1 (America/New_York)
`

func TestParseClaudeUsageOutput_AllSections(t *testing.T) {
	windows, acct := parseClaudeUsageOutput(fixtureClaudeUsageOutput)

	require.NotNil(t, acct)
	assert.Equal(t, "Claude Max", acct.PlanType)

	require.Len(t, windows, 4)

	sess := windows[0]
	assert.Equal(t, "Current session", sess.Name)
	assert.Equal(t, "session", sess.LimitID)
	assert.Equal(t, 300, sess.WindowMinutes)
	assert.Equal(t, 4.0, sess.UsedPercent)
	assert.Equal(t, "ok", sess.State)
	assert.Contains(t, sess.ResetsAt, "4pm")
	assert.Contains(t, sess.ResetsAt, "America/New_York")

	weeklyAll := windows[1]
	assert.Equal(t, "Current week (all models)", weeklyAll.Name)
	assert.Equal(t, "weekly-all", weeklyAll.LimitID)
	assert.Equal(t, 10080, weeklyAll.WindowMinutes)
	assert.Equal(t, 0.0, weeklyAll.UsedPercent)
	assert.Equal(t, "ok", weeklyAll.State)
	assert.Contains(t, weeklyAll.ResetsAt, "Apr 19")

	weeklySonnet := windows[2]
	assert.Equal(t, "Current week (Sonnet only)", weeklySonnet.Name)
	assert.Equal(t, "weekly-sonnet", weeklySonnet.LimitID)
	assert.Equal(t, 1.0, weeklySonnet.UsedPercent)
	assert.Equal(t, "ok", weeklySonnet.State)

	extra := windows[3]
	assert.Equal(t, "Extra usage", extra.Name)
	assert.Equal(t, "extra", extra.LimitID)
	assert.Equal(t, 0, extra.WindowMinutes)
	assert.Equal(t, 100.0, extra.UsedPercent)
	assert.Equal(t, "blocked", extra.State)
	assert.Contains(t, extra.ResetsAt, "May 1")
}

func TestParseClaudeUsageOutput_PlanTypeVariants(t *testing.T) {
	cases := []struct {
		header   string
		wantPlan string
	}{
		{"Haiku 3.5 · Claude Pro", "Claude Pro"},
		{"Opus 4.6 · Claude Team", "Claude Team"},
		{"Claude Enterprise plan", "Claude Enterprise"},
		{"Sonnet 4.6 · Claude Max", "Claude Max"},
	}
	for _, tc := range cases {
		t.Run(tc.wantPlan, func(t *testing.T) {
			_, acct := parseClaudeUsageOutput(tc.header + "\nCurrent session\n10% used\nResets 5pm (UTC)")
			require.NotNil(t, acct)
			assert.Equal(t, tc.wantPlan, acct.PlanType)
		})
	}
}

func TestParseClaudeUsageOutput_NoSections(t *testing.T) {
	windows, acct := parseClaudeUsageOutput("Welcome to Claude")
	assert.Empty(t, windows)
	assert.Nil(t, acct)
}

func TestParseClaudeUsageOutput_PartialOutput(t *testing.T) {
	// Only one section visible (e.g., capture caught mid-scroll)
	text := `Claude Max
Current session
████████                                           25% used
Resets 6am (America/Los_Angeles)
`
	windows, acct := parseClaudeUsageOutput(text)
	require.NotNil(t, acct)
	assert.Equal(t, "Claude Max", acct.PlanType)
	require.Len(t, windows, 1)
	assert.Equal(t, "Current session", windows[0].Name)
	assert.Equal(t, 25.0, windows[0].UsedPercent)
	assert.Equal(t, "ok", windows[0].State)
	assert.Contains(t, windows[0].ResetsAt, "6am")
}

func TestStripANSI(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"\x1b[1;32mGreen\x1b[0m", "Green"},
		{"\x1b[H\x1b[2JHello", "Hello"},
		{"No escapes", "No escapes"},
		{"\x1b[?2004htext\x1b[?2004l", "text"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, stripANSI(tc.input), "input: %q", tc.input)
	}
}

func TestExtractResetsText(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"Resets 4pm (America/New_York)", "4pm (America/New_York)"},
		{"Resets Apr 19, 11am (America/New_York)", "Apr 19, 11am (America/New_York)"},
		{"$200.10 / $200.00 spent · Resets May 1 (America/New_York)", "May 1 (America/New_York)"},
		{"No reset info available", ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, extractResetsText(tc.input), "input: %q", tc.input)
	}
}

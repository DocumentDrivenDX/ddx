package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDDxQueueStewardGuidance_InteractiveBlockingQuestionIsReadOnly asserts
// that broad blocking/queue-status prompts route to interactive-steward (or
// queue_steward) mode with code_edits_allowed=false. Workers executing beads
// are exempt from this restriction; interactive agents asking about queue
// health must not opportunistically edit code or execute a ready bead.
//
// AC2 from bead ddx-08c9d6a7.
func TestDDxQueueStewardGuidance_InteractiveBlockingQuestionIsReadOnly(t *testing.T) {
	root := interactiveStewardRepoRoot(t)
	fixtures := loadRouteFixtures(t, root)
	phraseIndex := make(map[string]routeFixture, len(fixtures))
	for _, fix := range fixtures {
		phraseIndex[fix.Phrase] = fix
	}

	readOnlyPhrases := []string{
		"what's blocking the queue now",
		"what should I work on next",
	}
	for _, phrase := range readOnlyPhrases {
		phrase := phrase
		t.Run(phrase, func(t *testing.T) {
			fix, ok := phraseIndex[phrase]
			require.True(t, ok, "phrase %q must be present in routing.jsonl", phrase)

			assert.False(t, fix.CodeEditsAllowed,
				"phrase %q must not allow code edits in interactive-steward mode", phrase)

			assert.NotEqual(t, "bead_execution", fix.Mode,
				"phrase %q must not route to bead_execution; only worker commands may execute beads", phrase)

			allowedModes := []string{"interactive-steward", "queue_steward"}
			modeOK := false
			for _, m := range allowedModes {
				if fix.Mode == m {
					modeOK = true
					break
				}
			}
			assert.True(t, modeOK,
				"phrase %q must route to interactive-steward or queue_steward, got %q", phrase, fix.Mode)
		})
	}
}

// TestDDxModeRoutingPrompts_DoWorkPrefersWorkerCommand asserts that "work the
// queue" style phrases are routed to bead_execution mode with ddx work (or
// ddx work --once) in queue_commands, rather than leaving the agent to pick
// and manually implement a ready bead in-session.
//
// AC3 from bead ddx-08c9d6a7.
func TestDDxModeRoutingPrompts_DoWorkPrefersWorkerCommand(t *testing.T) {
	root := interactiveStewardRepoRoot(t)
	fixtures := loadRouteFixtures(t, root)
	phraseIndex := make(map[string]routeFixture, len(fixtures))
	for _, fix := range fixtures {
		phraseIndex[fix.Phrase] = fix
	}

	// These phrases all mean "drain the queue" and must route to bead_execution
	// with a ddx work worker command — not to interactive-steward (which would
	// prompt the agent to pick a bead and implement it manually in-session).
	workerDispatchPhrases := []string{
		"work the queue",
		"drain the queue",
		"do work",
	}
	for _, phrase := range workerDispatchPhrases {
		phrase := phrase
		t.Run(phrase, func(t *testing.T) {
			fix, ok := phraseIndex[phrase]
			require.True(t, ok, "phrase %q must be present in routing.jsonl", phrase)

			assert.Equal(t, "bead_execution", fix.Mode,
				"phrase %q must route to bead_execution (worker dispatch), not interactive-steward", phrase)

			hasWorkerCmd := false
			for _, cmd := range fix.QueueCommands {
				if cmd == "ddx work" || cmd == "ddx work --once" {
					hasWorkerCmd = true
					break
				}
			}
			assert.True(t, hasWorkerCmd,
				"phrase %q must include ddx work or ddx work --once in queue_commands (got %v); "+
					"recommending the worker command prevents local manual execution", phrase, fix.QueueCommands)
		})
	}
}

// TestDDxWorkOnce_BeadExecutionIgnoresQueueStewardDefault asserts the
// ddx work --once routing entry uses bead_execution mode with
// code_edits_allowed=true, proving the worker path is not subject to
// queue-steward read-only defaults. Also cross-checks that the interactive
// mode for the same style of prompt (blocking queue question) is read-only,
// confirming the two paths are correctly separated.
//
// AC4 from bead ddx-08c9d6a7.
func TestDDxWorkOnce_BeadExecutionIgnoresQueueStewardDefault(t *testing.T) {
	root := interactiveStewardRepoRoot(t)
	fixtures := loadRouteFixtures(t, root)
	phraseIndex := make(map[string]routeFixture, len(fixtures))
	for _, fix := range fixtures {
		phraseIndex[fix.Phrase] = fix
	}

	// Worker path: ddx work --once must be bead_execution with code edits allowed.
	workerFix, ok := phraseIndex["ddx work --once"]
	require.True(t, ok, "'ddx work --once' must be present in routing.jsonl")
	assert.Equal(t, "bead_execution", workerFix.Mode,
		"'ddx work --once' must route to bead_execution, not interactive-steward")
	assert.True(t, workerFix.CodeEditsAllowed,
		"'ddx work --once' must allow code edits (bead AC implementation)")

	// Interactive path: the blocking-queue question must be read-only,
	// proving the queue-steward default is NOT the worker default.
	interactiveFix, hasInteractive := phraseIndex["what's blocking the queue now"]
	if hasInteractive {
		assert.False(t, interactiveFix.CodeEditsAllowed,
			"'what's blocking the queue now' must not allow code edits; "+
				"contrasts with ddx work --once to prove the worker carve-out")
		assert.NotEqual(t, "bead_execution", interactiveFix.Mode,
			"interactive blocking question must not use bead_execution; "+
				"the queue-steward default is read-only, worker mode is the exception")
	}

	// FEAT-010 must define both bead_execution and queue_steward as distinct
	// modes so the separation is grounded in a governing document.
	feat010Path := filepath.Join(root, "docs/helix/01-frame/features/FEAT-010-task-execution.md")
	feat010Bytes, err := os.ReadFile(feat010Path)
	require.NoError(t, err, "FEAT-010 must be readable")
	feat010 := string(feat010Bytes)

	assert.True(t, strings.Contains(feat010, "bead_execution"),
		"FEAT-010 must define bead_execution as a distinct mode that permits implementation")
	assert.True(t, strings.Contains(feat010, "queue_steward") || strings.Contains(feat010, "queue-steward"),
		"FEAT-010 must define queue_steward/queue-steward mode to distinguish it from bead_execution")
}

// TestDDxDirectImplementationMode_AllowsExplicitUserEdits asserts that
// explicit human requests to modify code or docs are not blocked by
// queue-steward guidance. An explicit implementation verb routes to
// direct_user_implementation with code_edits_allowed=true, and AGENTS.md
// must name that mode separately from the read-only interactive default.
//
// AC5 from bead ddx-08c9d6a7.
func TestDDxDirectImplementationMode_AllowsExplicitUserEdits(t *testing.T) {
	root := interactiveStewardRepoRoot(t)
	fixtures := loadRouteFixtures(t, root)
	phraseIndex := make(map[string]routeFixture, len(fixtures))
	for _, fix := range fixtures {
		phraseIndex[fix.Phrase] = fix
	}

	// Explicit implementation request must allow code edits and must not
	// route to interactive-steward (which is read-only).
	explicitFix, ok := phraseIndex["implement the top ready bead"]
	require.True(t, ok, "'implement the top ready bead' must be present in routing.jsonl")
	assert.True(t, explicitFix.CodeEditsAllowed,
		"explicit implementation request must allow code edits")
	assert.NotEqual(t, "interactive-steward", explicitFix.Mode,
		"explicit implementation request must not route to interactive-steward (read-only mode)")
	assert.Equal(t, "direct_user_implementation", explicitFix.Mode,
		"explicit implementation request must use direct_user_implementation mode")

	// AGENTS.md must name direct_user_implementation as a distinct mode from
	// the interactive/queue-steward default so that explicit edits are never
	// suppressed by the broad interactive guard.
	agentsPath := filepath.Join(root, "AGENTS.md")
	agentsBytes, err := os.ReadFile(agentsPath)
	require.NoError(t, err, "AGENTS.md must be readable")
	agents := string(agentsBytes)

	assert.True(t, strings.Contains(agents, "direct_user_implementation"),
		"AGENTS.md must name direct_user_implementation mode (explicit edits are allowed)")
	assert.True(t,
		strings.Contains(agents, "queue_steward") ||
			strings.Contains(agents, "interactive-steward") ||
			strings.Contains(agents, "Default Interactive Mode"),
		"AGENTS.md must name the interactive/queue-steward mode (read-only default) "+
			"to show it is separate from direct_user_implementation")
}

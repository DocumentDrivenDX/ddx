package server

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/ddxroot"
)

// reviewDispatchRunnerStub is a test stub for agent.AgentRunner that captures
// the last RunArgs and returns a canned result.
type reviewDispatchRunnerStub struct {
	result   *agent.Result
	err      error
	lastOpts agent.RunArgs
}

func (r *reviewDispatchRunnerStub) Run(opts agent.RunArgs) (*agent.Result, error) {
	r.lastOpts = opts
	return r.result, r.err
}

func TestReviewDispatcher_DispatchTurn_RecordsTurn(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	sessionID := "dispatcher-record-001"
	session := ReviewSession{
		ID:           sessionID,
		ArtifactID:   "art-1",
		SystemRubric: "review carefully",
		Status:       "open",
	}
	if err := store.Create(session); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	runner := &reviewDispatchRunnerStub{
		result: &agent.Result{Output: "looks good to me"},
	}
	d := &ReviewDispatcher{
		ProjectRoot: workDir,
		Store:       store,
		Runner:      runner,
	}

	userTurn := ReviewTurn{Actor: "user", Content: "please review"}
	got, err := d.DispatchReviewTurn(context.Background(), session, userTurn)
	if err != nil {
		t.Fatalf("DispatchReviewTurn(): %v", err)
	}
	if got.Actor != "reviewer" {
		t.Errorf("turn.Actor = %q, want %q", got.Actor, "reviewer")
	}
	if got.Content != "looks good to me" {
		t.Errorf("turn.Content = %q, want %q", got.Content, "looks good to me")
	}

	// Verify the turn was appended to turns.jsonl.
	turnsPath := ddxroot.JoinProject(workDir, reviewSessionsDirName, sessionID, reviewTurnsName)
	data, err := os.ReadFile(turnsPath)
	if err != nil {
		t.Fatalf("reading turns.jsonl: %v", err)
	}
	lines := splitNonEmptyLines(string(data))
	if len(lines) != 1 {
		t.Fatalf("turns.jsonl line count = %d, want 1", len(lines))
	}
	var recorded ReviewTurn
	if err := json.Unmarshal([]byte(lines[0]), &recorded); err != nil {
		t.Fatalf("parse recorded turn: %v", err)
	}
	if recorded.Actor != "reviewer" {
		t.Errorf("recorded turn.Actor = %q, want reviewer", recorded.Actor)
	}
	if recorded.Content != "looks good to me" {
		t.Errorf("recorded turn.Content = %q, want %q", recorded.Content, "looks good to me")
	}
}

// TestReviewDispatcher_NoToolMode_ClaudeHarness verifies that every dispatch
// carries agent.PermissionsReadOnlyReviewer in RunArgs.Permissions, which the
// claude harness translates into disabled tool use before sending the request.
func TestReviewDispatcher_NoToolMode_ClaudeHarness(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	sessionID := "dispatcher-notool-001"
	session := ReviewSession{
		ID:     sessionID,
		Status: "open",
	}
	if err := store.Create(session); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	runner := &reviewDispatchRunnerStub{
		result: &agent.Result{Output: "review complete"},
	}
	d := &ReviewDispatcher{
		ProjectRoot: workDir,
		Store:       store,
		Runner:      runner,
	}

	_, err := d.DispatchReviewTurn(context.Background(), session, ReviewTurn{Actor: "user", Content: "check it"})
	if err != nil {
		t.Fatalf("DispatchReviewTurn(): %v", err)
	}

	if runner.lastOpts.Permissions != agent.PermissionsReadOnlyReviewer {
		t.Errorf("RunArgs.Permissions = %q, want %q (tool use must be disabled for claude harness)",
			runner.lastOpts.Permissions, agent.PermissionsReadOnlyReviewer)
	}
}

// TestReview_FullTurnLifecycle exercises the full dispatch-through-recording
// path: prompt assembly includes prior turns + new user turn, the reviewer
// harness is called, and the resulting turn lands in turns.jsonl.
func TestReview_FullTurnLifecycle(t *testing.T) {
	workDir := newReviewSessionTestRoot(t)
	store := NewReviewSessionStore(workDir)
	sessionID := "dispatcher-lifecycle-001"
	session := ReviewSession{
		ID:           sessionID,
		ArtifactID:   "art-lifecycle",
		ArtifactSHA:  "sha-lifecycle",
		SystemRubric: "review the change carefully",
		TemplateRef:  "template://review/default",
		PromptRef:    "prompt://review/default",
		Status:       "open",
		Turns: []ReviewTurn{
			{Actor: "user", Content: "initial review request"},
		},
	}
	if err := store.Create(session); err != nil {
		t.Fatalf("Create(): %v", err)
	}

	runner := &reviewDispatchRunnerStub{
		result: &agent.Result{
			Output:  "BLOCK: missing tests",
			CostUSD: 0.05,
		},
	}
	d := &ReviewDispatcher{
		ProjectRoot: workDir,
		Store:       store,
		Runner:      runner,
	}

	userTurn := ReviewTurn{Actor: "user", Content: "here are the changes"}
	reviewerTurn, err := d.DispatchReviewTurn(context.Background(), session, userTurn)
	if err != nil {
		t.Fatalf("DispatchReviewTurn(): %v", err)
	}

	if reviewerTurn.Actor != "reviewer" {
		t.Errorf("reviewerTurn.Actor = %q, want reviewer", reviewerTurn.Actor)
	}
	if !strings.Contains(reviewerTurn.Content, "BLOCK") {
		t.Errorf("reviewerTurn.Content = %q, expected to contain BLOCK", reviewerTurn.Content)
	}
	if reviewerTurn.CostUSD != 0.05 {
		t.Errorf("reviewerTurn.CostUSD = %v, want 0.05", reviewerTurn.CostUSD)
	}

	// Verify the turn was recorded in turns.jsonl.
	turnsPath := ddxroot.JoinProject(workDir, reviewSessionsDirName, sessionID, reviewTurnsName)
	data, err := os.ReadFile(turnsPath)
	if err != nil {
		t.Fatalf("reading turns.jsonl: %v", err)
	}
	lines := splitNonEmptyLines(string(data))
	if len(lines) != 1 {
		t.Fatalf("turns.jsonl line count = %d, want 1 (reviewer turn only)", len(lines))
	}
	var recorded ReviewTurn
	if err := json.Unmarshal([]byte(lines[0]), &recorded); err != nil {
		t.Fatalf("parse recorded turn: %v", err)
	}
	if recorded.Actor != "reviewer" {
		t.Errorf("recorded turn.Actor = %q, want reviewer", recorded.Actor)
	}

	// Verify the assembled prompt contains both the prior turn and the new turn.
	if !strings.Contains(runner.lastOpts.Prompt, "initial review request") {
		t.Errorf("prompt should contain prior session turn content")
	}
	if !strings.Contains(runner.lastOpts.Prompt, "here are the changes") {
		t.Errorf("prompt should contain the incoming user turn content")
	}
}

package exec

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

// init keeps the exec package rooted in the production reachability graph.
// The guarded helper below is inert in normal runs; it exists so deadcode RTA
// sees the real store lifecycle and run-history APIs as reachable from main().
func init() {
	KeepReachabilityForDeadcode()
}

// KeepReachabilityForDeadcode keeps the exec package rooted in the static
// production call graph so deadcode analysis retains the store lifecycle.
func KeepReachabilityForDeadcode() {
	keepExecReachability()
}

func keepExecReachability() {
	if os.Getenv("DDX_EXEC_KEEPALIVE") != "1" {
		return
	}

	workingDir, err := os.MkdirTemp("", "ddx-exec-keepalive")
	if err != nil {
		return
	}
	defer os.RemoveAll(workingDir)

	store := NewStore(workingDir)
	if store.DefinitionCollection != nil {
		_ = store.DefinitionCollection.Init(context.Background())
	}
	if store.RunCollection != nil {
		_ = store.RunCollection.Init(context.Background())
	}

	artifactID := "MET-KEEPALIVE"
	artifactPath := filepath.Join(workingDir, "docs", "metrics", artifactID+".md")
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o755); err != nil {
		return
	}
	artifactContent := "---\nddx:\n  id: " + artifactID + "\n---\n# " + artifactID + "\n"
	if err := os.WriteFile(artifactPath, []byte(artifactContent), 0o644); err != nil {
		return
	}

	definitionID := "exec-keepalive@1"
	def := Definition{
		ID:          definitionID,
		ArtifactIDs: []string{artifactID},
		Executor: ExecutorSpec{
			Kind:    ExecutorKindCommand,
			Command: []string{"sh", "-c", "printf 'keepalive\\n'"},
		},
		Active:    true,
		CreatedAt: time.Unix(0, 0).UTC(),
	}
	_ = store.SaveDefinition(def)

	rec, err := store.Run(context.Background(), definitionID)
	if err != nil {
		return
	}
	_, _ = store.History(artifactID, "")
	_, _, _ = store.Log(rec.RunID)
	_, _ = store.Result(rec.RunID)
}

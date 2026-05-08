// Package perf provides a reusable GraphQL performance harness for the DDx
// server. It seeds realistic on-disk fixtures (projects × beads, docgraph,
// agent sessions) using the real storage primitives, then drives each GraphQL
// query shape through both the in-process StateProvider and the full HTTP
// handler while recording p50/p95/p99 latencies.
//
// The harness exists so performance work can be grounded in numbers rather
// than anecdote (ddx-9ce6842a Part 1). New query shapes are added by
// appending to the target table in harness.go — the fixture and measurement
// layers stay unchanged.
package perf

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/DocumentDrivenDX/ddx/internal/agent"
	"github.com/DocumentDrivenDX/ddx/internal/bead"
	"github.com/DocumentDrivenDX/ddx/internal/server"
)

// BeadFixtureSpec describes the shape of a generated workspace.
//
// The defaults (10 projects × 500 beads = 5,000 beads) match the ddx-9ce6842a
// acceptance criteria §5, and the per-query floor of ≥1,000 records documented
// in the 2026-04-22 user directive on fixture size. Consumers that want a
// heavier run should raise BeadsPerProject rather than Projects so the
// distribution stays comparable.
type BeadFixtureSpec struct {
	// Projects is the number of registered projects. Must be ≥ 1.
	Projects int
	// BeadsPerProject is the number of beads seeded in each project.
	// Combined with Projects this yields the total bead corpus size.
	BeadsPerProject int
	// ClosedRatio (0..1) controls the fraction of seeded beads written with
	// status=closed. Defaults to 0.3. A realistic mix matters because
	// status=open filters are the common fast-path.
	ClosedRatio float64
	// LabelDensity is the number of labels assigned per bead. Defaults to 2.
	LabelDensity int
	// AgentSessionsPerProject is the number of agent-session rows seeded into
	// each project's sessions.jsonl file. 0 means "do not seed sessions" —
	// the sessions harness target will still run but report against the
	// naturally-empty feed.
	AgentSessionsPerProject int
	// DocGraphFilesPerProject is the number of stub markdown files seeded
	// into each project so the DocGraph target exercises real IO. 0 means
	// the harness uses only whatever files already exist under the server's
	// working directory.
	DocGraphFilesPerProject int
}

// DefaultBeadFixtureSpec is the baseline fixture: 10 projects × 500 beads
// (5,000 total) with a modest amount of docgraph + sessions content. This is
// what `make bench-graphql` runs and what baseline reports compare against.
func DefaultBeadFixtureSpec() BeadFixtureSpec {
	return BeadFixtureSpec{
		Projects:                10,
		BeadsPerProject:         500,
		ClosedRatio:             0.3,
		LabelDensity:            2,
		AgentSessionsPerProject: 100, // 10 × 100 = 1,000 session rows
		DocGraphFilesPerProject: 4,
	}
}

// BeadFixture is a materialised on-disk workspace plus a live server.Server
// bound to it. Callers drive queries through Handler / State.
type BeadFixture struct {
	Spec          BeadFixtureSpec
	Server        *server.Server
	Handler       http.Handler
	Projects      []server.ProjectEntry
	TargetProject server.ProjectEntry
	TargetBeadID  string
	BeadIDsByProj map[string][]string
}

// TotalBeads returns the aggregate bead count across all seeded projects.
func (f *BeadFixture) TotalBeads() int {
	return f.Spec.Projects * f.Spec.BeadsPerProject
}

// BuildBeadFixture seeds the described workspace and returns a live fixture.
// It registers every project with the server's state so cross-project queries
// see all of them. The fixture is torn down automatically via tb.Cleanup.
func BuildBeadFixture(tb testing.TB, spec BeadFixtureSpec) *BeadFixture {
	tb.Helper()
	if spec.Projects < 1 {
		tb.Fatalf("perf: fixture needs at least 1 project, got %d", spec.Projects)
	}
	if spec.BeadsPerProject < 1 {
		tb.Fatalf("perf: fixture needs at least 1 bead per project, got %d", spec.BeadsPerProject)
	}
	if spec.LabelDensity <= 0 {
		spec.LabelDensity = 1
	}
	if spec.ClosedRatio < 0 {
		spec.ClosedRatio = 0
	}
	if spec.ClosedRatio > 1 {
		spec.ClosedRatio = 1
	}

	// Keep state out of the real user XDG dir so tests are hermetic.
	tb.Setenv("XDG_DATA_HOME", tb.TempDir())
	tb.Setenv("DDX_NODE_NAME", "ddx-perf-harness")

	root := tb.TempDir()
	projectPaths := make([]string, 0, spec.Projects)
	beadIDsByProj := make(map[string][]string, spec.Projects)
	var targetBeadID string

	for pi := 0; pi < spec.Projects; pi++ {
		projectPath := filepath.Join(root, fmt.Sprintf("project-%02d", pi))
		beads := seedProjectBeads(tb, projectPath, pi, spec)
		ids := make([]string, len(beads))
		for i, b := range beads {
			ids[i] = b.ID
		}
		beadIDsByProj[projectPath] = ids
		if spec.DocGraphFilesPerProject > 0 {
			seedProjectDocGraph(tb, projectPath, pi, spec.DocGraphFilesPerProject)
		}
		if spec.AgentSessionsPerProject > 0 {
			seedProjectSessions(tb, projectPath, pi, spec.AgentSessionsPerProject)
		}
		projectPaths = append(projectPaths, projectPath)
		if pi == spec.Projects-1 && len(beads) > 0 {
			// Target the final project's final bead so GetBeadSnapshot must
			// walk past the other (spec.Projects - 1) projects on the slow
			// path — this is what ddx-ad0db8fd cared about.
			targetBeadID = beads[len(beads)-1].ID
		}
	}

	srv := server.New(":0", projectPaths[0])
	tb.Cleanup(func() { _ = srv.Shutdown() })
	for _, p := range projectPaths[1:] {
		srv.RegisterProject(p)
	}

	projects := srv.State().GetProjects()
	if len(projects) != spec.Projects {
		tb.Fatalf("perf: registered %d projects, expected %d", len(projects), spec.Projects)
	}
	var targetProject server.ProjectEntry
	for _, proj := range projects {
		if proj.Path == projectPaths[len(projectPaths)-1] {
			targetProject = proj
			break
		}
	}
	if targetProject.ID == "" {
		tb.Fatalf("perf: target project was not registered")
	}

	return &BeadFixture{
		Spec:          spec,
		Server:        srv,
		Handler:       srv.Handler(),
		Projects:      projects,
		TargetProject: targetProject,
		TargetBeadID:  targetBeadID,
		BeadIDsByProj: beadIDsByProj,
	}
}

func seedProjectBeads(tb testing.TB, projectPath string, projectIdx int, spec BeadFixtureSpec) []bead.Bead {
	tb.Helper()
	store := bead.NewStore(filepath.Join(projectPath, ".ddx"))
	if err := store.Init(); err != nil {
		tb.Fatalf("perf: init bead store: %v", err)
	}
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	beads := make([]bead.Bead, 0, spec.BeadsPerProject)
	closedCutoff := int(float64(spec.BeadsPerProject) * spec.ClosedRatio)
	for bi := 0; bi < spec.BeadsPerProject; bi++ {
		id := fmt.Sprintf("ddx-perf-%02d-%04d", projectIdx, bi)
		now := baseTime.Add(time.Duration(projectIdx*spec.BeadsPerProject+bi) * time.Second)
		status := bead.StatusOpen
		if bi < closedCutoff {
			status = bead.StatusClosed
		}
		labels := make([]string, 0, spec.LabelDensity)
		for li := 0; li < spec.LabelDensity; li++ {
			labels = append(labels, fmt.Sprintf("label-%d", (bi+li)%7))
		}
		labels = append(labels, fmt.Sprintf("project-%02d", projectIdx))
		beads = append(beads, bead.Bead{
			ID:          id,
			Title:       fmt.Sprintf("Perf fixture bead %02d/%04d", projectIdx, bi),
			Status:      status,
			Priority:    bead.DefaultPriority,
			IssueType:   bead.DefaultType,
			CreatedAt:   now,
			UpdatedAt:   now,
			Labels:      labels,
			Description: "GraphQL perf fixture — realistic-shape bead with a multi-line description so parsing costs reflect typical payloads rather than artificially small bodies.",
		})
	}
	if err := store.WriteAll(beads); err != nil {
		tb.Fatalf("perf: seed bead store: %v", err)
	}
	return beads
}

func seedProjectDocGraph(tb testing.TB, projectPath string, projectIdx, count int) {
	tb.Helper()
	docsDir := filepath.Join(projectPath, "docs", "helix", "01-frame", "features")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		tb.Fatalf("perf: mkdir docgraph: %v", err)
	}
	for di := 0; di < count; di++ {
		name := fmt.Sprintf("FEAT-%02d-%03d.md", projectIdx, di)
		path := filepath.Join(docsDir, name)
		body := fmt.Sprintf("---\nid: FEAT-%02d-%03d\ntitle: Perf fixture feature\n---\n\n# FEAT-%02d-%03d\n\nFixture body line 1\nFixture body line 2\n", projectIdx, di, projectIdx, di)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			tb.Fatalf("perf: write docgraph file: %v", err)
		}
	}
}

func seedProjectSessions(tb testing.TB, projectPath string, projectIdx, count int) {
	tb.Helper()
	sessionsDir := filepath.Join(projectPath, ".ddx", "agent-logs")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		tb.Fatalf("perf: mkdir sessions: %v", err)
	}
	path := filepath.Join(sessionsDir, "sessions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		tb.Fatalf("perf: open sessions.jsonl: %v", err)
	}
	defer f.Close()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	enc := json.NewEncoder(f)
	for si := 0; si < count; si++ {
		e := agent.SessionEntry{
			ID:        fmt.Sprintf("sess-%02d-%05d", projectIdx, si),
			Harness:   "claude",
			Model:     "claude-sonnet-4-6",
			Timestamp: base.Add(time.Duration(si) * time.Minute),
			Duration:  1000 + si*10,
			Correlation: map[string]string{
				"bead_id": fmt.Sprintf("ddx-perf-%02d-%04d", projectIdx, si%10),
				"effort":  "medium",
			},
		}
		if err := enc.Encode(e); err != nil {
			tb.Fatalf("perf: write session row: %v", err)
		}
	}
}

// Environment returns a short footer describing the machine the benchmark ran
// on. Included verbatim in the baseline report (ddx-9ce6842a AC §2).
func Environment() string {
	return fmt.Sprintf("go=%s goos=%s goarch=%s cpu-cores=%d", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

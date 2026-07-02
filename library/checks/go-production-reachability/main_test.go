package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fixture builds a synthetic git repo containing a Go module with a single
// main package, commits an initial revision (without the new symbol), then
// applies a follow-up edit that adds an exported function. Returns the
// project root, base SHA, and head SHA.
type fixture struct {
	root string
	base string
	head string
}

// initialMain is committed at fixture base. It defines main() that calls
// existing helper Reachable.
const initialMain = `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
}

func Reachable() string {
	return "ok"
}
`

func setupFixture(t *testing.T, headMain string) *fixture {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/m\n\ngo 1.21\n")
	writeFile(t, filepath.Join(root, "main.go"), initialMain)
	writeFile(t, filepath.Join(root, ".ddx", "beads.jsonl"), "")

	gitInit(t, root)
	gitCommit(t, root, "initial")
	base := gitRev(t, root, "HEAD")

	writeFile(t, filepath.Join(root, "main.go"), headMain)
	gitCommit(t, root, "add new symbol")
	head := gitRev(t, root, "HEAD")

	return &fixture{root: root, base: base, head: head}
}

func runCheck(t *testing.T, f *fixture, beadsLine string) result {
	t.Helper()
	if beadsLine != "" {
		writeFile(t, filepath.Join(f.root, ".ddx", "beads.jsonl"), beadsLine+"\n")
	}
	evidenceDir := filepath.Join(f.root, ".ddx", "executions", "test")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}

	bin := buildBinary(t)
	cmd := exec.Command(bin,
		"--module-dir=.",
		"--packages=./...",
		"--name=production-reachability",
	)
	cmd.Dir = f.root
	cmd.Env = append(os.Environ(),
		"PROJECT_ROOT="+f.root,
		"EVIDENCE_DIR="+evidenceDir,
		"DIFF_BASE="+f.base,
		"DIFF_HEAD="+f.head,
		"BEAD_ID=test-bead",
		"CHECK_NAME=production-reachability",
		"RUN_ID=test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check binary failed: %v\n%s", err, out)
	}

	resultFile := filepath.Join(evidenceDir, "production-reachability.json")
	data, err := os.ReadFile(resultFile)
	if err != nil {
		t.Fatalf("read result: %v\nstdout:\n%s", err, out)
	}
	var r result
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("parse result %s: %v", data, err)
	}
	return r
}

// AC #5: an unwired exported function added in HEAD → status=block,
// listing the symbol in violations.
func TestUnwiredFunctionBlocks(t *testing.T) {
	f := setupFixture(t, `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
}

func Reachable() string {
	return "ok"
}

// Unwired is exported but never called from main.
func Unwired() string {
	return "dead"
}
`)
	r := runCheck(t, f, "")
	if r.Status != "block" {
		t.Fatalf("status=%q; want block. msg=%s violations=%v", r.Status, r.Message, r.Violations)
	}
	found := false
	for _, v := range r.Violations {
		if v.Symbol == "Unwired" && v.Kind == "unreachable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Unwired in violations; got %+v", r.Violations)
	}
}

// AC #6: same symbol but wired into the cmd handler (main) → status=pass.
func TestWiredFunctionPasses(t *testing.T) {
	f := setupFixture(t, `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
	fmt.Println(NowWired())
}

func Reachable() string {
	return "ok"
}

func NowWired() string {
	return "alive"
}
`)
	r := runCheck(t, f, "")
	if r.Status != "pass" {
		t.Fatalf("status=%q; want pass. msg=%s violations=%v", r.Status, r.Message, r.Violations)
	}
}

// AC #7a: unwired but annotated with `// wiring:pending` citing an OPEN
// bead → status=pass.
func TestPendingAnnotationOpenBeadPasses(t *testing.T) {
	f := setupFixture(t, `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
}

func Reachable() string {
	return "ok"
}

// wiring:pending ddx-pending01
//
// Will be wired in once the dispatcher lands.
func ToBeWired() string {
	return "later"
}
`)
	r := runCheck(t, f, `{"id":"ddx-pending01","status":"open","title":"future bead"}`)
	if r.Status != "pass" {
		t.Fatalf("status=%q; want pass (annotation). msg=%s violations=%v", r.Status, r.Message, r.Violations)
	}
}

// AC #7b: annotation citing a CLOSED or MISSING bead → status=block with
// kind=invalid_annotation.
func TestPendingAnnotationClosedBeadBlocks(t *testing.T) {
	f := setupFixture(t, `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
}

func Reachable() string {
	return "ok"
}

// wiring:pending ddx-closed01
func ToBeWired() string {
	return "later"
}
`)
	r := runCheck(t, f, `{"id":"ddx-closed01","status":"closed","title":"already done"}`)
	if r.Status != "block" {
		t.Fatalf("status=%q; want block (closed bead). violations=%v", r.Status, r.Violations)
	}
	hasInvalid := false
	for _, v := range r.Violations {
		if v.Kind == "invalid_annotation" {
			hasInvalid = true
		}
	}
	if !hasInvalid {
		t.Fatalf("want invalid_annotation violation; got %+v", r.Violations)
	}
}

func TestPendingAnnotationMissingBeadBlocks(t *testing.T) {
	f := setupFixture(t, `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
}

func Reachable() string {
	return "ok"
}

// wiring:pending ddx-doesnotexist
func ToBeWired() string {
	return "later"
}
`)
	r := runCheck(t, f, `{"id":"ddx-someother","status":"open"}`)
	if r.Status != "block" {
		t.Fatalf("status=%q; want block (missing bead). violations=%v", r.Status, r.Violations)
	}
}

// AC #4: a unit-test-only reference must NOT satisfy reachability. Add a
// new exported function that is only called from a _test.go file.
func TestTestOnlyReferenceDoesNotSatisfy(t *testing.T) {
	f := setupFixture(t, `package main

import "fmt"

func main() {
	fmt.Println(Reachable())
}

func Reachable() string {
	return "ok"
}

func TestOnlyCallee() string {
	return "only-tests"
}
`)
	// Add a _test.go that references TestOnlyCallee. _test.go files are
	// not part of the diff filter (we exclude them from changed-file
	// scan) and deadcode is not run with -test, so the reference must
	// not save the symbol.
	writeFile(t, filepath.Join(f.root, "main_test.go"), `package main

import "testing"

func TestUses(t *testing.T) {
	if TestOnlyCallee() == "" {
		t.Fatal("nope")
	}
}
`)
	gitCommit(t, f.root, "add test")
	f.head = gitRev(t, f.root, "HEAD")

	r := runCheck(t, f, "")
	if r.Status != "block" {
		t.Fatalf("status=%q; want block (test-only call should not satisfy). violations=%v", r.Status, r.Violations)
	}
	found := false
	for _, v := range r.Violations {
		if v.Symbol == "TestOnlyCallee" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected TestOnlyCallee in violations; got %+v", r.Violations)
	}
}

// ----- test helpers -----

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@t"},
		{"config", "user.name", "T"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitCommit(t *testing.T, dir, msg string) {
	t.Helper()
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-q", "-m", msg, "--allow-empty"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitRev(t *testing.T, dir, ref string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", ref)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

var (
	binaryPath string
	binaryDir  string
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "go-prod-reach-bin-")
	if err != nil {
		panic(err)
	}
	binaryDir = dir
	binaryPath = filepath.Join(dir, "go-production-reachability")
	if runtime.GOOS == "windows" {
		binaryPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if msg, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(dir)
		panic("build: " + err.Error() + "\n" + string(msg))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func buildBinary(t *testing.T) string {
	t.Helper()
	return binaryPath
}

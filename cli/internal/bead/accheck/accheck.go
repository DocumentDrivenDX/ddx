// Package accheck mechanically verifies acceptance criteria against the
// working tree and diff. It is invoked by `ddx bead ac-check <id>` to produce
// a structured per-AC result file that both implementer (self-check
// pre-commit) and reviewer (gate ratification) consume.
//
// Per ddx-c739d7ec: the tool classifies each AC line into a kind
// (test-name | build-gate | negative | symbol | mechanical | prose) then
// resolves evidence mechanically. Prose AC always yields needs_judgment;
// never auto-pass, never auto-fail.
package accheck

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// SchemaVersion is bumped when the output JSON shape changes incompatibly.
const SchemaVersion = 1

// Kind classifies an AC line by the evidence we know how to gather for it.
type Kind string

const (
	KindTestName    Kind = "test-name"
	KindBuildGate   Kind = "build-gate"
	KindNegative    Kind = "negative"
	KindSymbol      Kind = "symbol"
	KindMechanical  Kind = "mechanical"
	KindCommand     Kind = "command"
	KindFilePath    Kind = "file-path"
	KindCommandInvk Kind = "command-invocation"
	KindProse       Kind = "prose"
)

// Result is the verdict on one AC item.
type Result string

const (
	ResultPass          Result = "pass"
	ResultFail          Result = "fail"
	ResultNeedsJudgment Result = "needs_judgment"
	ResultError         Result = "error"
)

// Item represents one parsed AC line with its classification.
type Item struct {
	AC    int    // 1-based AC number
	Text  string // original AC text (single line)
	Kind  Kind   // classified kind
	Name  string // extracted entity (test name, symbol, etc.); empty for prose
	Alias string // operator-declared substitution (e.g., TestFoo -> TestFoo/bar)
}

// Output is the structured per-AC verification output written to
// .ddx/executions/<attempt-id>/ac-check.json.
type Output struct {
	SchemaVersion int     `json:"schema_version"`
	BeadID        string  `json:"bead_id"`
	AttemptID     string  `json:"attempt_id,omitempty"`
	Summary       Summary `json:"summary"`
	Items         []Entry `json:"items"`
}

// Summary aggregates counts across items.
type Summary struct {
	Pass          int `json:"pass"`
	Fail          int `json:"fail"`
	NeedsJudgment int `json:"needs_judgment"`
	Error         int `json:"error"`
}

// Entry is one per-AC result in the structured output.
type Entry struct {
	AC       int    `json:"ac"`
	Kind     Kind   `json:"kind"`
	Name     string `json:"name,omitempty"`
	Result   Result `json:"result"`
	Evidence string `json:"evidence"`
}

// Context bundles the evaluator's inputs (working directory, diff range, package
// hint, optional ACAliases map). Tests inject this directly; the CLI builds it
// from flags.
type Context struct {
	WorkingDir string
	RevBase    string // typically "merge-base..HEAD" or a single ref; empty = no diff
	Packages   []string
	Aliases    map[int]string // AC number -> substitution (e.g. "TestFoo -> TestFoo/bar")

	// Test hooks. When non-nil, these replace the real implementations so tests
	// can drive deterministic outcomes without invoking go/git subprocesses.
	RunTest  func(name string, packages []string) (bool, string, error)
	GitGrep  func(symbol string) (int, string, error) // returns hit count + sample
	DiffHits func(symbol string) (int, error)         // hits in diff hunks specifically
	FileInfo func(path string) (existsBefore, existsAfter bool, renamedFrom string)
}

// ParseAcceptance splits the bead's acceptance text into numbered AC items.
// It recognises lines that start with "1.", "2.", etc., or "(a)", "(b)", etc.
// Continuation lines (no leading number) are appended to the prior item.
var itemHeadRE = regexp.MustCompile(`^\s*(?:(\d+)\.|\(([a-z])\))\s+(.*)$`)

func ParseAcceptance(s string) []Item {
	var items []Item
	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	idx := 0
	for scanner.Scan() {
		line := scanner.Text()
		if m := itemHeadRE.FindStringSubmatch(line); m != nil {
			idx++
			text := strings.TrimSpace(m[3])
			items = append(items, Item{AC: idx, Text: text})
		} else if len(items) > 0 {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			items[len(items)-1].Text += " " + trimmed
		}
	}
	for i := range items {
		items[i].Kind, items[i].Name = classify(items[i].Text)
	}
	return items
}

// classify inspects an AC line and decides which mechanical evaluator should
// resolve it. Order matters: more-specific patterns first.
func classify(text string) (Kind, string) {
	// 1. File path with line number AC: cli/cmd/work.go:133, etc.
	if m := filePathRE.FindString(text); m != "" {
		return KindFilePath, ""
	}

	// 2. Backtick-symbol AC: `Foo.Bar()`, `pruneEvidenceDirs`, etc.
	if m := backtickSymbolRE.FindStringSubmatch(text); m != nil {
		// If the negative cue appears outside the backticks, this is a negative
		// AC about the symbol, not a positive symbol AC.
		if hasNegativeCue(text) {
			return KindNegative, m[1]
		}
		return KindSymbol, m[1]
	}

	// 3. Test-name AC: explicit Go test identifier.
	if m := testNameRE.FindString(text); m != "" {
		return KindTestName, m
	}

	// 4. Build-gate AC: lefthook, go test ./..., pre-commit pass language,
	// go run, bash scripts, bun run, etc.
	if buildGateRE.MatchString(text) {
		return KindBuildGate, ""
	}

	// 5. Negative AC without backticks (still operates on a symbol name).
	if hasNegativeCue(text) {
		if name := extractNegativeSymbol(text); name != "" {
			return KindNegative, name
		}
	}

	// 6. Mechanical AC: rename / doc / comment / file moves.
	if mechanicalRE.MatchString(text) {
		return KindMechanical, ""
	}

	// 7. Command AC: a quoted or unquoted shell-like command followed by an
	// outcome verb (returns/exits/passes/fails/outputs). These are mechanically
	// verifiable — the orchestrator/operator runs the command and ratifies the
	// outcome — and must not be misclassified as prose.
	if commandRE.MatchString(text) {
		return KindCommand, ""
	}

	// 8. Default: prose.
	return KindProse, ""
}

var (
	backtickSymbolRE = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_.]*(?:\\(\\))?)`")
	testNameRE       = regexp.MustCompile(`\bTest[A-Z]\w*`)
	buildGateRE      = regexp.MustCompile(`(?i)\b(lefthook|go test|go build|go run|bun run|bash scripts|pre-commit|all pass|test.* green|tests? pass|cd \S+ &&)\b`)
	filePathRE       = regexp.MustCompile(`\b[\w./-]+\.(go|md|yaml|yml|json|sh|ts|tsx|svelte):\d+\b`)
	negativeCueRE    = regexp.MustCompile(`(?i)\b(no longer|does not|do not|never|removed?|cannot|must not|no calls? to|absence of|without|excluded from|stop tracking|untrack)\b`)
	mechanicalRE     = regexp.MustCompile(`(?i)\b(rename|relocate(d|s)?|moved? to|file (exists?|present)|comment|docs?(\s+only)?|documentation|gitignore)\b`)
	negSymbolWordRE  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_.]{3,}`)
	// commandRE detects ACs that wrap a runnable shell-like command with an
	// outcome verb. Two shapes:
	//   - Quoted command: '<cmd>' / "<cmd>" / `<cmd>` followed by a verb.
	//   - Unquoted command: line starts with a lowercase command-like token,
	//     has at least one argument, then a verb + result expression.
	commandRE = regexp.MustCompile(
		`^\s*(?:` +
			"(?:'[^']+'|\"[^\"]+\"|`[^`]+`)\\s+(?i:returns|exits|passes|fails|outputs)\\b" +
			`|` +
			`[a-z][\w./-]*(?:\s+\S+)+\s+(?i:returns|exits|passes|fails|outputs)\s+\S` +
			`)`,
	)
)

func hasNegativeCue(s string) bool { return negativeCueRE.MatchString(s) }

// extractNegativeSymbol pulls a plausible symbol token from a negative AC.
// Heuristic: longest identifier-like token after the negative cue.
func extractNegativeSymbol(text string) string {
	loc := negativeCueRE.FindStringIndex(text)
	if loc == nil {
		return ""
	}
	tail := text[loc[1]:]
	var best string
	for _, m := range negSymbolWordRE.FindAllString(tail, -1) {
		if len(m) > len(best) {
			best = m
		}
	}
	return best
}

// Evaluate resolves each item to a result entry. ACAliases from ctx override
// the Name on a per-AC basis.
func Evaluate(items []Item, ctx Context) []Entry {
	out := make([]Entry, 0, len(items))
	for _, item := range items {
		if alias, ok := ctx.Aliases[item.AC]; ok {
			item.Alias = alias
		}
		out = append(out, evaluateOne(item, ctx))
	}
	return out
}

func evaluateOne(item Item, ctx Context) Entry {
	target := item.Name
	if item.Alias != "" {
		target = item.Alias
	}
	switch item.Kind {
	case KindTestName:
		return evalTestName(item, target, ctx)
	case KindBuildGate:
		return evalBuildGate(item, ctx)
	case KindNegative:
		return evalNegative(item, target, ctx)
	case KindSymbol:
		return evalSymbol(item, target, ctx)
	case KindMechanical:
		return evalMechanical(item, ctx)
	case KindCommand:
		return evalCommand(item, ctx)
	case KindFilePath:
		return evalFilePath(item, ctx)
	default:
		return Entry{
			AC:       item.AC,
			Kind:     KindProse,
			Result:   ResultNeedsJudgment,
			Evidence: "no mechanical anchor; defer to reviewer judgment",
		}
	}
}

func evalTestName(item Item, target string, ctx Context) Entry {
	e := Entry{AC: item.AC, Kind: KindTestName, Name: target}
	if target == "" {
		e.Result = ResultError
		e.Evidence = "test-name AC could not resolve a test identifier"
		return e
	}
	run := ctx.RunTest
	if run == nil {
		e.Result = ResultError
		e.Evidence = "no test runner configured in context"
		return e
	}
	ok, detail, err := run(target, ctx.Packages)
	if err != nil {
		e.Result = ResultError
		e.Evidence = fmt.Sprintf("test runner error: %v", err)
		return e
	}
	if ok {
		e.Result = ResultPass
		e.Evidence = fmt.Sprintf("test %s passed; %s", target, strings.TrimSpace(detail))
		return e
	}
	e.Result = ResultFail
	e.Evidence = fmt.Sprintf("test %s did not pass; %s", target, strings.TrimSpace(detail))
	return e
}

func evalBuildGate(item Item, ctx Context) Entry {
	// Build gates are run separately (lefthook, go test ./...) by the orchestrator;
	// here we report needs_judgment so the reviewer ratifies the orchestrator's
	// exit code rather than the ac-check running them a second time.
	return Entry{
		AC:       item.AC,
		Kind:     KindBuildGate,
		Result:   ResultNeedsJudgment,
		Evidence: "build/gate AC; orchestrator reports exit code separately",
	}
}

func evalNegative(item Item, target string, ctx Context) Entry {
	e := Entry{AC: item.AC, Kind: KindNegative, Name: target}
	if target == "" {
		e.Result = ResultNeedsJudgment
		e.Evidence = "negative AC without a resolvable symbol; defer to reviewer"
		return e
	}
	grep := ctx.GitGrep
	if grep == nil {
		e.Result = ResultError
		e.Evidence = "no git-grep hook configured in context"
		return e
	}
	hits, sample, err := grep(target)
	if err != nil {
		e.Result = ResultError
		e.Evidence = fmt.Sprintf("git-grep error: %v", err)
		return e
	}
	if hits == 0 {
		e.Result = ResultPass
		e.Evidence = fmt.Sprintf("symbol %q has 0 hits in post-image; absence confirmed", target)
		return e
	}
	e.Result = ResultFail
	e.Evidence = fmt.Sprintf("symbol %q still has %d hits in post-image; AC requires absence (%s)", target, hits, strings.TrimSpace(sample))
	return e
}

func evalSymbol(item Item, target string, ctx Context) Entry {
	e := Entry{AC: item.AC, Kind: KindSymbol, Name: target}
	if target == "" {
		e.Result = ResultError
		e.Evidence = "symbol AC could not resolve an identifier"
		return e
	}
	grep := ctx.GitGrep
	if grep == nil {
		e.Result = ResultError
		e.Evidence = "no git-grep hook configured in context"
		return e
	}
	hits, sample, err := grep(target)
	if err != nil {
		e.Result = ResultError
		e.Evidence = fmt.Sprintf("git-grep error: %v", err)
		return e
	}
	if hits == 0 {
		e.Result = ResultFail
		e.Evidence = fmt.Sprintf("symbol %q has 0 hits in post-image; AC requires its presence", target)
		return e
	}
	diffHits := 0
	if ctx.DiffHits != nil {
		dh, derr := ctx.DiffHits(target)
		if derr == nil {
			diffHits = dh
		}
	}
	if diffHits == 0 {
		e.Result = ResultFail
		e.Evidence = fmt.Sprintf("symbol %q exists (%d hits) but does NOT appear in diff; AC asks for an added/touched change", target, hits)
		return e
	}
	e.Result = ResultPass
	e.Evidence = fmt.Sprintf("symbol %q present with %d hits; diff touches %d (%s)", target, hits, diffHits, strings.TrimSpace(sample))
	return e
}

func evalCommand(item Item, ctx Context) Entry {
	// Command ACs are run by the orchestrator/operator; the reviewer ratifies
	// the recorded exit code or output. ac-check does not execute them itself.
	return Entry{
		AC:       item.AC,
		Kind:     KindCommand,
		Result:   ResultNeedsJudgment,
		Evidence: "command AC; operator/orchestrator runs the command, reviewer ratifies outcome",
	}
}

func evalMechanical(item Item, ctx Context) Entry {
	// Mechanical ACs are file-presence / rename detection. Without a specific
	// path, we yield needs_judgment so the reviewer adjudicates after seeing the
	// diff name-status.
	return Entry{
		AC:       item.AC,
		Kind:     KindMechanical,
		Result:   ResultNeedsJudgment,
		Evidence: "mechanical AC; reviewer adjudicates against diff name-status",
	}
}

func evalFilePath(item Item, ctx Context) Entry {
	// File path with line reference ACs reference specific locations in the
	// codebase. The reviewer checks these paths and line numbers against the
	// diff to verify they exist and make sense in context.
	return Entry{
		AC:       item.AC,
		Kind:     KindFilePath,
		Result:   ResultNeedsJudgment,
		Evidence: "file path with line reference; reviewer verifies path exists and line is relevant",
	}
}

// Aggregate builds the final Output from items and entries.
func Aggregate(beadID, attemptID string, entries []Entry) Output {
	out := Output{
		SchemaVersion: SchemaVersion,
		BeadID:        beadID,
		AttemptID:     attemptID,
		Items:         entries,
	}
	for _, e := range entries {
		switch e.Result {
		case ResultPass:
			out.Summary.Pass++
		case ResultFail:
			out.Summary.Fail++
		case ResultNeedsJudgment:
			out.Summary.NeedsJudgment++
		case ResultError:
			out.Summary.Error++
		}
	}
	return out
}

// WriteHuman writes a human-readable table of the per-AC results.
func WriteHuman(w io.Writer, o Output) error {
	fmt.Fprintf(w, "ac-check for %s", o.BeadID)
	if o.AttemptID != "" {
		fmt.Fprintf(w, " (attempt %s)", o.AttemptID)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "summary: %d pass / %d fail / %d needs-judgment / %d error\n\n",
		o.Summary.Pass, o.Summary.Fail, o.Summary.NeedsJudgment, o.Summary.Error)
	for _, e := range o.Items {
		fmt.Fprintf(w, "AC #%d  [%-13s]  %-14s  %s\n", e.AC, e.Kind, e.Result, truncate(e.Evidence, 100))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// DefaultGitGrep returns a GitGrep hook that runs `git grep -c <symbol>`
// inside workingDir. Empty string symbols are rejected.
func DefaultGitGrep(workingDir string) func(symbol string) (int, string, error) {
	return func(symbol string) (int, string, error) {
		if symbol == "" {
			return 0, "", fmt.Errorf("empty symbol")
		}
		// Use a fixed-string match to avoid regex surprises.
		cmd := exec.Command("git", "grep", "-c", "-F", "--", symbol)
		cmd.Dir = workingDir
		out, _ := cmd.Output() // exit=1 means no match; not an error here
		hits := 0
		sample := ""
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			// format: "path/file.go:N"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				// best-effort numeric
				var n int
				if _, err := fmt.Sscanf(parts[1], "%d", &n); err == nil {
					hits += n
				}
			}
			if sample == "" {
				sample = line
			}
		}
		return hits, sample, nil
	}
}

// DefaultDiffHits returns a DiffHits hook that counts occurrences of symbol in
// `git diff <base>..HEAD` output. base may be empty (no diff filter).
func DefaultDiffHits(workingDir, base string) func(symbol string) (int, error) {
	return func(symbol string) (int, error) {
		if symbol == "" {
			return 0, nil
		}
		args := []string{"diff"}
		if base != "" {
			args = append(args, base)
		}
		cmd := exec.Command("git", args...)
		cmd.Dir = workingDir
		out, err := cmd.Output()
		if err != nil {
			return 0, err
		}
		return strings.Count(string(out), symbol), nil
	}
}

// DefaultRunTest returns a RunTest hook that invokes `go test -run '^<name>$'`
// against the given packages (or `./...` if none).
func DefaultRunTest(workingDir string) func(name string, packages []string) (bool, string, error) {
	return func(name string, packages []string) (bool, string, error) {
		args := []string{"test", "-v", "-run", "^" + regexp.QuoteMeta(name) + "$", "-count=1", "-short"}
		if len(packages) == 0 {
			args = append(args, "./...")
		} else {
			args = append(args, packages...)
		}
		cmd := exec.Command("go", args...)
		cmd.Dir = filepath.Join(workingDir, "cli") // tests live under cli/
		out, err := cmd.CombinedOutput()
		text := strings.TrimSpace(string(out))
		if err != nil {
			return false, text, nil // test failure surfaces as ok=false, err nil
		}
		// Definitive evidence: the test runner reported PASS for this specific test.
		// Checking for absence of "[no tests to run]" is unreliable when running
		// ./pkg/... because sub-packages without the test also output that string.
		if strings.Contains(text, "--- PASS: "+name) {
			return true, text, nil
		}
		return false, text + " (no matching test found)", nil
	}
}

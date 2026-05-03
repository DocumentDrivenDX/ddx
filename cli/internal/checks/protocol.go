// Package checks implements the language-agnostic pre-merge check protocol.
//
// A check is a project-declared mechanical gate evaluated before a worker's
// worktree is merged back. Each check is one YAML file under
// .ddx/checks/*.yaml describing a command to run and the situations it
// applies to (path globs, bead labels). The check command is invoked as a
// subprocess with the following environment variables injected:
//
//	BEAD_ID       — the bead being evaluated
//	DIFF_BASE     — git revision the work started from
//	DIFF_HEAD     — current worktree HEAD
//	PROJECT_ROOT  — absolute path to the project root
//	EVIDENCE_DIR  — directory the check must write its result file into
//	RUN_ID        — unique identifier for this evaluation run
//
// The check writes its result to ${EVIDENCE_DIR}/${name}.json with shape:
//
//	{
//	  "status":   "pass" | "block" | "error",
//	  "message":  "<human-readable summary>",
//	  "violations": [
//	    {"file": "...", "line": 0, "symbol": "...", "kind": "...", "detail": "..."}
//	  ]
//	}
//
// Exit code 0 indicates the check ran to completion (the status field in the
// result file is authoritative). A non-zero exit, a missing result file, or
// an unparseable result file is treated as status=error.
package checks

// Status enumerates the three possible check outcomes.
type Status string

const (
	StatusPass  Status = "pass"
	StatusBlock Status = "block"
	StatusError Status = "error"
)

// HookPreMerge is the only currently supported lifecycle hook.
const HookPreMerge = "pre_merge"

// Check is the parsed representation of one .ddx/checks/*.yaml file.
type Check struct {
	// Name uniquely identifies the check; also the basename of its result
	// file (${EVIDENCE_DIR}/${Name}.json). Required.
	Name string `yaml:"name"`
	// Command is the shell command to execute. Required. Run via
	// `sh -c <command>` so projects can use pipes/redirection.
	Command string `yaml:"command"`
	// When is the lifecycle hook the check binds to. Currently only
	// "pre_merge" is supported. Required.
	When string `yaml:"when"`
	// AppliesTo gates whether this check runs for a given (paths, labels)
	// invocation context. Zero value means "always".
	AppliesTo AppliesTo `yaml:"applies_to"`

	// SourceFile records the on-disk path the check was loaded from
	// (used for diagnostics). Not part of the YAML schema.
	SourceFile string `yaml:"-"`
}

// AppliesTo declares the conditions under which a check is evaluated.
//
// Semantics:
//   - If neither Paths nor Labels is set, the check ALWAYS applies.
//   - If Paths is set, the check applies when any changed path matches
//     any glob, evaluated against slash-normalized paths relative to
//     PROJECT_ROOT. The recursive "**" segment is supported.
//   - If Labels is set, the check applies when the bead carries any of
//     the listed labels.
//   - If both are set, EITHER a matching path OR a matching label causes
//     the check to apply (logical OR).
type AppliesTo struct {
	Paths  []string `yaml:"paths,omitempty"`
	Labels []string `yaml:"labels,omitempty"`
}

// Result is the parsed outcome of running one check.
type Result struct {
	Name       string      `json:"name"`
	Status     Status      `json:"status"`
	Message    string      `json:"message,omitempty"`
	Violations []Violation `json:"violations,omitempty"`

	// ExitCode and Stderr are captured by the runner for diagnostics
	// when status=error. Not part of the on-disk JSON contract.
	ExitCode int    `json:"-"`
	Stderr   string `json:"-"`
}

// Violation describes one offending location reported by a check.
type Violation struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Symbol string `json:"symbol,omitempty"`
	Kind   string `json:"kind,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// InvocationContext bundles the runtime values injected as environment
// variables into each check command, plus the bead labels and changed
// paths used to evaluate AppliesTo filtering.
type InvocationContext struct {
	BeadID       string
	DiffBase     string
	DiffHead     string
	ProjectRoot  string
	EvidenceDir  string
	RunID        string
	BeadLabels   []string
	ChangedPaths []string
}

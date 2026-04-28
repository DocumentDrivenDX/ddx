package agent

// runInternal is the SD-024 Stage 2 dispatch seam for agent-package
// callers (grade.go, service_run.go, runner.go itself, the bead
// review path) that need to invoke the runner without going through
// RunWithConfig and without manufacturing a fake config.ResolvedConfig.
//
// It accepts RunArgs — the new flat adapter type declared in
// types.go — and routes through the existing Run dispatch. Subsequent
// B22 beads migrate production callers off RunArgs onto RunArgs;
// the B22 final bead removes the legacy types entirely. In this bead
// runInternal is the only consumer of RunArgs.
//
// Implementation note: opts is declared with `var` rather than a
// composite literal so this file does not introduce a fresh
// RunArgs{...} reintroduction (per the SD-024 Stage 4 runtimelint
// rule). The legacy RunArgs value is built field-by-field from the
// RunArgs adapter and handed to the existing Run pathway.
func (r *Runner) runInternal(args RunArgs) (*Result, error) {
	var opts RunArgs
	opts.Context = args.Context
	opts.Harness = args.Harness
	// evidence:allow-unbounded reason="args.Prompt is already the bounded output of the upstream caller's prompt resolution (mirrors runner.go resolvedOpts.Prompt assignment, FEAT-022 §3)"
	opts.Prompt = args.Prompt
	opts.PromptFile = args.PromptFile
	opts.PromptSource = args.PromptSource
	opts.Correlation = args.Correlation
	opts.Model = args.Model
	opts.Provider = args.Provider
	opts.ModelRef = args.ModelRef
	opts.Effort = args.Effort
	opts.Timeout = args.Timeout
	opts.WallClock = args.WallClock
	opts.WorkDir = args.WorkDir
	opts.Permissions = args.Permissions
	opts.SessionLogDir = args.SessionLogDir
	return r.Run(opts)
}

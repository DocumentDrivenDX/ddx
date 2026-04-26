package agent

// Pattern 1: forbidden durable-knob fields on a Runtime struct
// declared in the agent package. Each field is listed individually so
// the analysistest pins each diagnostic to its own line.

type BadRuntime struct {
	Harness          string // want `runtimelint: forbidden durable-knob field Harness`
	Model            string // want `runtimelint: forbidden durable-knob field Model`
	Provider         string // want `runtimelint: forbidden durable-knob field Provider`
	ModelRef         string // want `runtimelint: forbidden durable-knob field ModelRef`
	Effort           string // want `runtimelint: forbidden durable-knob field Effort`
	Permissions      string // want `runtimelint: forbidden durable-knob field Permissions`
	Timeout          int    // want `runtimelint: forbidden durable-knob field Timeout`
	WallClock        int    // want `runtimelint: forbidden durable-knob field WallClock`
	ContextBudget    int    // want `runtimelint: forbidden durable-knob field ContextBudget`
	ReviewMaxRetries int    // want `runtimelint: forbidden durable-knob field ReviewMaxRetries`
	SessionLogDir    string // want `runtimelint: forbidden durable-knob field SessionLogDir`
	Assignee         string // want `runtimelint: forbidden durable-knob field Assignee`
}

// Pattern 3: function declared in agent pkg with a parameter typed as
// a legacy options type. Both pointer and value forms are covered.

func paramRunOptions(opts RunOptions) {} // want `runtimelint: function parameter typed as legacy options`

func paramRunOptionsPtr(opts *RunOptions) {} // want `runtimelint: function parameter typed as legacy options`

func paramExecuteBeadLoopOptions(opts ExecuteBeadLoopOptions) {} // want `runtimelint: function parameter typed as legacy options`

func paramExecuteBeadOptions(opts ExecuteBeadOptions) {} // want `runtimelint: function parameter typed as legacy options`

func paramCompareOptions(opts CompareOptions) {} // want `runtimelint: function parameter typed as legacy options`

func paramQuorumOptions(opts QuorumOptions) {} // want `runtimelint: function parameter typed as legacy options`

// Pattern 2 (also visible in agent pkg): composite literal of a legacy
// options type. Same package, but pattern 2 is "anywhere in the repo".

func compositeRunOptions() RunOptions {
	return RunOptions{Prompt: "x"} // want `runtimelint: composite literal of legacy options type RunOptions`
}

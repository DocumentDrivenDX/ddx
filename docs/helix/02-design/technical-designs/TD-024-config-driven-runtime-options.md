---
ddx:
  id: TD-024
  depends_on:
    - SD-024
---
# Technical Design: Config-Driven Runtime Options

## Type Definitions

### `config.ResolvedConfig`

Immutable value type. All fields unexported. Consumers call methods.

Construction is sealed: zero-value `ResolvedConfig{}` and
`var rcfg ResolvedConfig` are valid Go expressions, but every public
accessor checks a sentinel field on first read and panics with a
specific message naming `LoadAndResolve` if the sentinel is unset.
This makes a zero-value `ResolvedConfig` syntactically constructible
but operationally unusable.

```go
package config

// ResolvedConfig is the loop/runner/reviewer's view of merged
// project config plus per-invocation overrides. It is constructed
// only by (*Config).Resolve and is safe to share across goroutines.
//
// Sealed: every accessor calls (r ResolvedConfig).requireSealed()
// on entry. A zero-value ResolvedConfig fails loudly on first read.
type ResolvedConfig struct {
    // sealed is set to true by Resolve. Accessors panic if false.
    sealed bool

    // unexported fields holding the merged values
    assignee                 string
    reviewMaxRetries         int
    noProgressCooldown       time.Duration
    maxNoChangesBeforeClose  int
    heartbeatInterval        time.Duration
    harness                  string
    model                    string
    provider                 string
    modelRef                 string
    profile                  string
    minTier                  string
    maxTier                  string
    effort                   string
    permissions              string
    timeout                  time.Duration
    wallClock                time.Duration
    contextBudget            string
    evidenceCaps             EvidenceCapsResolution
    sessionLogDir            string
    mirrorConfig             ExecutionsMirrorResolution
    resolvedLadder           map[string][]string
    reasoningLevels          map[string][]string
    armModels                map[int]string
    sandbox                  bool
    keepSandbox              bool
    postRun                  string
    noReview                 bool
    record                   bool
    replay                   bool
}

// requireSealed panics if r was not produced by Resolve.
// Called as the first statement of every public accessor.
func (r ResolvedConfig) requireSealed() {
    if !r.sealed {
        panic("config: ResolvedConfig used without going through " +
            "(*Config).Resolve or config.LoadAndResolve. " +
            "Production callers must obtain a ResolvedConfig from " +
            "LoadAndResolve; tests must use NewTestConfigFor*.")
    }
}
```

**Sealed-construction edge cases (out of scope as defenses).**
Three escape paths exist in Go that the sentinel does not close:

- `reflect.New(typeof(ResolvedConfig)).Elem()` returns an unsealed
  zero-value. Realistic only if production code calls reflect to
  construct routing values, which it does not.
- `unsafe.Pointer` reinterpretation can flip `sealed` directly.
  Realistic only if production code uses `unsafe`, which it does
  not for routing types.
- `gob.Decode` / `json.Unmarshal` into a `*ResolvedConfig` cannot
  set the unexported `sealed` field, so a deserialized value will
  panic on first accessor read. This is the *correct* behavior —
  serialization of `ResolvedConfig` is not supported and the
  panic surfaces the misuse loudly. The codebase does not register
  `ResolvedConfig` for any of these formats.

These escape paths are documented and accepted as out of scope.
The lint and the `requireSealed` panic together cover every
realistic in-codebase construction path; closing the reflective/
unsafe paths would require runtime checks expensive enough to
hurt the loop's hot path.

Accessor surface (one method per durable knob, ~22 total):

```go
func (r ResolvedConfig) Assignee() string
func (r ResolvedConfig) ReviewMaxRetries() int
func (r ResolvedConfig) NoProgressCooldown() time.Duration
func (r ResolvedConfig) MaxNoChangesBeforeClose() int
func (r ResolvedConfig) HeartbeatInterval() time.Duration
func (r ResolvedConfig) Harness() string
func (r ResolvedConfig) Model() string
func (r ResolvedConfig) Provider() string
func (r ResolvedConfig) ModelRef() string
func (r ResolvedConfig) Profile() string
func (r ResolvedConfig) MinTier() string
func (r ResolvedConfig) MaxTier() string
func (r ResolvedConfig) Effort() string
func (r ResolvedConfig) Permissions() string
func (r ResolvedConfig) Timeout() time.Duration
func (r ResolvedConfig) ContextBudget() string
func (r ResolvedConfig) EvidenceCaps() EvidenceCapsResolution
func (r ResolvedConfig) SessionLogDir() string
func (r ResolvedConfig) MirrorConfig() ExecutionsMirrorResolution
func (r ResolvedConfig) ResolvedLadder() map[string][]string
```

`EvidenceCapsResolution`, `ExecutionsMirrorResolution`, and the ladder
map returned from `ResolvedLadder()` are themselves immutable types or
defensively-copied snapshots. Returning a `map[string][]string` is
acceptable because callers reading it does not mutate the
`ResolvedConfig`'s underlying state — but a defensive copy is returned
on each call to make accidental mutation a no-op on the source.

### `config.CLIOverrides`

```go
package config

// CLIOverrides carries per-invocation flag values that override
// project config. Zero values mean "no override; use config".
// Boolean fields use *bool so test-side and runtime-side can
// distinguish "explicit false" from "not set".
type CLIOverrides struct {
    Harness     string
    Model       string
    Provider    string
    ModelRef    string
    Profile     string
    Effort      string
    Permissions string
    MinTier     string
    MaxTier     string
    Timeout     *time.Duration
    NoReview    *bool
    Assignee    string
}
```

`SpecOverrides` is an alias of `CLIOverrides` used at the server
dispatch site to map `ExecuteLoopWorkerSpec` fields onto the
`Resolve` call. Same shape, different conceptual origin.

### `(*Config).Resolve`

```go
package config

// Resolve produces a sealed ResolvedConfig by layering the provided
// overrides onto cfg. Overrides are applied as last-write-wins per
// field. The returned ResolvedConfig does not alias cfg's storage —
// every map and slice held by cfg's substructs is deep-cloned via
// per-type Clone methods before being captured.
//
// Resolve accepts a nil cfg and returns a ResolvedConfig populated
// from package defaults, sealed normally. The single exception that
// would warrant panicking — a misconstructed cfg from internal code
// — is caught at LoadAndResolve, not here. This matches the
// existing pre-refactor pattern where config.Load() callers
// commonly use `cfg, _ := Load()` and tolerate load failures.
func (cfg *Config) Resolve(overrides CLIOverrides) ResolvedConfig
```

Per-type Clone methods are required, not optional, to prevent
shared-state bugs:

```go
func (a *AgentConfig) Clone() *AgentConfig                 // copies Models, ReasoningLevels, Endpoints, Routing
func (r *RoutingConfig) Clone() *RoutingConfig             // copies ProfileLadders, ModelOverrides, plus other maps
func (e *EvidenceCapsConfig) Clone() *EvidenceCapsConfig   // copies PerHarness map AND its *EvidenceCapsOverride values
func (m *ExecutionsMirrorConfig) Clone() *ExecutionsMirrorConfig
func (w *WorkersConfig) Clone() *WorkersConfig
```

Each substruct that contains a map or slice (or a pointer to
something containing a map or slice) gets a `Clone` method that
recurses appropriately. `Resolve` calls these explicitly. Generic
shallow `*cfg` copies are forbidden — they alias every map.

Accessors that return collection types (e.g. `ResolvedLadder()
map[string][]string`) return a fresh copy on each call. The cost of
the per-call copy is acceptable for the resolution path; nothing in
the loop's hot path calls these accessors per-iteration.

### `config.LoadAndResolve`

```go
package config

// LoadAndResolve is the canonical dispatch-site helper. It loads
// the project's configuration from projectRoot, then calls Resolve
// with overrides. Returns the sealed ResolvedConfig and any
// load error.
//
// On load error, LoadAndResolve still returns a usable
// ResolvedConfig built from package defaults plus overrides — the
// caller decides whether to surface the error or proceed. This
// matches the pre-refactor pattern (pre-existing call sites use
// `cfg, _ := config.Load(...)` and tolerate failures). Production
// callers that require a valid config should check the error and
// abort; production callers that want best-effort behavior can
// proceed with the defaults-resolved value.
func LoadAndResolve(projectRoot string, overrides CLIOverrides) (ResolvedConfig, error)
```

Exactly three production sites call `LoadAndResolve`:

- `cli/cmd/agent_cmd.go:1997` — CLI `ddx work` dispatch.
- `cli/internal/server/workers.go:878` — server `runWorker` dispatch.
- `cli/internal/server/graphql/resolver_feat008.go:77` — GraphQL
  `StartWorker` mutation.

Adding a fourth dispatch site (e.g. a future REST endpoint that
starts a worker) requires calling `LoadAndResolve` at that site.
There is no other supported way to obtain a `ResolvedConfig` in
production code.

### Runtime structs

Each family loses its `*Options` name and durable fields.

```go
// cli/internal/agent/execute_bead_loop.go
type ExecuteBeadLoopRuntime struct {
    Log          io.Writer
    EventSink    LoopEventSink
    ProgressCh   chan<- LoopProgress
    PreClaimHook func(ctx context.Context, beadID string) error
    Once         bool
    PollInterval time.Duration  // runtime intent: tick rate, not durable
    NoReview     bool           // runtime intent per round-3 reclassification
    LabelFilter  string
    SessionID    string
    WorkerID     string
    // ProjectRoot derived from caller; passed to LoadAndResolve, not stored here.
}

// cli/internal/agent/types.go
type AgentRunRuntime struct {
    Prompt                 string
    PromptFile             string
    PromptSource           PromptSource          // runtime plumbing
    Output                 io.Writer
    Correlation            string
    Record                 bool
    Replay                 bool
    WorkDir                string
    EstimatedPromptTokens  int                   // agent v0.9.9+ smart-routing hint
    RequiresTools          bool                  // agent v0.9.9+ smart-routing hint
    SessionLogDirOverride  string                // per-invocation override; durable default lives in Config.Agent.SessionLogDir
}

// cli/internal/agent/execute_bead.go
type ExecuteBeadRuntime struct {
    Log         io.Writer
    FromRev     string
    Once        bool
    WorkerID    string
    PromptFile  string
    BeadEvents  BeadEventWriter   // runtime plumbing
    Service     agentlib.DdxAgent // test injection seam
    AgentRunner AgentRunner       // test injection seam
}
```

Comparison and quorum flows (`CompareOptions`, `QuorumOptions`) embed
`AgentRunRuntime`. They retain their per-call configuration (arms,
quorum strategy, sandbox flag) which is genuinely runtime intent.

## Run signatures (post-refactor)

```go
// Loop
func (w *ExecuteBeadWorker) Run(
    ctx context.Context,
    rcfg config.ResolvedConfig,
    runtime ExecuteBeadLoopRuntime,
) (*ExecuteBeadLoopResult, error)

// Single bead
func ExecuteBead(
    ctx context.Context,
    beadID string,
    rcfg config.ResolvedConfig,
    runtime ExecuteBeadRuntime,
) (*ExecuteBeadReport, error)

// Agent run
func Run(
    ctx context.Context,
    rcfg config.ResolvedConfig,
    runtime AgentRunRuntime,
) (*Result, error)
```

## Dispatch site code shape (post-Stage-1)

CLI:

```go
// cli/cmd/agent_cmd.go ~ runAgentExecuteLoop
overrides := config.CLIOverrides{
    Harness:  harnessFlag,
    Model:    modelFlag,
    Profile:  profileFlag,
    NoReview: optBool(noReviewFlag, cmd),
}
rcfg, err := config.LoadAndResolve(projectRoot, overrides)
if err != nil { return err }

runtime := agent.ExecuteBeadLoopRuntime{
    Log:        cmd.OutOrStdout(),
    Once:       onceFlag,
    SessionID:  sessionIDFlag,
    WorkerID:   workerIDFlag,
}
result, err := worker.Run(ctx, rcfg, runtime)
```

Server:

```go
// cli/internal/server/workers.go ~ runWorker
overrides := config.CLIOverrides{
    Harness:  spec.Harness,
    Model:    spec.Model,
    Profile:  spec.Profile,
    MinTier:  spec.MinTier,
    MaxTier:  spec.MaxTier,
    NoReview: optBool(spec.NoReview),
}
rcfg, err := config.LoadAndResolve(projectRoot, overrides)
if err != nil { return err }

runtime := agent.ExecuteBeadLoopRuntime{
    Log:        wlog,
    EventSink:  workerSink,
    ProgressCh: progress,
    Once:       spec.Once,
    SessionID:  spec.SessionID,
    WorkerID:   workerID,
}
result, err := worker.Run(ctx, rcfg, runtime)
```

The same pattern applies at the GraphQL resolver site, replacing
`spec` with the GraphQL `StartWorkerInput`.

## Field classification

Every field on every options family gets an explicit classification:
**durable** (moves into `*Config`, resolved to `ResolvedConfig`),
**runtime plumbing** (stays on `*Runtime` — non-serializable
or per-invocation control plane), or **runtime intent** (stays on
`*Runtime` — invocation-scoped flag with no durable analogue).

### `ExecuteBeadLoopOptions` → `ExecuteBeadLoopRuntime`

| Field                       | Class             | Post-refactor location                      |
|---                          |---                |---                                          |
| `Assignee`                  | durable           | `Config.Workers.Assignee`                   |
| `Once`                      | runtime intent    | `ExecuteBeadLoopRuntime.Once`               |
| `PollInterval`              | runtime intent    | `ExecuteBeadLoopRuntime.PollInterval`       |
| `NoProgressCooldown`        | durable (new)     | `Config.Workers.NoProgressCooldown`         |
| `MaxNoChangesBeforeClose`   | durable (new)     | `Config.Workers.MaxNoChangesBeforeClose`    |
| `HeartbeatInterval`         | durable (new)     | `Config.Workers.HeartbeatInterval`          |
| `Log`                       | runtime plumbing  | `ExecuteBeadLoopRuntime.Log`                |
| `EventSink`                 | runtime plumbing  | `ExecuteBeadLoopRuntime.EventSink`          |
| `ProgressCh`                | runtime plumbing  | `ExecuteBeadLoopRuntime.ProgressCh`         |
| `PreClaimHook`              | runtime plumbing  | `ExecuteBeadLoopRuntime.PreClaimHook`       |
| `NoReview`                  | runtime intent    | `ExecuteBeadLoopRuntime.NoReview` (every today-call site at `cli/cmd/agent_cmd.go:1565`, `cli/internal/server/workers.go:42,859,894`, `graphql_adapters.go:72,133`, `server.go:2006,2051`, and `execute_bead_loop.go:160` is per-invocation; no project-durable consumer exists. Reclassified per round-3 review.) |
| `LabelFilter`               | runtime intent    | `ExecuteBeadLoopRuntime.LabelFilter`        |
| `SessionID`                 | runtime intent    | `ExecuteBeadLoopRuntime.SessionID`          |
| `WorkerID`                  | runtime intent    | `ExecuteBeadLoopRuntime.WorkerID`           |
| `ProjectRoot`               | runtime intent    | derived; passed to `LoadAndResolve`         |
| `Harness`/`Model`/`Provider`/`ModelRef` | durable | `Config.Agent.*`, override-able           |
| `Profile`                   | durable           | `Config.Agent.Routing.Profile`              |
| `MinTier`/`MaxTier`         | durable           | `Config.Workers.DefaultSpec.MinTier` / `MaxTier` (existing fields at `cli/internal/config/types.go:70-71`; no new field needed) |
| `ReviewMaxRetries`          | durable (already) | already in `Config`; loop reads via `rcfg`  |

### `RunOptions` → `AgentRunRuntime`

| Field                       | Class             | Post-refactor location                      |
|---                          |---                |---                                          |
| `Harness`/`Model`/`Provider`/`ModelRef` | durable | `Config.Agent.*`                          |
| `Effort`                    | durable           | `Config.Agent.Effort`                       |
| `Permissions`               | durable           | `Config.Agent.Permissions`                  |
| `Timeout`                   | durable           | `Config.Agent.Timeout`                      |
| `WallClock`                 | durable           | `Config.Agent.WallClock`                    |
| `Prompt`                    | runtime intent    | `AgentRunRuntime.Prompt`                    |
| `PromptFile`                | runtime intent    | `AgentRunRuntime.PromptFile`                |
| `PromptSource`              | runtime plumbing  | `AgentRunRuntime.PromptSource`              |
| `Output`                    | runtime plumbing  | `AgentRunRuntime.Output`                    |
| `Correlation`               | runtime intent    | `AgentRunRuntime.Correlation`               |
| `Record`/`Replay`           | runtime intent    | `AgentRunRuntime.Record`/`.Replay`          |
| `WorkDir`                   | runtime intent    | `AgentRunRuntime.WorkDir`                   |
| `Context`                   | runtime plumbing  | passed via `ctx` parameter                  |
| `EstimatedPromptTokens`     | runtime intent    | `AgentRunRuntime.EstimatedPromptTokens` — per-invocation hint (agent v0.9.9+) for smart-routing context-window gate |
| `RequiresTools`             | runtime intent    | `AgentRunRuntime.RequiresTools` — per-invocation flag (agent v0.9.9+) signalling that the prompt requires tool support |
| `SessionLogDir`             | durable + override | `Config.Agent.SessionLogDir` (durable);   |
|                             |                   | `AgentRunRuntime.SessionLogDirOverride` (per-invocation override for worktree-scoped logs) |

### `ExecuteBeadOptions` → `ExecuteBeadRuntime`

| Field                       | Class             | Post-refactor location                      |
|---                          |---                |---                                          |
| `Harness`/`Model`/`Provider`/`ModelRef`/`Effort` | durable | `Config.Agent.*`                       |
| `ContextBudget`             | durable (new home) | `Config.Agent.EvidenceCaps.ContextBudget`  |
| `MirrorCfg`                 | durable (new wiring) | `Config.Executions.Mirror` (already parsed; now consumed) |
| `Log`                       | runtime plumbing  | `ExecuteBeadRuntime.Log`                    |
| `FromRev`                   | runtime intent    | `ExecuteBeadRuntime.FromRev`                |
| `Once`                      | runtime intent    | `ExecuteBeadRuntime.Once`                   |
| `WorkerID`                  | runtime intent    | `ExecuteBeadRuntime.WorkerID`               |
| `PromptFile`                | runtime intent    | `ExecuteBeadRuntime.PromptFile`             |
| `BeadEvents`                | runtime plumbing  | `ExecuteBeadRuntime.BeadEvents`             |
| `Service`                   | test injection seam | `ExecuteBeadRuntime.Service` (kept; tests need this) |
| `AgentRunner`               | test injection seam | `ExecuteBeadRuntime.AgentRunner`          |

### `CompareOptions` / `QuorumOptions`

These embed `RunOptions` (post-refactor: `AgentRunRuntime`).
Per-call comparison/quorum knobs that genuinely belong with the
invocation, not config:

| Field                       | Class             | Post-refactor location                      |
|---                          |---                |---                                          |
| `Arms` / `Harnesses`        | runtime intent    | `CompareRuntime.Arms` / `QuorumRuntime.Harnesses` |
| `ArmModels`                 | runtime intent    | `CompareRuntime.ArmModels`                  |
| `Sandbox`                   | runtime intent    | `CompareRuntime.Sandbox`                    |
| `KeepSandbox`               | runtime intent    | `CompareRuntime.KeepSandbox`                |
| `PostRun`                   | runtime intent    | `CompareRuntime.PostRun`                    |
| `Strategy` (quorum)         | runtime intent    | `QuorumRuntime.Strategy`                    |

These could plausibly become durable knobs in the future (e.g. a
project that always wants sandboxed compare runs), but adding them
to config is out of scope for this refactor. The classification is
explicit so a future caller can elevate one without re-architecting.

### `*Config` extensions

Two existing sub-structs extended (no new fields invented beyond
those listed; `WorkersConfig.DefaultSpec.MinTier`/`MaxTier` already
exist at `cli/internal/config/types.go:70-71` and are reused):
- `WorkersConfig` adds: `Assignee`, `NoProgressCooldown`,
  `MaxNoChangesBeforeClose`, `HeartbeatInterval`. The existing
  `WorkersConfig.DefaultSpec` (carrying `Harness`, `Profile`,
  `Effort`, `MinTier`, `MaxTier`) remains the primary surface for
  per-spec defaults; the new fields layer on as additional defaults
  for the loop's policy knobs that don't fit the spec shape.
  Resolution precedence: `CLIOverrides` > `WorkersConfig.DefaultSpec`
  for the Spec fields > new `WorkersConfig.*` defaults > package
  default constants. `NoReview` is **not** added to `WorkersConfig`
  (per round-3 reclassification — every consumer is per-invocation).
- `EvidenceCapsConfig` adds: `ContextBudget` (currently a CLI flag
  with no config home).

`ExecutionsConfig.Mirror` is parsed today but never consumed by
`ExecuteBeadOptions.MirrorCfg`. Stage 3 wires the consumption.

## Test config constructors

Per the user's directive: constructors, not builders. Each constructor
takes an explicit options struct that names every field a test might
care about. Zero defaults for production-relevant knobs — tests that
omit a field get a sentinel value that fails fast on read.

```go
package config

// TestLoopConfigOpts names every durable knob the loop reads.
// Tests must specify each. Zero strings and zero durations are
// allowed only where production also accepts them.
type TestLoopConfigOpts struct {
    Assignee                string
    ReviewMaxRetries        int
    NoProgressCooldown      time.Duration
    MaxNoChangesBeforeClose int
    HeartbeatInterval       time.Duration
    Harness                 string
    Model                   string
    Profile                 string
    MinTier                 string
    MaxTier                 string
    EvidenceCaps            EvidenceCapsConfig
}

// NewTestConfigForLoop returns a *Config that, when Resolve()d,
// produces a ResolvedConfig matching opts exactly. Used by tests
// for the execute-loop path.
func NewTestConfigForLoop(opts TestLoopConfigOpts) *Config

// Sibling constructors for the other paths.
func NewTestConfigForRun(opts TestRunConfigOpts) *Config
func NewTestConfigForBead(opts TestBeadConfigOpts) *Config
```

`NewTestConfigForLoop` cannot be bypassed by direct struct
construction in tests, because the test cannot construct a
`ResolvedConfig` itself (no exported fields). The only way to obtain
one is via `Resolve` on a `*Config`, and the only test-friendly way
to obtain a `*Config` is via these constructors.

## Lint rule (Stage 4)

Reuses the analyzer pattern from FEAT-022 Stage A2 (the
`evidencelint` analyzer in `cli/tools/lint/evidencelint/`). Adds a
sibling analyzer `runtimelint`.

**Scope discipline:** the lint applies *only* to struct types whose
name ends in `Runtime` and that are declared in `cli/internal/agent/`.
This avoids false-positives on result/status/event types that
legitimately contain fields named `Harness`, `Model`, `Provider`,
etc. — those are observable outputs of an invocation, not durable
inputs. Codex flagged this risk concretely: `cli/internal/agent/types.go`
has multiple result/status structs at lines 67-69 and 131-143 that
must not trigger the rule.

The analyzer flags:

1. **Forbidden field names on `*Runtime` structs.** Any struct in
   `cli/internal/agent/` whose name ends in `Runtime` containing a
   field whose name matches one of the durable-knob names from the
   classification table above. Concrete forbidden names (closed
   list, not regex-derived to avoid drift):
   `Harness`, `Model`, `Provider`, `ModelRef`, `Profile`, `Effort`,
   `Permissions`, `Timeout`, `WallClock`, `ContextBudget`, `MinTier`,
   `MaxTier`, `Assignee`, `ReviewMaxRetries`, `NoProgressCooldown`,
   `MaxNoChangesBeforeClose`, `HeartbeatInterval`,
   `SessionLogDir` (durable variant), `MirrorCfg`, `Models`,
   `ReasoningLevels`, `Endpoints`, `ProfileLadders`,
   `ModelOverrides`, `PerHarness`.
   Closed list is maintained as a Go constant; adding a new durable
   knob to `*Config` requires adding its name to this list.

   `NoReview`, `PollInterval`, and `SessionLogDirOverride` are
   explicitly NOT on the forbidden list — all three are runtime
   intent (per-invocation), not durable. The lint must allow them
   on `*Runtime` structs. Note `SessionLogDir` (durable) is
   forbidden but `SessionLogDirOverride` (per-invocation override)
   is allowed; the rename in the runtime struct enforces the
   distinction.

2. **No reintroduction of legacy options types.** Composite literal
   `Xxx{...}` where `Xxx` is one of `ExecuteBeadLoopOptions`,
   `RunOptions`, `ExecuteBeadOptions`, `CompareOptions`,
   `QuorumOptions` and the surrounding package is anywhere in the
   repo. These types must not exist post-cleanup; this catches
   reintroduction.

3. **No `*Options` parameters.** Any function declared in
   `cli/internal/agent/` whose signature includes a parameter typed
   as one of the legacy options types named in (2). These types
   must not exist post-cleanup; this catches accidental restoration.

4. **`PollInterval` exempt.** `PollInterval` is runtime intent, not
   durable — it's the loop's tick rate, scoped to a single
   invocation. The lint must not flag it. Codex explicitly flagged
   this as a false-positive risk on the original regex-based rule.

The lint plugs into the same Lefthook pre-commit hook and CI job as
`evidencelint`. Stage 4's acceptance includes running the analyzer
against the post-cleanup tree and asserting zero violations.

## Disabled test resolution

Per-file decisions for the four `*.disabled` files in
`cli/internal/config/`. Each decision is grounded in a grep of the
current loader for the underlying feature:

- **`config_enhanced_test.go.disabled`** — exercises `Validate` on
  `*Config`. Validation entry points exist in production (`cli/cmd/doctor.go`,
  config-load error paths). Test fixtures reference the v1.0 schema;
  current schema has drifted. **Action**: rewrite against the
  current schema and add coverage for `Resolve` validation paths.
  Restore as `config_enhanced_test.go`.

- **`config_library_test.go.disabled`** — tests `GetLibraryPathWithWorkingDir`,
  which **does not exist in the current code**. The current entry
  point is `ResolveLibraryResource` (`cli/internal/config/config.go:180`).
  Library path resolution is alive but the test's API target is gone.
  **Action**: delete the disabled file, write fresh tests against
  `ResolveLibraryResource` covering the cases the old test covered
  (relative path, absolute path, override path). The new tests live
  in a new file under `cli/internal/config/`; the old `.disabled` is
  removed with a one-line commit rationale citing API drift.

- **`config_us018_test.go.disabled`** — tests `${VAR}` substitution
  in YAML values. Grep confirms **no substitution code in the current
  loader** (`cli/internal/config/loader.go`); the only env-var
  consumer is `os.Getenv("DDX_LIBRARY_BASE_PATH")` at
  `cli/internal/config/config.go:81`. The feature the test targets
  is dead. **Action**: delete with a one-line commit rationale
  citing dead-feature confirmation. Do not restore.

- **`config_us019_test.go.disabled`** — tests environment-overlay
  files (`.ddx.dev.yml`, `.ddx.staging.yml`). Grep confirms **no
  overlay loader in the current loader** — `loader.go:39-47` only
  reads `.ddx/config.yaml`; no `DDX_ENV` lookup, no `.ddx.{env}.yml`
  pattern matching. The feature is dead. **Action**: delete with a
  one-line commit rationale citing dead-feature confirmation.

Net coverage outcome: of the four files, two restored (with rewrites
where needed), two deleted with explicit dead-feature rationale. The
restored two contribute new passing tests; the deleted two represent
test debt being cleared rather than test coverage being lost.

Stage 4's acceptance gates on this set of four decisions being
executed exactly: two new `*_test.go` files exist with the rewritten
content, two `.disabled` files no longer exist, and the commit log
records the rationale for each.

## Migration sequence and dependencies

```
Stage 1 (foundation + loop)
    │
    ├──> blocks: FEAT-022 Stage G (ddx-70c1d2e2)
    │
Stage 2 (RunOptions)            ── independent of Stage 3
Stage 3 (ExecuteBeadOptions)    ── independent of Stage 2
    │
Stage 4 (lint + disabled tests) ── depends on Stages 1, 2, 3
```

Stage 1 must land first because it introduces the foundation types
(`ResolvedConfig`, `Resolve`, `LoadAndResolve`, the test-config
constructors). Stages 2 and 3 reuse those types and are otherwise
independent. Stage 4 mechanically enforces what Stages 1–3 have
established.

## Test plan

Each stage adds tests that go through the production code path, not
test-injection shortcuts. The e2e tests are behavioral, not
introspective: they assert **what the loop does**, not **what
internal state the loop holds**. This eliminates the temptation to
add synthetic telemetry events whose only purpose is to expose
internals — which would itself be the injection-seam pattern this
refactor rejects.

**Stage 1 e2e behavioral tests (non-negotiable):**

For `ReviewMaxRetries` (the bug class's load-bearing case):

```
TestReviewRetryThresholdFromConfigCLI:
  given a temp project root with .ddx/config.yaml setting
       review_max_retries: 5,
  given a deterministic test runner that returns
       a parseable APPROVE only after the Nth review attempt,
  when ddx work --once is invoked against a bead seeded to fail
       review on the first 4 attempts and succeed on the 5th,
  then the bead closes successfully on attempt 5,
   and no review-manual-required event is emitted on attempts 1-4.

  Companion test:
  given the same setup,
  when the bead is seeded to fail on the first 5 attempts,
  then the bead emits review-manual-required on attempt 5,
   and the bead's status transitions to manual-required.

TestReviewRetryThresholdFromConfigServer:
  given a registered project with the same config,
  when StartWorker is invoked via runWorker,
  then the same observable behavior holds.

TestReviewRetryThresholdFromConfigGraphQL:
  given the same project served via the GraphQL endpoint,
  when StartWorker is invoked via the GraphQL mutation,
  then the same observable behavior holds.
```

Each test verifies the contract through observable bead events
(`review-manual-required`, `review-error`, bead status transitions),
not through inspection of any private struct field or any synthetic
event added for testability.

For `NoProgressCooldown` and `MaxNoChangesBeforeClose`: similar
behavioral tests once their resolvers land in Stage 1. Each has
observable consequences (claim reattempts, bead closure after N
no-change iterations) that a deterministic test runner can drive.

**Per-stage accessor tests:** every public method on `ResolvedConfig`
gets a unit test asserting (a) the value comes from `*Config` when no
override is set, (b) the override wins when set, (c) sealed-access
panic fires on a zero-value `ResolvedConfig{}`. ~25 accessor unit
tests in Stage 1.

**Sealed-construction tests:** explicit tests that
`var rcfg config.ResolvedConfig` and `config.ResolvedConfig{}` panic
on first accessor call with a message naming `LoadAndResolve`.
Without these tests, future refactoring could quietly remove the
sentinel check.

**Migration sanity:** each grouping of beads that migrates an options
family includes a "behavioral snapshot" test captured pre-rename
(asserting the existing observable behavior on representative
inputs). The same snapshot test must pass post-rename against the
new `*Runtime` API. Snapshots are kept in the suite until the
cleanup bead retires them.

**Stage 4 lint tests:** the `runtimelint` analyzer's own test
fixtures include forbidden patterns (durable field on `*Runtime`,
legacy `*Options` literal, legacy `*Options` parameter type) and
assert non-zero exit per fixture. Plus a positive test on the
current tree post-cleanup asserting zero violations.

## Bead inventory

The work splits into ~30 small beads. Each fits one execute-bead
worker window (target <500 LoC net change, <20 min agent time, <$10
spend) and leaves the tree green. Stage 1 unblocks FEAT-022 Stage G;
Stages 2-4 follow.

The inventory below is the authoritative breakdown the bead tracker
will receive. Bead IDs are assigned at `ddx bead create` time.

### Stage 1 — Foundation + Loop migration (unblocks FEAT-022 G)

**Foundation:**

1. `config: ResolvedConfig type + sealed-construction sentinel +
   accessor unit tests`. Adds the type, the sentinel field, the
   `requireSealed` panic helper, and unit tests for sealed-access
   panic + every accessor returning zero-value-after-seal. No
   production code consumes it yet.
2. `config: CLIOverrides struct + Resolve method + per-type Clone
   methods + unit tests`. Adds `(*Config).Resolve` that returns a
   sealed `ResolvedConfig`, plus `Clone` methods on `AgentConfig`,
   `RoutingConfig`, `EvidenceCapsConfig`, `ExecutionsMirrorConfig`,
   `WorkersConfig`. Tests assert deep-copy correctness on each
   clone path.
3. `config: LoadAndResolve helper + unit tests`. The single
   canonical entry point used by all dispatch sites.
4. `config: NoProgressCooldown / MaxNoChangesBeforeClose /
   HeartbeatInterval resolvers on WorkersConfig + unit tests`.
   Adds the three resolvers with sensible defaults matching today's
   hardcoded constants. Loop doesn't read them yet.
5. `config: NewTestConfigForLoop constructor + opts struct + unit
   tests`. Tests construct a `*Config` via this, call `Resolve`,
   and assert every loop-relevant accessor returns the configured
   value.

**Loop add-alongside:**

6. `agent: ExecuteBeadLoopRuntime struct (durable fields stripped)
   + RunWithConfig method (alongside existing Run) + unit test`.
   New method delegates to existing implementation under the hood;
   no behavior change. Depends on beads 1-4 (the foundation types
   plus the new resolvers, since `RunWithConfig` consumes them).

6.5. `agent/test: deterministic review-failure runner fixture
   + helper for behavioral e2e tests`. Production review path has
   no test seam today (`grep TestReviewRetry|fakeReviewer` returns
   zero hits). Beads 7-9 each need this fixture to drive
   N-deterministic-failures-then-success behavior. Centralizing the
   fixture in one bead means beads 7-9 reuse it instead of each
   reinventing it (and disagreeing on the seam shape).

**Loop production migration (one site per bead):**

7. `cmd/work: migrate ddx work / execute-loop CLI dispatch to
   LoadAndResolve + RunWithConfig + behavioral e2e test for
   ReviewMaxRetries threshold`. Acceptance: behavioral test
   `TestReviewRetryThresholdFromConfigCLI` passes against a real
   `.ddx/config.yaml`. Depends on bead 6.5 (fixture).
8. `server/workers: migrate runWorker dispatch to LoadAndResolve +
   RunWithConfig + behavioral e2e test`. Acceptance: behavioral
   test `TestReviewRetryThresholdFromConfigServer` passes. Depends
   on bead 6.5.
9. `server/graphql: migrate StartWorker resolver to LoadAndResolve +
   behavioral e2e test`. Acceptance: behavioral test
   `TestReviewRetryThresholdFromConfigGraphQL` passes. Depends on
   bead 6.5.
10. `agent: behavioral e2e tests for NoProgressCooldown +
    MaxNoChangesBeforeClose`. Same shape as the
    `ReviewMaxRetries` triplet, against the new resolvers from
    bead 4.

**Loop test migration (mechanical, grouped by file — per round-3
oversize splits):**

11a. `agent/test: migrate first half of execute_bead_loop_test.go
    (~520 LoC, ~12 ExecuteBeadLoopOptions literals) off legacy
    options to RunWithConfig`.
11b. `agent/test: migrate second half of execute_bead_loop_test.go
    (~520 LoC, ~11 ExecuteBeadLoopOptions literals)`.
12a. `agent/test: migrate execute_bead_review_retry_test.go +
    execute_bead_review_failure_modes_test.go +
    execute_bead_review_taxonomy_test.go (~486 LoC)`.
12b. `agent/test: migrate execute_bead_review_evidence_test.go +
    execute_bead_review_test.go (~600+ LoC)`.
13. `agent/test: migrate execute_bead_integration_test.go +
    tier_escalation_integration_test.go`.
14. `agent/test: migrate cmd/agent_metrics_review_evidence_test.go
    + cmd/agent_execute_loop_test.go`.
15a. `server/test: migrate workers_test.go (~1107 LoC) — bulk
    mechanical rename`.
15b. `server/test: migrate workers_watchdog_test.go +
    workers_stop_propagation_test.go + any sibling
    workers_*_test.go file that constructs loop options`.

**Loop cleanup:**

16. `agent: retire ExecuteBeadLoopOptions + Run method (legacy
    path)`. After all callers migrated. Includes deletion of any
    transitional shims.

This unblocks FEAT-022 Stage G — bead 7 is the one that surfaces
`ReviewMaxRetries` from `.ddx/config.yaml` to the running CLI loop.
Bead 8 does the same for the server worker (which is what
`ddx work` actually dispatches to today).

### Stage 2 — RunOptions migration

17. `agent: AgentRunRuntime struct + RunWithConfig (alongside Run) +
    NewTestConfigForRun constructor`.
18. `cmd/run: migrate ddx run dispatch + behavioral
    test`.
19. `cmd/agent_compare: migrate ddx agent compare + behavioral
    test`.
20. `cmd/agent_quorum: migrate ddx agent quorum + behavioral test`.
21. `agent/test: migrate RunOptions test sites (5-10 sites)`.
22. `agent: retire RunOptions + Run legacy method`.

### Stage 3 — ExecuteBeadOptions migration

23. `agent: ExecuteBeadRuntime struct + RunWithConfig (alongside
    Run) + NewTestConfigForBead constructor`.
24. `config: add ContextBudget to EvidenceCapsConfig + resolver +
    unit test`.
25. `cmd/try: migrate ddx try
    dispatch + behavioral test`.
26. `agent: wire ExecutionsConfig.Mirror into ExecuteBeadRuntime
    via ResolvedConfig.MirrorConfig() + behavioral test`.
27a. `agent/test: migrate ExecuteBeadOptions sites in
    execute_bead_artifacts_test.go + execute_bead_checkpoint_test.go +
    execute_bead_env_isolation_test.go (~6 sites)`.
27b. `agent/test: migrate ExecuteBeadOptions sites in
    executions_mirror_test.go + execute_bead_routing_evidence_test.go +
    integration_helper_test.go + agent_runner_service_test.go (~6 sites)`.
27c. `agent/test: migrate remaining ExecuteBeadOptions sites in
    execute_bead_e2e_test.go + cmd/ test files (~4 sites)`.
28. `agent: retire ExecuteBeadOptions + legacy method`.

### Stage 4 — Lint enforcement + test debt cleanup

29. `tools/lint: runtimelint analyzer (closed-list field-name
    forbidden + *Options-type forbidden) + unit tests + Lefthook
    + CI wiring`.
30. `config/test: rewrite config_enhanced_test.go against current
    schema + Resolve validation paths`.
31. `config/test: write fresh library-path-resolution tests
    against ResolveLibraryResource; delete
    config_library_test.go.disabled`.
32. `config/test: delete config_us018_test.go.disabled +
    config_us019_test.go.disabled (dead-feature confirmation
    in commit messages)`.
33. `agent: final sweep — runtimelint passes against the tree`.

### Bead dependency graph

- Foundation beads (1-5) are independent of each other except 2
  depends on 1 (`Resolve` returns a `ResolvedConfig`).
- Loop add-alongside (6) depends on 1-4 (foundation types + the new
  resolvers, since `RunWithConfig` consumes them).
- 6.5 (review-failure fixture) depends on 6.
- Loop production migration (7, 8, 9) each depends on 6.5; bead 10
  depends on 4 (the new resolvers).
- Loop test migration (11a, 11b, 12a, 12b, 13, 14, 15a, 15b)
  depends on 6 and is otherwise parallel.
- Loop cleanup (16) depends on 7-10 + 11a/b + 12a/b + 13 + 14 +
  15a/b (every caller migrated).
- **Stage 2 add-alongside (17) depends only on beads 1-3** (not on
  bead 16). Per round-3 review: `RunOptions` is independent of
  `ExecuteBeadLoopOptions`, so Stage 2's add-alongside can start
  immediately after foundation. Only Stage 2's cleanup (22)
  depends on bead 16 — both retirements need to coordinate to
  avoid leaving orphan `*Runtime` ↔ `*Options` shims.
- **Stage 3 add-alongside (23) depends only on beads 1-3.** Same
  reasoning. Stage 3's cleanup (28) depends on bead 16.
- Stage 2 cleanup (22) depends on 16, 17, 18, 19, 20, 21.
- Stage 3 cleanup (28) depends on 16, 23, 24, 25, 26, 27a, 27b, 27c.
- Stage 4 lint (29) depends on 16, 22, 28.
- Stage 4 test debt (30-32) is independent.
- Stage 4 final sweep (33) depends on 29.

## Out of scope

Repeated from SD-024 §Scope for clarity at the implementation
boundary:

- No changes to `~/Projects/agent`.
- No changes to `agentlib.ExecuteRequest` / `ExecuteResponse`.
- No new operator commands or workflows.
- No changes to the bead tracker, session log, or execution bundle
  formats.
- The `model_routes` removal effort tracked elsewhere benefits from
  this refactor but is not delivered by it.

## References

- SD-024 — design rationale and alternatives considered.
- FEAT-006, FEAT-022, SD-019 — context and dependencies.
- FEAT-022 evidencelint pattern — analyzer reuse for Stage 4.

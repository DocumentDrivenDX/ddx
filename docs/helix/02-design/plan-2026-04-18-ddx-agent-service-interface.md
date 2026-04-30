# Plan: `agentlib.Service` interface — narrow public surface

> **Historical / superseded.** Retained as a design record for the
> 2026-04-18 service-boundary investigation. Do not use this file as current
> routing-contract guidance. The current boundary is the top-level
> `ddx run` / `ddx try` / `ddx work` stack: DDx owns bead orchestration,
> success classification, evidence, and retry escalation; the agent owns
> concrete model/provider routing behind `Execute` and operator/debug status
> surfaces. Pre-resolved route injection and DDx-side route decisions described
> below are obsolete.

## Problem

DDx imports **150+ symbols across 11 packages** of `github.com/DocumentDrivenDX/agent`:

| Package | uses | symbols (top) |
|---|---|---|
| `agent` (root) | 113 | Provider, StreamingProvider, Request, Response, Message, Options, Event + 5 event types, ToolDef, Tool, ToolCall, Compactor, Run, RoutingReport |
| `prompt` | 46 | NewMetaPromptInjectorWithPaths, NewFromPreset, PresetNames, LoadContextFiles |
| `virtual` | 32 | New, Config, InlineResponse |
| `tool` | 27 | Bash/Edit/Read/Write/Grep/Glob/Ls Tool |
| `session` | 17 | NewLogger |
| `agentconfig` | 16 | provider config types |
| `compaction` | 11 | NewCompactor, Config, DefaultConfig |
| `provider/openai` | 5 | provider construction, LookupModelLimits |
| `observations` | 3 | observability hookup |
| `modelcatalog` | 3 | catalog lookup |
| `provider/anthropic` | 2 | provider construction |

DDx has reimplemented the agent loop using ddx-agent as a parts catalog. Concrete consequences:

- `cli/internal/agent/agent_runner.go:148` — DDx constructs the compactor with hardcoded 131K, ignoring the model's actual 256K context (bragi qwen3.6-35b-a3b case).
- DDx orchestrates prompt construction, tool registration, session logging, compaction, observability — every concern the agent should own.
- Whenever upstream evolves a concept (compaction config, tool wrapper shape, event schema), DDx breaks or silently underutilizes the change.

## Proposal

Expose **one** interface from ddx-agent. All other types are either inputs/outputs of its methods or move internal.

```go
package agentlib

type Service interface {
    ListProviders(ctx context.Context) ([]ProviderInfo, error)
    ListModels(ctx context.Context, provider string) ([]ModelInfo, error)
    HealthCheck(ctx context.Context, provider string) error

    // Execute runs a single agent task to completion synchronously.
    Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)

    // ExecuteStream runs a single agent task and emits events as they happen.
    // The returned channel closes when execution terminates; callers must drain.
    ExecuteStream(ctx context.Context, req ExecuteRequest) (<-chan Event, error)
}

func New(opts Options) (Service, error)

// Public input/output types — stable contract.
type Options struct {
    ConfigPath string         // optional override; default $XDG_CONFIG_HOME/ddx-agent
    Logger     io.Writer      // optional; service writes its own session log
    // Nothing else. No injected providers, tools, compactors.
}

type ProviderInfo struct {
    Name       string
    Type       string  // "openai-compat", "anthropic", etc.
    BaseURL    string
    Status     string  // "connected", "unreachable", "error: <msg>"
    ModelCount int
}

type ModelInfo struct {
    ID                 string
    ContextLength      int     // resolved (provider API > catalog > default)
    Capabilities       []string  // "tool_use", "vision", "json_mode" if known
}

type ExecuteRequest struct {
    Prompt        string                  // required
    SystemPrompt  string                  // optional override
    Provider      string                  // optional; default from config
    Model         string                  // optional; resolved via routing if empty
    ModelRef      string                  // optional alternative: catalog ref ("cheap", "smart", etc.)
    Tools         []string                // names, e.g. ["bash","edit","read","write","grep","glob","ls"]
    WorkDir       string                  // required when tools is non-empty
    MaxIterations int                     // default 100
    Timeout       time.Duration           // wall-clock cap
    PromptPreset  string                  // optional: well-known preset (replaces prompt synthesis)
    ContextFiles  []string                // optional: file paths whose contents are appended
    Metadata      map[string]string       // free-form; echoed back in response
}

type ExecuteResponse struct {
    Status      Status                    // success | failed | cancelled | timed_out
    Output      string                    // final assistant message
    ToolCalls   []ToolCall                // tools invoked, in order
    Usage       TokenUsage
    SessionID   string
    Duration    time.Duration
    Error       string                    // when Status != success
    RoutingInfo RoutingInfo               // resolved provider/model + reason
}

// Event union for ExecuteStream (interface or tagged struct — TBD upstream).
type Event interface{ Kind() string }
// Concrete event types: TextDelta, ToolCallStart, ToolCallEnd, CompactionEvent (rolled up — single event per actual compaction, not pre/post probes), Final.
```

## Principle: uniform harness invocation shape, embedded transport for ddx-agent

The existing harnesses — `claude`, `codex`, `opencode`, `gemini`, `pi` — share a uniform invocation contract: prompt + model + workdir + permission level + effort → result + JSONL event stream. They happen to run via subprocess because they're third-party binaries DDx didn't write.

**ddx-agent should expose the same invocation shape, but stay in-process.** No subprocess required — the agent library is Go and DDx is Go. The reason to make the shape uniform isn't transport; it's so DDx can have a single `Harness` interface in its own code with multiple implementations:

```go
// DDx-side
type Harness interface {
    Run(ctx context.Context, req RunRequest) (*RunResult, <-chan Event, error)
}

// Implementations:
// - subprocessHarness  — wraps exec.Command for claude/codex/opencode/gemini/pi
// - agentHarness       — calls agentlib.Service.ExecuteStream in-process
```

`RunRequest` carries: `Prompt`, `Model`, `WorkDir`, `Permissions`, `Effort`, `Timeout`, `Metadata`. Nothing else. Tools are not in `RunRequest` — claude doesn't take a tools flag, codex doesn't, ddx-agent won't.

## Tools do not cross the boundary

Tools are entirely internal to whichever harness is running. `claude` does not let DDx pick its tools; `codex` does not; `ddx-agent` won't either. Permission level is the only knob; the harness's own catalog provides the tools at that level. **`Tools []string` deletes from the proposed `ExecuteRequest`.** `findTool` and any future tool extensions belong inside ddx-agent's tool catalog; DDx never wraps or aliases tools.

## Extras: catalog / discovery / route inspection

ddx-agent has three capabilities the cloud-vendor harnesses don't: model catalog, provider discovery, route inspection. These don't fit the per-execution `Run` shape — they're queries against the agent's runtime knowledge. Expose them as additional methods on the same `Service`:

```go
package agentlib

// DdxAgent is the entire public surface. Six methods mirroring CLI verbs, with
// type-safety and zero subprocess overhead. Anything else stays internal.
type DdxAgent interface {
    // Execute runs an agent task in-process and streams events; the returned
    // channel closes when execution terminates. Non-streaming callers can use
    // a convenience wrapper that drains the channel into a single response.
    Execute(ctx context.Context, req ExecuteRequest) (<-chan Event, error)

    // ListProviders returns all known providers and their reachability status.
    ListProviders(ctx context.Context) ([]ProviderInfo, error)

    // ListModels returns models for one provider (always probes live; falls
    // back to catalog when the provider's /v1/models is unhelpful).
    ListModels(ctx context.Context, provider string) ([]ModelInfo, error)

    // HealthCheck probes one provider and returns nil if reachable.
    HealthCheck(ctx context.Context, provider string) error

    // ExplainRoute returns the candidate plan for a (provider?, model?, ref?,
    // effort?, permissions?) tuple — what would Execute pick, why, and what
    // are the fallbacks.
    ExplainRoute(ctx context.Context, req RouteRequest) (*RouteExplanation, error)

    // RouteStatus returns global routing state: cooldowns, recent failures,
    // observation-store latency by provider.
    RouteStatus(ctx context.Context) (*RouteStatusReport, error)
}

func New(opts Options) (DdxAgent, error)
```

Six methods. `Execute` is the primary verb (mirrors the harness invocation); the other five are the "extras" only ddx-agent has — model catalog, discovery, route inspection.

DDx's `agentHarness` implementation of the `Harness` interface delegates `Run` → `Service.ExecuteStream`. DDx commands that need the extras (`ddx agent route-status`, `ddx agent providers`, `ddx agent models`, `ddx agent check`) call the corresponding `Service` method directly — they don't go through the harness abstraction since they're not invocations.

## Internal-package enforcement

## Internal-package enforcement

**Everything else literally moves under `internal/`:** Go's compiler blocks any external package from importing under `internal/`. This is the enforcement mechanism — not deprecation warnings, not lint rules, not API discipline. **Compiler-enforced.** Anyone wanting to break the boundary has to fork or vendor the module ("serious surgery").

Concrete physical moves in the agent repo:

| from | to |
|---|---|
| `compaction/` | `internal/compaction/` |
| `prompt/` | `internal/prompt/` (keep `prompt/types` only if the public API truly needs cross-module structs, which it should not) |
| `tool/` | `internal/tool/` (tool function implementations stay internal; callers select tools by name in `ExecuteRequest.Tools`) |
| `session/` | `internal/session/` (Service writes its own log; expose path via `ExecuteResponse.SessionLogPath`) |
| `observations/` | `internal/observations/` (callers consume events via `ExecuteStream`, never raw observations) |
| `modelcatalog/` | `internal/modelcatalog/` (Service exposes resolved info via `ListModels`) |
| `provider/openai/` | `internal/provider/openai/` |
| `provider/anthropic/` | `internal/provider/anthropic/` |
| `provider/virtual/` | `internal/provider/virtual/` (test-only; surface a `NewWithFakeProvider` test helper at root if needed) |
| `config/` | retain *only* the small subset that callers must set (`Options`); rest moves internal |

After the move, the agent module's external API is **only** what's exported from the root `agent` package — i.e., the `Service` interface, the `Options` constructor input, and the public input/output struct types listed above. Imports from `internal/` outside the module fail to compile.

Callers get JSON-shaped data and a stable interface.

## Migration shape

### Upstream (agent repo)

1. **Spec the interface.** Get HELIX maintainers' sign-off on shape (next consumer after DDx).
2. **Implement `Service` over the existing internals.** Wire the existing agent loop behind the new interface. Existing `cmd/ddx-agent/main.go` consumes the new `Service` (proves the interface is sufficient).
3. **Tag `v0.4.0-pre`** with the new `Service` interface available alongside legacy exports. Mark every legacy public export `Deprecated:` so consumers see warnings but still compile. Do NOT yet move packages under `internal/` — DDx needs a window with both APIs available to migrate.
4. **DDx migrates onto `Service`** (steps 6-12 below). After DDx merges, no external consumer uses any legacy exports.
5. **Tag `v0.4.0`** as the migration boundary: legacy exports still public but stamped Deprecated.
6. **Move packages under `internal/`** per the table above. This is the enforcement step. External imports of `compaction`, `tool`, `session`, etc. now fail at compile. Any caller still depending on them is surfaced loudly. Tag `v0.5.0`.
7. **Cleanup pass.** Audit the still-public surface and remove anything that wasn't intentionally part of the new API. Anything that snuck through is a sign the `Service` is incomplete; either add a method or close the gap.

### DDx (this repo)

6. **Bump go.mod to v0.4.x.** Both interfaces coexist briefly.
7. **agent_runner.go migration.** Replace the `compactor := compaction.NewCompactor(...)` + `agentlib.Request{Provider: ..., Tools: ..., Compactor: ...}` + `agentlib.Run(...)` block with `service.Execute(ctx, req)` or `service.ExecuteStream(ctx, req)`. ~50 of the 113 root-package refs collapse here.
8. **Providers/models migration.** Replace direct `provider/openai`, `provider/anthropic` imports with `service.ListProviders` / `service.ListModels`.
9. **Tools migration.** Move `findTool` upstream first (a small dependency on the agent epic — file as a precursor bead). Then replace `tool.BashTool` etc. with `Tools: []string{"bash","edit",...,"find"}` in `ExecuteRequest`. Drop the entire `agent/tool` import.
10. **Prompt migration.** Replace `prompt.NewMetaPromptInjectorWithPaths`, `prompt.NewFromPreset` with `ExecuteRequest.PromptPreset` + `ContextFiles`. Drop `prompt` import.
11. **Sessions/observations migration.** Drop `session.NewLogger` (Service writes its own log; expose path via response). Drop `observations` (subscribe to `ExecuteStream` events).
12. **Compaction migration.** Already covered by `agent-6f8caa00`/`ddx-76df1a46`, subsumed: delete `embeddedCompactionConfig` and the entire `compaction` import.
13. **Virtual provider for tests.** Today DDx imports `virtual.New`/`virtual.Config`/`virtual.InlineResponse` directly. New shape: `Options{ConfigPath: "/tmp/test-config.yaml"}` where the config selects the virtual provider; or expose `agentlib.NewWithFakeProvider(responses []FakeResponse)` as a test-only helper.
14. **Bump go.mod to v0.5.0.** Confirm DDx compiles with no `agent/{prompt,tool,session,compaction,observations,modelcatalog,provider/...}` imports — only `agent` (the Service) and possibly `agent/types` for the public structs.
15. **Cleanup.** Delete dead code paths in DDx that supported the old DI-heavy API (CompactorOverride, etc.).

## Call-site mapping (every DDx file that imports ddx-agent)

Only **10 non-test files** import `github.com/DocumentDrivenDX/agent`. The 113 symbol uses are concentrated in `agent_runner.go`. Mapping each file to its target Service method or fate:

### cmd/ (operator-facing — straight Service-method maps)

| File | Today | After |
|---|---|---|
| `cli/cmd/agent_check.go` | `agentconfig.Load(workingDir)` to enumerate providers + probe them | `service.ListProviders(ctx)` + `service.HealthCheck(ctx, provider)` |
| `cli/cmd/agent_providers.go` | `agentconfig.Load(workingDir)`, render provider list | `service.ListProviders(ctx)` |
| `cli/cmd/agent_models.go` | `agentconfig.Load`, `modelcatalog.Default()`, `cat.AllConcreteModels(...)`, `oai.RankModels(pr.Models, knownModels, pc.ModelPattern)` | `service.ListModels(ctx, provider)` (live discovery + catalog merge happen inside the service) |
| `cli/cmd/agent_route_status.go` | `agentconfig.Load`, `agentconfig.ModelRouteCandidateConfig`, `observations.LoadStore`, `observations.Key{...}` to render cooldown + speed table | `service.RouteStatus(ctx)` returns the full report shape; rendering stays in DDx |

### internal/agent/ (runtime — Execute or fold into Service internals)

| File | Today | After |
|---|---|---|
| `cli/internal/agent/agent_runner.go` | The big one. Constructs providers (`oai.New`, `anthropic.New`, `virtual.New`), constructs compactor (`compaction.NewCompactor(embeddedCompactionConfig(...))`), registers tools (`tool.BashTool`, `tool.EditTool`, …, the `findTool` wrapper), builds prompt (`prompt.NewMetaPromptInjectorWithPaths`, `prompt.NewFromPreset`), opens session log (`session.NewLogger`), drives the loop via `agentlib.Run`, listens to `agentlib.Event` (compaction-end inspection for stall/breaker logic at lines 200-318) | Becomes the `agentHarness` impl of DDx's `Harness` interface. Body shrinks to: build `ExecuteRequest{Prompt, Model, WorkDir, Permissions, Effort, Timeout, StallPolicy}`, call `service.Execute(ctx, req)`, drain events. **All of:** provider construction, compactor wiring, tool registration, prompt building, session opening — moves into agentlib internals. The stall/circuit-breaker logic at 200-318 needs an upstream home — see "open structural decisions" below. |
| `cli/internal/agent/runner.go` | `Runner` struct exposes `AgentProvider interface{}` (test injection seam — line 36) and `CompactorOverride func(...)` (test compactor seam — line 53) | Drop `CompactorOverride` (compaction is internal). Replace `AgentProvider interface{}` with `Options.FakeProvider` test seam at the agentlib root — first-class part of the design, not hand-waved. |
| `cli/internal/agent/provider_deadline.go` | Wraps `agentlib.Provider` with per-request and idle-timeout timeouts; implements `StreamingProvider` too | Deletes. Move semantics into agentlib (it already owns provider construction). Caller passes `ExecuteRequest.Timeout` and optionally `ExecuteRequest.IdleTimeout`. |
| `cli/internal/agent/discovery.go` | `agentconfig.Load(workDir)`, exposes `Config agentconfig.ProviderConfig` for live `/v1/models` probing | Service owns discovery (`ListModels` is live-by-default per the principle). DDx-side discovery wrapper deletes; callers use `service.ListModels`. |
| `cli/internal/agent/providerstatus/probe.go` | `Probe(ctx, agentconfig.ProviderConfig)` calls `oai.DiscoverModels(...)` | Replaced by `service.HealthCheck(ctx, provider)`. The `providerstatus` package can stay as a DDx-side type for the result, sourced from the service. |

### internal/server/ (graphql resolver)

| File | Today | After |
|---|---|---|
| `cli/internal/server/graphql/resolver_providers.go` | `agentconfig.Load(r.WorkingDir)` to surface provider list over GraphQL | `service.ListProviders(ctx)` (server holds a `DdxAgent` instance the same as CLI commands do) |

### Open structural decisions (call out before locking the spec)

1. **Where do the stall + compaction-stuck circuit breakers live?** Today `agent_runner.go:200-318` inspects private JSON fields on compaction-end events to enforce DDx-defined limits (no read-only-tool-only iterations, no consecutive no-op compactions). Two options:
   - **Move enforcement upstream.** `ExecuteRequest.StallPolicy{ReadOnlyTools int, NoopCompactions int}` and the agent enforces, ending execution with `Status: stalled`. DDx no longer needs the event-inspection code.
   - **Keep enforcement in DDx.** Then `Event` payloads must carry the structured `compaction-end{success bool, no_compaction bool}` shape, which slightly leaks compaction internals.
   - Recommendation: **upstream**. Stall is an execution concern, not a DDx concern; HELIX/Dun would want the same limits.

2. **`findTool` rename:** rename upstream `glob` → `find`. Same handler. No back-compat alias (pre-release). DDx-side `findTool` wrapper deletes.

3. **Test injection seam shape.** `Options.FakeProvider FakeProvider` where `FakeProvider` is a public test-only type (struct with prerecorded responses, NOT a Provider interface). Lives in `agentlib` root; nothing about real Provider crosses the boundary.

4. **`RoutingReport` / `RoutingReporter`.** Today DDx receives `agentlib.RoutingReport` from a provider that implements the optional `RoutingReporter` interface. Its data (resolved provider, fallback chain) belongs in `ExecuteResponse.RoutingInfo` — already noted; just make sure the field exists.

5. **`agentlib.Result.Messages` (history continuation).** Some DDx flows want the message history back. Decide: include in `ExecuteResponse` (cheap), or expose only on-disk session log path? Probably both — `Messages` for in-process consumers, `SessionLogPath` for forensic tools.

## Existing landscape (archeology, 2026-04-18)

### Specs that encode the current boundary
- **PLAN-2026-04-08-AGENT-ROUTING-AND-CATALOG-RESOLUTION** [superseded] — original two-layer design.
- **FEAT-006 — Agent Service** — DDx owns harness orchestration + cross-harness routing; embedded agent owns provider/backend selection.
- **SD-015 — Resolution path** — 5-mode precedence (harness override → explicit model → profile → default → provider targeting), candidate-ranking rules, and fuzzy match with shortest-suffix tiebreak.
- **SD-023 — Routing visibility** — explicit DDx/agent boundary; DDx accesses agent state via Go package APIs, not shellout. Eight visibility beads block on this boundary.
- **agent-side: plan-2026-04-10-model-first-routing.md, SD-005 (provider config), SD-002 (standalone CLI)** — model-first routing, ModelRouteCandidateConfig, RoutingConfig (weight tuning), CandidateScorer interface for DDx quota overlay.

### Code that implements (or fails to implement) the design

DDx side, all in `cli/internal/agent/`:
| File | What it does | Key smell |
|---|---|---|
| legacy DDx routing planner | Enumerates harnesses, builds CandidatePlan per harness | Parallel impl with agent's routing_smart.go |
| legacy DDx candidate evaluator | Per-harness scoring, catalog resolution per harness surface | Capability gating per provider+model is missing |
| legacy DDx route request normalization | Maps CLI flags + config → RouteRequest | **`--provider` silently dropped** (ddx-8610020e) |
| legacy DDx candidate scorer/selector | Rank + select | Profile policy hardcoded; no per-model-route override |
| legacy DDx live route probe | Live probe + build | Probes every harness/provider, no cache |
| `discovery.go` `FuzzyMatchModel` | Cross-provider model fuzzy match | **No case norm, no vendor-prefix strip** (ddx-0486e601) |
| `tier_escalation.go:57` `AdaptiveMinTier` | Trailing success rate → tier promotion | Doesn't talk to agent's per-route failover; they don't know about each other |
| `routing_metrics.go`, `routing_signals*.go` | Routing observation surfaces | DDx-only; agent has its own observation store |

Agent side (`/Users/erik/Projects/agent/`):
| File | What it does |
|---|---|
| `cmd/ddx-agent/routing_provider.go:38` `newRouteProvider` | Per-route provider wrapper with candidate failover |
| `cmd/ddx-agent/routing_smart.go:28` smartRouteHistory/Candidate/Plan | Runtime scoring: reliability, performance, cost, capability |
| `cmd/ddx-agent/routing_scorer.go` `CandidateScorer` interface | Hook point so DDx can overlay quota/cost (Option-A pattern) |
| `config/config.go:69` `RoutingConfig` | Weights: reliability 0.4, performance 0.3, load 0.15, cost 0.1, capability 0.05 |
| `config/config.go:118` `ModelRouteConfig` + `:110` `ModelRouteCandidateConfig` | Strategy + candidates per model |

### Open beads (the "badly implemented" parts)

| ID | P | What |
|---|---|---|
| ddx-2d974641 | P1 | **Epic** — autorouting "use qwen3.6 on vidar" should Just Work end-to-end |
| ddx-8610020e | P1 (in_progress) | RouteRequest missing Provider field; `--provider` dropped at normalization |
| ddx-0486e601 | P1 | Fuzzy match: case + canonical-prefix normalization (qwen/qwen3.6 ≠ Qwen3.6-35B-A3B-4bit) |
| ddx-3c5ba7cc | P1 | Tier escalation must respect `--provider` affinity |
| ddx-2f5a2284 | P2 | ModelRef/Profile must consult discovery + apply `--provider` soft preference |
| ddx-4817edfd subtree | P2-P4 | Per-provider+model gating: tool-support, effort/permissions, context-window, structured-output |
| ddx-0216b966 | P3 | Discovery fuzzy tiebreak: prefer latest version over shortest suffix |

Closed but relevant: `agent-f5a6b7c8` (CandidateScorer interface), `agent-f4065aa8` (model-first routing), `agent-c232a6da` (routing attribution tests).

### Six concrete "badly implemented" smells the archeology surfaced

1. **Two parallel routing engines** (DDx routing.go vs agent routing_smart.go), neither calls the other.
2. **Provider affinity bypass** — `--provider` dropped at normalization; `RunOptions.Provider` bypasses routing entirely via `resolveEmbeddedAgentProvider`.
3. **Fuzzy matching fragmentation** — DDx pools providers without canonical form; agent probes per-provider independently. No unified pool.
4. **Profile semantics split** — DDx escalates tiers based on success rate; agent does provider failover within a route. Neither informs the other.
5. **Capability gating asymmetry** — DDx checks per-harness; agent doesn't check per-provider+model.
6. **Health/quota fragmentation** — DDx HarnessState (sync probes) vs agent route-health files (async tracking). No shared probe abstraction.

### Archeology's bias readout

The existing spec landscape **biases toward Option B** (finish the existing two-layer design rather than consolidate). All open beads propose DDx-side improvements. CandidateScorer was designed exactly for the boundary pattern. No spec or bead currently proposes "move harnesses down."

## Decision: Option C — consolidate harnesses down into ddx-agent

### What ddx-agent becomes

Two roles, one module:

1. **A direct first-class agent.** Native in-process runtime over the model providers (bragi, vidar, openrouter, anthropic). Designed to be the high-performance choice for batch noninteractive prompts.

2. **A wrapper around other agents.** Subprocess harness layer for claude, codex, opencode, pi, gemini — used when their interactive features, vendor billing, or specific capabilities matter, OR when comparison/fallback routing wants them in the candidate pool.

**The product:** ddx-agent is the one stop shop for optimally routed one-shot noninteractive agentic prompts. DDx is one consumer (the bead-driven workflow); HELIX, Dun, and standalone CLI users are others.

### Why C is right (overriding the existing spec landscape)

1. **Testability of the service boundary.** Under C, every harness — native and subprocess — is reachable through the same `DdxAgent` Service interface, with the same input/output contract. The boundary is **one surface to test**. Under B, the boundary is N+1 surfaces (one Service interface plus N harness binaries DDx invokes directly), and parity testing requires bridging two modules. The "comparison suite" argument from earlier rounds was a weaker version of this point — testability is the actual reason.

2. **Two-role coherence.** ddx-agent's value prop ("optimally routed one-shot noninteractive prompts") only makes sense if it owns the routing **across** harnesses, not just within in-process providers. A consumer that wants "give me the best harness/provider/model for this prompt under cost constraint X" needs one entrypoint that knows about all options. Under B, that consumer has to glue DDx's harness routing to ddx-agent's provider routing themselves.

3. **The six "badly implemented" smells are structural duplication.** Codex's review (round 3) was right that under B you can patch each side. But "patch each side" is exactly how the duplication arose — and exactly what produced the smells. C eliminates the possibility of recurrence by deleting one of the two homes.

4. **Existing spec inertia is the entanglement we're escaping, not the constraint we honor.** Per the user's direction: "we shouldn't be separate projects if we can't maintain documented interfaces legibly." The 7+ governing docs encoding B are themselves the failure mode. Replacing them with a single agent-owned contract IS the spec work, not a price to pay.

### Spec cleanup: delete with prejudice, replace with one contract doc

The cross-repo entanglement is the failure mode. **Do not supersede. Do not deprecate. Delete the files and clean up every reference.** Encapsulation has eroded; the boundary docs that documented and reinforced the erosion go away with the erosion itself.

#### Files to DELETE on the agent side (`/Users/erik/Projects/agent/`)

- `docs/helix/02-design/contracts/CONTRACT-002-ddx-harness-interface.md` — the CandidateScorer/DDx-coupling contract is the entanglement. Deleted; replaced by the new contract.

#### Files to DELETE on the DDx side (`/Users/erik/Projects/ddx/`)

- `docs/helix/02-design/solution-designs/SD-015-agent-routing-and-catalog-resolution.md` — routing is not a DDx concern.
- `docs/helix/02-design/solution-designs/SD-015-resolution-path-trace.md` — same.
- `docs/helix/02-design/solution-designs/SD-023-agent-routing-visibility.md` — visibility is contract calls, no design needed.
- `docs/helix/02-design/plan-2026-04-08-agent-routing-and-catalog-resolution.md` — superseded by this plan; delete, don't keep the supersession marker.

#### Files to GUT on the DDx side

- `docs/helix/01-frame/features/FEAT-006-agent-service.md` — rewrite to one page: "DDx invokes LLMs via the ddx-agent contract. DDx-side responsibilities: bead-driven invocation, execute-bead orchestration, evidence/session capture. End." No harness, routing, or provider language survives.

#### Files to TRIM on the agent side (drop boundary claims; describe internals in agent's own language)

- `docs/helix/02-design/architecture.md` — internal architecture only; one line pointing at the contract for the external surface.
- `docs/helix/02-design/plan-2026-04-10-model-first-routing.md` — internal routing impl; no claims about who calls.
- `docs/helix/02-design/solution-designs/SD-002-standalone-cli.md` — describes the standalone CLI as one consumer of the contract.
- `docs/helix/02-design/solution-designs/SD-005-provider-config.md` — internal config schema only.
- `docs/helix/01-frame/features/FEAT-004-model-routing.md` — value prop and internal capability; no DDx-side claims.
- `docs/helix/01-frame/prd.md` — drop any cross-repo language.

#### One new doc on the agent side

`docs/helix/02-design/contracts/CONTRACT-001-ddx-agent-service.md`:
- The `DdxAgent` interface (6 methods).
- The input/output struct types (ExecuteRequest, ExecuteResponse, Event, ProviderInfo, ModelInfo, HarnessInfo, RouteRequest, RouteDecision).
- The JSON event schema for `ExecuteStream`.
- "This is the entire external surface. Anything not here is internal. Internal subject to change without notice. Consumers ask for changes via PR against this doc."

#### Reference cleanup (do this aggressively)

After deletes/trims, grep both repos for references to the deleted IDs and fix or remove every hit:

```bash
# In both repos:
grep -rn "SD-015\|SD-023\|CONTRACT-002\|plan-2026-04-08-agent-routing" --include="*.md" --include="*.go"
```

Any remaining reference is a re-entanglement risk. Remove with prejudice. Code-side imports of `cli/internal/agent` types from outside DDx (if any — see HELIX/Dun audit below) get the same treatment.

#### How DDx asks for changes

When DDx needs new contract behavior, it files an issue/bead against ddx-agent referencing the use case. ddx-agent maintainers decide. DDx does not edit ddx-agent's internal specs and does not maintain a parallel description of agent's behavior. **One contract; one direction of dependency.**

#### Spec work for this epic, in scope

1. Write CONTRACT-001 on the agent side.
2. DELETE the four DDx docs (SD-015, SD-015-resolution-path-trace, SD-023, plan-2026-04-08).
3. DELETE CONTRACT-002 on agent side.
4. GUT DDx FEAT-006 to the one-page reference.
5. TRIM agent's boundary-claiming docs (architecture, plan-2026-04-10, SD-002, SD-005, FEAT-004, PRD).
6. Reference-cleanup grep + remove in both repos.

### What moves down into ddx-agent

| DDx file | Fate under C |
|---|---|
| `cli/internal/agent/registry.go` | **Moves to ddx-agent** — harness catalog (claude, codex, opencode, pi, gemini, agent-native, virtual, script) |
| `cli/internal/agent/routing.go` | **Moves to ddx-agent**, merges with `cmd/ddx-agent/routing_smart.go` + `routing_provider.go` into one routing engine |
| `cli/internal/agent/discovery.go` | **Moves to ddx-agent** (already partially there) |
| `cli/internal/agent/tier_escalation.go` | **Moves to ddx-agent** |
| `cli/internal/agent/routing_metrics.go`, `routing_signals*.go` | **Moves to ddx-agent**; DDx keeps a thin observation hook for execute-bead evidence capture |
| `cli/internal/agent/agent_runner.go`, `runner.go` | **Deletes**; replaced by Service.Execute calls |
| `cli/internal/agent/provider_deadline.go` | **Moves to ddx-agent** (deadlines are internal to execution) |
| `cli/internal/agent/types.go` (RouteRequest, CandidatePlan, HarnessState, Harness) | **Moves to ddx-agent** as internal types |
| `cli/internal/agent/compare.go`, `benchmark.go`, `quorum.go`, `condense.go` | **Move to ddx-agent** — these ARE the comparison suite the user wants in one module |
| `cli/internal/agent/providerstatus/probe.go` | **Moves to ddx-agent** |
| `cli/internal/agent/claude_stream.go`, `claude_quota_cache.go` | **Move to ddx-agent** as part of the claude harness implementation |
| `cli/internal/agent/jsonl.go`, `format.go`, `session_log_format.go`, `session_log_tailer.go` | **Mostly move** to ddx-agent (event/format primitives); DDx keeps any UI rendering |
| `cli/internal/agent/state.go`, `executions_mirror.go`, `executor.go`, `script.go` | **Stay** in DDx — these are execute-bead orchestration (DDx-specific concern) |
| `cli/internal/agent/grade.go`, `pricing.go`, `models.go`, `catalog.go`, `model_catalog_yaml.go`, `agent_catalog_shared.go`, `worktree_skills.go` | Mixed — likely most move; some DDx-specific glue stays |
| `cli/cmd/agent_*.go` | **Stay** in DDx as commands, but become thin Service.* callers |
| `cli/internal/server/graphql/resolver_providers.go` | **Stays**, becomes a Service.ListProviders caller |

### What the new Service interface looks like under C

Same 5-method `DdxAgent` shape from the v2 plan, **but the methods now own the entire LLM-execution-and-routing surface** (including subprocess harness invocation):

```go
type DdxAgent interface {
    // Execute runs an agent task. Caller specifies harness + (provider?, model?)
    // OR a profile + criteria; service routes if not fully specified.
    // Streams events; final event carries timing/cost/routing info.
    Execute(ctx, req ExecuteRequest) (<-chan Event, error)

    // ListHarnesses returns all harness implementations (claude, codex, ...,
    // agent-native, virtual). Each carries CostClass, IsLocal, capability metadata.
    ListHarnesses(ctx) ([]HarnessInfo, error)

    // ListProviders returns providers known to the native-agent harness.
    ListProviders(ctx) ([]ProviderInfo, error)

    // ListModels returns models available — across all harnesses (caller filters)
    // or for a specific harness/provider.
    ListModels(ctx, filter ModelFilter) ([]ModelInfo, error)

    // HealthCheck probes a harness or provider.
    HealthCheck(ctx, target HealthTarget) error

    // ResolveRoute returns the routing decision for an under-specified request,
    // without executing. (Replaces both DDx's legacy planner and agent's
    // routing_smart.go scoring with one entrypoint.)
    ResolveRoute(ctx, req RouteRequest) (*RouteDecision, error)
}
```

That's 6 methods. Adds `ListHarnesses` (because under C the agent owns the harness catalog) and renames `ExplainRoute` → `ResolveRoute` (now does both inspection AND the actual routing, since it's the single source of truth).

### How each "badly implemented" smell gets fixed by consolidation

| Smell | Fix under C |
|---|---|
| Parallel routing engines | One routing engine; the other deletes |
| `--provider` bypass | One RouteRequest type with Provider field; one normalization path; bypass impossible |
| Fuzzy matching fragmentation | One canonical-form helper; all candidates pool through it |
| Profile semantics split | Tier escalation and provider failover live next to each other; one event loop sees both |
| Capability gating asymmetry | One gating predicate, applied per-candidate (whether candidate is a harness or a (harness, provider, model) tuple) |
| Health/quota fragmentation | One probe abstraction, one observation store, one cooldown store |

All six fixed structurally, not by patching each side.



Two-layer routing — DDx routing across harnesses (claude/codex/opencode/agent/pi), agent routing across providers (bragi/vidar/openrouter) — is **a long-standing requirement, badly implemented today**. Before locking the interface, do an archeology pass to find every existing spec, code piece, bead, and design note that touches it. The "repair" must be complete, not greenfield.

Three architectural options on the table:

**A. Routing stays in ddx-agent, exposed via a candidate-ranking API.** DDx calls in to score candidates for harness routing. Two-layer routing where the inner layer asks the agent.

**B. Lift all routing to DDx.** ddx-agent becomes a mechanism (list/probe/resolve/execute). DDx ranks `(harness, provider?, model)` tuples uniformly. Single-layer routing in DDx.

**C. Move harnesses DOWN into ddx-agent.** All "LLM routing" lives in one module — including subprocess wrappers for claude/codex/opencode/pi/gemini. DDx becomes a thin client. ddx-agent owns the whole LLM-execution-and-routing surface.

**Decisive C-favoring argument: comparison suite.** A stated project goal is that ddx-agent's native harness should *outperform* the subprocess wrappers (claude/codex/opencode) on batch noninteractive tasks. Proving that requires a comparison suite that runs identical prompts through every harness implementation and measures latency, cost, success rate, output quality. Today that suite is split: harness invocation lives in DDx (`cli/internal/agent/compare.go`, `cli/internal/agent/benchmark.go`), routing lives in DDx, but the native agent runtime lives in ddx-agent. Under C, the comparison suite collapses into one module that can A/B test (`harness=claude` subprocess) vs (`harness=agent-native`, `provider=bragi`, `model=qwen3.6`) with the same input/output schema and metrics collection. Under A or B, the suite still has to bridge two modules and measurement parity is harder to maintain.

| | A | B | C |
|---|---|---|---|
| Algorithm location | ddx-agent (exposed) | DDx | ddx-agent (internal) |
| Harness execution | DDx | DDx | ddx-agent |
| API surface | 7 methods (+candidate-ranking API) | 5 methods | 5-6 methods |
| Standalone CLI gets smart routing | Yes (providers only) | No | Yes (all harnesses) |
| Algorithm impls | 1 (exposed) | 1 (DDx only) | 1 (internal) |
| Conceptual model | Two layers | One layer in DDx | One layer in ddx-agent |
| ddx-agent name fit | OK | Best (it's just "agent runtime") | Worst (it's "LLM router + runtime") |

**Decision deferred** to after the archeology. Pick the option best supported by what's already implemented and specced — the existing pieces are the constraint, not the design.

## Technical refinements (incorporated from review rounds 1-3)

- **Test seam.** `Options.FakeProvider` is a public test-only struct with prerecorded responses. Three patterns supported: static script (sequence of responses), dynamic per-call fake (function callback), error injection (per-call status override). Spec'd in CONTRACT-001 with worked examples; not hand-waved.
- **Migration windows.** Two windows: (a) `agent v0.4.0-pre` ships the new contract alongside legacy `agentlib.Run` API; (b) DDx migrates against `v0.4.0-pre` with a `cli/internal/agent/legacy/` adapter layer letting old call sites keep working call-site by call-site. Then `agent v0.5.0` deletes legacy. **No flag-day.**
- **`ListHarnesses` info density.** Returns `HarnessInfo{Name, Available, Path, Error, IsLocal, IsSubscription, ExactPinSupport, SupportedPermissions[], SupportedEfforts[], CostClass, Quota}`. Quota is the live field that changes between calls; everything else is static.
- **`HealthCheck` polymorphic target.** `HealthCheck(ctx, Target)` where `Target` is `{Type: "harness"|"provider", Name: string}`. Triggers a fresh probe and updates internal state.
- **Event schema.** `ExecuteStream` event types form a closed union: `text_delta`, `tool_call`, `tool_result`, `compaction`, `routing_decision`, `final`. Each has a documented JSON shape that both subprocess and in-process backends emit identically. Defined in CONTRACT-001.
- **OS-level cancellation.** `Service` guarantees that `ctx.Done()` triggers cleanup of any subprocess (PTY teardown, orphan reap). Tests prove it.
- **Stall enforcement.** `ExecuteRequest.StallPolicy{ReadOnlyTools int, NoopCompactions int}` — the agent enforces, ends execution with `Status: stalled`. DDx's circuit-breaker code at `agent_runner.go:200-318` deletes.
- **HELIX/Dun audit.** Before lock-in, grep both repos for `cli/internal/agent` imports. Any non-DDx consumer is added as a named migration target with their own bead.

## Concrete contract draft (CONTRACT-001-ddx-agent-service.md)

This section is the actual contract content, not an outline. Filed verbatim into `/Users/erik/Projects/agent/docs/helix/02-design/contracts/CONTRACT-001-ddx-agent-service.md` as the first bead.

### Interface

```go
package agentlib

import (
    "context"
    "time"
)

// DdxAgent is the entire public Go surface of the ddx-agent module.
// Anything not reachable through this interface (or the public types
// referenced by its methods) is internal and may change without notice.
type DdxAgent interface {
    // Execute runs an agent task in-process; emits Events on the channel until
    // the task terminates (channel closes). Final event is type=final with
    // status, usage, cost, session log path, optional message history, and
    // routing_actual (resolved fallback chain that fired).
    Execute(ctx context.Context, req ExecuteRequest) (<-chan Event, error)

    // TailSessionLog streams events from a previously-started or in-progress
    // session by ID. Used by DDx workers/UI to subscribe to a run started
    // elsewhere (e.g., server-managed worker that DDx CLI wants to follow).
    TailSessionLog(ctx context.Context, sessionID string) (<-chan Event, error)

    ListHarnesses(ctx context.Context) ([]HarnessInfo, error)
    ListProviders(ctx context.Context) ([]ProviderInfo, error)
    ListModels(ctx context.Context, filter ModelFilter) ([]ModelInfo, error)
    HealthCheck(ctx context.Context, target HealthTarget) error

    // ResolveRoute resolves a single under-specified request to (Harness,
    // Provider, Model). The returned RouteDecision can be passed back to
    // Execute via ExecuteRequest.PreResolved to skip re-resolution (used by
    // dry-run-then-execute flows like `ddx agent route-status` followed by
    // an explicit invocation).
    ResolveRoute(ctx context.Context, req RouteRequest) (*RouteDecision, error)

    // RouteStatus returns global routing state across all routes: cooldowns,
    // recent decisions, observation-derived per-(provider,model) latency
    // summary. Used by `ddx agent route-status` to render the operator
    // dashboard. NOT a per-request resolution — that's ResolveRoute.
    RouteStatus(ctx context.Context) (*RouteStatusReport, error)
}

// New constructs a DdxAgent. Options is intentionally minimal.
func New(opts Options) (DdxAgent, error)
```

**Seven methods.** Adds `TailSessionLog` (worker-tailing surface that DDx's `workers.go:1203` and `session_log_tailer.go` consume today) and `RouteStatus` (the operator dashboard surface that `agent_route_status.go` needs — distinct from per-request `ResolveRoute`).

### Types

```go
type Options struct {
    ConfigPath        string             // optional override; default $XDG_CONFIG_HOME/ddx-agent/config.yaml
    Logger            io.Writer          // optional; agent writes structured session logs internally regardless

    // Test-only injection seams. Each of these MUST be nil in production
    // builds — enforced by a build tag (`//go:build testseam`). Forming an
    // Options with any of these set in a non-test build is a compile error.
    // Four seams exist because DDx today injects at four different layers:
    FakeProvider           *FakeProvider           // pre-recorded chat responses (replaces virtual.New)
    PromptAssertionHook    PromptAssertionHook     // observe constructed system+user prompts (used in agent_runner_test.go)
    CompactionAssertionHook CompactionAssertionHook // observe compaction inputs/outputs (used in compaction tests)
    ToolWiringHook         ToolWiringHook          // observe tool registration order/names (used in tool tests)
}

// FakeProvider supports three patterns:
//   - Static script: sequence of pre-recorded responses, consumed in order.
//   - Dynamic callback: function called per request that returns a response.
//   - Error injection: per-call status override.
type FakeProvider struct {
    Static     []FakeResponse                                                  // for static script pattern
    Dynamic    func(req FakeRequest) (FakeResponse, error)                     // for dynamic per-call pattern
    InjectError func(callIndex int) error                                      // for error injection pattern
}

type FakeRequest struct {
    Messages []Message
    Tools    []string
    Model    string
}

type FakeResponse struct {
    Text      string
    ToolCalls []ToolCall
    Usage     TokenUsage
    Status    string  // "success" by default; override for error patterns
}

// PromptAssertionHook is called once per Execute, with the system+user prompt
// the agent actually sent to the model. Used by tests that verify prompt
// construction/compaction without running a real provider.
type PromptAssertionHook func(systemPrompt, userPrompt string, contextFiles []string)

// CompactionAssertionHook is called whenever a real compaction runs. No-op
// compactions are NOT delivered (they don't fire compaction events either).
type CompactionAssertionHook func(messagesBefore, messagesAfter int, tokensFreed int)

// ToolWiringHook is called once per Execute, with the resolved tool list
// and the harness that received it. Used by tests that verify the right
// tools land at the right harness given the request's permission level.
type ToolWiringHook func(harness string, toolNames []string)

type ExecuteRequest struct {
    Prompt       string            // required
    SystemPrompt string            // optional; agent supplies a sane default if empty
    Model        string            // optional; resolved via ResolveRoute if empty
    Provider     string            // optional preference (soft); empty = router decides
    Harness      string            // optional preference (hard); empty = router decides
    ModelRef     string            // optional alias from the catalog: cheap/standard/smart/<custom>
    Effort       string            // "low" | "medium" | "high"; empty = harness default
    Permissions  string            // "safe" | "supervised" | "unrestricted"; default "safe"
    WorkDir      string            // required when the chosen harness uses tools

    // PreResolved bypasses ResolveRoute when the caller already has a decision
    // (e.g., from a prior ResolveRoute call). When non-nil, agent uses these
    // values verbatim and does not re-route. Provider/Model/Harness fields
    // above are ignored in this mode.
    PreResolved *RouteDecision

    // Three independent timeout knobs:
    //   Timeout        — wall-clock cap; the request fails after this duration
    //                    regardless of activity. 0 = no cap.
    //   IdleTimeout    — streaming-quiet cap; the request fails after this
    //                    duration of no events from the model. 0 = use harness
    //                    default (typically 60s).
    //   ProviderTimeout — per-HTTP-request cap to the provider; longer requests
    //                    are retried per the harness's failover rules. 0 = use
    //                    provider default.
    Timeout         time.Duration
    IdleTimeout     time.Duration
    ProviderTimeout time.Duration

    // Optional stall policy. When non-nil, agent enforces and ends execution
    // with Status="stalled" if any limit hits. The agent also derives an
    // implicit MaxIterations ceiling from StallPolicy (typically 2× the
    // ReadOnly limit) — caller does not configure MaxIterations directly.
    StallPolicy *StallPolicy

    // SessionLogDir overrides the default session-log directory for this
    // request. Used by execute-bead to direct logs into a per-bundle evidence
    // directory. Empty = use Options.ConfigPath-derived default.
    SessionLogDir string

    // Metadata is bidirectional: echoed back in every Event via Event.Metadata,
    // AND stamped onto every line written to the session log (e.g., bead_id,
    // attempt_id) so external log consumers can correlate.
    Metadata map[string]string
}

type StallPolicy struct {
    MaxReadOnlyToolIterations int // 0 = disabled
    MaxNoopCompactions        int // 0 = disabled
}

type RouteRequest struct {
    // Same shape as ExecuteRequest's routing-relevant fields.
    Model       string
    Provider    string
    Harness     string
    ModelRef    string
    Effort      string
    Permissions string
}

type RouteDecision struct {
    Harness    string         // resolved harness name
    Provider   string         // resolved provider (empty for harnesses without provider concept)
    Model      string         // resolved concrete model
    Reason     string         // human-readable explanation
    Candidates []Candidate    // full ranking, including rejected candidates with reasons
}

type Candidate struct {
    Harness     string
    Provider    string
    Model       string
    Score       float64
    Eligible    bool
    Reason      string  // why this score / why ineligible
    EstimatedCost CostEstimate
    PerfSignal    PerfSignal
}

type HarnessInfo struct {
    Name                  string
    Type                  string   // "native" | "subprocess"
    Available             bool
    Path                  string   // for subprocess harnesses
    Error                 string   // when Available=false
    IsLocal               bool
    IsSubscription        bool
    ExactPinSupport       bool
    SupportedPermissions  []string // subset of {"safe","supervised","unrestricted"}
    SupportedEfforts      []string // subset of {"low","medium","high"}
    CostClass             string   // "local" | "cheap" | "medium" | "expensive"
    Quota                 *QuotaState // nil if not applicable; live field
}

type ProviderInfo struct {
    Name           string
    Type           string  // "openai-compat" | "anthropic" | "virtual"
    BaseURL        string
    Status         string  // "connected" | "unreachable" | "error: <msg>"
    ModelCount     int
    Capabilities   []string  // {"tool_use","vision","json_mode","streaming"}
    IsDefault      bool      // matches the configured default_provider
    DefaultModel   string    // the per-provider configured default model, if any
    CooldownState  *CooldownState  // nil if not in cooldown; set if router has demoted this provider
}

type ModelInfo struct {
    ID              string
    Provider        string
    Harness         string  // for subprocess-only models, the owning harness
    ContextLength   int     // resolved (provider API > catalog > default)
    Capabilities    []string
    Cost            CostInfo
    PerfSignal      PerfSignal
    Available       bool
    IsConfigured    bool    // matches an explicit model_routes entry
    IsDefault       bool    // matches the configured default model
    CatalogRef      string  // canonical catalog reference if the catalog recognizes this model
    RankPosition    int     // ordinal in the latest discovery rank for this provider; -1 if unranked
}

type CooldownState struct {
    Reason     string    // "consecutive_failures" | "manual" | etc.
    Until      time.Time // when cooldown expires
    FailCount  int
    LastError  string
}

type RouteStatusReport struct {
    Routes          []RouteStatusEntry
    GeneratedAt     time.Time
    GlobalCooldowns []CooldownState
}

type RouteStatusEntry struct {
    Model            string             // route key
    Strategy         string             // "priority-round-robin" | "first-available"
    Candidates       []RouteCandidateStatus
    LastDecision     *RouteDecision     // most recent ResolveRoute result for this key (cached)
    LastDecisionAt   time.Time
}

type RouteCandidateStatus struct {
    Provider           string
    Model              string
    Priority           int
    Healthy            bool
    Cooldown           *CooldownState
    RecentLatencyMS    float64  // observation-derived
    RecentSuccessRate  float64  // 0-1
}

type ModelFilter struct {
    Harness  string  // empty = all harnesses
    Provider string  // empty = all providers
}

type HealthTarget struct {
    Type string  // "harness" | "provider"
    Name string
}

type Event struct {
    Type     string          // see event types below
    Sequence int64
    Time     time.Time
    Data     json.RawMessage // shape depends on Type; see schemas below
}

// Closed event-type union:
//   text_delta       — streaming text chunk
//   tool_call        — model invoked a tool
//   tool_result      — tool returned
//   compaction       — actual compaction performed (no-op compactions emit nothing)
//   routing_decision — harness/provider/model resolved (emitted at start)
//   stall            — StallPolicy triggered; execution ending
//   final            — execution complete; carries usage, cost, status, session log path
```

### Event JSON shapes

```jsonc
// type=text_delta
{"text": "..."}

// type=tool_call
{"id": "...", "name": "bash", "input": {...}}

// type=tool_result
{"id": "...", "output": "...", "error": "...", "duration_ms": 123}

// type=compaction
{"messages_before": 30, "messages_after": 12, "tokens_freed": 4521}

// type=routing_decision
{
  "harness": "agent",
  "provider": "bragi",
  "model": "qwen/qwen3.6-35b-a3b",
  "reason": "cheap-tier match; bragi reachable; 256K context",
  "fallback_chain": ["openrouter:qwen/qwen3.6"]
}

// type=stall
{"reason": "no_compactions_exceeded", "count": 50}

// type=final
{
  "status": "success" | "failed" | "stalled" | "timed_out" | "cancelled",
  "exit_code": 0,
  "error": "",
  "duration_ms": 12345,
  "usage": {"input_tokens": 7996, "output_tokens": 267, "total_tokens": 8263},
  "cost_usd": 0.0042,
  "session_log_path": "/path/to/session.jsonl",
  "messages": [...],   // optional history continuation
  "routing_actual": {
    "harness": "agent",
    "provider": "openrouter",            // distinct from start-event routing_decision when fallback fired
    "model": "qwen/qwen3.6",
    "fallback_chain_fired": ["bragi:qwen/qwen3.6 (timeout)", "openrouter:qwen/qwen3.6 (success)"]
  }
}
```

### Behaviors the contract names (so callers don't have to)

The agent owns these execution-time behaviors that DDx today implements piecemeal in `agent_runner.go`. The contract guarantees them; callers do not opt in or out:

- **Orphan-model validation.** When `Model` is set but matches no provider's discovery and no catalog entry, Execute fails fast with `Status="failed", error="orphan model: <name>"` rather than silently picking the wrong provider.
- **Provider request deadline wrapping.** Every HTTP call to a provider is wrapped with `ProviderTimeout`. Per-request failures classified as transport/auth/upstream are eligible for failover within the route's candidate list; prompt/tool-schema errors are not.
- **Route-reason attribution.** The start-event `routing_decision` and final-event `routing_actual` together capture why each candidate was tried/picked.
- **Stall detection.** Per `StallPolicy`. Default policy (when caller passes `nil`) uses conservative limits that match today's `agent_runner.go:200-318` thresholds.
- **Compaction-stuck breaking.** Implicit in the StallPolicy default; callers don't configure separately.
- **OS-level subprocess cleanup.** On `ctx.Done()`, agent reaps PTY + orphan processes for subprocess harnesses. Tested and guaranteed.

### Internal harness interface (NOT part of the contract; for ddx-agent maintainers)

Inside ddx-agent, every harness — native and subprocess — implements:

```go
package internal

type Harness interface {
    Info() HarnessInfo
    HealthCheck(ctx context.Context) error
    Execute(ctx context.Context, req executeReq) (<-chan agentlib.Event, error)
}
```

Native and subprocess implementations live side-by-side in `internal/harnesses/`. The routing engine treats them uniformly.

## Implementation sequence (every step listed; dependencies explicit)

### Phase 0 — Pre-work (parallel-able)

- 0.1 **Cross-repo consumer audit.** Greps to run:
    - `grep -rn "DocumentDrivenDX/ddx/cli/internal/agent" ~/Projects/{helix,dun}` if those dirs exist (Go imports).
    - `grep -rn "SD-015\|SD-023\|CONTRACT-002\|plan-2026-04-08-agent-routing" ~/Projects/{helix,dun}` (cross-repo doc references — already known: helix has `~/Projects/helix/docs/helix/02-design/contracts/CONTRACT-001-audit.md:27`).
    - Any non-zero hit becomes a named migration target with its own bead. Spec-cleanup work must touch HELIX too — not just both core repos.
- 0.2 **Snapshot baseline.** Run the existing test suites in both repos. Tag the baseline commits in each repo. Migration must keep these passing throughout (mod the migration-window exemptions).

### Phase 1 — Spec authority (agent first, then DDx)

- 1.1 **Write CONTRACT-001-ddx-agent-service.md** in agent repo. Verbatim from the contract draft above.
- 1.2 **DELETE CONTRACT-002** from agent repo. No supersession marker. Reference-grep + remove all hits.
- 1.3 **TRIM agent's boundary-claiming docs** — architecture, plan-2026-04-10, SD-002, SD-005, FEAT-004, PRD. Each rewritten to describe agent internals only; reference CONTRACT-001 for the external surface.
- 1.4 **DELETE DDx's boundary docs** — SD-015, SD-015-resolution-path-trace, SD-023, plan-2026-04-08. No supersession markers.
- 1.5 **GUT DDx's FEAT-006** to a one-page reference pointing at CONTRACT-001.
- 1.6 **Reference-grep cleanup** in both repos. Any remaining mention of deleted IDs is a re-entanglement risk; remove it.

Phase 1 must merge before Phase 2 begins. Spec authority is the foundation.

### Phase 2 — Implement DdxAgent over EXISTING externals (do NOT move to internal/ yet)

This phase builds the new contract on top of the current package layout. Internal/ comes later (Phase 6.0) when DDx has migrated and the legacy externals are no longer in use. Reordering this from the v1 plan: implement first, then enforce the boundary.

- 2.1 **`Options.FakeProvider` test seam.** Public struct; static-script and dynamic-callback patterns. Build-tag-guarded so production binaries can't construct one.
- 2.2 **`Execute` over native providers.** Wraps the existing agent loop. Per-model context resolution internal (supersedes agent-6f8caa00 / agent-8682793b).
- 2.3 **No-op compaction telemetry suppression** (supersedes agent-22c95151).
- 2.4 **`StallPolicy` enforcement** inside Execute. DDx's circuit-breaker code becomes deletable in Phase 5.
- 2.5 **PromptAssertionHook + CompactionAssertionHook + ToolWiringHook test seams.** Three additional injection points beyond FakeProvider, each guarded by the testseam build tag.
- 2.6 **Three timeout knobs** plumbed: Timeout (wall-clock), IdleTimeout (activity-quiet), ProviderTimeout (per-HTTP). Documented behavior matches contract.
- 2.7 **Orphan-model validation, route-reason attribution, OS-level subprocess cleanup** per contract guarantees.

### Phase 2.5 — Harness migration content inventory (PORT FAITHFULLY, do not lose subtlety)

The harness wrappers are not just `exec.Command + parse JSON`. They include subtle PTY/tmux scraping, quota cache lifecycles, stream-format fallback paths, and signal extraction that has been hardened against real-world vendor-CLI quirks. **Every file below ports verbatim or near-verbatim into agent's `internal/harnesses/`. None of this is a "rewrite from scratch" opportunity.**

#### Production code to port (~3,460 lines)

| File (current path under `cli/internal/agent/`) | Lines | What it does | New home |
|---|---|---|---|
| `claude_stream.go` | 518 | claude CLI subprocess invocation, stream-json parsing, args-unsupported fallback, finalize | `internal/harnesses/claude/` |
| `claude_quota_cache.go` | 234 | Quota snapshot read/write, freshness check, routing decision, async refresh | `internal/harnesses/claude/` |
| `routing_signal_tmux.go` | 316 | **TMUX-based quota probing for both claude (`--print /usage`) AND codex (`exec /status`)**; ANSI stripping; output parsing for two distinct formats | `internal/harnesses/{claude,codex}/quota_tmux.go` (split per harness) |
| `routing_signal_http.go` | 414 | HTTP-based signal extraction for HTTP providers | `internal/routing/signals/` |
| `routing_signal_adapters.go` | 701 | Adapter layer routing signals → routing engine | `internal/routing/signals/` |
| `routing_signals.go` | 497 | Core signal abstraction (RoutingSignal, signal lifecycle) | `internal/routing/signals/` |
| `session_log_format.go` | 192 | Per-harness session-log line formatting (claude/codex/agent each have distinct shapes) | `internal/sessionlog/format.go` |
| `session_log_tailer.go` | 109 | Streaming tail of session log JSONL | `internal/sessionlog/tailer.go` |
| `registry.go` | 263 | Harness registry definitions (Binary, BaseArgs, PermissionArgs, PromptMode, ModelFlag, EffortFlag, etc.) | `internal/harnesses/registry.go` |
| `state.go` | 216 | Runner state, ProbeHarnessState, availability check, **depends on Harness/Registry/HarnessState — must move together** | `internal/harnesses/state.go` |

#### Test code to port (~3,000 lines)

| File | Lines | What it covers |
|---|---|---|
| `routing_test.go` | 915 | Primary routing tests for the legacy DDx planner, evaluator, ranking, selection, and request normalization |
| `routing_signal_tmux_test.go` | 143 | TMUX scrape parser tests for claude `/usage` and codex `/status` output |
| `routing_signal_adapters_test.go` | 142 | Signal adapter tests |
| `routing_signals_integration_test.go` | 206 | End-to-end signal flow |
| `routing_discovery_test.go` | 356 | Discovery + fuzzy matching |
| `routing_metrics_test.go` | 84 | Routing metric aggregation |
| `claude_stream_test.go` | (TBD) | Stream parser, fallback handling |
| `tier_escalation_test.go` + `_integration_test.go` | 559 | Tier escalation rules |
| `provider_deadline_test.go` | 188 | Per-request deadlines, idle timeouts |
| `script_test.go` | 346 | Script harness (test-only directive interpreter) |
| `virtual_test.go` | 224 | Virtual provider (test-only replay) |

**Tests come with their own subtle fixtures**: `routing_signal_tmux_test.go` has multi-line ANSI-laden golden strings of real claude/codex output. `claude_stream_test.go` has stream-json fixtures that exercise the fallback path when claude emits non-JSON. These are **regression assets** against real vendor-CLI quirks — they port verbatim.

#### TUI/PTY subtleties that must NOT regress

1. **`routing_signal_tmux.go::ReadClaudeQuotaViaTmux`** — spawns `tmux` to invoke `claude --bare --print /usage`, captures pane output, strips ANSI, parses quota windows. Fragile to claude CLI output format changes; tests guard against regressions. Same pattern for codex.
2. **`claude_stream.go::parseClaudeStream`** — parses `claude --output-format stream-json` line by line, handles partial JSON, extracts tool calls, total tokens. Has an args-unsupported fallback (`claudeStreamArgsUnsupported`) that retries without `--verbose` for older claude versions.
3. **`claude_quota_cache.go::RefreshClaudeQuotaAsync`** — debounced async refresh with goroutine lifecycle. Wrong handling produces zombie goroutines.
4. **OS-level cancellation** — `runClaudeStreaming` and the codex equivalent must reap subprocess + PTY on `ctx.Done()`. Existing tests use `signal.Notify`-aware cleanup; that has to come along.

#### Non-negotiable: behavioral parity tests

Before deleting any of these from DDx, the upstream port must pass an **identical-behavior test suite**: identical inputs (recorded fixtures of claude/codex output, recorded provider responses) produce identical outputs (parsed quota, routing decisions, session log lines). The comparison-suite framework in `compare.go`/`benchmark.go` (also moving upstream) is the natural place to run this parity check.

### Phase 3 — Port harness wrappers + implement listing/routing surface

After Phase 2 produces a working contract over native providers, Phase 3 brings the subprocess harnesses into the agent module and exposes the listing/routing methods. Each step ports content (not "rewrites cleanly"); see Phase 2.5 inventory for verbatim-vs-rewrite breakdown.

- 3.1 **Port `routing_signal_tmux.go`** — verbatim-port-able (316 lines, no DDx-specific imports). Lives at `internal/harnesses/{claude,codex}/quota_tmux.go` (split per harness).
- 3.2 **Port `claude_quota_cache.go`** — NOT verbatim. Path namespace rebase: `~/.local/state/ddx/claude-quota.json` → `~/.local/state/ddx-agent/claude-quota.json`. Env var `DDX_CLAUDE_QUOTA_CACHE` → `DDX_AGENT_CLAUDE_QUOTA_CACHE`. Back-compat read-only window: also try the old path/env if the new one is empty, for one minor version.
- 3.3 **Port `claude_stream.go`** — NOT verbatim. Stream parser + subprocess driver port; result-shape rewrite required. Currently emits DDx `Result` shape; rewrite to emit `agentlib.Event`. Tests need rewrite (claude_stream_test.go is tied to the current emit format). **Budget 2-3 days.**
- 3.4 **Port codex / opencode / gemini / pi subprocess harness wrappers.** Each implements the internal `Harness` interface. Source: DDx `registry.go` definitions. Per-harness peculiarities (codex `--json`, opencode `run --format json`, etc.) preserved verbatim.
- 3.5 **Port `registry.go` + `state.go` together** — they're conjoined (state.go depends on Harness/Registry/HarnessState types from registry.go). Both move to `internal/harnesses/`. ProbeHarnessState becomes part of the harness implementations.
- 3.6 **`ListHarnesses`** with rich HarnessInfo (install state, supported permissions, supported efforts, quota probe).
- 3.7 **`ListProviders` + polymorphic `HealthCheck`** — including `IsDefault`, `DefaultModel`, `CooldownState` per contract.
- 3.8 **`ListModels`** with full metadata: cost, perf, capabilities, context_length, IsConfigured, IsDefault, CatalogRef, RankPosition. Live discovery + catalog merge internal.
- 3.9 **`ResolveRoute`** — single routing engine. Source: DDx's `routing.go` (915 lines + 915 test lines) + agent's `routing_smart.go` consolidated. Test rewrite: `newTestRunnerForRouting()` → `newTestRoutingEngine()` mechanical replacement. Consolidates the six "badly implemented" smells:
    - Single RouteRequest type (Provider field present from day one — fixes ddx-8610020e).
    - Canonical-form fuzzy matcher (case + vendor-prefix normalization — fixes ddx-0486e601).
    - Per-(harness,provider,model) capability gating (fixes ddx-4817edfd subtree).
    - Profile-aware tier escalation that talks to provider failover (fixes ddx-3c5ba7cc).
    - Single observation store + cooldown abstraction.
- 3.10 **`RouteStatus`** — operator-dashboard surface. Aggregates cooldown state, recent decisions cache, observation-derived per-(provider,model) latency. Distinct from per-request `ResolveRoute`.
- 3.11 **`TailSessionLog`** — surfaces in-progress session events to subscribers (DDx workers). Backed by the same internal session-log writer Execute uses.
- 3.12 **Event schema implementation.** All event types (text_delta, tool_call, tool_result, compaction, routing_decision, stall, final) emitted identically by every harness backend. `final` carries `routing_actual` distinct from start-event.
- 3.13 **Port `provider_deadline.go` semantics into `internal/execution/`** — three timeout knobs (wall-clock, idle, per-request) become internal to Execute.

**Phase 3 dependency: 3.1-3.5 (port harnesses) blocks 3.9 (ResolveRoute), since routing scores harness candidates that don't exist until they're ported.**

### Phase 3.5 — Integration suite (lives in agent, grows over time)

Concrete deliverable: ddx-agent ships an integration suite that, given credentials for a variety of harnesses + providers, runs a small test suite across all of them and reports comparison results. This is not Phase 7 verification — it's a permanent part of the agent module that callers (DDx, HELIX, anyone else) can invoke or extend.

**What it does:**

- 3.5.1 **Discovery.** From config, enumerate every available `(harness, provider, model)` combination with valid credentials. Skip any that fail health check.
- 3.5.2 **Test corpus.** A small, curated set of tasks that exercise:
    - Single tool call (e.g., "list files in `cmd/`")
    - Multi-tool sequence (e.g., "find files matching `*.go`, count their lines")
    - Tool error handling (e.g., "read a file that doesn't exist; report what happened")
    - Reasoning + tool combination (e.g., "what's the largest file in this directory")
    - Long-output generation (basic completion latency under load)
- 3.5.3 **Run sweep.** Each test runs against every discovered candidate. Captures: success/failure, total tokens, cost, wall-clock latency, time-to-first-token, tool-call count, output (for parity check).
- 3.5.4 **Report.** Tabular comparison: candidate × test → metrics. Identifies parity failures (different harnesses producing materially different answers) for human review.

**Where it lives:**

- `cmd/ddx-agent-bench/` (new) — the runner binary.
- `bench/corpus/` — test corpus YAML or JSON files. Versioned with the repo. Anyone can extend.
- `bench/results/` (gitignored) — local result storage. Operators can compare runs over time.
- The corpus + runner build on `compare.go`/`benchmark.go`/`quorum.go` infrastructure (which is moving from DDx in Phase 5.11), so most of the building blocks already exist — they just need a new home and a corpus around them.

**Credential management:**

- Reads from the same agent config the runtime uses, layered: CLI flags > env vars > project config > user config > agent defaults. Same precedence as `cli/cmd/agent_models.go` exhibits today; bench mirrors it explicitly.
- Skips harnesses without credentials gracefully (report "skipped: no credentials" not "fail").

**Cost guard:**

- `--max-cost-usd <N>` flag (default $0.50) — bench halts the sweep if accumulated cost exceeds the cap. Any harness mid-sweep when the cap is hit completes its current task, then no further candidates run.
- Per-task estimated cost is checked against remaining budget before invocation; tasks that would push over are skipped with explicit "skipped: cost cap" reason.

**Determinism:**

- All bench tasks set `temperature=0`, `seed=<task-specific>` where the harness/provider supports it. Seeds are recorded with results so reruns are reproducible.
- Harnesses/providers that don't support deterministic sampling are flagged in the report ("non-deterministic: {reason}"); the bench still runs them but parity-comparison treats their outputs as advisory only.

**Parity comparison method:**

- **Tool-call sequence equality** is the primary signal — two harnesses producing the same sequence of `(tool_name, normalized_args)` for the same task are considered behaviorally equivalent regardless of prose differences.
- **Final-answer string-similarity** (cosine on character n-grams) is a secondary signal flagging "looks similar but not identical" cases for human review.
- **No automated semantic equivalence judgment.** The report tabulates results; humans review divergences. This is an explicit scope limit — semantic equivalence is a research problem, not a Phase 3.5 deliverable.
- Future scope (out): embedding-similarity, LLM-as-judge for semantic equivalence. Filed as follow-up beads, not blocking.

**Growth path:**

- Each new harness shipped (gemini today, future ones tomorrow) adds itself to the discovery automatically; no bench code changes.
- Each new bug class found in production (e.g., "the model returned the wrong tool name") becomes a new corpus entry.
- Eventually the bench is the canonical "is the native agent harness winning at batch noninteractive?" measurement — the question that originally drove Option C.

**Beads filed under Phase 3.5 in the breakdown:** integration-suite scaffolding, initial corpus, report format, credential discovery, parity comparison.

### Phase 4 — Tag `agent v0.4.0-pre`

- 4.1 **Tag pre-release** with new contract + legacy `agentlib.Run` available side by side. Legacy marked Deprecated:.
- 4.2 **Smoke test** the standalone `ddx-agent` CLI against the new contract.

### Phase 5 — DDx migration (no flag day; per-call-site)

- 5.1 **Bump go.mod to `agent v0.4.0-pre`** in DDx.
- 5.2 **Add `cli/internal/agent/legacy/` adapter layer** — wraps the new Service to emit the old call shapes during migration. Lets each cmd/ migrate independently.
- 5.3 **Migrate `cmd/agent_check.go`** → `service.HealthCheck` + `service.ListProviders`.
- 5.4 **Migrate `cmd/agent_providers.go`** → `service.ListProviders` (including `IsDefault`/`CooldownState` rendering).
- 5.5 **Migrate `cmd/agent_models.go`** → `service.ListModels` (including `IsConfigured`/`IsDefault`/`RankPosition` rendering).
- 5.6 **Migrate `cmd/agent_route_status.go`** → `service.RouteStatus` (the report surface) and optionally `service.ResolveRoute` for "what would I pick now" probes.
- 5.7 **Migrate `cli/internal/server/graphql/resolver_providers.go`** → `service.ListProviders`.
- 5.8a **Wire the agentHarness adapter** in `agent_runner.go` calling `service.Execute(ctx, req)`. Leave the old `runner.go`/`agent_runner.go` code paths in place behind feature-gate or switch. CI must stay green.
- 5.8b **Delete the old code paths** — `embeddedCompactionConfig`, `findTool`, `buildAgentProvider`, `wrapProviderWithDeadlines`, all the now-unreachable functions. Separate bead from 5.8a so each is reviewable independently.
- 5.9 **Migrate virtual-provider test sites** (32 of them) to `Options.FakeProvider`. Static-script and dynamic-callback patterns per spec. Tests using prompt assertions adopt `PromptAssertionHook`; tests using compaction adopt `CompactionAssertionHook`; tests using tool wiring adopt `ToolWiringHook`.
- 5.10 **Delete DDx-side stall/circuit-breaker code** (`agent_runner.go:200-318`) — replaced by upstream StallPolicy.
- 5.11 **Move routing infrastructure upstream first** — `routing.go`, `routing_metrics.go`, `routing_signals*.go`, `tier_escalation.go`, `discovery.go`, `provider_deadline.go`, `providerstatus/probe.go`, `registry.go`, parts of `types.go` (RouteRequest, CandidatePlan, Harness, HarnessState). **Slice by file, not by category** — ~7 separate beads (one per file or tightly-coupled file group). Order within: state.go + registry.go together (mutually dependent), then routing.go (depends on registry types), then signals/metrics/discovery/etc. independently.
- 5.12 **Move comparison/benchmark upstream after routing** — `compare.go`, `benchmark.go`, `quorum.go`, `condense.go` → agent. `compare.go` calls the legacy DDx candidate ranking API that moved in 5.11; this depends on 5.11 completing. ~4 separate beads.
- 5.13 **Verify imports.** `grep -rn "DocumentDrivenDX/agent/" cli/` should show ONLY `agentlib` (root) — no internal subpackage imports.

### Phase 6 — Cutover and cleanup

- 6.0 **Move agent's residual public packages under `internal/`** — compaction, prompt, tool, session, observations, modelcatalog, provider/openai, provider/anthropic, provider/virtual. `go build ./...` proves no external import slipped through. **This is the v0.5.0 trigger.**
- 6.1 **Rename `glob` → `find`** in `internal/tool/` (no alias). Tool catalog updated.
- 6.2 **Tag `agent v0.5.0`** — internal-only surface; legacy `agentlib.Run` removed.
- 6.3 **Bump DDx go.mod to `agent v0.5.0`.** Compile failures expose any remaining old-API usage; fix or fail.
- 6.4 **Delete DDx's `cli/internal/agent/legacy/` adapter** — no callers.
- 6.5 **Close superseded routing beads** with notes pointing at the upstream replacements: ddx-8610020e, ddx-0486e601, ddx-3c5ba7cc, ddx-2f5a2284, ddx-4817edfd subtree, ddx-0216b966, ddx-2d974641, ddx-76df1a46, agent-6f8caa00, agent-22c95151.

### Phase 7 — Verification

- 7.1 **Re-run baseline test suites** in both repos. Must pass.
- 7.2 **Run the comparison suite** (now in agent) — confirm native-vs-subprocess A/B test executes uniformly.
- 7.3 **Reference-grep one more time** for any cross-repo entanglement that snuck back in.
- 7.4 **Doc audit**: list all `.md` files in both `docs/helix/` trees; confirm none reference deleted IDs.

## Risk register

| Risk | Mitigation |
|---|---|
| Mid-migration production fix needed | Adapter layer (`legacy/`) lets old code paths keep working until each is migrated. Hot-fix on legacy is allowed during migration. |
| Test suite churn | Four test seams (FakeProvider, PromptAssertionHook, CompactionAssertionHook, ToolWiringHook) spec'd before migration starts. Any test that can't migrate cleanly is a gap in the seam — fix the seam, don't add a workaround. |
| HELIX/Dun consumer break | Phase 0.1 audit (Go imports + doc references) catches them; named migration beads filed before Phase 5. |
| Subprocess harness PTY/orphan cleanup regressions | Contract names this guarantee; Phase 2.7 + Phase 3.13 mandate explicit cancellation tests. Don't skip. |
| Spec drift recurrence | Reference-cleanup grep in Phase 1 + Phase 7 catches it. CI lint can be added later (post-epic). |
| Routing behavior change goes unnoticed | Existing test suites + integration suite A/B run before/after migration; any divergence on identical inputs is a bug. |
| **Test environment drift** (CI runs corrupt operator state via real `~/.local/state/ddx/claude-quota.json` paths) | All cache-path env vars must be honored; CI sets them to `$TMPDIR/<job-id>/...`. Phase 3.2 (claude_quota_cache port) is also where back-compat read-only window lands. |
| **Vendor CLI fixture rot** (claude/codex CLIs evolve; tmux scrape patterns break) | Owners commit to refresh fixtures monthly and on observed CI failures. Failed fixtures are alerts, not silent skips. |
| **`go.mod` pseudo-version pin churn** during migration window | Document bump cadence: agent maintainer bumps a fresh pre-release tag whenever a Phase-3 contract addition lands; DDx pulls within 1 day. Pre-release tags follow `v0.4.0-pre.<sha>` form. |
| **Observation-store schema collision** (DDx routing_metrics + agent observations both write today) | Phase 3.9 spec'd which schema wins (agent's ModelRouteCandidateConfig-keyed observations) and provides a one-shot migration script for DDx-side stores. Migration runs on first execute against `agent v0.4.0-pre`. |
| **Path-namespace rebase regressions** (claude-quota cache, etc.) | Back-compat read window for one minor version. Phase 6.0 (internal/ move, v0.5.0) closes the window. |

## Bead breakdown across both projects (DRAFT — review-then-file)

The breakdown below is informed by Phases 0-7 above. **Not yet filed** — review-then-file per the user's directive.



### ddx-agent epic (`/Users/erik/Projects/agent/`)

**Title:** `Epic: ddx-agent becomes the LLM-routing-and-execution module — single Service contract, harnesses owned`

**Sub-beads (ordered, dependencies as listed):**

1. **Write CONTRACT-001-ddx-agent-service.md** — the entire external surface in one doc. Blocks: everything else.
2. **Spec cleanup** — DELETE CONTRACT-002; TRIM architecture/plan-2026-04-10/SD-002/SD-005/FEAT-004/PRD to drop boundary claims; reference-cleanup grep. Blocks: nothing (parallel-able with code work).
3. **Move packages under `internal/`** — compaction, prompt, tool, session, observations, modelcatalog, provider/openai, provider/anthropic, provider/virtual, config (selective). Compiler-enforced boundary. Blocks: 4-9.
4. **Implement `DdxAgent.Execute` over native providers** — the existing agent loop wrapped in the new contract. Backed by Options.FakeProvider test seam.
5. **Implement subprocess harness wrappers** — claude, codex, opencode, pi, gemini. Move `cli/internal/agent/registry.go`, `claude_stream.go`, `claude_quota_cache.go` from DDx. Each harness implements an internal `Harness` interface; routing engine sees them uniformly.
6. **Implement `DdxAgent.ListHarnesses`** — rich HarnessInfo per refinements above.
7. **Implement `DdxAgent.ListProviders` + `HealthCheck`** — polymorphic Target.
8. **Implement `DdxAgent.ListModels`** — live discovery + catalog merge, full metadata (cost, perf, capabilities, context_window).
9. **Implement `DdxAgent.ResolveRoute`** — consolidates DDx routing.go + agent routing_smart.go into one engine. Fixes smells 1, 2, 4 structurally.
10. **Per-model context window resolution into Execute** — supersedes `agent-6f8caa00` and `agent-8682793b`. The per-model lookup happens internally; DDx never sees ContextWindow.
11. **Compaction telemetry suppression** — supersedes `agent-22c95151`. No-op compactions emit nothing at default verbosity.
12. **`ResolveModel` exact + fuzzy with canonical normalization** — consolidates upstream half of `ddx-0486e601`. Case-insensitive, vendor-prefix-aware.
13. **`StallPolicy` enforcement inside Execute** — moves DDx's circuit breakers upstream. No-op compaction + read-only-tool stall both end execution with `Status: stalled`.
14. **Move `findTool` upstream** — rename `glob` → `find`. No alias.
15. **Tag `agent v0.4.0-pre`** — new contract + legacy `agentlib.Run` available side-by-side.
16. **Tag `agent v0.5.0`** — DELETE legacy `agentlib.Run`, all the `agent/{compaction,prompt,tool,session,observations,modelcatalog,provider/*}` exports. Compiler-enforced minimal surface.

### DDx epic (`/Users/erik/Projects/ddx/`)

**Title:** `Epic: DDx becomes thin Service consumer of ddx-agent contract`

**Sub-beads (ordered):**

1. **Spec cleanup** — DELETE `SD-015-agent-routing-and-catalog-resolution.md`, `SD-015-resolution-path-trace.md`, `SD-023-agent-routing-visibility.md`, `plan-2026-04-08-agent-routing-and-catalog-resolution.md`. GUT `FEAT-006-agent-service.md` to a one-page reference. Reference-cleanup grep + remove. Blocks: nothing (can run in parallel with code work).
2. **HELIX/Dun import audit** — grep both projects for `cli/internal/agent` imports. File a migration bead per consumer. Blocks: 16.
3. **Bump go.mod to `agent v0.4.0-pre`** — new contract available; legacy still compiles. Blocks: 4-12.
4. **Add `cli/internal/agent/legacy/` adapter layer** — wraps the new Service to expose the old call shapes during migration. Lets cmd/ files migrate one at a time without a flag day. Deletes in step 14.
5. **Migrate `cmd/agent_check.go`** → `service.HealthCheck` + `service.ListProviders`.
6. **Migrate `cmd/agent_providers.go`** → `service.ListProviders`.
7. **Migrate `cmd/agent_models.go`** → `service.ListModels`.
8. **Migrate `cmd/agent_route_status.go`** → `service.ResolveRoute` + agent's RouteStatus surface.
9. **Migrate `cli/internal/server/graphql/resolver_providers.go`** → `service.ListProviders`.
10. **Replace `agent_runner.go` + `runner.go`** with the agentHarness adapter. Body collapses to `service.Execute(ctx, req)` + drain events. Delete `embeddedCompactionConfig`, `findTool`, `buildAgentProvider`, `wrapProviderWithDeadlines`, etc.
11. **Migrate virtual-provider test sites** (32 of them) to `Options.FakeProvider`. Blocks: 14.
12. **Migrate stall/circuit-breaker code** — delete `agent_runner.go:200-318` (replaced by upstream StallPolicy).
13. **Move comparison/benchmark to ddx-agent** — `compare.go`, `benchmark.go`, `quorum.go`, `condense.go` move upstream (where the comparison suite belongs per the testability argument).
14. **Move routing infrastructure to ddx-agent** — `routing.go`, `routing_metrics.go`, `routing_signals*.go`, `tier_escalation.go`, `discovery.go`, `provider_deadline.go`, `providerstatus/probe.go`, `registry.go`, parts of `types.go` (RouteRequest, CandidatePlan, Harness, HarnessState). Delete from DDx.
15. **Bump go.mod to `agent v0.5.0`** — legacy gone. DDx must compile against the minimal surface only. Any remaining import of `agent/{compaction,prompt,tool,session,...}` fails to compile. Fix or fail.
16. **Delete `cli/internal/agent/legacy/` adapter** — no callers left.
17. **Close superseded routing beads** — with notes pointing at the upstream replacements. Includes: `ddx-8610020e`, `ddx-0486e601`, `ddx-3c5ba7cc`, `ddx-2f5a2284`, `ddx-4817edfd` subtree, `ddx-0216b966`, `ddx-2d974641` (the autorouting epic — its acceptance is now "the new contract works"), `ddx-76df1a46`.

### Cross-tracker sequencing

- Agent step 1 (CONTRACT-001) **blocks** DDx step 1 (because DDx's gutted FEAT-006 references the contract).
- Agent step 4 + 5 **block** DDx step 3 (because there has to be something to bump to).
- Agent step 16 (`v0.5.0`) **blocks** DDx step 15 (final cutover).
- Otherwise, work parallelizes per-side.

### Bead-filing notes

- Both epics filed first; sub-beads link via `--parent`.
- Cross-tracker references go in `--description` (not deps, since dep edges are intra-tracker only).
- `--labels` include `area:agent`, `area:contract`, `phase:build` per existing conventions.

## Bead decomposition

Epic on agent tracker: **Service interface — narrow ddx-agent public surface**
- agent-A: design — write up the interface + JSON event schema; HELIX sign-off
- agent-B: implement Service; refactor cmd/ddx-agent/main.go to consume it
- agent-C: deprecate legacy exports, release v0.4.0
- agent-D: remove legacy exports, release v0.5.0 (after DDx migration confirmed)

Epic on DDx tracker: **migrate to ddx-agent Service interface**
- ddx-1: bump go.mod, agent_runner.go migration (largest single change)
- ddx-2: providers/models migration
- ddx-3: tools migration
- ddx-4: prompt migration
- ddx-5: sessions/observations/compaction migration (supersedes ddx-76df1a46)
- ddx-6: virtual-provider test paths migration
- ddx-7: bump to v0.5.0; verify import-list shrunk; cleanup
- ddx-8: HELIX/Dun consumer docs (optional — only if those projects are ready)

Subprocess CLI (`ddx-agent` binary as a polyglot front door) is **deferred** — easy to add later as a `cliService` implementation of the same interface if/when a non-Go consumer needs it.

## What this plan deliberately does NOT do

- Does not switch DDx to subprocess execution. Reason: the boundary problem is API surface, not in-process vs out-of-process. Subprocess can be added as a `cliService` adapter without changing DDx's call sites if the interface is right.
- Does not redesign routing/discovery. Routing is still configured via the agent-side config; DDx passes provider+model (or a ref) and the Service routes.
- Does not change the harness model (codex/claude/gemini etc.). Those remain configured upstream; DDx selects via `ExecuteRequest.Provider`.
- Does not add new observability primitives. The event stream from `ExecuteStream` is sufficient; structured logging stays internal.

## Open questions for review

1. Is `Tools []string` (names) the right surface, or do callers need to define custom tools? If custom tools are real, we need `Tools []ToolSpec` where ToolSpec is a JSON-Schema-shaped struct (still no Go function pointers crossing the boundary).
2. Does `ExecuteStream` need a write-back channel (e.g., for human-in-the-loop tool approvals), or is it strictly read-only?
3. Should `ExecuteRequest` carry the working directory, or should the Service launch in the caller's CWD by default? (Probably: keep WorkDir explicit; Service should never inherit ambient state.)
4. Is one `Service` instance per `ExecuteRequest` correct, or should `New(Options)` return a long-lived service that handles many executes? (Probably long-lived; routing tables, provider connections, catalog cache should be amortized.)
5. Do we need `ListHarnesses` as a fifth verb? Today DDx surfaces `ddx agent list` showing codex/claude/gemini/etc. — is that "providers" or its own concept?

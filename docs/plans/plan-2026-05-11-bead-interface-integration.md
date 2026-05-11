---
ddx:
  id: plan-2026-05-11-bead-interface-integration
---
# Bead Interface Refactor — Integration Plan

Date: 2026-05-11
Status: Draft — pre-execution integrity audit and enforcement strategy for the bead backend interface refactor (gating bead `ddx-c6317784`)

## Why this exists

The bead backend interface refactor (`docs/helix/02-design/plan-2026-05-11-bead-backend-interfaces.md`) is large and load-bearing. The risk is not the new interfaces themselves — those are well-specified. The risk is that **side-doors and legacy paths persist after the refactor**, silently undermining the contracts the design depends on:

- A new backend (e.g. AxonStore) implements `Backend` correctly, but callers continue to grab `*Store` concretely and reach for the legacy concrete methods — which silently bypass the per-row optimization the new design exists to enable.
- A workflow operation that should flow through `Apply` is performed via direct field mutation on a loaded bead — bypassing the named-Operation catalog and the optimization paths it unlocks.
- A new file outside `cli/internal/bead/` reads `.ddx/beads.jsonl` directly, encoding the file format into another package and creating a maintenance liability that the interface refactor is meant to eliminate.
- Tests use legacy patterns (`*Store` concretely, fixture-bypassing factories) and pass — making CI green even when the contract is incomplete.
- Removed methods come back via legacy callers that no one noticed.
- Deprecated mechanisms (`DDX_AXON_EXPERIMENTAL` env var, the old `AxonBackend` whole-corpus path) stick around indefinitely.

This plan **catalogs** every side-door and legacy path I can identify, **specifies a closure mechanism** for each, **defines the integrity gates** that will catch new violations before they land, and **sequences the cleanup** so the integration completes cleanly. The user's framing: "extremely robust integration plan that ensures that no inadvertent side-doors or legacy paths remain. This will take detailed review and planning, don't short cut it."

## Surface area

### What changes

- **New interfaces** under `cli/internal/bead/`: `BeadInitializer`, `BeadReader`, `BeadLifecycle`, `BeadEventReader/Writer`, `BeadQueries`, `BeadDependency{Reader,Writer}`, `BeadArchive`, `BeadInterchange{Reader,Writer}`, the composite `Backend`, and the convenience `ReadOnlyBackend`. Plus parallel `LifecycleSubscriber` and `IDGenerator`.
- **New types**: `Operation` interface + ~20 named ops + `MutateFunc` escape hatch.
- **New functions/types**: `ValidateID`, `RandomHexIDGenerator`, `SequentialIDGenerator`, sentinel errors, ctx typed accessors.
- **New `*Store.Apply`** method routing through `WithLock + read + op.Apply + write`.
- **`*Store`'s concrete methods** internally rewritten to delegate to `Apply` with the appropriate `Operation` (so the optimization path lives only in `JSONLBackend.Apply`).
- **New helper packages** under `cli/internal/bead/ops/{claim,cancel,cooldown,lifecycle,queue}/`.
- **`*WatcherHub.SubscribeLifecycle` signature** gains a `ctx` parameter; one caller (`cli/internal/server/graphql/resolver_sub_bead.go`) is updated.
- **`RawBackend` docstring** updated with warning.

### What does NOT change

- `*Store`'s public method set (still 69 concrete methods). Callers that hold `*Store` concretely keep compiling unchanged.
- The on-disk format of `.ddx/beads.jsonl`, `.ddx/beads-archive.jsonl`, `.ddx/attachments/`, or any sidecar (heartbeat lease file, etc.).
- `cli/cmd/`, `cli/internal/server/`, `cli/internal/agent/` callers (no signature changes propagated outward).
- Existing test fixtures (continue to pass; new conformance tests added separately).

## Side-door catalog

Identified side-doors, each with detection + closure + ongoing enforcement.

### S1. Direct `*Store` field types in callers — workflow methods bypass helpers

**What:** 27+ files under `cli/cmd/`, `cli/internal/agent/`, `cli/internal/server/`, `cli/internal/escalation/`, `cli/internal/agentmetrics/`, `cli/internal/processmetrics/`, `cli/internal/exec/` hold `*bead.Store` as a struct field or local variable. They can call any of the 69 concrete methods, including the workflow-shaped ones (`Heartbeat`, `Claim`, `TransitionLifecycle`, `RequestCancel`, `SetExecutionCooldown`, …) that the design says should flow through helpers + `Apply`.

**Why it matters:** If callers bypass the helpers and call `*Store.Heartbeat(id)` directly, the optimization path (sidecar-write-only) only fires when `*Store.Heartbeat` itself routes through `Apply(SetClaimHeartbeat{})`. Without that internal rewrite, the concrete method and the helper take DIFFERENT paths, and the helper's optimization (or, later, AxonStore's native SQL path) is never reached for legacy callers.

**Closure (in the gating bead `ddx-c6317784`):**

- **Mandatory: `*Store`'s concrete workflow methods are rewritten to delegate to `*Store.Apply` with a named `Operation`.** This is AC #6 of the gating bead, verified by `TestStore_HeartbeatRoutesThroughApply` (and equivalent routing tests for `Claim`/`Unclaim`/`RequestCancel`/`SetExecutionCooldown`/etc.). After this, the concrete method and the helper take the SAME path; the optimization fires for both.
- `*Store.Apply` type-asserts the wrapped `RawBackend` to the optional `OperationApplier` interface and delegates if available; otherwise generic load-mutate-save. `JSONLBackend` implements `OperationApplier` to host the sidecar-write optimization. See design note §"How Apply flows through *Store over RawBackend."
- Compile-time assertion: `var _ Backend = (*Store)(nil)` + per-sub-interface assertions confirm `*Store` satisfies the new contract.

**Ongoing enforcement:**

- **Lint rule (new):** `cli/tools/lints/concrete-store-methods` — flags new files in `cli/` that hold `*bead.Store` as a struct field. **Allowlist-only model**: every file currently using `*bead.Store` is grandfathered into the allowlist with a one-line rationale. New files outside the allowlist are errors. No warning/error gradient — either allowlisted (pass) or rejected (fail). Allowlist additions are reviewable git changes requiring explicit rationale.
- **Allowlist scale acknowledgment:** the initial allowlist will have ~60+ entries (every file currently importing `cli/internal/bead` and holding `*Store`). Maintenance overhead is acknowledged; the trade-off is real defense against caller-side regression. Reviewers screen allowlist additions for whether a narrower sub-interface dependency would work.
- **Documentation:** CLAUDE.md amendment in `cli/internal/bead/CLAUDE.md` (if exists, else create) explaining the architecture and warning against `*Store` concrete deps for new code.

**Severity:** Medium. Behavior-preserving because the concrete method rewrite is mandatory in the gating bead. The lint is about preventing regression.

---

### S2. Direct file I/O against `.ddx/beads.jsonl` outside `cli/internal/bead/`

**Identified call sites (from grep):**

- `cli/cmd/bead_doctor.go:67` — repair tool reads `.ddx/beads.jsonl` directly to detect and rewrite oversized fields. **Legitimate** — it operates on the file as data, not on bead-as-entity. This is a repair tool that fixes corruption the high-level interface can't express.
- `cli/cmd/bead.go:1550-1586` — git conflict resolver reads and writes `.ddx/beads.jsonl` directly. **Legitimate** — it's resolving merge conflicts at the file level, which is below the interface.
- `cli/cmd/bead.go:80-127` — `beadAutoCommit*` knows the path for git auto-commit. **Legitimate** — git ops, not bead-as-entity.
- `cli/cmd/sync.go:20-251` — `ddx sync` knows the path because it's the file being synced. **Legitimate** — sync tool, file-level operation.
- `cli/internal/agent/execute_bead_dangling_success.go:159` — reads `beadsPath` for dangling-success detection. **Likely legitimate but verify** — if this is reading bead state for inspection, it should go through `BeadReader.Get` or `BeadReader.ReadAll`. If it's specifically inspecting the file for repair, it's legitimate.

**Why it matters:** Direct file I/O outside `bead/` encodes the file format in another package. If `.ddx/beads.jsonl`'s format changes (e.g., new field, schema version bump), every direct-IO caller needs to update independently. Worse: a new direct-IO caller could be added without anyone noticing.

**Closure (in the gating bead `ddx-c6317784`):**

- **Audit each identified call site.** For each: confirm it's legitimately operating on the file as a file (doctor, sync, git ops), not on bead-as-entity. If any caller is doing entity-level work via direct I/O, migrate it to use the interface in this bead. Specifically: verify `execute_bead_dangling_success.go:159` — if it's reading bead records, migrate.
- **Document the exception list.** Add a section to `cli/internal/bead/README.md` (create if missing) listing the legitimate exceptions: `bead_doctor.go`, `bead.go` (git ops + conflict resolution), `sync.go`. Each exception documented with rationale.

**Ongoing enforcement:**

- **Lint rule (new):** `cli/tools/lints/direct-bead-jsonl-io` — flags `os.ReadFile`/`os.WriteFile`/`os.OpenFile`/`os.Create` calls against paths containing `beads.jsonl` or `beads-archive.jsonl` outside the allowlist. Allowlist maintained in the lint config; new additions require explicit waiver with rationale.
- **CI gate:** the lint runs on every PR.

**Severity:** High. Format changes silently break direct-IO callers; new direct-IO is hard to detect without the lint.

---

### S3. `RawBackend` retained — new backends could pick the wrong shape

**What:** `RawBackend` (whole-corpus `Init/ReadAll/WriteAll/WithLock`) stays in place for `JSONLBackend` and `ExternalBackend`. The design says new backends should implement `Backend` directly. But the type is still exported; a new contributor could implement `RawBackend` thinking it's the path forward, and inherit the whole-corpus problem (the catastrophic Axon path the audit identified).

**Why it matters:** The whole-corpus shape is fundamentally wrong for per-row stores (Postgres). Recreating the same mistake for a different backend means the same months of latent dead-code path the original audit found.

**Closure (in the gating bead `ddx-c6317784`):**

- **Docstring warning on `RawBackend`** (AC #10): explicit text saying new backends should not implement `RawBackend`.

**Ongoing enforcement:**

- **Lint rule (new):** `cli/tools/lints/no-new-rawbackend-impls` — flags new types implementing `bead.RawBackend` outside the allowlist (`JSONLBackend`, `ExternalBackend`). Allowlist maintained in the lint config; net-new additions fail CI.
- **Code review checklist:** add to the project's code-review template a note: "Are you implementing `RawBackend` for a new backend? If yes, are you SURE? See `docs/helix/02-design/plan-2026-05-11-bead-backend-interfaces.md` §RawBackend."

**Severity:** Medium. The warning + lint should be sufficient; it's a contributor-discipline issue.

---

### S4. `Operation` catalog drift — new ops added without backend coverage

**What:** When a new `Operation` type is added to `cli/internal/bead/operation.go`, the generic backend fallback (`Get + op.Apply + Save`) always works. But optimized backends (eventually AxonStore) need an explicit type-switch case to take the fast path. Without that case, the new op silently falls through to the generic path and pays the latency cost.

**Why it matters:** Operations are intended to be the unit of optimization. Forgetting a case means the optimization door is closed for that op until someone notices. This is latent — there's no test failure, just degraded performance.

**Closure (in `ddx-c6317784` for the test scaffolding; AxonStore-side coverage in `ddx-9c5bca8f`):**

- **Test scaffolding (gating bead AC #11):** `TestOperationCatalog_*` enumerates `Operation` types via reflection. Produces the canonical list. Used in downstream beads for coverage assertion.
- **In AxonStore bead (`ddx-9c5bca8f`):** add `TestOperationCatalog_AxonStoreSwitchCoverage` that asserts every operation type in the catalog has a corresponding case in `AxonStore.Apply`. Falling through to the default path is acceptable only if explicitly documented (an annotation on the op type, or a per-op opt-out in the test).

**Ongoing enforcement:**

- **CI gate (downstream):** the catalog-coverage test runs on every PR. New op without optimization = test failure; either add the SQL path or annotate "generic fallback acceptable" with rationale.

**Severity:** Low. The generic fallback is correct; this is about discovering missed optimizations.

---

### S5. `*WatcherHub.NewStore` constructs its own backend

**What:** `cli/internal/bead/watcher.go:54-66` — `WatcherHub.SubscribeLifecycle` internally constructs a `*Store` via `NewStore(projectID + "/.ddx")`. This bypasses any factory-level configuration (capability validation, backend selection). Today it always produces a JSONL-backed `*Store`; when Axon ships, the watcher might use a different backend than the rest of the system.

**Why it matters:** A multi-tenant Databricks deployment would route bead state through Axon (Postgres). If the watcher constructs its own `*Store` (which is JSONL-only), it polls a stale or empty local file while the actual bead state lives in Lakebase.

**Closure (deferred to a follow-up bead, not the gating bead):**

- **In the gating bead (`ddx-c6317784`):** `WatcherHub.SubscribeLifecycle` signature is updated to take `ctx context.Context` as first parameter (AC #8). The interior `NewStore` call is not changed.
- **In a follow-up bead (new, file as part of integration cleanup):** `WatcherHub` is rewritten to take a `bead.BeadReader` (or `Backend`) dependency at construction, instead of building its own `*Store`. Callers passing the watcher pass the project's configured backend.
  - **Bead title:** "WatcherHub takes BeadReader at construction, removes internal NewStore"
  - **AC:** `WatcherHub` constructor signature changes; one call site (`cli/internal/server/server.go`) is updated; tests parameterized over backend.

**Ongoing enforcement:**

- After the follow-up bead lands, lint rule extension: `cli/tools/lints/no-internal-store-construction` flags `bead.NewStore(...)` calls outside the test packages and the configured factory.

**Severity:** High after AxonStore ships, but low today (everything is JSONL). **`ddx-900a8d38` (S5 follow-up) is a HARD PREREQUISITE for AxonStore production deployment**, not for AxonStore implementation. AxonStore can be implemented and conformance-tested with the existing WatcherHub; but before any production deployment selects `bead.backend: axon`, the WatcherHub must be wired through the configured backend factory. This sequencing is recorded in `ddx-9c5bca8f`'s open questions and rolls up into the AxonStore production-readiness checklist.

---

### S6. `*Store` constructed directly in tests, bypassing factory + capability validation

**What:** Tests under `cli/internal/bead/*_test.go` (and likely elsewhere) construct `*Store` via `NewStore(...)` for fixture setup. This bypasses the factory's capability validation path. Tests pass because they're testing `*Store` directly; production callers go through a factory that enforces capability bundles; the test path and production path diverge.

**Why it matters:** A backend that fails to declare a capability would pass tests (which use the concrete type directly) but fail in production (where the factory rejects it). This is the classic mock/prod divergence problem.

**Closure (gating bead `ddx-c6317784`):**

- **AC #12 explicitly states "backwards compatibility verification"** — every existing test continues to pass without modification. Direct `NewStore(...)` in tests is acceptable.
- **No closure required for tests that test `*Store` internals.** A test that wants to verify `*Store.Heartbeat` calls the right internal Apply path uses the concrete type directly. Legitimate.

**Ongoing enforcement (deferred):**

- Once AxonStore lands, conformance tests run against BOTH backends. A test that ONLY tests `*Store` directly is fine; a test that purports to validate cross-backend behavior must use the conformance suite.
- Documentation in `cli/internal/bead/README.md`: "Tests testing storage *contracts* use the parameterized conformance suite. Tests testing JSONL-internal implementation details use `NewStore` directly."

**Severity:** Low. Acknowledged as part of the integration; documentation captures intent.

---

### S7. `DDX_AXON_EXPERIMENTAL` env var continues to exist

**What:** `cli/internal/bead/axon_backend.go:37-39` declares `AxonExperimentalEnv = "DDX_AXON_EXPERIMENTAL"`. Per the file docs: "retained for compatibility with older tests and tooling, but it no longer gates store selection." The env var is legacy scaffolding.

**Why it matters:** Legacy env vars create confusion. Someone reading the codebase might think setting this env var enables Axon (it doesn't anymore). They might write scripts that depend on it.

**Closure (in the AxonStore implementation bead `ddx-9c5bca8f`, NOT the gating bead):**

- **Remove `AxonExperimentalEnv` constant and all its references.** AxonStore is selected via config (`bead.backend: axon`) and capability-validated at the factory. The env var path is dead.
- **Grep audit:** `grep -rn "DDX_AXON_EXPERIMENTAL\|AxonExperimentalEnv" cli/` returns zero hits after removal.

**Ongoing enforcement:**

- After removal, no specific lint needed — the symbol doesn't exist.

**Severity:** Low. Cosmetic but worth resolving.

---

### S8. Operation type registration discoverability

**What:** Named `Operation` types live in `cli/internal/bead/operation.go`. New ops added without documentation could be missed by:

- Code reviewers (forget to add a backend type-switch case for AxonStore)
- Helpers (a workflow that should use the new op doesn't know it exists)
- Documentation generators (the website's auto-gen catalog of ops is stale)

**Why it matters:** Ops are the unit of optimization. Forgetting one closes the optimization door for that workflow.

**Closure:**

- **Package doc on `cli/internal/bead/operation.go`** lists every named op + its semantic + which workflows use it. Maintained as the canonical catalog.
- **`go doc bead` produces the catalog.** Reviewable.
- **`TestOperationCatalog_AllNamedOpsImplementApply`** (gating bead AC #2) enumerates via reflection — catches typos in op declarations.

**Ongoing enforcement:**

- Code review checklist: "Did you add a new `Operation` type? Did you add it to the package doc, the catalog test, and (if optimized) the backend type-switches?"

**Severity:** Low. Catalog discipline is a process concern.

---

### S9. `context.Context` value bypass — callers stuffing arbitrary state on ctx

**What:** Once interfaces take `context.Context`, callers may be tempted to pass per-call options ("force refresh", "include archived") via `ctx.WithValue` instead of method arguments. Backends would have to look up untyped values. Classic god-object pattern.

**Why it matters:** The Operation pattern + method arguments are the canonical way to express call intent. Stuffing options on ctx undermines the interface and introduces hidden coupling.

**Closure (gating bead `ddx-c6317784`):**

- **Typed accessors required** (AC #5): `WithIdentity`, `WithTrace` + their `FromContext` counterparts. No untyped string keys in `bead` package.
- **Package docstring** in `cli/internal/bead/context.go` enumerates allowed ctx values explicitly and lists anti-patterns.

**Ongoing enforcement:**

- **Lint rule (new):** `cli/tools/lints/typed-context-accessors-only` — flags `ctx.Value(...)` calls within `cli/internal/bead/` that don't go through a typed accessor. Errors on non-allowlisted calls.
- **Documentation:** `cli/internal/bead/context.go` package docstring is normative.

**Severity:** Medium. The lint catches the most common violation; review catches the rest.

---

## Legacy path catalog

Existing code that should be deprecated or removed once the new architecture proves out.

### L1. `AxonBackend` (whole-corpus path)

**Status:** Live, default-not-wired. Used only as JSONL-fallback emulation.

**Deprecation:** Mark `AxonBackend` as deprecated when `AxonStore` (the new per-row implementation) lands and passes conformance.

**Removal:** After 1 release cycle of deprecation, remove `AxonBackend` and its associated `axon_backend_test.go`. The new `AxonStore` covers all functionality with the right granularity.

**Timeline:** Removal happens in a bead AFTER `ddx-9c5bca8f` (AxonStore impl) lands. New bead to file: "Remove deprecated AxonBackend whole-corpus path."

**Acceptance criteria for safe removal:**

1. `AxonStore` passes the parameterized conformance suite.
2. No callers reference `bead.AxonBackend` or `bead.NewAxonBackend`.
3. `axon_backend.go` is deleted; `axon_backend_test.go` is deleted.
4. `axon_backend` types (`AxonGraphQLTransport`, `AxonBackend`, `AxonExperimentalEnv`, `AxonBeadsCollection`, etc.) are removed.
5. `cd cli && go test ./...` is green.

---

### L2. Workflow methods on `*Store` (`Heartbeat`, `Claim`, `TransitionLifecycle`, etc.)

**Status:** Live. Concrete methods on `*Store` that implement workflow logic. Per the design, internally rewritten in `ddx-c6317784` to delegate to `Apply` + Operation; but the public methods stay for backwards compat.

**Deprecation:** Long-term, callers should use the helper packages (`bead/ops/claim`, `bead/ops/cancel`, etc.) instead of calling `*Store.Heartbeat` etc. directly. But this is OPTIONAL — the concrete methods continue to work.

**Removal:** Not planned. The concrete methods are part of `*Store`'s API; removing them would break callers. The integration plan does NOT remove them.

**Documentation:** `cli/internal/bead/store.go` adds a comment to each workflow method: "Convenience wrapper for callers that don't want to construct an Operation. Equivalent to `s.Apply(ctx, id, bead.SetClaimHeartbeat{At: time.Now()})`. New code should prefer the `ops/<concern>/` helpers."

**Ongoing enforcement:** None. The concrete methods are stable API.

---

### L3. `RawBackend` interface (long-term)

**Status:** Live. JSONLBackend and ExternalBackend implement it. The composition path (`*Store` over `RawBackend`) is the production default for JSONL.

**Deprecation:** Not deprecated. Keeps working for JSONL-shaped backends.

**Removal:** Not planned. Removing `RawBackend` would require rewriting JSONLBackend and ExternalBackend to implement `Backend` directly. Possible but not in scope.

**Documentation:** Already covered by the warning docstring (S3).

**Ongoing enforcement:** S3 lint rule prevents new implementations.

---

### L4. `DDX_AXON_EXPERIMENTAL` env var

**Status:** Live but no-ops. Documented as legacy.

**Removal:** In `ddx-9c5bca8f` (AxonStore impl bead). See S7.

---

### L5. `MigrateToAxon` writing JSONL (per the audit's gap 6)

**Status:** Live. `cli/internal/bead/migrate.go:635-686` — `MigrateToAxon` exists but writes JSONL files under `.ddx/axon/`, not Postgres. The "migration" produces JSONL, not the actual destination format.

**Deprecation:** Mark deprecated when the real JSONL→Postgres importer ships.

**Removal:** After the new importer lands and is verified. New bead to file: "Replace MigrateToAxon JSONL-writer with real Postgres importer."

**Acceptance criteria for safe removal:**

1. New `MigrateToAxon` (or similar) writes to Postgres via the AxonStore Apply path.
2. The old JSONL-writing `MigrateToAxon` is renamed `migrateToAxonJSONLLegacy` and marked deprecated.
3. After one release: removed entirely.

---

### L6. Lifecycle schema marker methods (`HasLifecycleSchemaMarker`, `WriteLifecycleSchemaMarker`, `LifecycleSchemaMarkerPath`)

**Status:** Live. JSONL-specific bootstrap for schema versioning. Used during lifecycle migration.

**Deprecation:** When AxonStore handles schema versioning differently (Lakebase migrations), these JSONL-specific methods stay JSONL-only and are not promoted to any interface.

**Removal:** Not planned for the foreseeable future. JSONL is still a supported backend.

---

### L7. Concrete-method-only `*Store` extras (`MigrateLifecycle`, `MigrateFromHelix`, `ReconcileLifecycleMetadata`, etc.)

**Status:** Live. Admin/one-time operations. Stay concrete on `*Store`, not on any interface.

**Deprecation:** Each method evaluated independently. `MigrateFromHelix` may be one-time-only and removable after a release. `ReconcileLifecycleMetadata` may be ongoing operator-tooling.

**Removal:** Case-by-case, separately tracked. Not part of this integration plan.

---

## Integrity mechanisms

Three layers of integrity: compile-time (Go's type system), test-time (CI tests), and process-time (review checklists and lints).

### Compile-time

| Check | Mechanism | Caught |
|---|---|---|
| `*Store` satisfies `Backend` | `var _ Backend = (*Store)(nil)` | Missing methods after refactor |
| `*Store` satisfies each sub-interface | `var _ BeadReader = (*Store)(nil)` etc. | Sub-interface drift |
| `*WatcherHub` satisfies `LifecycleSubscriber` | `var _ LifecycleSubscriber = (*WatcherHub)(nil)` | Subscription contract drift |
| Operation types implement `Operation` | Each op-struct has explicit `Apply(*Bead) error` method (Go's struct-method-on-named-type) | Typos / missing implementations |
| Sentinel errors are non-nil | `var ErrNotFound = errors.New(...)` form | Compile-time guarantees init |

### Test-time

| Test | Catches |
|---|---|
| `TestBackendConformance_*` (parameterized) | Behavior drift between backends — all backends must produce equivalent results under the same input |
| `TestOperation_AllNamedOpsImplementApply` (reflection) | Op declared without `Apply` method |
| `TestOperation_ClaimOp_RejectsConflict` | CAS semantics on Claim |
| `TestOperation_UnclaimOp_RequireOwner` | Ownership check on Unclaim |
| `TestJSONLBackend_Apply_SetClaimHeartbeat_UsesSidecar` | Sidecar optimization preserved under new pattern |
| `TestValidateID_*` (4 tests) | ID contract enforcement |
| `TestSentinelErrors_AreErrorValues` | Sentinels are distinct, errors.Is-discriminable |
| `TestOpsClaim_*`, `TestOpsCancel_*`, `TestOpsCooldown_*`, `TestOpsLifecycle_*`, `TestOpsQueue_*` | Helper-package correctness against `*Store` |
| `TestOperationCatalog_AxonStoreSwitchCoverage` (downstream bead) | AxonStore type-switches every named op or explicitly opts out |

### Process-time (lint rules and review checklists)

**Status note**: the lint suite is a **HARD PREREQUISITE** for "no side-doors remain" — several side-doors (S2 file I/O, S3 RawBackend, S5+S6 internal `NewStore`, S9 ctx values) have lint rules as their PRIMARY closure. Without the lint suite, those side-doors are open and the integration is incomplete. The lint suite is filed as `ddx-e91a45c0` and runs in parallel with the gating bead. The "Layer 3" framing below describes the architecture; it does not imply the layer is optional.

| Lint rule | Location | Catches |
|---|---|---|
| `direct-bead-jsonl-io` | `cli/tools/lints/` | New `os.ReadFile`/`WriteFile` against `beads.jsonl` outside allowlist |
| `no-new-rawbackend-impls` | `cli/tools/lints/` | New types implementing `RawBackend` outside allowlist |
| `typed-context-accessors-only` | `cli/tools/lints/` | Untyped `ctx.Value` in `bead/` |
| `concrete-store-methods` | `cli/tools/lints/` | New `*bead.Store` field type in fresh callers |
| `no-internal-store-construction` (after WatcherHub fix) | `cli/tools/lints/` | New `bead.NewStore(...)` calls in production code |

Each lint:
- Implemented as a Go analyzer (`golang.org/x/tools/go/analysis`).
- Runs as part of `lefthook` pre-commit AND CI's `make lint` step.
- Has a maintained allowlist (YAML or Go-constant) with explicit waiver rationale for each grandfathered entry.
- New violations fail the build; allowlist additions require code review.

Review checklist additions (`docs/helix/06-iterate/code-review-checklist.md` — create if missing):

- [ ] New caller of bead state — does it use the narrowest sub-interface (ISP), or hold `*Store` concretely? If concrete, why?
- [ ] New `Operation` type added — is it in the package doc? Is the catalog test still passing? If a fast-path backend exists, is the type-switch updated?
- [ ] New direct file I/O against `.ddx/`? If yes, is it in the allowlist? Was the lint waiver requested?
- [ ] New backend implementation — does it implement `Backend` directly (not `RawBackend`)? Does it pass the conformance suite?

### CI gates

`.github/workflows/ci.yml` is amended to add:

```yaml
- name: Bead interface integrity gates
  run: |
    cd cli
    go test ./internal/bead/... -run 'TestBackendConformance|TestOperation|TestValidateID|TestSentinelErrors'
    go vet -vettool=$(which bead-lints) ./...   # custom analyzer composing the 5 lints
```

The "bead-lints" tool is a small wrapper around the 5 analyzers above. Built as part of `cli/tools/lints/Makefile` target. Installed in CI.

## Process governance

### G1. Architecture Decision Record (ADR)

File a new ADR at `docs/helix/02-design/adr/ADR-027-bead-backend-interface-architecture.md` (next free number per `ls docs/helix/02-design/adr/`) capturing:

- The decision (11 sub-interfaces + Operation pattern + LifecycleSubscriber + IDGenerator).
- The forces (LSP, SRP, storage-vs-workflow separation, Axon production-readiness, read-only deployment shape).
- The consequences (caller migration is opportunistic; backwards compat preserved; the optimization door opens).
- Cross-reference to the design note and this integration plan.

ADR filed as part of bead `ddx-c6317784` (or as a separate one-line bead if AC bloat is a concern).

### G2. CLAUDE.md amendments

- **`/CLAUDE.md`** (project root): brief paragraph in the Architecture section pointing at the design note and integration plan.
- **`cli/internal/bead/CLAUDE.md`** (create if missing): short subdirectory guidance —
  - Storage primitives live in `cli/internal/bead/` and on `Backend` sub-interfaces.
  - Workflow operations are typed `Operation` values + helpers in `cli/internal/bead/ops/<concern>/`.
  - New code uses the narrowest sub-interface (ISP); existing `*Store` concrete callers are grandfathered.
  - Pluggable `IDGenerator`; `bead.ValidateID` is the canonical contract.
  - Direct file I/O against `.ddx/beads.jsonl` is restricted to the documented allowlist (bead_doctor, sync, git ops, conflict resolver).

### G3. Lefthook pre-commit hook for the lints

Amend `lefthook.yml`:

```yaml
pre-commit:
  commands:
    bead-lints:
      glob: "{cli/**/*.go,**/*.ddx.yml}"
      run: cli/tools/lints/run.sh
```

The lints run on staged Go files; violations block commits unless explicitly waived.

### G4. Documentation: `cli/internal/bead/README.md`

Create a README.md in the bead package that captures:

- Architecture overview (link to design note).
- Sub-interface taxonomy (table).
- Operation pattern with a small example.
- Helper packages with one-line descriptions.
- Allowlists (direct file I/O exceptions, RawBackend implementations).
- Test discipline (when to use conformance suite, when to use concrete `*Store`).

## Caller-pattern audit (initial)

Initial classification of bead-importing files. Done as part of the gating bead's review pass.

| Package | Files importing bead | Current pattern | Migration intent |
|---------|---|---|---|
| `cli/cmd/` | 16 | Hold `*Store` concretely; use full method surface | Grandfathered. New cmd/ files use narrowest sub-interface. |
| `cli/internal/agent/` | 21 | Hold `*Store` concretely; use lifecycle/claim/event methods | Grandfathered. Future: opportunistic narrowing to `BeadLifecycle + BeadEventWriter` where applicable. |
| `cli/internal/server/graphql/` | 8 | Hold `*Store` concretely; mix of read + write resolvers | Grandfathered. Read-side resolvers (e.g. `resolver_beads.go`) are good candidates for narrowing to `BeadQueries` + `BeadReader`. Write-side resolvers use full Backend. |
| `cli/internal/server/` | 3 | `server.go` constructs *Store; `workers.go` uses it | Grandfathered. The `*WatcherHub` constructor stays as-is until S5's follow-up bead. |
| `cli/internal/escalation/` | 3 | Specific lifecycle + cooldown methods | Grandfathered. Future: opportunistic. |
| `cli/internal/exec/` | 2 | `bead_runtime.go`, `store.go` — hold `*Store` for exec-definition lookup; use `Create`, `Get`, `List` | Grandfathered. Could narrow to `BeadReader + BeadLifecycle` later. |
| `cli/internal/bead/axon/` | 2 | Axon GraphQL client subscription transport (generated.go + subscription.go) | Stays; the new AxonStore will be built alongside in `ddx-9c5bca8f`. |
| `cli/internal/agent/try/` | 2 | `attempt.go`, `conflict_recovery.go` — try-attempt path; uses `Create`, `Get`, `Ready`, `Claim`, lifecycle methods | Grandfathered. Mixed read+write; full Backend appropriate. |
| `cli/internal/processmetrics/`, `cli/internal/attemptmetrics/`, `cli/internal/agentmetrics/` | 3 | metrics loaders; primarily `ReadAll` for scanning | Grandfathered. Strong narrowing candidates → `BeadReader`. |
| `cli/internal/escalation/` | 3 | escalation flow; uses cooldown + lifecycle | Grandfathered. Could narrow once helpers exist. |

Detailed audit happens as the gating bead implementation reads each file. Any caller that's doing something unexpected (direct file I/O for entity reads, not on the allowlist) gets surfaced and either added to the allowlist (with rationale) or migrated.

## Rollout sequencing

```
[ddx-c6317784]  ← gating bead: declare interfaces, Operation pattern, helpers,
       │           context accessors, sentinel errors, RawBackend warning,
       │           *Store internal rewrite, *Store compile-time assertions
       │
       ├──→ [ADR + CLAUDE.md updates + cli/internal/bead/README.md]
       │     (part of gating bead, or sibling bead if scope grows)
       │
       ├──→ [WatcherHub takes BeadReader] (S5 follow-up)
       │     (file as separate bead; lower priority)
       │
       ├──→ [lint rules: 5 analyzers + Makefile + lefthook] (G3)
       │     (file as separate bead; can land in parallel with gating bead)
       │
       ├──→ [ddx-9c5bca8f]  ← AxonStore implementation
       │     (depends on gating bead; includes S7 removal of DDX_AXON_EXPERIMENTAL)
       │
       ├──→ [ddx-29f02cf4]  ← config-driven factory + capability validation
       │
       ├──→ [ddx-8bf23be0]  ← schema.graphql reconciliation
       │
       ├──→ [ddx-958b8fc3]  ← parameterized conformance against AxonStore
       │
       ├──→ [ddx-8dd19492]  ← subscription smoke (incl. native Axon if available)
       │
       ├──→ [new] schema versioning + v0→v1 migration ladder
       ├──→ [new] real JSONL→Postgres importer (replaces L5)
       ├──→ [new] real-wire Axon integration tests
       │
       └──→ [new] remove deprecated AxonBackend (L1)
             (after AxonStore is verified in production for 1 release cycle)
```

Two new beads to file as part of integration setup:

1. **"WatcherHub takes BeadReader at construction (interface-refactor S5 follow-up)"** — small bead, low priority, but tracked so it doesn't get forgotten.
2. **"Bead interface integrity: 5 lint rules + CI gate (interface-refactor S2/S3/S4/S9 enforcement)"** — moderate bead, tracked.

I'll file these now as part of the integration kickoff.

## Acceptance criteria for "no side-doors remain"

Verification that the integration completed cleanly:

1. **Compile-time:** `var _ Backend = (*Store)(nil)` and per-sub-interface assertions compile.
2. **Test-time:** the full integrity test suite is green (`TestBackendConformance_*`, `TestOperation_*`, `TestValidateID_*`, `TestJSONLBackend_Apply_SetClaimHeartbeat_UsesSidecar`, all helper-package tests).
3. **Process-time:**
   - 5 lint rules implemented and running in CI.
   - Allowlists for direct-file-IO, RawBackend impls, concrete `*Store` callers are committed in `cli/tools/lints/`.
   - CLAUDE.md amendments landed.
   - `cli/internal/bead/README.md` exists.
   - ADR filed.
   - The 2 follow-up beads (S5, lint suite) filed and tracked.
4. **No grep hits for legacy patterns in net-new code:** `grep -rn "DDX_AXON_EXPERIMENTAL"` returns nothing once AxonStore lands; `grep -rn "bead.AxonBackend"` returns nothing once L1 cleanup lands.
5. **Allowlists are not silently growing:** the lint config in `cli/tools/lints/` has Git history; reviewers can see additions over time and challenge them.

## Risks and rollback

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Gating bead lands incomplete; *Store concrete-method rewrite is missed | Low | High (optimization path silently bypassed) | AC #6 of gating bead explicitly requires the rewrite; **`TestStore_HeartbeatRoutesThroughApply`** (and equivalents for `Claim`/`Unclaim`/`RequestCancel`/`SetExecutionCooldown`) use an instrumented `RawBackend` to record `Apply` calls and assert the concrete method routes through `*Store.Apply`. Verifiable, not "review verifies." |
| Existing callers break due to context being added to interface methods | Medium | Medium (compilation errors) | Approach: parallel-wrapper. `*Store` keeps non-ctx method signatures; new ctx-aware methods are added alongside. Compile-time assertions enforce both sets coexist. |
| Lint rules are too aggressive and block legitimate code | Medium | Low (developer friction) | Allowlists with explicit waivers; gradual rollout (warnings first, errors after one release cycle); maintainer review for additions. |
| `WatcherHub` doesn't get its S5 follow-up done; AxonStore deployment finds it polling a stale JSONL file | Low | High (silent stale-data bug) | The S5 follow-up bead is filed and tracked; the AxonStore acceptance criteria require this to be resolved before AxonStore is declared production-default. |
| Operation-catalog coverage test isn't enforced for AxonStore | Low | Medium (latent slow ops) | The downstream bead `ddx-9c5bca8f` explicitly requires it; reviewers check. |
| Direct file I/O slipped past the audit | Medium | Medium (format-change risk) | The `direct-bead-jsonl-io` lint runs on every PR; new violations require explicit allowlist additions reviewed by maintainers. |
| Helper package logic drifts from `*Store` concrete-method behavior | Medium | Medium | Helper-package tests + conformance suite verify behavior equivalence. |
| Caller migration to interfaces never happens; concrete `*Store` deps accumulate over time | High | Low (optimization door stays open but ISP benefits unrealized) | Acceptable. The lint warns on new callers. Migration is opportunistic and not on the critical path. |

**Rollback strategy:** the gating bead's changes are entirely additive — new interfaces, new types, new helpers, new tests. Reverting the bead's commit restores the old `*Store`-concrete-only world without breaking any caller (because callers don't use the new interfaces yet). The `*Store` internal rewrite (delegating concrete methods to `Apply`) is the only intrusive change; if it breaks something, revert the per-method-rewrite portion while keeping the interface declarations. The lint rules can be temporarily disabled via CI workflow tweaks if they cause unforeseen friction.

## Open questions

1. **Should the ADR be a separate bead or part of the gating bead?** Recommend separate (so ADR review can happen in parallel with implementation review). File as: "ADR: bead backend interface architecture (integration plan governance)".

2. **Lint rules: which language?** Go analyzers (`golang.org/x/tools/go/analysis`) are the right choice — type-aware, integrate with `go vet`. Custom lints in another language (semgrep, etc.) would work but add tooling. Recommend Go analyzers.

3. **Helper package import paths: `cli/internal/bead/ops/claim` vs `cli/internal/beadops/claim`?** Locating under `bead/` makes the relationship clear; locating under a sibling `beadops/` makes the workflow-vs-storage separation more visible. Recommend `cli/internal/bead/ops/<concern>/` (operator already approved this in prior turn).

4. **`make lint` integration vs. separate `make bead-lint` target?** Recommend integrate into `make lint` so it always runs alongside `golangci-lint`. Single command for the developer; CI runs one step.

5. **Backwards-compat duration for L2 (concrete workflow methods on `*Store`)?** The plan says "not planned for removal." Confirm: do we ever envision removing `*Store.Heartbeat` etc., or are they permanent API? Recommend permanent — they're useful convenience, and the helpers exist alongside.

6. **Should `cli/internal/bead/CLAUDE.md` exist today, or wait for the bead to create it?** Recommend create it as part of `ddx-c6317784` — short doc, captures the architecture for future agent runs.

## Bottom line

This integration plan catalogs **9 side-doors**, **7 legacy paths**, **5 lint rules** for ongoing enforcement, **3 layers of integrity** (compile/test/process), and **4 governance artifacts** (ADR, CLAUDE.md, README, lefthook). It identifies **2 follow-up beads** to file alongside the gating bead.

The integration is designed to be:
- **Robust** — three independent integrity layers; any single bypass is caught by at least one other.
- **Discoverable** — README + CLAUDE.md + ADR; new contributors find the architecture before they accidentally violate it.
- **Auditable** — every allowlist entry has a rationale; every lint waiver has a commit-message-level justification.
- **Reversible** — gating bead is additive; lint rules are switch-offable; rollback is possible at each stage.

No side-doors. No legacy paths.

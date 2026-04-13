---
ddx:
  id: SD-019
  depends_on:
    - FEAT-002
    - FEAT-013
    - FEAT-004
    - FEAT-006
    - FEAT-008
    - FEAT-012
    - FEAT-020
    - SD-004
    - SD-006
    - SD-012
    - SD-013
---
# Solution Design: Host+User Multi-Project Server Topology

## Purpose

Define the `ddx server` topology that runs as a per-user host daemon and
hosts multiple project roots on one machine without collapsing back to a
single-repo assumption. One `ddx-server` process serves one operating-system
user on one machine, holds its identity and project registry in user-level
state (FEAT-020), and supervises execute-loop workers for every registered
project. The server remains local-first and git-native. The contract adds an
explicit registry, request scoping, per-project isolation, an in-process
worker boundary, and backward compatibility for today's single-project
invocation.

## Scope

- Project registry and discovery
- Request routing and project selection
- Per-project adapter, cache, and failure isolation
- Project-scoped worktree and worker-pool boundaries
- Migration semantics from the current single-project server

Out of scope:

- Cluster orchestration or remote machine provisioning
- Cross-machine storage backends
- Automatic merge/conflict resolution policy
- New authority for workflow policy beyond the explicit project context

## Project Registry And Discovery

`ddx server` holds one project registry per host+user. The registry is
persisted at user-level state (not inside any project) and projects enter it
through a mix of server startup, CLI auto-registration, and optional config
seed entries. FEAT-020 is the authoritative spec for the on-disk shape and
auto-registration flow; SD-019 describes how the server uses that registry to
route requests and isolate projects.

### Storage Location

Runtime state is host+user scoped:

```
~/.local/share/ddx/            (XDG_DATA_HOME/ddx if set)
  server-state.json            node identity + project registry
  server.addr                  last-known server URL + PID
```

State that describes the server instance never lives inside one project's
`.ddx/` directory. The addr file and state file both belong to the operator
(the user running the server), not to any project. One server per machine:
the addr file is a single-entry file; a new server start overwrites it.

### Registry Population

Three paths contribute entries:

1. **Server startup** registers the project at the server's own working
   directory.
2. **CLI auto-registration** — `ddx agent`, `ddx bead`, and `ddx doc`
   subcommand groups fire a fire-and-forget `POST /api/projects/register` in
   `PersistentPreRunE` so any project the user touches on the machine shows
   up in the host+user registry without manual configuration (FEAT-020).
3. **Optional config seed** — a `server.projects` block in config may list
   known roots so a fresh install can boot with a non-empty registry before
   any CLI command runs. Seed entries are merged into the persisted state,
   not kept as a separate source of truth.

Each registry entry carries:

- `id` — stable project key used in URLs, MCP calls, and caches
- `name` — optional display text for UI labels
- `path` — the absolute filesystem path for the project
- `git_remote` — captured when available
- `registered_at` / `last_seen` timestamps

If the state file is absent and no CLI has registered, the server synthesizes
a singleton registry from the current working directory so today's `ddx server`
invocation still works. If an `id` is omitted, the server derives one from the
root basename plus a short path hash and keeps it stable for the lifetime of
the registry.

### Discovery Contract

The server publishes project metadata through the same local HTTP and MCP
surface it uses for other DDx resources:

- `GET /api/projects` lists all registered projects and their health
- `GET /api/projects/:project` shows one project context
- `ddx_list_projects` enumerates registry entries for MCP clients
- `ddx_show_project` resolves one project context for MCP clients

The registry payload includes:

- `id`
- `root`
- `name`
- `is_default`
- `status` (`ready`, `degraded`, or `error`)
- an optional `error` summary when the project cannot be loaded

The server must never merge project state into one anonymous host-level view.
Every request is resolved against one project context before any adapter runs.

## Request Routing And Selection

The canonical routing model is project-scoped. The project selection step
happens before the feature-specific handler sees the request.

### HTTP Routing

The canonical HTTP shape is:

- `/api/projects` for registry operations
- `/api/projects/:project/...` for project-scoped resources
- `/projects/:project/...` for the web UI

Legacy unscoped `/api/...` routes remain as compatibility aliases only when
the request can resolve to a single project context. They should not become
the long-term multi-project contract.

Request selection precedence:

1. explicit project in the path
2. explicit project supplied by the caller
3. configured default project
4. implicit singleton project derived from the current working directory

If multiple projects are registered and no selection is present, the server
returns a project-selection error instead of guessing.

### MCP Routing

MCP tools take the same explicit project context. The selector is part of the
tool request, not hidden in process-global state.

- `ddx_list_projects` and `ddx_show_project` operate on the registry
- project-aware tools resolve their data through the selected project context
- singleton mode may omit the selector for backward compatibility
- multi-project mode requires the selector unless the tool is explicitly
  registry-wide

### UI Routing

The web UI mirrors the same scoping model:

- `/projects` shows the project picker/registry when more than one root exists
- `/projects/:project/dashboard` is the selected-project entry point
- the root `/` redirects to the default project when one exists
- if multiple projects exist and no default is configured, the UI lands on the
  picker instead of picking a project implicitly

The browser should preserve the selected project in the URL so direct links
remain unambiguous.

## Per-Project Isolation

Each project gets its own adapter set, cache namespace, and failure boundary.
One broken project must not poison sibling projects.

### Adapter Boundaries

Every project context owns its own instances of:

- document library adapter
- bead store adapter
- doc-graph adapter
- agent-session adapter
- execution read-model adapter
- worktree/workspace adapter

Adapters are never shared across project roots. They may share implementation
code, but their runtime state is keyed by project id and absolute root path.

### Cache Boundaries

Transient server caches are namespaced per project:

- in-memory caches use the project id or derived project key
- on-disk caches live under a per-project subdirectory in the configured cache
  root
- project-local `.ddx/` data remains authoritative for that project only

No cache entry may assume that one project's document, bead, or agent-session
state is valid for another project.

### Failure Modes

- a malformed or missing project root marks only that project degraded
- a bad config entry does not stop healthy sibling projects from serving
- duplicate project ids are a startup/configuration error
- a missing default project only affects unscoped legacy routes
- a request that targets a removed project returns a project-not-found error

Health reporting must expose the status of each project separately so one bad
project can be repaired without taking down the others.

## Worktrees And Worker Pools

The host+user `ddx-server` process hosts an in-process `WorkerManager` that
supervises execute-loop workers as goroutines. Worker lifecycle — start, live
logs, stop, and on-disk record — is owned by the server, but every worker is
scoped to one project context and the worktree and worker pools remain
project-scoped.

The first shipped execute-bead supervisor is a single-project worker running
inside the server's `WorkerManager`. Multi-project scheduling across workers
is still one-worker-per-project-context: a goroutine handling project A never
reaches into project B. Epic execution introduces a second worker type: an
epic-scoped worker that owns one persistent epic branch and worktree for that
project.

Worker records live under the project's own `.ddx/workers/` directory, and
execute-bead attempt artifacts live under `.ddx/executions/<attempt-id>/` in
the same project (FEAT-006). Host+user server state never absorbs those
files; the user-level state file only tracks node identity and the project
registry, not the replay-backed artifacts produced by a worker.

### Worktree Model

- Each project owns its own worktree base directory
- A worktree name is only unique within one project context
- A worker may only operate on one project context at a time
- Worktree cleanup and preservation rules apply per project, not globally
- Single-ticket workers use temporary execution worktrees
- Epic workers use one long-lived worktree per active epic and keep that
  worktree until the epic is merged, reset, or abandoned
- Epic worktrees are tied to epic branch names such as `ddx/epics/<epic-id>`
  and are not reused across different epics

### Worker Pools

- The server may enforce a global worker limit and a per-project worker limit
- Scheduling decisions happen within the selected project context
- One noisy project must not starve every other project when the global limit
  still has headroom
- Queue polling and claim handling remain tied to the project that owns the
  bead or execution item
- Single-ticket workers are prioritized ahead of epic workers when both are
  competing for the same project capacity
- Different epics may run in parallel in different worktrees when project and
  global worker limits allow it
- One epic worker executes child beads sequentially inside its epic worktree;
  it does not run multiple child beads from the same epic concurrently

### Epic Landing Contract

Epic execution preserves the history of child tickets on the epic branch and
lands that branch with a regular merge commit rather than a fast-forward.

- Child ticket completions are committed sequentially on the epic branch
- The branch itself is named `ddx/epics/<epic-id>` and is the durable
  integration surface for that epic's work for the lifetime of the epic
- The epic worker attaches one persistent managed worktree to the epic branch
  and does not spawn per-child worktrees
- Epic merge gates run on the merge candidate, not just on individual child
  commits
- The final integration to the target branch uses a regular merge commit
  (`git merge --no-ff`) so the full epic branch history remains visible in
  git history; epic branches are never fast-forwarded to the target branch

Child beads may be closed mid-epic as soon as their own acceptance and
required gates pass inside the epic branch context. Such a child is "closed
on epic": its commit is durable on `ddx/epics/<epic-id>` but has not yet
reached the target branch. Downstream single-ticket work that depends on the
target-branch effect of a closed-on-epic child must wait until the epic
merges.

Before the epic worker creates the final merge commit, it evaluates an
aggregate merge gate that confirms:

- every required child bead for the epic is in a closed state
- no child execution is still in flight inside the epic worktree
- no in-flight external dependencies remain for the epic
- any epic-level required executions pass against the merge candidate (the
  epic branch rebased onto the current target tip)

If any of those checks fail, the worker preserves the epic branch and
worktree for operator inspection rather than silently discarding the work or
landing a partial epic.

This topology is intentionally compatible with a later server-managed
`execute-bead` supervisor, but it does not itself define the full execution
state machine.

## Migration Semantics

Migration must preserve the current single-project local use case.

### Backward Compatibility

- `ddx server` with no project registry still serves the current working
  directory
- current local CLI usage keeps working with no new project selector required
- unscoped HTTP and MCP requests keep working when the server has only one
  visible project
- project-local data adapters continue to read and write the same repository
  artifacts they use today

### Multi-Project Adoption

- adding a second project to the registry does not require migrating file
  formats or introducing a shared database
- the new registry is additive; it changes request routing, not repository
  ownership
- legacy clients can keep using the default project while new clients opt into
  explicit project selection

### Failure Recovery

- if one project fails to initialize, the server can still serve the others
- if the registry shape is invalid, such as when duplicate project ids are
  configured, startup should fail fast before serving partial context
- if a project is removed from the registry, its cache and runtime state are
  left isolated rather than merged into another project

## Service Manager Integration

One-machine multi-project topology assumes a single long-lived `ddx server`
process per user session. That process is supervised by the host's
user-level service manager so project registry, node state, and worker
pools survive logout, crash, and reboot.

Service manager integration is **user-level only** in this phase. Both
Linux (systemd user units) and macOS (launchd user agents) are supported,
with parallel lifecycle semantics:

- **Linux (systemd user unit)**
  - unit path: `~/.config/systemd/user/ddx-server.service`
  - logs: `<workdir>/.ddx/logs/ddx-server.log` via `StandardOutput=append:`
  - env file: `<workdir>/.ddx/server.env`
  - restart: `Restart=on-failure`, `RestartSec=5`
  - install: `systemctl --user enable --now ddx-server.service`

- **macOS (launchd user agent)**
  - plist path: `~/Library/LaunchAgents/com.documentdriven.ddx-server.plist`
  - label: `com.documentdriven.ddx-server`
  - working directory: configured project root or `$HOME`
  - logs: `~/Library/Logs/ddx-server/stdout.log` and `stderr.log`
  - environment overrides: `DDX_NODE_NAME`, `DDX_DATA_HOME`, TLS cert
    paths via `EnvironmentVariables`
  - run policy: `RunAtLoad=true`, `KeepAlive=true`, `ThrottleInterval=10`
  - install/enable: `launchctl load -w <plist>`
  - disable/remove: `launchctl unload <plist>` and delete the file
  - restart: `launchctl kickstart -k gui/<uid>/com.documentdriven.ddx-server`

Node state and the project registry live in `~/.local/share/ddx/`
(or `$XDG_DATA_HOME/ddx`) on every platform, per FEAT-020, and are never
stored inside the service-manager unit directory. The address file at
`~/.local/share/ddx/server.addr` is authoritative for client discovery.

Machine-level installs (root `LaunchDaemon` under `/Library/LaunchDaemons`
or system-level systemd units) are out of scope for this phase. The full
per-platform contract is specified in FEAT-002 under *Service Manager
Integration*.

## Validation

This design should be covered by tests that verify:

- registry enumeration returns multiple project roots and reflects host+user
  state persisted at `~/.local/share/ddx/server-state.json`
- CLI auto-registration adds a previously unknown project to the registry
  through `POST /api/projects/register`
- scoped HTTP and MCP requests resolve the correct project context
- singleton fallback preserves today's `ddx server` invocation
- a broken project stays isolated from healthy sibling projects (host+user
  isolation: one bad project does not poison the rest of the host)
- concurrent requests that hit different registered projects do not collide
  on adapters, caches, or worker records
- UI routing lands on the correct project-specific route at
  `/nodes/:nodeId/projects/:projectId/...`
- execute-loop workers supervised by `WorkerManager` start, stream logs, and
  stop cleanly, producing attempt artifacts under the owning project's
  `.ddx/executions/<attempt-id>/` directory
- a worker running against one project cannot reach into another project's
  bead store, worktree, or execution artifacts

TP-002 carries the concrete test cases that implement this validation plan.

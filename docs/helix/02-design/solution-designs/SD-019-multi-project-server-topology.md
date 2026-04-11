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
    - SD-004
    - SD-006
    - SD-012
    - SD-013
---
# Solution Design: One-Machine Multi-Project Server Topology

## Purpose

Define the first-step `ddx server` topology that can host multiple project
roots on one machine without collapsing back to a single-repo assumption.
The server remains local-first and git-native. The new contract adds an
explicit registry, request scoping, per-project isolation, and backward
compatibility for today's single-project invocation.

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

`ddx server` loads a registry of project roots before it serves requests.
Registry entries are config-first and describe local roots that belong to the
same server process.

### Registry Shape

The canonical server config shape is:

```yaml
server:
  projects:
    - id: ddx
      root: /Users/erik/Projects/ddx
      name: DDx
      default: true
    - id: notes
      root: /Users/erik/Projects/notes
      name: Notes
```

Fields:

- `id` is the stable project key used in URLs, MCP calls, and caches
- `root` is the absolute filesystem path for the project
- `name` is optional display text for UI labels
- `default` marks the project that legacy unscoped routes resolve to

If `server.projects` is absent, the server synthesizes a singleton registry
from the current working directory. That preserves today's `ddx server`
invocation without requiring a new config file shape.

If an `id` is omitted, the server may derive one from the root basename plus a
short path hash, but the derived value must remain stable for that root during
the lifetime of the registry.

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

The server is allowed to supervise worktree-backed activity across multiple
projects, but the worktree and worker pools remain project-scoped.

The first shipped execute-bead supervisor remains a single-project worker.
Multi-project scheduling is a later-stage server topology concern and does not
change the contract for one worker operating on one project context at a time.
Epic execution introduces a second worker type: an epic-scoped worker that owns
one persistent epic branch and worktree for that project.

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
- The branch itself is named after the epic and is the durable integration
  surface for that epic's work
- Epic merge gates run on the merge candidate, not just on individual child
  commits
- The final integration to the target branch uses a regular merge commit so the
  full epic branch history remains visible in git history

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

## Validation

This design should be covered by tests that verify:

- registry enumeration returns multiple project roots with a default marker
- scoped HTTP and MCP requests resolve the correct project context
- singleton fallback preserves today's `ddx server` invocation
- a broken project stays isolated from healthy sibling projects
- UI routing lands on the correct project-specific route

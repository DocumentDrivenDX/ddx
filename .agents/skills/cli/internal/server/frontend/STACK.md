# Stage 3 UI Stack — Svelte 5 Compatibility Verification

Verified: 2026-04-15 against npm registry.
Scaffold baseline: Svelte 5.55.2, @sveltejs/kit 2.57.0, vite 8.0.7.

---

## bits-ui

| Item | Value |
|---|---|
| Latest version | 2.17.3 |
| Peer dep | `svelte: "^5.33.0"` |
| Svelte 5 supported | **YES** — v2.x requires Svelte 5 (no Svelte 4 support) |
| Compatible with scaffold | YES |

**Fallback**: none needed.

---

## lucide-svelte

| Item | Value |
|---|---|
| Latest version | 1.0.1 |
| Peer dep | `svelte: "^3 \|\| ^4 \|\| ^5.0.0-next.42"` |
| Svelte 5 supported | **YES** — supports Svelte 3, 4, and 5 |
| Compatible with scaffold | YES |

**Fallback**: none needed.

---

## mode-watcher

| Item | Value |
|---|---|
| Latest version | 1.1.0 |
| Peer dep | `svelte: "^5.27.0"` |
| Svelte 5 supported | **YES** — v1.x requires Svelte 5 (no Svelte 4 support) |
| Compatible with scaffold | YES |

**Fallback**: none needed.

---

## GraphQL Client — Decision: Option B (graphql-request + graphql-ws)

**Decision recorded: 2026-04-15**

### Problem

Houdini 1.5.10 (and houdini-svelte@2.x / houdini@next) have unresolved peer-dep
incompatibility with Vite 8 + @sveltejs/kit 2.57. See analysis below for detail.
Stage 4.12 requires `workerProgress` subscriptions (graphql-ws protocol) — the
most likely point where the peer-dep workaround would collapse.

### Decision

**Option B chosen** — replace Houdini with:

- `graphql-request@^7.x` — lightweight typed GraphQL HTTP client, no Vite dependency
- `graphql-ws@^5.x` — WebSocket subscription client (graphql-ws protocol)
- `graphql@^16.x` — peer dep for graphql-request

**Future type safety**: add `@graphql-codegen/cli` + `@graphql-codegen/typescript`
+ `@graphql-codegen/typescript-operations` when the query surface grows beyond a
handful of operations. For now, manual TypeScript interfaces in `src/lib/gql/`
mirror the schema types with zero build-step overhead.

### Code paths

| Houdini (removed) | Replacement |
|---|---|
| `HoudiniClient` in `src/client.ts` | `GraphQLClient` from `graphql-request` via `$lib/gql/client.ts` |
| `load_NodeInfo` auto-generated load fn | Manual `client.request<NodeInfoResult>(NODE_INFO_QUERY)` in `+layout.ts` |
| Houdini store subscription pattern | `data.nodeInfo` passed directly from `load()` return value |
| Vite plugin `houdini/vite` | Removed — no build-time codegen step |
| `$houdini` alias | Removed from `vite.config.ts` and `svelte.config.js` |
| `houdini generate` postinstall | Removed from `package.json` |
| `graphql-ws` subscriptions (future) | `subscribeWorkerProgress()` in `$lib/gql/subscriptions.ts` |

### Rationale for Option B over Option A

Option A (downgrade Vite to 6.x) would require also downgrading `@sveltejs/kit`
to ≤2.21.0 — a months-old release. The whole scaffold was chosen for Vite 8 +
kit 2.57 stability. Locking to Vite 6 to unblock a single dependency (Houdini)
inverts the trade-off: we keep Houdini's codegen magic at the cost of pinning the
rest of the stack to older, less-maintained versions.

Option B preserves the full Vite 8 + kit 2.57 + Svelte 5 stack and loses only
Houdini's `load()` auto-generation convenience. The replacement (`graphql-request`
+ manual types) is simpler, widely used in SvelteKit projects, and has zero peer-dep
constraints against Vite or kit.

---

## Previous Houdini Analysis (historical — decision made above)

### Houdini (houdini + houdini-svelte) — REMOVED 2026-04-15

| Item | Value |
|---|---|
| houdini latest | 1.5.10 |
| houdini-svelte latest | 2.1.20 |
| houdini-svelte next | 3.0.0-next.13 |
| Svelte 5 supported | YES — houdini-svelte@2.x+ requires `svelte: "^5.0.0"` |
| Compatible with scaffold | **NO** — see below |

#### Incompatibility detail

The scaffold's `@sveltejs/vite-plugin-svelte@7.0.0` requires `vite: "^8.0.0"`.
No Houdini release currently supports vite 8:

| Release | vite peer dep | @sveltejs/kit peer dep | Status |
|---|---|---|---|
| houdini@1.5.10 + houdini-svelte@2.1.20 | `^5.3.3 \|\| ^6.0.3` | `<=2.21.0` | Blocked: vite 8, kit 2.57 both out of range |
| houdini@2.0.0-next.11 + houdini-svelte@3.0.0-next.13 | `^7.0.0` | `^2.9.0` | Blocked: vite 8 out of range (`^7.0.0` = <8) |
| houdini-svelte@canary (2025-03-26) | `^5.3.3 \|\| ^6.0.3` | `^2.9.0` | Blocked: vite 8 out of range, and canary unstable |

---

## Summary

| Library | Svelte 5 version | Svelte 5 OK | Vite 8 OK | Status |
|---|---|---|---|---|
| bits-ui | 2.17.3 | YES | YES | ✓ in use |
| lucide-svelte | 1.0.1 | YES | YES | ✓ in use |
| mode-watcher | 1.1.0 | YES | YES | ✓ in use |
| graphql-request | 7.x | n/a | YES | ✓ in use (Option B) |
| graphql-ws | 5.x | n/a | YES | ✓ in use (Option B) |
| houdini-svelte | 2.1.20 (stable), 3.0.0-next.13 (pre) | YES | **NO** | ✗ removed — replaced by Option B |

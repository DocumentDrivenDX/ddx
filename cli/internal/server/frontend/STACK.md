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

## Houdini (houdini + houdini-svelte)

| Item | Value |
|---|---|
| houdini latest | 1.5.10 |
| houdini-svelte latest | 2.1.20 |
| houdini-svelte next | 3.0.0-next.13 |
| Svelte 5 supported | YES — houdini-svelte@2.x+ requires `svelte: "^5.0.0"` |
| Compatible with scaffold | **NO** — see below |

### Incompatibility detail

The scaffold's `@sveltejs/vite-plugin-svelte@7.0.0` requires `vite: "^8.0.0"`.
No Houdini release currently supports vite 8:

| Release | vite peer dep | @sveltejs/kit peer dep | Status |
|---|---|---|---|
| houdini@1.5.10 + houdini-svelte@2.1.20 | `^5.3.3 \|\| ^6.0.3` | `<=2.21.0` | Blocked: vite 8, kit 2.57 both out of range |
| houdini@2.0.0-next.11 + houdini-svelte@3.0.0-next.13 | `^7.0.0` | `^2.9.0` | Blocked: vite 8 out of range (`^7.0.0` = <8) |
| houdini-svelte@canary (2025-03-26) | `^5.3.3 \|\| ^6.0.3` | `^2.9.0` | Blocked: vite 8 out of range, and canary unstable |

### Fallback options

**Option A — downgrade vite to 6.x (use houdini-svelte@2.x stable)**

Downgrade `vite` to `^6.x`, `@sveltejs/vite-plugin-svelte` to `^5.x` or `^6.x`, and
`@sveltejs/kit` to `<=2.21.0`. This makes the stable `houdini@1.5.10 + houdini-svelte@2.1.20`
work. Trade-offs: locks out vite 8 features; requires kit downgrade; kit 2.21 is months-old.

**Option B — replace Houdini with graphql-request + graphql-codegen (recommended)**

Use:
- `graphql-request@^7.x` — lightweight typed GraphQL client, no vite dependency
- `@graphql-codegen/cli@^6.x` — generates TypeScript types from `schema.graphql`
- Native `WebSocket` API for subscriptions (graphql-ws protocol)

Compatible with vite 8, @sveltejs/kit 2.57, Svelte 5. No peer-dep conflicts. Well-established
in SvelteKit community. Loses Houdini's `load()` codegen magic but keeps full type safety.

**Option C — wait for Houdini vite 8 support**

The houdini-svelte `next` branch tracks vite; vite 8 support will likely land in
`3.0.0-next.x`. Not suitable for Stage 3 now.

### Decision required

A reviewer must choose Option A or B before bead 30 (Install Houdini + codegen) is filed.
Option B is recommended: avoids pre-release risk and kit downgrade, and graphql-codegen
provides equivalent type safety to Houdini's codegen step.

---

## Summary

| Library | Svelte 5 version | Svelte 5 OK | Vite 8 OK | Action needed |
|---|---|---|---|---|
| bits-ui | 2.17.3 | YES | YES | none |
| lucide-svelte | 1.0.1 | YES | YES | none |
| mode-watcher | 1.1.0 | YES | YES | none |
| houdini-svelte | 2.1.20 (stable), 3.0.0-next.13 (pre) | YES | **NO** | choose fallback before bead 30 |

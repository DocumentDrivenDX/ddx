# Completion Note: Stage 4.17 Final Spec Consistency Audit

## Grep Command Run

```
grep -riE '(react|vite|tanstack|react-router|minisearch|MSW|frontend/dist|npm install|pnpm)' \
  docs/ library/ website/ CLAUDE.md README.md cli/ .github/ lefthook.yml 2>/dev/null
```

## Files With Matches

```
CLAUDE.md                                          — NO MATCHES
README.md                                          — NO MATCHES
cli/cmd/list.go
cli/go.sum
cli/internal/agent/agent_test.go
cli/internal/server/frontend/bun.lock
cli/internal/server/frontend/.gitignore
cli/internal/server/frontend/package.json
cli/internal/server/frontend/.prettierignore
cli/internal/server/frontend/README.md
cli/internal/server/frontend/src/lib/gql/subscriptions.smoke.spec.ts
cli/internal/server/frontend/src/lib/vitest-examples/greet.spec.ts
cli/internal/server/frontend/STACK.md
cli/internal/server/frontend/vite.config.ts
docs/helix/01-frame/features/FEAT-008-web-ui.md
docs/helix/02-design/adr/ADR-005-local-first-beads-ui.md
docs/helix/02-design/plan-2026-04-04.md
docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md
docs/helix/03-test/test-plans/TP-002-server-web-ui.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-agent-beads.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-exec-model.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-repo-2.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-repo.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-agent-token.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-beads.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-delta.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-planning-stack.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-post-sprint.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-repo.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-v0.2.0.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-06-evolution.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-06-repo.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-07-skill-install-cleanup.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-09-repo.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-10-repo.md
docs/helix/06-iterate/alignment-reviews/AR-2026-04-14-server-plan.md
docs/resources/spec-driven.md
.github/cspell.json
library/environments/brew/ai-tools-macos/Brewfile
library/environments/brew/ai-tools-macos/setup.sh
library/environments/docker/ai-development-base/Dockerfile
library/environments/docker/ai-development-base/README.md
library/environments/vagrant/ai-development-vm/provision.sh
library/mcp-servers/servers/notion.yml
website/static/demos/02-init-explore.cast
website/static/demos/ddx-server-ui.webm
```

## Classification of Every Match

### Category (a) — Historical/superseded, explicitly marked OK

All alignment-review docs carry the banner:
> **Historical** — describes the pre-2026-04-14 React stack. Current stack: see ADR-002 v2.

- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-agent-beads.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-exec-model.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-repo-2.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-04-repo.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-agent-token.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-beads.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-delta.md` — React stack history (TanStack Query, Vite)
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-planning-stack.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-post-sprint.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-repo.md` — React+Vite stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-05-v0.2.0.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-06-evolution.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-06-repo.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-07-skill-install-cleanup.md` — React stack history
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-09-repo.md` — React+Vite SPA history (also `frontend/dist` reference)
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-10-repo.md` — Vite dev-server history (also Vite references)
- `docs/helix/06-iterate/alignment-reviews/AR-2026-04-14-server-plan.md` — React stack history
- `docs/helix/02-design/plan-2026-04-04.md` — has "Historical" header at top; Vite+React+TanStack+MiniSearch references are the old stack being described
- `docs/helix/03-test/test-plans/TP-002-server-web-ui.md` — has "Historical" header at top
- `docs/helix/02-design/adr/ADR-005-local-first-beads-ui.md` — marked "Superseded by ADR-002 v2 (2026-04-14)"; MiniSearch reference is the superseded approach

### Category (b) — Intentional, justified OK

**cli/cmd/list.go** (lines 289, 357–358)
- Uses `"react"` as a generic library/framework filter example (`ddx list --filter react`). This is a generic text filter demonstration, not a reference to React as DDx's chosen framework.

**cli/go.sum** (line 232)
- False positive: `MSW` appears inside a base64 Go module checksum hash (`MSWPKKo0FU=`). This is an auto-generated file; the character sequence is coincidental.

**cli/internal/agent/agent_test.go** (line 1062)
- Test case `{"please run npm install", false}` — intentionally verifies that `npm install` commands are **blocked** by the agent. This is a security/safety check, not a dependency on npm.

**cli/internal/server/frontend/bun.lock**
- Auto-generated lockfile. The SvelteKit stack legitimately uses Vite as its build tool; `vite` and related entries are correct current dependencies, not stale React artifacts. `pnpm` appears as an ecosystem package name in transitive deps.

**cli/internal/server/frontend/.gitignore** (lines 23–25)
- `vite.config.js.timestamp-*` / `vite.config.ts.timestamp-*` — standard Vite/SvelteKit gitignore patterns for the current stack.

**cli/internal/server/frontend/.prettierignore** (line 3)
- `pnpm-lock.yaml` — standard prettier ignore pattern for pnpm lockfiles; present in every SvelteKit scaffold template. Harmless even though we use bun.

**cli/internal/server/frontend/package.json** (lines 7–9, 25–26, 41–42)
- `vite dev`, `vite build`, `vite preview` — current SvelteKit build commands. `@sveltejs/vite-plugin-svelte`, `@tailwindcss/vite`, `vite` are all required by the current SvelteKit+Tailwind stack. `vitest` is the current test runner.

**cli/internal/server/frontend/vite.config.ts**
- Current SvelteKit build configuration. Vite is the build tool for the current stack.

**cli/internal/server/frontend/STACK.md**
- Technical analysis document explaining why Houdini doesn't support Vite 8 and why we chose `graphql-request` instead. All Vite references are part of the Vite 8 compatibility analysis for the current stack choice.

**cli/internal/server/frontend/README.md** (line 35, 71)
- References Vitest (current unit test framework) and the scaffold command that installs it. No React references.

**cli/internal/server/frontend/src/lib/vitest-examples/greet.spec.ts** (line 1)
- `import { describe, it, expect } from 'vitest'` — uses Vitest, the current test framework.

**cli/internal/server/frontend/src/lib/gql/subscriptions.smoke.spec.ts** (line 9)
- `import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'` — uses Vitest, the current test framework.

**docs/helix/01-frame/features/FEAT-008-web-ui.md** (line 706)
- `"Playwright for testing (no MSW)"` — explicitly states MSW is **not** used. This is a correct negative specification, not a stale reference to using MSW.

**docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md**
- Migration design document that intentionally describes both the old React stack (to explain what was migrated away from) and the new SvelteKit stack. References like "Delete React entirely", "React+REST architecture", "minisearch" are context for the migration story. React/Vite/TanStack references in acceptance criteria rows describe what the migration was supposed to remove (and has removed).

**docs/resources/spec-driven.md** (line 182)
- `"implement using React with Redux"` — generic technology example used to illustrate how LLMs make implementation assumptions. This is educational/illustrative text, not a DDx stack dependency.

**.github/cspell.json** (lines 27, 121, 128)
- `"ReactJS"`, `"vite"`, `"pnpm"` — spelling dictionary entries needed so cspell does not flag these technical terms as spelling errors in source files. These are not technology choices; they are spellcheck allowlist entries.

**library/environments/brew/ai-tools-macos/Brewfile** (line 41)
- `brew "pnpm"` — generic developer environment setup including common JS toolchain options. These library environments are general-purpose dev machine templates, not DDx-specific deployments.

**library/environments/brew/ai-tools-macos/setup.sh**
- `npm install -g` — generic macOS dev tooling setup script.

**library/environments/docker/ai-development-base/Dockerfile**
- `npm install -g npm@latest`, `npm install -g ...` — generic Docker dev environment setup, not DDx-specific.

**library/environments/docker/ai-development-base/README.md**
- `npm install` — generic example in a general-purpose Docker environment README.

**library/environments/vagrant/ai-development-vm/provision.sh**
- `npm install -g npm@latest`, `npm install -g ...`, `create-react-app` — generic VM provision script for a general-purpose AI development environment. Not a DDx deployment artifact.

**library/mcp-servers/servers/notion.yml** (lines 26, 28)
- FALSE POSITIVE: "Invite" and "invite" contain the substring `vite`. No Vite/build-tool reference present.

**website/static/demos/02-init-explore.cast** (line 8)
- Terminal recording showing `ddx list --filter react` as a generic usage example in a demo. The filter value "react" demonstrates the list command, not a framework dependency.

**website/static/demos/ddx-server-ui.webm**
- Binary video file. Matched by filename pattern or binary content; no text references to classify.

### Category (c) — Stale, MUST fix

**ZERO category-(c) matches.**

## Action Taken

- `docs/plan-svelte-migration.md` — **deleted** per bead acceptance criteria (temporary plan doc).
- No other files required changes. All 175 grep matches are accounted for as category (a) or (b).

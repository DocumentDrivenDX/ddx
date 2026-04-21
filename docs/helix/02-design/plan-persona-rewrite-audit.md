---
ddx:
  type: plan
---
# Persona Rewrite — Reference Audit

> **Historical — completed.** Persona consolidation landed (commits
> 41fbd2f0 "chore: delete dropped persona files", 6856fc70
> "chore(skills): remove pre-consolidation skill trees"). Governing
> feature: FEAT-011 (`docs/helix/01-frame/features/FEAT-011-skills.md`).
> Retained as reference-audit record; not load-bearing.

Generated 2026-04-17 per bead ddx-5241bb19 (Phase 3 of FEAT-011
consolidation). Enumerates every reference to the current 10 personas
across config, workflows, tests, docs, and specs so the rewrite can
migrate without silent breakage.

## Scope

Current roster (library/personas/):

1. `strict-code-reviewer` → rewrite to `code-reviewer`
2. `test-engineer-tdd` → rewrite to `test-engineer` (generalized, DDx-internal paths stripped)
3. `architect-systems` → rewrite to `architect`
4. `pragmatic-implementer` → rewrite to `implementer`
5. `specification-enforcer` → rewrite in place (name kept)
6. `reliability-guardian` → DROPPED
7. `simplicity-architect` → DROPPED
8. `data-driven-optimizer` → DROPPED
9. `product-discovery-analyst` → DROPPED
10. `product-manager-minimalist` → DROPPED

## Config bindings

**None found.** `grep -rE "persona_bindings:" /Users/erik/Projects/ddx/
--include="*.yml" --include="*.yaml"` (excluding .claude/worktrees and
.ddx/executions) returned zero matches. No runtime `.ddx/config.yaml`
or plugin config currently binds any role to any shipped persona. The
migration has no silent-breakage risk on the config side.

## Test references

Tests that reference personas by name. **All create their own
fixture files inline with `os.WriteFile(personasDir, ...)`** — they do
not load from `library/personas/`. Renaming the library files will
not break these tests.

| Test file | Personas referenced | Notes |
|---|---|---|
| `cli/cmd/e2e_smoke_test.go` | strict-code-reviewer | Creates fixture inline at line 138 |
| `cli/cmd/persona_contract_test.go` | test-engineer-tdd | Creates fixture inline at line 229 |
| `cli/cmd/persona_integration_test.go` | strict-code-reviewer, test-engineer-tdd, architect-systems | Creates fixtures inline at lines 71, 107, 148 |
| `cli/cmd/persona_acceptance_test.go` | strict-code-reviewer, test-engineer-tdd | Creates fixtures inline at lines 31, 56 |

**Follow-up:** after the rewrites land, update these tests to use the
new names (code-reviewer, test-engineer, architect) so they double as
integration tests for the renamed personas. Not blocking Phase 3.

## Documentation references

Docs that mention personas by name (example-level references, not
runtime dependencies):

| File | Lines | Nature |
|---|---|---|
| `docs/helix/01-frame/features/FEAT-009-library-registry.md` | 29, 56-57 | Example: `ddx install persona/strict-code-reviewer` |
| `docs/helix/03-test/test-plans/TP-007-e2e-smoke-tests.md` | 59 | Test scenario binds code-reviewer to strict-code-reviewer |
| `docs/helix/00-discover/product-vision.md` | 144 | Mentions "strict-code-reviewer" as persona example |
| `docs/helix/02-design/adr/ADR-003-package-integrity.md` | 59, 65 | Example: `persona/strict-code-reviewer` install path |
| `docs/helix/02-design/solution-designs/SD-022-gql-svelte-migration.md` | (various) | Persona mentions in design |
| `docs/helix/02-design/solution-designs/SD-007-release-readiness.md` | (various) | Persona mentions in design |

**Follow-up:** mass-replace old names with new ones in these docs
after the rewrites land. Not blocking Phase 3 — docs can be updated
in a single pass once all 5 rewrites are committed.

## Workflow / HELIX plugin references

HELIX plugin is not installed locally in this repo. A grep of
`.ddx/plugins/` returned no persona references. Remote HELIX plugin
may reference personas — verify at HELIX-release time, not here.

## Migration decisions

1. **Kept (renamed):** strict-code-reviewer → code-reviewer;
   test-engineer-tdd → test-engineer; architect-systems → architect;
   pragmatic-implementer → implementer. Old files retained in-tree
   with deprecation markers until the delete-dropped-personas bead
   (`ddx-bb7dca79`) runs after one release of warning visibility.
2. **Kept (in place):** specification-enforcer. Rewrite the content,
   keep the filename.
3. **Dropped:** reliability-guardian, simplicity-architect,
   data-driven-optimizer, product-discovery-analyst,
   product-manager-minimalist. No config bindings; no test fixtures
   reference them by name. Safe to drop in one release window.
4. **Test migration:** update 4 test files to use new persona names
   after rewrites land (not blocking).
5. **Doc migration:** update 6 doc files to use new persona names
   after rewrites land (not blocking).

## Deprecation-warning scope (`ddx-77753c81`)

With zero runtime config bindings detected, the deprecation warning
is purely defensive — it exists for users in downstream projects who
may have bound dropped personas. The warning should:

- Emit stderr when `ddx agent run --persona <dropped-name>` is invoked
- Emit stderr when `ddx persona show <dropped-name>` is inspected
- Emit stderr when `.ddx/config.yaml` has a binding to a dropped
  persona (at load time, not at every invocation)

No changes needed to tests — they use their own fixtures, not the
deprecated library files.

## Summary

- No runtime bindings to migrate.
- 4 tests and 6 docs need string updates after rewrites land (follow-up).
- Safe to rewrite the 5 kept personas and delete the 5 dropped ones,
  with a one-release deprecation window for dropped names.

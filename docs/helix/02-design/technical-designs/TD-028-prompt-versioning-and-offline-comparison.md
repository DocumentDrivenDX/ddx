---
ddx:
  id: TD-028
  depends_on:
    - FEAT-022
  status: draft
  implementation: deferred
---
# Technical Design: Prompt Versioning, `DDX_PROMPT_VARIANT` Selector, and Offline Comparison

## Status

Design only. **Implementation is deferred.** This TD captures the
measurement roadmap that grew out of Story 12 (optimize the static
prompts that drive `ddx agent execute-bead`). Story 12 lands the
mechanical pieces — guardrail dedup (B1/B2) and `prompt_sha` on the
attempt manifest (B3). Everything below — the human-readable
`prompt_version` field, the `DDX_PROMPT_VARIANT` environment selector
for offline A/B, and the offline-comparison roadmap that consumes
them — is intentionally NOT implemented in Story 12. A follow-up
story will pick this up once we have enough post-Story-12 attempts
in `.ddx/executions/` to know which slices actually move the cheap-
tier metrics.

## Motivation

The Story 12 rewrite tightens two ~1,000-word static prompts
(`executeBeadInstructionsClaudeText`, `executeBeadInstructionsAgentText`
in `cli/internal/agent/execute_bead.go`) by ~30% and adds a
load-bearing-guardrail comment contract (FEAT-022 amendment). Two
risks fall out of that work:

1. **No way to tell whether the rewrite helped.** Cheap-tier behavior
   on the new shorter prompts could be better, the same, or worse, and
   we have no clean way to slice attempt outcomes (efficacy,
   `no_evidence_produced`, escalation rate) by prompt body.
2. **No way to A/B alternative wordings without shipping them.** Once
   the rewrite lands, every future tightening is a flying-blind edit
   to a constant, with the same measurement gap.

B3 closes part of (1) by writing `prompt_sha` (sha256 hex of the
rendered prompt bytes) into every attempt's `manifest.json`. That is
sufficient for a coarse before/after slice, but it has two
limitations:

- A sha is opaque. A reviewer reading an analytics dashboard cannot
  tell from `prompt_sha=ab12…` what version of the prompt produced
  the row.
- It only differentiates whatever variant DDx happened to ship at the
  time. There is no mechanism to run an experimental variant
  side-by-side with the live one.

This TD describes how we close both gaps without growing the
runtime surface area.

## Design

### 1. `prompt_version` field on the attempt manifest

`AttemptManifest` already carries `prompt_sha` (see
`cli/internal/agent/execute_bead.go` field `PromptSHA` at line 177
and `promptSHA(...)` at line 1478). Add an adjacent string field
`prompt_version` populated from a per-(harness, variant) constant
defined alongside the prompt body:

```go
const (
    // executeBeadPromptVersionClaude is bumped by hand whenever
    // executeBeadInstructionsClaudeText changes in a way that
    // analytics should treat as a new prompt. Format:
    // "<story>-<harness>-<n>", e.g. "story12-claude-1".
    executeBeadPromptVersionClaude = "story12-claude-1"
    executeBeadPromptVersionAgent  = "story12-agent-1"
)
```

Rules:

- The version string is a developer-curated label, NOT computed.
  Whitespace-only edits do not bump it; semantic edits do. The
  guardrail-list comment block above each prompt constant
  (FEAT-022 amendment) is the natural place to record the version
  bump alongside the change.
- `prompt_sha` remains the source of truth for "is this attempt
  byte-identical to that one." `prompt_version` is the human label
  used for grouping in dashboards and changelogs.
- Both fields are written into `manifest.json`. Existing readers
  that ignore unknown fields are unaffected.

Why both: a sha alone forces every analytics consumer to maintain
its own sha→label mapping. A label alone allows accidental
collisions (developer forgets to bump). Carrying both keeps the
sha as the authoritative grouping key and the label as the
display name.

### 2. `DDX_PROMPT_VARIANT` environment selector

Add an opt-in environment variable read at prompt-render time:

- `DDX_PROMPT_VARIANT` unset (default): render the live prompt as
  today. Behavior is identical to current code paths.
- `DDX_PROMPT_VARIANT=<name>`: select an experimental prompt body
  registered in a small in-process registry, e.g.
  `map[string]promptBundle` keyed by `<name>`, where
  `promptBundle` contains the `{claudeText, agentText, version}`
  triple.

Constraints:

- The selector ONLY swaps the static body. The harness selector
  (`agent`/`fiz`/`embedded` → Agent variant, everything else →
  Claude variant; see `execute_bead.go:1343`) is unchanged. An
  experimental bundle must supply BOTH variants.
- Unknown variant name is a hard error at render time, not a
  silent fallback. Operators running an A/B want loud failure if
  they typo the variant name.
- The variant name (and its version label) is recorded on the
  manifest as `prompt_variant` and folded into `prompt_version`
  (e.g. `experiment-tighter-step0-claude-1`). `prompt_sha` still
  hashes the actual rendered bytes, so it remains the
  authoritative grouping key.
- Variants live in source under
  `cli/internal/agent/prompt_variants/` with the same load-bearing-
  guardrail comment contract as the live prompts. A variant that
  drops a guardrail fails the FEAT-022 regression test, same as an
  edit to the live prompt would.
- Variants are NOT a config-file concept. They are compiled in.
  This keeps offline experimentation a developer activity, not an
  end-user surface, and avoids inventing a new config schema for
  something we expect to retire after each round of tuning.

Why an env var and not a CLI flag: `ddx agent execute-bead` is
invoked by the loop (`agent execute-loop` / `ddx work`), and the
loop does not currently thread per-call CLI flags into the
prompt-render path. An env var is the cheapest way to attach a
variant selection to a whole drain run without rewriting the
loop's argv plumbing.

### 3. Offline-comparison roadmap

The combination of `prompt_sha`, `prompt_version`, and
`prompt_variant` on the manifest lets us reuse the telemetry that
already exists in the repo, without standing up new infrastructure:

- **`.ddx/executions/<run-id>/manifest.json`** — the per-attempt
  record this TD extends. Already includes outcome, harness, bead
  id, base rev, and (post-B3) `prompt_sha`. Adding
  `prompt_version` and `prompt_variant` here is the only schema
  change this roadmap requires.
- **`.ddx/executions/<run-id>/usage.json`** — token usage emitted
  by harnesses that report it. Joined to the manifest by
  `<run-id>`, this gives a per-prompt-version cost profile.
- **`session_index.go`** (`cli/internal/agent/session_index.go`)
  — already maintains the session-token index that ties an
  attempt's run id to its harness session events. No change
  required; new fields ride along.
- **`resolver_feat008.go`** (`cli/internal/server/graphql/`) —
  exposes the efficacy view (`EfficacyRows`, `EfficacyAttempts`).
  The efficacy resolver already aggregates per attempt; extending
  it to group/filter by `prompt_version` or `prompt_variant` is a
  resolver-only change once the manifest fields are populated.

Concretely, the offline workflow becomes:

1. Operator sets `DDX_PROMPT_VARIANT=experiment-foo` and runs
   `ddx work` over a representative bead queue (or replays a
   captured queue with `--from <rev>`).
2. Each attempt writes a manifest tagged
   `prompt_variant=experiment-foo`,
   `prompt_version=experiment-foo-claude-1`, plus the rendered
   `prompt_sha`.
3. The efficacy view (extended) groups attempts by
   `prompt_version` and reports outcome distribution, retry
   count, escalation rate, mean tokens, and mean wall time per
   group. A second drain with the variant unset or set to a
   different name produces the comparison set.
4. If the experiment wins, promote the variant: copy its body
   over the live constant, bump the live version label, and
   delete the variant entry. If it loses, delete the variant.

Out of scope for this TD:

- Statistical significance testing. The cheap-tier sample sizes
  in our queue are too small for clean significance claims;
  decisions will be directional ("variant clearly worse → drop"
  or "variant clearly better → ship"), not p-value gated.
- Online A/B (random-assigning live attempts to variants). The
  selector is offline-only by design — a single run uses a single
  variant. Online A/B would require per-attempt variant
  assignment, manifest schema changes beyond this TD, and a story
  of its own.
- Cross-project comparison. The efficacy view is per-project; a
  variant evaluated in one project's queue is not automatically
  comparable to another's.

### Interactions with existing code

- `buildPrompt` (`execute_bead.go:1392`): the natural insertion
  point for variant lookup. It already owns prompt assembly; it
  would gain a `selectVariant(harness, os.Getenv(...))` call
  before reading the static constants.
- `manifest.json` writer
  (`AttemptManifest` struct + the `PromptSHA:` write at
  `execute_bead.go:1127`): gains two adjacent string fields. Same
  serialization path.
- `executions_mirror.go`: already mirrors manifests into
  `.ddx/executions/` for the resolver to read. No change.
- `BuildReviewPromptBounded` and `beadReviewInstructions`: not
  in scope. The reviewer prompt is intentionally untouched by
  Story 12 (AC9) and remains untouched by this TD.

## Non-goals

- This TD does not propose a new config-file surface. No
  `.ddx.yml` keys, no new commands.
- This TD does not propose runtime variant rotation. One drain
  uses one variant.
- This TD does not propose changing the harness selector. The
  Claude/Agent split (`execute_bead.go:1343`) is preserved per
  Story 12 AC10.
- This TD does not propose statistical-test infrastructure.

## Migration / rollout

When a follow-up story implements this:

1. Land `prompt_version` first, defaulting to the current Story
   12 labels. Manifests gain the field; nothing else changes.
   Backfill is unnecessary — historical manifests stay on
   `prompt_sha` only, and the resolver treats missing
   `prompt_version` as a sentinel ("pre-versioning").
2. Land the `DDX_PROMPT_VARIANT` selector and the variants
   directory. Default behavior (variable unset) is unchanged.
3. Extend the efficacy resolver's grouping last. Until then,
   offline comparison is "grep manifests by `prompt_version`."

## Open questions

- Should `prompt_version` be enforced via a unit test that fails
  when the constant body changes without a corresponding version
  bump? A naive sha-of-body equality check is workable but
  noisy on whitespace edits. Defer the decision to the
  implementing story; the FEAT-022 guardrail-list comment
  contract already gives reviewers a place to catch missed
  bumps.
- Should the variant registry live in a `prompt_variants_test.go`
  build-tagged file rather than production code, so experimental
  bodies do not ship in the binary? Probably yes for variants
  intended only for one-off offline runs; defer.

## References

- Story 12 plan: `/tmp/story-12-final.md` (working doc, see
  "New TD" section).
- FEAT-022: `docs/helix/01-frame/features/FEAT-022-prompt-evidence-assembly.md`
  (minimum-prompt rule amendment from B4a).
- Live prompt constants:
  `cli/internal/agent/execute_bead.go`
  (`executeBeadInstructionsClaudeText` ~line 1208,
  `executeBeadInstructionsAgentText` ~line 1274,
  harness selector ~line 1343, `buildPrompt` ~line 1392).
- Manifest field landed in B3:
  `AttemptManifest.PromptSHA` (~line 177),
  populated at the manifest write site (~line 1127),
  computed by `promptSHA` (~line 1478).
- Telemetry consumed by the offline-comparison roadmap:
  `cli/internal/agent/session_index.go`,
  `cli/internal/server/graphql/resolver_feat008.go`
  (efficacy view), per-run `usage.json` written by harnesses.

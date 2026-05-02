---
title: Drift Is Debt
weight: 8
---

# Drift is debt

> Documentation that lives separately from the code it describes will
> drift out of correctness.

## The principle

Specifications, code, configs, and documentation drift apart whenever
one is changed without the others. Each instance of drift is a small
debt: the next reader has to reconcile the conflict, the next agent
has to guess which is authoritative, the next change has to pay
interest on both. Left untreated, drift compounds until the documents
are decorative and the truth lives only in the code — at which point
spec-first development has quietly collapsed into code-first
development.

Drift is not a discipline failure of any particular team. It is a
structural property of any system that maintains two artifacts and
asks humans to keep them in sync by hand. Forty years of accumulated
evidence — across documentation styles, languages, and project sizes —
shows that divergence accumulates regardless of process maturity. The
factory's response is therefore not heroic vigilance; it is mechanical
detection. Drift checks belong in CI, in review, and in the alignment
step, so that divergence shows up as a failing build rather than a
future surprise.

For AI-driven systems the stakes are higher because agents *read the
docs* to decide what to do. Drifted docs mislead the agent the same
way they mislead a new hire, except an agent acts faster and asks
fewer clarifying questions. DDx invests in alignment tooling because
the alternative is slow rot, and slow rot is the failure mode that
kills document-driven projects.

## Evidence

- **REF-017 — Knuth, "Literate Programming."** The structural
  proposal: a single source with prose and code interleaved as one
  artifact, with the runnable code extracted as a derived view.
  Changing the explanation and changing the code are the same edit.
- **REF-028 — 2023 documentation drift study.** Across an
  open-source corpus, documentation diverged from implementation at
  measurable rates; API docs drifted faster than architecture docs,
  both faster than glossaries; divergence accumulated regardless of
  project maturity, language, or team size.
- **RSCH-008.** REF-028 says drift is universal when doc and code are
  separate; REF-017 says the answer is to make them not separate.

See `docs/helix/00-discover/research/RSCH-008-drift-is-debt.md`.

## DDx response

- **`helix align`** reconciles spec and implementation, and surfaces
  the divergences that need to be resolved in one direction or the
  other.
- **Beads carry their AC inline.** The AC is the test contract;
  there is no second prose document to keep in sync with the bead
  description.
- **Ratchets and lints in CI** detect drift before merge — broken
  references, stale `depends_on` edges, and AC items that no longer
  point at runnable evidence all fail the build.
- **The post-merge review pass** flags AC items the implementation
  no longer satisfies, so divergence between what was promised and
  what shipped surfaces as a reopened bead, not as silent rot.
- **`ddx doc audit`** walks `depends_on` edges and reports broken
  references — the structural equivalent of a build error for
  documentation.

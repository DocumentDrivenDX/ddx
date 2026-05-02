---
ddx:
  id: RSCH-008
  status: draft
  depends_on:
    - REF-017
    - REF-028
id: RSCH-008
title: "Drift Is Debt"
kind: research-synthesis
summary: "Documentation diverges from implementation predictably and measurably; the only stable answer is to keep the doc and the code in the same artifact, regenerated together."
tags: [documentation, drift, ddx-principle]
---

# Drift Is Debt

## Principle

Documentation that lives separately from the code it describes will drift
out of correctness. The fix is structural, not cultural — keep the doc and
the artifact it describes in the same regenerable unit.

## Synthesis

Knuth's "Literate Programming" (REF-017) is the foundational structural
proposal. Knuth's claim was not that programmers should write better
comments; it was that the program should be written *primarily* for human
readers, with prose and code interleaved as a single artifact, and the
machine-runnable code extracted as a derived view. The reason this
matters for drift is that there is only one source: changing the
explanation and changing the code are the same edit. There is no second
artifact to forget about.

The 2023 empirical study of documentation drift (REF-028) supplies the
data Knuth's argument predicts. Across a corpus of open-source projects,
documentation diverged from implementation at measurable rates;
different documentation types decayed at different speeds (API docs
faster than architecture docs, both faster than glossaries); and the
divergence accumulated regardless of project maturity, language, or
team size. The drift is not a discipline failure of any particular team
— it is a structural property of any system that maintains two
artifacts and asks humans to keep them in sync by hand.

The composition is direct. REF-028 says drift is universal when the doc
and the code are separate. REF-017 says the answer is to make them not
separate. Forty years of evidence has accumulated since the literate
programming proposal, and every iteration of the principle — Doxygen,
docstrings, OpenAPI generated from code, type-driven docs — has the same
shape: a single source, with the human-readable view and the
machine-runnable view both derived from it.

For AI-driven systems the stakes are higher because agents *read the
docs* to decide what to do. Drifted docs mislead the agent the same way
they mislead a new hire, except an agent acts faster and asks fewer
clarifying questions.

## DDx Implication

DDx treats specs, beads, AC, and code as parts of one regenerable
system, not as separate artifacts maintained in parallel. Beads carry
their AC inline; the AC is the test contract. The execute-bead loop
captures what the agent actually read versus what the spec said,
exposing drift the moment it is introduced. The doc audit
(`ddx doc audit`) walks `depends_on` edges and flags broken references,
which is the structural equivalent of a build error for documentation.
The product principle is that drift is not a process problem to be
solved by quarterly cleanups — it is a tooling problem to be solved by
making the divergence impossible to commit in the first place.

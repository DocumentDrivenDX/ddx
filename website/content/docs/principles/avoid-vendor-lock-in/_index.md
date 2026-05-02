---
title: Avoid Vendor Lock-In
weight: 7
---

# Avoid vendor lock-in

> Standardize the contract; let implementations compete underneath.

## The principle

Model vendors change pricing, deprecate models, and rewrite APIs on
their own schedule. A factory that bakes a single vendor's name into
its routing logic, prompts, or contracts hands that vendor a veto over
the factory's roadmap. A platform whose ceiling is one vendor's
strategy and lifespan is a platform whose ceiling moves whenever that
vendor's product decisions move.

The defense is abstraction at the right seam: endpoints, capabilities,
and protocols rather than provider names; live discovery rather than
hardcoded model lists; portable prompt formats rather than
vendor-specific control tokens. The cost is a layer of indirection and
the discipline to resist convenient shortcuts. The payoff is the
freedom to swap, mix, and tier providers as the market moves — and to
run local models alongside hosted ones without rewriting the harness.

The structural property that produces durability is specifying
*interfaces*, not implementations, and authoring those interfaces to
be implementable by anyone. Standards that smuggle implementation
choices into the contract — by mandating a specific data format,
network library, or configuration system — collapse back into vendor
lock-in by the back door. DDx treats portability as non-negotiable,
even when a single-vendor path would be cheaper to ship this quarter.

## Evidence

- **REF-025 — Language Server Protocol.** Collapsed an `m × n`
  editor-language integration problem to `m + n` by standardizing a
  JSON-RPC contract between editor ("client") and language tooling
  ("server"). The contract was the leverage; implementations fell
  into place once it was stable.
- **REF-030 — Portability lineage.** POSIX (1988) made OS services
  portable across UNIX vendors; SQL (1987) did the same for relational
  databases; OCI (2015) standardized container image and runtime
  contracts and broke Docker's de facto monopoly. All four standards
  specify interfaces, not implementations.
- **RSCH-007.** REF-025 and REF-030 compose into the bet that the
  projects surviving the next decade of AI tooling churn are the ones
  whose contract surfaces are stable enough to outlast any individual
  vendor's roadmap.

See `docs/helix/00-discover/research/RSCH-007-avoid-vendor-lock-in.md`.

## DDx response

- **The bead tracker has a JSONL import/export surface** compatible
  with `bd`/`br`, so beads are portable across tooling.
- **The agent service speaks to harnesses through a typed
  RunOptions/Output contract**, not to vendor SDKs. New harnesses plug
  in by satisfying the contract, not by being the right brand.
- **MCP for tool surfaces** uses an open protocol rather than a
  proprietary interface.
- **Endpoint-first routing** explicitly replaces named provider
  profiles with discovery against a generic endpoint contract — the
  same move LSP made against editor-specific integrations.
- **Plain files and standard git** for the document library and bead
  store: `cat`, `diff`, and `git` are the access protocol, not a
  proprietary API.

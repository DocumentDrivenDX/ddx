---
ddx:
  id: RSCH-007
  status: draft
  depends_on:
    - REF-025
    - REF-030
id: RSCH-007
title: "Avoid Vendor Lock-In"
kind: research-synthesis
summary: "Durable platforms are built on stable contract surfaces, not stable vendors; the LSP, POSIX, SQL, and OCI lineage shows that the right thing to standardize is the protocol, not the implementation."
tags: [portability, protocols, ddx-principle]
---

# Avoid Vendor Lock-In

## Principle

Standardize the contract; let implementations compete underneath. A
platform that takes a hard dependency on a single vendor is a platform
whose ceiling is that vendor's strategy and lifespan.

## Synthesis

The Language Server Protocol (REF-025) is the canonical recent example.
Before LSP, every editor needed bespoke integrations with every language
toolchain — an `m × n` integration problem that practically capped how
many languages an editor could support and how many editors a language
could reach. By standardizing a JSON-RPC contract between editor
("client") and language tooling ("server"), Microsoft collapsed the
problem to `m + n`. The result was an explosion of supported
language/editor combinations, none of which had to be implemented by any
single vendor. The contract was the leverage; the implementations
fell into place once the contract was stable.

The portability lineage (REF-030) shows the pattern is decades older and
domain-independent. POSIX (1988) made operating-system services portable
across UNIX vendors by standardizing the syscall and shell contract; the
implementations diverged radically (AIX, Solaris, Linux, macOS) but
portable software kept working. SQL (ISO/IEC 9075, 1987) did the same
for relational databases — the dialects argue endlessly, but the core
contract is durable enough that an application can swap engines without
rewriting its data layer. OCI (2015) standardized container image and
runtime contracts and broke Docker's de facto monopoly almost
immediately, enabling Podman, containerd, and a Kubernetes ecosystem
that no longer depended on a single vendor's roadmap.

What all four standards share: they specify *interfaces*, not
implementations, and they were authored to be implementable by anyone.
That is the structural property that produces durability. Standards
that smuggle implementation choices into the contract — by mandating a
specific data format, network library, or configuration system —
collapse back into vendor lock-in by the back door.

## DDx Implication

DDx is a protocol-shaped system on purpose. The bead tracker has a JSONL
import/export surface compatible with `bd`/`br`. The agent service speaks
to harnesses through a typed RunOptions/Output contract, not to vendor
SDKs. The MCP integration uses an open protocol, not a proprietary
interface. The endpoint-first routing redesign explicitly replaces named
provider profiles with discovery against a generic endpoint contract —
the same move LSP made against editor-specific integrations. The bet,
backed by REF-025 and REF-030, is that the projects that survive the
next decade of AI tooling churn are the ones whose contract surfaces are
stable enough to outlast any individual vendor's roadmap.

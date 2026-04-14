---
ddx:
  id: ADR-005
  depends_on:
    - FEAT-008
    - FEAT-004
    - ADR-002
---
# ADR-005: Local-First Client-Side Data Layer for Beads UI

**Status:** Superseded by ADR-002 v2 (2026-04-14)
**Date:** 2026-04-04
**Revised:** 2026-04-07

This ADR is superseded by the SvelteKit + Houdini stack decision recorded in ADR-002 (revised 2026-04-14). The client-side MiniSearch approach described here was a workaround for the absence of server-side pagination; the migration to GraphQL with Relay-style cursor connections (introduced in ADR-002 v2) solves this at the schema level, making a full client-side data load unnecessary. This file is retained as a historical record.

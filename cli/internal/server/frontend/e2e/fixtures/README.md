# Playwright e2e fixture workspace

This directory provides a self-contained DDx workspace that the default
Playwright harness copies into a temp directory before booting the Go DDx
server. The fixture isolates e2e runs from the developer's home directory and
from the repository's live `.ddx/` state.

Contents:

- `.ddx/config.yaml` — minimal DDx config pointing at the local plugin library.
- `.ddx/beads.jsonl` — open, closed, and blocked beads so the bead API endpoints
  return non-empty data.
- `.ddx/plugins/ddx/` — minimal personas, prompts, and templates so the document
  and persona endpoints have something to list.
- `docs/` — small docs library for the document graph endpoint.

The fixture is read-only; the harness copies it to a `mktemp -d` workspace at
boot so test runs do not mutate these files. Do not reference user-home paths
from anything in this directory.

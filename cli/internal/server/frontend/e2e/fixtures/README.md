# Playwright e2e fixture workspace

This directory provides a self-contained DDx workspace that the default
Playwright harness copies into a temp directory before booting the Go DDx
server. The fixture isolates e2e runs from the developer's home directory and
from the repository's live `.ddx/` state.

Contents:

- `.ddx/config.yaml` — minimal DDx config for the copied fixture workspace.
- `.ddx/beads.jsonl` — open, closed, and blocked beads so the bead API endpoints
  return non-empty data.
- `.ddx/plugins/ddx/` — fixture-only local overlay data for local-overlay and
  legacy-compatibility endpoint coverage. Marketplace plugin payloads should
  not be copied into project fixtures; normal installs use `.ddx/plugins.lock.yaml`,
  the shared XDG cache, and generated adapters.
- `docs/` — small docs library for the document graph endpoint.

The fixture is read-only; the harness copies it to a `mktemp -d` workspace at
boot so test runs do not mutate these files. Do not reference user-home paths
from anything in this directory.

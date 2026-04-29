# Project Concerns

## Active Concerns

<!-- Select from .ddx/plugins/helix/workflows/concerns/ or add custom entries.
     Each line: concern-name (category) — ADR reference that justified selection.
     The ADR ref is how the concern/ADR/spike chain stays traceable. -->

- [concern-name] ([category]) — ADR-NNN: [one-line rationale]

## Project Overrides

<!-- Override specific library practices. Cite the governing ADR. -->

- [Practice]: [override] (see [ADR-NNN])

## Area Labels

This project uses the following area labels for concern scoping:

<!-- Declare which area labels your beads use. Concerns are injected into
     beads based on matching these labels against each concern's `areas` field. -->

- `area:ui` — user-facing interfaces
- `area:api` — backend services and endpoints
- `area:data` — database, storage, data pipeline
- `area:infra` — deployment, CI/CD, infrastructure
- `area:cli` — command-line tools

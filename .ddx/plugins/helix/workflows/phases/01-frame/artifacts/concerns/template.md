---
ddx:
  id: concerns
---

# Project Concerns

Project Concerns declare active cross-cutting context for downstream work. They
are not principles, requirements, ADRs, test plans, or implementation tasks.

## Active Concerns

<!-- Select from .ddx/plugins/helix/workflows/concerns/ or declare project-local
     entries. Include only concerns that change downstream work across more
     than one artifact or implementation area. -->

| Concern | Source | Areas | Why Active | Key Practices |
|---------|--------|-------|------------|---------------|
| [concern-name] | [library or project-local] | `area:*` | [Why this changes downstream work] | [Practices downstream work must consider] |

## Project Overrides

<!-- Override specific library practices only when the project has a real reason.
     Cite the governing ADR when available. -->

| Concern | Practice | Override | Authority |
|---------|----------|----------|-----------|
| [concern-name] | [library practice] | [project-specific override] | [ADR-NNN or "Needs ADR"] |

## Area Labels

This project uses the following area labels for concern scoping:

<!-- Declare which area labels work items use. Concerns are injected into
     downstream context based on matching labels against each concern's area
     scope. -->

- `area:ui` — user-facing interfaces
- `area:api` — backend services and endpoints
- `area:data` — database, storage, data pipeline
- `area:infra` — deployment, CI/CD, infrastructure
- `area:cli` — command-line tools

## Concern Conflicts

<!-- Resolve conflicts between active concerns. -->

| Conflict | Resolution |
|----------|------------|
| [Concern A] vs. [Concern B] | [How downstream work should decide] |

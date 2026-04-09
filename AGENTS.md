# DDx Agent Instructions

This repository uses DDx's built-in bead tracker for durable work management.

## Bead Policy

- Treat `.ddx/beads.jsonl` as DDx-managed data, not as a hand-edited document.
- Create beads only with `ddx bead create`.
- Modify bead metadata only with `ddx bead update`.
- Manage dependencies only with `ddx bead dep add` and `ddx bead dep remove`.
- Close work only with `ddx bead close`.
- Use `ddx bead import` and `ddx bead export` for bulk migration or interchange.

## Prohibited Actions

- Do not edit `.ddx/beads.jsonl` manually.
- Do not add, remove, or rewrite bead rows with `apply_patch`, editors, scripts, or ad hoc JSONL manipulation.
- Do not invent bead IDs or prefixes such as `hx-*` or `ddx-*`.
- Do not treat nearby tracker entries as a naming pattern to copy.

## If The CLI Seems Insufficient

- Prefer the nearest supported `ddx bead` command.
- If the required tracker operation is not supported by `ddx bead`, stop and ask rather than editing tracker storage directly.

## Verification

- Use `ddx bead show <id>` to inspect one bead.
- Use `ddx bead list`, `ddx bead ready`, and `ddx bead status` to inspect queue state.
- Use `ddx bead --help` and `ddx bead create --help` before assuming a flag exists.

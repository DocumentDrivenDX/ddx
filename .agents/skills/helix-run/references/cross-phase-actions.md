# Cross-Phase Actions

Use these repo-level HELIX actions when the task is larger than a single phase
or story.

## Queue Check

Use when:

- the user wants to know whether there is more HELIX work to do
- the ready queue has drained and the next step is unclear
- the user wants an operational answer such as implement, align, backfill, wait, ask for guidance, or stop

Reference docs:

- `workflows/actions/check.md`

Key rules:

- use `ddx bead ready`, not `ddx bead list --ready`
- treat the check as a bounded queue-health probe
- do not claim work from the check action
- use it when the implementation loop drains
- follow its `NEXT_ACTION` result with the exact recommended command

## Documentation Backfill

Use when:

- the repo has incomplete or missing `docs/helix/` artifacts
- the codebase exists but the planning stack does not
- the user wants HELIX docs reconstructed from current evidence

Reference docs:

- `workflows/actions/backfill-helix-docs.md`
- `workflows/templates/backfill-report.md`
Key rules:

- research first
- review recursively across folders and file-sets
- maintain a coverage ledger
- use confidence levels: `HIGH`, `MEDIUM`, `LOW`
- ask the user before finalizing low-confidence canonical content
- write the durable report to `docs/helix/06-iterate/backfill-reports/`

## Alignment Review

Use when:

- the planning stack already exists and needs reconciliation against implementation
- the user wants drift classification, traceability auditing, or remediation planning

Reference docs:

- `workflows/actions/reconcile-alignment.md`
- `workflows/templates/alignment-review.md`
Key rules:

- review top-down
- classify gaps explicitly
- create a review epic plus review issues in `ddx bead`
- consolidate findings before creating execution issues

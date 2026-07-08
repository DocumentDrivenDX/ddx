### Findings

| Severity | Area | Finding |
|---|---|---|
| BLOCKING | Terminal suppression | `TerminalBlockTTL`, “desired-state generation,” and “active suppression reason” are not defined in the target. The plan says a block suppresses only when those predicates hold, but does not name the constant value, storage/source for generation, or the allowed suppression-reason set. Agents could implement timestamp-only expiry, invent new durable state, or classify terminal reasons differently. |
| BLOCKING | Liveness predicate | `workerRecordLive(rec)` is still underspecified. “Sidecar heartbeat is fresh when available” and “run-state evidence is not expired” need exact evidence paths, freshness thresholds, and conflict precedence. Without that, one bead can choose heartbeat-first behavior while another chooses run-state-first behavior. |
| BLOCKING | JSONL append helper | The locking contract is not specific enough for compatible implementation. The target does not define lock file location, advisory-lock mechanism, timeout behavior, fsync/durability expectations, partial-row recovery, or whether lock acquisition failure drops, retries, or returns typed evidence. “Use it for attempts, locks, routing/ingest/event mirrors” is also too broad without enumerating call sites. |
| BLOCKING | Preserved-needs-review gate | “Operator records an explicit unblock/review-accepted note” is not mapped to an existing command, evidence type, timestamp ordering rule, or fingerprint. This invites agents to invent tracker fields or direct JSONL/evidence conventions, conflicting with the stated bead lifecycle constraints. |
| BLOCKING | Bead boundaries | Several proposed beads are not execution-ready under the DDx bead standard. Items 4, 5, 6, 7, and 8 name broad subsystems rather than root cause file:line, exact in-scope call sites, specific `Test*` symbols, dependencies, and command AC. They are good epic slices, but not yet worker beads. |
| WARNING | Resource pressure | The plan says other projects continue reconciling independently during EMFILE/FD pressure, but does not define whether resource checks are per project, per supervisor process, or host-wide. EMFILE is often process-wide, so restart/backoff behavior needs a per-project isolation rule and a global throttling rule. |
| WARNING | Meta-scan | The meta-scan may “clear dead-PID run-state” and “report stale claims,” but the mutation boundary is unclear. It should specify which operations are read-only reports versus durable state changes, and which existing commands or store APIs perform those changes. |
| WARNING | Readiness decoding | The string-or-list decoder is mostly concrete, but array normalization should specify whether empty entries are preserved, trimmed, rejected, or joined exactly. Otherwise tests may encode different newline behavior. |
| NOTE | Overall shape | The revised design is directionally better because it extends the current supervisor and explicitly rejects a new durable worker-slot schema, but some sections still describe policy intent rather than implementation contracts. |

### Verdict: BLOCK

### Summary

Do not file these as execution beads yet. The plan is now strong enough to become an epic plus design note, but the state-machine contracts still leave multiple incompatible choices open, especially terminal suppression, liveness evidence, JSONL locking, and preserved-review unblocking. Tighten those contracts first, then split into beads with exact root-cause citations, call sites, test names, dependencies, and command-based acceptance criteria.
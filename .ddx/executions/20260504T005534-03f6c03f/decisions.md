# Decisions — internal/bead reachability backfill (ddx-b42dd3a0)

One line per symbol: WIRE | DELETE | PENDING <reason>.

- internal/bead/backend.go:82 NewBackend — DELETE thin re-export of NewStore with no callers; aspirational façade, no consumers in cli/cmd or agent loop yet.
- internal/bead/backend_jsonl.go:93 jsonlFallbackForCollection — DELETE unused helper; store.go:121 already constructs the fallback inline using the resolved file/lock paths.
- internal/bead/lock.go:70 Store.breakStaleLock — DELETE method-receiver wrapper for breakStaleLockDir; package function is the only caller and is invoked directly inside acquireDirLock.
- internal/bead/registry.go:104 Registry.IDs — DELETE; only test caller. registry_test.go updated to assert via Lookup for the two shipping collections.
- internal/bead/store.go:133 NewStoreWithBackend — DELETE; doc-comment claims "for testing" but no test or production caller exists.
- internal/bead/store.go:1244 Store.detectCurrentCommit — DELETE; never called; not wired into any event-recording path.
- internal/bead/types.go:96 IsCanonicalStatus — WIRE; replaces the inline status switch in Store.validateBead (cli/internal/bead/store.go) so CanonicalStatuses is now reachable through a single predicate.
- internal/bead/types.go:141 IsValidStatusTransition — DELETE; speculative state-machine implementation with no production integration point. operator_prompt_test.go TestStatusTransitionsForProposed removed alongside.

Originating bead reopen: deletions above were speculative APIs; no AC of the originating bead (ddx-83440482 or its parent) describes runtime behavior that depends on these symbols, so no follow-up reopen needed.

Verification:
- cd cli && go test ./internal/bead/... → pass
- cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | grep internal/bead/ → no output (zero remaining dead in package)

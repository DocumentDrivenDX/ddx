# Subscription.go Dead Code Resolution

## Context

The production-reachability check identified 11 symbols in `cli/internal/bead/axon/subscription.go` as unreachable:
1. ChangeEventFromLifecycle (line 33)
2. ChangeEvent.ToLifecycleEvent (line 47)
3. DualTransport.Query (line 73)
4. DualTransport.Subscribe (line 81)
5. NewWebSocketSubscriptionTransport (line 98)
6. WebSocketSubscriptionTransport.Query (line 103)
7. WebSocketSubscriptionTransport.Subscribe (line 109)
8. normalizeWebSocketURL (line 232)
9. cloneHeader (line 243)
10. cloneDialer (line 254)
11. mustJSON (line 263)
12. Client.SubscribeChangeEvents (line 275)

## Analysis

**Origin**: Commit a884a1500 added this as "feat(bead): add axon changeEvents websocket scaffold" with intent to support GraphQL subscriptions over WebSocket for bead lifecycle events.

**Current Usage**: Only referenced in test files:
- cli/internal/bead/axon/client_test.go
- cli/internal/bead/axon/subscription_smoke_test.go
- cli/internal/bead/axon/subscription_test.go

**Production Integration**: Zero usage in production code. The AxonBackend in axon_backend.go uses only Query operations (ReadAll, WriteAll via GraphQL mutations), not Subscribe.

**Design Context**: Per ADR-002 and FEAT-008, GraphQL subscriptions are designed for the **Web UI** (live bead updates, worker progress via graphql-ws client), not for the bead storage backend itself.

## Decision: DELETE

**Rationale**: This is speculative scaffolding added before the subscription architecture was finalized. The actual subscription consumer (web UI) uses graphql-ws directly on the frontend, not via the bead backend's subscription layer. The code has zero production integration and exists only in test fixtures.

**Action Taken**: Delete the entire subscription.go file and its associated test files. The AxonGraphQLTransport interface and Query/mutation operations in axon_backend.go remain intact for the core backend functionality.

## Files Deleted

- cli/internal/bead/axon/subscription.go (12 symbols)
- cli/internal/bead/axon/subscription_test.go (test fixtures)
- cli/internal/bead/axon/subscription_smoke_test.go (smoke test)

## Files Modified

- cli/internal/bead/axon/client_test.go: Removed lines 84-101 which tested ChangeEventFromLifecycle and ChangeEvent.ToLifecycleEvent conversions

## Verification

✓ Post-deletion deadcode check returns clean for the axon package (0 matches)
✓ go test ./internal/bead/axon -v passes
✓ lefthook run pre-commit passes (all checks green)
✓ Commit 862628965 landed successfully with [ddx-8bc79046] identifier

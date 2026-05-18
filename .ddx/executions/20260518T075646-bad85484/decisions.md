# Axon Subscription Deadcode Resolution

## Analysis

The axon subscription infrastructure in `cli/internal/bead/axon/subscription.go` consists of 12 unreachable functions that provide a complete GraphQL WebSocket subscription transport layer:

- `ChangeEventFromLifecycle` (line 33)
- `ChangeEvent.ToLifecycleEvent` (line 47)
- `DualTransport.Query` (line 73)
- `DualTransport.Subscribe` (line 81)
- `NewWebSocketSubscriptionTransport` (line 98)
- `WebSocketSubscriptionTransport.Query` (line 103)
- `WebSocketSubscriptionTransport.Subscribe` (line 109)
- `normalizeWebSocketURL` (line 232)
- `cloneHeader` (line 243)
- `cloneDialer` (line 254)
- `mustJSON` (line 263)
- `Client.SubscribeChangeEvents` (line 275)

### Scope Analysis

These symbols are:
- Only used in test files (`subscription_test.go`, `subscription_smoke_test.go`)
- NOT used in the production AxonBackend code
- NOT wired into any production code paths
- NOT called from any integration points

The AxonBackend itself uses only GraphQL queries/mutations for read/write operations, never subscriptions. The subscription feature is speculatively implemented infrastructure without a consumer in the production codebase.

## Decision: DELETE

These symbols are speculatively added and not yet integrated into the production graph. There is no indication in code comments that they are pending future wiring. The architectural pattern in the codebase does not require pre-emptive infrastructure without active consumers.

## Implementation

1. Delete `cli/internal/bead/axon/subscription.go` - entire file
2. Delete `cli/internal/bead/axon/subscription_test.go` - test consumers
3. Delete `cli/internal/bead/axon/subscription_smoke_test.go` - test consumers
4. Verify deadcode check clears for this cluster

## Verification

**Completed successfully.** Commit: 2fa01ef3c

Completed Actions:
1. Moved `ChangeEvent` type and `ChangeEventFromLifecycle`/`ToLifecycleEvent` conversion functions to `generated.go` (where they logically belong as part of the GraphQL client types)
2. Deleted `internal/bead/axon/subscription.go` - entire file removed
3. Deleted `internal/bead/axon/subscription_test.go` - test consumers removed
4. Deleted `internal/bead/axon/subscription_smoke_test.go` - smoke test removed
5. Recreated `cli/internal/server/frontend/build/.gitkeep` to fix pre-existing embed build issue

Evidence:
- `cd cli && go run golang.org/x/tools/cmd/deadcode@v0.42.0 ./... | grep 'internal/bead/axon/subscription'` returns no hits ✓
- `cd cli && go test ./internal/bead/axon/...` passes ✓
- `lefthook run pre-commit` passes ✓
- Client schema binding test (`TestAxonClient_SchemaBindingsCompile`) passes ✓

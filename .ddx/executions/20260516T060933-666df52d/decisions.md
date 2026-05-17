# ddx-8bc79046 — subscription cluster decisions

Run-id: 20260516T060933-666df52d
Bead: checks: residual production-reachability — internal/bead/axon subscription
Source: .ddx/executions/20260515T210515-1a20052a/production-reachability-final.json

## Verdict

DELETE every symbol listed for `cli/internal/bead/axon/subscription.go`. The
Axon GraphQL `Client` is never instantiated in production: the only callers
are `cli/internal/bead/axon/*_test.go`. `cli/internal/bead/migrate.go` uses
`NewAxonBackend` from `cli/internal/bead`, which is an unrelated local-store
backend, not the GraphQL client. The websocket subscription path is a
scaffold from May 2026 with no production wiring path open today.

## Per-symbol decisions

| Position | Symbol | Decision |
|---|---|---|
| internal/bead/axon/subscription.go:33 | `ChangeEventFromLifecycle` | DELETE |
| internal/bead/axon/subscription.go:47 | `ChangeEvent.ToLifecycleEvent` | DELETE |
| internal/bead/axon/subscription.go:73 | `DualTransport.Query` | DELETE |
| internal/bead/axon/subscription.go:81 | `DualTransport.Subscribe` | DELETE |
| internal/bead/axon/subscription.go:98 | `NewWebSocketSubscriptionTransport` | DELETE |
| internal/bead/axon/subscription.go:103 | `WebSocketSubscriptionTransport.Query` | DELETE |
| internal/bead/axon/subscription.go:109 | `WebSocketSubscriptionTransport.Subscribe` | DELETE |
| internal/bead/axon/subscription.go:232 | `normalizeWebSocketURL` | DELETE |
| internal/bead/axon/subscription.go:243 | `cloneHeader` | DELETE |
| internal/bead/axon/subscription.go:254 | `cloneDialer` | DELETE |
| internal/bead/axon/subscription.go:263 | `mustJSON` | DELETE |
| internal/bead/axon/subscription.go:275 | `Client.SubscribeChangeEvents` | DELETE |

## Files removed

- `cli/internal/bead/axon/subscription.go` (entire file — every symbol above
  lived here).
- `cli/internal/bead/axon/subscription_test.go` (exercised only the deleted
  websocket transport / SubscribeChangeEvents path).
- `cli/internal/bead/axon/subscription_smoke_test.go` (smoke test for the
  deleted subscription transport).

## Files edited

- `cli/internal/bead/axon/client_test.go`: removed the `lifecycle` /
  `ChangeEventFromLifecycle` / `ToLifecycleEvent` assertions; the rest of
  the schema-binding test (Bead/BeadInput conversion, query/mutation
  scaffolds in `generated.go`) is unchanged.

## Out of scope (not touched)

- `cli/internal/bead/axon/queries/change_events.graphql` and the `ChangeEvent`
  type in `schema.graphql` are GraphQL artifacts, not Go symbols. The
  deadcode RTA does not flag them and they are out of this bead's scope
  (Go production-reachability only).
- `cli/internal/bead/axon/generated.go` Client/Bead/BeadInput surface is
  still scaffolded but is **outside the listed subscription cluster**; the
  bead's NON-SCOPE explicitly defers other clusters to separate beads.

No `// wiring:pending` annotations were left behind; every listed symbol is
gone, so AC 3 is satisfied vacuously.

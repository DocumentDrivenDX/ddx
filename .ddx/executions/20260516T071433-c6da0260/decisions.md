# Decisions — ddx-8bc79046

Residual deadcode RTA cluster: `cli/internal/bead/axon/subscription.go`.

The Axon GraphQL `Client` (in `generated.go`) is not imported by any
production code path (`rg '"github.com/DocumentDrivenDX/ddx/internal/bead/axon"'`
returns no hits). The subscription scaffold in `subscription.go` was reachable
only from `subscription_test.go`, `subscription_smoke_test.go`, and a small
lifecycle-event block inside `client_test.go`. There is no production
consumer to wire it into, so per the bead's DELETE branch we drop the
subscription scaffold and the tests that exercise it.

| # | Symbol | File:Line | Decision |
|---|--------|-----------|----------|
| 1 | `ChangeEventFromLifecycle` | `internal/bead/axon/subscription.go:33` | DELETE |
| 2 | `ChangeEvent.ToLifecycleEvent` | `internal/bead/axon/subscription.go:47` | DELETE |
| 3 | `DualTransport.Query` | `internal/bead/axon/subscription.go:73` | DELETE |
| 4 | `DualTransport.Subscribe` | `internal/bead/axon/subscription.go:81` | DELETE |
| 5 | `NewWebSocketSubscriptionTransport` | `internal/bead/axon/subscription.go:98` | DELETE |
| 6 | `WebSocketSubscriptionTransport.Query` | `internal/bead/axon/subscription.go:103` | DELETE |
| 7 | `WebSocketSubscriptionTransport.Subscribe` | `internal/bead/axon/subscription.go:109` | DELETE |
| 8 | `normalizeWebSocketURL` | `internal/bead/axon/subscription.go:232` | DELETE |
| 9 | `cloneHeader` | `internal/bead/axon/subscription.go:243` | DELETE |
| 10 | `cloneDialer` | `internal/bead/axon/subscription.go:254` | DELETE |
| 11 | `mustJSON` | `internal/bead/axon/subscription.go:263` | DELETE |
| 12 | `Client.SubscribeChangeEvents` | `internal/bead/axon/subscription.go:275` | DELETE |

Companion deletions:

- `cli/internal/bead/axon/subscription.go` — removed entirely.
- `cli/internal/bead/axon/subscription_test.go` — exercised only the deleted
  `WebSocketSubscriptionTransport` / `SubscribeChangeEvents` surface.
- `cli/internal/bead/axon/subscription_smoke_test.go` — likewise.
- `cli/internal/bead/axon/client_test.go` — drop the `ChangeEventFromLifecycle`
  / `ToLifecycleEvent` lifecycle-assertion block; the surrounding `Client`
  schema-binding assertions are kept.

No `// wiring:pending` annotations were added; nothing is being deferred.

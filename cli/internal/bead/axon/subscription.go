package axon

import (
	"context"
	"errors"

	"github.com/DocumentDrivenDX/ddx/internal/bead"
)

const changeEventsSubscriptionQuery = `subscription ChangeEvents($projectID: ID!) { changeEvents(projectID: $projectID) { ...ChangeEventFields } }`

// ChangeEventsTransport is the minimal subscription transport surface used by
// the Axon scaffold. The real websocket transport can be wired in later
// without changing the call sites that consume streamed change events.
type ChangeEventsTransport interface {
	Subscribe(ctx context.Context, query string, variables map[string]any) (<-chan ChangeEvent, func(), error)
}

// SubscriptionClient wraps the transport used for GraphQL changeEvent streams.
type SubscriptionClient struct {
	transport ChangeEventsTransport
}

// NewSubscriptionClient wires a subscription transport into the scaffolded
// stream client.
func NewSubscriptionClient(transport ChangeEventsTransport) *SubscriptionClient {
	return &SubscriptionClient{transport: transport}
}

// ChangeEvents subscribes to the axon changeEvents stream for one project.
func (c *SubscriptionClient) ChangeEvents(ctx context.Context, projectID string) (<-chan ChangeEvent, func(), error) {
	if c == nil || c.transport == nil {
		return nil, nil, errors.New("axon: subscription transport is nil")
	}
	return c.transport.Subscribe(ctx, changeEventsSubscriptionQuery, map[string]any{"projectID": projectID})
}

// SubscribeLifecycle maps changeEvents into the bead lifecycle event shape so
// higher-level callers can consume the stream without knowing the GraphQL
// payload details.
func (c *SubscriptionClient) SubscribeLifecycle(ctx context.Context, projectID string) (<-chan bead.LifecycleEvent, func(), error) {
	events, cancel, err := c.ChangeEvents(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}

	out := make(chan bead.LifecycleEvent)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-events:
				if !ok {
					return
				}
				lifecycle := evt.ToLifecycleEvent()
				select {
				case out <- lifecycle:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, cancel, nil
}

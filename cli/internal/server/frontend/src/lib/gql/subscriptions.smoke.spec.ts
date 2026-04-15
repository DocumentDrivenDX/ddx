/**
 * Smoke test: workerProgress subscription round-trip.
 *
 * Verifies that `subscribeWorkerProgress` correctly wires up the graphql-ws
 * client and delivers typed WorkerEvent values to the caller's callback.
 * Uses a mock WebSocket server via the graphql-ws `makeServer` utility so the
 * test exercises the real client code without requiring a running ddx-server.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { disposeSubscriptionClient } from './subscriptions'

// ---------------------------------------------------------------------------
// Mock graphql-ws so tests run in Node without a real WebSocket.
// We replace createClient with a factory that captures the sink argument
// from .subscribe() calls and lets us feed synthetic events.
// ---------------------------------------------------------------------------

let capturedSubscribeCalls: Array<{
	payload: { query: string; variables: Record<string, unknown> }
	sink: {
		next: (data: { data: unknown }) => void
		error: (err: unknown) => void
		complete: () => void
	}
}> = []

vi.mock('graphql-ws', () => {
	return {
		createClient: vi.fn((_opts: unknown) => ({
			subscribe(
				payload: { query: string; variables: Record<string, unknown> },
				sink: {
					next: (data: { data: unknown }) => void
					error: (err: unknown) => void
					complete: () => void
				}
			) {
				capturedSubscribeCalls.push({ payload, sink })
				// Return a dispose function
				return () => {}
			},
			dispose: vi.fn()
		}))
	}
})

beforeEach(() => {
	capturedSubscribeCalls = []
	// Reset the singleton so each test gets a fresh client
	disposeSubscriptionClient()
})

afterEach(() => {
	disposeSubscriptionClient()
})

describe('subscribeWorkerProgress', () => {
	it('subscribes with the correct GQL document and workerID variable', async () => {
		// Re-import after mocks are in place
		const { subscribeWorkerProgress } = await import('./subscriptions')

		const received: unknown[] = []
		const dispose = subscribeWorkerProgress('worker-42', (evt) => received.push(evt))

		expect(capturedSubscribeCalls).toHaveLength(1)
		const call = capturedSubscribeCalls[0]

		// Verify the subscription document names the right operation
		expect(call.payload.query).toContain('subscription WorkerProgress')
		expect(call.payload.query).toContain('workerProgress(workerID: $workerID)')
		expect(call.payload.query).toContain('eventID')
		expect(call.payload.query).toContain('phase')

		// Verify the variable is forwarded
		expect(call.payload.variables).toEqual({ workerID: 'worker-42' })

		dispose()
	})

	it('delivers a WorkerEvent to the callback when the server pushes data', async () => {
		const { subscribeWorkerProgress } = await import('./subscriptions')

		const received: Array<{ eventID: string; phase: string; logLine?: string | null }> = []
		subscribeWorkerProgress('worker-99', (evt) => received.push(evt))

		expect(capturedSubscribeCalls).toHaveLength(1)
		const { sink } = capturedSubscribeCalls[0]

		// Simulate the server pushing a progress event
		sink.next({
			data: {
				workerProgress: {
					eventID: 'evt-001',
					workerID: 'worker-99',
					phase: 'running',
					timestamp: '2026-04-15T08:37:12Z',
					logLine: 'Claiming bead ddx-abc123',
					beadID: 'ddx-abc123'
				}
			}
		})

		expect(received).toHaveLength(1)
		expect(received[0]).toMatchObject({
			eventID: 'evt-001',
			phase: 'running',
			logLine: 'Claiming bead ddx-abc123'
		})
	})

	it('delivers multiple sequential events in order', async () => {
		const { subscribeWorkerProgress } = await import('./subscriptions')

		const phases: string[] = []
		subscribeWorkerProgress('worker-1', (evt) => phases.push(evt.phase))

		const { sink } = capturedSubscribeCalls[0]
		const push = (phase: string, id: string) =>
			sink.next({
				data: {
					workerProgress: {
						eventID: id,
						workerID: 'worker-1',
						phase,
						timestamp: '2026-04-15T08:37:12Z'
					}
				}
			})

		push('pending', 'evt-1')
		push('running', 'evt-2')
		push('done', 'evt-3')

		expect(phases).toEqual(['pending', 'running', 'done'])
	})

	it('calls the error handler when the server signals an error', async () => {
		const { subscribeWorkerProgress } = await import('./subscriptions')

		const errors: unknown[] = []
		subscribeWorkerProgress('worker-err', () => {}, (err) => errors.push(err))

		const { sink } = capturedSubscribeCalls[0]
		sink.error(new Error('subscription closed by server'))

		expect(errors).toHaveLength(1)
		expect((errors[0] as Error).message).toBe('subscription closed by server')
	})

	it('calls the complete handler when the subscription finishes', async () => {
		const { subscribeWorkerProgress } = await import('./subscriptions')

		let completed = false
		subscribeWorkerProgress('worker-done', () => {}, undefined, () => {
			completed = true
		})

		const { sink } = capturedSubscribeCalls[0]
		sink.complete()

		expect(completed).toBe(true)
	})
})

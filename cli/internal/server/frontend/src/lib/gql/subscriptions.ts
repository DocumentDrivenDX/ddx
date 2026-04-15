import { createClient, type Client } from 'graphql-ws'

// ---------------------------------------------------------------------------
// Subscription client singleton
// ---------------------------------------------------------------------------

let _subClient: Client | null = null

function resolveWsUrl(): string {
	if (typeof window === 'undefined') {
		// Node / SSR — subscriptions are client-only, so this path is never
		// reached in practice, but we provide a sensible default for testing.
		return 'ws://localhost:7743/graphql'
	}
	const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
	return `${proto}://${window.location.host}/graphql`
}

/**
 * Returns the shared graphql-ws client instance, creating it on first call.
 * Accepts an optional URL override (used in tests).
 */
export function getSubscriptionClient(urlOverride?: string): Client {
	if (!_subClient) {
		_subClient = createClient({ url: urlOverride ?? resolveWsUrl() })
	}
	return _subClient
}

/** Tear down the singleton — call between tests or on hot-reload. */
export function disposeSubscriptionClient(): void {
	if (_subClient) {
		_subClient.dispose()
		_subClient = null
	}
}

// ---------------------------------------------------------------------------
// Typed event shapes (mirror schema.graphql subscription event types)
// ---------------------------------------------------------------------------

export interface WorkerEvent {
	eventID: string
	workerID: string
	/** Execution phase: "pending" | "running" | "done" | "error" */
	phase: string
	/** ISO-8601 timestamp */
	timestamp: string
	logLine?: string | null
	beadID?: string | null
}

// ---------------------------------------------------------------------------
// subscribeWorkerProgress
// ---------------------------------------------------------------------------

const WORKER_PROGRESS_SUBSCRIPTION = `
subscription WorkerProgress($workerID: ID!) {
  workerProgress(workerID: $workerID) {
    eventID
    workerID
    phase
    timestamp
    logLine
    beadID
  }
}
`

/**
 * Subscribe to live progress events for a given worker.
 *
 * Returns a dispose function — call it to unsubscribe and free resources.
 *
 * @example
 * const dispose = subscribeWorkerProgress('worker-123', (evt) => {
 *   logLines.push(evt.logLine ?? '')
 * })
 * onDestroy(dispose)
 */
export function subscribeWorkerProgress(
	workerID: string,
	onEvent: (event: WorkerEvent) => void,
	onError?: (err: unknown) => void,
	onComplete?: () => void
): () => void {
	const client = getSubscriptionClient()
	return client.subscribe(
		{ query: WORKER_PROGRESS_SUBSCRIPTION, variables: { workerID } },
		{
			next(data) {
				const evt = (data.data as Record<string, unknown> | null | undefined)
					?.workerProgress as WorkerEvent | undefined
				if (evt) onEvent(evt)
			},
			error(err) {
				if (onError) {
					onError(err)
				} else {
					console.error('[ddx] workerProgress subscription error:', err)
				}
			},
			complete() {
				onComplete?.()
			}
		}
	)
}

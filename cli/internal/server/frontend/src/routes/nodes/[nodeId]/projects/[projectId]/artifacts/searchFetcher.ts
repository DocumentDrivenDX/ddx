/**
 * Monotonic request sequence used to drop stale responses on race.
 *
 * Each `next()` call issues a fresh token and marks it as the latest.
 * `isCurrent(token)` returns true only if no newer token has been issued
 * (and the sequence has not been explicitly invalidated). `invalidate()`
 * drops all in-flight tokens without issuing a new one — useful when an
 * upstream input (e.g. the search query) changes and any pending response
 * should be discarded.
 */
export interface RequestSequence {
	next(): number
	isCurrent(token: number): boolean
	invalidate(): void
}

export function createRequestSequence(): RequestSequence {
	let issued = 0
	let latest = 0
	return {
		next() {
			issued += 1
			latest = issued
			return issued
		},
		isCurrent(token: number) {
			return token === latest && token !== 0
		},
		invalidate() {
			latest = 0
		}
	}
}

/**
 * Run an async fn under a request sequence. Returns the result if the
 * call is still the latest when it resolves; otherwise returns
 * `{ stale: true }`. Errors propagate; stale-after-error is silently
 * swallowed so callers do not surface exceptions from canceled requests.
 */
export async function runLatest<T>(
	seq: RequestSequence,
	fn: () => Promise<T>
): Promise<{ stale: false; value: T } | { stale: true }> {
	const token = seq.next()
	try {
		const value = await fn()
		if (!seq.isCurrent(token)) return { stale: true }
		return { stale: false, value }
	} catch (err) {
		if (!seq.isCurrent(token)) return { stale: true }
		throw err
	}
}

import { describe, it, expect } from 'vitest'
import { createRequestSequence, runLatest } from './searchFetcher'

function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void; reject: (e: unknown) => void } {
	let resolve!: (v: T) => void
	let reject!: (e: unknown) => void
	const promise = new Promise<T>((res, rej) => {
		resolve = res
		reject = rej
	})
	return { promise, resolve, reject }
}

describe('createRequestSequence', () => {
	it('marks only the latest token as current', () => {
		const seq = createRequestSequence()
		const a = seq.next()
		const b = seq.next()
		expect(seq.isCurrent(a)).toBe(false)
		expect(seq.isCurrent(b)).toBe(true)
	})

	it('invalidate() drops all in-flight tokens', () => {
		const seq = createRequestSequence()
		const t = seq.next()
		seq.invalidate()
		expect(seq.isCurrent(t)).toBe(false)
	})

	it('returns false for a never-issued token', () => {
		const seq = createRequestSequence()
		expect(seq.isCurrent(0)).toBe(false)
		expect(seq.isCurrent(99)).toBe(false)
	})
})

describe('runLatest', () => {
	it('returns the value when no newer call superseded it', async () => {
		const seq = createRequestSequence()
		const result = await runLatest(seq, async () => 42)
		expect(result).toEqual({ stale: false, value: 42 })
	})

	it('drops the older response when a newer call resolves first', async () => {
		// Simulates the artifacts search race: user types "fo", then "foo"; the
		// "fo" response (page 1) arrives AFTER "foo" page 1 has already painted.
		const seq = createRequestSequence()
		const older = deferred<string>()
		const newer = deferred<string>()

		const olderPromise = runLatest(seq, () => older.promise)
		const newerPromise = runLatest(seq, () => newer.promise)

		newer.resolve('foo-results')
		const newerOutcome = await newerPromise
		expect(newerOutcome).toEqual({ stale: false, value: 'foo-results' })

		older.resolve('fo-results')
		const olderOutcome = await olderPromise
		expect(olderOutcome).toEqual({ stale: true })
	})

	it('drops a response invalidated mid-flight (e.g. search changed)', async () => {
		const seq = createRequestSequence()
		const d = deferred<string>()
		const promise = runLatest(seq, () => d.promise)
		// User typed a new query; caller invalidates the in-flight loadMore.
		seq.invalidate()
		d.resolve('page2')
		expect(await promise).toEqual({ stale: true })
	})

	it('treats successive paginated calls (page 1, page 2) as non-stale when uncontended', async () => {
		// Correctness past page 1: when the user has not changed the search
		// while paging, both responses are honored in order.
		const seq = createRequestSequence()
		type Page = { edges: number[]; cursor: string | null }
		const page1: Page = { edges: [1, 2, 3], cursor: 'c1' }
		const page2: Page = { edges: [4, 5, 6], cursor: null }

		const r1 = await runLatest(seq, async () => page1)
		expect(r1).toEqual({ stale: false, value: page1 })

		const r2 = await runLatest(seq, async () => page2)
		expect(r2).toEqual({ stale: false, value: page2 })
	})

	it('swallows errors from stale responses but propagates fresh errors', async () => {
		const seq = createRequestSequence()
		const stale = deferred<never>()
		const stalePromise = runLatest(seq, () => stale.promise)
		seq.next() // newer call supersedes
		stale.reject(new Error('boom'))
		expect(await stalePromise).toEqual({ stale: true })

		const seq2 = createRequestSequence()
		await expect(runLatest(seq2, async () => { throw new Error('x') })).rejects.toThrow('x')
	})
})

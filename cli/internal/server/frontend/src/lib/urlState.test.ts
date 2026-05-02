import { describe, it, expect } from 'vitest'
import { readState, writeState, backHref, DEFAULT_GROUP_BY } from './urlState'

describe('urlState.readState', () => {
	it('returns defaults for an empty URL', () => {
		const s = readState(new URLSearchParams(''))
		expect(s.q).toBe('')
		expect(s.mediaType).toBeNull()
		expect(s.groupBy).toBe(DEFAULT_GROUP_BY)
		expect(s.sort).toBeNull()
		expect(s.filters).toEqual({})
	})

	it('parses known keys and filter.* entries', () => {
		const s = readState(
			new URLSearchParams('q=foo&mediaType=text/markdown&groupBy=prefix&sort=title&filter.tag=v1')
		)
		expect(s.q).toBe('foo')
		expect(s.mediaType).toBe('text/markdown')
		expect(s.groupBy).toBe('prefix')
		expect(s.sort).toBe('title')
		expect(s.filters).toEqual({ tag: 'v1' })
	})

	it('falls back to default groupBy on unknown values', () => {
		const s = readState(new URLSearchParams('groupBy=nonsense'))
		expect(s.groupBy).toBe(DEFAULT_GROUP_BY)
	})
})

describe('urlState.writeState', () => {
	it('preserves unrelated keys, including back', () => {
		const next = writeState(new URLSearchParams('back=/x?y=1&other=keep'), { q: 'hi' })
		expect(next.get('back')).toBe('/x?y=1')
		expect(next.get('other')).toBe('keep')
		expect(next.get('q')).toBe('hi')
	})

	it('drops groupBy when it equals the default', () => {
		const next = writeState(new URLSearchParams('groupBy=prefix'), { groupBy: 'folder' })
		expect(next.has('groupBy')).toBe(false)
	})

	it('deletes keys when given empty/null values', () => {
		const next = writeState(new URLSearchParams('q=old&mediaType=x'), {
			q: '',
			mediaType: null
		})
		expect(next.has('q')).toBe(false)
		expect(next.has('mediaType')).toBe(false)
	})

	it('replaces filter.* entries atomically', () => {
		const next = writeState(new URLSearchParams('filter.a=1&filter.b=2&q=keep'), {
			filters: { c: '3' }
		})
		expect(next.has('filter.a')).toBe(false)
		expect(next.has('filter.b')).toBe(false)
		expect(next.get('filter.c')).toBe('3')
		expect(next.get('q')).toBe('keep')
	})
})

describe('urlState.backHref', () => {
	it('omits the trailing ? when there are no params', () => {
		expect(backHref('/x', new URLSearchParams(''))).toBe('/x')
	})
	it('joins path and params', () => {
		expect(backHref('/x', new URLSearchParams('q=1'))).toBe('/x?q=1')
	})
})

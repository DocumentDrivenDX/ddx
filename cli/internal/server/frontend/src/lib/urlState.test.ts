import { describe, it, expect } from 'vitest';
import { KNOWN_KEYS, readState, writeState, backHref, DEFAULT_GROUP_BY } from './urlState';

describe('urlState.readState', () => {
	it('returns defaults for an empty URL', () => {
		const s = readState(new URLSearchParams(''));
		expect(s.q).toBe('');
		expect(s.mediaType).toBeNull();
		expect(s.groupBy).toBe(DEFAULT_GROUP_BY);
		expect(s.sort).toBeNull();
		expect(s.staleness).toBeNull();
		expect(s.phase).toBeNull();
		expect(s.prefix).toEqual([]);
		expect(s.filters).toEqual({});
	});

	it('parses all reserved facet keys (q, mediaType, staleness, phase, prefix, sort) plus groupBy', () => {
		const s = readState(
			new URLSearchParams(
				'q=foo&mediaType=text/markdown&staleness=fresh&phase=01-frame&prefix=ADR,FEAT&sort=TITLE&groupBy=workflowStage'
			)
		);
		expect(s.q).toBe('foo');
		expect(s.mediaType).toBe('text/markdown');
		expect(s.staleness).toBe('fresh');
		expect(s.phase).toBe('01-frame');
		expect(s.prefix).toEqual(['ADR', 'FEAT']);
		expect(s.sort).toBe('TITLE');
		expect(s.groupBy).toBe('workflowStage');
	});

	it('parses prefix as a comma-separated multi-value list and trims empties', () => {
		const s = readState(new URLSearchParams('prefix=ADR,,FEAT, US '));
		expect(s.prefix).toEqual(['ADR', 'FEAT', 'US']);
	});

	it('parses known keys and filter.* entries', () => {
		const s = readState(
			new URLSearchParams(
				'q=foo&mediaType=text/markdown&groupBy=prefix&sort=title&staleness=stale&filter.tag=v1'
			)
		);
		expect(s.q).toBe('foo');
		expect(s.mediaType).toBe('text/markdown');
		expect(s.groupBy).toBe('prefix');
		expect(s.sort).toBe('title');
		expect(s.staleness).toBe('stale');
		expect(s.filters).toEqual({ tag: 'v1' });
	});

	it('falls back to default groupBy on unknown values', () => {
		const s = readState(new URLSearchParams('groupBy=nonsense'));
		expect(s.groupBy).toBe(DEFAULT_GROUP_BY);
	});

	it('accepts the workflowStage axis in query params', () => {
		const s = readState(new URLSearchParams('groupBy=workflowStage'));
		expect(s.groupBy).toBe('workflowStage');
	});
});

describe('urlState contract', () => {
	it('publishes the reserved query keys', () => {
		expect(Array.from(KNOWN_KEYS).sort()).toEqual([
			'groupBy',
			'mediaType',
			'phase',
			'prefix',
			'q',
			'sort',
			'staleness'
		]);
	});

	it('round-trips all reserved facets while preserving unrelated params', () => {
		const params = new URLSearchParams(
			'back=%2Fnodes%2Fnode-1%2Fprojects%2Fproject-1%2Fartifacts&filter.owner=team'
		);

		const next = writeState(params, {
			q: 'needle',
			mediaType: 'text/markdown',
			groupBy: 'prefix',
			sort: 'TITLE',
			staleness: 'fresh',
			phase: '01-frame',
			prefix: ['ADR', 'FEAT']
		});

		expect(next.get('back')).toBe('/nodes/node-1/projects/project-1/artifacts');
		expect(next.get('filter.owner')).toBe('team');
		expect(next.get('q')).toBe('needle');
		expect(next.get('mediaType')).toBe('text/markdown');
		expect(next.get('groupBy')).toBe('prefix');
		expect(next.get('sort')).toBe('TITLE');
		expect(next.get('staleness')).toBe('fresh');
		expect(next.get('phase')).toBe('01-frame');
		expect(next.get('prefix')).toBe('ADR,FEAT');

		expect(readState(next)).toEqual({
			q: 'needle',
			mediaType: 'text/markdown',
			groupBy: 'prefix',
			sort: 'TITLE',
			staleness: 'fresh',
			phase: '01-frame',
			prefix: ['ADR', 'FEAT'],
			filters: { owner: 'team' }
		});
	});
});

describe('urlState.writeState', () => {
	it('preserves unrelated keys, including back', () => {
		const next = writeState(new URLSearchParams('back=/x?y=1&other=keep'), { q: 'hi' });
		expect(next.get('back')).toBe('/x?y=1');
		expect(next.get('other')).toBe('keep');
		expect(next.get('q')).toBe('hi');
	});

	it('drops groupBy when it equals the default', () => {
		const next = writeState(new URLSearchParams('groupBy=prefix'), { groupBy: 'folder' });
		expect(next.has('groupBy')).toBe(false);
	});

	it('round-trips workflowStage through the query string', () => {
		const next = writeState(new URLSearchParams(''), { groupBy: 'workflowStage' });
		expect(next.get('groupBy')).toBe('workflowStage');
		expect(readState(next).groupBy).toBe('workflowStage');
	});

	it('deletes keys when given empty/null values', () => {
		const next = writeState(new URLSearchParams('q=old&mediaType=x'), {
			q: '',
			mediaType: null
		});
		expect(next.has('q')).toBe(false);
		expect(next.has('mediaType')).toBe(false);
	});

	it('round-trips sort and staleness, and clears them on null', () => {
		const set = writeState(new URLSearchParams(''), { sort: 'TITLE', staleness: 'stale' });
		expect(set.get('sort')).toBe('TITLE');
		expect(set.get('staleness')).toBe('stale');
		expect(readState(set).sort).toBe('TITLE');
		expect(readState(set).staleness).toBe('stale');

		const cleared = writeState(set, { sort: null, staleness: null });
		expect(cleared.has('sort')).toBe(false);
		expect(cleared.has('staleness')).toBe(false);
	});

	it('round-trips phase and prefix, clears them on null/empty', () => {
		const set = writeState(new URLSearchParams(''), {
			phase: '01-frame',
			prefix: ['ADR', 'FEAT']
		});
		expect(set.get('phase')).toBe('01-frame');
		expect(set.get('prefix')).toBe('ADR,FEAT');
		expect(readState(set).phase).toBe('01-frame');
		expect(readState(set).prefix).toEqual(['ADR', 'FEAT']);

		const cleared = writeState(set, { phase: null, prefix: [] });
		expect(cleared.has('phase')).toBe(false);
		expect(cleared.has('prefix')).toBe(false);
	});

	it('composes phase and prefix with q/mediaType/staleness/sort without dropping any', () => {
		const next = writeState(new URLSearchParams(''), {
			q: 'spec',
			mediaType: 'text/markdown',
			staleness: 'fresh',
			phase: '02-design',
			prefix: ['ADR'],
			sort: 'TITLE'
		});
		expect(next.get('q')).toBe('spec');
		expect(next.get('mediaType')).toBe('text/markdown');
		expect(next.get('staleness')).toBe('fresh');
		expect(next.get('phase')).toBe('02-design');
		expect(next.get('prefix')).toBe('ADR');
		expect(next.get('sort')).toBe('TITLE');
	});

	it('replaces filter.* entries atomically', () => {
		const next = writeState(new URLSearchParams('filter.a=1&filter.b=2&q=keep'), {
			filters: { c: '3' }
		});
		expect(next.has('filter.a')).toBe(false);
		expect(next.has('filter.b')).toBe(false);
		expect(next.get('filter.c')).toBe('3');
		expect(next.get('q')).toBe('keep');
	});
});

describe('urlState.backHref', () => {
	it('omits the trailing ? when there are no params', () => {
		expect(backHref('/x', new URLSearchParams(''))).toBe('/x');
	});
	it('joins path and params', () => {
		expect(backHref('/x', new URLSearchParams('q=1'))).toBe('/x?q=1');
	});
});

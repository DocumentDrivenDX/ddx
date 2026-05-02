// Shared URL-state helper for list views (artifacts, etc.).
// Reads/writes the well-known query params (q, mediaType, groupBy, sort,
// filters.*) while preserving any unrelated keys, including ?back=.

export type GroupBy = 'folder' | 'prefix' | 'mediaType'

const KNOWN_KEYS = new Set(['q', 'mediaType', 'groupBy', 'sort', 'staleness'])
const FILTER_PREFIX = 'filter.'

export interface ListURLState {
	q: string
	mediaType: string | null
	groupBy: GroupBy
	sort: string | null
	staleness: string | null
	filters: Record<string, string>
}

export const DEFAULT_GROUP_BY: GroupBy = 'folder'

function parseGroupBy(raw: string | null): GroupBy {
	if (raw === 'prefix' || raw === 'mediaType' || raw === 'folder') return raw
	return DEFAULT_GROUP_BY
}

export function readState(params: URLSearchParams): ListURLState {
	const filters: Record<string, string> = {}
	for (const [k, v] of params.entries()) {
		if (k.startsWith(FILTER_PREFIX)) filters[k.slice(FILTER_PREFIX.length)] = v
	}
	return {
		q: params.get('q') ?? '',
		mediaType: params.get('mediaType'),
		groupBy: parseGroupBy(params.get('groupBy')),
		sort: params.get('sort'),
		staleness: params.get('staleness'),
		filters
	}
}

// writeState mutates a copy of `params` with the patch applied. Unrelated
// keys (including `back`) are preserved. Empty/null values delete the key.
export function writeState(
	params: URLSearchParams,
	patch: Partial<ListURLState>
): URLSearchParams {
	const next = new URLSearchParams(params)

	if ('q' in patch) {
		if (patch.q) next.set('q', patch.q)
		else next.delete('q')
	}
	if ('mediaType' in patch) {
		if (patch.mediaType) next.set('mediaType', patch.mediaType)
		else next.delete('mediaType')
	}
	if ('groupBy' in patch) {
		if (patch.groupBy && patch.groupBy !== DEFAULT_GROUP_BY) next.set('groupBy', patch.groupBy)
		else next.delete('groupBy')
	}
	if ('sort' in patch) {
		if (patch.sort) next.set('sort', patch.sort)
		else next.delete('sort')
	}
	if ('staleness' in patch) {
		if (patch.staleness) next.set('staleness', patch.staleness)
		else next.delete('staleness')
	}
	if ('filters' in patch && patch.filters) {
		// Replace only the filter.* keys; leave everything else intact.
		for (const k of Array.from(next.keys())) {
			if (k.startsWith(FILTER_PREFIX)) next.delete(k)
		}
		for (const [k, v] of Object.entries(patch.filters)) {
			if (v) next.set(FILTER_PREFIX + k, v)
		}
	}

	return next
}

// Build a back-href that captures the current pathname + relevant search.
export function backHref(pathname: string, params: URLSearchParams): string {
	const s = params.toString()
	return s ? `${pathname}?${s}` : pathname
}

export { KNOWN_KEYS, FILTER_PREFIX }

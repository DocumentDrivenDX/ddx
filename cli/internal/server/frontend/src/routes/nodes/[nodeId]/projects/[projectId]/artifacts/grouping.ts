// Group derivations for the artifacts list page.
// Pure, regex-based; safe to run on every render.

export type GroupBy = 'folder' | 'prefix' | 'mediaType'

// folderOf("docs/helix/01-frame/prd.md") => "docs/helix/01-frame"
// folderOf("README.md") => "/"
const FOLDER_RE = /^(.*)\/[^/]+$/
export function folderOf(path: string): string {
	const m = path.match(FOLDER_RE)
	if (!m) return '/'
	return m[1] || '/'
}

// prefixOf("docs/helix/01-frame/prd.md") => "docs"
// prefixOf("/library/personas/x.md") => "library"
const PREFIX_RE = /^\/?([^/]+)/
export function prefixOf(path: string): string {
	const m = path.match(PREFIX_RE)
	return m ? m[1] : ''
}

// Workflow stage derivation. Matches the canonical HELIX-style numbered
// stage segment (e.g. 01-frame, 02-design) anywhere in the path. Returns
// the bare stage name lowercased, or null when no stage segment is found.
const STAGE_RE = /(?:^|\/)\d{2}-([a-z][a-z0-9-]*)(?:\/|$)/i
export function workflowStageOf(path: string): string | null {
	const m = path.match(STAGE_RE)
	return m ? m[1].toLowerCase() : null
}

// Whether the workflow-stage axis should be shown for the given paths.
// The axis is hidden when no artifact in view has a derivable stage.
export function axisAvailable(paths: Iterable<string>): boolean {
	for (const p of paths) {
		if (workflowStageOf(p) !== null) return true
	}
	return false
}

export interface Groupable {
	path: string
	mediaType: string
}

export function groupKey(item: Groupable, by: GroupBy): string {
	switch (by) {
		case 'folder':
			return folderOf(item.path)
		case 'prefix':
			return prefixOf(item.path)
		case 'mediaType':
			return item.mediaType || 'unknown'
	}
}

export function groupItems<T extends Groupable>(
	items: T[],
	by: GroupBy
): { key: string; items: T[] }[] {
	const buckets = new Map<string, T[]>()
	for (const it of items) {
		const k = groupKey(it, by)
		const arr = buckets.get(k)
		if (arr) arr.push(it)
		else buckets.set(k, [it])
	}
	return Array.from(buckets.entries())
		.sort(([a], [b]) => a.localeCompare(b))
		.map(([key, items]) => ({ key, items }))
}

export const GROUP_BY_LABELS: Record<GroupBy, string> = {
	folder: 'Folder',
	prefix: 'Prefix',
	mediaType: 'Media type'
}

// Generic, project-neutral label for the workflow-stage axis.
export const WORKFLOW_STAGE_LABEL = 'Workflow stage'

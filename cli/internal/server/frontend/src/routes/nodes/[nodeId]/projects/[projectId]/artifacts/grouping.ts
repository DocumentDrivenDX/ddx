// Group derivations for the artifacts list page.
// Pure, regex-based; safe to run on every render.

export type GroupBy = import('$lib/urlState').GroupBy;
import { prefixOf } from '$lib/artifacts/grouping';

// folderOf("docs/helix/01-frame/prd.md") => "docs/helix/01-frame"
// folderOf("README.md") => "/"
const FOLDER_RE = /^(.*)\/[^/]+$/;
export function folderOf(path: string): string {
	const m = path.match(FOLDER_RE);
	if (!m) return '/';
	return m[1] || '/';
}

export { prefixOf };

// Workflow stage derivation. Matches the canonical HELIX-style numbered
// stage segment (e.g. 01-frame, 02-design) anywhere in the path. Returns
// the bare stage name lowercased, or null when no stage segment is found.
const STAGE_RE = /(?:^|\/)\d{2}-([a-z][a-z0-9-]*)(?:\/|$)/i;
export function workflowStageOf(path: string): string | null {
	const m = path.match(STAGE_RE);
	return m ? m[1].toLowerCase() : null;
}

// Whether the workflow-stage axis should be shown for the given paths.
// The axis is hidden when no artifact in view has a derivable stage.
export function axisAvailable(paths: Iterable<string>): boolean {
	for (const p of paths) {
		if (workflowStageOf(p) !== null) return true;
	}
	return false;
}

export interface Groupable {
	path: string;
	mediaType: string;
}

// Generic, project-neutral label for the workflow-stage axis.
export const WORKFLOW_STAGE_LABEL = 'Workflow stage';

export const WORKFLOW_STAGE_FALLBACK_LABEL = 'Unstaged';

export function groupKey(item: Groupable, by: GroupBy): string {
	switch (by) {
		case 'folder':
			return folderOf(item.path);
		case 'prefix':
			return prefixOf(item.path);
		case 'mediaType':
			return item.mediaType || 'unknown';
		case 'workflowStage':
			return workflowStageOf(item.path) ?? WORKFLOW_STAGE_FALLBACK_LABEL;
	}
}

export function groupItems<T extends Groupable>(
	items: T[],
	by: GroupBy
): { key: string; items: T[] }[] {
	const buckets = new Map<string, T[]>();
	for (const it of items) {
		const k = groupKey(it, by);
		const arr = buckets.get(k);
		if (arr) arr.push(it);
		else buckets.set(k, [it]);
	}
	return Array.from(buckets.entries())
		.sort(([a], [b]) => a.localeCompare(b))
		.map(([key, items]) => ({ key, items }));
}

export const GROUP_BY_LABELS: Record<GroupBy, string> = {
	folder: 'Folder',
	prefix: 'Prefix',
	mediaType: 'Media type',
	workflowStage: WORKFLOW_STAGE_LABEL
};

export const GROUP_BY_OPTIONS: { value: GroupBy; label: string }[] = [
	{ value: 'folder', label: GROUP_BY_LABELS.folder },
	{ value: 'prefix', label: GROUP_BY_LABELS.prefix },
	{ value: 'mediaType', label: GROUP_BY_LABELS.mediaType },
	{ value: 'workflowStage', label: WORKFLOW_STAGE_LABEL }
];

export function groupCountLabel(
	groupCount: number,
	loadedCount: number,
	hasNextPage: boolean
): string {
	return hasNextPage ? `${groupCount} of ${loadedCount} loaded` : `${groupCount}`;
}

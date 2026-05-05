import type { GroupBy } from './grouping';

export interface GroupByStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
	removeItem(key: string): void;
}

export function groupByStorageKey(projectId: string): string {
	return `artifacts:groupBy:${projectId}`;
}

export function readPersistedGroupBy(storage: GroupByStorage, projectId: string): GroupBy | null {
	const raw = storage.getItem(groupByStorageKey(projectId));
	if (raw === 'folder' || raw === 'prefix' || raw === 'mediaType' || raw === 'workflowStage') {
		return raw;
	}
	return null;
}

export function writePersistedGroupBy(
	storage: GroupByStorage,
	projectId: string,
	groupBy: GroupBy
): void {
	storage.setItem(groupByStorageKey(projectId), groupBy);
}

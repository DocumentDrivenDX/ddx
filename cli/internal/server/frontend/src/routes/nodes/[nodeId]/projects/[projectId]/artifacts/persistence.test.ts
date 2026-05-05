import { describe, it, expect } from 'vitest';
import { readPersistedGroupBy, writePersistedGroupBy } from './persistence';

class MemoryStorage {
	private items = new Map<string, string>();

	getItem(key: string): string | null {
		return this.items.get(key) ?? null;
	}

	setItem(key: string, value: string): void {
		this.items.set(key, value);
	}

	removeItem(key: string): void {
		this.items.delete(key);
	}
}

describe('artifacts groupBy persistence', () => {
	it('round-trips the selected groupBy per project', () => {
		const storage = new MemoryStorage();
		writePersistedGroupBy(storage, 'project-a', 'workflowStage');
		expect(readPersistedGroupBy(storage, 'project-a')).toBe('workflowStage');
	});

	it('keeps project keys isolated', () => {
		const storage = new MemoryStorage();
		writePersistedGroupBy(storage, 'project-a', 'prefix');
		expect(readPersistedGroupBy(storage, 'project-b')).toBeNull();
	});

	it('ignores unknown stored values', () => {
		const storage = new MemoryStorage();
		storage.setItem('artifacts:groupBy:project-a', 'nope');
		expect(readPersistedGroupBy(storage, 'project-a')).toBeNull();
	});
});

import { describe, expect, it } from 'vitest';
import ConfirmDialog from './ConfirmDialog.svelte';
import Tooltip from './Tooltip.svelte';
import TypedConfirmDialog, { typedConfirmMatches } from './TypedConfirmDialog.svelte';

describe('shared UI primitives', () => {
	it('compile as importable Svelte components', () => {
		expect(ConfirmDialog).toBeTruthy();
		expect(TypedConfirmDialog).toBeTruthy();
		expect(Tooltip).toBeTruthy();
	});

	it('gates typed confirmation on an exact expected text match', () => {
		expect(typedConfirmMatches('ddx-84daf44b', 'ddx-84daf44b')).toBe(true);
		expect(typedConfirmMatches('DDX-84DAF44B', 'ddx-84daf44b')).toBe(false);
		expect(typedConfirmMatches(' ddx-84daf44b ', 'ddx-84daf44b')).toBe(false);
	});
});

import { describe, it, expect } from 'vitest';
import { BEAD_STATUS_OPTIONS, beadStatusWireValue } from './beadStatusOptions';

describe('BEAD_STATUS_OPTIONS', () => {
	const EXPECTED = ['open', 'in-progress', 'blocked', 'closed', 'proposed', 'cancelled'];

	it('contains exactly the six lifecycle statuses', () => {
		expect(BEAD_STATUS_OPTIONS).toHaveLength(6);
		for (const s of EXPECTED) {
			expect(BEAD_STATUS_OPTIONS).toContain(s);
		}
	});

	it('does NOT contain "ready" (removed legacy pseudo-status)', () => {
		expect(BEAD_STATUS_OPTIONS).not.toContain('ready');
	});

	it('contains "proposed"', () => {
		expect(BEAD_STATUS_OPTIONS).toContain('proposed');
	});

	it('contains "cancelled"', () => {
		expect(BEAD_STATUS_OPTIONS).toContain('cancelled');
	});
});

describe('beadStatusWireValue', () => {
	it('converts "in-progress" to "in_progress"', () => {
		expect(beadStatusWireValue('in-progress')).toBe('in_progress');
	});

	it('leaves statuses without hyphens unchanged', () => {
		expect(beadStatusWireValue('open')).toBe('open');
		expect(beadStatusWireValue('proposed')).toBe('proposed');
		expect(beadStatusWireValue('cancelled')).toBe('cancelled');
		expect(beadStatusWireValue('blocked')).toBe('blocked');
		expect(beadStatusWireValue('closed')).toBe('closed');
	});
});

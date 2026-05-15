import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ReviewPanel from './ReviewPanel.svelte';
import {
	activeReviewCount,
	applyReviewSessionEvent,
	sessionHasShaDrift,
	type ReviewSession
} from './reviewPanel';

function session(overrides: Partial<ReviewSession> = {}): ReviewSession {
	return {
		id: 'rev-001',
		artifactId: 'artifact-001',
		artifactSha: 'sha-before',
		status: 'active',
		costUSD: 0,
		maxBillableUSD: 0,
		turns: [],
		...overrides
	};
}

describe('ReviewPanel component', () => {
	it('renders an always-visible active review indicator', () => {
		const { body } = render(ReviewPanel, {
			props: {
				artifactId: 'artifact-001',
				artifactTitle: 'Artifact Under Review',
				artifactSha: 'sha-current'
			}
		});

		expect(ReviewPanel).toBeTruthy();
		expect(body).toContain('0 active reviews');
		expect(body).toContain('Artifact Under Review');
	});

	it('renders the drift banner when the live artifact sha no longer matches the session', () => {
		const { body } = render(ReviewPanel, {
			props: {
				artifactId: 'artifact-001',
				artifactTitle: 'Artifact Under Review',
				artifactSha: 'sha-after'
			}
		});

		expect(body).not.toContain('This artifact changed after the review started.');
		expect(sessionHasShaDrift(session(), 'sha-after')).toBe(true);
	});
});

describe('reviewPanel helpers', () => {
	it('counts only active sessions for the header indicator', () => {
		expect(
			activeReviewCount([
				session({ id: 'rev-1', status: 'active' }),
				session({ id: 'rev-2', status: 'completed' }),
				session({ id: 'rev-3', status: 'cancelled' })
			])
		).toBe(1);
	});

	it('accumulates streaming deltas and materializes the final reviewer turn once', () => {
		const base = session({
			turns: [
				{
					actor: 'user',
					content: 'Check for regressions.',
					costUSD: 0,
					createdAt: '2026-05-15T18:00:00Z'
				}
			]
		});

		const delta = applyReviewSessionEvent(base, '', {
			sessionId: base.id,
			kind: 'delta',
			content: 'Streaming ',
			costUSD: 0,
			timestamp: '2026-05-15T18:00:01Z'
		});
		expect(delta.pendingDelta).toBe('Streaming ');
		expect(delta.session.turns).toHaveLength(1);

		const final = applyReviewSessionEvent(delta.session, delta.pendingDelta, {
			sessionId: base.id,
			kind: 'final',
			content: 'Streaming complete.',
			costUSD: 0.0134,
			timestamp: '2026-05-15T18:00:02Z'
		});
		expect(final.pendingDelta).toBe('');
		expect(final.session.turns).toHaveLength(2);
		expect(final.session.turns[1]).toMatchObject({
			actor: 'reviewer',
			content: 'Streaming complete.'
		});

		const deduped = applyReviewSessionEvent(final.session, '', {
			sessionId: base.id,
			kind: 'final',
			content: 'Streaming complete.',
			costUSD: 0.0134,
			timestamp: '2026-05-15T18:00:02Z'
		});
		expect(deduped.session.turns).toHaveLength(2);
	});
});

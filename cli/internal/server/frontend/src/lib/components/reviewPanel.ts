export interface ReviewTurn {
	actor: string;
	content: string;
	costUSD: number;
	createdAt: string;
}

export interface ReviewSession {
	id: string;
	artifactId: string;
	artifactSha: string;
	status: string;
	costUSD: number;
	maxBillableUSD: number;
	turns: ReviewTurn[];
}

export interface ReviewSessionEvent {
	sessionId: string;
	kind: string;
	content: string;
	costUSD: number;
	timestamp: string;
}

export function activeReviewCount(sessions: ReviewSession[]): number {
	return sessions.filter((session) => session.status === 'active').length;
}

export function sessionHasShaDrift(
	session: Pick<ReviewSession, 'artifactSha'> | null,
	currentSha: string | null | undefined
): boolean {
	return Boolean(session?.artifactSha && currentSha && session.artifactSha !== currentSha);
}

export function applyReviewSessionEvent(
	session: ReviewSession,
	pendingDelta: string,
	event: ReviewSessionEvent
): { session: ReviewSession; pendingDelta: string } {
	if (event.kind === 'delta') {
		return {
			session,
			pendingDelta: pendingDelta + event.content
		};
	}

	if (event.kind !== 'final') {
		return { session, pendingDelta };
	}

	const lastTurn = session.turns.at(-1);
	const turns =
		lastTurn?.actor === 'reviewer' && lastTurn.content === event.content
			? session.turns
			: [
					...session.turns,
					{
						actor: 'reviewer',
						content: event.content,
						costUSD: event.costUSD,
						createdAt: event.timestamp
					}
				];

	return {
		session: {
			...session,
			costUSD: Math.max(session.costUSD, event.costUSD),
			turns
		},
		pendingDelta: ''
	};
}

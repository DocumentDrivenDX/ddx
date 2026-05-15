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

export interface ReviewFinding {
	id: string;
	message: string;
	summary: string;
	sourceTurnIndex: number;
}

const findingListPrefix = /^(?:[-*•]\s+|\d+[.)]\s+)/;
const ignoredReviewerFindings = new Set(['review pending dispatcher integration']);

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

function normalizeFindingText(text: string): string {
	return text.replace(/\s+/g, ' ').trim();
}

function findingSegments(content: string): string[] {
	const trimmed = content.trim();
	if (!trimmed) return [];

	const lines = trimmed
		.split(/\r?\n/)
		.map((line) => line.trim())
		.filter(Boolean);
	const listItems = lines
		.filter((line) => findingListPrefix.test(line))
		.map((line) => normalizeFindingText(line.replace(findingListPrefix, '')))
		.filter(Boolean);
	if (listItems.length > 0) return listItems;

	const paragraphs = trimmed
		.split(/\n\s*\n/)
		.map((part) => normalizeFindingText(part))
		.filter(Boolean);
	if (paragraphs.length > 1) return paragraphs;

	return [normalizeFindingText(trimmed)];
}

function findingSummary(message: string): string {
	if (message.length <= 72) return message;
	return `${message.slice(0, 69).trimEnd()}...`;
}

export function extractReviewFindings(
	session: Pick<ReviewSession, 'turns'> | null
): ReviewFinding[] {
	if (!session) return [];

	const findings: ReviewFinding[] = [];
	const seen = new Set<string>();

	session.turns.forEach((turn, turnIndex) => {
		if (turn.actor !== 'reviewer') return;

		for (const segment of findingSegments(turn.content)) {
			const normalized = normalizeFindingText(segment);
			const key = normalized.toLowerCase();
			if (!normalized || ignoredReviewerFindings.has(key) || seen.has(key)) continue;
			seen.add(key);
			findings.push({
				id: `finding-${turnIndex}-${findings.length}`,
				message: normalized,
				summary: findingSummary(normalized),
				sourceTurnIndex: turnIndex
			});
		}
	});

	return findings;
}

export function buildReviewFindingOperatorPrompt(
	artifactTitle: string,
	artifactId: string,
	finding: ReviewFinding
): string {
	return [
		`Address review finding for ${artifactTitle}`,
		'',
		`Artifact ID: ${artifactId}`,
		`Artifact: ${artifactTitle}`,
		'',
		'Finding to address:',
		finding.message
	].join('\n');
}

export function buildReviewFindingFollowUp(
	artifactTitle: string,
	artifactId: string,
	finding: ReviewFinding
): {
	title: string;
	status: string;
	priority: number;
	issueType: string;
	description: string;
} {
	return {
		title: `Follow up review finding: ${finding.summary}`,
		status: 'open',
		priority: 2,
		issueType: 'task',
		description: [
			`Artifact ID: ${artifactId}`,
			`Artifact: ${artifactTitle}`,
			'',
			'Review finding:',
			finding.message
		].join('\n')
	};
}

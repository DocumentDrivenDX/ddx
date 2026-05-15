<script lang="ts">
	import { onDestroy } from 'svelte';
	import {
		AlertTriangle,
		Loader2,
		MessageSquareMore,
		ShieldAlert,
		Sparkles,
		X
	} from 'lucide-svelte';
	import { gql } from 'graphql-request';
	import { createClient } from '$lib/gql/client';
	import {
		subscribeReviewSessionEvents,
		type ReviewSessionEvent as StreamingReviewSessionEvent
	} from '$lib/gql/subscriptions';
	import {
		activeReviewCount,
		applyReviewSessionEvent,
		sessionHasShaDrift,
		type ReviewSession
	} from './reviewPanel';

	type Props = {
		artifactId: string;
		artifactTitle: string;
		artifactSha: string | null;
	};

	type ReviewSessionEnvelope = {
		reviewSessionStart: ReviewSession;
	};

	type ReviewSessionRespondEnvelope = {
		reviewSessionRespond: ReviewSession;
	};

	type ReviewSessionCancelEnvelope = {
		reviewSessionCancel: boolean;
	};

	let { artifactId, artifactTitle, artifactSha }: Props = $props();

	const REVIEW_SESSION_START_MUTATION = gql`
		mutation ReviewSessionStart($input: ReviewSessionStartInput!) {
			reviewSessionStart(input: $input) {
				id
				artifactId
				artifactSha
				status
				costUSD
				maxBillableUSD
				turns {
					actor
					content
					costUSD
					createdAt
				}
			}
		}
	`;

	const REVIEW_SESSION_RESPOND_MUTATION = gql`
		mutation ReviewSessionRespond($sessionId: ID!, $turn: ReviewTurnInput!) {
			reviewSessionRespond(sessionId: $sessionId, turn: $turn) {
				id
				artifactId
				artifactSha
				status
				costUSD
				maxBillableUSD
				turns {
					actor
					content
					costUSD
					createdAt
				}
			}
		}
	`;

	const REVIEW_SESSION_CANCEL_MUTATION = gql`
		mutation ReviewSessionCancel($sessionId: ID!) {
			reviewSessionCancel(sessionId: $sessionId)
		}
	`;

	let session = $state<ReviewSession | null>(null);
	let draft = $state('');
	let alertMessage = $state<string | null>(null);
	let submitting = $state(false);
	let cancelling = $state(false);
	let pendingReviewerDelta = $state('');
	let subscribedSessionId: string | null = null;
	let unsubscribe: (() => void) | null = null;

	const activeSessions = $derived(session ? [session] : []);
	const activeCount = $derived(activeReviewCount(activeSessions));
	const drifted = $derived(sessionHasShaDrift(session, artifactSha));
	const submitLabel = $derived(session ? 'Send follow-up' : 'Start review');
	const submitDisabled = $derived(
		submitting ||
			cancelling ||
			!artifactSha ||
			!draft.trim() ||
			(session?.status === 'cancelled' && !drifted)
	);
	const reviewUnavailable = $derived(!artifactSha);

	function formatStatus(status: string | null | undefined): string {
		if (!status) return 'idle';
		return status.replaceAll('_', ' ');
	}

	function formatCost(costUSD: number): string {
		return `$${costUSD.toFixed(4)}`;
	}

	function errorText(err: unknown): string {
		if (err instanceof Error) return err.message;
		if (typeof err === 'string') return err;
		return 'Unknown error.';
	}

	function sessionBadgeClass(status: string | null | undefined): string {
		switch (status) {
			case 'active':
				return 'bg-green-100 text-green-900 dark:bg-green-900/30 dark:text-green-200';
			case 'completed':
				return 'bg-blue-100 text-blue-900 dark:bg-blue-900/30 dark:text-blue-200';
			case 'cancelled':
				return 'bg-fg-muted/20 text-fg-muted dark:bg-dark-fg-muted/20 dark:text-dark-fg-muted';
			default:
				return 'bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted';
		}
	}

	function reviewerBubbleClass(actor: string): string {
		return actor === 'reviewer'
			? 'bg-bg-surface text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-ink'
			: 'bg-accent-lever/10 text-fg-ink dark:bg-dark-accent-lever/15 dark:text-dark-fg-ink';
	}

	function teardownSubscription() {
		unsubscribe?.();
		unsubscribe = null;
		subscribedSessionId = null;
	}

	function handleStreamEvent(event: StreamingReviewSessionEvent) {
		if (!session || event.sessionId !== session.id) return;
		const next = applyReviewSessionEvent(session, pendingReviewerDelta, event);
		session = next.session;
		pendingReviewerDelta = next.pendingDelta;
	}

	function ensureSubscription(sessionId: string) {
		if (subscribedSessionId === sessionId) return;
		teardownSubscription();
		subscribedSessionId = sessionId;
		unsubscribe = subscribeReviewSessionEvents(
			sessionId,
			(event) => {
				handleStreamEvent(event);
			},
			(err) => {
				alertMessage = `Review stream disconnected. ${errorText(err)}`;
			}
		);
	}

	async function handleSubmit(event: SubmitEvent) {
		event.preventDefault();
		if (reviewUnavailable) return;
		const message = draft.trim();
		if (!message) return;

		submitting = true;
		alertMessage = null;
		try {
			const client = createClient();
			let activeSession = session;
			if (!activeSession || activeSession.status === 'cancelled' || drifted) {
				const started = await client.request<ReviewSessionEnvelope>(REVIEW_SESSION_START_MUTATION, {
					input: {
						artifactId,
						artifactSha
					}
				});
				activeSession = started.reviewSessionStart;
				session = activeSession;
			}

			ensureSubscription(activeSession.id);
			pendingReviewerDelta = '';

			const updated = await client.request<ReviewSessionRespondEnvelope>(
				REVIEW_SESSION_RESPOND_MUTATION,
				{
					sessionId: activeSession.id,
					turn: {
						content: message
					}
				}
			);

			session = updated.reviewSessionRespond;
			pendingReviewerDelta = '';
			draft = '';
		} catch (err) {
			alertMessage = `Review request failed. ${errorText(err)}`;
		} finally {
			submitting = false;
		}
	}

	async function handleCancel() {
		if (!session || session.status !== 'active') return;
		cancelling = true;
		alertMessage = null;
		try {
			const client = createClient();
			await client.request<ReviewSessionCancelEnvelope>(REVIEW_SESSION_CANCEL_MUTATION, {
				sessionId: session.id
			});
			session = {
				...session,
				status: 'cancelled'
			};
			pendingReviewerDelta = '';
		} catch (err) {
			alertMessage = `Could not cancel review. ${errorText(err)}`;
		} finally {
			cancelling = false;
		}
	}

	onDestroy(() => {
		teardownSubscription();
	});
</script>

<section
	data-testid="review-panel"
	class="border-border-line bg-bg-elevated dark:border-dark-border-line dark:bg-dark-bg-elevated space-y-4 border p-4"
>
	<div class="flex flex-wrap items-start justify-between gap-3">
		<div class="space-y-1">
			<h2 class="text-headline-md text-fg-ink dark:text-dark-fg-ink font-semibold">Review</h2>
			<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted max-w-2xl">
				Ask for a focused correctness pass on <span class="font-medium">{artifactTitle}</span>.
				Streaming deltas render live when the reviewer emits them.
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<span
				data-testid="review-active-indicator"
				class="font-label-caps text-label-caps bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted inline-flex items-center gap-2 rounded-full px-3 py-1 uppercase"
			>
				<Sparkles class="h-3.5 w-3.5" />
				{activeCount} active review{activeCount === 1 ? '' : 's'}
			</span>
			{#if session}
				<span
					data-testid="review-status-badge"
					class="font-label-caps text-label-caps inline-flex rounded-full px-3 py-1 uppercase {sessionBadgeClass(
						session.status
					)}"
				>
					{formatStatus(session.status)}
				</span>
			{/if}
		</div>
	</div>

	{#if alertMessage}
		<div
			role="alert"
			data-testid="review-alert"
			class="border-error/30 bg-error/10 text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error rounded-md border px-3 py-2 text-sm"
		>
			{alertMessage}
		</div>
	{/if}

	{#if drifted}
		<div
			role="alert"
			data-testid="review-drift-banner"
			class="flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950/40 dark:text-amber-200"
		>
			<AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />
			<div>
				This artifact changed after the review started. Start a fresh review before acting on older
				findings.
			</div>
		</div>
	{/if}

	{#if reviewUnavailable}
		<div
			data-testid="review-unavailable"
			class="border-border-line bg-bg-surface text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-muted rounded-md border px-3 py-2 text-sm"
		>
			Review is unavailable because the current artifact SHA could not be determined.
		</div>
	{:else}
		<form class="space-y-3" onsubmit={handleSubmit}>
			<label class="block">
				<span class="text-fg-ink dark:text-dark-fg-ink text-sm font-medium"> Review focus </span>
				<textarea
					data-testid="review-input"
					bind:value={draft}
					rows="4"
					placeholder="Ask for bugs, regressions, or missing tests."
					class="border-border-line bg-bg-canvas text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:ring-accent-lever dark:border-dark-border-line dark:bg-dark-bg-canvas dark:text-dark-fg-ink dark:placeholder-dark-fg-muted mt-1 w-full rounded-md border px-3 py-2 text-sm focus:ring-1 focus:outline-none"
				></textarea>
			</label>
			<div class="flex flex-wrap items-center gap-3">
				<button
					data-testid="review-submit"
					type="submit"
					disabled={submitDisabled}
					class="bg-fg-ink text-bg-canvas disabled:bg-fg-muted disabled:text-bg-surface dark:bg-dark-fg-ink dark:text-dark-bg-canvas dark:disabled:bg-dark-fg-muted dark:disabled:text-dark-bg-surface inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium disabled:cursor-not-allowed"
				>
					{#if submitting}
						<Loader2 class="h-4 w-4 animate-spin" />
					{:else}
						<MessageSquareMore class="h-4 w-4" />
					{/if}
					{submitLabel}
				</button>
				{#if session?.status === 'active'}
					<button
						data-testid="review-cancel"
						type="button"
						disabled={submitting || cancelling}
						onclick={handleCancel}
						class="border-border-line text-fg-muted hover:text-fg-ink dark:border-dark-border-line dark:text-dark-fg-muted dark:hover:text-dark-fg-ink inline-flex items-center gap-2 rounded-md border px-3 py-2 text-sm disabled:cursor-not-allowed disabled:opacity-60"
					>
						{#if cancelling}
							<Loader2 class="h-4 w-4 animate-spin" />
						{:else}
							<X class="h-4 w-4" />
						{/if}
						Cancel review
					</button>
				{/if}
				{#if session}
					<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
						Session {session.id} · {formatCost(session.costUSD)}
					</span>
				{/if}
			</div>
		</form>
	{/if}

	{#if session}
		<div
			data-testid="review-transcript"
			class="border-border-line dark:border-dark-border-line space-y-3 border p-3"
		>
			<div class="flex items-center justify-between gap-2">
				<h3 class="text-fg-ink dark:text-dark-fg-ink text-sm font-medium">Transcript</h3>
				<span class="text-body-sm text-fg-muted dark:text-dark-fg-muted">
					{session.turns.length} turn{session.turns.length === 1 ? '' : 's'}
				</span>
			</div>

			{#if session.turns.length === 0 && !pendingReviewerDelta}
				<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No review turns yet.</p>
			{/if}

			{#each session.turns as turn, idx (`${turn.actor}-${turn.createdAt}-${idx}`)}
				<div
					data-testid={`review-turn-${turn.actor}`}
					class="space-y-1 rounded-md px-3 py-2 {reviewerBubbleClass(turn.actor)}"
				>
					<div class="flex items-center justify-between gap-2">
						<span class="font-label-caps text-label-caps uppercase">
							{turn.actor === 'reviewer' ? 'reviewer' : 'operator'}
						</span>
						<span class="text-[11px] opacity-70">
							{new Date(turn.createdAt).toLocaleString()}
						</span>
					</div>
					<p class="text-sm whitespace-pre-wrap">{turn.content}</p>
				</div>
			{/each}

			{#if pendingReviewerDelta}
				<div
					data-testid="review-streaming-delta"
					class="bg-bg-surface text-fg-ink dark:bg-dark-bg-surface dark:text-dark-fg-ink space-y-1 rounded-md px-3 py-2"
				>
					<div class="font-label-caps text-label-caps flex items-center gap-2 uppercase">
						<ShieldAlert class="h-3.5 w-3.5" />
						reviewer streaming
					</div>
					<p class="text-sm whitespace-pre-wrap">{pendingReviewerDelta}</p>
				</div>
			{/if}
		</div>
	{/if}
</section>

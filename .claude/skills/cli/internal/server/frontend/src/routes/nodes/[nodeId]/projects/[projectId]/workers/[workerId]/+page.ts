import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const WORKER_QUERY = gql`
	query WorkerDetail($id: ID!) {
		worker(id: $id) {
			id
			kind
			state
			status
			harness
			model
			effort
			once
			pollInterval
			startedAt
			finishedAt
			currentBead
			lastError
			attempts
			successes
			failures
			currentAttempt {
				attemptId
				beadId
				phase
				startedAt
				elapsedMs
			}
			recentEvents {
				kind
				text
				name
				inputs
				output
			}
			lifecycleEvents {
				action
				actor
				timestamp
				detail
				beadId
			}
		}
	}
`;

const WORKER_LOG_QUERY = gql`
	query WorkerLog($workerID: ID!) {
		workerLog(workerID: $workerID) {
			stdout
			stderr
		}
	}
`;

const WORKER_SESSIONS_QUERY = gql`
	query WorkerSessions($first: Int) {
		agentSessions(first: $first) {
			edges {
				node {
					id
					projectId
					workerId
					beadId
					harness
					model
					status
					startedAt
					durationMs
					cost
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
	}
`;

interface CurrentAttempt {
	attemptId: string;
	beadId: string | null;
	phase: string;
	startedAt: string;
	elapsedMs: number;
}

export interface WorkerDetail {
	id: string;
	kind: string;
	state: string;
	status: string | null;
	harness: string | null;
	model: string | null;
	effort: string | null;
	once: boolean | null;
	pollInterval: string | null;
	startedAt: string | null;
	finishedAt: string | null;
	currentBead: string | null;
	lastError: string | null;
	attempts: number | null;
	successes: number | null;
	failures: number | null;
	currentAttempt: CurrentAttempt | null;
	recentEvents: WorkerRecentEvent[];
	lifecycleEvents: WorkerLifecycleEvent[];
}

export interface WorkerRecentEvent {
	kind: string;
	text: string | null;
	name: string | null;
	inputs: string | null;
	output: string | null;
}

export interface WorkerLifecycleEvent {
	action: string;
	actor: string;
	timestamp: string;
	detail: string | null;
	beadId: string | null;
}

export interface WorkerSession {
	id: string;
	projectId: string;
	workerId: string | null;
	beadId: string | null;
	harness: string;
	model: string;
	status: string;
	startedAt: string;
	durationMs: number;
	cost: number | null;
}

interface WorkerResult {
	worker: WorkerDetail | null;
}

interface WorkerLogResult {
	workerLog: { stdout: string; stderr: string };
}

interface WorkerSessionsResult {
	agentSessions: {
		edges: Array<{ node: WorkerSession; cursor: string }>;
		pageInfo: { hasNextPage: boolean; endCursor: string | null };
		totalCount: number;
	};
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const [workerResult, logResult, sessionsResult] = await Promise.all([
		client.request<WorkerResult>(WORKER_QUERY, { id: params.workerId }),
		client
			.request<WorkerLogResult>(WORKER_LOG_QUERY, { workerID: params.workerId })
			.catch(() => ({ workerLog: { stdout: '', stderr: '' } })),
		client
			.request<WorkerSessionsResult>(WORKER_SESSIONS_QUERY, { first: 100 })
			.catch(() => ({
				agentSessions: {
					edges: [],
					pageInfo: { hasNextPage: false, endCursor: null },
					totalCount: 0
				}
			}))
	]);
	const workerSessions = sessionsResult.agentSessions.edges
		.map((edge) => edge.node)
		.filter((session) => session.projectId === params.projectId && session.workerId === params.workerId);
	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		worker: workerResult.worker
			? {
					...workerResult.worker,
					recentEvents: workerResult.worker.recentEvents ?? [],
					lifecycleEvents: workerResult.worker.lifecycleEvents ?? []
				}
			: null,
		initialLog: logResult.workerLog.stdout,
		sessions: workerSessions
	};
};

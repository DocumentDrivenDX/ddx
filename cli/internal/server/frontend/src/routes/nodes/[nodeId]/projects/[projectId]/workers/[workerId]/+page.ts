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
}

export interface WorkerRecentEvent {
	kind: string;
	text: string | null;
	name: string | null;
	inputs: string | null;
	output: string | null;
}

interface WorkerResult {
	worker: WorkerDetail | null;
}

interface WorkerLogResult {
	workerLog: { stdout: string; stderr: string };
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const [workerResult, logResult] = await Promise.all([
		client.request<WorkerResult>(WORKER_QUERY, { id: params.workerId }),
		client
			.request<WorkerLogResult>(WORKER_LOG_QUERY, { workerID: params.workerId })
			.catch(() => ({ workerLog: { stdout: '', stderr: '' } }))
	]);
	return {
		worker: workerResult.worker
			? { ...workerResult.worker, recentEvents: workerResult.worker.recentEvents ?? [] }
			: null,
		initialLog: logResult.workerLog.stdout
	};
};

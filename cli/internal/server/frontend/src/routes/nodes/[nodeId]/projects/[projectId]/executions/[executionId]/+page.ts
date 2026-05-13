import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const EXECUTION_DETAIL_QUERY = gql`
	query ExecutionDetail($id: ID!) {
		execution(id: $id) {
			id
			projectId
			beadId
			beadTitle
			sessionId
			workerId
			harness
			model
			verdict
			status
			rationale
			createdAt
			startedAt
			finishedAt
			durationMs
			costUsd
			tokens
			exitCode
			baseRev
			resultRev
			bundlePath
			promptPath
			manifestPath
			resultPath
			agentLogPath
			prompt
			manifest
			result
		}
	}
`;

export interface ExecutionDetail {
	id: string;
	projectId: string;
	beadId: string | null;
	beadTitle: string | null;
	sessionId: string | null;
	workerId: string | null;
	harness: string | null;
	model: string | null;
	verdict: string | null;
	status: string | null;
	rationale: string | null;
	createdAt: string;
	startedAt: string | null;
	finishedAt: string | null;
	durationMs: number | null;
	costUsd: number | null;
	tokens: number | null;
	exitCode: number | null;
	baseRev: string | null;
	resultRev: string | null;
	bundlePath: string;
	promptPath: string | null;
	manifestPath: string | null;
	resultPath: string | null;
	agentLogPath: string | null;
	prompt?: string | null;
	manifest?: string | null;
	result?: string | null;
}

interface ExecutionResult {
	execution: ExecutionDetail | null;
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<ExecutionResult>(EXECUTION_DETAIL_QUERY, {
		id: params.executionId
	});
	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		execution: data.execution
	};
};

import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

export const EXECUTIONS_QUERY = gql`
	query ExecutionsPage($projectID: ID!, $first: Int, $after: String, $verdict: String) {
		executions(projectId: $projectID, first: $first, after: $after, verdict: $verdict) {
			edges {
				node {
					id
					projectId
					beadId
					beadTitle
					sessionId
					harness
					model
					verdict
					status
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

export interface ExecutionNode {
	id: string;
	projectId: string;
	beadId: string | null;
	beadTitle: string | null;
	sessionId: string | null;
	harness: string | null;
	model: string | null;
	verdict: string | null;
	status: string | null;
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

interface ExecutionEdge {
	node: ExecutionNode;
	cursor: string;
}

interface PageInfo {
	hasNextPage: boolean;
	endCursor: string | null;
}

interface ExecutionConnection {
	edges: ExecutionEdge[];
	pageInfo: PageInfo;
	totalCount: number;
}

interface ExecutionsResult {
	executions: ExecutionConnection;
}

export const load: PageLoad = async ({ params, url, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const verdict = url.searchParams.get('verdict') ?? undefined;
	const data = await client.request<ExecutionsResult>(EXECUTIONS_QUERY, {
		projectID: params.projectId,
		first: 50,
		verdict
	});

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		executions: data.executions,
		activeVerdict: verdict ?? null
	};
};

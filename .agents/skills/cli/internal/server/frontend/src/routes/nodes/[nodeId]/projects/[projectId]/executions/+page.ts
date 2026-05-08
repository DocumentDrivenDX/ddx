import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const EXECUTIONS_QUERY = gql`
	query ExecutionsPage(
		$projectId: ID!
		$first: Int
		$after: String
		$beadId: String
		$verdict: String
		$harness: String
		$search: String
	) {
		executions(
			projectId: $projectId
			first: $first
			after: $after
			beadId: $beadId
			verdict: $verdict
			harness: $harness
			search: $search
		) {
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
					bundlePath
				}
				cursor
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
				startCursor
				endCursor
			}
			totalCount
		}
	}
`;

export interface ExecutionListNode {
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
	bundlePath: string;
}

interface ExecutionEdge {
	node: ExecutionListNode;
	cursor: string;
}

interface PageInfo {
	hasNextPage: boolean;
	hasPreviousPage: boolean;
	startCursor: string | null;
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

const PAGE_SIZE = 50;

export const load: PageLoad = async ({ params, url, fetch }) => {
	const after = url.searchParams.get('after') ?? undefined;
	const beadId = url.searchParams.get('bead') ?? undefined;
	const verdict = url.searchParams.get('verdict') ?? undefined;
	const harness = url.searchParams.get('harness') ?? undefined;
	const search = url.searchParams.get('q') ?? undefined;

	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<ExecutionsResult>(EXECUTIONS_QUERY, {
		projectId: params.projectId,
		first: PAGE_SIZE,
		after,
		beadId,
		verdict,
		harness,
		search
	});
	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		executions: data.executions,
		filters: {
			after: after ?? null,
			bead: beadId ?? '',
			verdict: verdict ?? '',
			harness: harness ?? '',
			search: search ?? ''
		}
	};
};

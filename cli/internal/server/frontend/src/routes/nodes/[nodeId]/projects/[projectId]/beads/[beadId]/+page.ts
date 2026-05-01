import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const BEAD_QUERY = gql`
	query Bead($id: ID!, $projectID: String!) {
		bead(id: $id) {
			id
			title
			status
			priority
			issueType
			owner
			createdAt
			createdBy
			updatedAt
			labels
			parent
			description
			acceptance
			notes
			dependencies {
				issueId
				dependsOnId
				type
				createdAt
				createdBy
			}
		}
		projectBeads: beadsByProject(projectID: $projectID, first: 500) {
			edges {
				node {
					id
					parent
				}
			}
		}
		beadExecutions: executions(projectId: $projectID, beadId: $id, first: 50) {
			edges {
				node {
					id
					verdict
					harness
					createdAt
					durationMs
					costUsd
				}
			}
			totalCount
		}
		beadRuns: runs(projectID: $projectID, beadID: $id, first: 50) {
			edges {
				node {
					id
					layer
					status
					harness
					startedAt
					durationMs
				}
			}
			totalCount
		}
	}
`;

interface Dependency {
	issueId: string;
	dependsOnId: string;
	type: string;
	createdAt: string | null;
	createdBy: string | null;
}

export interface BeadDetail {
	id: string;
	title: string;
	status: string;
	priority: number;
	issueType: string;
	owner: string | null;
	createdAt: string;
	createdBy: string | null;
	updatedAt: string;
	labels: string[] | null;
	parent: string | null;
	description: string | null;
	acceptance: string | null;
	notes: string | null;
	dependencies: Dependency[] | null;
	childCount: number;
}

type BeadQueryDetail = Omit<BeadDetail, 'childCount'>;

export interface BeadExecution {
	id: string;
	verdict: string | null;
	harness: string | null;
	createdAt: string;
	durationMs: number | null;
	costUsd: number | null;
}

export interface BeadRun {
	id: string;
	layer: string;
	status: string;
	harness: string | null;
	startedAt: string | null;
	durationMs: number | null;
}

interface BeadResult {
	bead: BeadQueryDetail | null;
	projectBeads?: {
		edges: Array<{
			node: {
				id: string;
				parent: string | null;
			};
		}>;
	};
	beadExecutions?: {
		edges: Array<{ node: BeadExecution }>;
		totalCount: number;
	};
	beadRuns?: {
		edges: Array<{ node: BeadRun }>;
		totalCount: number;
	};
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<BeadResult>(BEAD_QUERY, {
		id: params.beadId,
		projectID: params.projectId
	});
	const childCount =
		data.projectBeads?.edges.filter((edge) => edge.node.parent === data.bead?.id).length ?? 0;
	const executions = data.beadExecutions?.edges.map((e) => e.node) ?? [];
	const runs = data.beadRuns?.edges.map((e) => e.node) ?? [];
	return {
		bead: data.bead ? { ...data.bead, childCount } : null,
		nodeId: params.nodeId,
		projectId: params.projectId,
		executions,
		runs
	};
};

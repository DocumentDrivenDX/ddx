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
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<BeadResult>(BEAD_QUERY, {
		id: params.beadId,
		projectID: params.projectId
	});
	const childCount =
		data.projectBeads?.edges.filter((edge) => edge.node.parent === data.bead?.id).length ?? 0;
	return {
		bead: data.bead ? { ...data.bead, childCount } : null
	};
};

import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

export const DOCUMENTS_QUERY = gql`
	query Documents($projectID: ID!, $first: Int, $after: String) {
		documents(projectID: $projectID, first: $first, after: $after) {
			edges {
				node {
					id
					path
					title
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

interface DocumentNode {
	id: string;
	path: string;
	title: string;
}

interface DocumentEdge {
	node: DocumentNode;
	cursor: string;
}

interface PageInfo {
	hasNextPage: boolean;
	endCursor: string | null;
}

interface DocumentConnection {
	edges: DocumentEdge[];
	pageInfo: PageInfo;
	totalCount: number;
}

interface DocumentsResult {
	documents: DocumentConnection;
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const data = await client.request<DocumentsResult>(DOCUMENTS_QUERY, {
		projectID: params.projectId,
		first: 50
	});

	return {
		nodeId: params.nodeId,
		projectId: params.projectId,
		documents: data.documents
	};
};

import { gql } from 'graphql-request';

export const OPERATOR_PROMPT_SUBMIT_MUTATION = gql`
	mutation OperatorPromptSubmit($input: OperatorPromptSubmitInput!) {
		operatorPromptSubmit(input: $input) {
			deduplicated
			autoApproved
			bead {
				id
				title
				status
				priority
				issueType
				createdAt
				updatedAt
				labels
				description
			}
		}
	}
`;

export const OPERATOR_PROMPT_APPROVE_MUTATION = gql`
	mutation OperatorPromptApprove($id: ID!) {
		operatorPromptApprove(id: $id) {
			bead {
				id
				title
				status
				priority
				issueType
				updatedAt
			}
		}
	}
`;

export const OPERATOR_PROMPT_CANCEL_MUTATION = gql`
	mutation OperatorPromptCancel($id: ID!) {
		operatorPromptCancel(id: $id) {
			bead {
				id
				title
				status
				priority
				issueType
				updatedAt
			}
		}
	}
`;

export const RECENT_OPERATOR_PROMPTS_QUERY = gql`
	query RecentOperatorPrompts($projectID: String!) {
		beadsByProject(projectID: $projectID, label: "kind:operator-prompt", first: 20) {
			edges {
				node {
					id
					title
					status
					priority
					issueType
					createdAt
					updatedAt
					labels
				}
			}
		}
	}
`;

export interface OperatorPromptBead {
	id: string;
	title: string;
	status: string;
	priority: number;
	issueType: string;
	createdAt?: string;
	updatedAt: string;
	labels?: string[] | null;
	description?: string | null;
}

export interface OperatorPromptSubmitResult {
	operatorPromptSubmit: {
		deduplicated: boolean;
		autoApproved: boolean;
		bead: OperatorPromptBead;
	};
}

export interface OperatorPromptApproveResult {
	operatorPromptApprove: { bead: OperatorPromptBead };
}

export interface OperatorPromptCancelResult {
	operatorPromptCancel: { bead: OperatorPromptBead };
}

export interface RecentOperatorPromptsResult {
	beadsByProject: {
		edges: Array<{ node: OperatorPromptBead }>;
	};
}

let cachedToken: string | null = null;

export async function getCsrfToken(
	fetchFn: typeof globalThis.fetch = globalThis.fetch
): Promise<string> {
	if (cachedToken) return cachedToken;
	const resp = await fetchFn('/api/csrf-token', { credentials: 'same-origin' });
	if (!resp.ok) {
		throw new Error(`csrf token fetch failed: ${resp.status}`);
	}
	const body = (await resp.json()) as { token?: string };
	cachedToken = body.token ?? '';
	return cachedToken;
}

export function resetCsrfToken(): void {
	cachedToken = null;
}

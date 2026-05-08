import type { PageLoad } from './$types';
import { createClient } from '$lib/gql/client';
import { gql } from 'graphql-request';

const EFFICACY_ROWS_QUERY = gql`
	query EfficacyRows($projectId: String) {
		efficacyRows(projectId: $projectId) {
			rowKey
			harness
			provider
			model
			attempts
			successes
			successRate
			medianInputTokens
			medianOutputTokens
			medianDurationMs
			medianCostUsd
			warning {
				kind
				threshold
			}
		}
	}
`;

const COMPARISONS_QUERY = gql`
	query Comparisons {
		comparisons {
			id
			state
			armCount
		}
	}
`;

interface EfficacyWarning {
	kind: string;
	threshold: number | null;
}

export interface EfficacyRow {
	rowKey?: string;
	harness: string;
	provider: string;
	model: string;
	attempts: number;
	successes: number;
	successRate: number;
	medianInputTokens: number;
	medianOutputTokens: number;
	medianDurationMs: number;
	medianCostUsd: number | null;
	warning: EfficacyWarning | null;
	tier?: string | null;
	labels?: string[] | null;
	specId?: string | null;
}

export interface ComparisonRecord {
	id: string;
	state: string;
	armCount: number;
}

interface EfficacyRowsResult {
	efficacyRows: EfficacyRow[];
}

interface ComparisonsResult {
	comparisons: ComparisonRecord[];
}

export const load: PageLoad = async ({ params, url, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch);
	const [rowsData, comparisonsData] = await Promise.all([
		client.request<EfficacyRowsResult>(EFFICACY_ROWS_QUERY, { projectId: params.projectId }),
		client.request<ComparisonsResult>(COMPARISONS_QUERY)
	]);

	return {
		projectId: params.projectId,
		rows: rowsData.efficacyRows,
		comparisons: comparisonsData.comparisons,
		activeTier: url.searchParams.get('tier') ?? '',
		activeLabel: url.searchParams.get('label') ?? '',
		activeSpecId: url.searchParams.get('spec-id') ?? ''
	};
};

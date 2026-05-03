import { gql } from 'graphql-request';

export const PROJECT_QUEUE_SUMMARY_QUERY = gql`
	query ProjectQueueSummary($projectId: String!) {
		queueSummary(projectId: $projectId) {
			ready
			blocked
			inProgress
		}
	}
`;

export const WORKER_DISPATCH_MUTATION = gql`
	mutation WorkerDispatch($kind: String!, $projectId: String!, $args: String) {
		workerDispatch(kind: $kind, projectId: $projectId, args: $args) {
			id
			state
			kind
		}
	}
`;

export const EFFICACY_ROWS_QUERY = gql`
	query EfficacyRows {
		efficacyRows {
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

export const EFFICACY_ATTEMPTS_QUERY = gql`
	query EfficacyAttempts($rowKey: String!) {
		efficacyAttempts(rowKey: $rowKey) {
			rowKey
			attempts {
				beadId
				outcome
				durationMs
				costUsd
				evidenceBundleUrl
			}
		}
	}
`;

export const COMPARISONS_QUERY = gql`
	query Comparisons {
		comparisons {
			id
			state
			armCount
		}
	}
`;

export const COMPARISON_DISPATCH_MUTATION = gql`
	mutation ComparisonDispatch($arms: [ComparisonArmInput!]!) {
		comparisonDispatch(arms: $arms) {
			id
			state
			armCount
		}
	}
`;

export const PERSONAS_FLAT_QUERY = gql`
	query Personas {
		personas {
			id
			name
			roles
			description
			tags
			content
			body
			source
			bindings {
				projectId
				role
				persona
			}
			filePath
			modTime
		}
	}
`;

export const PROJECT_BINDINGS_QUERY = gql`
	query ProjectBindings($projectId: String!) {
		projectBindings(projectId: $projectId)
	}
`;

export const PERSONA_BIND_MUTATION = gql`
	mutation PersonaBind($role: String!, $persona: String!, $projectId: String!) {
		personaBind(role: $role, persona: $persona, projectId: $projectId) {
			ok
			role
			persona
		}
	}
`;

export const PLUGINS_LIST_QUERY = gql`
	query PluginsList {
		pluginsList {
			name
			version
			installedVersion
			type
			description
			keywords
			status
			registrySource
			diskBytes
			manifest
			skills
			prompts
			templates
		}
	}
`;

export const PLUGIN_DETAIL_QUERY = gql`
	query PluginDetail($name: String!) {
		pluginDetail(name: $name) {
			name
			version
			installedVersion
			type
			description
			keywords
			status
			registrySource
			diskBytes
			manifest
			skills
			prompts
			templates
		}
	}
`;

export const PLUGIN_DISPATCH_MUTATION = gql`
	mutation PluginDispatch($name: String!, $action: String!, $scope: String!) {
		pluginDispatch(name: $name, action: $action, scope: $scope) {
			id
			state
			action
		}
	}
`;

export const PALETTE_SEARCH_QUERY = gql`
	query PaletteSearch($query: String!) {
		paletteSearch(query: $query) {
			documents {
				kind
				path
				title
			}
			beads {
				kind
				id
				title
			}
			actions {
				kind
				id
				label
			}
			navigation {
				kind
				route
				title
			}
		}
	}
`;

export const RUN_REQUEUE_MUTATION = gql`
	mutation RunRequeue($input: RunRequeueInput!) {
		runRequeue(input: $input) {
			bead {
				id
				title
				status
			}
			deduplicated
		}
	}
`;

export const BEAD_CLOSE_MUTATION = gql`
	mutation BeadClose($id: ID!, $reason: String) {
		beadClose(id: $id, reason: $reason) {
			id
			title
			status
			priority
			issueType
		}
	}
`;

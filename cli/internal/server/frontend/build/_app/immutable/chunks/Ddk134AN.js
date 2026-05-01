import{n as e}from"./DBr1FBxI.js";var t=e`
	query ProjectQueueSummary($projectId: String!) {
		queueSummary(projectId: $projectId) {
			ready
			blocked
			inProgress
		}
	}
`,n=e`
	mutation WorkerDispatch($kind: String!, $projectId: String!, $args: String) {
		workerDispatch(kind: $kind, projectId: $projectId, args: $args) {
			id
			state
			kind
		}
	}
`;e`
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
`,e`
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
`,e`
	query Comparisons {
		comparisons {
			id
			state
			armCount
		}
	}
`,e`
	mutation ComparisonDispatch($arms: [ComparisonArmInput!]!) {
		comparisonDispatch(arms: $arms) {
			id
			state
			armCount
		}
	}
`,e`
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
`,e`
	query ProjectBindings($projectId: String!) {
		projectBindings(projectId: $projectId)
	}
`,e`
	mutation PersonaBind($role: String!, $persona: String!, $projectId: String!) {
		personaBind(role: $role, persona: $persona, projectId: $projectId) {
			ok
			role
			persona
		}
	}
`;var r=e`
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
`,i=e`
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
`,a=e`
	mutation PluginDispatch($name: String!, $action: String!, $scope: String!) {
		pluginDispatch(name: $name, action: $action, scope: $scope) {
			id
			state
			action
		}
	}
`;e`
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
`,e`
	mutation BeadClose($id: ID!, $reason: String) {
		beadClose(id: $id, reason: $reason) {
			id
			title
			status
			priority
			issueType
		}
	}
`;export{n as a,t as i,i as n,a as r,r as t};
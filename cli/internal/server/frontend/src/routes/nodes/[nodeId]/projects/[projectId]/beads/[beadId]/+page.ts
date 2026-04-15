import type { PageLoad } from './$types'
import { createClient } from '$lib/gql/client'
import { gql } from 'graphql-request'

const BEAD_QUERY = gql`
	query Bead($id: ID!) {
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
	}
`

interface Dependency {
	issueId: string
	dependsOnId: string
	type: string
	createdAt: string | null
	createdBy: string | null
}

export interface BeadDetail {
	id: string
	title: string
	status: string
	priority: number
	issueType: string
	owner: string | null
	createdAt: string
	createdBy: string | null
	updatedAt: string
	labels: string[] | null
	parent: string | null
	description: string | null
	acceptance: string | null
	notes: string | null
	dependencies: Dependency[] | null
}

interface BeadResult {
	bead: BeadDetail | null
}

export const load: PageLoad = async ({ params, fetch }) => {
	const client = createClient(fetch as unknown as typeof globalThis.fetch)
	const data = await client.request<BeadResult>(BEAD_QUERY, {
		id: params.beadId
	})
	return {
		bead: data.bead
	}
}

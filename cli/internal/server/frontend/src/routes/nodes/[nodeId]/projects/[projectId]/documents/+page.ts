import { redirect } from '@sveltejs/kit'
import type { PageLoad } from './$types'

export const load: PageLoad = async ({ params }) => {
	throw redirect(
		302,
		`/nodes/${params.nodeId}/projects/${params.projectId}/artifacts?mediaType=text%2Fmarkdown`
	)
}

import type { PageLoad } from './$types'

interface DocContent {
	path: string
	content: string
}

export const load: PageLoad = async ({ params, fetch }) => {
	const response = await fetch(`/api/documents/${params.path}`)
	if (!response.ok) {
		return { path: params.path, content: null }
	}
	const data: DocContent = await response.json()
	return {
		path: data.path,
		content: data.content
	}
}

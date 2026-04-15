export interface ProjectContext {
	id: string
	name: string
	path: string
}

function createProjectStore() {
	let value = $state<ProjectContext | null>(null)

	return {
		get value() {
			return value
		},
		set(v: ProjectContext | null) {
			value = v
		}
	}
}

export const projectStore = createProjectStore()

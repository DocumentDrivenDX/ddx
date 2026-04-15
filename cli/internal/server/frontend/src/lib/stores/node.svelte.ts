export interface NodeContext {
	id: string
	name: string
}

function createNodeStore() {
	let value = $state<NodeContext | null>(null)

	return {
		get value() {
			return value
		},
		set(v: NodeContext | null) {
			value = v
		}
	}
}

export const nodeStore = createNodeStore()

import type { NodeInfo$input, NodeInfo$result, QueryStore, QueryStoreFetchParams} from '$houdini'

export declare class NodeInfoStore extends QueryStore<NodeInfo$result, NodeInfo$input> {
	constructor() {
		// @ts-ignore
		super({})
	}
}

export declare const load_NodeInfo: (params: QueryStoreFetchParams<NodeInfo$result, NodeInfo$input>) => Promise<{NodeInfo: NodeInfoStore}>

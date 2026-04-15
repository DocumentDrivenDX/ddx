// @ts-nocheck
import type { LayoutLoad } from './$types'
import { load_NodeInfo } from '$houdini'

export const load = async (event: Parameters<LayoutLoad>[0]) => {
	return await load_NodeInfo({ event })
}

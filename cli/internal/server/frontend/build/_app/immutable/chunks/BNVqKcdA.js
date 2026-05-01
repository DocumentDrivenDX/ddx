import{n as e,t}from"./DBr1FBxI.js";var n=e`
	query ShellRouteDefaults {
		nodeInfo {
			id
		}
		projects {
			edges {
				node {
					id
				}
			}
		}
	}
`;async function r(e,r){let i=await t(r).request(n),a=i.nodeInfo.id,o=i.projects.edges[0]?.node.id;return o?`/nodes/${a}/projects/${o}/${e}`:`/nodes/${a}`}export{r as t};
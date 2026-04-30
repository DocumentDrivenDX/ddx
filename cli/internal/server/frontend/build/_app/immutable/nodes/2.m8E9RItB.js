import{t as e}from"../chunks/Dkx8MRxF.js";import{B as t,ft as n,k as r,z as i}from"../chunks/DkIPlaea.js";import"../chunks/BEDIq91W.js";import{n as a,t as o}from"../chunks/vpGPUDNZ.js";import{t as s}from"../chunks/BeTTU8eB.js";import{t as c}from"../chunks/BJ_KTwLw.js";var l=e({load:()=>d}),u=a`
	query ProjectsForLayout {
		projects {
			edges {
				node {
					id
					name
					path
				}
			}
		}
	}
`,d=async({params:e,fetch:t})=>{let n=(await o(t).request(u)).projects.edges.map(e=>e.node).find(t=>t.id===e.projectId);if(!n)throw c(404,`project ${e.projectId} not found`);return s.set(n),{project:n}};function f(e,a){var o=t();r(n(o),()=>a.children),i(e,o)}export{f as component,l as universal};
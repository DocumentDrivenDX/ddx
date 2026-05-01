import{t as e}from"../chunks/CuWRZlNB.js";import{B as t,ft as n,k as r,z as i}from"../chunks/D5KUolLB.js";import"../chunks/Q94VlVm2.js";import{n as a,t as o}from"../chunks/CiWV3YVb.js";import{t as s}from"../chunks/BTc7iXGc.js";import{t as c}from"../chunks/DGFmmtCw.js";var l=e({load:()=>d}),u=a`
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
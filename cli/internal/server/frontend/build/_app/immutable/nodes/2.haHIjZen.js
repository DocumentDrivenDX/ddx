import{t as e}from"../chunks/D-_T_ixn.js";import{B as t,ft as n,k as r,z as i}from"../chunks/C9gjXInc.js";import"../chunks/BpAyAfhb.js";import{n as a,t as o}from"../chunks/DBr1FBxI.js";import{t as s}from"../chunks/Q_aVztQu.js";import{t as c}from"../chunks/M2wJvVQH.js";var l=e({load:()=>d}),u=a`
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
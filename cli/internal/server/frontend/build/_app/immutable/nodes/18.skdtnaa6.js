import{t as e}from"../chunks/Dkx8MRxF.js";import{Dt as t,F as n,Ft as r,G as i,K as a,L as o,M as s,N as c,Ot as l,St as u,V as d,Z as f,at as p,dt as m,g as h,gt as g,mt as _,pt as v,r as y,vt as b,xt as x,yt as ee,z as S}from"../chunks/DkIPlaea.js";import{t as C}from"../chunks/DQ6KeVQs.js";import"../chunks/BEDIq91W.js";import{n as w,t as T}from"../chunks/vpGPUDNZ.js";import"../chunks/nt8TlDV5.js";import{t as E}from"../chunks/q4_J4xQI.js";var D=e({COMMIT_EXECUTION_QUERY:()=>k,load:()=>j}),O=w`
	query Commits($projectID: ID!, $first: Int, $after: String) {
		commits(projectID: $projectID, first: $first, after: $after) {
			edges {
				node {
					sha
					shortSha
					author
					date
					subject
					body
					beadRefs
				}
				cursor
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
				startCursor
				endCursor
			}
			totalCount
		}
	}
`,k=w`
	query ExecutionByResultRev($projectID: ID!, $sha: String!) {
		executionByResultRev(projectId: $projectID, sha: $sha) {
			id
		}
	}
`,A=20,j=async({params:e,url:t,fetch:n})=>{let r=t.searchParams.get(`after`)??void 0,i=await T(n).request(O,{projectID:e.projectId,first:A,after:r});return{projectId:e.projectId,commits:i.commits,after:r??null}},M=d(`<button class="rounded bg-blue-100 px-1.5 py-0.5 font-mono text-xs text-blue-700 hover:bg-blue-200 dark:bg-blue-900/40 dark:text-blue-300 dark:hover:bg-blue-900/60"> </button>`),N=d(`<div class="flex flex-wrap gap-1"></div>`),P=d(`<span class="text-gray-300 dark:text-gray-600">—</span>`),te=d(`<a class="font-mono text-blue-600 hover:underline dark:text-blue-400"> </a>`),ne=d(`<span class="text-gray-300 dark:text-gray-600">—</span>`),F=d(`<tr class="border-b border-gray-100 last:border-0 dark:border-gray-700"><td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400"> </td><td class="px-4 py-3 text-gray-900 dark:text-gray-100"><span> </span></td><td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400"> </td><td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400"> </td><td class="px-4 py-3"><!></td><td class="px-4 py-3 text-xs"><!></td></tr>`),I=d(`<tr><td colspan="6" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">No commits found.</td></tr>`),L=d(`<div class="space-y-4"><div class="flex items-center justify-between"><h1 class="text-xl font-semibold dark:text-white">Commits</h1> <span class="text-sm text-gray-500 dark:text-gray-400"> </span></div> <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"><table class="w-full text-sm"><thead><tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800"><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">SHA</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Subject</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Author</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Date</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Beads</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Execution</th></tr></thead><tbody><!><!></tbody></table></div> <div class="flex items-center justify-between"><button class="rounded border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800">← Previous</button> <span class="text-xs text-gray-400 dark:text-gray-500"> </span> <button class="rounded border border-gray-200 px-3 py-1.5 text-sm text-gray-600 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-40 dark:border-gray-700 dark:text-gray-300 dark:hover:bg-gray-800">Next →</button></div></div>`);function R(e,i){l(i,!0);let d=()=>u(E,`$page`,w),[w,D]=x(),O=b(_({}));y(async()=>{let e=T(fetch),t=d().params.projectId;await Promise.all(i.data.commits.edges.map(async n=>{let r=n.node.sha;if(f(O)[r]===void 0)try{let n=await e.request(k,{projectID:t,sha:r});g(O,{...f(O),[r]:n.executionByResultRev?.id??null},!0)}catch{g(O,{...f(O),[r]:null},!0)}}))});function A(e){let t=d().params;return`/nodes/${t.nodeId}/projects/${t.projectId}/executions/${e}`}function j(e){return new Date(e).toLocaleString()}function R(){let e=i.data.commits.pageInfo.endCursor;if(e){let t=d().params;C(`/nodes/${t.nodeId}/projects/${t.projectId}/commits?after=${encodeURIComponent(e)}`)}}function z(){let e=d().params;C(`/nodes/${e.nodeId}/projects/${e.projectId}/commits`)}function re(e){let t=d().params;C(`/nodes/${t.nodeId}/projects/${t.projectId}/beads/${e}`)}var B=L(),V=m(B),H=v(m(V),2),U=m(H);r(H),r(V);var W=v(V,2),G=m(W),K=v(m(G)),q=m(K);s(q,17,()=>i.data.commits.edges,e=>e.cursor,(e,t)=>{let i=ee(()=>f(t).node);var l=F(),u=m(l),d=m(u,!0);r(u);var g=v(u),_=m(g),y=m(_,!0);r(_),r(g);var b=v(g),x=m(b,!0);r(b);var C=v(b),w=m(C,!0);r(C);var T=v(C),E=m(T),D=e=>{var t=N();s(t,21,()=>f(i).beadRefs,c,(e,t)=>{var n=M(),i=m(n,!0);r(n),p(e=>o(i,e),[()=>f(t).slice(0,8)]),a(`click`,n,e=>{e.stopPropagation(),re(f(t))}),S(e,n)}),r(t),S(e,t)},k=e=>{S(e,P())};n(E,e=>{f(i).beadRefs&&f(i).beadRefs.length>0?e(D):e(k,-1)}),r(T);var I=v(T),L=m(I),R=e=>{var t=te(),n=m(t,!0);r(t),p((e,r)=>{h(t,`href`,e),o(n,r)},[()=>A(f(O)[f(i).sha]),()=>f(O)[f(i).sha].slice(0,18)]),S(e,t)},z=e=>{S(e,ne())};n(L,e=>{f(O)[f(i).sha]?e(R):e(z,-1)}),r(I),r(l),p(e=>{o(d,f(i).shortSha),h(_,`title`,f(i).body??void 0),o(y,f(i).subject),o(x,f(i).author),o(w,e)},[()=>j(f(i).date)]),S(e,l)});var J=v(q),Y=e=>{S(e,I())};n(J,e=>{i.data.commits.edges.length===0&&e(Y)}),r(K),r(G),r(W);var X=v(W,2),Z=m(X),Q=v(Z,2),ie=m(Q);r(Q);var $=v(Q,2);r(X),r(B),p(()=>{o(U,`${i.data.commits.totalCount??``} total`),Z.disabled=!i.data.after,o(ie,`${i.data.commits.edges.length??``} commits shown`),$.disabled=!i.data.commits.pageInfo.hasNextPage}),a(`click`,Z,z),a(`click`,$,R),S(e,B),t(),D()}i([`click`]);export{R as component,D as universal};
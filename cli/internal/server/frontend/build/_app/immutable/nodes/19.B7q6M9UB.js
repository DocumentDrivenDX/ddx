import{t as e}from"../chunks/Dkx8MRxF.js";import{Dt as t,F as n,Ft as r,G as i,K as a,L as o,M as s,Ot as c,St as l,V as u,Z as d,at as f,dt as p,pt as m,xt as h,z as g}from"../chunks/DkIPlaea.js";import{t as _}from"../chunks/DQ6KeVQs.js";import"../chunks/BEDIq91W.js";import{n as v,t as y}from"../chunks/vpGPUDNZ.js";import"../chunks/nt8TlDV5.js";import{t as b}from"../chunks/q4_J4xQI.js";import{t as x}from"../chunks/CnwI0EJY.js";var S=e({load:()=>w}),C=v`
	query Documents($first: Int, $after: String) {
		documents(first: $first, after: $after) {
			edges {
				node {
					id
					path
					title
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
	}
`,w=async({fetch:e})=>({docs:(await y(e).request(C,{first:200})).documents}),T=u(`<tr class="cursor-pointer border-b border-gray-100 last:border-0 hover:bg-gray-50 dark:border-gray-700 dark:hover:bg-gray-800"><td class="px-4 py-3 text-gray-900 dark:text-gray-100"><div class="flex items-center gap-2"><!> </div></td><td class="px-4 py-3 font-mono text-xs text-gray-500 dark:text-gray-400"> </td></tr>`),E=u(`<tr><td colspan="2" class="px-4 py-8 text-center text-gray-700 dark:text-gray-300">No documents found.</td></tr>`),D=u(`<div class="space-y-4"><div class="flex items-center justify-between"><h1 class="text-xl font-semibold dark:text-white">Documents</h1> <span class="text-sm text-gray-700 dark:text-gray-300"> </span></div> <div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"><table class="w-full text-sm"><thead><tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800"><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Title</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Path</th></tr></thead><tbody><!><!></tbody></table></div></div>`);function O(e,i){c(i,!0);let u=()=>l(b,`$page`,v),[v,y]=h();function S(e){let t=u().params;_(`/nodes/${t.nodeId}/projects/${t.projectId}/documents/${e}`)}var C=D(),w=p(C),O=m(p(w),2),k=p(O);r(O),r(w);var A=m(w,2),j=p(A),M=m(p(j)),N=p(M);s(N,17,()=>i.data.docs.edges,e=>e.cursor,(e,t)=>{var n=T(),i=p(n),s=p(i),c=p(s);x(c,{class:`h-4 w-4 shrink-0 text-gray-400 dark:text-gray-500`});var l=m(c);r(s),r(i);var u=m(i),h=p(u,!0);r(u),r(n),f(()=>{o(l,` ${d(t).node.title??``}`),o(h,d(t).node.path)}),a(`click`,n,()=>S(d(t).node.path)),g(e,n)});var P=m(N),F=e=>{g(e,E())};n(P,e=>{i.data.docs.edges.length===0&&e(F)}),r(M),r(j),r(A),r(C),f(()=>o(k,`${i.data.docs.totalCount??``} total`)),g(e,C),t(),y()}i([`click`]);export{O as component,S as universal};
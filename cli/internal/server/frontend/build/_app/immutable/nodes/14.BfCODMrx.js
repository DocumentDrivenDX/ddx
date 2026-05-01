import{t as e}from"../chunks/D-_T_ixn.js";import{C as t,Dt as n,F as r,Ft as i,G as a,K as o,L as s,M as c,N as l,Ot as u,St as d,V as f,Z as p,at as m,dt as h,gt as g,mt as _,ot as v,pt as y,vt as b,w as x,xt as ee,yt as S,z as C}from"../chunks/C9gjXInc.js";import{t as te}from"../chunks/DrjC4ShN.js";import"../chunks/BpAyAfhb.js";import{n as w,t as T}from"../chunks/DBr1FBxI.js";import"../chunks/Dx2xiYXE.js";import{t as ne}from"../chunks/DLleOg3b.js";var E=e({load:()=>O}),D=w`
	query BeadsAllProjects($first: Int, $after: String, $status: String, $label: String, $projectID: String) {
		beads(first: $first, after: $after, status: $status, label: $label, projectID: $projectID) {
			edges {
				node {
					id
					title
					status
					priority
					labels
					projectID
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
			totalCount
		}
		projects {
			edges {
				node {
					id
					name
				}
			}
		}
	}
`,O=async({url:e,fetch:t})=>{let n=e.searchParams.get(`status`)??void 0,r=e.searchParams.get(`label`)??void 0,i=e.searchParams.get(`project`)??void 0,a=await T(t).request(D,{first:20,status:n,label:r,projectID:i}),o={};for(let{node:e}of a.projects.edges)o[e.id]=e.name;return{beads:a.beads,projects:a.projects.edges.map(e=>e.node),projectNames:o,activeStatus:n??null,activeLabel:r??null,activeProject:i??null}},re=f(`<button> </button>`),ie=f(`<button class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface">clear</button>`),ae=f(`<button> </button>`),oe=f(`<button class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface">clear</button>`),k=f(`<div class="flex flex-wrap gap-2"><span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Project:</span> <!> <!></div>`),se=f(`<button> </button>`),ce=f(`<button class="rounded-sm border border-border-line bg-bg-elevated px-3 py-1 text-xs text-fg-ink hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface">clear</button>`),le=f(`<div class="flex flex-wrap gap-2"><span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Label:</span> <!> <!></div>`),ue=f(`<tr class="border-b border-border-line last:border-0 dark:border-dark-border-line"><td class="px-4 py-3 font-mono-code text-xs text-lever"> </td><td class="px-4 py-3 text-fg-ink dark:text-dark-fg-ink"> </td><td class="px-4 py-3"><span class="inline-flex items-center border border-border-line px-2 py-0.5 text-xs font-medium text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"> </span></td><td class="px-4 py-3"><span> </span></td><td> </td></tr>`),de=f(`<tr><td colspan="5" class="px-4 py-8 text-center text-fg-muted dark:text-dark-fg-muted">No beads found.</td></tr>`),fe=f(`<div class="flex justify-center"><button class="rounded-sm border border-border-line bg-bg-elevated px-4 py-2 text-sm text-fg-ink hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface"> </button></div>`),A=f(`<div class="min-h-full space-y-4 bg-bg-canvas dark:bg-dark-bg-canvas"><div class="flex items-center justify-between"><h1 class="text-xl font-semibold text-fg-ink dark:text-dark-fg-ink">All Beads</h1> <span class="text-sm text-fg-muted dark:text-dark-fg-muted"> </span></div> <div class="flex flex-wrap gap-2"><span class="self-center text-xs text-fg-muted dark:text-dark-fg-muted">Status:</span> <!> <!></div> <!> <!> <div class="overflow-hidden border border-border-line bg-bg-elevated dark:border-dark-border-line dark:bg-dark-bg-elevated"><table class="w-full border-collapse text-sm"><thead><tr class="border-b border-border-line bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-surface"><th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">ID</th><th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Title</th><th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Project</th><th class="px-4 py-3 text-left font-medium text-fg-muted dark:text-dark-fg-muted">Status</th><th class="px-4 py-3 text-right font-medium text-fg-muted dark:text-dark-fg-muted">Priority</th></tr></thead><tbody><!><!></tbody></table></div> <!></div>`);function j(e,a){u(a,!0);let f=()=>d(ne,`$page`,E),[E,D]=ee(),O=w`
		query BeadsAllProjects($first: Int, $after: String, $status: String, $label: String, $projectID: String) {
			beads(first: $first, after: $after, status: $status, label: $label, projectID: $projectID) {
				edges {
					node {
						id
						title
						status
						priority
						labels
						projectID
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
	`,j=[`open`,`in-progress`,`closed`,`blocked`],M=b(_([])),N=b(null),P=b(!1),F=S(()=>`${a.data.activeStatus}::${a.data.activeLabel}::${a.data.activeProject}`),I=b(``);v(()=>{p(F)!==p(I)&&(g(I,p(F),!0),g(M,[],!0),g(N,null))});let L=S(()=>[...a.data.beads.edges,...p(M)]),R=S(()=>p(N)??a.data.beads.pageInfo),pe=S(()=>a.data.beads.totalCount),z=S(()=>Array.from(new Set(p(L).flatMap(e=>e.node.labels??[]))).sort());function B(e,t){let n=new URLSearchParams(f().url.searchParams);t===null?n.delete(e):n.set(e,t),n.delete(`after`);let r=n.toString();te(r?`?${r}`:f().url.pathname,{replaceState:!1})}function V(e){B(`status`,a.data.activeStatus===e?null:e)}function me(e){B(`label`,a.data.activeLabel===e?null:e)}function he(e){B(`project`,a.data.activeProject===e?null:e)}async function ge(){if(!(!p(R).hasNextPage||p(P))){g(P,!0);try{let e=await T().request(O,{first:20,after:p(R).endCursor,status:a.data.activeStatus??void 0,label:a.data.activeLabel??void 0,projectID:a.data.activeProject??void 0});g(M,[...p(M),...e.beads.edges],!0),g(N,e.beads.pageInfo,!0)}finally{g(P,!1)}}}function _e(e){switch(e){case`open`:return`badge-status-open`;case`in-progress`:return`badge-status-in-progress`;case`closed`:return`badge-status-closed`;case`blocked`:return`badge-status-blocked`;case`running`:return`badge-status-running`;case`completed`:return`badge-status-completed`;case`failed`:return`badge-status-failed`;default:return`badge-status-neutral`}}function H(e){return e?`rounded-sm border px-3 py-1 text-xs font-medium border-accent-lever bg-accent-lever/10 text-accent-lever dark:border-dark-accent-lever dark:bg-dark-accent-lever/20 dark:text-dark-accent-lever`:`rounded-sm border px-3 py-1 text-xs font-medium border-border-line bg-bg-elevated text-fg-ink hover:border-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-ink dark:hover:bg-dark-bg-surface`}function ve(e){return e?a.data.projectNames[e]??e:`—`}var U=A(),W=h(U),G=y(h(W),2),ye=h(G);i(G),i(W);var K=y(W,2),q=y(h(K),2);c(q,17,()=>j,l,(e,n)=>{var r=re(),c=h(r,!0);i(r),m(e=>{t(r,1,e),s(c,p(n))},[()=>x(H(a.data.activeStatus===p(n)))]),o(`click`,r,()=>V(p(n))),C(e,r)});var be=y(q,2),xe=e=>{var t=ie();o(`click`,t,()=>B(`status`,null)),C(e,t)};r(be,e=>{a.data.activeStatus&&e(xe)}),i(K);var J=y(K,2),Se=e=>{var n=k(),u=y(h(n),2);c(u,17,()=>a.data.projects,l,(e,n)=>{var r=ae(),c=h(r,!0);i(r),m(e=>{t(r,1,e),s(c,p(n).name)},[()=>x(H(a.data.activeProject===p(n).id))]),o(`click`,r,()=>he(p(n).id)),C(e,r)});var d=y(u,2),f=e=>{var t=oe();o(`click`,t,()=>B(`project`,null)),C(e,t)};r(d,e=>{a.data.activeProject&&e(f)}),i(n),C(e,n)};r(J,e=>{a.data.projects.length>0&&e(Se)});var Y=y(J,2),Ce=e=>{var n=le(),u=y(h(n),2);c(u,17,()=>p(z),l,(e,n)=>{var r=se(),c=h(r,!0);i(r),m(e=>{t(r,1,e),s(c,p(n))},[()=>x(H(a.data.activeLabel===p(n)))]),o(`click`,r,()=>me(p(n))),C(e,r)});var d=y(u,2),f=e=>{var t=ce();o(`click`,t,()=>B(`label`,null)),C(e,t)};r(d,e=>{a.data.activeLabel&&e(f)}),i(n),C(e,n)};r(Y,e=>{p(z).length>0&&e(Ce)});var X=y(Y,2),Z=h(X),Q=y(h(Z)),$=h(Q);c($,17,()=>p(L),e=>e.cursor,(e,n)=>{var r=ue(),a=h(r),o=h(a,!0);i(a);var c=y(a),l=h(c,!0);i(c);var u=y(c),d=h(u),f=h(d,!0);i(d),i(u);var g=y(u),_=h(g),v=h(_,!0);i(_),i(g);var b=y(g),x=h(b);i(b),i(r),m((e,r)=>{s(o,p(n).node.id),s(l,p(n).node.title),s(f,e),t(_,1,`inline-block border px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${r??``}`),s(v,p(n).node.status),t(b,1,`px-4 py-3 text-right font-mono-code text-xs font-medium text-priority-p${p(n).node.priority??``}`),s(x,`P${p(n).node.priority??``}`)},[()=>ve(p(n).node.projectID),()=>_e(p(n).node.status)]),C(e,r)});var we=y($),Te=e=>{C(e,de())};r(we,e=>{p(L).length===0&&e(Te)}),i(Q),i(Z),i(X);var Ee=y(X,2),De=e=>{var t=fe(),n=h(t),r=h(n,!0);i(n),i(t),m(()=>{n.disabled=p(P),s(r,p(P)?`Loading…`:`Load more`)}),o(`click`,n,ge),C(e,t)};r(Ee,e=>{p(R).hasNextPage&&e(De)}),i(U),m(()=>s(ye,`${p(L).length??``} of ${p(pe)??``}`)),C(e,U),n(),D()}a([`click`]);export{j as component,E as universal};
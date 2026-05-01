import{t as e}from"../chunks/CuWRZlNB.js";import{A as t,B as n,C as r,Dt as i,F as a,Ft as o,G as s,K as c,L as l,M as u,N as d,Ot as f,P as p,Pt as m,St as h,V as g,Z as _,a as v,at as y,c as b,d as ee,dt as x,ft as S,g as C,gt as w,h as T,k as E,mt as D,o as O,ot as k,p as A,pt as j,u as M,vt as N,xt as P,yt as F,z as I}from"../chunks/D5KUolLB.js";import{n as L,t as R}from"../chunks/k-mresXx.js";import"../chunks/Q94VlVm2.js";import{n as z,t as B}from"../chunks/CiWV3YVb.js";import"../chunks/D93nnCPt.js";import{t as V}from"../chunks/BbtnOfMK.js";import{t as H}from"../chunks/DSqzhCK4.js";import{t as U}from"../chunks/yG-R4fkq.js";import{t as W}from"../chunks/COvM7PrA.js";import{t as te}from"../chunks/B5ImfunL.js";import{t as ne}from"../chunks/BOinCphV.js";import{t as re}from"../chunks/DmL3C-GQ.js";import{t as ie}from"../chunks/DPEH9Lqz.js";import{t as ae}from"../chunks/PDVrgSnN.js";import{t as oe}from"../chunks/nsYM5RRm.js";function se(e,r){let i=v(r,[`children`,`$$slots`,`$$events`,`$$legacy`]),a=[[`path`,{d:`M20 6 9 17l-5-5`}]];W(e,b({name:`check`},()=>i,{get iconNode(){return a},children:(e,i)=>{var a=n();t(S(a),r,`default`,{},null),I(e,a)},$$slots:{default:!0}}))}function ce(e,r){let i=v(r,[`children`,`$$slots`,`$$events`,`$$legacy`]),a=[[`path`,{d:`M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2`}],[`circle`,{cx:`9`,cy:`7`,r:`4`}],[`line`,{x1:`19`,x2:`19`,y1:`8`,y2:`14`}],[`line`,{x1:`22`,x2:`16`,y1:`11`,y2:`11`}]];W(e,b({name:`user-plus`},()=>i,{get iconNode(){return a},children:(e,i)=>{var a=n();t(S(a),r,`default`,{},null),I(e,a)},$$slots:{default:!0}}))}var G=e({load:()=>ue}),le=z`
	query Bead($id: ID!, $projectID: String!) {
		bead(id: $id) {
			id
			title
			status
			priority
			issueType
			owner
			createdAt
			createdBy
			updatedAt
			labels
			parent
			description
			acceptance
			notes
			dependencies {
				issueId
				dependsOnId
				type
				createdAt
				createdBy
			}
		}
		projectBeads: beadsByProject(projectID: $projectID, first: 500) {
			edges {
				node {
					id
					parent
				}
			}
		}
		beadExecutions: executions(projectId: $projectID, beadId: $id, first: 50) {
			edges {
				node {
					id
					verdict
					harness
					createdAt
					durationMs
					costUsd
				}
			}
			totalCount
		}
	}
`,ue=async({params:e,fetch:t})=>{let n=await B(t).request(le,{id:e.beadId,projectID:e.projectId}),r=n.projectBeads?.edges.filter(e=>e.node.parent===n.bead?.id).length??0,i=n.beadExecutions?.edges.map(e=>e.node)??[];return{bead:n.bead?{...n.bead,childCount:r}:null,nodeId:e.nodeId,projectId:e.projectId,executions:i}};function de(e,t){return e===t}var fe=g(`<div class="mb-3"><!></div>`),pe=g(`<!> <label class="block text-sm font-medium text-fg-ink dark:text-dark-fg-ink"> </label> <input type="text" autocomplete="off" autocapitalize="off" spellcheck="false" class="mt-2 w-full rounded-none border border-border-line bg-bg-elevated px-3 py-2 font-mono text-sm text-fg-ink placeholder-fg-muted focus:border-accent-lever focus:ring-1 focus:ring-accent-lever focus:outline-none dark:border-dark-border-line dark:bg-dark-bg-surface dark:text-dark-fg-ink dark:placeholder-dark-fg-muted dark:focus:border-dark-accent-lever" aria-describedby="typed-confirm-dialog-expected"/> <p id="typed-confirm-dialog-expected" class="mt-2 font-mono text-xs text-fg-muted dark:text-dark-fg-muted"> </p>`,1);function me(e,t){f(t,!0);let r=O(t,`open`,15,!1),s=O(t,`title`,19,()=>t.actionLabel),c=O(t,`cancelLabel`,3,`Cancel`),u=O(t,`expectedLabel`,3,`confirmation text`),d=O(t,`destructive`,3,!1),p=O(t,`confirmDisabled`,3,!1),m=O(t,`returnFocusTo`,3,null),h=N(``),g=`typed-confirm-dialog-input`,v=F(()=>de(_(h),t.expectedText));k(()=>{r()&&w(h,``)});{let i=e=>{var n=pe(),r=S(n),i=e=>{var n=fe(),r=x(n);t.summary(r),o(n),I(e,n)};a(r,e=>{t.summary&&e(i)});var s=j(r,2);C(s,`for`,g);var c=x(s);o(s);var d=j(s,2);T(d),C(d,`id`,g);var f=j(d,2),p=x(f,!0);o(f),y(()=>{l(c,`Type the ${u()??``} to confirm`),C(d,`placeholder`,t.expectedText),l(p,t.expectedText)}),A(d,()=>_(h),e=>w(h,e)),I(e,n)},f=F(()=>p()||!_(v));U(e,{get actionLabel(){return t.actionLabel},get title(){return s()},get cancelLabel(){return c()},get destructive(){return d()},get returnFocusTo(){return m()},get confirmDisabled(){return _(f)},get onConfirm(){return t.onConfirm},get onCancel(){return t.onCancel},get onOpenChange(){return t.onOpenChange},get open(){return r()},set open(e){r(e)},summary:i,children:(e,r)=>{var i=n(),o=S(i),s=e=>{var r=n();E(S(r),()=>t.children),I(e,r)};a(o,e=>{t.children&&e(s)}),I(e,i)},$$slots:{summary:!0,default:!0}})}i()}var he=g(`<span class="shrink-0 truncate text-xs text-fg-muted dark:text-dark-fg-muted"> </span>`),ge=g(`<button class="flex items-center gap-1.5 rounded-none bg-accent-lever px-3 py-1.5 text-sm font-medium text-white hover:bg-accent-lever/90 disabled:cursor-not-allowed disabled:opacity-50"><!> Claim</button>`),_e=g(`<button class="flex items-center gap-1.5 rounded-none border border-border-line px-3 py-1.5 text-sm font-medium text-fg-muted hover:bg-bg-surface disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-elevated"><!> Unclaim</button>`),ve=g(`<!> <button class="flex items-center gap-1.5 rounded-none border border-border-line px-3 py-1.5 text-sm font-medium text-fg-muted hover:bg-bg-surface dark:border-dark-border-line dark:text-dark-fg-ink dark:hover:bg-dark-bg-elevated"><!> Edit</button> <button class="flex items-center gap-1.5 rounded-none border border-error/30 px-3 py-1.5 text-sm font-medium text-error hover:bg-error/10 disabled:cursor-not-allowed disabled:opacity-50 dark:border-dark-error/30 dark:text-dark-error dark:hover:bg-dark-error/10"><!> Delete</button>`,1),ye=g(`<div class="shrink-0 border-b border-error/30 bg-error/10 px-6 py-2 text-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"> </div>`),be=g(`<div class="col-span-2"><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Parent</dt> <dd class="mt-1 font-mono-code text-xs text-fg-muted dark:text-dark-fg-muted"> </dd></div>`),xe=g(`<span class="rounded-none bg-bg-canvas px-2 py-0.5 text-xs text-fg-muted dark:bg-dark-bg-elevated dark:text-dark-fg-ink"> </span>`),Se=g(`<div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Labels</dt> <dd class="mt-1 flex flex-wrap gap-1"></dd></div>`),Ce=g(`<div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Description</dt> <dd class="mt-1 whitespace-pre-wrap text-fg-muted dark:text-dark-fg-ink"> </dd></div>`),we=g(`<div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Acceptance</dt> <dd class="mt-1 whitespace-pre-wrap text-fg-muted dark:text-dark-fg-ink"> </dd></div>`),Te=g(`<div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Notes</dt> <dd class="mt-1 whitespace-pre-wrap text-fg-muted dark:text-dark-fg-ink"> </dd></div>`),Ee=g(`<span class="rounded-none border border-border-line bg-bg-canvas px-1 py-0.5 text-[10px] uppercase text-fg-muted dark:border-dark-border-line dark:bg-dark-bg-elevated dark:text-dark-fg-muted"> </span>`),De=g(`<a class="flex items-center justify-between rounded-none border border-border-line px-2 py-1 text-xs hover:bg-bg-surface dark:border-dark-border-line dark:hover:bg-dark-bg-elevated"><span class="flex items-center gap-2"><span class="font-mono-code text-accent-lever dark:text-dark-accent-lever"> </span> <!></span> <span class="text-fg-muted dark:text-dark-fg-muted"> </span></a>`),Oe=g(`<div data-testid="bead-executions"><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted"> </dt> <dd class="mt-1 space-y-1"></dd></div>`),ke=g(`<div class="font-mono-code text-xs text-fg-muted dark:text-dark-fg-muted"> <span class="text-fg-muted dark:text-dark-fg-muted"> </span></div>`),Ae=g(`<div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Dependencies</dt> <dd class="mt-1 space-y-1"></dd></div>`),je=g(`<h2 class="mb-5 text-xl font-semibold text-fg-ink dark:text-dark-fg-ink"> </h2> <dl class="space-y-4 text-sm"><div class="grid grid-cols-2 gap-4"><div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Priority</dt> <dd class="mt-1 text-fg-ink dark:text-dark-fg-ink"> </dd></div> <div><dt class="text-xs font-medium tracking-wide text-fg-muted uppercase dark:text-dark-fg-muted">Type</dt> <dd class="mt-1 text-fg-ink dark:text-dark-fg-ink"> </dd></div> <!></div> <!> <!> <!> <!> <!> <!> <div class="border-t border-border-line pt-4 text-xs text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted"><div> </div> <div> </div></div></dl>`,1),Me=g(`<span>This closes <span class="font-mono"> </span> as deleted.</span>`),Ne=g(`<label class="mt-4 flex items-start gap-3 rounded-none border border-error/30 bg-error/10 p-3 text-sm text-error dark:border-dark-error/30 dark:bg-dark-error/10 dark:text-dark-error"><input type="checkbox" class="mt-1 h-4 w-4 rounded-none border-error/50 text-error focus:ring-error dark:border-dark-error/50 dark:bg-dark-bg-elevated"/> <span><span class="block font-medium">Cascade to child beads</span> <span class="block text-xs text-error dark:text-dark-error"> </span></span></label>`),Pe=g(`<div class="fixed top-0 right-0 z-50 flex h-full w-full max-w-xl flex-col bg-bg-elevated shadow-xl dark:bg-dark-bg-canvas" style="max-width: 36rem;"><div class="flex shrink-0 items-center justify-between border-b border-border-line px-6 py-4 dark:border-dark-border-line"><div class="flex min-w-0 items-center gap-3"><span data-testid="bead-detail-id" class="min-w-0 truncate font-mono-code text-xs text-fg-muted dark:text-dark-fg-muted"> </span> <button type="button" aria-label="Copy bead id" data-testid="bead-detail-copy-id" class="shrink-0 rounded-none p-1 text-fg-muted hover:bg-bg-canvas hover:text-fg-ink dark:hover:bg-dark-bg-elevated dark:hover:text-dark-fg-ink"><!></button> <span> </span> <!></div> <div class="ml-3 flex shrink-0 items-center gap-2"><!> <button class="rounded-none p-1.5 text-fg-muted hover:bg-bg-canvas dark:text-dark-fg-muted dark:hover:bg-dark-bg-elevated" aria-label="Close panel"><!></button></div></div> <!> <div class="flex-1 overflow-auto p-6"><!></div> <!></div>`);function Fe(e,t){f(t,!0);let s=O(t,`executions`,19,()=>[]),h=O(t,`nodeId`,3,``),g=O(t,`projectId`,3,``);function v(e){return`/nodes/${h()}/projects/${g()}/executions/${e}`}function b(e){try{return new Date(e).toLocaleString()}catch{return e}}let E=N(D({...t.bead})),k=N(!1),A=N(!1),P=N(null),R=N(!1),V=N(!1),U=N(null),W=N(!1),G=null,le=F(()=>(_(E).childCount??0)>0);async function ue(){try{await navigator.clipboard.writeText(_(E).id),w(W,!0),G&&clearTimeout(G),G=setTimeout(()=>{w(W,!1)},1500)}catch{}}let de=z`
		mutation BeadClaim($id: ID!, $assignee: String!) {
			beadClaim(id: $id, assignee: $assignee) {
				id
				title
				status
				priority
				issueType
				owner
				createdAt
				createdBy
				updatedAt
				labels
				parent
				description
				acceptance
				notes
				dependencies {
					issueId
					dependsOnId
					type
					createdAt
					createdBy
				}
			}
		}
	`,fe=z`
		mutation BeadUnclaim($id: ID!) {
			beadUnclaim(id: $id) {
				id
				title
				status
				priority
				issueType
				owner
				createdAt
				createdBy
				updatedAt
				labels
				parent
				description
				acceptance
				notes
				dependencies {
					issueId
					dependsOnId
					type
					createdAt
					createdBy
				}
			}
		}
	`,pe=z`
		mutation BeadClose($id: ID!, $reason: String) {
			beadClose(id: $id, reason: $reason) {
				id
				title
				status
				priority
				issueType
				owner
				createdAt
				createdBy
				updatedAt
				labels
				parent
				description
				acceptance
				notes
				dependencies {
					issueId
					dependsOnId
					type
					createdAt
					createdBy
				}
			}
		}
	`;async function Fe(){w(A,!0),w(P,null);try{let e=B(),t=H.value?.name??`user`;w(E,(await e.request(de,{id:_(E).id,assignee:t})).beadClaim,!0),L()}catch(e){w(P,e instanceof Error?e.message:`Claim failed`,!0)}finally{w(A,!1)}}async function Ie(){w(A,!0),w(P,null);try{w(E,(await B().request(fe,{id:_(E).id})).beadUnclaim,!0),L()}catch(e){w(P,e instanceof Error?e.message:`Unclaim failed`,!0)}finally{w(A,!1)}}function Le(){w(V,!1),w(R,!0)}async function Re(){w(A,!0),w(P,null);try{await B().request(pe,{id:_(E).id,reason:`deleted via UI`}),await L(),t.onClose()}catch(e){w(P,e instanceof Error?e.message:`Delete failed`,!0)}finally{w(A,!1)}}function ze(e){switch(e){case`open`:return`text-accent-lever dark:text-dark-accent-lever`;case`in-progress`:return`text-accent-load dark:text-dark-accent-load`;case`closed`:return`text-status-closed dark:text-status-closed`;case`blocked`:return`text-error dark:text-dark-error`;default:return`text-fg-muted dark:text-dark-fg-muted`}}var K=Pe(),q=x(K),J=x(q),Y=x(J),Be=x(Y,!0);o(Y);var X=j(Y,2),Ve=x(X),He=e=>{se(e,{class:`h-3.5 w-3.5 text-status-closed`})},Ue=e=>{te(e,{class:`h-3.5 w-3.5`})};a(Ve,e=>{_(W)?e(He):e(Ue,-1)}),o(X);var Z=j(X,2),We=x(Z,!0);o(Z);var Ge=j(Z,2),Ke=e=>{var t=he(),n=x(t);o(t),y(()=>l(n,`@ ${_(E).owner??``}`)),I(e,t)};a(Ge,e=>{_(E).owner&&e(Ke)}),o(J);var qe=j(J,2),Je=x(qe),Ye=e=>{var t=ve(),n=S(t),r=e=>{var t=ge();ce(x(t),{class:`h-3.5 w-3.5`}),m(),o(t),y(()=>t.disabled=_(A)),c(`click`,t,Fe),I(e,t)},i=e=>{var t=_e();ie(x(t),{class:`h-3.5 w-3.5`}),m(),o(t),y(()=>t.disabled=_(A)),c(`click`,t,Ie),I(e,t)};a(n,e=>{_(E).status===`open`||_(E).status===`blocked`?e(r):_(E).status===`in-progress`&&e(i,1)});var s=j(n,2);ne(x(s),{class:`h-3.5 w-3.5`}),m(),o(s);var l=j(s,2);re(x(l),{class:`h-3.5 w-3.5`}),m(),o(l),M(l,e=>w(U,e),()=>_(U)),y(()=>{s.disabled=_(A),l.disabled=_(A)}),c(`click`,s,()=>w(k,!0)),c(`click`,l,Le),I(e,t)};a(Je,e=>{_(k)||e(Ye)});var Q=j(Je,2);ae(x(Q),{class:`h-4 w-4`}),o(Q),o(qe),o(q);var Xe=j(q,2),Ze=e=>{var t=ye(),n=x(t,!0);o(t),y(()=>l(n,_(P))),I(e,t)};a(Xe,e=>{_(P)&&e(Ze)});var $=j(Xe,2),Qe=x($),$e=e=>{var t=n();p(S(t),()=>_(E)?.id,e=>{oe(e,{get bead(){return _(E)},onSuccess:e=>{w(E,e,!0),w(k,!1)},onCancel:()=>w(k,!1)})}),I(e,t)},et=e=>{var t=je(),n=S(t),r=x(n,!0);o(n);var i=j(n,2),c=x(i),f=x(c),p=j(x(f),2),m=x(p,!0);o(p),o(f);var h=j(f,2),g=j(x(h),2),ee=x(g,!0);o(g),o(h);var w=j(h,2),T=e=>{var t=be(),n=j(x(t),2),r=x(n,!0);o(n),o(t),y(()=>l(r,_(E).parent)),I(e,t)};a(w,e=>{_(E).parent&&e(T)}),o(c);var D=j(c,2),O=e=>{var t=Se(),n=j(x(t),2);u(n,21,()=>_(E).labels,d,(e,t)=>{var n=xe(),r=x(n,!0);o(n),y(()=>l(r,_(t))),I(e,n)}),o(n),o(t),I(e,t)};a(D,e=>{_(E).labels&&_(E).labels.length>0&&e(O)});var k=j(D,2),A=e=>{var t=Ce(),n=j(x(t),2),r=x(n,!0);o(n),o(t),y(()=>l(r,_(E).description)),I(e,t)};a(k,e=>{_(E).description&&e(A)});var M=j(k,2),N=e=>{var t=we(),n=j(x(t),2),r=x(n,!0);o(n),o(t),y(()=>l(r,_(E).acceptance)),I(e,t)};a(M,e=>{_(E).acceptance&&e(N)});var P=j(M,2),F=e=>{var t=Te(),n=j(x(t),2),r=x(n,!0);o(n),o(t),y(()=>l(r,_(E).notes)),I(e,t)};a(P,e=>{_(E).notes&&e(F)});var L=j(P,2),R=e=>{var t=Oe(),n=x(t),r=x(n);o(n);var i=j(n,2);u(i,21,s,e=>e.id,(e,t)=>{var n=De(),r=x(n),i=x(r),s=x(i,!0);o(i);var c=j(i,2),u=e=>{var n=Ee(),r=x(n,!0);o(n),y(()=>l(r,_(t).verdict)),I(e,n)};a(c,e=>{_(t).verdict&&e(u)}),o(r);var d=j(r,2),f=x(d,!0);o(d),o(n),y((e,r)=>{C(n,`href`,e),l(s,_(t).id),l(f,r)},[()=>v(_(t).id),()=>b(_(t).createdAt)]),I(e,n)}),o(i),o(t),y(()=>l(r,`Executions (${s().length??``})`)),I(e,t)};a(L,e=>{s().length>0&&e(R)});var z=j(L,2),B=e=>{var t=Ae(),n=j(x(t),2);u(n,21,()=>_(E).dependencies,d,(e,t)=>{var n=ke(),r=x(n),i=j(r),a=x(i);o(i),o(n),y(()=>{l(r,`${_(t).dependsOnId??``} `),l(a,`(${_(t).type??``})`)}),I(e,n)}),o(n),o(t),I(e,t)};a(z,e=>{_(E).dependencies&&_(E).dependencies.length>0&&e(B)});var V=j(z,2),H=x(V),U=x(H);o(H);var W=j(H,2),te=x(W);o(W),o(V),o(i),y((e,t)=>{l(r,_(E).title),l(m,_(E).priority),l(ee,_(E).issueType||`—`),l(U,`Created: ${e??``}${_(E).createdBy?` by ${_(E).createdBy}`:``}`),l(te,`Updated: ${t??``}`)},[()=>new Date(_(E).createdAt).toLocaleString(),()=>new Date(_(E).updatedAt).toLocaleString()]),I(e,t)};a(Qe,e=>{_(k)?e($e):e(et,-1)}),o($),me(j($,2),{actionLabel:`Delete bead`,title:`Delete bead`,get expectedText(){return _(E).id},expectedLabel:`bead id`,destructive:!0,get confirmDisabled(){return _(A)},get returnFocusTo(){return _(U)},onConfirm:Re,get open(){return _(R)},set open(e){w(R,e,!0)},summary:e=>{var t=Me(),n=j(x(t)),r=x(n,!0);o(n),m(),o(t),y(()=>l(r,_(E).id)),I(e,t)},children:(e,t)=>{var r=n(),i=S(r),s=e=>{var t=Ne(),n=x(t);T(n);var r=j(n,2),i=j(x(r),2),a=x(i);o(i),o(r),o(t),y(()=>l(a,`Apply the delete intent to ${_(E).childCount??``} child ${_(E).childCount===1?`bead`:`beads`}.`)),ee(n,()=>_(V),e=>w(V,e)),I(e,t)};a(i,e=>{_(le)&&e(s)}),I(e,r)},$$slots:{summary:!0,default:!0}}),o(K),y(e=>{C(Y,`title`,_(E).id),l(Be,_(E).id),r(Z,1,`shrink-0 font-medium ${e??``}`),l(We,_(E).status)},[()=>ze(_(E).status)]),c(`click`,X,ue),c(`click`,Q,function(...e){t.onClose?.apply(this,e)}),I(e,K),i()}s([`click`]);var Ie=g(`<div class="fixed inset-0 z-40 bg-black/20 dark:bg-black/40" role="button" tabindex="-1" aria-label="Close panel"></div> <!>`,1);function Le(e,t){f(t,!0);let r=()=>h(V,`$page`,o),[o,s]=P();function l(){let e=r().url.pathname.split(`/`);e.pop();let t=e.join(`/`),n=r().url.searchParams.toString();R(n?`${t}?${n}`:t)}var u=n(),d=S(u),m=e=>{var n=Ie(),r=S(n);p(j(r,2),()=>t.data.bead.id,e=>{Fe(e,{get bead(){return t.data.bead},onClose:l,get executions(){return t.data.executions},get nodeId(){return t.data.nodeId},get projectId(){return t.data.projectId}})}),c(`click`,r,l),c(`keydown`,r,e=>e.key===`Escape`&&l()),I(e,n)};a(d,e=>{t.data.bead&&e(m)}),I(e,u),i(),s()}s([`click`,`keydown`]);export{Le as component,G as universal};
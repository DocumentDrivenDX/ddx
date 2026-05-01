import{t as e}from"../chunks/D-_T_ixn.js";import{B as t,Dt as n,F as r,Ft as i,G as a,K as o,L as s,M as c,Ot as l,St as u,V as d,W as f,Z as p,at as m,dt as h,ft as g,g as _,gt as v,mt as y,ot as b,pt as x,q as ee,u as te,vt as S,xt as C,yt as w,z as T}from"../chunks/C9gjXInc.js";import{t as ne}from"../chunks/DrjC4ShN.js";import"../chunks/BpAyAfhb.js";import{n as E,t as D}from"../chunks/DBr1FBxI.js";import"../chunks/Dx2xiYXE.js";import{t as O}from"../chunks/DLleOg3b.js";import{n as re,r as k}from"../chunks/DaIVqPkW.js";var A=e({load:()=>P}),j=E`
	query WorkerDetail($id: ID!) {
		worker(id: $id) {
			id
			kind
			state
			status
			harness
			model
			effort
			once
			pollInterval
			startedAt
			finishedAt
			currentBead
			lastError
			attempts
			successes
			failures
			currentAttempt {
				attemptId
				beadId
				phase
				startedAt
				elapsedMs
			}
			recentEvents {
				kind
				text
				name
				inputs
				output
			}
			lifecycleEvents {
				action
				actor
				timestamp
				detail
				beadId
			}
		}
	}
`,M=E`
	query WorkerLog($workerID: ID!) {
		workerLog(workerID: $workerID) {
			stdout
			stderr
		}
	}
`,N=E`
	query WorkerSessions($first: Int) {
		agentSessions(first: $first) {
			edges {
				node {
					id
					projectId
					workerId
					beadId
					harness
					model
					status
					startedAt
					durationMs
					cost
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
`,P=async({params:e,fetch:t})=>{let n=D(t),[r,i,a]=await Promise.all([n.request(j,{id:e.workerId}),n.request(M,{workerID:e.workerId}).catch(()=>({workerLog:{stdout:``,stderr:``}})),n.request(N,{first:100}).catch(()=>({agentSessions:{edges:[],pageInfo:{hasNextPage:!1,endCursor:null},totalCount:0}}))]),o=a.agentSessions.edges.map(e=>e.node).filter(t=>t.projectId===e.projectId&&t.workerId===e.workerId);return{nodeId:e.nodeId,projectId:e.projectId,worker:r.worker?{...r.worker,recentEvents:r.worker.recentEvents??[],lifecycleEvents:r.worker.lifecycleEvents??[]}:null,initialLog:i.workerLog.stdout,sessions:o}},ie=d(`<div><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Harness</div> <div class="mt-0.5 text-fg-ink dark:text-dark-fg-ink"> </div></div>`),ae=d(`<div><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Model</div> <div class="mt-0.5 text-fg-ink dark:text-dark-fg-ink"> </div></div>`),oe=d(`<div><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Effort</div> <div class="mt-0.5 text-fg-ink dark:text-dark-fg-ink"> </div></div>`),se=d(`<div class="col-span-2"><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Current bead</div> <div class="mt-0.5 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink"> </div></div>`),ce=d(`<div><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Attempts</div> <div class="mt-0.5 text-fg-ink dark:text-dark-fg-ink"> <span class="text-mono-code text-fg-muted dark:text-dark-fg-muted"> </span></div></div>`),le=d(`<div><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Phase</div> <div class="mt-0.5"><span class="font-medium text-status-in-progress"> </span> <span class="ml-1 text-mono-code text-fg-muted dark:text-dark-fg-muted"> </span></div></div>`),ue=d(`<div class="col-span-2"><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Last error</div> <div class="mt-0.5 text-status-failed"> </div></div>`),de=d(`<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No sessions recorded yet.</p>`),fe=d(`<tr class="border-t border-border-line dark:border-dark-border-line"><td class="px-3 py-2"><div class="font-mono-code text-mono-code text-lever"> </div> <div class="text-fg-muted dark:text-dark-fg-muted"> </div></td><td class="px-3 py-2 font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"> </td><td class="px-3 py-2 text-fg-ink dark:text-dark-fg-ink"> </td><td class="px-3 py-2 text-right font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"> </td></tr>`),pe=d(`<div class="overflow-hidden border border-border-line dark:border-dark-border-line"><table class="w-full text-xs"><thead class="bg-bg-surface text-fg-muted dark:bg-dark-bg-surface dark:text-dark-fg-muted"><tr><th class="px-3 py-2 text-left text-label-caps font-label-caps uppercase tracking-wide">Session</th><th class="px-3 py-2 text-left text-label-caps font-label-caps uppercase tracking-wide">Bead</th><th class="px-3 py-2 text-left text-label-caps font-label-caps uppercase tracking-wide">Status</th><th class="px-3 py-2 text-right text-label-caps font-label-caps uppercase tracking-wide">Cost</th></tr></thead><tbody></tbody></table></div>`),me=d(`<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">No lifecycle actions recorded.</p>`),he=d(`<span class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"> </span>`),ge=d(`<div class="mt-0.5 text-fg-muted dark:text-dark-fg-muted"> </div>`),_e=d(`<li class="flex items-start justify-between gap-3 text-xs"><div><span class="font-medium text-fg-ink dark:text-dark-fg-ink"> </span> <span class="text-fg-muted dark:text-dark-fg-muted"> </span> <!> <!></div> <time class="shrink-0 text-fg-muted dark:text-dark-fg-muted"> </time></li>`),ve=d(`<ul class="space-y-2"></ul>`),ye=d(`<div class="alert-caution border px-2 py-1 text-xs">Reconnecting…</div>`),be=d(`<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted">Waiting for response…</p>`),xe=d(`<p class="whitespace-pre-wrap"> </p>`),Se=d(`<div class="border-t border-border-line px-3 pt-3 pb-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:border-dark-border-line dark:text-dark-fg-muted">Output</div> <pre class="overflow-x-auto px-3 pb-3 font-mono-code text-mono-code whitespace-pre-wrap"> </pre>`,1),Ce=d(`<details class="border border-border-line dark:border-dark-border-line"><summary role="button" class="cursor-pointer px-3 py-2 font-mono-code text-mono-code text-fg-ink dark:text-dark-fg-ink"> </summary> <div class="border-t border-border-line dark:border-dark-border-line"><div class="px-3 pt-3 pb-1 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Inputs</div> <pre class="overflow-x-auto px-3 pb-3 font-mono-code text-mono-code whitespace-pre-wrap"> </pre> <!></div></details>`),we=d(`<p class="text-body-sm text-fg-muted dark:text-dark-fg-muted"> <a class="text-accent-lever hover:underline dark:text-dark-accent-lever">Evidence bundle</a></p>`),Te=d(`<button class="px-2 py-0.5 text-xs text-accent-lever hover:bg-bg-surface dark:text-dark-accent-lever dark:hover:bg-dark-bg-surface">↓ Follow</button>`),Ee=d(`<span class="text-gray-600 dark:text-gray-500">No log output yet…</span>`),De=d(`<div class="fixed inset-0 z-40 bg-black/20 dark:bg-black/40" role="button" tabindex="-1" aria-label="Dismiss panel"></div> <div class="fixed top-0 right-0 z-50 flex h-full w-full max-w-2xl flex-col bg-bg-elevated shadow-xl dark:bg-dark-bg-elevated"><div class="flex shrink-0 items-center justify-between border-b border-border-line px-6 py-4 dark:border-dark-border-line"><div><h2 class="text-headline-md font-headline-md text-fg-ink dark:text-dark-fg-ink"> </h2> <p class="font-mono-code text-mono-code text-fg-muted dark:text-dark-fg-muted"> </p></div> <button class="p-1.5 text-fg-muted hover:bg-bg-surface hover:text-fg-ink dark:text-dark-fg-muted dark:hover:bg-dark-bg-surface dark:hover:text-dark-fg-ink" aria-label="Close">✕</button></div> <div class="grid shrink-0 grid-cols-2 gap-x-6 gap-y-3 border-b border-border-line px-6 py-4 text-sm dark:border-dark-border-line"><div><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">State</div> <div class="mt-0.5 font-medium text-fg-ink dark:text-dark-fg-ink"> </div></div> <!> <!> <!> <!> <!> <!> <!></div> <section class="shrink-0 border-b border-border-line px-6 py-4 text-sm dark:border-dark-border-line"><div class="mb-3 flex items-center justify-between gap-3"><h3 class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Sessions</h3> <a class="text-body-sm text-accent-lever hover:underline dark:text-dark-accent-lever">All sessions</a></div> <!></section> <section class="shrink-0 border-b border-border-line px-6 py-4 text-sm dark:border-dark-border-line"><div class="mb-3 text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Lifecycle audit</div> <!></section> <section role="region" aria-label="Live response" aria-live="polite" class="shrink-0 border-b border-border-line px-6 py-4 text-sm dark:border-dark-border-line"><div class="mb-2 flex items-center justify-between gap-3"><div class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Live response</div> <!></div> <div aria-live="polite" class="space-y-2 text-fg-ink dark:text-dark-fg-ink"><!> <!></div></section> <div class="flex min-h-0 flex-1 flex-col"><div class="flex shrink-0 items-center justify-between border-b border-border-line px-4 py-2 dark:border-dark-border-line"><span class="text-label-caps font-label-caps uppercase tracking-wide text-fg-muted dark:text-dark-fg-muted">Log output</span> <div class="flex items-center gap-3"><span class="text-mono-code text-fg-muted dark:text-dark-fg-muted"> </span> <!></div></div> <pre class="flex-1 overflow-auto bg-gray-950 px-4 py-3 font-mono text-xs leading-relaxed text-green-400 dark:text-green-300"><!></pre></div></div>`,1);function F(e,a){l(a,!0);let d=()=>u(O,`$page`,A),[A,j]=C(),M=S(y([])),N=S(null),P=S(!0),F=S(y([])),I=S(!1),L=S(!1),R=S(!1),z=S(null),B=S(`idle`),V=E`
		query WorkerRecentEvents($id: ID!) {
			worker(id: $id) {
				id
				recentEvents {
					kind
					text
					name
					inputs
					output
				}
			}
		}
	`;b(()=>{let e=a.data.initialLog??``;v(M,e.length>0?e.split(`
`):[],!0)}),b(()=>{v(F,a.data.worker?.recentEvents??[],!0),v(R,!1),v(z,null)}),b(()=>{p(M).length,p(P)&&p(N)&&Promise.resolve().then(()=>{p(N)&&(p(N).scrollTop=p(N).scrollHeight)})}),b(()=>{let e=a.data.worker?.id;if(!(!e||p(K)))return re(e,e=>{if(G.has(e.phase)&&(v(R,!0),v(z,e.timestamp,!0)),!(p(K)||G.has(e.phase))&&e.logLine!=null&&e.logLine.length>0){v(M,[...p(M),e.logLine],!0);let t=Ve(e.logLine);t&&Be(t)}})}),b(()=>{let e=k.state;v(I,k.showBanner||p(L),!0),a.data.worker?.id&&p(B)!==`idle`&&p(B)!==`connected`&&e===`connected`&&He(a.data.worker.id),v(B,e,!0)});function Oe(){p(N)&&v(P,p(N).scrollHeight-p(N).scrollTop-p(N).clientHeight<20)}function H(){let e=d().url.pathname.split(`/`);e.pop(),ne(e.join(`/`))}function ke(e){return e<1e3?`${e}ms`:e<6e4?`${(e/1e3).toFixed(1)}s`:`${Math.floor(e/6e4)}m${Math.floor(e%6e4/1e3)}s`}function U(e){return e==null?``:typeof e==`string`?e:JSON.stringify(e)}function Ae(e){let t=U(e.inputs),n=je(e.inputs);return n&&t?`${e.name??`tool`} path ${n} ${t}`:t?`${e.name??`tool`} ${t}`:e.name??`tool`}function je(e){let t=typeof e==`string`?W(e):e;if(!t||typeof t!=`object`||Array.isArray(t))return``;let n=t.path??t.file;return typeof n!=`string`||n.length===0?``:n.split(`/`).pop()??n}function W(e){try{return JSON.parse(e)}catch{return null}}function Me(e){return`/executions/${encodeURIComponent(e)}/result.json`}function Ne(e){if(!e)return`terminal state`;let t=new Date(e);return Number.isNaN(t.getTime())?e:t.toLocaleTimeString([],{hour:`2-digit`,minute:`2-digit`,second:`2-digit`})}function Pe(e){let t=new Date(e);return Number.isNaN(t.getTime())?e:t.toLocaleString()}function Fe(e){return e<1e3?`${e}ms`:e<6e4?`${(e/1e3).toFixed(1)}s`:`${Math.floor(e/6e4)}m ${Math.floor(e%6e4/1e3)}s`}function Ie(e){return e==null?`—`:`$${e.toFixed(4)}`}function Le(){return`/nodes/${a.data.nodeId}/projects/${a.data.projectId}/sessions`}let Re=w(()=>a.data.sessions??[]),ze=w(()=>a.data.worker?.lifecycleEvents??[]);function Be(e){v(F,[...p(F),e],!0)}function Ve(e){let t=e.trim();if(!t.startsWith(`{`))return null;try{let e=JSON.parse(t),n=String(e.kind??e.type??``),r=e.data&&typeof e.data==`object`?e.data:e;if(n===`text_delta`){let t=e.text??r.text??r.delta;return typeof t==`string`?{kind:`text_delta`,text:t,name:null,inputs:null,output:null}:null}if(n===`tool_call`)return{kind:`tool_call`,text:null,name:typeof r.name==`string`?r.name:null,inputs:U(r.inputs??r.input),output:typeof r.output==`string`?r.output:null}}catch{return null}return null}async function He(e){v(L,!0);try{v(F,(await D(fetch).request(V,{id:e})).worker?.recentEvents??p(F),!0)}catch(e){console.error(`[ddx] worker recentEvents catch-up failed:`,e)}finally{v(L,!1),v(I,k.showBanner,!0)}}let G=new Set([`done`,`exited`,`stopped`,`failed`,`error`,`preserved`]),K=w(()=>a.data.worker?.state===`done`||a.data.worker?.state===`exited`||a.data.worker?.state===`stopped`||a.data.worker?.state===`failed`||a.data.worker?.state===`error`||p(R)||!!a.data.worker?.finishedAt),Ue=w(()=>a.data.worker?.finishedAt??p(z)),We=w(()=>{let e=[];for(let t of p(F))if(t.kind===`text_delta`&&t.text){let n=e.at(-1);n?.type===`text`?n.text+=t.text:e.push({type:`text`,text:t.text})}else t.kind===`tool_call`&&e.push({type:`tool_call`,event:t,key:`${e.length}-${t.name??`tool`}-${U(t.inputs).slice(0,40)}`});return e});var q=t(),Ge=g(q),Ke=e=>{var n=De(),l=g(n),u=x(l,2),d=h(u),y=h(d),b=h(y),S=h(b,!0);i(b);var C=x(b,2),w=h(C,!0);i(C),i(y);var ne=x(y,2);i(d);var E=x(d,2),D=h(E),O=x(h(D),2),re=h(O,!0);i(O),i(D);var k=x(D,2),A=e=>{var t=ie(),n=x(h(t),2),r=h(n,!0);i(n),i(t),m(()=>s(r,a.data.worker.harness)),T(e,t)};r(k,e=>{a.data.worker.harness&&e(A)});var j=x(k,2),F=e=>{var t=ae(),n=x(h(t),2),r=h(n,!0);i(n),i(t),m(()=>s(r,a.data.worker.model)),T(e,t)};r(j,e=>{a.data.worker.model&&e(F)});var L=x(j,2),R=e=>{var t=oe(),n=x(h(t),2),r=h(n,!0);i(n),i(t),m(()=>s(r,a.data.worker.effort)),T(e,t)};r(L,e=>{a.data.worker.effort&&e(R)});var z=x(L,2),B=e=>{var t=se(),n=x(h(t),2),r=h(n,!0);i(n),i(t),m(()=>s(r,a.data.worker.currentBead)),T(e,t)};r(z,e=>{a.data.worker.currentBead&&e(B)});var V=x(z,2),je=e=>{var t=ce(),n=x(h(t),2),r=h(n),o=x(r),c=h(o);i(o),i(n),i(t),m(()=>{s(r,`${a.data.worker.attempts??``} `),s(c,`(${a.data.worker.successes??0??``}✓ / ${a.data.worker.failures??0??``}✗)`)}),T(e,t)};r(V,e=>{a.data.worker.attempts!=null&&e(je)});var W=x(V,2),Be=e=>{var t=le(),n=x(h(t),2),r=h(n),o=h(r,!0);i(r);var c=x(r,2),l=h(c);i(c),i(n),i(t),m(e=>{s(o,a.data.worker.currentAttempt.phase),s(l,`(${e??``})`)},[()=>ke(a.data.worker.currentAttempt.elapsedMs)]),T(e,t)};r(W,e=>{a.data.worker.currentAttempt&&e(Be)});var Ve=x(W,2),He=e=>{var t=ue(),n=x(h(t),2),r=h(n,!0);i(n),i(t),m(()=>s(r,a.data.worker.lastError)),T(e,t)};r(Ve,e=>{a.data.worker.lastError&&e(He)}),i(E);var G=x(E,2),q=h(G),Ge=x(h(q),2);i(q);var Ke=x(q,2),qe=e=>{T(e,de())},Je=e=>{var t=pe(),n=h(t),r=x(h(n));c(r,21,()=>p(Re),e=>e.id,(e,t)=>{var n=fe(),r=h(n),a=h(r),o=h(a,!0);i(a);var c=x(a,2),l=h(c);i(c),i(r);var u=x(r),d=h(u,!0);i(u);var f=x(u),g=h(f,!0);i(f);var _=x(f),v=h(_,!0);i(_),i(n),m((e,n,r)=>{s(o,e),s(l,`${p(t).harness??``} · ${n??``}`),s(d,p(t).beadId??`—`),s(g,p(t).status),s(v,r)},[()=>p(t).id.slice(0,12),()=>Fe(p(t).durationMs),()=>Ie(p(t).cost)]),T(e,n)}),i(r),i(n),i(t),T(e,t)};r(Ke,e=>{p(Re).length===0?e(qe):e(Je,-1)}),i(G);var J=x(G,2),Ye=x(h(J),2),Xe=e=>{T(e,me())},Ze=e=>{var t=ve();c(t,21,()=>p(ze),e=>`${e.action}-${e.timestamp}`,(e,t)=>{var n=_e(),a=h(n),o=h(a),c=h(o,!0);i(o);var l=x(o,2),u=h(l);i(l);var d=x(l,2),f=e=>{var n=he(),r=h(n);i(n),m(()=>s(r,`· ${p(t).beadId??``}`)),T(e,n)};r(d,e=>{p(t).beadId&&e(f)});var g=x(d,2),v=e=>{var n=ge(),r=h(n,!0);i(n),m(()=>s(r,p(t).detail)),T(e,n)};r(g,e=>{p(t).detail&&e(v)}),i(a);var y=x(a,2),b=h(y,!0);i(y),i(n),m(e=>{s(c,p(t).action),s(u,`by ${p(t).actor??``}`),_(y,`datetime`,p(t).timestamp),s(b,e)},[()=>Pe(p(t).timestamp)]),T(e,n)}),i(t),T(e,t)};r(Ye,e=>{p(ze).length===0?e(Xe):e(Ze,-1)}),i(J);var Y=x(J,2),X=h(Y),Qe=x(h(X),2),$e=e=>{T(e,ye())};r(Qe,e=>{p(I)&&!p(K)&&e($e)}),i(X);var et=x(X,2),tt=h(et),nt=e=>{T(e,be())},rt=e=>{var n=t();c(g(n),17,()=>p(We),e=>e.type===`tool_call`?e.key:e.text,(e,n)=>{var a=t(),o=g(a),c=e=>{var t=xe(),r=h(t,!0);i(t),m(()=>s(r,p(n).text)),T(e,t)},l=e=>{var t=Ce(),a=h(t),o=h(a,!0);i(a);var c=x(a,2),l=x(h(c),2),u=h(l,!0);i(l);var d=x(l,2),f=e=>{var t=Se(),r=x(g(t),2),a=h(r,!0);i(r),m(()=>s(a,p(n).event.output)),T(e,t)};r(d,e=>{p(n).event.output&&e(f)}),i(c),i(t),m((e,t)=>{s(o,e),s(u,t)},[()=>Ae(p(n).event),()=>U(p(n).event.inputs)]),T(e,t)};r(o,e=>{p(n).type===`text`?e(c):e(l,-1)}),T(e,a)}),T(e,n)};r(tt,e=>{p(We).length===0?e(nt):e(rt,-1)});var it=x(tt,2),at=e=>{var t=we(),n=h(t),r=x(n);i(t),m((e,t)=>{s(n,`Completed at ${e??``}. `),_(r,`href`,t)},[()=>Ne(p(Ue)),()=>Me(a.data.worker.id)]),T(e,t)};r(it,e=>{p(K)&&e(at)}),i(et),i(Y);var ot=x(Y,2),Z=h(ot),st=x(h(Z),2),Q=h(st),ct=h(Q);i(Q);var lt=x(Q,2),ut=e=>{var t=Te();o(`click`,t,()=>{v(P,!0),p(N)&&(p(N).scrollTop=p(N).scrollHeight)}),T(e,t)};r(lt,e=>{p(P)||e(ut)}),i(st),i(Z);var $=x(Z,2),dt=h($),ft=e=>{T(e,Ee())},pt=e=>{var t=f();m(e=>s(t,e),[()=>p(M).join(`
`)]),T(e,t)};r(dt,e=>{p(M).length===0?e(ft):e(pt,-1)}),i($),te($,e=>v(N,e),()=>p(N)),i(ot),i(u),m(e=>{s(S,a.data.worker.kind),s(w,a.data.worker.id),s(re,a.data.worker.state),_(Ge,`href`,e),s(ct,`${p(M).length??``} lines`)},[()=>Le()]),o(`click`,l,H),o(`keydown`,l,e=>e.key===`Escape`&&H()),o(`click`,ne,H),ee(`scroll`,$,Oe),T(e,n)};r(Ge,e=>{a.data.worker&&e(Ke)}),T(e,q),n(),j()}a([`click`,`keydown`]);export{F as component,A as universal};
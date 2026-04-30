import{C as e,Dt as t,E as n,F as r,Ft as i,L as a,M as o,N as s,Ot as c,S as l,St as u,V as d,W as ee,Z as f,at as p,dt as m,ft as h,g,gt as _,lt as v,mt as y,ot as te,pt as b,r as x,rt as S,vt as C,xt as w,yt as T,z as E}from"../chunks/DkIPlaea.js";import"../chunks/BEDIq91W.js";import{n as D,t as O}from"../chunks/vpGPUDNZ.js";import{t as k}from"../chunks/q4_J4xQI.js";var A=d(`<span class="text-sm text-gray-500 dark:text-gray-400"> </span>`),j=d(`<span class="text-gray-500 dark:text-gray-400">Strategy: <span class="font-medium text-gray-700 dark:text-gray-300"> </span></span>`),M=d(`/ <span class="font-mono text-gray-700 dark:text-gray-300"> </span>`,1),N=d(`<span class="text-gray-500 dark:text-gray-400">Resolves to: <span class="font-medium text-green-700 dark:text-green-400"> </span> <!></span>`),P=d(`<span class="font-medium text-red-600 dark:text-red-400">No healthy candidate available</span>`),ne=d(`<div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-gray-700 dark:bg-gray-800/50"><h2 class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">Current route for default profile</h2> <div class="flex flex-wrap gap-4 text-sm"><span class="text-gray-500 dark:text-gray-400">Model ref: <span class="font-mono font-medium text-gray-900 dark:text-white"> </span></span> <!> <!></div></div>`),F=d(`<div class="py-8 text-center text-sm text-gray-400 dark:text-gray-600" data-testid="loading">Loading agent endpoints…</div>`),re=d(`<div class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400"> </div>`),ie=d(`<span class="ml-1 inline-flex items-center rounded-full bg-blue-100 px-1.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">default</span>`),ae=d(`<span class="ml-1 inline-flex items-center rounded-full bg-red-100 px-1.5 py-0.5 text-xs font-medium text-red-700 dark:bg-red-900/30 dark:text-red-300">cooldown</span>`),oe=d(`<span class="ml-1 text-xs text-gray-400">·</span>`),se=d(`<span class="text-gray-400">not reported</span>`),ce=d(`<div class="flex items-center gap-2"><div class="h-2 w-20 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700"><div class="h-full bg-blue-500"></div></div> <span class="text-xs text-gray-500 tabular-nums dark:text-gray-400"> </span></div>`),le=d(`<span class="text-xs text-gray-400">not reported</span>`),ue=d(`<div class="w-full bg-blue-400 dark:bg-blue-500"></div>`),de=d(`<div class="flex h-6 w-24 items-end gap-[1px]" role="img"></div>`),fe=d(`<span class="text-xs text-gray-400">—</span>`),pe=d(`<tr class="border-b border-gray-100 last:border-0 dark:border-gray-700"><td class="px-4 py-3 font-medium text-gray-900 dark:text-gray-100"><a class="text-blue-600 hover:underline dark:text-blue-400"> </a> <!> <!></td><td class="px-4 py-3 text-xs text-gray-500 uppercase dark:text-gray-400"> </td><td class="px-4 py-3 text-gray-600 dark:text-gray-400"> </td><td class="max-w-xs truncate px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-300"> </td><td class="px-4 py-3"><span> </span> <span class="ml-1 text-gray-500 dark:text-gray-400"> </span> <!></td><td class="px-4 py-3 text-gray-600 tabular-nums dark:text-gray-300"><!></td><td class="px-4 py-3"><!></td><td class="px-4 py-3"><!></td></tr>`),me=d(`<tr><td colspan="8" class="px-4 py-8 text-center text-gray-400 dark:text-gray-600">No agent endpoints configured. Add providers to .ddx/config.yaml or install a
								harness binary.</td></tr>`),I=d(`<div class="overflow-hidden rounded-lg border border-gray-200 dark:border-gray-700"><table class="w-full text-sm" data-testid="agent-endpoints-table"><thead><tr class="border-b border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800"><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Name</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Kind</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Type</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Model</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Status</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Tokens (1h / 24h)</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Utilization</th><th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-gray-300">Trend (24h)</th></tr></thead><tbody><!><!></tbody></table></div>`),L=d(`<div class="space-y-6" data-testid="agent-endpoints"><div class="flex items-center justify-between"><h1 class="text-xl font-semibold dark:text-white">Agent endpoints</h1> <!></div> <!> <!></div>`);function R(d,R){c(R,!0);let z=()=>u(k,`$page`,B),[B,V]=w(),H=D`
		query ProviderStatuses {
			providerStatuses {
				name
				kind
				providerType
				baseURL
				model
				status
				reachable
				detail
				modelCount
				isDefault
				cooldownUntil
				lastCheckedAt
				defaultForProfile
				usage {
					tokensUsedLastHour
					tokensUsedLast24h
					requestsLastHour
					requestsLast24h
				}
				quota {
					ceilingTokens
					ceilingWindowSeconds
					remaining
					resetAt
				}
				sparkline
			}
			harnessStatuses {
				name
				kind
				providerType
				baseURL
				model
				status
				reachable
				detail
				modelCount
				isDefault
				cooldownUntil
				lastCheckedAt
				defaultForProfile
				usage {
					tokensUsedLastHour
					tokensUsedLast24h
					requestsLastHour
					requestsLast24h
				}
				quota {
					ceilingTokens
					ceilingWindowSeconds
					remaining
					resetAt
				}
				sparkline
			}
		}
	`,U=D`
		query DefaultRouteStatus {
			defaultRouteStatus {
				modelRef
				resolvedProvider
				resolvedModel
				strategy
			}
		}
	`,W=C(y([])),G=C(null),K=C(!0),q=C(null),J=C(null);te(()=>{!f(K)&&f(J)===null&&_(J,Date.now(),!0)});let Y=null;async function he(e){let t=await e.request(H);_(W,[...t.providerStatuses??[],...t.harnessStatuses??[]],!0)}x(()=>{let e=O();return he(e).catch(e=>{_(q,e instanceof Error?e.message:String(e),!0)}).finally(()=>{_(K,!1)}),e.request(U).then(e=>{_(G,e.defaultRouteStatus??null,!0)}).catch(()=>{}),Y=setInterval(()=>{he(e).catch(()=>{})},2500),()=>{Y!=null&&(clearInterval(Y),Y=null)}});function ge(e){if(e.reachable)return`text-green-600 dark:text-green-400`;let t=e.status.toLowerCase();return t.includes(`connected`)||t===`available`||t.includes(`api key configured`)?`text-green-600 dark:text-green-400`:t.includes(`cooldown`)||t.includes(`unreachable`)||t.includes(`error`)||t===`unavailable`||t.startsWith(`unavailable`)?`text-red-600 dark:text-red-400`:`text-yellow-600 dark:text-yellow-400`}function X(e){return e==null?`—`:e<1e3?`${e}`:e<1e6?`${(e/1e3).toFixed(1)}k`:`${(e/1e6).toFixed(2)}M`}function Z(e,t){if(!e||!t||t.ceilingTokens==null||t.ceilingTokens<=0)return null;let n=(t.ceilingWindowSeconds??60)<=3600?e.tokensUsedLastHour:e.tokensUsedLast24h;return n==null?null:Math.min(100,Math.round(n*100/t.ceilingTokens))}function _e(e){let t=0;for(let n of e)n>t&&(t=n);return t===0?1:t}function ve(e,t){let n=Math.round(e*100/t);return`${Math.max(2,n)}%`}function ye(e){return`/nodes/${z().params.nodeId}/providers/${encodeURIComponent(e.name)}`}var Q=L();n(`1lrl555`,e=>{S(()=>{v.title=`Agent endpoints · DDx`})});var $=m(Q),be=b(m($),2),xe=e=>{var t=A(),n=m(t);i(t),p((e,t)=>a(n,`${f(W).length??``} total (${e??``} endpoints · ${t??``} harnesses)`),[()=>f(W).filter(e=>e.kind===`ENDPOINT`).length,()=>f(W).filter(e=>e.kind===`HARNESS`).length]),E(e,t)};r(be,e=>{f(K)||e(xe)}),i($);var Se=b($,2),Ce=e=>{var t=ne(),n=b(m(t),2),o=m(n),s=b(m(o)),c=m(s,!0);i(s),i(o);var l=b(o,2),u=e=>{var t=j(),n=b(m(t)),r=m(n,!0);i(n),i(t),p(()=>a(r,f(G).strategy)),E(e,t)};r(l,e=>{f(G).strategy&&e(u)});var d=b(l,2),ee=e=>{var t=N(),n=b(m(t)),o=m(n,!0);i(n);var s=b(n,2),c=e=>{var t=M(),n=b(h(t)),r=m(n,!0);i(n),p(()=>a(r,f(G).resolvedModel)),E(e,t)};r(s,e=>{f(G).resolvedModel&&e(c)}),i(t),p(()=>a(o,f(G).resolvedProvider)),E(e,t)},g=e=>{E(e,P())};r(d,e=>{f(G).resolvedProvider?e(ee):e(g,-1)}),i(n),i(t),p(()=>a(c,f(G).modelRef)),E(e,t)};r(Se,e=>{f(G)&&f(G).modelRef&&e(Ce)});var we=b(Se,2),Te=e=>{E(e,F())},Ee=e=>{var t=re(),n=m(t);i(t),p(()=>a(n,`Error: ${f(q)??``}`)),E(e,t)},De=t=>{var n=I(),c=m(n),u=b(m(c)),d=m(u);o(d,17,()=>f(W),e=>e.kind+`|`+e.name,(t,n)=>{var c=pe(),u=m(c),d=m(u),h=m(d,!0);i(d);var _=b(d,2),v=e=>{E(e,ie())};r(_,e=>{f(n).isDefault&&e(v)});var y=b(_,2),te=e=>{var t=ae();p(()=>g(t,`title`,`Cooldown until ${f(n).cooldownUntil??``}`)),E(e,t)};r(y,e=>{f(n).cooldownUntil&&e(te)}),i(u);var x=b(u),S=m(x,!0);i(x);var C=b(x),w=m(C,!0);i(C);var D=b(C),O=m(D,!0);i(D);var k=b(D),A=m(k),j=m(A,!0);i(A);var M=b(A,2),N=m(M,!0);i(M);var P=b(M,2),ne=e=>{var t=oe();p(()=>g(t,`title`,`Last checked ${f(n).lastCheckedAt??``}`)),E(e,t)};r(P,e=>{f(n).lastCheckedAt&&e(ne)}),i(k);var F=b(k),re=m(F),me=e=>{var t=ee();p((e,n)=>a(t,`${e??``} / ${n??``}`),[()=>X(f(n).usage.tokensUsedLastHour),()=>X(f(n).usage.tokensUsedLast24h)]),E(e,t)},I=e=>{E(e,se())};r(re,e=>{f(n).usage?e(me):e(I,-1)}),i(F);var L=b(F),R=m(L),z=e=>{var t=ce(),r=m(t),o=m(r);i(r);var s=b(r,2),c=m(s);i(s),i(t),p((e,t)=>{l(o,`width: ${e??``}%`),a(c,`${t??``}%`)},[()=>Z(f(n).usage,f(n).quota),()=>Z(f(n).usage,f(n).quota)]),E(e,t)},B=T(()=>Z(f(n).usage,f(n).quota)!=null),V=e=>{E(e,le())};r(R,e=>{f(B)?e(z):e(V,-1)}),i(L);var H=b(L),U=m(H),W=e=>{let t=T(()=>_e(f(n).sparkline));var r=de();o(r,21,()=>f(n).sparkline,s,(e,n)=>{var r=ue();p(e=>{l(r,`height: ${e??``}`),g(r,`title`,`${f(n)??``} tokens`)},[()=>ve(f(n),f(t))]),E(e,r)}),i(r),p(()=>{g(r,`aria-label`,`24-hour token trend for ${f(n).name??``}`),g(r,`data-testid`,`endpoint-sparkline-bars-${f(n).name??``}`)}),E(e,r)},G=e=>{E(e,fe())};r(U,e=>{f(n).sparkline&&f(n).sparkline.length>=6?e(W):e(G,-1)}),i(H),i(c),p((t,r)=>{g(c,`data-testid`,`endpoint-row-${f(n).name??``}`),g(d,`href`,t),g(d,`data-testid`,`endpoint-link-${f(n).name??``}`),a(h,f(n).name),g(x,`data-testid`,`endpoint-kind-${f(n).name??``}`),a(S,f(n).kind===`ENDPOINT`?`endpoint`:`harness`),a(w,f(n).providerType),g(D,`title`,f(n).model),a(O,f(n).model||`—`),e(A,1,`font-medium ${r??``}`),g(A,`data-testid`,`endpoint-reachable-${f(n).name??``}`),a(j,f(n).reachable?`reachable`:`not reachable`),g(M,`title`,f(n).detail),a(N,f(n).status),g(F,`data-testid`,`endpoint-tokens-${f(n).name??``}`),g(H,`data-testid`,`endpoint-sparkline-${f(n).name??``}`)},[()=>ye(f(n)),()=>ge(f(n))]),E(t,c)});var h=b(d),_=e=>{E(e,me())};r(h,e=>{f(W).length===0&&e(_)}),i(u),i(c),i(n),E(t,n)};r(we,e=>{f(K)?e(Te):f(q)?e(Ee,1):e(De,-1)}),i(Q),E(d,Q),t(),V()}export{R as component};
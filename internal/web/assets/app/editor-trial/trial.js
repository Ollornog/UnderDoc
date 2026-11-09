var m=`# Network plan

The **router** sits in the *rack* and runs \`nftables\`. See [[Wiki links]], [the part](/o/7k2m9q4x1c#h-part) and [the vendor](https://example.com/docs).

## Devices

- Router
- Switch with **PoE**
  - Patch panel

1. Unpack
2. Mount
3. Connect

- [ ] Order cables
- [x] Label ports

| Name | Value |
| --- | ---: |
| Height | 50 m |
| Ports | 24 |

\`\`\`text
router 192.0.2.1
\`\`\`

> A quote with *emphasis*.

::embed{ref="7k2m9q4x1c#b2"}

---

![alt](/o/x.png)
`;var g=[],h=[],x={},b=e=>(String(e||"").match(/editor-trial\/(\w+)(?:\.js|-chunks\/)/)||[])[1]||"page";document.addEventListener("securitypolicyviolation",e=>{g.push({bundle:b(e.sourceFile),directive:e.effectiveDirective||e.violatedDirective,blocked:e.blockedURI,source:e.sourceFile?`${e.sourceFile.replace(/^.*\/editor-trial\//,"").replace(/\?.*$/,"")}:${e.lineNumber}:${e.columnNumber}`:"",sample:e.sample||""})});window.addEventListener("error",e=>{h.push({bundle:b(e.filename),message:String(e.message)})});window.addEventListener("unhandledrejection",e=>{h.push({bundle:"promise",message:String(e.reason&&(e.reason.stack||e.reason.message)||e.reason)})});function f(e,o){let t=e.replace(/\n+$/,"").split(`
`),a=o.replace(/\n+$/,"").split(`
`),s=t.length,r=a.length,n=Array.from({length:s+1},()=>new Uint16Array(r+1));for(let p=s-1;p>=0;p--)for(let i=r-1;i>=0;i--)n[p][i]=t[p]===a[i]?n[p+1][i+1]+1:Math.max(n[p+1][i],n[p][i+1]);let c=[],d=0,l=0;for(;d<s||l<r;)d<s&&l<r&&t[d]===a[l]?(d++,l++):l<r&&(d===s||n[d][l+1]>=n[d+1][l])?c.push(`+ ${a[l++]}`):c.push(`- ${t[d++]}`);return c}function w(e){let o=f(m,e);return{identical:o.length===0,diff:o,markdown:e}}function u(e,o,t){let a=document.createElement(e);return o&&(a.className=o),t!==void 0&&(a.textContent=t),a}function $(e,o){if(e.replaceChildren(),o.length===0){e.append(u("span","et-same","\u2705 identical to the fixture"));return}e.append(u("span","et-changed",`\u274C ${o.length} differing lines`));for(let t of o)e.append(u("span",t.startsWith("+")?"et-add":"et-del",t))}async function y(e,o){let t=document.querySelector(`[data-editor="${e}"]`);if(!t)return;let a=t.querySelector("[data-host]"),s=t.querySelector("[data-out-text]"),r=t.querySelector("[data-diff]");t.dataset.state="mounting";try{let n=await o(a,m);await new Promise(c=>requestAnimationFrame(()=>requestAnimationFrame(c))),n.initial=w(n.getMarkdown()),x[e]=n,t.dataset.state="ready",t.querySelector("[data-out]").addEventListener("click",()=>{let c=n.getMarkdown();s.textContent=c,s.hidden=!1,$(r,f(m,c)),r.hidden=!1})}catch(n){t.dataset.state="failed",a.append(u("p","et-failed",`\u274C Mounting failed: ${n&&n.message}`)),h.push({bundle:e,message:`mount: ${n&&(n.stack||n.message)}`})}}var k=[],v=document.readyState!=="loading";document.addEventListener("DOMContentLoaded",()=>{v=!0;for(let[e,o]of k.splice(0))y(e,o);S()});async function S(){let e=document.querySelector('script[src*="editor-trial/trial.js"]'),o=e?new URL(e.src).searchParams.get("v"):"",t;try{t=await(await fetch(`/assets/app/editor-trial/meta.json?v=${o}`)).json()}catch{return}for(let a of document.querySelectorAll("[data-editor]")){let s=a.dataset.editor,r=t.bundles[s];if(!r)continue;let n=i=>`${(i/1024).toFixed(0)} KiB`,c=a.dataset.main.split(" "),d=r.packages.filter(i=>c.includes(i.name)),l=[...new Set(r.packages.map(i=>i.license))].join(", "),p=r.lazyFiles?` \xB7 on demand ${r.lazyFiles} chunks, gzip ${n(r.lazyGzip)}`:"";a.querySelector("[data-note]").replaceChildren(u("span","",d.map(i=>`${i.name} ${i.version}`).join(" \xB7 ")),u("span","",`Licences of all ${r.packages.length} bundled packages: ${l}`),u("span","",`JS gzip ${n(r.jsGzip)} (${n(r.js)}) \xB7 CSS gzip ${n(r.cssGzip)}${p}`))}}window.editorTrial={fixture:m,editors:x,violations:g,errors:h,lineDiff:f,roundTrip:w,register(e,o){v?y(e,o):k.push([e,o])}};export{f as lineDiff};

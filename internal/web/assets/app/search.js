// SPDX-License-Identifier: MIT
// The search of the workspace (ADR-24, M-12): the palette on Ctrl+K with hits while typing, the results page with its
// filters, and the jump to the section a hit names, with the matching words marked there. It reads /api/search and
// uses the state and helpers of demo.js (S, D, SID, render, openDoc, esc, icon, hitItem …), which loads first. What it
// draws is C22's: the palette a .command, hits .item entries, the filters small fields and buttons.

const udSearch=(()=>{
  const real=()=>!document.getElementById('root').hasAttribute('data-demo');
  const TYPES={page:'Page',folder:'Folder',collection:'Collection',notes:'Quick notes',table:'Table',list:'List',file:'File',database:'Database'};
  /* What the results page asks: its query, strategy and filters, and what came back. */
  const F={type:'',collection:'',author:'',from:'',to:'',tags:[]};
  let result=null,asked='',timer=null,seq=0;
  /* The palette: its query, what the server found for it, and which entry is chosen. */
  let pal=null;

  const url=(q,limit,filters)=>{const p=new URLSearchParams({q,limit:String(limit)});
    if(filters){['type','collection','author','from','to'].forEach(k=>{if(filters[k])p.set(k,filters[k]);});filters.tags.forEach(t=>p.append('tag',t));}
    if(S.strat==='exact')p.set('exact','1');return '/api/search?'+p;};
  async function ask(q,limit,filters){
    const r=await fetch(url(q,limit,filters),{credentials:'same-origin',headers:{Accept:'application/json'}});
    if(!r.ok)throw new Error(String(r.status));return r.json();
  }
  const filtered=()=>F.type||F.collection||F.author||F.from||F.to||F.tags.length;
  const key=()=>JSON.stringify([S.sq.trim(),S.strat,F]);

  /* The results page runs its query a moment after the last key, and only the newest answer counts. */
  function results(){
    clearTimeout(timer);const q=S.sq.trim(),k=key();
    if(!q&&!filtered()){result=null;asked=k;refreshHits();return;}
    if(k===asked&&result)return;
    const n=++seq;timer=setTimeout(async()=>{
      try{const a=await ask(q,50,F);if(n!==seq)return;result=a;asked=k;}catch(e){if(n!==seq)return;result={engine:'',notice:'The search did not answer.',hits:[],facets:{}};asked=k;}
      refreshHits();},150);
  }

  const keyOfHit=h=>SID[h.id];
  const meta=h=>[h.place,h.heading].filter(Boolean).map(esc).join(' › ')+(h.changed?` · ${esc(h.changed)}`:'')+(h.by?` · ${esc(h.by)}`:'');
  /* A hit opens its object at its section. The snippet comes from the server with every piece escaped and the matches in
     mark, so it goes in as it is. */
  const hitAttrs=(h,act)=>`data-sact="${act}"${keyOfHit(h)?` data-id="${keyOfHit(h)}"`:''} data-sid="${esc(h.id)}" data-anchor="${esc(h.anchor||'')}" data-heading="${esc(h.heading||'')}" title="${esc(h.n)}"`;
  const hitIcon=h=>TYPE[h.t]?TYPE[h.t][0]:'page';
  const hitHtml=(h,act)=>hitItem(hitAttrs(h,act),hitIcon(h),esc(h.n),meta(h),h.snippet||'');
  /* In the palette a hit is an entry of the command menu: the title, its place and the excerpt below it. */
  const palHit=h=>`<button type="button" role="menuitem" ${hitAttrs(h,'go')}>${icon(hitIcon(h))}<div><span>${esc(h.n)}</span><small>${meta(h)}</small>${h.snippet?`<small data-snip>${h.snippet}</small>`:''}</div></button>`;

  function resultsHtml(){
    const q=S.sq.trim();
    if(!q&&!filtered())return hint('Type to search titles, headings and text. Error codes are found exactly as written. Ctrl+K opens the quick search anywhere.');
    if(!result||asked!==key())return hint('Searching…');
    const said=result.notice?note(esc(result.notice),' role="status"'):'';
    const strat=S.strat==='meaning'?hint('Similar meaning comes with AI (M-13). Until then this is the full text.'):'';
    const n=result.hits.length;
    return `${said}${strat}${colLabel(`${n} ${n===1?'hit':'hits'}${n===50?' (the first 50)':''}`)}${n?hitList(result.hits.map(h=>hitHtml(h,'go')).join('')):hint('Nothing found. Fewer words, or another filter, may help.')}`;
  }

  /* The filters of the results page, as in Confluence: type, collection, author, the time of the last change, and tags
     that all must be there. Their choices come from what the search found. */
  function filtersHtml(){
    const fc=(result&&result.facets)||{};
    const opts=(name,list,all,label)=>{const have=list.some(x=>x.value===F[name]);
      return `<select class="select" data-size="sm" data-sf="${name}" aria-label="${label}"><option value="">${all}</option>${list.map(x=>`<option value="${esc(x.value)}"${F[name]===x.value?' selected':''}>${esc(x.label)}</option>`).join('')}${F[name]&&!have?`<option value="${esc(F[name])}" selected>${esc(F[name])}</option>`:''}</select>`;};
    const types=(fc.type||[]).map(x=>({value:x.value,label:TYPES[x.value]||x.label}));
    const tags=[...new Set([...F.tags,...(fc.tags||[]).map(x=>x.value)])];
    const day=(k,name)=>prow(name,`<input class="input w-40" data-size="sm" type="date" data-sf="${k}" value="${esc(F[k])}" aria-label="${name}">`);
    return `<div class="flex flex-col gap-2" data-filters>${opts('type',types,'All types','Type')}${opts('collection',fc.collection||[],'All collections','Collection')}${opts('author',fc.author||[],'Everyone','Author')}
      ${day('from','Changed from')}${day('to','Changed until')}
      ${tags.length?`<div class="flex flex-wrap gap-1">${tags.map(t=>tagToggle(`data-sact="tag" data-tag="${esc(t)}"`,esc(t),F.tags.includes(t))).join('')}</div>`:''}</div>`;
  }
  function pageHtml(){
    if(asked!==key())results();
    const n=['type','collection','author','from','to'].filter(k=>F[k]).length+F.tags.length;
    return `<div class="flex flex-col gap-2"><div>${pbtn(`data-act="filters" aria-expanded="${S.filters}"`,'Filter'+(n?' ('+n+')':''),'view')}</div>
      ${S.filters?filtersHtml():''}<div id="sres" class="flex flex-col gap-2">${resultsHtml()}</div></div>`;
  }

  /* --- The palette (Ctrl+K): pages and folders by their title at once, the full text as the server answers. */
  function titleMatches(q){
    const w=q.toLowerCase();if(!w)return [];
    const ids=Object.keys(D).filter(id=>{const d=D[id];return d.real&&(d.t==='page'||d.t==='folder'||d.t==='coll')&&!d.gone&&String(d.n).toLowerCase().includes(w);});
    ids.sort((a,b)=>{const x=String(D[a].n).toLowerCase(),y=String(D[b].n).toLowerCase();return (y===w)-(x===w)||(y.startsWith(w))-(x.startsWith(w))||x.localeCompare(y);});
    return ids.slice(0,6);
  }
  function palMenu(){
    if(!pal)return '';
    const q=pal.q.trim(),titles=titleMatches(q);
    const go=titles.map(id=>`<button type="button" role="menuitem" data-sact="open" data-key="${id}" data-filter="${D[id].n}">${icon(D[id].t==='coll'?'grid':TYPE[D[id].t]?TYPE[D[id].t][0]:'page')}<div><span>${D[id].n}</span><small>${placeOf(id)}</small></div></button>`).join('');
    const found=(pal.res&&pal.res.q===q?pal.res.hits:[]).filter(h=>!titles.includes(keyOfHit(h))||h.anchor).slice(0,8);
    const text=found.map(palHit).join('');
    const said=pal.res&&pal.res.q===q&&pal.res.notice?`<p class="text-muted-foreground px-2 py-1 text-xs">${esc(pal.res.notice)}</p>`:'';
    const all=q?`<button type="button" role="menuitem" data-sact="all">${icon('search')}All results for “${esc(q)}”</button>`:'';
    const groups=[go?`<div role="group" aria-label="Pages and folders"><span role="heading">Go to</span>${go}</div>`:'',
      text?`<div role="group" aria-label="Found in the text"><span role="heading">Found</span>${text}</div>`:'',said,all].filter(Boolean);
    return groups.join('<hr role="separator">')||`<p class="text-muted-foreground px-2 py-1 text-xs">Type a title, words or an error code.</p>`;
  }
  function palHtml(){
    if(!pal)return '';
    /* C22's command menu, fixed below the search field; udpal is the handle the script finds it by. */
    return `<div class="command udpal fixed z-50 w-96 border shadow-md" data-style="left:${pal.x}px;top:${pal.y}px" aria-label="Quick search"><header>${icon('search')}<input data-palq value="${esc(pal.q)}" placeholder="Search or jump to…" aria-label="Quick search" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true" aria-controls="udpalmenu"></header><div role="menu" class="scrollbar-sm" id="udpalmenu">${palMenu()}</div></div>`;
  }
  function openPalette(){
    const f=document.getElementById('search'),z=S.scale/100,r=f?f.getBoundingClientRect():null;
    pal={q:'',res:null,k:0,x:r?Math.round(r.left/z):16,y:r?Math.round(r.bottom/z)+4:16};
    S.newAt=null;S.ctx=null;S.addIn=null;render();
    const i=document.querySelector('[data-palq]');if(i)i.focus();
  }
  function closePalette(){if(!pal)return false;pal=null;render();return true;}
  /* The chosen entry is kept by its place, so it stays chosen when the workspace draws itself anew. */
  function choose(m){const items=[...m.querySelectorAll('[role="menuitem"]')];if(!items.length)return;pal.k=Math.min(pal.k,items.length-1);items[pal.k].classList.add('active');}
  function refreshPalette(){const m=document.getElementById('udpalmenu');if(m){m.innerHTML=palMenu();choose(m);}}
  let palTimer=null,palSeq=0;
  function palType(q){
    pal.q=q;pal.k=0;refreshPalette();clearTimeout(palTimer);const t=q.trim();if(!t)return;const n=++palSeq;
    palTimer=setTimeout(async()=>{try{const a=await ask(t,8,null);if(!pal||n!==palSeq)return;pal.res={q:t,hits:a.hits,notice:a.notice};refreshPalette();}catch(e){}},120);
  }
  function palStep(dir){
    const items=[...document.querySelectorAll('.udpal [role="menuitem"]')];if(!items.length)return;
    const k=items.findIndex(n=>n.classList.contains('active'));items.forEach(n=>n.classList.remove('active'));
    pal.k=(k<0?(dir>0?0:items.length-1):k+dir+items.length)%items.length;const n=items[pal.k];n.classList.add('active');n.scrollIntoView({block:'nearest'});
  }

  /* --- Opening a hit at its section. The words that matched are marked in that section until another page opens. */
  /* The words to mark: what the server marked in the snippet, which includes words found despite a typing error, and the
     words of the query, which a match in a heading does not show in the snippet of the text below it. */
  function terms(html,q){const box=document.createElement('div');box.innerHTML=html||'';
    return [...new Set([...[...box.querySelectorAll('mark')].map(m=>m.textContent.trim()),...String(q||'').split(/\s+/)].filter(t=>t.length>1))];}
  function openHit(sid,anchor,heading,snippet,q){
    const k=SID[sid];
    if(!k||!D[k]){location.href='/o/'+sid.split('/').map(encodeURIComponent).join('/')+(anchor?'#'+encodeURIComponent(anchor):'');return;}
    pal=null;openDoc(k);const w=S.wins[S.focus]||S.wins[0],t=w.tabs[w.a];
    if(t)t.hl={d:k,anchor,heading,terms:terms(snippet,q),scroll:true};
    render();
  }
  function target(root,hl){
    if(!hl.anchor&&!hl.heading)return null;
    const byBlock=hl.anchor&&[...root.querySelectorAll('[data-block]:not(.cm-ud-marker)')].find(n=>n.dataset.block===hl.anchor);if(byBlock)return byBlock;
    const byID=hl.anchor&&[...root.querySelectorAll('[id]')].find(n=>n.id===hl.anchor);if(byID)return byID;
    return hl.heading?[...root.querySelectorAll('h1,h2,h3,h4,h5,h6')].find(n=>n.textContent.trim()===hl.heading)||null:null;
  }
  /* The section of a hit: a heading, or the block of a heading, and what follows up to the next heading (ADR-22); any
     other block alone. */
  const isHead=n=>/^H[1-6]$/.test(n.tagName)||!!n.querySelector(':scope>h1,:scope>h2,:scope>h3,:scope>h4,:scope>h5,:scope>h6');
  function section(el){
    if(!isHead(el))return [el];
    const out=[el];for(let n=el.nextElementSibling;n&&!isHead(n);n=n.nextElementSibling)out.push(n);return out;
  }
  function mark(nodes,words){
    if(!words.length)return;
    /* The longest first, so a word is marked whole and not by a shorter one it starts with. */
    const re=new RegExp([...words].sort((a,b)=>b.length-a.length).map(w=>w.replace(/[.*+?^${}()|[\]\\]/g,'\\$&')).join('|'),'gi');
    nodes.forEach(root=>{const tw=document.createTreeWalker(root,NodeFilter.SHOW_TEXT),texts=[];while(tw.nextNode())if(!tw.currentNode.parentElement.closest('mark,button'))texts.push(tw.currentNode);
      texts.forEach(n=>{const s=n.nodeValue;re.lastIndex=0;if(!re.test(s))return;re.lastIndex=0;const f=document.createDocumentFragment();let last=0,m;
        while((m=re.exec(s))){f.append(s.slice(last,m.index));const k=document.createElement('mark');k.className='hitmark';k.textContent=m[0];f.append(k);last=m.index+m[0].length;}
        f.append(s.slice(last));n.replaceWith(f);});});
  }
  function after(){
    /* The palette keeps the focus in its field and its choice through every redraw of the workspace. */
    if(pal){const m=document.getElementById('udpalmenu');if(m)choose(m);const i=document.querySelector('[data-palq]');if(i&&document.activeElement!==i){i.focus();i.setSelectionRange(i.value.length,i.value.length);}}
    S.wins.forEach((w,i)=>{const t=at(i);if(!t||!t.hl)return;if(t.hl.d!==t.d){delete t.hl;return;}
      const root=document.querySelector(`[data-art="${i}"]`);if(!root)return;const el=target(root,t.hl);if(!el)return;
      const nodes=section(el);mark(nodes,t.hl.terms);
      if(t.hl.scroll){t.hl.scroll=false;el.scrollIntoView({block:'start'});const sc=document.querySelector(`[data-sc="${i}"]`);if(sc)t.top=sc.scrollTop;}});
  }

  /* --- Events. Ctrl+K (Cmd+K on a Mac) opens the palette anywhere but in the editor, where it makes a link (M-8). */
  window.addEventListener('keydown',e=>{
    if((e.ctrlKey||e.metaKey)&&!e.altKey&&!e.shiftKey&&e.key.toLowerCase()==='k'&&!(e.target.closest&&e.target.closest('.cm-editor'))){e.preventDefault();e.stopPropagation();if(pal)closePalette();else openPalette();return;}
    const x=e.target;if(!pal||!x.dataset||x.dataset.palq===undefined)return;
    if(e.key==='Escape'){e.preventDefault();e.stopPropagation();closePalette();return;}
    if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();e.stopPropagation();palStep(e.key==='ArrowDown'?1:-1);return;}
    if(e.key==='Enter'){e.preventDefault();e.stopPropagation();const n=document.querySelector('.udpal [role="menuitem"].active')||document.querySelector('.udpal [role="menuitem"]');if(n)n.click();}
  },true);
  document.addEventListener('input',e=>{const x=e.target;if(x.dataset&&x.dataset.palq!==undefined){e.stopPropagation();palType(x.value);}},true);
  document.addEventListener('change',e=>{const x=e.target;if(!x.dataset||x.dataset.sf===undefined)return;e.stopPropagation();F[x.dataset.sf]=x.value;results();render();},true);
  document.addEventListener('click',e=>{
    const a=e.target.closest&&e.target.closest('[data-sact]');
    if(pal&&!(e.target.closest&&e.target.closest('.udpal'))&&!a){pal=null;render();}
    if(!a)return;e.preventDefault();e.stopImmediatePropagation();
    switch(a.dataset.sact){
      case 'go':openHit(a.dataset.sid,a.dataset.anchor,a.dataset.heading,(a.querySelector('[data-snip]')||{}).innerHTML,pal?pal.q:S.sq);return;
      case 'open':pal=null;openDoc(a.dataset.key);render();return;
      case 'all':{const q=pal?pal.q.trim():'';pal=null;if(S.view!=='search')S.prev=S.view;S.view='search';S.rail=true;S.sq=q;results();render();const f=document.getElementById('search');if(f){f.focus();f.setSelectionRange(f.value.length,f.value.length);}return;}
      case 'tag':{const t=a.dataset.tag;F.tags=F.tags.includes(t)?F.tags.filter(x=>x!==t):[...F.tags,t];results();render();return;}
    }
  },true);

  return {real,results,resultsHtml,pageHtml,palHtml,after,open:openPalette,
    /* The first hit, for Enter in the search field. */
    first:()=>result&&asked===key()&&result.hits[0]?result.hits[0]:null,openHit,
    /* A tag chip of a page searches for its tag. */
    tag:t=>{F.tags=[t];S.sq='';results();}};
})();

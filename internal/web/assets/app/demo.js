// SPDX-License-Identifier: MIT
// The workspace exactly as in the approved mockup (ADR-16), with real data where UnderDoc has a backend. Sprint 2026-11-08.

const I = {
  page:'<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 12h6M9 16h6"/>',
  folder:'<path d="M3 7h6l2 2h10v10H3z"/>',
  table:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M3 10h18M3 15h18M9 5v14M15 5v14"/>',
  excel:'<path d="M6 3h8l4 4v14H6z"/><path d="M14 3v4h4M9 11l6 6M15 11l-6 6"/>',
  images:'<rect x="3" y="5" width="18" height="14" rx="1"/><circle cx="8.5" cy="10" r="1.5"/><path d="m21 16-5-5-8 8"/>',
  view:'<path d="M4 5h16l-6 7v6l-4 2v-8z"/>',
  db:'<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>',
  chev:'<path d="m9 6 6 6-6 6"/>', panel:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M9 5v14"/>',
  list:'<path d="M9 7h11M9 12h11M9 17h11M4 7h1M4 12h1M4 17h1"/>', info:'<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  box:'<path d="M3 7h18v13H3zM3 7l3-3h12l3 3"/><path d="M12 11v5"/>', hd:'<rect x="3" y="6" width="18" height="12" rx="1"/><path d="M7 14h.01M11 14h6"/>'
};
/* The workspace is made of C22's building blocks (T-12): its icons are inline SVG marked with data-icon-lu and take their
   size from the block they sit in (a button, a menu entry, the figure of an entry). */
const LU={page:'file-text',folder:'folder',table:'table',chev:'chevron-right',plus:'plus',x:'x',list:'list',search:'search',code:'code',
  split:'columns-2',chat:'message-square',gear:'settings',more:'ellipsis',lock:'lock',pencil:'pencil',book:'book-open',down:'chevron-down',
  up:'chevron-up',win:'app-window',panel:'panel-left',hd:'hard-drive',db:'database',shield:'shield-check',globe:'globe',mail:'mail',
  user:'user',ai:'sparkles',quote:'quote',grid:'layout-grid',check:'square-check',cal:'calendar',clock:'clock',sun:'sun',note:'sticky-note',
  pin:'pin',star:'star',flag:'flag',pulse:'activity',box:'archive',view:'filter',images:'images',video:'video',slides:'presentation',
  grip:'grip-vertical',link:'link',trash:'trash-2',send:'send',alleft:'text-align-start',alcenter:'text-align-center',alright:'text-align-end',
  text:'type',heading:'heading',olist:'list-ordered',hr:'minus',opennew:'external-link',excel:'file-spreadsheet',info:'info',bold:'bold',
  italic:'italic',strike:'strikethrough',back:'chevron-left',layout:'layout-panel-left',board:'kanban',net:'network',spark:'sparkle',eye:'eye',
  gap:'chevrons-up-down',copy:'copy',sort:'arrow-down-a-z',rowup:'between-horizontal-start',rowdown:'between-horizontal-end',
  colleft:'between-vertical-start',colright:'between-vertical-end',rowdel:'table-rows-split',coldel:'table-columns-split'};
const icon = n => `<svg data-icon-lu="${LU[n]||n}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${I[n]}</svg>`;
/* An icon outside a block that sizes it (in running text, a label, a badge) takes C22's size utility. */
const ic = n => icon(n).replace('<svg ','<svg class="size-4" ');
/* Small parts of the panels and drawers, from C22: a quiet hint, a quiet line in a menu, a button of a panel. */
const hint=(t,attrs='')=>`<p class="text-muted-foreground text-xs"${attrs}>${t}</p>`;
const pbtn=(attrs,label,ico)=>`<button type="button" class="btn" data-variant="outline" data-size="xs" ${attrs}>${ico?icon(ico):''}${label}</button>`;
/* Settings as segments (C22's .button-group of small outline buttons, the chosen one pressed); a group that may not be
   changed is disabled as a whole. */
const segs=(act,opts,cur,extra='',label='')=>{const off=/\bdisabled\b/.test(extra);
  return `<div class="button-group" role="group"${label?` aria-label="${label}"`:''}${off?' aria-disabled="true"':''}>${opts.map(x=>{const [id,l]=Array.isArray(x)?x:[x,x];return `<button type="button" class="btn" data-variant="outline" data-size="xs" data-act="${act}" data-id="${id}" aria-pressed="${String(cur)===String(id)}" ${extra}>${l}</button>`;}).join('')}</div>`;};
/* A row of a panel: its name, then its control at the end. */
const prow=(name,ctrl,attrs='')=>`<div class="flex flex-wrap items-center justify-between gap-3"${attrs}><span class="text-sm">${name}</span>${ctrl}</div>`;
/* A panel of nav3 (profile, settings, administration, a source): C22's card with a heading. */
const panel=(inner,attrs='')=>`<aside class="card gap-3 px-4 py-4"${attrs}>${inner}</aside>`;
const ptitle=t=>`<h3 class="font-semibold">${t}</h3>`;
/* A hint in a page or a list: C22's alert. */
const note=(t,attrs='')=>`<div class="alert"${attrs}><section>${t}</section></div>`;
/* A table of made-up content: C22's .table in its container. */
const tableHtml=(h,rows)=>`<div class="table-container"><table class="table"><thead><tr>${h.map(x=>`<th>${x}</th>`).join('')}</tr></thead><tbody>${rows.map(r=>`<tr${r.dim?' data-style="opacity:.55"':''}>${r.map(x=>`<td>${x}</td>`).join('')}</tr>`).join('')}</tbody></table></div>`;
/* A to-do in a window: C22's framed entry with its checkbox; done is checked (C22 strikes it through), the due date at
   its end. The checkbox carries the app's action, read on change. */
const todoItem=(x,k,act,extra='')=>`<label class="item" data-variant="outline" data-size="sm" role="listitem"><input class="input" type="checkbox" data-act="${act}" data-id="${k}"${extra} aria-label="Done: ${esc(x.t)}"${x.done?' checked':''}><section><h4>${esc(x.t)}</h4></section>${x.due?`<aside><time>${x.due}</time></aside>`:''}</label>`;
const TYPE = {folder:['folder','Folder'],page:['page','Page'],table:['table','Table'],excel:['excel','Excel file'],images:['images','Image collection'],view:['view','Database view'],db:['db','Database']};
const LOGO = 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAxNzU4IDE3NTgiIHJvbGU9ImltZyIgYXJpYS1sYWJlbD0iVW5kZXJEb2MiPjx0aXRsZT5VbmRlckRvYzwvdGl0bGU+PHN0eWxlPnBhdGh7ZmlsbDojMDAwfUBtZWRpYSAocHJlZmVycy1jb2xvci1zY2hlbWU6ZGFyayl7cGF0aHtmaWxsOiNmZmZ9fTwvc3R5bGU+PHBhdGggZmlsbC1ydWxlPSJldmVub2RkIiBkPSJNNjg2IDQ5TDcwMSA1MFE3MTYgNTAgNzE5IDUxTDcxOSA1MVE3MjIgNTIgNzIzIDU0TDcyMyA1NFE3MjUgNTYgNzI2IDU4TDcyNiA1OFE3MjcgNjAgNzI3IDE1MEw3MjggMjk0TDczMCAyOTdMNzY2IDI5N0w3NjcgMjk4UTc2OCAyOTggNzc2IDI5OUw3NzYgMjk5UTc4NCAzMDAgNzkyIDMwMkw3OTIgMzAyUTgwMCAzMDUgODEzIDMxMkw4MTMgMzEyUTgyNiAzMjAgODQyIDMzNEw4NDIgMzM0UTg1NiAzNDcgODU5IDM1MUw4NTkgMzUxUTg2MiAzNTQgODYyIDM1OEw4NjIgMzYwTDgyNyAzOTZRNzkyIDQzMCA3ODkgNDM1TDc4OSA0MzVRNzg2IDQ0MCA3ODMgNDQ4TDc4MyA0NDhRNzgwIDQ1OCA3ODAgNDY1TDc3OSA0NzJMNzc4IDQ3NEw3NzggNDg2TDc3NyA0ODZMNzc3IDUwMkw3NzYgNTAyTDc3NiA1NDRMNzc3IDU0Nkw3NzcgNTUwUTc3NyA1NTQgNzgwIDU3Mkw3ODAgNTcyUTc4NCA1OTAgNzg4IDYwMEw3ODggNjAwUTc5MSA2MTAgNzk3IDYyM0w3OTcgNjIzUTgwMyA2MzYgODEyIDY0OEw4MTIgNjQ4UTgyMiA2NjIgODMzIDY3Mkw4MzMgNjcyUTg0NCA2ODMgODU4IDY5Mkw4NTggNjkyUTg3MiA3MDEgODg2IDcwOEw4ODYgNzA4UTkwMCA3MTQgOTE0IDcxOEw5MTQgNzE4UTkzMCA3MjMgOTQyIDcyNUw5NDIgNzI1UTk1NiA3MjcgOTU2IDcyOEw5NTYgNzI4TDk3NiA3MjlMOTc2IDczMEwxMDE4IDczMEwxMDIwIDcyOUwxMDMwIDcyOUwxMDMwIDcyOFExMDMwIDcyOCAxMDQwIDcyN0wxMDQwIDcyN1ExMDUwIDcyNiAxMDU2IDcyNEwxMDU2IDcyNFExMDYyIDcyMyAxMDYzIDcyM0wxMDY1IDcyNEwxMDY0IDcyNlExMDYyIDcyOCAxMDU2IDc0NEwxMDU2IDc0NFExMDUwIDc2MCAxMDQzIDc3NkwxMDQzIDc3NlExMDM2IDc5MiAxMDMyIDgwMkwxMDMyIDgwMlExMDI4IDgxNCAxMDI2IDgxNkwxMDI2IDgxNlExMDI1IDgxOCAxMDIzIDgyNEwxMDIzIDgyNFExMDIxIDgzMCAxMDA5IDg1OEwxMDA5IDg1OFE5OTcgODg2IDk5NiA4OTBMOTk2IDg5MFE5OTUgODkyIDk5NSA4OTZMOTk1IDg5NlE5OTUgOTAwIDk5NiA5MDJMOTk2IDkwMlE5OTcgOTA0IDk5OSA5MDdMOTk5IDkwN1ExMDAyIDkxMCAxMDQwIDkyNkwxMDQwIDkyNlExMDc4IDk0MSAxMDgwIDk0MkwxMDgwIDk0MlExMDgyIDk0NCAxMTA3IDk1NEwxMTA3IDk1NFExMTMyIDk2MyAxMTQwIDk2N0wxMTQwIDk2N1ExMTQ4IDk3MSAxMTY2IDk3OEwxMTY2IDk3OFExMTg0IDk4NCAxMTg2IDk4NEwxMTg2IDk4NFExMTkwIDk4NCAxMjc3IDk2MkwxMzMwIDk0OFExNDE4IDkyNiAxNDIxIDkyNEwxNDIxIDkyNFExNDI0IDkyMiAxNDMwIDkxNkwxNDMwIDkxNlExNDM1IDkxMCAxNDM4IDkwMEwxNDM4IDkwMFExNDQyIDg4OCAxNDQyIDg4MkwxNDQyIDg4MlExNDQzIDg3NiAxNDQ0IDg3MkwxNDQ0IDg3MlExNDQ1IDg2OCAxNDQ1IDg2MUwxNDQ1IDg2MVExNDQ1IDg1NCAxNDQwIDg0MEwxNDQwIDg0MFExNDM1IDgyNiAxNDI4IDgxMkwxNDI4IDgxMlExNDIwIDc5OCAxNDEzIDc5MEwxNDEzIDc5MFExNDA2IDc4MiAxMzgzIDc2MEwxMzgzIDc2MFExMzYwIDczOCAxMzU4IDczNEwxMzU4IDczNFExMzU2IDcyOCAxMzU2IDcwOUwxMzU2IDcwOVExMzU3IDY5MCAxMzU4IDY4OEwxMzU4IDY4OFExMzU4IDY4OCAxMzgxIDY3MUwxMzgxIDY3MVExNDA0IDY1NCAxNDA4IDY1MkwxNDA4IDY1MlExNDEwIDY1MSAxNDIyIDY1MEwxNDMyIDY1MEwxNDM0IDY0OUwxNDY2IDY0OFExNTAwIDY0OCAxNTE5IDY1NkwxNTE5IDY1NlExNTM4IDY2NCAxNTQyIDY2N0wxNTQyIDY2N1ExNTQ1IDY3MCAxNTQ2IDY3M0wxNTQ2IDY3M1ExNTQ4IDY3NiAxNTQ4IDcwMkwxNTQ4IDcwMlExNTQ4IDcyNiAxNTQ4IDcyOUwxNTQ4IDcyOVExNTQ3IDczMiAxNTM2IDc1NkwxNTM2IDc1NlExNTI0IDc4MCAxNTE5IDc5NUwxNTE5IDc5NVExNTE0IDgxMCAxNTE0IDgxMkwxNTE0IDgxMlExNTE0IDgxNCAxNTEyIDgyNEwxNTEyIDgyNFExNTA5IDgzNCAxNTA4IDgzOUwxNTA4IDgzOVExNTA4IDg0NCAxNTA3IDg0OEwxNTA3IDg0OFExNTA2IDg1MCAxNTA2IDg1M0wxNTA2IDg1M1ExNTA2IDg1NiAxNTA1IDg1OEwxNTA1IDg1OFExNTA0IDg2MiAxNTA0IDg2NEwxNTA0IDg2NFExNTA0IDg2NiAxNTAwIDg4NEwxNTAwIDg4NFExNDk3IDkwMiAxNDk0IDkyMUwxNDk0IDkyMVExNDkxIDk0MCAxNDg5IDk0NkwxNDg5IDk0NlExNDg3IDk1MiAxNDg1IDk1NkwxNDg1IDk1NlExNDgzIDk2MCAxNDc3IDk2NUwxNDc3IDk2NVExNDcwIDk3MSAxNDY0IDk3NEwxNDY0IDk3NFExNDU4IDk3NiAxNDQ1IDk3OEwxNDQ1IDk3OFExNDMyIDk4MSAxNDI2IDk4M0wxNDI2IDk4M1ExNDIwIDk4NSAxNDE4IDk4NUwxNDE4IDk4NVExNDE2IDk4NSAxMzkzIDk5MUwxMzkzIDk5MVExMzcwIDk5NyAxMzY4IDk5N0wxMzY4IDk5N1ExMzY2IDk5NyAxMzUyIDEwMDFMMTM1MiAxMDAxUTEzMzggMTAwNSAxMzM2IDEwMDVMMTMzNiAxMDA1UTEzMzQgMTAwNSAxMzE5IDEwMDlMMTMxOSAxMDA5UTEzMDQgMTAxMyAxMzAyIDEwMTNMMTMwMiAxMDEzUTEzMDAgMTAxMyAxMjgyIDEwMThMMTI4MiAxMDE4UTEyNjQgMTAyMyAxMjYyIDEwMjNMMTI2MiAxMDIzUTEyNjAgMTAyMyAxMjM4IDEwMjhMMTIzOCAxMDI4UTEyMTYgMTAzNCAxMjA5IDEwMzhMMTIwOSAxMDM4UTEyMDIgMTA0MSAxMTk3IDEwNDZMMTE5NyAxMDQ2UTExOTIgMTA1MiAxMTg1IDEwNjRMMTE4NSAxMDY0UTExNzggMTA3NiAxMTc2IDEwODJMMTE3NiAxMDgyUTExNzMgMTA4OCAxMTcyIDEwOTZMMTE3MiAxMDk2UTExNzAgMTEwMiAxMTcwIDExMTZMMTE2OSAxMTMwTDExNjggMTEzMkwxMTY5IDE0NjBMMTE1MyAxNDcyUTExMzYgMTQ4MyAxMTI0IDE0OTFMMTEyNCAxNDkxUTExMTAgMTQ5OSAxMDk5IDE1MDdMMTA5OSAxNTA3UTEwODggMTUxNSAxMDg0IDE1MTdMMTA4NCAxNTE3UTEwODAgMTUxOSAxMDUwIDE1MzlMMTA1MCAxNTM5UTEwMjIgMTU1OSAxMDAyIDE1NzFMMTAwMiAxNTcxUTk4MiAxNTgzIDk4MSAxNTg0TDk4MSAxNTg0UTk4MCAxNTg2IDkwNCAxNjM2TDc5NCAxNzA5TDc5MiAxNzA4UTc5MCAxNzA4IDc4MiAxNzAyTDc4MiAxNzAyUTc3NCAxNjk2IDczMCAxNjY1TDczMCAxNjY1UTY4NiAxNjM0IDY0NCAxNjAzTDY0NCAxNjAzUTYwMiAxNTcyIDU5NiAxNTY4TDU5NiAxNTY4UTU5MCAxNTY0IDU2OCAxNTQ4TDU2OCAxNTQ4UTU0NiAxNTMxIDUyOSAxNTIwTDUyOSAxNTIwUTUxMiAxNTA4IDUxMCAxNTA2TDUxMCAxNTA2UTUwOCAxNTA0IDQ3NCAxNDgwTDQ3NCAxNDgwUTQ0MCAxNDU2IDQyNiAxNDQ0TDQyNiAxNDQ0UTQxMCAxNDMzIDMzNyAxMzgxTDMwNSAxMzU3UTIzMiAxMzA1IDIyMSAxMjk2TDIxMCAxMjg4TDIxMSAxMjg1UTIxMiAxMjgyIDIyNiAxMjYyTDIyNiAxMjYyUTI0MCAxMjQyIDI1MSAxMjI4TDI1MSAxMjI4UTI2MiAxMjE0IDMxNCAxMTQwTDM2NiAxMDY5UTQxOCA5OTYgNDIwIDk5M0w0MjAgOTkzUTQyMSA5OTAgNDIyIDk4NEw0MjIgOTg0UTQyNCA5NzggNDI4IDk0OEw0MjggOTQ4UTQzMSA5MTggNDMyIDkxN0w0MzIgOTE2TDQzMiA5MTRRNDMyIDkxMCA0MzQgOTAzTDQzNCA5MDNRNDM1IDg5NiA0MzUgODkyTDQzNSA4ODhMNDM2IDg4OFE0MzYgODg4IDQzOCA4NzBMNDM4IDg3MFE0NDAgODUyIDQ0MiA4NDRMNDQyIDg0NFE0NDMgODM2IDQ0NSA4MThMNDQ1IDgxOFE0NDcgNzk4IDQ0OCA3OThMNDQ4IDc5OFE0NDggNzk4IDQ1MCA3ODRMNDUwIDc4NFE0NTEgNzcwIDQ1MiA3NjlMNDUyIDc2OEw0NTIgNzY2UTQ1MiA3NjIgNDU0IDc1NUw0NTQgNzU1UTQ1NSA3NDggNDU2IDc0MEw0NTYgNzMyTDQ1NiA3MzJRNDU3IDczMiA0NTkgNzE0TDQ1OSA3MTRRNDYxIDY5NiA0NjIgNjk1TDQ2MiA2OTRMNDYyIDY4OEw0NjMgNjg4TDQ2MyA2ODRRNDYzIDY4MiA0NjQgNjc0TDQ2NCA2NzRRNDY2IDY2NiA0NjggNjQ0TDQ2OCA2NDRRNDcxIDYyMiA0NzIgNjIyTDQ3MiA2MjJRNDcyIDYyMiA0NzQgNjAyTDQ3NCA2MDJRNDc3IDU4MiA0NzggNTgxTDQ3OCA1ODBMNDc4IDU3Mkw0NzkgNTcyTDQ3OSA1NjRMNDgwIDU2Mkw0ODAgNTU4UTQ4MCA1NTQgNDgxIDU1MEw0ODIgNTQ0TDQ4MCA1NDJRNDc5IDU0MCA0MjAgNDgwTDQyMCA0ODBRMzYwIDQyMiAzNTcgNDE2TDM1NyA0MTZRMzU0IDQxMiAzNTIgNDA2TDM1MiA0MDZRMzUxIDQwMiAzNTEgMzk0TDM1MSAzOTRRMzUxIDM4NiAzNTMgMzc5TDM1MyAzNzlRMzU1IDM3MiAzNTcgMzY4TDM1NyAzNjhRMzU5IDM2NCA0MjMgMzAxTDU5MyAxMzFRNjU2IDY3IDY2NiA1OUw2NjYgNTlRNjc0IDUxIDY4MCA1MEw2ODAgNTBRNjg0IDUwIDY4NSA1MEw2ODYgNDlaTTExOTAgMTEwTDEyMDAgMTEwUTEyMDggMTEwIDEyMTQgMTEyTDEyMTQgMTEyUTEyMjAgMTEzIDEyMzAgMTE4TDEyMzAgMTE4UTEyNDAgMTIzIDEyNDIgMTI0TDEyNDMgMTI2TDEyNDMgMTI4TDEyNDAgMTMxUTEyMzYgMTM0IDEyMzMgMTM2TDEyMzMgMTM2UTEyMzAgMTM3IDEyMjUgMTQwTDEyMjUgMTQwUTEyMjAgMTQzIDEyMDMgMTUyTDEyMDMgMTUyUTExODYgMTYwIDExNzEgMTY4TDExNzEgMTY4UTExNTYgMTc2IDExNTQgMTc4TDExNTQgMTc4UTExNTIgMTgwIDExNTAgMTgyTDExNTAgMTgyUTExNDkgMTg0IDExNDggMTg4TDExNDggMTkyTDExNDkgMTkyTDExNDkgMjA4TDExNTAgMjA4TDExNTAgMjI0TDExNTEgMjI2TDExNTMgMjY4TDExNTQgMjY4TDExNTcgMzI0TDExNTggMzI2TDExNTggMzMwUTExNTggMzM0IDExNTYgMzM3TDExNTQgMzQwTDEwODQgMzQyUTEwMTQgMzQzIDk3NSAzMzFMOTc1IDMzMVE5MzYgMzE5IDkzNSAzMTlMOTM1IDMxOVE5MzQgMzE5IDkzMiAzMThMOTMyIDMxOFE5MzAgMzE3IDkxMCAzMTJMOTEwIDMxMlE4OTIgMzA3IDg4OCAzMDZMODg4IDMwNlE4ODQgMzA0IDg4MyAzMDRMODgzIDMwNFE4ODIgMzA0IDg3OCAzMDJMODc4IDMwMlE4NzQgMzAxIDg3MyAzMDFMODczIDMwMVE4NzIgMzAxIDg2NiAyOTlMODY2IDI5OVE4NjIgMjk3IDg1OSAyOTRMODU2IDI5Mkw4NTYgMjkwUTg1NiAyOTAgODY2IDI2OUw4NjYgMjY5UTg3NyAyNDggODg4IDIyOUw4ODggMjI5UTg5OSAyMTAgOTA3IDE5NEw5MDcgMTk0UTkxNSAxNzggOTIxIDE2OUw5MjEgMTY5UTkyNyAxNjAgOTMzIDE1Nkw5MzMgMTU2UTkzOCAxNTEgOTQ0IDE0OEw5NDQgMTQ4UTk1MCAxNDYgOTY0IDE0NEw5NjQgMTQ0UTk3NiAxNDIgOTc3IDE0Mkw5NzcgMTQyUTk3OCAxNDEgOTkxIDE0MEw5OTEgMTQwUTEwMDQgMTM4IDEwMDggMTM3TDEwMDggMTM3UTEwMTIgMTM2IDEwNDkgMTMxTDEwNDkgMTMxUTEwODYgMTI2IDEwOTcgMTI0TDEwOTcgMTI0UTExMDggMTIyIDExMTEgMTIyTDExMTEgMTIyUTExMTQgMTIyIDExMTggMTIxTDExMTggMTIxUTExMjIgMTIwIDExMzggMTE4TDExMzggMTE4UTExNTYgMTE2IDExNTYgMTE2TDExNTYgMTE2UTExNTYgMTE1IDExNjYgMTE0TDExNjYgMTE0UTExNzYgMTEzIDExNzkgMTEyTDExNzkgMTEyUTExODIgMTExIDExODYgMTExTDExOTAgMTExTDExOTAgMTEwWk0xMTUwIDM4NkwxMjA4IDM4NkwxMjA4IDM4N0wxMjc2IDM4OEwxMjc2IDM4OUwxMjkyIDM4OUwxMjk0IDM5MEwxMzA2IDM5MEwxMzA4IDM5MUwxMzIwIDM5MUwxMzIwIDM5MkwxMzM0IDM5MkwxMzM0IDM5M0wxMzQ2IDM5M0wxMzQ4IDM5NEwxNDA2IDM5OEwxNDA4IDM5OUwxNDEyIDM5OVExNDE2IDM5OSAxNDE5IDQwMEwxNDE5IDQwMFExNDIyIDQwMiAxNDQ2IDQyNUwxNDQ2IDQyNVExNDcwIDQ0OCAxNDcyIDQ1MkwxNDcyIDQ1MlExNDc0IDQ1NiAxNDc0IDQ5OUwxNDczIDU0MkwxNDcyIDU0MkwxNDcyIDU1NVExNDcxIDU2OCAxNDcwIDU3NEwxNDcwIDU3NFExNDY4IDU4MCAxNDY2IDU4NkwxNDY2IDU4NlExNDY0IDU5MCAxNDYyIDU5NEwxNDYyIDU5NFExNDYwIDU5OCAxNDU3IDYwMUwxNDU3IDYwMVExNDU0IDYwNCAxNDUwIDYwNkwxNDUwIDYwNlExNDQ2IDYwNyAxNDQwIDYwN0wxNDQwIDYwN1ExNDM0IDYwNyAxNDI1IDYwNEwxNDI1IDYwNFExNDE2IDYwMSAxNDA4IDYwMEwxNDA4IDYwMFExNDAwIDU5OCAxMzk4IDU5N0wxMzk4IDU5N1ExMzk2IDU5NiAxMzU4IDU4NkwxMzU4IDU4NlExMzIwIDU3NyAxMzE4IDU3NkwxMzE4IDU3NlExMzE2IDU3NSAxMzA2IDU3M0wxMzA2IDU3M1ExMjk2IDU3MSAxMjg4IDU2OEwxMjg4IDU2OFExMjgwIDU2NiAxMjc4IDU2NkwxMjc4IDU2NlExMjc2IDU2NiAxMjY4IDU2NEwxMjY4IDU2NFExMjYwIDU2MSAxMjU3IDU1OEwxMjU3IDU1OFExMjU0IDU1NiAxMjUwIDU1MkwxMjUwIDU1MlExMjQ3IDU0OCAxMjQ0IDU0MUwxMjQ0IDU0MVExMjQxIDUzNCAxMjQwIDQ5OEwxMjM5IDQ2MkwxMTkwIDQ2M0wxMTkwIDQ2NEwxMTc5IDQ2NEwxMTc4IDQ5MEwxMTc3IDQ5MEwxMTc3IDUyMkwxMTc2IDUyNEwxMTc1IDU0NkwxMTc0IDU0OEwxMTc0IDU1MFExMTc0IDU1NCAxMTcyIDU2NEwxMTcyIDU2NFExMTcwIDU3NCAxMTY2IDU4NkwxMTY2IDU4NlExMTYxIDYwMCAxMTU0IDYxMUwxMTU0IDYxMVExMTQ4IDYyMiAxMTQ0IDYyN0wxMTQ0IDYyN1ExMTQxIDYzMiAxMTM0IDYzOEwxMTM0IDYzOFExMTI4IDY0NSAxMTE2IDY1M0wxMTE2IDY1M1ExMTA2IDY2MSAxMDk4IDY2NUwxMDk4IDY2NVExMDkwIDY2OSAxMDc4IDY3NEwxMDc4IDY3NFExMDY2IDY3OCAxMDU2IDY4MEwxMDU2IDY4MFExMDQ0IDY4MyAxMDM4IDY4NEwxMDMwIDY4NEwxMDMwIDY4NUw5ODIgNjg2TDk4MCA2ODVMOTY2IDY4NUw5NjQgNjg0TDk1NyA2ODRROTUwIDY4MyA5MzMgNjc4TDkzMyA2NzhROTE2IDY3NCA5MDQgNjY4TDkwNCA2NjhRODkwIDY2MiA4ODEgNjU2TDg4MSA2NTZRODcyIDY0OSA4NjIgNjQwTDg2MiA2NDBRODUzIDYzMCA4NDggNjI0TDg0OCA2MjRRODQ0IDYxOCA4MzggNjA4TDgzOCA2MDhRODMzIDU5OCA4MzAgNTg4TDgzMCA1ODhRODI2IDU3OCA4MjQgNTY3TDgyNCA1NjdRODIxIDU1NiA4MjAgNTQ4TDgyMCA1NDBMODE5IDU0MEw4MTkgNTE2TDgyMCA1MTZMODIxIDQ5M1E4MjIgNDcwIDgyMiA0NjhMODIyIDQ2OFE4MjMgNDY2IDgyNiA0NjFMODI2IDQ2MVE4MjggNDU2IDg1OSA0MjZMODU5IDQyNlE4OTAgMzk2IDg5MyAzOTRMODkzIDM5NFE4OTYgMzkzIDkwNiAzOTNMOTE0IDM5M0w5MTYgMzkyTDEwMTQgMzkwTDEwMTQgMzg5TDEwNTIgMzg5TDEwNTIgMzg4TDExNDggMzg3TDExNTAgMzg2Wk0xMDQ0IDQzOEwxMDQ0IDQzOUwxMDEyIDQzOUwxMDEwIDQ0MEw5NzggNDQwTDk3OCA0NDFMOTM0IDQ0Mkw5MzQgNDQzTDkzMCA0NDNROTI2IDQ0MyA5MjIgNDQ1TDkyMiA0NDVROTE2IDQ0NyA5MDMgNDYwTDkwMyA0NjBRODkwIDQ3MiA4OTAgNDc0TDg5MCA0NzRRODg5IDQ3NiA4ODggNDg0TDg4OCA0OTJMODg5IDQ5NEw4ODkgNTA4TDg5MCA1MTBMODkwIDUyNEw4OTEgNTI0TDg5MSA1NDBMODk0IDU0MUw4OTQgNTQwUTg5NiA1MzkgOTA2IDUzNEw5MDYgNTM0UTkxOCA1MjggOTUyIDUwOEw5NTIgNTA4UTk4NiA0ODcgMTAyOCA0NjRMMTAyOCA0NjRRMTA3MCA0NDAgMTA3MCA0MzlMMTA3MCA0MzhMMTA0NCA0MzhaTTU1MCA4MTZMNTUwIDgxN1E1NTAgODE4IDU1MSA4MjBMNTUxIDgyMFE1NTIgODIyIDU1OCA4NDRMNTU4IDg0NFE1NjMgODY2IDU2NCA4NjhMNTY0IDg2OFE1NjUgODcwIDU3NiA5MTZMNTc2IDkxNlE1ODggOTYyIDU4OSA5NjRMNTg5IDk2NFE1OTAgOTY2IDU5NCA5ODJMNTk0IDk4MlE1OTcgOTk2IDU5OCA5OThMNTk4IDk5OFE1OTkgMTAwMCA2MDAgMTAwNEw2MDAgMTAwNFE2MDAgMTAwOCA2MDEgMTAxMEw2MDEgMTAxMFE2MDIgMTAxMiA2MDggMTAzM0w2MTQgMTA1NEw2MjEgMTA1NlE2MjggMTA1NyA2MzQgMTA1N0w2NDAgMTA1N0w2NDAgMTA1OEw2NjQgMTA1OUw2NjQgMTA2MEw2NzggMTA2MEw2ODAgMTA2MUw2OTQgMTA2MUw2OTQgMTA2Mkw3MDYgMTA2Mkw3MDggMTA2M0w3MTggMTA2M0w3MjAgMTA2NEw3MzIgMTA2NEw3MzIgMTA2NUw3NDQgMTA2NUw3NDQgMTA2Nkw3NTkgMTA2N1E3NzQgMTA2OCA3ODAgMTA3MEw3ODAgMTA3MFE3ODYgMTA3MiA4MzUgMTA5Mkw4ODMgMTExMkw4ODMgMTExMVE4ODIgMTExMCA4NTIgMTA4N0w4NTIgMTA4N1E4MjAgMTA2NCA4MDcgMTA1M0w4MDcgMTA1M1E3OTQgMTA0MiA3OTAgMTA0MEw3OTAgMTA0MFE3ODYgMTAzNyA3NjYgMTAzMkw3NjYgMTAzMlE3NDggMTAyOCA3MzggMTAyNkw3MzggMTAyNlE3MjggMTAyNSA3MjYgMTAyNEw3MjYgMTAyNFE3MjQgMTAyMyA3MTYgMTAyMkw3MTYgMTAyMlE3MTAgMTAyMSA3MDcgMTAyMEw3MDcgMTAyMFE3MDQgMTAxOSA2ODAgMTAxNEw2NTQgMTAwOUw2MzkgOTgyUTYyNCA5NTQgNjE0IDkzNEw2MTQgOTM0UTYwMyA5MTIgNTk2IDkwMkw1OTYgOTAyUTU5MCA4OTAgNTgxIDg3Mkw1ODEgODcyUTU3MiA4NTQgNTYxIDgzNUw1NTAgODE2WiIvPjwvc3ZnPgo=';

/* Made-up example content */
const P = (intro, ...secs) => ({intro, secs});
const D = {
  net:{t:'folder',n:'Network',kids:['plan','tickets'],path:'operations/network/',why:'Default roles for everything below.',vis:'Group ops'},
  plan:{t:'page',n:'Network plan',w:'wide',kids:['vlans','ports'],path:'operations/network/plan.md',by:'A. Keller',ch:'today 09:41 by A. Keller',vis:'Group ops, inherited',
    uses:{ext:['vendor manual'],int:['VLAN list','On-call handbook'],data:['Site photos'],vals:['router.ip','tower.height'],db:['Inventory · Open tickets']},usedIn:['On-call handbook','Tower 50 m'],
    doc:P('The site has one router, two core switches and a radio link on the tower.',['Overview','<p>The router sits at <code>192.0.2.1</code> (value <code>{{router.ip}}</code>). The tower is <code>50 m</code> high (value <code>{{tower.height}}</code>), which sets the range of the radio link.</p><div class="alert"><section>Mermaid diagrams come with M-3. They render here, inside the same window.</section></div>'],['Core switches','<p>Both core switches form one stack. See <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="ports">Switch ports</a>.</p><div class="table-container"><table class="table"><tr><th>Name</th><th>Room</th><th>Uplink</th></tr><tr><td>core-a</td><td>Server room</td><td>10 Gbit</td></tr><tr><td>core-b</td><td>Workshop</td><td>10 Gbit</td></tr></table></div>'],['Radio link','<p>The radio link runs from the tower to the gatehouse. Check it after every storm.</p><p>Keep a spare antenna in the workshop, and note each swap in the handbook.</p>'],['Open tickets','<p>This table comes from the view <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="tickets">Open tickets</a> and shows how old its data is.</p><div class="table-container"><table class="table"><tr><th>#</th><th>Title</th><th>Owner</th></tr><tr><td>412</td><td>Replace fan in core-b</td><td>A. Keller</td></tr><tr><td>415</td><td>Label patch panel</td><td>J. Roth</td></tr></table></div>'],['Change log','<p>2026-10-02: new uplink on core-b.</p><p>2026-09-14: radio link firmware.</p><p>2026-08-30: tower antenna replaced.</p>'])},
  vlans:{t:'page',n:'VLAN list',w:'slim',kids:[],path:'operations/network/plan/vlans.md',by:'A. Keller',ch:'Oct 28',vis:'Group ops, inherited',uses:{ext:[],int:['Network plan'],data:[],vals:[],db:[]},usedIn:['Network plan'],
    doc:P('Every VLAN on the site.',['Office','<p>VLAN 10, for desks and printers.</p>'],['Devices','<p>VLAN 20, for cameras and sensors.</p>'])},
  ports:{t:'table',n:'Switch ports',w:'wide',kids:[],path:'not a file, lives in the database',by:'J. Roth',ch:'yesterday',vis:'Group ops, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Network plan'],
    tab:{note:'Native table: edit rows here, with history per row (M-6).',head:['Port','Device','VLAN','Status'],rows:[['1/1','core-b','trunk','up'],['1/2','camera gate','20','up'],['1/3','printer office','10','up'],['1/4','(free)','–','down'],['1/5','radio link','trunk','up']]}},
  tickets:{t:'view',n:'Open tickets',w:'wide',kids:[],path:'not a file, a saved query',by:'J. Roth',ch:'Oct 30',vis:'Group ops',uses:{ext:[],int:[],data:[],vals:[],db:['Inventory']},usedIn:['Network plan'],
    tab:{note:'From the database Inventory. Data is 4 minutes old (M-4).',head:['#','Title','Owner','Age'],rows:[['412','Replace fan in core-b','A. Keller','3 d'],['415','Label patch panel','J. Roth','1 d'],['416','Renew switch license','A. Keller','4 h']]}},
  logs:{t:'folder',n:'Site logs (11 children, stays closed)',kids:Array.from({length:11},(_,i)=>'log'+i),path:'operations/site-logs/',why:'More children than the limit below, so it does not open by itself.',vis:'Group ops'},
  hand:{t:'page',n:'On-call handbook',w:'slim',kids:['esc','cont'],path:'operations/on-call.md',by:'M. Sander',ch:'Oct 29',vis:'Everyone in the company',uses:{ext:['status page'],int:['Network plan'],data:[],vals:['router.ip'],db:[]},usedIn:[],
    doc:P('What to do when something breaks at night.',['First five minutes','<p>Open the <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="plan">Network plan</a> next to this page. Check the router at <code>192.0.2.1</code>.</p>'],['Who to call','<p>See the <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="cont">Contacts</a> table below this page.</p>'],['After the incident','<p>Write down what happened on the same day.</p>'])},
  esc:{t:'page',n:'Escalation',w:'slim',kids:[],path:'operations/on-call/escalation.md',by:'M. Sander',ch:'Oct 12',vis:'Everyone, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['On-call handbook'],doc:P('Who decides what, and when.',['Levels','<p>Level one fixes, level two decides.</p>'],['Times','<p>Fifteen minutes per level.</p>'])},
  cont:{t:'table',n:'Contacts',w:'slim',kids:[],path:'not a file, lives in the database',by:'M. Sander',ch:'Oct 20',vis:'Everyone, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['On-call handbook'],tab:{note:'Native table. A changed number shows on every page that embeds it.',head:['Name','Role','Phone'],rows:[['A. Keller','Network','+00 555 0101'],['J. Roth','Site','+00 555 0102'],['M. Sander','Lead','+00 555 0103']]}},
  tower:{t:'page',n:'Tower 50 m',w:'full',kids:['dims','photos'],path:'projects/tower.md',by:'J. Roth',ch:'Oct 31',vis:'Project team',uses:{ext:[],int:['Network plan'],data:['Tower dimensions','Site photos'],vals:['tower.height'],db:[]},usedIn:['Network plan'],
    doc:P('This page asks for the full width, so the left navigation hides while its window is active.',['Status','<p>The tower stands at <code>50 m</code>. Foundation and first segment are done.</p>'],['Dimensions','<p>From the named table in <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="dims">Tower dimensions</a>.</p>'],['Photos','<p>See <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="photos">Site photos</a>.</p>'])},
  dims:{t:'excel',n:'Tower dimensions',w:'wide',kids:[],path:'projects/tower/tower-dimensions.xlsx',by:'J. Roth (from the share log)',ch:'today 08:12 by J. Roth, over the share',vis:'Project team, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Tower 50 m'],
    tab:{note:'Excel file on the share: open it in Excel or LibreOffice, and the web shows the same file. Share path: \\\\server\\projects\\tower\\tower-dimensions.xlsx (M-2).',head:['Segment','Height (m)','Diameter (m)'],rows:[['Base','12','4.2'],['Middle','20','3.4'],['Top','18','2.6']]}},
  photos:{t:'images',n:'Site photos',w:'wide',kids:[],path:'projects/tower/photos/',by:'J. Roth',ch:'Oct 18',vis:'Project team, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Tower 50 m','Network plan']},
  inv:{t:'db',n:'Inventory',w:'wide',kids:['tickets2'],path:'not a file, a connection (Resources tab)',by:'A. Keller',ch:'Sep 02',vis:'Group ops',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:['Open tickets'],
    tab:{note:'A connection, placed here as well. It exists once: the Resources tab lists the same object.',head:['Table','Rows','Read only'],rows:[['devices','1,204','yes'],['tickets','318','yes'],['sites','14','yes']]}},
  tickets2:{t:'view',n:'Devices by site',w:'wide',kids:[],path:'not a file, a saved query',by:'A. Keller',ch:'Sep 03',vis:'Group ops, inherited',uses:{ext:[],int:[],data:[],vals:[],db:['Inventory']},usedIn:[],tab:{note:'A saved query below its database.',head:['Site','Devices'],rows:[['Main','842'],['Gatehouse','362']]}},
  n1:{t:'page',n:'Ideas for the workshop',w:'slim',kids:[],path:'not a file yet, private note',by:'you',ch:'today',vis:'Only you',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[],doc:P('A private note, in your own collection.',['Shelves','<p>Move spare parts next to the door.</p>'],['Labels','<p>Print new ones.</p>'])},
};
for (let i=0;i<11;i++) D['log'+i]={t:'page',n:'Log week '+(30+i),w:'slim',kids:[],path:`operations/site-logs/week-${30+i}.md`,by:'J. Roth',ch:'Oct',vis:'Group ops, inherited',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[],doc:P('Weekly site log.',['Work','<p>Routine checks.</p>'],['Notes','<p>Nothing unusual.</p>'])};
const C = [{id:'ops',n:'Operations',top:['net','logs','hand']},{id:'pro',n:'Projects',top:['tower','inv']},{id:'me',n:'My notes',top:['n1']}];
function bodyHtml(w,i){
  if(D[w.d].t==='epd') return {h:`<div data-timeframe="${D[w.d].src}" data-win="${i}"></div>`,hs:[],to:null};
  const d=D[w.d]; let hs=d.doc?d.doc.secs.map(s=>s[0]):[],to=null;
  let h='';
  /* The contents of a page of the database are its blocks that begin with a heading (M-8), each a jump to its block. */
  if(d.blocks){h+=blocksHtml(d,w,i);const heads=d.blocks.filter(b=>/^#{1,3} \S/.test(b.raw||''));hs=heads.map(b=>esc((b.raw||'').split('\n')[0].replace(/^#+ /,'')));to=heads.map(b=>`w${i}${b.id}`);}
  else if(d.doc) h+=(d.real?d.doc.intro:`<p>${d.doc.intro}</p>`)+d.doc.secs.map((s,j)=>`<h2 id="w${i}h${j}">${s[0]}</h2>${s[1]}`).join('');
  if(d.tab) h+=note(d.tab.note)+tableHtml(d.tab.head,d.tab.rows);
  if(d.t==='images'){const tones=[['oklch(45% .06 240)','Base, north'],['oklch(50% .05 150)','Foundation'],['oklch(42% .04 40)','Segment 2'],['oklch(38% .05 280)','Crane'],['oklch(48% .03 200)','Antenna'],['oklch(36% .02 100)','Gatehouse']];h+=`<div class="alert"><section>These images lie in a folder on the disk. This collection only shows them.</section></div><div class="grid grid-cols-2 gap-2 md:grid-cols-3">${tones.map(t=>`<figure class="card aspect-video items-center justify-center text-sm" data-style="background:${t[0]};color:#fff">${t[1]}</figure>`).join('')}</div>`;}
  if(d.t==='folder') h+=`<p>${d.why}</p>`;
  if(d.kids.length) h+=`${d.t==='folder'||d.t==='coll'?'':'<h2>Below this document</h2>'}<div class="item-group" role="list" data-kids>${d.kids.filter(shown).map(k=>`<a href="#" class="item" data-variant="outline" data-size="sm" role="listitem" data-act="open" data-id="${k}"><figure>${icon(D[k].i||TYPE[D[k].t][0])}</figure><section><h4>${D[k].n}</h4><p class="text-muted-foreground text-xs">${TYPE[D[k].t][1]}</p></section></a>`).join('')}</div>`;
  if(d.html) h=d.t==='coll'?d.html+h:h+d.html;
  return {h,hs,to};
}
/* Lucide (ISC, licenses/Lucide-LICENSE.txt): book-open, bold, italic, strikethrough, heading, list-ordered, type. */
Object.assign(I,{book:'<path d="M12 7v14"/><path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z"/>',
  bold:'<path d="M6 12h9a4 4 0 0 1 0 8H7a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h7a4 4 0 0 1 0 8"/>',
  italic:'<path d="M19 4h-9M14 20H5M15 4 9 20"/>',
  strike:'<path d="M16 4H9a3 3 0 0 0-2.83 4"/><path d="M14 12a4 4 0 0 1 0 8H6"/><path d="M4 12h16"/>',
  heading:'<path d="M6 12h12M6 20V4M18 20V4"/>',
  olist:'<path d="M10 12h11M10 18h11M10 6h11M4 10h2M4 6h1v4"/><path d="M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"/>',
  text:'<path d="M12 4v16M4 7V5a1 1 0 0 1 1-1h14a1 1 0 0 1 1 1v2M9 20h6"/>'});
Object.assign(I,{chat:'<path d="M4 5h16v11H9l-5 4z"/>',search:'<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  /* The gear is Lucide's "settings" (ISC, licenses/Lucide-LICENSE.txt): the drawn one read as a sun (2026-11-09). */
  gear:'<path d="M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915"/><circle cx="12" cy="12" r="3"/>',
  pin:'<path d="M9 4h6l-1 6 3 3v2H7v-2l3-3zM12 15v6"/>', more:'<path d="M5 12h.01M12 12h.01M19 12h.01"/>', x:'<path d="M6 6l12 12M18 6 6 18"/>',
  grip:'<path d="M9 6h.01M15 6h.01M9 12h.01M15 12h.01M9 18h.01M15 18h.01"/>', star:'<path d="m12 3 2.8 5.8 6.2.9-4.5 4.4 1 6.3-5.5-3-5.5 3 1-6.3L3 9.7l6.2-.9z"/>',
  code:'<path d="m8 8-4 4 4 4M16 8l4 4-4 4M14 5l-4 14"/>', pulse:'<path d="M3 12h4l3-8 4 16 3-8h4"/>', flag:'<path d="M5 21V4h13l-3 4 3 4H5"/>',
  pencil:'<path d="m4 20 4-1L19 8l-3-3L5 16z"/>', back:'<path d="m15 6-6 6 6 6"/>', layout:'<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M8 4v16M3 10h5M3 15h5"/>', split:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="M12 5v14"/>',up:'<path d="m6 15 6-6 6 6"/>', down:'<path d="m6 9 6 6 6-6"/>'});
const TAGS_={net:['network'],plan:['network','main-site'],vlans:['network'],ports:['network','hardware'],tickets:['network'],hand:['on-call'],esc:['on-call'],cont:['on-call','people'],tower:['tower','main-site'],dims:['tower'],photos:['tower','main-site'],inv:['hardware'],tickets2:['hardware'],n1:['idea']};
const TAGS=TAGS_;
Object.assign(I,{check:'<rect x="4" y="4" width="16" height="16" rx="2"/><path d="m8 12 3 3 5-6"/>',board:'<rect x="3" y="4" width="18" height="16" rx="1"/><path d="M9 4v16M15 4v16"/>',
  video:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="m10 9 5 3-5 3z"/>',slides:'<rect x="3" y="4" width="18" height="12" rx="1"/><path d="M12 16v4M8 20h8"/>',
  note:'<path d="M5 4h14v11l-5 5H5z"/><path d="M14 20v-5h5"/>',mail:'<rect x="3" y="5" width="18" height="14" rx="1"/><path d="m3 7 9 6 9-6"/>',
  net:'<rect x="9" y="3" width="6" height="5" rx="1"/><rect x="3" y="16" width="6" height="5" rx="1"/><rect x="15" y="16" width="6" height="5" rx="1"/><path d="M12 8v4M6 16v-4h12v4"/>',send:'<path d="M21 3 3 10l7 3 3 7z"/>',cal:'<rect x="3" y="5" width="18" height="16" rx="1"/><path d="M3 10h18M8 3v4M16 3v4"/>',clock:'<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>',sun:'<circle cx="12" cy="12" r="4"/><path d="M12 3v2M12 19v2M3 12h2M19 12h2M5.6 5.6 7 7M17 17l1.4 1.4M5.6 18.4 7 17M17 7l1.4-1.4"/>',globe:'<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3 3 15 0 18M12 3c-3 3-3 15 0 18"/>',grid:'<rect x="4" y="4" width="7" height="7"/><rect x="13" y="4" width="7" height="7"/><rect x="4" y="13" width="7" height="7"/><rect x="13" y="13" width="7" height="7"/>',lock:'<rect x="5" y="11" width="14" height="10" rx="1"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>',shield:'<path d="M12 3 4 6v6c0 5 3.5 8 8 9 4.5-1 8-4 8-9V6z"/><path d="m9 12 2 2 4-4"/>',opennew:'<path d="M14 4h6v6M20 4l-9 9"/><path d="M18 14v6H4V6h6"/>',quote:'<path d="M9 7H5v6h4zM19 7h-4v6h4zM9 13c0 2-1 4-3 4M19 13c0 2-1 4-3 4"/>',spark:'<path d="M12 3l2 7 7 2-7 2-2 7-2-7-7-2 7-2z"/>',ai:'<path d="M12 3l2 7 7 2-7 2-2 7-2-7-7-2 7-2z"/><path d="M19 3v4M17 5h4"/>',user:'<circle cx="12" cy="8" r="4"/><path d="M4 21c1-5 4-7 8-7s7 2 8 7"/>',plus:'<path d="M12 5v14M5 12h14"/>',eye:'<path d="M2 12s4-7 10-7 10 7 10 7-4 7-10 7-10-7-10-7z"/><circle cx="12" cy="12" r="3"/>'});
Object.assign(TYPE,{todo:['check','To-do list'],kanban:['board','Kanban board'],video:['video','Video'],slides:['slides','PowerPoint'],notes:['note','Quick notes']});
const none={uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[]};
D.ttodo={t:'todo',n:'Tower tasks',kids:[],path:'not a file, lives in the database',by:'J. Roth',ch:'today',vis:'Project team, inherited',...none,
  items:[{t:'Order the second segment',done:true},{t:'Book the crane',done:true},{t:'Inspect the foundation',done:false},{t:'Mount the antenna',done:false},{t:'Update the network plan',done:false}],
  get stat(){return `${this.items.filter(x=>x.done).length} of ${this.items.length} done`;},
  view:true,get html(){return `<div class="item-group" role="list">${this.items.map((x,k)=>todoItem(x,k,'todo')).join('')}</div>`;}};
D.tboard={t:'kanban',n:'Tower board',w:'full',kids:[],path:'not a file, lives in the database',by:'J. Roth',ch:'today',vis:'Project team, inherited',...none,stat:'6 cards in 3 columns',
  view:true,html:`<p class="text-muted-foreground mb-3 text-sm">A board is another view of a list, so the to-do list above could show like this too. Dragging cards comes with M-6.</p><div class="board">${[['To do',['Mount the antenna','Update the network plan']],['Doing',['Inspect the foundation','Paint the top segment']],['Done',['Order the second segment','Book the crane']]].map(c=>`<section class="card" data-size="sm" aria-label="${c[0]}"><header><h3 class="card-title">${c[0]} <small>${c[1].length}</small></h3></header><section>${c[1].map(x=>`<article class="card" data-size="sm"><section>${x}</section></article>`).join('')}</section></section>`).join('')}</div>`};
D.walk={t:'video',n:'Site walkthrough',w:'wide',kids:[],path:'operations/network/walkthrough.mp4',by:'A. Keller',ch:'Oct 12',vis:'Group ops, inherited',...none,stat:'12:34 · 48 MB',
  html:`<figure class="card aspect-video items-center justify-center text-muted-foreground" role="img" aria-label="Video player">${icon('video').replace('<svg ','<svg class="size-24" ')}<figcaption>0:00 / 12:34</figcaption></figure><p class="text-muted-foreground text-sm">The file lies on the share. The browser plays it, nothing is copied.</p>`};
D.deck={t:'slides',n:'Kickoff slides',w:'wide',cur:0,kids:[],path:'projects/tower/kickoff.pptx',by:'J. Roth',ch:'Sep 30',vis:'Project team, inherited',...none,
  titles:['Tower 50 m','Why we build','Schedule','Open questions'],
  get stat(){return `Slide ${this.cur+1} of ${this.titles.length}`;},
  get html(){return `<figure class="card aspect-video items-center justify-center text-2xl font-semibold">${this.titles[this.cur]}</figure><div class="flex flex-wrap gap-2" role="group" aria-label="Slides">${this.titles.map((x,k)=>`<button type="button" class="btn" data-variant="outline" data-size="sm" data-act="slide" data-id="${k}" aria-pressed="${k===this.cur}"><b>${k+1}</b>${x}</button>`).join('')}</div><p class="text-muted-foreground text-sm">Read-only preview. To edit, open the file in PowerPoint or LibreOffice over the share.</p>`;}};
D.qn={t:'notes',n:'Quick notes',w:'full',kids:[],path:'not a file, private notes',by:'you',ch:'today',vis:'Only you',...none,
  notes:[{h:'Call the crane company',b:'Ask for the Tuesday slot.'},{h:'',b:'Spare antenna: check the shelf in the workshop.'},{h:'Shopping',b:'Cable ties\nLabels\nGloves'},{h:'Idea',b:'Put the tower height on the start page.'},{h:'',b:'Rack photo is out of date.'}],
  get stat(){return `${this.notes.length} notes`;},
  get html(){return `<div class="flex flex-col gap-4"><input class="input mx-auto max-w-lg" type="text" data-addnote placeholder="Take a note, then Enter" aria-label="Take a note"><div class="grid grid-cols-2 gap-2 md:grid-cols-3" data-notes>${this.notes.map(n=>`<article class="card" data-size="sm"><section>${n.h?`<h4 class="font-semibold">${esc(n.h)}</h4>`:''}<p class="text-sm">${esc(n.b).replace(/\n/g,'<br>')}</p></section></article>`).join('')}</div></div>`;}};
D.tower.kids=[...D.tower.kids,'ttodo','tboard','deck'];
D.net.kids=[...D.net.kids,'walk'];
C.find(c=>c.id==='me').top=['qn','n1'];
Object.assign(TAGS,{ttodo:['tower'],tboard:['tower'],deck:['tower'],walk:['network'],qn:['idea']});
D.plan.desc='Router, core switches and the radio link of the main site.';D.plan.cby='J. Roth';D.plan.created='Sep 12';D.tower.desc='Build of the 50 m tower: status, dimensions and photos.';D.hand.desc='What to do when something breaks at night.';
D.tower.doc.intro='This page stores the width full, so it uses all the room its window has. The navigation stays where it is.';
D.hand.toc='side';
D.old={t:'page',n:'Old offer notes',w:'slim',kids:[],path:'archive/old-offer.md',by:'A. Keller',ch:'Mar 04',vis:'Group ops',uses:{ext:[],int:[],data:[],vals:[],db:[]},usedIn:[],doc:P('Kept for reference.',['Result','<p>Not ordered.</p>'],['Reason','<p>Too expensive.</p>'])};
Object.entries(D).forEach(([k,d])=>{d.w=d.w||'slim';d.toc=d.toc||'marker';d.tags=TAGS[k]||[];});
const ICON={ops:'pulse',pro:'flag',me:'pencil',arc:'box'};
C.push({id:'arc',n:'Archive',top:['old']});
C.forEach(c=>c.i=ICON[c.id]);
/* A collection is an object like a document: it has an ID, opens in a window with its overview, and can sit inside another collection. */
TYPE.coll=['grid','Collection'];TYPE.line=['grid','Line'];TYPE.space=['grid','Space'];
const isDeco=id=>!!D[id]&&(D[id].t==='line'||D[id].t==='space');
/* The container of an entry of nav3: its parent, or the collection it lies in at the top. */
const contOf=id=>PARENT[id]||(COLL[id]&&COLL[id].id);
/* Made-up content changes only in this browser. Objects of the database change there, and only where one may edit;
   the folder source is read only. */
const canPlace=cont=>!!D[cont]&&(D[cont].db?accOf(cont)==='edit':!D[cont].real);
const placeWhy=cont=>D[cont]&&D[cont].real&&!D[cont].db?'The folder source is read only':ACC[accOf(cont)]||'Not here';
function divItems(mode,id){const cont=mode==='first'?id:contOf(id),ok=canPlace(cont)?'':'disabled title="'+placeWhy(cont)+'"';
  return '<hr role="separator">'+`<button type="button" role="menuitem" data-act="line${mode}" data-id="${id}" ${ok}>${icon('hr')}Line ${mode==='first'?'at the top of it':'below it'}</button><button type="button" role="menuitem" data-act="space${mode}" data-id="${id}" ${ok}>${icon('gap')}Space ${mode==='first'?'at the top of it':'below it'}</button>`;}
function collDoc(id,n,kids,i){D[id]={t:'coll',n,i,kids,w:'wide',toc:'marker',tags:[],path:'not a file, a collection',by:'A. Keller',ch:'Oct 31',vis:'Group ops',...none,
  get stat(){return `${this.kids.length} entries`;},
  /* The overview holds the settings of the collection too, for whoever may change them (the maintainer, 2026-11-09). */
  get html(){const ac=accOf(id),may=this.db?ac==='edit'||!!this.mine:!this.real&&ac==='edit',top=C.some(c=>c.id===id&&!c.alias),owner=isOwner(id);
    const facts=`<dl><dt>Owner</dt><dd>${this.owner||'A. Keller'}</dd>${may?'':`<dt>Others can</dt><dd>${ACC[othersOf(id)].toLowerCase()}${this.acc?'':' (the default)'}</dd>`}${this.db&&!this.private?'':`<dt>Visible to</dt><dd>${id==='me'||this.private?'only you':'everyone signed in'}</dd>`}<dt>Entries</dt><dd>${this.kids.length}</dd></dl>`;
    const set=may?`<h2>Settings</h2><div class="flex max-w-sm flex-col gap-2"><label class="label" for="cname">Name</label><input id="cname" class="input" data-cname="${id}" value="${this.n}" aria-label="Name of the collection">
      <span class="label">Others can</span>${segs('cacc',(top?(this.db?[['none','Nobody']]:[]):[['inherit','As above']]).concat([['edit','Edit'],['comment','Comment'],['read','Read']]),this.acc||(top?(this.db?'none':'edit'):'inherit'),`data-c="${id}"`+(owner?'':' disabled title="Only the owner sets this"'),'Others can')}
      <p class="text-muted-foreground text-sm">Everyone in this workspace. Inherited by everything in it, unless something sets its own. The owner can always edit, and only the owner sets this; groups and people get roles in its rights.</p></div>`:'';
    const rights=this.db?`<p>${pbtn(`data-act="rights" data-id="${id}"`,'Rights and sharing')}</p>`:'';
    return facts+rights+set+(this.kids.length?'':'<p class="text-muted-foreground text-sm">Empty. New + at its heading in nav3 adds something.</p>');}};}
/* A collection in nav2 reads its name and list from its object, so moving it between nav2 and nav3 keeps one object. */
function asTop(c){Object.defineProperty(c,'n',{get(){return D[c.id].n;},set(v){D[c.id].n=v;},configurable:true,enumerable:true});Object.defineProperty(c,'top',{get(){return D[c.id].kids;},configurable:true,enumerable:true});return c;}
C.forEach(c=>{const n=c.n,top=c.top;delete c.n;delete c.top;collDoc(c.id,n,top,c.i);asTop(c);});
D.bud={t:'page',n:'Budget 2027',w:'slim',toc:'marker',tags:[],kids:[],path:'projects/planning-2027/budget.md',by:'J. Roth',ch:'Nov 02',vis:'Project team, inherited',...none,doc:P('First numbers for next year.',['Hardware','<p>Two switches and a spare antenna.</p>'],['Services','<p>The crane for one day.</p>'])};
collDoc('plan27','Planning 2027',['bud'],'grid');D.plan27.desc='Ideas and numbers for next year. A collection inside Projects.';
C.find(c=>c.id==='pro').top.push('plan27');
/* Collections inside a collection below Projects, to try showing one in nav2 (2026-11-09). */
collDoc('bud27','Budget lines',[],'grid');D.plan27.kids.push('bud27');collDoc('q127','Quarter 1',[],'grid');D.bud27.kids.push('q127');
D.plan27.i='grid';
D.ops.desc='Network, site logs and the on-call handbook.';D.arc.acc='read';D.hand.acc='comment';
TYPE.coll[0]='grid';
D.pro.desc='Builds and purchases.';
const PARENT={},COLL={};
/* A collection inside another can be shown in nav2 as well (the maintainer, 2026-11-09). That entry only points at it: the
   collection lies where it lies, the tree takes no notice of the entry, and the entry goes when the collection stops
   being one, or comes to stand in nav2 itself. */
const aliasOf=id=>C.some(c=>c.alias&&c.id===id);
function asAlias(id){return {id,alias:true,get n(){return D[id].n;},get top(){return D[id].kids;},get i(){return D[id].i||'grid';}};}
function pruneAliases(){for(let i=C.length-1;i>=0;i--){const c=C[i];if(c.alias&&(!D[c.id]||D[c.id].t!=='coll'||C.some(x=>!x.alias&&x.id===c.id)))C.splice(i,1);}}
function rebuildPlaces(){for(const k in PARENT)delete PARENT[k];for(const k in COLL)delete COLL[k];pruneAliases();
  C.forEach(c=>{if(c.alias)return;const walk=(id,p)=>{PARENT[id]=p;COLL[id]=c;D[id].kids.forEach(k=>walk(k,id));};c.top.forEach(id=>walk(id,null));});}
rebuildPlaces();
const placeOf=id=>{if(D[id].place)return D[id].place;if(C.some(c=>c.id===id))return 'Workspace';const a=[];for(let x=PARENT[id];x;x=PARENT[x])a.unshift(D[x].n);return (COLL[id]?[COLL[id].n,...a]:a).join(' › ');};
const esc=s=>String(s).replace(/[&<>"]/g,m=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[m]));
const unesc=s=>String(s).replace(/&(amp|lt|gt|quot);/g,(m,k)=>({amp:'&',lt:'<',gt:'>',quot:'"'}[k]));
const plain=h=>{const e=document.createElement('div');e.innerHTML=h;return e.textContent;};
/* The phone form of C22's workspace starts below 48rem: nav2 and nav3 open over the window. */
const phone=()=>matchMedia('(width < 48rem)').matches;
const W3=300;
const newTab=(d,general=false)=>({d,general,top:0,fly:false,gear:false,cmt:false,find:false,q:'',cur:0,src:false});
const S={help:false,user:{name:'',login:'',email:'',groups:[],initials:'?',admin:false},view:'workspace',prev:'workspace',sec:{files:'share',resources:'values',rights:'groups',mail:'inbox',bookmarks:'bar',news:'all',profile:'acct',todo:'mine',cal:'all',time:'week',vac:'y26',notes:'quick'},landing:false,cq:'',addIn:null,just:null,prof:false,filters:false,newN:0,
  focus:0,rail:!phone(),limit:8,tog:{},treeTop:0,s1:0,s2:3,w3:W3,theme:'system',scale:100,first:false,clock:false,
  ctx:null,newAt:null,tmenu:null,hist:null,tlist:null,toast:'',aid:false,chat:[],into:'qn',tpls:[],
  fav:{ops:true,pro:true,me:true,arc:false},open:{ops:true,pro:true},pin:{ops:true},so:{todo:{mine:true},cal:{all:true},time:{week:true},vac:{y26:true},notes:{quick:true}},spin:{},mods:{mail:true,private:true,bookmarks:true,news:true,ai:true,network:true,lists:true,video:true,slides:true,notes:true},menu:null,
  sq:'',strat:'full',ai:false,ftype:'',ftags:[],
  wins:[{tabs:[newTab('plan',true)],a:0,hist:['plan']}]};
const at=i=>S.wins[i].tabs[S.wins[i].a]||null;
/* What others may do: edit, comment or only read. Set on a collection, folder or document, and inherited by what is below it. */
const ACC={edit:'Can edit',comment:'Can comment',read:'Read only',none:'Nobody else'};
const contAcc=id=>PARENT[id]?accOf(PARENT[id]):COLL[id]?accOf(COLL[id].id):'edit';
/* What others may do: the level set on an object or above it (shown as a lock and in the settings). What the viewer may
   do comes from the server as its role (ADR-29) and decides what is greyed out; the demo has no roles and uses the level. */
const othersOf=id=>{for(let x=id;x;x=PARENT[x])if(D[x]&&D[x].acc)return D[x].acc;const c=COLL[id];return c&&D[c.id].acc||(D[id]&&D[id].db?'none':'edit');};
const RANK={read:1,comment:2,edit:3,manage:4,owner:5};
const accOf=id=>{const r=D[id]&&D[id].db&&D[id].role;if(!r)return othersOf(id);return RANK[r]>=3?'edit':r==='comment'?'comment':'read';};
const kidsOpen=id=>{const v=S.tog[id];return v!==undefined?v:id.startsWith('dir:')?false:D[id].kids.length<=S.limit;};
/* Moments come from the server in UTC (RFC 3339) and show in the time zone of this device (2026-11-09); what is no
   moment, such as "today" or made-up dates, shows as it is. */
const when=(v,time=true)=>{const t=/^\d{4}-\d\d-\d\dT/.test(v||'')?new Date(v):null;if(!t||isNaN(t))return esc(v||'');
  return t.toLocaleString('en-US',{month:'short',day:'2-digit',...(time?{hour:'2-digit',minute:'2-digit',hourCycle:'h23'}:{})}).replace(',','');};
const chg=d=>String(d.ch).includes(' by ')?d.ch:`${d.ch} by ${d.by||'–'}`;
/* Tags in the head of a window: square chips of the pack (C22 a.badge). */
const tagBadges=(tags,act)=>tags.map(t=>`<a href="#" class="badge" data-variant="outline" data-act="${act}" data-tag="${t}" title="All objects tagged ${t}">#${t}</a>`).join('');

/* A row of a list in nav3 is C22's tree row (li > div): the toggle first (a button while it has children, else an empty
   place), then the name. The document open in the active window carries aria-current and the word "open"; what was just
   made, and what is selected, aria-selected. A line or a space between entries is a row of its own. */
const toggleHtml=(id,label,kids,open)=>kids?`<button type="button" data-toggle data-act="tog" data-id="${id}" aria-expanded="${open}" aria-label="Toggle ${label}">${icon('chev')}</button>`:'<span data-toggle aria-hidden="true"></span>';
/* A line or a space is C22's row holding only an hr (a space: hr[data-variant=space]); data-deco names its kind for the
   script and the tests, without a look of its own. */
const decoInner=t=>t==='line'?'<hr>':'<hr data-variant="space">';
function rowHtml(id){
  if(!shown(id))return '';
  const sl=isSel('n3',id)||S.just===id?' aria-selected="true"':'';
  if(isDeco(id))return `<li><div data-deco="${D[id].t}" data-row="${id}"${sl} title="${D[id].n}: drag it, or right click to remove it">${decoInner(D[id].t)}</div></li>`;
  const d=D[id],open=kidsOpen(id),t=at(S.focus),cur=t&&t.d===id;
  return `<li><div data-row="${id}"${sl}>${toggleHtml(id,d.n,d.kids.length>0,open)}${S.ren===id?`${icon(d.i||TYPE[d.t][0])}<input class="input" data-size="sm" data-ren="${id}" value="${d.n}" aria-label="Name">`
    :`<button type="button" data-act="open" data-id="${id}" title="${d.n}"${cur?' aria-current="page"':''}>${icon(d.i||TYPE[d.t][0])}<span>${d.n}</span></button>${cur?'<small>open</small>':''}`}</div>
    ${d.kids.length&&open?`<ul>${d.kids.map(rowHtml).join('')}</ul>`:''}</li>`;
}
const ADDTYPES=['page','folder','bookmark','search','table','todo','kanban','notes','excel','slides','video','images','view','db','ipnet','devices'];
/* The first build: switched-off modules disappear, but what is not built yet stays in sight, greyed out. */
const LATER={view:['files','resources','bookmarks','news','mail','todo','cal','time','vac','notes'],add:ADDTYPES.filter(k=>!['page','folder'].includes(k)),prof:['lang','notif','src','store'],tool:['split','opencol','newwin','aid','qn']};
const later=(kind,id)=>S.first&&LATER[kind].includes(id);
/* Why a document cannot be edited, for the greyed pencil: nothing that is not built or not allowed simply goes missing. */
/* Whether a tab shows its document for editing: what was chosen for this document, else the setting "open for editing". */
/* Pages and folders are made in collections of the database only; elsewhere New says why, instead of making a page that
   cannot be written (2026-11-09). */
const madeHere=id=>{const d=D[id];if(!d||d.db||id==='qn')return '';return d.real?'The folder source is read only. Pages are made in collections of the database':'Demo content. Pages are made in collections of the database';};
const editing=t=>!!(t&&(t.editOf===t.d?t.edit:S.openEdit));
const setEdit=(t,v)=>{t.edit=!!v;t.editOf=t.d;};
const noEdit=id=>{const d=D[id];return d.blocks?'You may only '+(accOf(id)==='comment'?'comment on':'read')+' this page'
  :d.real?'From the folder source, read only: change the file in the share, or import the folder (underdoc import)':'Made-up content of the demo, not editable';};
const off=(kind,id)=>later(kind,id)?' disabled data-later="1"':'';
const addTypes=()=>ADDTYPES.filter(k=>!MODTYPE[k]||S.mods[MODTYPE[k]]);
function treeHtml(){
  const open=openColls();
  if(!open.length) return '<p class="text-muted-foreground text-xs">No collection is open. Pick one in nav2.</p>';
  /* One section per open collection: its head on the pack's band, the lock when others may not edit, New at the end. */
  return open.map(c=>`<section><header data-cdrop="${c.id}" data-ctx="coll" data-ctxid="${c.id}"><a href="#" data-act="open" data-id="${c.id}" title="Open ${c.n}"><span>${c.n}</span>${othersOf(c.id)!=='edit'?`${icon('lock').replace('<svg ',`<svg data-lock aria-label="${ACC[othersOf(c.id)]}" `).replace(' aria-hidden="true"','')}`:''}</a>${canPlace(c.id)?newBtn(`data-act="addmenu" data-id="${c.id}" aria-expanded="${S.addIn===c.id}" aria-label="New in ${c.n}" title="New in ${c.n}"`):''}</header>
    <ul class="workspace-tree">${c.top.map(rowHtml).join('')}</ul></section>`).join('');
}
/* New at the head of nav2 or of a collection: C22's small outline button with the word and the plus. */
const newBtn=attrs=>`<button type="button" class="btn" data-variant="outline" data-size="xs" ${attrs}><span>New</span>${icon('plus')}</button>`;
/* New, in nav2 and in the head of a collection: one menu in groups, and typing filters it (the command menu of C22).
   It floats over everything below its button, so nothing moves. */
Object.assign(I,{link:'<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1 1"/><path d="M14 10a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1-1"/>',hr:'<path d="M4 12h16"/>',gap:'<path d="M4 5h16M4 19h16M12 9v6"/>'});
const SOON=' disabled data-later="1" title="Comes later"';
/* New in a collection: what is needed most comes first, a page, then tables, databases, single records, and boards and
   lists (the maintainer, 2026-11-09); within a block the same order of need. Right after the page come the templates a
   page can start from (M-8). */
const NEWGROUPS=[['',['page']],['Tables',['table','excel']],['Databases',['db','view']],['Records',['ipnet','devices','bookmark']],['Boards and lists',['kanban','todo','notes']],['Media',['images','slides','video']],['Organize',['folder','search']]];
function addPop(){
  if(!S.addIn)return '';
  const n2=S.addIn==='n2',c=!n2&&D[S.addIn];if(!n2&&!c)return '';
  /* An entry with a description (a template) is C22's div > span + small after the icon. */
  const it=(act,id,label,ico,extra='',desc='')=>`<button type="button" role="menuitem" data-act="${act}" data-id="${id}" data-filter="${label}" ${extra}>${icon(ico)}${desc?`<div><span>${label}</span><small>${esc(desc)}</small></div>`:label}</button>`;
  const grp=(h,items)=>`<div role="group">${h?`<span role="heading">${h}</span>`:''}${items}</div>`;
  const place=n2?'':canPlace(S.addIn)?'':' disabled title="'+placeWhy(S.addIn)+'"';
  const groups=n2?[grp('Make',it('newcoll','','Collection','grid')),grp('Set apart',it('n2line','','Line','hr')+it('n2space','','Space','gap')),grp('Bring in',it('none','','Import','down',SOON)+it('none','','Link','link',SOON))]
    :[...NEWGROUPS.flatMap(([h,ks],k)=>[k===1&&c.db&&S.tpls.length?grp('From a template',S.tpls.map(x=>it('addtpl',x.id,x.n,'page',`data-c="${S.addIn}"`+(madeHere(S.addIn)?` disabled title="${madeHere(S.addIn)}"`:''),x.desc||x.place)).join('')):'',(()=>{const items=ks.filter(k=>addTypes().includes(k)).map(k=>[TYPE[k][1],it('add',k,TYPE[k][1],TYPE[k][0],`data-c="${S.addIn}"`+(off('add',k)||(madeHere(S.addIn)?` disabled title="${madeHere(S.addIn)}"`:'')))]);
        if(h==='Organize')items.push(['Collection',it('newcoll',S.addIn,'Collection','grid')]);
        return items.length?grp(h,items.map(x=>x[1]).join('')):'';})()]).filter(Boolean),
      grp('Set apart',it('lineend',S.addIn,'Line','hr',place)+it('spaceend',S.addIn,'Space','gap',place)),
      grp('Bring in',it('none','','Import','down',SOON)+it('none','','Link','link',SOON)+(S.mods.bookmarks?it('open','bm:import','Import bookmarks','down',off('add','bookmark')):''))];
  /* C22's command menu, fixed below the head it belongs to (data-style, kept inside the window after a render). */
  return `<div class="command fixed z-50 w-64 border shadow-md" data-menu="new" data-style="left:${S.addAt.x}px;top:${S.addAt.y}px" aria-label="New ${n2?'in nav2':'in '+c.n}"><header>${icon('search')}<input data-nfilter value="${esc(S.addQ)}" placeholder="Type to filter" aria-label="Filter" autocomplete="off" spellcheck="false" role="combobox" aria-expanded="true"></header><div role="menu" class="scrollbar-sm" data-empty="Nothing matches.">${groups.join('<hr role="separator">')}</div></div>`;
}
/* Typing filters the entries by their names; arrows move the choice, Enter takes it. The focus stays in the field. */
function filterNew(q){
  const items=[...document.querySelectorAll('[data-menu="new"] [role="menuitem"]')],w=q.trim().toLowerCase();
  items.forEach(n=>{n.classList.remove('active');n.setAttribute('aria-hidden',String(!!w&&!n.dataset.filter.toLowerCase().includes(w)));});
  const first=items.find(n=>n.getAttribute('aria-hidden')!=='true'&&!n.disabled);if(first&&w)first.classList.add('active');
}
function stepNew(dir){
  const items=[...document.querySelectorAll('[data-menu="new"] [role="menuitem"]')].filter(n=>n.getAttribute('aria-hidden')!=='true'&&!n.disabled);if(!items.length)return;
  const k=items.findIndex(n=>n.classList.contains('active'));items.forEach(n=>n.classList.remove('active'));
  const n=items[(k<0?(dir>0?0:items.length-1):k+dir+items.length)%items.length];n.classList.add('active');n.scrollIntoView({block:'nearest'});
}
/* A new collection gets the first free number, and its name is written in place. */
function freeName(base){const used=new Set(Object.values(D).filter(d=>d.t==='coll'||d.t==='folder').map(d=>plain(d.n)));let n=1;while(used.has(`${base} (${n})`))n++;return `${base} (${n})`;}
/* Every entry of a menu carries its icon (2026-11-09). */
Object.assign(I,{trash:'<path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/>',copy:'<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V5a1 1 0 0 0-1-1H5a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h3"/>',
  sort:'<path d="M7 4v16M4 17l3 3 3-3M14 6h6M14 12h4M14 18h2"/>',win:'<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18"/>'});
const CTXI={copen:'eye',cpin:'pin',collhead:'x',open:'eye',addmenu2:'plus',newcoll:'grid',n2line:'hr',n2space:'gap',linefirst:'hr',lineafter:'hr',lineend:'hr',spacefirst:'gap',spaceafter:'gap',spaceend:'gap',
  cgear:'gear',opengear:'gear',decodel:'trash',opentab:'layout',opencol:'split',addchild:'plus',copylink:'link',sec:'eye',secclose:'x',secup:'up',secdown:'down',none:'pencil',view:'eye',
  selopen:'layout',selpin:'pin',selclear:'x',dupl:'copy',sortaz:'sort',sortza:'sort',close:'x',closeothers:'x',closeright:'x',closeleft:'x',closeall:'x',keep:'pin',dup:'copy',copypath:'link',
  newwin:'split',move:'split',closewin:'x',aid:'ai',split:'split',delobj:'trash',trash:'trash',export:'down',problems:'info'};
const mi=act=>icon(CTXI[act]||'more');
/* A right click opens the menu of what is under it, everywhere: tab, collection, list entry, section, view. */
/* Duplicate, where one may add next to it, and sort the children of anything that holds two or more. */
/* Turn a collection into a folder or back. A collection at the top, and the private notes, stay collections. */
/* Show a collection that lies inside another in nav2 too, or no longer. */
function showItem(id){
  if(!D[id]||D[id].t!=='coll'||C.some(c=>!c.alias&&c.id===id))return '';
  return aliasOf(id)?`<button type="button" role="menuitem" data-act="unshow" data-id="${id}">${icon('x')}Remove from nav2</button>`:`<button type="button" role="menuitem" data-act="show" data-id="${id}">${icon('pin')}Show in nav2</button>`;
}
function kindItem(id){
  const d=D[id];if(d.t!=='coll'&&d.t!=='folder')return '';if(d.real&&!d.db)return '';
  if(d.t==='coll'&&(C.some(c=>c.id===id&&!c.alias)||d.private))return '';
  const may=d.db?accOf(id)==='edit':true,to=d.t==='coll'?'folder':'collection';
  return `<button type="button" role="menuitem" data-act="tokind" data-id="${id}" data-k="${to}"${may?'':' disabled'}>${icon(to==='folder'?'folder':'grid')}Turn into a ${to}</button>`;
}
/* Delete sits last, after a line, and asks in place: the first click turns it into the question, the second deletes
   (no confirm(), as in C22). What lies in the database goes into the trash with everything below it; made-up content
   goes here and now. Whoever may change it may delete it, a collection at the top only its owner, and private notes stay. */
function delItem(id){
  const d=D[id];if(!d||isDeco(id))return '';
  const top=C.some(c=>c.id===id&&!c.alias),ask=!!(S.ctx&&S.ctx.ask);
  const why=d.real&&!d.db?'The folder source is read only':d.private||id==='me'?'Private notes stay':!d.db?'':top&&!d.mine?'Only its owner deletes a collection at the top':!d.mine&&accOf(id)!=='edit'?ACC[accOf(id)]:'';
  const u=ask&&S.ctx.uses,used=u&&(u.usedIn.length||u.hidden)?`<p class="text-muted-foreground w-60 px-2 py-1 text-xs whitespace-normal" data-uses="${id}">Embedded in ${[...u.usedIn,...(u.hidden?[u.hidden+(u.hidden===1?' page':' pages')+' you cannot see']:[])].join(', ')}. You choose what happens there.</p>`:'';
  return `<hr role="separator"><button type="button" role="menuitem" data-variant="destructive" data-act="delobj" data-id="${id}" aria-pressed="${ask}"${why?` disabled title="${why}"`:''}>${mi('delobj')}${ask?'Delete? Yes':'Delete'}</button>${used}`;
}
/* Before deleting, the question names where the object, or anything below it, is embedded (M-28). */
async function loadUses(id){
  const d=D[id];if(!d||!d.db)return;
  try{const r=await fetch('/api/objects/'+encodeURIComponent(d.sid)+'/uses',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    const a=await r.json();if(S.ctx&&S.ctx.ask&&S.ctx.id===id){S.ctx.uses={usedIn:a.usedIn.map(esc),hidden:a.hidden|0};render();}}
  catch(e){}
}
/* Export as files: a zip of the collection as normal files, what the viewer sees (docs/export.md). */
function exportItem(id){return D[id]&&D[id].db?`<button type="button" role="menuitem" data-act="export" data-id="${id}">${mi('export')}Export as files</button>`:'';}
function dupSort(id){
  const cont=contOf(id),may=cont?canPlace(cont):(D[id].db?S.canCreate:!D[id].real),kids=D[id].kids.filter(x=>!isDeco(x)).length,sorts=kids>1&&(D[id].db?accOf(id)==='edit':!D[id].real);
  return `<hr role="separator"><button type="button" role="menuitem" data-act="dupl" data-id="${id}"${may?'':' disabled'}>${mi('dupl')}Duplicate</button>`+(sorts?`<button type="button" role="menuitem" data-act="sortaz" data-id="${id}">${mi('sortaz')}Sort its children A to Z</button><button type="button" role="menuitem" data-act="sortza" data-id="${id}">${mi('sortza')}Sort its children Z to A</button>`:'');
}
function ctxHtml(){
  const c=S.ctx;if(!c)return '';
  const B=(act,label,id=c.id,extra='')=>`<button type="button" role="menuitem" data-act="${act}" data-id="${id}" ${extra}>${mi(act)}${label}</button>`;let items='',title='';
  if(c.kind==='coll'){title=D[c.id].n;items=B('copen','Open, close what is not kept open')+B('cpin',S.pin[c.id]?'Stop keeping it open':'Keep it open',c.id,'title="Also a double click"')+(S.open[c.id]?B('collhead','Close'):'')+B('open','Open its overview')+'<hr role="separator">'+B('addmenu2','New in it')+B('newcoll','New collection inside it')+(c.n3?divItems('first',c.id):'<hr role="separator">'+B('n2line','Line below it')+B('n2space','Space below it'))+showItem(c.id)+dupSort(c.id)+exportItem(c.id)+kindItem(c.id)+'<hr role="separator">'+B('cgear','Settings and name')+delItem(c.id);}
  if(c.kind==='deco'){title=D[c.id].n;items=B('decodel','Remove');}
  if(c.kind==='win'){const w=S.wins[0],older=olderOf(w);title='Window';items=tabMenuHtml(0,w.a)+(older.length?'<hr role="separator">'+older.map(x=>`<button type="button" role="menuitem" data-act="open" data-id="${x}">${icon(TYPE[D[x].t][0])}${D[x].n}</button>`).join(''):'')+(S.mods.ai?'<hr role="separator">'+B('aid','AI assistant','',off('tool','aid')):'');}
  if(c.kind==='sel'&&S.sel){const z=S.sel.z;title=S.sel.ids.length+' selected';items=(z==='n3'?B('selopen','Open as tabs'):z==='n2'?B('selopen','Open them in nav3')+B('selpin','Keep them open'):'')+'<hr role="separator">'+B('selclear','Clear the selection');}
  if(c.kind==='row'){const d=D[c.id];title=d.n;items=B('open','Open')+B('opentab','Open as a tab of its own')+(S.wins.length<4?B('opencol','Open in a new column',c.id,off('tool','opencol')):'')+'<hr role="separator">'+(COLL[c.id]?B('addchild','New page below it',c.id,madeHere(c.id)?'disabled title="'+madeHere(c.id)+'"':accOf(c.id)==='edit'?'':'disabled title="'+ACC[accOf(c.id)]+'"'):'')+divItems('after',c.id)+B('copylink','Copy link')+showItem(c.id)+dupSort(c.id)+kindItem(c.id)+B('opengear','Settings and metadata')+delItem(c.id);}
  if(c.kind==='row'&&isDeco(c.id)){title=D[c.id].n;items=B('decodel','Remove',c.id,canPlace(contOf(c.id))?'':'disabled title="'+placeWhy(contOf(c.id))+'"');}
  if(c.kind==='sec'){title=c.label;items=B('sec','Open')+(S.so[S.view]&&S.so[S.view][c.id]?B('secclose','Close'):'')+(c.sort?B('secup','Move up')+B('secdown','Move down'):'')+'<hr role="separator">'+B('none','Rename','','disabled title="Comes with editing"');}
  if(c.kind==='free'){title='New';items=c.acts.map(x=>{if(x[0]==='-')return '<hr role="separator">';const [act,id]=x[0].split(':');return B(act,x[1],id||'');}).join('');}
  if(c.kind==='view'){title=c.label;items=B('view','Open')+(S.view===c.id&&S.rail?B('view','Hide nav2 and nav3'):'');}
  return ctxMenu(title,`<div role="heading">${title}</div>${items}`,c.x,c.y);
}
/* A menu that opens where the pointer or its button is: C22's context menu, its panel fixed at that place (data-style,
   moved inside the window after a render). It hangs inside the workspace like every menu of it. */
const ctxMenu=(label,inner,x,y,attrs='')=>`<div class="context-menu"${attrs}><div data-cm-panel role="menu" aria-label="${esc(plain(label))}" class="max-h-full" data-style="left:${x}px;top:${y}px">${inner}</div></div>`;
/* An entry of nav1 or nav2 (C22 .workspace-item): what is shown or open carries aria-current; what is not built yet stays
   in sight, greyed out (aria-disabled). */
/* Kept open is data-pinned: the pack draws its line at the start thicker than that of an entry only open for now (a
   shape, not a colour), so the entry needs no pin of its own. A pinned section of a view keeps its pin. */
const pinMark=()=>icon('pin').replace('<svg ','<svg data-pin ');
function navItem(act,id,label,ico,on,extra='',no=false,title=''){const l=no||(act==='view'&&later('view',id))||(act==='sec'&&S.view==='profile'&&later('prof',id));return `<button type="button" class="workspace-item" data-act="${act}" data-id="${id}"${on?' aria-current="page"':''} title="${title||label}${l?', not in the first build':''}" ${extra}${l?' disabled aria-disabled="true" data-later="1"':''}>${icon(ico)}<span>${label}</span></button>`;}
const navLabel=(ico,label,title='')=>`<span class="workspace-label"${title?` title="${title}"`:''}>${icon(ico)}<span>${label}</span></span>`;
/* What nav3 shows: the open collections of nav2, in its order, those shown there from inside others among them. */
const openColls=()=>C.filter(c=>!isDeco(c.id)&&S.open[c.id]);
function nav2Html(){
  const s=phone()?1:S.s2;
  if(S.view==='workspace'){
    /* Every collection is in nav2, so an open one is always in sight: favorites on top, the others below. A right click has the rest. */
    /* Open for now or kept open: kept open draws the line on the left. Lines and spaces set groups apart; all of it is
       sorted by dragging, and the account keeps the order (the browser a copy). */
    const sl=c=>isSel('n2',c.id)?' aria-selected="true"':'';
    const row=c=>isDeco(c.id)?`<div data-deco="${D[c.id].t}" data-favid="${c.id}"${sl(c)} data-ctx="deco" data-ctxid="${c.id}" title="${D[c.id].n}: drag it, or right click to remove it">${decoInner(D[c.id].t)}</div>`
      :S.ren===c.id?`<div class="workspace-item">${icon(c.i||'grid')}<input class="input" data-size="sm" data-ren="${c.id}" value="${D[c.id].n}" aria-label="Name of the collection"></div>`
      :navItem('coll',c.id,c.n,c.i||D[c.id].i||'grid',!!S.open[c.id],`data-ctx="coll" data-ctxid="${c.id}" data-favid="${c.id}"${sl(c)}${c.alias?' data-alias="1" data-ref':''}${S.pin[c.id]?' data-pinned="1"':''}`,false,c.alias?`Lies in ${placeOf(c.id)}, shown here too`:'');
    return `<header><span>Collections</span>${newBtn(`data-act="addmenu" data-id="n2" aria-expanded="${S.addIn==='n2'}" aria-label="New in nav2" title="New: a collection, a line, a space"`)}</header><div id="clist">${C.map(row).join('')}</div>`;
  }
  if(S.view==='search') return '<div>'+[['exact','Exact words','quote'],['full','Full text','search'],['meaning','Similar meaning','spark']].map(x=>navItem('strat',x[0],x[1],x[2],S.strat===x[0])).join('')+'<hr>'+navItem('ai','ai','Also ask AI','ai',S.ai)+'</div>';
  if(S.view==='profile') return '<div>'+PROFSET.map(x=>navItem('sec',x[0],x[1],x[2],S.sec.profile===x[0])).join('')+'<hr>'+navLabel('shield','Administration')+PROFADM.map(x=>navItem('sec',x[0],x[1],x[2],S.sec.profile===x[0])).join('')+'</div>';
  /* Lists that belong to the user can be sorted by dragging. Entries that act, such as import, sit apart below them. */
  const all=SECTIONS[S.view](),srt=!!SORT[S.view],items=all.filter(x=>!x[3]),acts=all.filter(x=>x[3]);
  const so=S.so[S.view],sp=S.spin[S.view]||{};
  const one=x=>navItem('sec',x[0],x[1],x[2],so&&!x[3]?!!so[x[0]]:S.sec[S.view]===x[0],(srt&&!x[3]&&!x[4]?`data-secid="${x[0]}"${isSel('sec',x[0])?' aria-selected="true"':''}`:'')+(sp[x[0]]&&!x[3]?' data-pinned="1"':''));
  return '<div>'+items.map(one).join('')+(acts.length?'<hr>'+acts.map(one).join(''):'')+'</div>';
}
const SECTIONS={
  files:()=>FS.pending?[]:FS.on?[...FS.roots.map(r=>[r.id,esc(r.n),r.role==='user'?'user':r.role==='tenant'?'hd':'grid']),['newfcoll','New collection','plus',1],['ext','Add a source','plus',1]]
    :[['home','My files','user'],['share','Share','hd'],['ext','Add a source','plus',1]],
  todo:()=>[...TL.map(l=>[l.id,l.n,l.i]),['newlist','New list','plus',1]],
  cal:()=>[['all','All calendars','grid',0,1],...CALS.map(c=>[c.id,c.n,c.i]),['import','Import a calendar','down',1]],
  time:()=>timeReal('time')?[['tt','Time tracking','clock']]:[['week','This week','clock'],['last','Last week','clock'],['oct','October','cal'],['export','Export','down',1]],
  vac:()=>timeReal('vac')?[['tl','Leave planning','sun']]:[['y26','2026','sun'],['y27','2027','sun'],['plan','Plan a vacation','plus',1]],
  notes:()=>[...BOARDS.map(b=>[b.id,b.n,b.i]),['newboard','New board','plus',1]],
  resources:()=>RES.map(g=>[g.id,g.n,g.i]),
  /* The tab Rights (ADR-29) is drawn by rights.js. */
  rights:()=>window.UDRights?UDRights.SECTIONS:[],
  mail:()=>[['inbox','Inbox','mail'],['sent','Sent','send'],['archive','Archive','box']],
  bookmarks:()=>[...BM.map(f=>[f.id,f.n,f.i]),['import','Import','down',1]],
  news:()=>[...FEEDS.map(f=>[f.id,f.n,f.i]),['sub','Subscribe','plus',1]],
};
const docText=d=>[d.n,d.tags.join(' '),d.doc?d.doc.intro:'',d.doc?d.doc.secs.map(s=>s[0]+' '+plain(s[1])).join(' '):'',d.tab?d.tab.rows.flat().join(' '):'',d.html?plain(d.html):''].join(' ');
function hits(){
  const q=S.sq.trim();
  if(!q&&!S.ftags.length&&!S.ftype) return null;
  return Object.keys(D).filter(id=>{const d=D[id];if(id.includes(':')||d.gone)return false;
    if(S.ftype&&d.t!==S.ftype) return false;
    if(!S.ftags.every(t=>d.tags.includes(t))) return false;
    if(!q) return true;
    return S.strat==='exact'?docText(d).includes(q):docText(d).toLowerCase().includes(q.toLowerCase());});
}
/* The full-text search of the server finds pages that are not loaded yet; its hits come first. */
const hitsWithServer=()=>{const local=hits();const q=S.sq.trim();if(local===null||!q||S.apiQ!==q||!S.apiHits)return local;
  const real=S.apiHits.map(h=>SID[h.id]).filter(id=>D[id]&&!local.includes(id)&&(!S.ftype||D[id].t===S.ftype));return [...real,...local];};
let searchTimer=null;
function searchServer(){if(udSearch.real()){udSearch.results();return;}clearTimeout(searchTimer);const q=S.sq.trim();if(!q)return;searchTimer=setTimeout(async()=>{
  try{const r=await fetch('/api/search?q='+encodeURIComponent(q),{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)return;
    const hs=await r.json();if(S.sq.trim()!==q)return;S.apiQ=q;S.apiHits=hs;S.apiSnip={};hs.forEach(h=>{if(SID[h.id])S.apiSnip[SID[h.id]]=h.snippet;});refreshHits();}catch(e){}},180);}
function snippet(d,q){const t=docText(d),i=t.toLowerCase().indexOf(q.toLowerCase());if(!q||i<0)return '';const a=Math.max(0,i-28);return (a?'… ':'')+esc(t.slice(a,i))+'<mark>'+esc(t.slice(i,i+q.length))+'</mark>'+esc(t.slice(i+q.length,i+q.length+46))+' …';}
/* A hit of a search is C22's entry: its icon, the title, the place, the excerpt with the matches marked (data-snip). In a
   list of hits they stand in an .item-group. */
const hitItem=(attrs,ico,title,place,snip)=>`<a href="#" class="item" data-size="sm" role="listitem" ${attrs}><figure>${icon(ico)}</figure><section><h4>${title}</h4>${place?`<p>${place}</p>`:''}${snip?`<p data-snip>${snip}</p>`:''}</section></a>`;
const hitList=items=>`<div class="item-group" role="list">${items}</div>`;
/* A tag to filter by: a small outline button, pressed while it filters. */
const tagToggle=(attrs,t,on)=>`<button type="button" class="btn" data-variant="outline" data-size="xs" ${attrs} aria-pressed="${!!on}">#${t}</button>`;
function resultsHtml(){
  if(udSearch.real())return udSearch.resultsHtml();
  const h=hitsWithServer(),q=S.sq.trim();
  const allTags=[...new Set(Object.values(D).flatMap(d=>d.tags))].sort();
  if(h===null) return `${colLabel('Recent')}<ul class="workspace-tree">${['radio link','tower','on-call'].map(x=>`<li><div><span data-toggle aria-hidden="true"></span><button type="button" data-act="recent" data-id="${x}" title="${x}">${icon('search')}<span>${x}</span></button></div></li>`).join('')}</ul>
    <hr>${colLabel('Tags')}<div class="flex flex-wrap gap-1">${allTags.map(t=>`<a href="#" class="badge" data-variant="outline" data-act="ftag" data-tag="${t}">#${t}</a>`).join('')}</div>`;
  const answer=S.ai&&q&&h.length?`<div class="alert" data-answer><h2>AI answer, made up for this mockup</h2><section>A real answer would stand here in one or two sentences. Sources: ${h.slice(0,2).map(x=>D[x].n).join(', ')}.</section></div>`:'';
  const note=S.strat==='meaning'?hint('In this mockup, this finds the same hits as full text.'):'';
  return `${answer}${colLabel(`${h.length} ${h.length===1?'hit':'hits'}`)}${note}${h.length?hitList(h.map(x=>hitItem(`data-act="open" data-id="${x}"`,TYPE[D[x].t][0],D[x].n,placeOf(x),q?((S.apiSnip&&S.apiQ===q&&S.apiSnip[x])||snippet(D[x],q)):'')).join('')):hint('Nothing found. Another strategy may help.')}`;
}
/* nav3 of the search: Filter as a small button, its choices below it, then the results. */
function searchHtml(){
  if(udSearch.real())return udSearch.pageHtml();
  const allTags=[...new Set(Object.values(D).flatMap(d=>d.tags))].sort(),n=S.ftags.length+(S.ftype?1:0);
  return `<div class="flex flex-col gap-2"><div>${pbtn(`data-act="filters" aria-expanded="${S.filters}"`,'Filter'+(n?' ('+n+')':''),'view')}</div>
    ${S.filters?`<div class="flex flex-col gap-2" data-filters><select class="select" data-size="sm" id="stype" aria-label="Type"><option value="">All types</option>${Object.keys(TYPE).filter(k=>!['dir','res','mail'].includes(k)).map(k=>`<option value="${k}"${S.ftype===k?' selected':''}>${TYPE[k][1]}</option>`).join('')}</select><div class="flex flex-wrap gap-1">${allTags.map(t=>tagToggle(`data-act="ftag" data-tag="${t}"`,t,S.ftags.includes(t))).join('')}</div></div>`:''}
    <div id="sres" class="flex flex-col gap-2">${resultsHtml()}</div></div>`;
}
function sourceOf(d){
  if(d.blocks)return ['---',`title: ${d.n}`,'---',''].concat(...d.blocks.map(b=>[...b.raw.split('\n'),'']));
  const front=['---',`title: ${d.n}`,`width: ${d.w}`,`contents: ${d.toc}`,`tags: [${d.tags.join(', ')}]`,'---',''];
  const body=[d.doc.intro,''];
  d.doc.secs.forEach(s=>body.push('## '+s[0],'',plain(s[1]),''));
  return front.concat(body);
}
function statusHtml(d){
  let left;
  if(d.doc){const x=sourceOf(d),txt=plain(d.doc.intro)+' '+d.doc.secs.map(s=>s[0]+' '+plain(s[1])).join(' '),w=(txt.match(/\S+/g)||[]).length;
    /* On a phone the status line keeps to one row (C22's window there is narrow): the words and the format. */
    left=`<span>${w} words</span>`+(phone()?'':`<span>${txt.length} characters</span><span>${x.length} lines</span><span>${Math.max(1,Math.ceil(w/200))} min read</span>`);}
  else if(d.tab) left=`<span>${d.tab.rows.length} rows</span><span>${d.tab.head.length} columns</span>`;
  else if(d.t==='images') left='<span>6 images</span>';
  else if(d.html) left=`<span>${d.stat}</span>`;
  else left=`<span>${d.kids.length} children</span>`;
  return `<footer aria-label="Status of the document">${left}<span data-fill></span><span>${d.doc?'Markdown · UTF-8':TYPE[d.t][1]}</span></footer>`;
}
const EMBED_KINDS={part:'Part',section:'Section',block:'Block',page:'Page'};
/* The facts in the drawer: C22's drawer lists, a dl of names and values each; where something is used, a plain list. */
const factList=pairs=>pairs.length?`<dl>${pairs.map(([k,v])=>`<dt>${k}</dt><dd>${v}</dd>`).join('')}</dl>`:'';
function factsHtml(d){
  const u=d.uses||{},rows=(k,l)=>(u[k]||[]).map(x=>[l,x]);
  const uses=[...rows('ext','External'),...rows('int','Internal'),...rows('data','Data'),...rows('vals','Value'),...rows('db','Database'),...(u.emb||[]).map(e=>[e.l,e.x])];
  return `<div class="flex flex-col gap-2" data-facts><h3>Metadata</h3><dl><dt>Name</dt><dd>${d.n}</dd><dt>Type</dt><dd>${TYPE[d.t][1]}</dd>
    ${d.desc?`<dt>Description</dt><dd>${d.desc}</dd>`:''}<dt>In workspace</dt><dd>${placeOf(Object.keys(D).find(k=>D[k]===d))}</dd><dt>On disk</dt><dd>${d.path}</dd>
    ${d.by?`<dt>Created</dt><dd>${d.created||'Sep 12'} by ${d.cby||d.by}</dd><dt>Changed</dt><dd>${chg(d)}</dd>`:''}${d.db&&d.owner?`<dt>Owner</dt><dd>${d.owner}</dd>`:''}<dt>Visible to</dt><dd data-seen-by>${d.vis}</dd>${d.share?`<dt>On the share</dt><dd><code data-share-path>${esc(d.share.path)}</code>${(d.share.open_by||[]).length?`<br><small data-open-by>Open over Samba by ${esc(d.share.open_by.join(', '))}</small>`:''}</dd>`:''}</dl>${attachmentsHtml(d)}
    ${d.t!=='folder'&&!d.html?`<h3>Uses</h3>${uses.length?factList(uses):hint('Nothing yet.')}
    <h3>Used in</h3>${(d.usedIn||[]).length?`<ul class="text-sm">${d.usedIn.map(x=>`<li>${x}</li>`).join('')}</ul>`:hint('No other document.')}
    <h3>Versions</h3>${d.blocks?versionsHtml(d):factList([['now',d.ch],['earlier','12 versions, with restore']])}`:''}
    <h3>Later</h3>${hint('Export, save as (M-17) and share (M-5).')}</div>`;
}
/* The drawer of a window (C22's aside[data-drawer]): its head with an icon, the name and a close button, then its parts. */
const drawer=(label,head,inner,act,i)=>`<aside data-drawer aria-label="${label}"><header>${head}<button type="button" class="btn" data-variant="ghost" data-size="icon-sm" data-act="${act}" data-w="${i}" aria-label="Close ${label.toLowerCase()}">${icon('x')}</button></header>${inner}</aside>`;
/* The comments of a made-up document: what they belong to as the drawer's heading, the comments as C22's timeline, other
   commented elements as entries. */
function cmtHtml(t,i){
  if(D[t.d].blocks)return dbCmtHtml(t,i);
  const d=D[t.d],cs=d.comments||[],sel=t.cb,g={};cs.forEach(c=>(g[c.b]=g[c.b]||[]).push(c));
  const row=c=>`<li><header><b>${esc(c.who)}</b><time>${esc(c.when)}</time></header><p>${esc(c.t)}</p></li>`;
  const here=sel?`<h3 data-cfor><span>${esc(t.cbl||sel)}</span> <small>#${esc(sel)}</small></h3>${(g[sel]||[]).length?`<ol class="timeline" data-size="sm">${g[sel].map(row).join('')}</ol>`:hint('No comments on this yet.')}
    <div class="input-group"><input class="input" data-addcmt="${i}" placeholder="Write a comment, then Enter" aria-label="Write a comment"></div>`
    :hint('A comment belongs to an element of the page. Point at a paragraph, heading, list or table and click its speech bubble.');
  const rest=Object.keys(g).filter(k=>k!==sel).map(k=>cmtGo(i,k,g[k][0].l,String(g[k].length))).join('');
  return drawer('Comments',`${icon('chat')}Comments`,`${here}${rest?`<h3>${sel?'Elsewhere on this page':'On this page'}</h3><div class="item-group" role="list">${rest}</div>`:''}`,'cmt',i);
}
/* An element of the page with comments, to go to them: C22's entry with the count at its end. */
const cmtGo=(i,id,label,side)=>`<a href="#" class="item" data-size="sm" role="listitem" data-act="cblk" data-w="${i}" data-id="${esc(id)}" data-l="${esc(label)}"><figure>${icon('quote')}</figure><section><h4>${esc(label)}</h4></section><aside><small>${side}</small></aside></a>`;
function gearHtml(t,i,hasSecs){
  const d=D[t.d],no=d.db&&!mayTag(t.d)?` disabled title="Only who may change this sets it"`:'',seg=(act,opts,cur,label)=>segs(act,opts,cur,`data-w="${i}"`+(act==='setacc'?(isOwner(t.d)?'':' disabled title="Only the owner sets this"'):no),label);
  return drawer('Document settings','Document settings',`<h3>View</h3>
    ${hasSecs?prow('Contents',seg('settoc',['marker','side','hidden'],d.toc,'Contents')):''}
    ${prow('Width',seg('setw',['slim','wide','full'],d.w,'Width'))}
    ${prow('Page view',pbtn('disabled title="Comes later"','DIN A4, later'))}
    ${d.db&&d.blocks?prow('Template',seg('settpl',['off','offered'],d.tpl?'offered':'off','Template')):''}
    <h3>Others can</h3>${prow(d.acc?'Set here':'Inherited: '+ACC[othersOf(t.d)].toLowerCase(),seg('setacc',['inherit','edit','comment','read'],d.acc||'inherit','Others can'))}
    ${hint(d.db?'Both are kept with this document, for everyone who opens it.':'Both belong to this document, in its metadata. The source view shows them. Here a change lasts until you reload.')}
    ${d.db?prow('People, groups and links',pbtn(`data-act="rights" data-id="${t.d}"`,'Rights and sharing')):''}
    ${d.db&&d.blocks?prow('Passwords in the text',pbtn(`data-act="secrets" data-id="${t.d}"`,'Passwords')):''}
    ${tagsHtml(t.d,i)}
    ${factsHtml(d)}`,'gear',i);
}
/* Tags of an object of the database are kept in it: whoever may change it adds them here and takes them off with the x of
   a tag (M-28). Pages of the folder source take theirs from the front matter; the demo keeps its own until reload. */
const mayTag=id=>{const d=D[id];return d.db&&(d.canEdit!==undefined?d.canEdit:d.mine||accOf(id)==='edit');};
function tagsHtml(id,i){
  /* Each tag is C22's outline badge that searches for it; the x beside it takes it off. */
  const d=D[id],may=mayTag(id),chips=d.tags.map(t=>tagBadges([t],'tagsearch')+(may?`<button type="button" class="btn" data-variant="ghost" data-size="icon-xs" data-act="tagdel" data-w="${i}" data-tag="${t}" aria-label="Remove the tag ${t}" title="Remove the tag ${t}">${icon('x')}</button>`:'')).join('');
  const add=may||!d.db&&!d.real?`<input class="input" data-size="sm" data-addtag="${i}" placeholder="Add a tag, then Enter" aria-label="Add a tag" maxlength="40">`
    :d.db?hint('Only who may change this adds tags.'):hint('Tags of the folder source come from the front matter of the file.');
  return `<h3>Tags</h3><div class="flex flex-wrap items-center gap-1" data-tags>${chips||'<span class="text-muted-foreground text-xs">No tags yet.</span>'}</div>${add}`;
}
/* Width and contents of a document of the database are kept with it (ADR-16, M-8); made-up content keeps them until
   reload. If the server refuses, the view goes back. */
async function saveView(id,key,field,value){
  const d=D[id],before=d[key];d[key]=value;render();
  if(!d.db)return;
  try{const r=await fetch('/api/objects/'+encodeURIComponent(d.sid)+'/view',{...same,method:'PUT',body:JSON.stringify({[field]:value})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){d[key]=before;render();toast(e.message==='403'?'Only who may change this sets it.':'The view could not be saved.');}
}
/* Templates (M-8): UnderDoc's own and the pages offered as one, for New. A page is offered from its gear. */
async function loadTemplates(){
  try{const r=await fetch('/api/templates',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    S.tpls=(await r.json()).map(x=>({id:esc(x.id),n:esc(x.n),desc:esc(x.desc||''),place:esc(x.place||'')}));render();}
  catch(e){S.tpls=[];}
}
async function saveTemplate(id,on){
  const d=D[id],before=!!d.tpl;d.tpl=on;render();
  try{const r=await fetch('/api/objects/'+encodeURIComponent(d.sid)+'/template',{...same,method:'PUT',body:JSON.stringify({template:on})});if(!r.ok)throw new Error(String(r.status));loadTemplates();}
  catch(e){d.tpl=before;render();toast(e.message==='403'?'Only who may change this page offers it.':'The template could not be saved.');}
}
async function saveTags(id,tags){
  const d=D[id];
  try{const r=await fetch('/api/objects/'+encodeURIComponent(d.sid)+'/tags',{...same,method:'PUT',body:JSON.stringify({tags})});
    if(r.status===400){toast('A tag is 1 to 40 letters, digits, - _ or ., and an object has at most 50.');return;}
    if(!r.ok)throw new Error(String(r.status));
    d.tags=(await r.json()).tags.map(esc);render();}
  catch(e){toast(e.message==='403'?'Only who may change this adds tags.':'The tags could not be saved.');}
}
function tabHtml(x,k,i){
  const on=k===S.wins[i].a;
  /* The general tab only shows while it holds a document. Older ones lie below it as a stack, and its arrow lists them. */
  if(x.general){if(!x.d)return '';const d=D[x.d],w=S.wins[i],older=olderOf(w).length;
    return `<div class="workspace-tab" data-general${older?' data-stack':''} data-dw="${i}" data-dt="${k}" title="General tab: the next document replaces it. Drag it to keep it as a tab."><button type="button" role="tab" aria-selected="${on}" data-act="sel" data-w="${i}" data-t="${k}">${icon(TYPE[d.t][0])}<span>${d.n}</span></button>${older?`<button type="button" data-act="hist" data-w="${i}" aria-haspopup="menu" aria-expanded="${S.hist===i}" aria-label="Opened here before" title="Opened here before, up to 15">${icon('down')}</button>`:''}</div>`;}
  const d=D[x.d];
  return `<div class="workspace-tab" data-dw="${i}" data-dt="${k}"><button type="button" role="tab" aria-selected="${on}" data-act="sel" data-w="${i}" data-t="${k}" title="${d.n}">${icon(TYPE[d.t][0])}<span>${d.n}</span></button>
    <button type="button" data-act="close" data-w="${i}" data-t="${k}" title="Close" aria-label="Close ${d.n}">${icon('x')}</button></div>`;
}
/* Everything opens in the general tab. While a single window holds nothing else, no tab is drawn: the page stands
   alone, and a button beside New holds what the tab offers. A second tab or window brings the tabs back
   (the maintainer, 2026-11-09). */
const solo=()=>S.wins.length===1&&!S.wins[0].tabs.some(x=>!x.general);
/* A window is C22's .workspace-window inside .workspace-windows: its own tab row, then the window (.workspace-main) with
   its head, the document and the status line. With several windows the one worked in carries data-focus. */
/* What the general tab offers while a single window has no tabs (solo): beside New in the top row; on a phone, where the
   top row holds only New, in the tab row of the window. */
const winBtn=v=>`<button type="button" class="btn" data-variant="${v}" data-size="icon-sm" data-act="winmenu" aria-expanded="${!!S.ctx&&S.ctx.kind==='win'}" aria-label="Window and tabs" title="Window and tabs: keep as a tab, a new column, opened before">${icon('win')}</button>`;
const ghostBtn=(attrs,ico)=>`<button type="button" class="btn" data-variant="ghost" data-size="icon-sm" ${attrs}>${icon(ico)}</button>`;
function winHtml(w,i){
  const t=at(i),has=t&&t.d;
  /* Solo: no tab row, the window takes its place (C22). On a phone the row stays for the button beside New, which the
     narrow top row there has no room for. */
  const head=solo()?(phone()?`<div class="workspace-tabs">${winBtn('ghost')}</div>`:''):`<div class="workspace-tabs"><div role="tablist" aria-label="Open in window ${i+1}">${w.tabs.map((x,k)=>tabHtml(x,k,i)).join('')}</div>
    ${ghostBtn(`data-act="tablist" data-w="${i}" aria-expanded="${S.tlist===i}" aria-label="All tabs of this window" title="All tabs of this window"`,'list')}${S.wins.length>1?ghostBtn(`data-act="closewin" data-w="${i}" aria-label="Close window" title="Close window"`,'x'):''}${i===S.wins.length-1&&S.mods.ai?ghostBtn(`${off('tool','aid')} data-act="aid" aria-pressed="${S.aid}" aria-label="AI assistant" title="AI assistant"`,'ai'):''}</div>`;
  /* The menus of the tab row open at their tab or button (S.menuAt), as C22's context menus. */
  const at0=S.menuAt||{x:0,y:0},pop=(S.newAt===i?newMenuHtml('tab'):'')+(S.tmenu&&S.tmenu.w===i?ctxMenu('Tab',tabMenuHtml(i,S.tmenu.t),S.tmenu.x,S.tmenu.y,' data-menu="tab"'):'')
    +(S.hist===i?ctxMenu('Opened here before',`<div role="heading">Opened here before</div>${olderOf(w).map(x=>`<button type="button" role="menuitem" data-act="open" data-id="${x}">${icon(TYPE[D[x].t][0])}${D[x].n}</button>`).join('')||'<div role="heading">Nothing yet.</div>'}`,at0.x,at0.y,' data-menu="hist"'):'')
    +(S.tlist===i?ctxMenu('Tabs of this window',`<div role="heading">Tabs of this window</div>${w.tabs.map((x,k)=>`<button type="button" role="menuitemradio" data-act="sel" data-w="${i}" data-t="${k}" aria-checked="${k===w.a}">${x.d?icon(TYPE[D[x.d].t][0])+D[x.d].n:'<em>empty general tab</em>'}</button>`).join('')}`,at0.x,at0.y,' data-menu="tabs"'):'');
  const ai=i===S.wins.length-1&&S.aid&&S.mods.ai?aiHtml():'';
  /* C22's drop zones of a window, hidden until a drag points at one. */
  const open=`<section class="workspace-window" data-win="${i}"${S.wins.length>1&&i===S.focus?' data-focus':''} aria-label="Window ${i+1}">`;
  if(!has) return `${open}${head}${pop}<main class="workspace-main"><div data-drop-zone="start" hidden></div><div data-drop-zone="end" hidden></div><div data-drop-zone="here" hidden></div><section data-sc="${i}"><p class="text-muted-foreground">Empty.<br>What a list opens lands here, in the active window.</p></section>${ai}</main></section>`;
  const d=D[t.d],{h,hs,to}=bodyHtml(t,i),mode=d.toc,ids=hs.map((x,j)=>to?to[j]:`w${i}h${j}`);
  /* On a phone the window is narrow: the tools take C22's small size and the sections only their icon, so the head
     keeps one row of tools and the page its height. */
  const ph=phone(),toolBtn=(attrs,ico)=>`<button type="button" class="btn" data-variant="ghost" data-size="${ph?'icon-xs':'icon-sm'}" ${attrs}>${icon(ico)}</button>`;
  /* The sections of the document: a menu under a small button in the tools, or a contents column beside the document. */
  const marker=hs.length&&mode==='marker'&&!t.src?`<div class="dropdown-menu"><button type="button" class="btn" data-variant="${ph?'ghost" data-size="icon-xs" aria-label="'+hs.length+' sections':'outline" data-size="xs'}" data-act="fly" data-w="${i}" aria-haspopup="menu" aria-expanded="${t.fly}" title="In this document">${icon('list')}${ph?'':`<span>${hs.length} sections</span>`}</button><div data-popover data-align="end" aria-hidden="${!t.fly}"><div role="menu" aria-label="In this document"><div role="group"><div role="heading">In this document</div>${hs.map((x,j)=>`<button type="button" role="menuitem" data-act="jump" data-w="${i}" data-id="${ids[j]}">${x}</button>`).join('')}</div></div></div></div>`:'';
  const toc=hs.length&&mode==='side'&&!t.src?`<nav class="toc" data-variant="compact" data-marker="bar" aria-label="Contents">${hs.map((x,j)=>`<a href="#${ids[j]}" data-act="jump" data-w="${i}" data-id="${ids[j]}">${x}</a>`).join('')}</nav>`:'';
  const find=t.find?`<div class="input-group"><span data-align="inline-start" aria-hidden="true">${icon('search')}</span><input class="input" data-findq="${i}" type="search" value="${esc(t.q)}" placeholder="Find on this page" aria-label="Find on this page"><span data-align="inline-end" data-findc="${i}"></span><button type="button" class="btn" data-variant="ghost" data-size="icon-xs" data-align="inline-end" data-act="findnav" data-w="${i}" data-id="-1" aria-label="Previous hit" title="Previous hit">${icon('up')}</button><button type="button" class="btn" data-variant="ghost" data-size="icon-xs" data-align="inline-end" data-act="findnav" data-w="${i}" data-id="1" aria-label="Next hit" title="Next hit">${icon('down')}</button></div>`:'';
  /* The heading is not content. The path sits above the title in the head, the tags and what others may do to its right,
     the tools at the end. Created, changed and the rest are metadata. */
  const crumbs=placeOf(t.d),parts=String(crumbs).split(' › ');
  const place=`<nav class="breadcrumb" aria-label="Path" title="${crumbs}"><ol>${parts.map((x,k)=>(k?`<li aria-hidden="true">${icon('chev')}</li>`:'')+`<li><span>${x}</span></li>`).join('')}</ol></nav>`;
  const ac=othersOf(t.d),badge=ac==='edit'?'':`<span class="badge" data-variant="secondary" title="What others may do here">${icon(ac==='read'?'lock':'quote')}${ACC[ac]}</span>`;
  /* Reading or editing, as a large button left of the title: the state in words and icon, a click turns it (2026-11-09). */
  const ed=editing(t)&&d.blocks&&d.canEdit,mode2=a=>`<button type="button" class="btn" data-mode data-variant="outline" data-act="edit" data-w="${i}" ${a}>`;
  const status=d.blocks&&d.canEdit?`${mode2(`aria-pressed="${!!ed}" aria-label="${ed?'Editing':'Reading'}" title="${ed?'Editing. Click to read':'Reading. Click to edit'}"`)}${icon(ed?'pencil':'book')}</button>`
    :d.blocks||d.doc?`${mode2(`disabled aria-label="Read only" title="Read only. ${noEdit(t.d)}"`)}${icon('lock')}</button>`:'';
  const n=cmtCount(d);
  /* Tags and what others may do stand right of the path. */
  const marks=tagBadges(d.tags,'tagsearch')+badge;
  const tools=`<div role="toolbar" aria-label="Tools">${marker}${find}
    ${toolBtn(`data-act="find" data-w="${i}" aria-pressed="${t.find}" aria-label="Find on this page" title="Find on this page"`,'search')}
    ${d.doc?toolBtn(`data-act="src" data-w="${i}" aria-pressed="${t.src}" aria-label="Source view" title="Source view"`,'code'):''}
    ${S.wins.length<4?toolBtn(`data-act="split"${off('tool','split')} data-w="${i}" aria-label="Show also in a new column" title="Show also in a new column"`,'split'):''}
    ${cmtShown(t.d)?toolBtn(`data-act="cmt" data-w="${i}" aria-pressed="${t.cmt}" aria-label="Comments" title="Comments${n?', '+n:''}"${n?` data-count="${n}"`:''}`,'chat'):fromSource(t.d)?toolBtn(`disabled aria-label="Comments" title="No comments here: this page comes from the folder source, read only. Comments are kept on pages of the database."`,'chat'):''}
    ${toolBtn(`data-act="gear" data-w="${i}" aria-pressed="${t.gear}" aria-label="Document settings" title="Document settings"`,'gear')}
    <div class="dropdown-menu">${toolBtn(`data-act="menu" data-w="${i}" aria-haspopup="menu" aria-expanded="${S.menu===i}" aria-label="More" title="More"`,'more')}${S.menu===i?menuHtml(i):''}</div></div>`;
  /* The document column is C22's article.typeset with its width; the blocks of a page are its div[data-block] wrappers,
     and C22 keeps room for their handles beside it. A view (a list of to-dos, the month, a board, a saved search) stands
     straight in the window's section, as C22's views do. */
  const whead=`<header>${status}<div>${place}${marks}<span data-fill></span>${tools}</div><h1>${d.n}</h1>${d.desc?`<p>${d.desc}</p>`:''}</header>`;
  const inner=t.src?`<pre class="code-block" data-line-numbers data-art="${i}"><code>${sourceOf(d).map(l=>`<span>${esc(l)}</span>`).join('')}</code></pre>`:d.view?h:`<article class="typeset" data-width="${d.w}" data-art="${i}">${h}</article>`;
  return `${open}${head}${pop}<main class="workspace-main">${whead}<div data-drop-zone="start" hidden></div><div data-drop-zone="end" hidden></div><div data-drop-zone="here" hidden></div>${ed&&!t.src?edBarHtml(t.d,i):''}
    <section data-sc="${i}">${toc}${inner}</section>${t.gear?gearHtml(t,i,hs.length>0):''}${t.cmt&&cmtShown(t.d)?cmtHtml(t,i):''}${ai}${statusHtml(d)}</main></section>`;
}
/* More in the tools of a window: C22's dropdown menu under its button, drawn open by this script. */
function menuHtml(i){return `<div data-popover data-align="end" aria-hidden="false"><div role="menu" aria-label="More">${tabMenuHtml(i,S.wins[i].a)}</div></div>`;}
/* One menu for a tab, from a right click on it or from the dots: the same choices in both places. */
function tabMenuHtml(i,k){
  const w=S.wins[i],t=w.tabs[k],n=w.tabs.length,has=!!t.d,room=S.wins.length<4;
  const B=(act,label,extra='')=>`<button type="button" role="menuitem" data-act="${act}" data-w="${i}" data-t="${k}" ${extra}>${mi(act)}${label}</button>`;
  return [has?B('close',t.general?'Clear':'Close','title="Also a middle click"'):'',n>1?B('closeothers','Close others'):'',k<n-1?B('closeright','Close to the right'):'',
    k>0?B('closeleft','Close to the left'):'',B('closeall','Close all'),
    has?'<hr role="separator">'+(t.general?B('keep','Keep as a tab of its own'):B('dup','Duplicate'))+B('copypath','Copy path'):'',
    has&&(room||S.wins.length>1)?'<hr role="separator">':'',has&&room?B('newwin','Move to new column, left',`data-id="${i}"${off('tool','newwin')}`)+B('newwin','Move to new column, right',`data-id="${i+1}"${off('tool','newwin')}`):'',
    has&&i>0?B('move','Move to the window on the left',`data-id="${i-1}"`):'',has&&i<S.wins.length-1?B('move','Move to the window on the right',`data-id="${i+1}"`):'',
    S.wins.length>1?'<hr role="separator">'+B('closewin','Close window'):''].join('');
}
function closeWhere(i,keep){
  const w=S.wins[i],cur=w.tabs[w.a];
  if(w.tabs[0]&&w.tabs[0].general&&!keep(0))Object.assign(w.tabs[0],newTab(null,true));
  w.tabs=w.tabs.filter((t,j)=>t.general||keep(j));
  const c=w.tabs.indexOf(cur);w.a=c>=0?c:Math.max(0,w.tabs.length-1);compact();
}
/* New always lands somewhere: in the chosen collection, or in Quick notes. The last choice stays. */
function newMenuHtml(where){
  const into=S.into,q=into==='qn',caps=t=>`<h3 class="text-muted-foreground text-xs font-semibold tracking-wider uppercase">${t}</h3>`;
  /* C22's New of the top row: where it goes as segments, a field, a hint, templates and types as entries of a grid. */
  const body=`<div class="flex items-center gap-2 text-sm"><span class="text-muted-foreground">Into</span><div class="button-group flex-wrap" role="group" aria-label="Where it goes">${[['qn','Quick notes','note'],...C.filter(c=>!isDeco(c.id)).map(c=>[c.id,c.n,c.i])].map(x=>`<button type="button" class="btn" data-variant="outline" data-size="xs" data-act="into" data-id="${x[0]}" aria-pressed="${into===x[0]}"${x[0]==='qn'?off('tool','qn'):''}>${icon(x[2])}${x[1]}</button>`).join('')}</div></div>
    <input class="input" data-quick placeholder="${q?'Just type, Enter keeps it as a note':'Just type, Enter makes a page'}" aria-label="New">
    <p class="text-muted-foreground text-sm">${q?'A card in Quick notes. It can move to a collection later.':'A Markdown page at the top of '+C.find(c=>c.id===into).n+'. Drag it to its place.'}</p>
    ${!q&&D[into]&&D[into].db&&S.tpls.length?`${caps('From a template')}<div class="grid grid-cols-2 gap-2">${S.tpls.map(x=>`<a href="#" class="item" data-variant="outline" data-size="sm" data-act="newtpl" data-id="${x.id}"><figure>${icon('page')}</figure><section><h4 class="font-semibold">${esc(x.n)}</h4><p class="text-muted-foreground line-clamp-2 text-xs">${esc(x.desc||x.place)}</p></section></a>`).join('')}</div>`:''}
    ${q?'<p class="text-muted-foreground text-sm">Pick a collection for other types.</p>':`${caps('Or start as')}<div class="grid grid-cols-2 gap-2">${addTypes().map(k=>{const l=later('add',k);return `<a${l?' aria-disabled="true" data-later="1" title="Comes later"':` href="#" data-act="newdoc" data-id="${k}"`} class="item" data-size="sm"><figure>${icon(TYPE[k][0])}</figure><section><h4>${k==='page'?'Page, Markdown':TYPE[k][1]}</h4></section></a>`;}).join('')}</div>`}`;
  /* In the top row it is the popover of the New button; from the tab row of a window a card at the place of the double
     click. */
  if(where==='bar')return `<div data-popover aria-hidden="false" class="w-80" data-menu="quick" role="dialog" aria-label="New"><div class="flex flex-col gap-3">${body}</div></div>`;
  const p=S.newPos||{x:16,y:16};
  return `<section class="card fixed z-50 w-80 gap-3 py-3 shadow-lg" data-menu="quick" role="dialog" aria-label="New" data-style="left:${p.x}px;top:${p.y}px"><div class="flex flex-col gap-3 px-3">${body}</div></section>`;
}
function createIn(cid,k,text){
  const id='new'+(++S.newN),first=(text||'').split('\n')[0].trim();
  D[id]={t:k,n:first?first.slice(0,48):'New '+TYPE[k][1].toLowerCase(),w:'slim',toc:'marker',tags:[],kids:[],path:'not saved, this is a mockup',by:'you',ch:'now',vis:'As its collection',...none,stat:'new',
    html:text?`<p>${esc(text)}</p>`:'<p class="text-muted-foreground text-sm">Empty. Drag it in the list to where it belongs.</p>'};
  C.find(c=>c.id===cid).top.unshift(id);S.open[cid]=true;rebuildPlaces();S.just=id;return id;
}
/* Files and Resources are contexts of the left list. What they open lands in the same windows as everything else. */
TYPE.dir=['folder','Folder on disk'];TYPE.res=['db','Resource'];
const FILEROWS={'~':[['scans/','folder','–','Oct 12',''],['notes-2026.md','page','2 KB','today',''],['travel-expenses.xlsx','excel','40 KB','Oct 30','']],
  '~/scans':[['receipt-oct.pdf','page','220 KB','Oct 12','']],
  '':[['operations/','folder','–','Oct 31','Operations'],['projects/','folder','–','today','Projects']],
  operations:[['network/','folder','–','Oct 28','Network'],['on-call.md','page','4 KB','Oct 29','On-call handbook'],['site-logs/','folder','–','Oct 31','Site logs']],
  'operations/network':[['plan.md','page','6 KB','today','Network plan'],['plan/','folder','–','Oct 28','Network plan (children)'],['rack-photo.jpg','images','2.1 MB','Sep 11',''],['walkthrough.mp4','video','48 MB','Oct 12','Site walkthrough']],
  'operations/network/plan':[['vlans.md','page','1 KB','Oct 28','VLAN list']],
  projects:[['tower.md','page','3 KB','Oct 31','Tower 50 m'],['tower/','folder','–','today','Tower 50 m (children)'],['old-offer.pdf','page','410 KB','Mar 04','']],
  'projects/tower':[['tower-dimensions.xlsx','excel','88 KB','today','Tower dimensions'],['photos/','images','34 MB','Oct 18','Site photos'],['kickoff.pptx','slides','6 MB','Sep 30','Kickoff slides'],['~$tower-dimensions.xlsx','excel','hidden','today','hidden: lock file']]};
const FILEDOC={'on-call.md':'hand','plan.md':'plan','tower.md':'tower','tower-dimensions.xlsx':'dims','photos/':'photos','site-logs/':'logs','vlans.md':'vlans','walkthrough.mp4':'walk','kickoff.pptx':'deck'};
const FILETAGS={'network/':'network','on-call.md':'on-call','plan.md':'network','rack-photo.jpg':'hardware','tower.md':'tower','tower-dimensions.xlsx':'tower','photos/':'tower'};
const subOf=(dir,x)=>(dir?dir+'/':'')+x[0].replace(/\/$/,'');
const isDir=(dir,x)=>x[0].endsWith('/')&&!!FILEROWS[subOf(dir,x)];
const fileTarget=(dir,x)=>isDir(dir,x)?'dir:'+subOf(dir,x):FILEDOC[x[0]]||'file:'+subOf(dir,x);
const pseudo=(id,o)=>{D[id]={w:'wide',toc:'marker',tags:[],kids:[],uses:{},usedIn:[],...o};};
const openBtn=(id,label,icon)=>`<a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="${id}">${icon?ic(icon)+' ':''}${label}</a>`;
Object.keys(FILEROWS).forEach(dir=>{
  const rows=FILEROWS[dir].map(x=>{const hid=x[2]==='hidden',r=[hid?x[0]:openBtn(fileTarget(dir,x),x[0]),x[2],x[3],x[4]?x[4]:'<span class="text-destructive">△ not placed</span>',FILETAGS[x[0]]?'#'+FILETAGS[x[0]]:''];r.dim=hid;return r;});
  pseudo('dir:'+dir,{t:'dir',n:dir==='~'?'My files':dir===''?'Share':dir.split('/').pop()+'/',path:dir==='~'?'your own folder':dir===''?'the share':dir+'/',place:'Files › '+(dir.startsWith('~')?dir.replace('~','My files'):'Share'+(dir?' › '+dir:'')),vis:dir.startsWith('~')?'Only you':'Follows the disk or share',stat:`${FILEROWS[dir].length} entries`,
    html:`<div class="alert"><section>Moving something here does not move it in your workspace, and the reverse holds too. Lock files stay hidden in normal use. Upload and share access come with M-2.</section></div>`+tableHtml(['Name','Size','Changed','In your workspace','Tags'],rows)});
  FILEROWS[dir].forEach(x=>{if(x[4]||x[2]==='hidden'||isDir(dir,x))return;pseudo('file:'+subOf(dir,x),{t:'dir',n:x[0],path:subOf(dir,x),place:'Files › '+dir,vis:'Follows the disk or share',stat:x[2],tags:FILETAGS[x[0]]?[FILETAGS[x[0]]]:[],html:'<div class="alert"><section>This file lies on the disk but has no place in your workspace yet.</section></div>'});});
});
TYPE.mail=['mail','Mail'];
const MAILS=[{id:5,box:'sent',from:'To J. Roth',date:'Oct 27',unread:false,subj:'Antenna swap',body:'Done, noted in the handbook.'},{id:6,box:'archive',from:'Supplier desk',date:'Sep 02',unread:false,subj:'Old offer',body:'Thanks for your interest.'},{id:1,box:'inbox',from:'J. Roth',date:'09:12',unread:true,subj:'Crane on Tuesday',body:'The crane company confirmed Tuesday at seven. Can you check the tower dimensions before then?'},
 {id:2,box:'inbox',from:'Supplier desk',date:'Mon',unread:true,subj:'Your switch license',body:'The license for your core switches runs out at the end of the month.'},
 {id:3,box:'inbox',from:'M. Sander',date:'Oct 30',unread:false,subj:'On-call handbook',body:'Please look at the new escalation levels in the handbook.'},
 {id:4,box:'inbox',from:'A. Keller',date:'Oct 28',unread:false,subj:'Rack photo',body:'New photo is on the share, but not placed yet.'}];
MAILS.forEach(m=>pseudo('mail:'+m.id,{t:'mail',n:m.subj,w:'slim',path:'not a file, from the mail account',place:'Mail › '+m.box,vis:'Only you',stat:'1 message',
  html:`<dl><dt>From</dt><dd>${m.from}</dd><dt>To</dt><dd>you</dd><dt>Date</dt><dd>${m.date}</dd></dl><p>${m.body}</p><div class="alert"><section>Reply, and linking a mail to a document, are to be settled in M-22.</section></div>`}));
function diskRow(dir,x){
  const id=fileTarget(dir,x),folder=isDir(dir,x),sub=subOf(dir,x),t=at(S.focus),cur=t&&t.d===id,open=kidsOpen(id);
  const kids=folder?FILEROWS[sub].filter(y=>y[2]!=='hidden'):[];
  return `<li><div>${toggleHtml(id,x[0],folder,open)}<button type="button" data-act="open" data-id="${id}" title="${x[0]}"${cur?' aria-current="page"':''}>${icon(TYPE[x[1]][0])}<span>${x[0]}</span></button>${!x[4]&&dir?'<small title="Not placed in the workspace">△</small>':''}</div>
    ${folder&&open?`<ul>${kids.map(y=>diskRow(sub,y)).join('')}</ul>`:''}</li>`;
}
const RES=[
 {id:'values',n:'Values',m:'M-3',i:'list',head:['Name','Value','Source','Used in'],rows:[['tower.height','50 m','tower-dimensions.xlsx','4 documents'],['router.ip','192.0.2.1','_values.yaml','2 documents']]},
 {id:'snippets',n:'Snippets',m:'M-3',i:'code',head:['Name','Kind','Used in'],rows:[['customers','SQL','3 documents'],['maintenance-note','Text','7 documents']]},
 {id:'dbs',n:'Databases and views',m:'M-4',i:'db',head:['Name','Type','Placed in workspace','Used in','Tags'],rows:[['Inventory','Database','Projects','1 view','#hardware'],['Open tickets','Database view','Operations › Network','1 document','#network'],['Weekly export','Database view','△ not placed','0 documents','']]},
 {id:'ext',n:'External sources',m:'M-4',i:'box',head:['Name','Kind','Data age','State'],rows:[['Ticket system','REST','4 min','✓ working'],['Supplier list','CSV','2 d','△ last good state']]}];
const RESDOC={Inventory:'inv','Open tickets':'tickets'};
/* A row of a real group carries its own key and a link to where it lives. */
const resTarget=(g,r)=>r.key||RESDOC[r[0]]||`res:${g.id}/${r[0]}`;
function resPseudos(g){
  const note=g.real?g.note:`<div class="alert"><section>What has no place in a folder. Comes with ${g.m}.</section></div>`;
  pseudo('res:'+g.id,{t:'res',n:g.n,path:'not a file',place:'Resources',vis:g.real?'What you may read':'Everyone with access to this workspace',stat:`${g.rows.length} entries`,
    html:note+tableHtml(g.head,g.rows.map(r=>[openBtn(resTarget(g,r),r[0]),...r.slice(1)]))});
  g.rows.forEach(r=>{if(!RESDOC[r[0]])pseudo(resTarget(g,r),{t:'res',n:r[0],path:'not a file',place:'Resources › '+g.n,vis:g.real?'What you may read':'Everyone with access to this workspace',stat:g.n,
    html:tableHtml(['Field','Content'],g.head.map((h,k)=>[h,r[k]]))+(r.link?`<p>Lives in ${r.link}</p>`:'')});});
}
RES.forEach(resPseudos);
/* With a database, values and snippets are real (M-3): what the viewer may read, from /api/values and /api/snippets.
   Databases and external sources stay the mock-up of the first build until M-4, and say so. */
const SNIPKIND={part:'Part',section:'Section',block:'Block'};
async function loadResources(){
  const get=async u=>{const r=await fetch(u,{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));return r.json();};
  try{
    const [vals,snips]=await Promise.all([get('/api/values'),get('/api/snippets')]);
    const used=n=>n?(n===1?'1 place':n+' places'):'not used yet',link=x=>`<a href="${esc(x.url)}">${esc(x.title)}</a>`;
    const real=(id,head,note,list,cells)=>{const g=RES.find(x=>x.id===id);g.real=true;g.head=head;g.note=`<div class="alert"><section>${note}</section></div>`;
      g.rows=list.map((x,i)=>{const r=cells(x);r.key=`res:${id}/${i}`;r.link=link(x);return r;});resPseudos(g);};
    real('values',['Name','Value','Page','Used in'],'Parts with a name. Mark text in a page and name it to make one.',vals,x=>[esc(x.name),esc(x.text),esc(x.title),used(x.uses)]);
    real('snippets',['Text','Kind','Page','Used in'],'Parts, sections and blocks that other pages embed.',snips,x=>[esc(x.text)||'#'+esc(x.ref.split('#')[1]),SNIPKIND[x.kind]||'',esc(x.title),used(x.uses)]);
    RES.filter(g=>!g.real&&!g.n.endsWith(' (demo)')).forEach(g=>{g.n+=' (demo)';resPseudos(g);});
    render();
  }catch(e){/* the mock-up stays */}
}
/* A window goes when only its empty general tab is left, unless it is the last window. */
function compact(){
  const f=S.wins[S.focus],gone=w=>w.tabs.length<=1&&!(w.tabs[0]&&w.tabs[0].d);
  S.wins=S.wins.filter(w=>!gone(w));if(!S.wins.length)S.wins=[{tabs:[newTab(null,true)],a:0,hist:[]}];
  S.wins.forEach(w=>{if(w.a>=w.tabs.length)w.a=w.tabs.length-1;if(w.a===0&&!w.tabs[0].d&&w.tabs.length>1)w.a=1;});
  S.focus=Math.max(0,S.wins.indexOf(f));
}
const MODS=[['mail','Mail','An entry in nav1'],['private','Private area','Entries at the bottom of nav1'],['bookmarks','Bookmarks','A document type, with an import'],['news','News','An entry in nav1'],['ai','AI assistant','A drawer on the right'],['network','Network','IP ranges and device lists, as document types'],['lists','To-do and Kanban','Two document types'],['video','Video','A document type'],['slides','PowerPoint','A document type'],['notes','Quick notes','A document type']];
const MODTYPE={bookmark:'bookmarks',todo:'lists',kanban:'lists',video:'video',slides:'slides',notes:'notes',ipnet:'network',devices:'network'};
const shown=id=>{const m=MODTYPE[D[id].t];return !m||S.mods[m];};
const STAGES=[50,84,116,172],STAGENAMES=[['0','Icon'],['1','Stacked'],['2','Text'],['3','Beside']];
const seg3=(act,opts,cur)=>segs(act,opts,cur);
const plink=(href,label)=>`<a class="btn" data-variant="outline" data-size="xs" href="${href}">${label}</a>`;
/* An entry of a panel list (C22's framed small .item): a name, a quiet line below it, a mark at its end. */
const pitem=(tag,attrs,name,sub,side)=>`<${tag} class="item" data-variant="outline" data-size="sm" role="listitem" ${attrs}><section><h4>${name}</h4>${sub?`<p class="text-muted-foreground text-xs">${sub}</p>`:''}</section>${side?`<aside>${side}</aside>`:''}</${tag}>`;
const onOff=on=>`<span class="badge" data-variant="${on?'secondary':'outline'}">${on?'✓ On':'✗ Off'}</span>`;
const plabel=t=>`<h4 class="text-sm font-medium">${t}</h4>`;
function settingsHtml(sec){
  let b='';
  if(sec==='look') b=`${ptitle('Appearance')}${prow('Theme',seg3('theme',[['system','System'],['light','Light'],['dark','Dark']],S.theme))}
    ${prow('Page scale',`<div class="button-group" role="group" aria-label="Page scale">${pbtn('data-act="scale" data-id="-10" aria-label="Smaller"','−')}${pbtn('data-act="scale" data-id="0" title="Back to 100%"',S.scale+'%')}${pbtn('data-act="scale" data-id="10" aria-label="Larger"','+')}</div>`)}`;
  if(sec==='navi') b=`${ptitle('Navigation')}
    ${plabel('Clicks')}${hint('A click opens only this. Ctrl and click selects it too, Shift and click a range: dragging one moves them all, and on a window they open as tabs. A double click keeps a collection open, or no longer; a middle click opens a document as a tab of its own. A click on a collection closes the others unless they are kept open. Anything with children also opens them in nav3, and a second click closes them.')}
    ${plabel('Lists')}<label class="flex flex-wrap items-center justify-between gap-3" for="limit"><span class="text-sm">Open by themselves up to</span><div class="input-group w-32"><input class="input" id="limit" type="number" min="0" max="50" value="${S.limit}"><span data-align="inline-end">children</span></div></label>
    ${plabel('Width, or drag the right edge of each column')}
    ${prow('Nav1',seg3('s1',STAGENAMES,S.s1))}${prow('Nav2',seg3('s2',STAGENAMES,S.s2))}
    ${prow('Nav3',pbtn('data-act="w3reset" title="Back to the default"',`${S.w3} px, reset`))}`;
  if(sec==='docs') b=`${ptitle('Documents')}${prow('Open documents for',seg3('openedit',[['false','Reading'],['true','Editing']],!!S.openEdit))}
    ${hint('The button left of the title turns a document between reading and editing; this says how it opens. Only pages you may change open for editing.')}`;
  if(sec==='lang') b=ptitle('Language')+hint('English for now. German comes as a translation of the interface.');
  if(sec==='notif') b=ptitle('Notifications')+hint('Mail for mentions and replies comes with M-16.');
  return panel(`${b}${hint('These belong to you. With the database your account keeps them; without, this browser does.')}`);
}
const ADMINTXT={users:['Users and groups','Who exists, who is in which group, who may administer. Comes with M-5.'],src:['Sources','Connections to databases and other systems. Comes with M-4.'],store:['Storage and backup','Where documents lie, the share, the export. Comes with M-2.'],signin:['Sign-in','How people sign in. The gate in front decides today.'],audit:['Audit log','Who did what. Comes with M-2 and M-5.']};
function adminHtml(sec){
  /* Users and groups are the tab Rights (rights.js), shown here too for the administration in the profile (ADR-16, ADR-29). */
  if(sec==='users'&&S.real&&window.UDRights) return UDRights.nav3('users',S.user)+UDRights.nav3('groups',S.user);
  if(sec==='audit'&&S.real&&window.UDRights) return UDRights.nav3('log',S.user);
  if(sec==='signin') return panel(`${ptitle('Sign-in')}${hint('Everyone signs in through the sign-in gate in front of UnderDoc. Who may come in, the ways in and the accounts are managed there; administrators hold the role of the app there.')}`);
  if(sec==='mods'&&!document.getElementById('root').hasAttribute('data-demo')) return panel(`${ptitle('Modules')}<div class="item-group" role="list">${(S.modules||[]).map(m=>pitem('article','',esc(m.name),esc(m.description),m.on?onOff(true):'<span class="badge" data-variant="outline">✗ Off</span>')).join('')}</div>${hint('Off means gone for everyone, and no data is touched.')}<div class="flex flex-wrap gap-1.5">${S.user.admin?plink('/admin/modules','Switch modules'):''}${plink('/account/modules','Hide modules for me')}</div>`);
  if(sec!=='mods') return panel(`${ptitle(ADMINTXT[sec][0])}${hint(ADMINTXT[sec][1])}`);
  return panel(`${ptitle('Modules')}<div class="item-group" role="list">${MODS.map(m=>pitem('button',`type="button" data-act="mod" data-id="${m[0]}" aria-pressed="${S.mods[m[0]]}"`,m[1],m[2],onOff(S.mods[m[0]]))).join('')}</div>${hint('Off means gone for everyone, and no data is touched. Only administrators see this part of the profile.')}`);
}
/* A row of a list in nav3 that opens one thing (C22 tree row); the column at its end is a small. */
const listRow=(id,ico,label,side='')=>{const t=at(S.focus);return `<li><div><span data-toggle aria-hidden="true"></span><button type="button" data-act="open" data-id="${id}" title="${label}"${t&&t.d===id?' aria-current="page"':''}>${icon(ico)}<span>${label}</span></button>${side?`<small>${side}</small>`:''}</div></li>`;};
const colLabel=t=>`<span class="workspace-label"><span>${t}</span></span>`;
/* A quiet line in nav3: the size of the hints there (the old block set 13px for everything in nav3). */
const muted=t=>`<p class="text-muted-foreground text-xs">${t}</p>`;
const BM=[{id:'bar',n:'Toolbar',i:'star',items:[{n:'Ticket system',u:'https://tickets.example.com',d:'Open tickets of the team.'},{n:'Supplier portal',u:'https://supplier.example.net',d:'Orders and licenses.'},{n:'Weather radar',u:'https://weather.example.org',d:'Before crane work.'}]},
  {id:'work',n:'Work',i:'folder',items:[{n:'Switch manual',u:'https://docs.example.com/switch',d:'Firmware 9, chapter on VLANs.'},{n:'Crane company',u:'https://crane.example.net',d:'Booking and phone numbers.'}]},
  {id:'read',n:'Reading list',i:'list',items:[{n:'Antenna mounting guide',u:'https://example.org/antenna',d:'Read before the mounting.'}]}];
const FEEDS=[{id:'all',n:'All',i:'list'},{id:'blog',n:'Project blog',i:'globe'},{id:'sec',n:'Security notes',i:'shield'},{id:'rel',n:'Release notes',i:'flag'}];
const NEWS=[{id:1,f:'sec',date:'09:40',unread:true,h:'Switch firmware 9.4 fixes a login flaw',b:'The vendor advises the update for all core switches. A made-up entry.'},
  {id:2,f:'rel',date:'Mon',unread:true,h:'Office suite 26.2 is out',b:'Better import of slides. A made-up entry.'},
  {id:3,f:'blog',date:'Oct 30',unread:false,h:'How we document a tower build',b:'A short story about pages, values and photos. A made-up entry.'}];
/* Private: only its user sees it. Each entry is a view like any other: nav1 opens its lists in nav2, nav2 opens one in nav3, nav3 opens into the active window. */
const PRIVV=[['todo','To-do','check'],['cal','Calendar','cal'],['time','Time','clock'],['vac','Vacation','sun'],['notes','Notes','note']];
const TL=[{id:'mine',n:'My to-dos',i:'check',items:[{t:'Renew the first aid course',done:false,due:'Nov 20'},{t:'Hand in travel expenses',done:true},{t:'Plan the winter holidays',done:false}]},
  {id:'shop',n:'Shopping',i:'list',items:[{t:'Cable ties',done:false},{t:'Labels',done:false},{t:'Gloves',done:true}]},
  {id:'car',n:'Car',i:'list',items:[{t:'Winter tyres',done:false,due:'Nov 14'},{t:'Inspection',done:false,due:'Dec 02'}]}];
const CALS=[{id:'me',n:'Personal',i:'cal',ev:{3:'Dentist',12:'Review',24:'Vacation',25:'Vacation'}},{id:'team',n:'Team',i:'user',ev:{7:'Crane call',18:'Team lunch'},src:'Shared by the project team'},{id:'hol',n:'Holidays',i:'globe',ev:{1:'Public holiday'},src:'Imported from a calendar file'}];
const BOARDS=[{id:'quick',n:'Quick notes',i:'note',doc:'qn'},{id:'ideas',n:'Ideas',i:'spark',doc:'nb:ideas'}];
const SORT={todo:TL,cal:CALS,notes:BOARDS,bookmarks:BM};
const PROFSET=[['acct','Account','user'],['look','Appearance','sun'],['navi','Navigation','panel'],['docs','Documents','page'],['lang','Language','globe'],['notif','Notifications','mail']];
const PROFADM=[['mods','Modules','grid'],['users','Users and groups','user'],['src','Sources','db'],['store','Storage and backup','hd'],['signin','Sign-in','lock'],['audit','Audit log','list']];
/* A to-do in nav3 is C22's small entry with its checkbox (done is checked and struck through); its title opens it. */
const tickRow=(l,k,x)=>{const t=at(S.focus),id=`ti:${l.id}/${k}`;return `<label class="item" data-size="xs" role="listitem"${t&&t.d===id?' aria-current="page"':''}><input class="input" type="checkbox" data-act="tick" data-l="${l.id}" data-id="${k}" aria-label="Done: ${esc(x.t)}"${x.done?' checked':''}><section><h4><button type="button" data-act="open" data-id="${id}" title="${esc(x.t)}">${esc(x.t)}</button></h4></section>${x.due?`<aside><time>${x.due}</time></aside>`:''}</label>`;};
/* Private lists follow the rule of collections: a click opens one and closes what is not kept open, a double click keeps one open. */
function privPart(v,sec){
  const pinned=(S.spin[v]||{})[sec],hd=(n,doc,add='')=>`<section><header data-sechead="${sec}"><a href="#" data-act="open" data-id="${doc}" title="Open ${esc(n)}">${pinned?pinMark():''}<span>${esc(n)}</span></a>${add}</header>`;
  if(v==='todo'){const l=TL.find(x=>x.id===sec);if(!l)return '';return hd(l.n,'tl:'+l.id,newBtn(`data-act="tadd" data-id="${l.id}" aria-label="Add a to-do in ${esc(l.n)}" title="Add a to-do"`))+`<div class="item-group" role="list">${l.items.map((x,k)=>tickRow(l,k,x)).join('')}</div></section>`;}
  if(v==='cal'){const cs=sec==='all'?CALS:CALS.filter(c=>c.id===sec);if(!cs.length)return '';const ev=cs.flatMap(c=>Object.entries(c.ev).map(([d,n])=>[+d,n,c])).sort((a,b)=>a[0]-b[0]);
    return hd(sec==='all'?'All calendars':cs[0].n,'cal:'+sec)+`<ul class="workspace-tree">${ev.map(e=>listRow(`ev:${e[2].id}/${e[0]}`,'clock',e[1],'Nov '+e[0])).join('')}</ul>${sec!=='all'&&cs[0].src?`<p class="text-muted-foreground text-xs">${cs[0].src}.</p>`:''}</section>`;}
  if(v==='time'){const P={week:['priv:time','This week',[['Mon','8:12'],['Tue','8:27'],['Wed','7:45'],['Thu','7:16']]],last:['time:last','Last week',[['Mon','8:00'],['Tue','8:05'],['Wed','8:30'],['Thu','7:50'],['Fri','6:10']]],oct:['time:oct','October',[['Week 40','39:10'],['Week 41','40:05'],['Week 42','38:20'],['Week 43','40:00']]]}[sec];if(!P)return '';
    return hd(P[1],P[0])+`<ul class="workspace-tree">${P[2].map(r=>listRow(P[0],'cal',r[0],r[1])).join('')}</ul></section>`;}
  if(v==='vac'){const P={y26:['priv:vac','2026',[['Mar 02 – 13','10 days ✓'],['Jul 20 – 27','8 days ✓'],['Nov 24 – 28','5 days △']]],y27:['vac:27','2027',[]]}[sec];if(!P)return '';
    return hd(P[1],P[0])+`<ul class="workspace-tree">${P[2].map(r=>listRow(P[0],'sun',r[0],r[1])).join('')}</ul></section>`;}
  if(v==='notes'){const b=BOARDS.find(x=>x.id===sec);if(!b)return '';const ns=D[b.doc].notes;return hd(b.n,b.doc)+`<ul class="workspace-tree">${ns.map(n=>listRow(b.doc,'note',esc(n.h||n.b.split('\n')[0]))).join('')}</ul></section>`;}
  return '';
}
function privNav3(v){
  const lock=navLabel('lock','Only you can see this.'),so=S.so[v]||{};
  const parts=SECTIONS[v]().filter(x=>so[x[0]]).map(x=>privPart(v,x[0])).join('');
  return `<div id="tree">${lock}${parts||muted('Nothing open. Pick a list in nav2.')}</div>`;
}
/* The time tracking (module Time, docs/time-tracking.md): with the module on, Time and Vacation of the private area are
   EightPerDay's pages, each opened like a document; its window shows the page in a frame of the same origin, which
   timetracking.js keeps over the window. With the module off they are gone. */
const timeReal=v=>S.real&&S.timeOn&&(v==='time'||v==='vac')&&!!window.UnderDocTime;
TYPE.epd=['clock','Time tracking'];
function timePseudos(){
  if(!window.UnderDocTime)return;
  [...UnderDocTime.pages,...UnderDocTime.leave].forEach(p=>pseudo('epd:'+p.path,{t:'epd',n:p.name,path:'not a file, private',
    place:'Private › Time tracking',vis:'Only you, and your organization what you record for it',by:'you',ch:'',stat:'Time tracking',src:'/time'+p.path}));
}
function timeNav3(v){
  const lock=navLabel('lock','Only you see this. What you record for the organization of this workspace its administrators see.');
  const list=v==='vac'?UnderDocTime.leave:UnderDocTime.pages;
  return `<div id="tree">${lock}<ul class="workspace-tree">${list.map(p=>listRow('epd:'+p.path,p.icon,p.name)).join('')}</ul></div>`;
}
/* The app password for SMB (ADR-30): the Samba user, a new password shown once, the shares this account reaches. */
function sambaHtml(){
  if(S.samba===undefined){S.samba=null;fget('/api/samba').then(x=>{S.samba=x;render();}).catch(()=>{S.samba={on:false};});}
  const sm=S.samba;if(!sm||!sm.on)return '';
  const pass=S.sambaPass?note(`App password for <code>${esc(S.sambaPass.name)}</code>, shown only now: <code>${esc(S.sambaPass.password)}</code>. Windows asks for it once and keeps it.`,' data-samba-password'):'';
  if(!sm.has)return panel(`${ptitle('SMB share')}${hint('Reach your files as a network drive on Windows, with a user name and an app password of their own. The password is shown once, and only its hash is kept.')}<div class="flex flex-col gap-1.5"><label class="label" for="udsamba">User name</label><input class="input" data-size="sm" id="udsamba" data-samba-name value="${esc(sm.suggest||'')}" aria-label="Samba user name"></div><div>${pbtn('data-act="sambanew"','Make an app password')}</div>${pass}`,' data-samba');
  /* Saves over SMB a quota turned away stay on the share without a version, until there is room (ADR-30). */
  const refused=(sm.refused||[]).length?`<div class="alert" data-variant="warning" role="status" data-samba-refused><h2>Not taken from the share</h2><section><ul>${sm.refused.map(x=>`<li><code>${esc(x.library)}/${esc(x.path)}</code>: ${esc(x.why)}</li>`).join('')}</ul>They stay on the share without a version. Make room, then save again.</section></div>`:'';
  return panel(`${ptitle('SMB share')}${hint(`User name <code data-samba-user>${esc(sm.name)}</code>. In Windows: map a network drive to a path below, with this name and your app password.`)}<ul class="text-muted-foreground text-xs" data-samba-shares>${(sm.shares||[]).map(s=>`<li><code>${esc(s)}</code></li>`).join('')||'<li>No share yet: the next pass of the share gives them.</li>'}</ul>${refused}<div class="flex flex-wrap gap-1.5">${pbtn('data-act="sambanew"','New app password')}${pbtn('data-act="sambadel"','Remove the app password')}</div>${pass}`,' data-samba');
}
function sambaAct(act){
  if(act==='sambanew'){const f=document.querySelector('[data-samba-name]');
    fsend('/api/samba/password','POST',{name:f?f.value:''}).then(a=>{S.sambaPass=a;S.samba=undefined;render();}).catch(e=>toast(e.message));}
  if(act==='sambadel'&&window.confirm('Remove the app password? SMB stops at once for this account.'))
    fsend('/api/samba/password','DELETE').then(()=>{S.sambaPass=null;S.samba=undefined;render();}).catch(e=>toast(e.message));
}
/* The account: C22's entry with the avatar, the groups as badges, then its links. */
function accountHtml(){
  const u=S.user;
  return panel(`<div class="item" data-size="sm" data-who><figure><span class="avatar"><span>${esc(u.initials)}</span></span></figure><section><h4>${esc(u.name)}</h4><p class="text-muted-foreground text-xs break-words">${esc(u.email||u.login)}</p></section></div><div class="flex flex-wrap gap-1">${u.groups.map(g=>`<span class="badge" data-variant="outline">${esc(g)}</span>`).join('')}</div>
    ${hint(`Name, mail and groups come from the sign-in gate, which also keeps the ways in and the sessions. Favorites, settings and the private area belong to this account.${u.agent?' This request comes from an agent, not from a session: it may not manage rights, share or delete for good.':''}`)}<div class="flex flex-wrap gap-1.5">${plink('/.tinysesam/logout','Sign out')}</div>`)+sambaHtml();
}
/* Mail and news in nav3: C22's entries, the sender and the date over the subject; unread is bold (data-state), the open
   one aria-current. */
const msgList=rows=>{const t=at(S.focus);return `<div class="item-group" role="list">${rows.map(([id,from,date,subj,unread])=>`<a href="#" class="item" data-size="sm" role="listitem" data-act="open" data-id="${id}"${t&&t.d===id?' aria-current="page"':''}${unread?' data-state="unread"':''}><header><span>${from}</span><time>${date}</time></header><section><h4>${subj}</h4></section></a>`).join('')}</div>`;};
function nav3Html(){
  const v=S.view,sec=S.sec[v];
  if(v==='workspace') return `<div id="tree">${treeHtml()}</div>`;
  if(v==='search') return searchHtml();
  if(v==='rights') return window.UDRights?UDRights.nav3(sec,S.user):'';
  if(v==='profile'){const sec=S.sec.profile;return `<div>${sec==='acct'?accountHtml():PROFADM.some(x=>x[0]===sec)?adminHtml(sec):settingsHtml(sec)}</div>`;}
  if(timeReal(v)) return timeNav3(v);
  if(PRIVV.some(x=>x[0]===v)) return privNav3(v,sec);
  if(v==='files'){
    if(FS.on&&sec!=='ext')return filesNav3();
    if(sec==='ext') return `<div>${panel(`${ptitle('Add a source')}<div class="item-group" role="list">${[['SMB share','A Windows or NAS share'],['WebDAV','A cloud folder over WebDAV'],['SFTP','A folder on a server'],['S3','An object store bucket']].map(x=>pitem('button','type="button" disabled',x[0],x[1],'')).join('')}</div>${hint('Later. A source you add shows here next to your files and the share, and every user sees only the sources they may use.')}`)}</div>`;
    const root=sec==='home'?'~':'',head=sec==='home'?'My files · only you':'Share';
    return `<div id="tree">${colLabel(head)}<ul class="workspace-tree">${listRow('dir:'+root,'folder',sec==='home'?'All my files':'All of the share')}${FILEROWS[root].map(x=>diskRow(root,x)).join('')}</ul></div>`;}
  if(v==='bookmarks'){
    if(sec==='import') return `<div id="tree">${colLabel('Import')}<ul class="workspace-tree">${listRow('bm:import','down','From a browser')}</ul>${muted('Every browser exports its bookmarks as one HTML file.')}</div>`;
    const f=BM.find(x=>x.id===sec);return `<div id="tree">${colLabel(f.n)}<ul class="workspace-tree">${f.items.map((b,k)=>listRow(`bm:${f.id}/${k}`,'globe',b.n)).join('')}</ul></div>`;}
  if(v==='news'){
    if(sec==='sub') return `<div id="tree">${colLabel('Subscribe')}<ul class="workspace-tree">${listRow('news:sub','plus','Add a feed')}</ul></div>`;
    const t=at(S.focus),its=NEWS.filter(n=>sec==='all'||n.f===sec);return `<div id="tree">${colLabel(FEEDS.find(x=>x.id===sec).n)}${msgList(its.map(n=>['news:'+n.id,FEEDS.find(x=>x.id===n.f).n,n.date,n.h,n.unread]))}${hint('Later. Made-up entries.')}</div>`;}
  if(v==='resources'){const g=RES.find(x=>x.id===sec);return `<div id="tree">${colLabel(g.n)}<ul class="workspace-tree">${listRow('res:'+g.id,'list','All '+g.n.toLowerCase())}${g.rows.map(r=>listRow(resTarget(g,r),g.i,r[0])).join('')}</ul></div>`;}
  if(v==='mail'){const t=at(S.focus),ms=MAILS.filter(m=>m.box===sec);return `<div id="tree">${colLabel(sec)}${(ms.length?msgList(ms.map(m=>['mail:'+m.id,m.from,m.date,m.subj,m.unread])):hint('Nothing here.'))}${hint('Comes with M-22. Made-up mail.')}</div>`;}
  return '';
}
function render(){
  if(S.booted)saveSettings();
  /* The editor of a page keeps the focus through a render; S.focusNext asks for it, at a block if it names one. */
  const ae=document.activeElement,ren=ae&&ae.dataset&&ae.dataset.ren!==undefined?{v:ae.value,a:ae.selectionStart,b:ae.selectionEnd}:null,want=S.focusNext,edFocus=Object.keys(EDS).find(k=>EDS[k].ed.hasFocus);S.focusNext=null;
  const kept=[...document.querySelectorAll('[data-keep]')].map(n=>({k:n.dataset.keep,field:n.tagName==='INPUT',pick:n.tagName==='SELECT',v:n.value,f:n===ae,a:n.selectionStart,b:n.selectionEnd}));
  const root=document.documentElement,ph=phone();
  if(window.udSetTheme&&window.udTheme&&window.udTheme()!==S.theme)window.udSetTheme(S.theme);
  root.style.zoom=S.scale/100;
  /* No bar of its own: name, New and search sit above the columns, the tabs of each window in the same row beside them;
     where nav2 and nav3 are away (and on a phone) only New stays.
     The frame is C22's .workspace (T-12); data-behaviour="app" leaves its columns, grips and drawer to this script. */
  const wide=S.rail&&!ph;
  const bar=`<header class="workspace-top">${wide?`<a class="brand" title="UnderDoc"><span class="brand-mark"><img class="size-full" src="${LOGO}" alt=""></span><span>UnderDoc</span></a>
    <form role="search"><input class="input" type="search" id="search" value="${esc(S.sq)}" placeholder="Search · Ctrl+K" aria-label="Search" title="Search: / here, Ctrl+K anywhere"></form>`:''}
    <div class="popover"><button type="button" class="btn" data-variant="outline" data-size="icon-sm" data-act="newmenu" aria-haspopup="dialog" aria-expanded="${S.newAt==='bar'}" aria-label="New" title="New: type a note or pick a type">${icon('plus')}</button>${S.newAt==='bar'?newMenuHtml('bar'):''}</div>${solo()&&!ph?winBtn('outline'):''}</header>`;
  const n1=(v,n,i)=>navItem('view',v,n,i,S.view===v);
  const grip=(n,label,title)=>`<span data-grip="${n}" role="separator" aria-orientation="vertical" aria-label="${label}" tabindex="0" title="${title}"></span>`;
  const privs=S.mods.private?`<hr>${navLabel('lock','Private','Only you see this')}${PRIVV.filter(x=>!S.real||S.timeOn||!['time','vac'].includes(x[0])).map(x=>n1(x[0],x[1],x[2])).join('')}`:'';
  const strip=`<nav class="workspace-nav" data-level="1" data-step="${ph?1:S.s1}" aria-label="Nav1"><div>${n1('workspace','Workspace','panel')}${n1('files','Files','hd')}${n1('resources','Resources','db')}${S.real&&window.UDRights?n1('rights','Rights','shield'):''}${S.mods.news?n1('news','News','globe'):''}${S.mods.mail?n1('mail','Mail','mail'):''}<hr>${n1('search','Search','search')}<span data-fill></span>${privs}</div><footer><hr>
    <button type="button" class="workspace-item" data-act="view" data-id="profile"${S.view==='profile'?' aria-current="page"':''} title="Profile, settings and administration"><span class="avatar" data-size="sm"><span>${esc(S.user.initials)}</span></span><span>Profile</span></button></footer>${grip(1,'Width of nav1','Drag: snaps to the next view')}</nav>`;
  const rail=S.rail?`<nav class="workspace-nav" data-level="2" data-step="${ph?1:S.s2}" aria-label="Nav2">${nav2Html()}${grip(2,'Width of nav2','Drag: snaps to the next view')}</nav><nav class="workspace-nav" data-level="3" aria-label="Nav3">${nav3Html()}${grip(3,'Width of nav3','Drag to change the width')}</nav>`:'';
  /* A short message is C22's toast in its toaster; it hangs in the workspace like the menus. */
  const pops=`${S.toast?`<div class="toaster" data-align="center"><div class="toast" role="status" aria-atomic="true"><div class="toast-content"><section><h2>${esc(S.toast)}</h2></section></div></div></div>`:''}${ctxHtml()}${addPop()}${waysHtml()}${udSearch.palHtml()}`;
  const toast0=document.querySelector('.workspace > .toaster');
  rendering=true;document.getElementById('root').innerHTML=`<div class="workspace" data-behaviour="app"${S.resizing?' data-resizing':''}${S.rail?(ph?' data-nav-open':''):' data-nav-hidden'}>${bar}${strip}${rail}<div class="workspace-windows">${S.wins.map(winHtml).join('')}</div>${pops}</div>`;
  /* A toast that stays through a render is the same node, so it does not come in again. */
  const toast1=document.querySelector('.workspace > .toaster');if(toast0&&toast1&&toast0.textContent===toast1.textContent)toast1.replaceWith(toast0);
  /* The width of nav3 is the block's custom property, set in the CSSOM (no style attribute under the CSP). */
  const wsEl=document.querySelector('.workspace');wsEl.style.setProperty('--workspace-nav3',S.w3+'px');
  mountEditors(edFocus,want);
  S.wins.forEach((w,i)=>{const t=at(i),e=document.querySelector(`[data-sc="${i}"]`);if(t&&e)e.scrollTop=t.top;applyFind(i);});
  udSearch.after();
  if(window.UnderDocTime)UnderDocTime.place();
  rendering=false;
  /* A name being written keeps its text and the focus through a render. */
  const rn=ren&&document.querySelector('[data-ren]');if(rn){rn.value=ren.v;rn.focus();rn.setSelectionRange(ren.a,ren.b);}
  /* So does a name being typed into a form of the files, and a button of the files reached by the keyboard keeps the
     focus: a redraw of anything else (a toast going, an upload ending) would otherwise put the first name back and drop
     the focus, and the typing or the Enter after it would go nowhere (2026-11-17). */
  kept.forEach(x=>{const n=[...document.querySelectorAll('[data-keep]')].find(m=>m.dataset.keep===x.k);if(!n)return;if(x.field||x.pick)n.value=x.v;
    if(x.f){n.focus({preventScroll:true});if(x.field)n.setSelectionRange(x.a,x.b);}});
  const tr=document.getElementById('tree');if(tr)tr.scrollTop=S.treeTop;
  /* A menu with no room below its heading opens above it. */
  /* A menu stays inside the window: at the edge it moves in, and a long one scrolls (2026-11-09). Its place lives in
     data-style, which is applied after a render, so a moved place is written there too. */
  const stay=(el,up)=>{if(!el)return;applyStyles(el);const z=S.scale/100,r=el.getBoundingClientRect(),cs=el.style;let x=parseFloat(cs.left),y=parseFloat(cs.top),moved=false;
    if(r.right>innerWidth-4&&!isNaN(x)){x=Math.max(0,x-(r.right-innerWidth+4)/z);moved=true;}
    if(r.bottom>innerHeight-4&&!isNaN(y)){y=up!==undefined?Math.max(4,up-r.height/z):Math.max(4,(innerHeight-4-r.height)/z);moved=true;}
    if(moved){el.dataset.style=(isNaN(x)?'':`left:${x}px;`)+(isNaN(y)?'':`top:${y}px`);el.style.cssText=el.dataset.style;}};
  document.querySelectorAll('.context-menu > [data-cm-panel],section[data-menu="quick"]').forEach(el=>stay(el));
  /* The menu of a handle opens below it, or above it where the window has no room below, so it stays inside the window
     as the right-click menus do (2026-11-10). */
  const bm=document.querySelector('.editor-handle > [data-popover]'),bh=bm&&bm.previousElementSibling;
  if(bm&&bh){const r=bm.getBoundingClientRect();if(r.bottom>innerHeight-4&&bh.getBoundingClientRect().top-r.height-6>=0)bm.dataset.side='top';}
  /* The menu New opens below its button, or above it where there is no room, and the field takes the typing at once. */
  const ap=document.querySelector('[data-menu="new"]');if(ap){stay(ap,S.addAt.up);filterNew(S.addQ);const f=ap.querySelector('[data-nfilter]');f.focus();f.setSelectionRange(f.value.length,f.value.length);}
  /* Every element of a page has its ID, and a speech bubble beside it holds its comments. */
  document.querySelectorAll('[data-art]').forEach(art=>{const i=+art.dataset.art,t=at(i);if(!t||!t.d||t.src||D[t.d].tool)return;if(D[t.d].blocks){dbBubbles(art,i,t);return;}if(accOf(t.d)==='read'||fromSource(t.d))return;const doc=art.matches('article.typeset')?art:null;if(!doc)return;
    const cs=D[t.d].comments||[],n={p:0,h:0,t:0,l:0,c:0};
    doc.querySelectorAll('p,h2,h3,ul,ol,pre,.table-container').forEach(el=>{if(el.parentElement.closest('.editor-embed,[role=list],.table-container,ul,ol,[data-notes],.board,[data-facts],.alert,.empty,figure,dl')||el.matches('.text-muted-foreground')||el.closest('.editor-embed'))return;
      const k=el.matches('.table-container')?'t':el.matches('h2,h3')?'h':el.matches('ul,ol')?'l':el.matches('pre')?'c':'p',id=k+(++n[k]),cnt=cs.filter(c=>c.b===id).length;
      const lab=k==='t'?'Table: '+[...el.querySelectorAll('th')].map(x=>x.textContent).join(', '):el.textContent.trim().slice(0,60);
      const b=bubble(i,id,lab,cnt,cnt?cnt+' comments on this':'Comment on this');
      /* A table scrolls sideways in its container, which would cut the bubble off: its anchor is a wrapper. The anchor
         carries the focus of the open comments. */
      let anchor=el;
      if(el.matches('.table-container')){anchor=document.createElement('div');anchor.className='comment-anchor';el.before(anchor);anchor.append(el,b);}
      else{el.classList.add('comment-anchor');el.append(b);}
      if(t.cmt&&t.cb===id)commentFocus(anchor);});});
  /* A table made in a page gets its ID right away. Its address can be pasted into another page, where it shows live. */
  document.querySelectorAll('[data-art]').forEach(art=>{const t=at(+art.dataset.art);if(!t||!t.d||t.src||D[t.d].tool)return;
    if(D[t.d].blocks)return;
    (art.matches('article.typeset')?art:document.createElement('div')).querySelectorAll('table').forEach((tb,k)=>{if(tb.closest('.editor-embed'))return;const own=tb.closest('[data-block]'),part=own?own.dataset.block:'t'+(k+1),b=document.createElement('button');b.type='button';b.className='btn';b.dataset.variant='ghost';b.dataset.size='xs';b.dataset.act='blocklink';b.dataset.id=t.d+'#'+part;b.title='Copy the address of this table';b.innerHTML=icon('opennew')+'<span>#'+part+'</span>';(tb.closest('.table-container')||tb).before(b);});});
}
/* AI drawer on the right: it knows the document in the active window. */
const aiAnswer=()=>{const t=at(S.focus);return t&&t.d?`Made up for this mockup. A real answer about “${D[t.d].n}” would stand here, with links to its sources.`:'Made up for this mockup. Open a document, and the answer can refer to it.';};
/* The assistant is the window's drawer as in C22's views: what it is about, question and answer as bubbles in a log,
   prompts as small buttons, the field in an input group. */
function aiHtml(){
  const t=at(S.focus),about=t&&t.d?D[t.d].n:'nothing open';
  return drawer('AI assistant',`${icon('ai')}AI assistant`,`<p class="text-muted-foreground text-sm">About: <b>${esc(about)}</b></p>
    <div role="log" aria-label="Conversation">${S.chat.map(m=>`<div class="bubble" data-align="end">${esc(m.q)}</div><div class="bubble" data-variant="outline">${m.a}</div>`).join('')||hint('Ask about the open document, or let it draft something.')}</div>
    <div class="flex flex-wrap gap-1.5">${['Summarize this','Find related documents','Draft a note from this'].map(x=>pbtn(`data-act="aiask" data-id="${x}"`,x)).join('')}</div>
    <div class="input-group"><input class="input" data-aiq placeholder="Ask, then Enter" aria-label="Ask the AI"></div>
    ${hint('Which model, and where it runs, is settled in M-13. It is off until an administrator sets it up.')}`,'aid','');
}
function applyFind(i,scroll){
  const t=at(i);if(!t||!t.find)return;
  const root=document.querySelector(`[data-art="${i}"]`)||document.querySelector(`[data-sc="${i}"]`),out=document.querySelector(`[data-findc="${i}"]`);
  if(!root||!out)return;
  if(!t.q){out.textContent='';return;}
  const re=new RegExp(t.q.replace(/[.*+?^${}()|[\]\\]/g,'\\$&'),'gi'),nodes=[],tw=document.createTreeWalker(root,NodeFilter.SHOW_TEXT);
  while(tw.nextNode())nodes.push(tw.currentNode);
  nodes.forEach(n=>{const s=n.nodeValue;re.lastIndex=0;if(!re.test(s))return;re.lastIndex=0;const f=document.createDocumentFragment();let last=0,m;
    while((m=re.exec(s))){f.append(s.slice(last,m.index));const k=document.createElement('mark');k.textContent=m[0];f.append(k);last=m.index+m[0].length;}
    f.append(s.slice(last));n.replaceWith(f);});
  const ms=[...root.querySelectorAll('mark')];
  if(!ms.length){out.textContent='no hit';return;}
  t.cur=((t.cur%ms.length)+ms.length)%ms.length;
  /* The current hit is C22's mark[aria-current] (framed and bold). */
  ms[t.cur].setAttribute('aria-current','true');out.textContent=`${t.cur+1} of ${ms.length}`;
  if(scroll)ms[t.cur].scrollIntoView({block:'center'});
}
/* Every window has its own general tab, and a list opens into the active window. */
function openDoc(id){
  const w=S.wins[S.focus]||S.wins[0];let k=w.tabs.findIndex(t=>t.d===id);
  if(k<0){k=0;w.tabs[0]=newTab(id,true);w.hist=[id,...(w.hist||[]).filter(x=>x!==id)].slice(0,15);}
  w.a=k;if(id.startsWith('mail:'))MAILS.find(m=>'mail:'+m.id===id).unread=false;if(phone())S.rail=false;
  if(D[id]&&D[id].real){if(!D[id].loaded)loadDoc(id);const a=addressOf(id);if(location.pathname!==a)history.pushState({id},'',a);}
}
/* What was opened here before and is not open as a tab of its own. */
const olderOf=w=>(w.hist||[]).filter(x=>D[x]&&x!==w.tabs[0].d&&!w.tabs.some((t,j)=>j>0&&t.d===x));
/* When the general tab gives up its document, the one opened before takes its place. */
function fallback(w){const g=w.tabs[0],n=olderOf(w)[0]||null;Object.assign(g,newTab(n,true));if(!n&&w.a===0&&w.tabs.length>1)w.a=1;}
function popGeneral(w){const g=w.tabs[0];w.hist=(w.hist||[]).filter(x=>x!==g.d);fallback(w);}
function take(w,k){const t=w.tabs[k];
  if(t.general){const c={...t,general:false,gear:false,fly:false};fallback(w);return c;}
  w.tabs.splice(k,1);if(w.a>k)w.a--;return t;}
function placeTab(i,k,to,p){
  const src=S.wins[i],dst=S.wins[to],t=take(src,k);
  p=Math.max(1,Math.min(p,dst.tabs.length));
  dst.tabs.splice(p,0,t);dst.a=p;S.focus=to;compact();
}
/* Dragging the general tab keeps its document as a tab where it lands, and the general tab stays, empty. */
function newColumn(i,k,pos){
  if(S.wins.length>=4)return;
  const t=take(S.wins[i],k),f={tabs:[newTab(null,true),t],a:1,hist:[]};
  S.wins.splice(pos,0,f);S.focus=pos;compact();S.focus=Math.max(0,S.wins.indexOf(f));
}
/* One rule everywhere: a click opens only this, a double click keeps it open, Ctrl and click selects. */
function onlyThis(id){const o={};C.forEach(c=>{if(S.pin[c.id])o[c.id]=true;});o[id]=true;S.open=o;}
function togglePin(id){if(S.pin[id]){S.pin[id]=false;S.open[id]=false;}else{S.pin[id]=true;S.open[id]=true;}}
/* The second click of a double click comes with a click count of 2. The list is drawn anew after the first click, so the browser's dblclick event may never arrive. */
/* New opens below its row: the head of nav2 or of the collection. */
function openNew(id,anchor){const z=S.scale/100,r=anchor?anchor.getBoundingClientRect():{left:0,top:0,bottom:0};S.addIn=id;S.addQ='';S.addAt={x:Math.round(r.left/z),y:Math.round(r.bottom/z)+2,up:Math.round(r.top/z)-2};}
function collClick(id,e){if(e.detail===2)togglePin(id);else if(e.detail<2)onlyThis(id);}
/* Ctrl and click adds an entry of nav2 or nav3 to a selection or takes it out, Shift and click takes the range from the
   last one; a selection stays in one list. Keeping a collection open is a double click or the right click. */
/* The columns of C22's workspace, by their level. */
const N1='nav.workspace-nav[data-level="1"]',N2='nav.workspace-nav[data-level="2"]',N3='nav.workspace-nav[data-level="3"]';
const SELECTABLE=`${N3} li > div[data-row],#clist > [data-favid],${N2} [data-secid]`;
const zoneOfEl=n=>n.closest(N3)?'n3':n.matches('[data-secid]')?'sec':'n2';
const idOfEl=n=>n.dataset.row||n.dataset.favid||n.dataset.secid;
const isSel=(z,id)=>!!S.sel&&!S.sel.soft&&S.sel.z===z&&S.sel.ids.includes(id);
/* The entry clicked last, or the document open in the window, counts as selected without showing it, so Ctrl and Shift
   add to it. */
function selectClick(el,e){
  const z=zoneOfEl(el),id=idOfEl(el);
  if(!S.sel||S.sel.z!==z){const t=at(S.focus),cur=z==='n3'&&t&&t.d&&elOf('row',t.d)?t.d:null;S.sel={z,ids:cur?[cur]:[],anchor:cur};}
  S.sel.soft=false;
  const all=[...document.querySelectorAll(SELECTABLE)].filter(n=>zoneOfEl(n)===z).map(idOfEl);
  if(e.shiftKey&&S.sel.anchor&&all.includes(S.sel.anchor)){const a=all.indexOf(S.sel.anchor),b=all.indexOf(id);S.sel.ids=all.slice(Math.min(a,b),Math.max(a,b)+1);}
  else{const k=S.sel.ids.indexOf(id);if(k<0)S.sel.ids.push(id);else S.sel.ids.splice(k,1);S.sel.anchor=id;}
  if(!S.sel.ids.length)S.sel=null;
}
function openTab(id){const w0=S.wins[S.focus];let k=w0.tabs.findIndex((x,j)=>j>0&&x.d===id);if(k<0){w0.tabs.push(newTab(id));k=w0.tabs.length-1;}w0.a=k;}
document.addEventListener('scroll',e=>{const x=e.target;if(!x.dataset)return;if(x.dataset.sc!==undefined){const t=at(+x.dataset.sc);if(t)t.top=x.scrollTop;}else if(x.id==='tree')S.treeTop=x.scrollTop;},true);
/* What floats over the workspace: menus (C22's context menu, command menu, the open popovers) and the ways dialog. A click
   inside one that hits no button leaves it open. */
const POPS='.context-menu,[data-menu],.dropdown-menu > [data-popover],.popover > [data-popover],[data-ways]';
document.addEventListener('click',e=>{
  if(noClick){noClick=false;e.stopPropagation();e.preventDefault();return;}
  /* A field, or a label round a checkbox, takes its click itself; a button inside a label still acts. */
  if(e.target.closest('input,select,label')&&!e.target.closest('label button[data-act]'))return;
  const pick=(e.ctrlKey||e.metaKey||e.shiftKey)&&!e.target.closest('[data-toggle],'+POPS)&&e.target.closest(SELECTABLE);
  if(pick){e.preventDefault();selectClick(pick,e);S.ctx=null;render();return;}
  if(S.sel&&!e.target.closest('.context-menu'))S.sel=null;
  /* A plain click on an entry is where a range starts: Shift and click then takes everything up to there. */
  const plain=!e.target.closest('[data-toggle],'+POPS)&&e.target.closest(SELECTABLE);
  if(plain)S.sel={z:zoneOfEl(plain),ids:[idOfEl(plain)],anchor:idOfEl(plain),soft:true};
  const b=e.target.closest('button[data-act],a[data-act]'),win=e.target.closest('[data-win]'),act=b&&b.dataset.act;
  /* Links of the frame that act (the head of a collection, tags, contents) do not navigate. */
  if(b&&b.tagName==='A')e.preventDefault();
  if(e.target.closest(POPS)&&!b)return;
  let dirty=S.menu!==null||S.tmenu||S.hist!==null||S.tlist!==null||S.addIn!==null||S.ctx!==null||S.newAt!==null;
  const hadMenu=S.menu,hadT=S.tmenu,hadHist=S.hist,hadTl=S.tlist,hadAdd=S.addIn,hadNew=S.newAt,hadCtx=S.ctx&&S.ctx.kind,ctx0=S.ctx,hadAsk=S.trashAsk;
  if(hadAsk){S.trashAsk=null;dirty=true;}
  if(win&&+win.dataset.win!==S.focus){S.focus=+win.dataset.win;dirty=true;}
  const hadBm=S.bmenu;if(S.bmenu)dirty=true;
  S.menu=null;S.tmenu=null;S.hist=null;S.tlist=null;S.addIn=null;S.ctx=null;S.newAt=null;S.bmenu=null;
  if(!b){if(dirty)render();return;}
  const a=b.dataset,i=a.w!==undefined?+a.w:S.focus,w=S.wins[i],t=at(i),tk=a.t!==undefined?+a.t:w&&w.a;
  if(act!=='add')S.just=null;
  switch(act){
    case 'help':S.help=!S.help;break;
    case 'first':S.first=!S.first;if(S.first){if(S.into==='qn')S.into=C[0].id;if(LATER.view.includes(S.view))S.view='workspace';S.aid=false;}break;
    case 'rail':S.rail=!S.rail;break;
    case 'view':if(S.view===a.id&&S.rail&&a.id!=='search'){S.rail=false;break;}if(a.id==='search'&&S.view!=='search')S.prev=S.view;if(S.view!==a.id)S.treeTop=0;S.view=a.id;S.rail=true;if(a.id==='search'){render();const f=document.getElementById('search');f.focus();f.setSelectionRange(f.value.length,f.value.length);return;}break;
    case 'sec':{const v=S.view;S.treeTop=0;
      /* Where a third column adds nothing, nav2 opens straight into the window. */
      const ad={cal:{import:'cal:import'},time:{export:'time:export'},vac:{plan:'vac:new'},bookmarks:{import:'bm:import'},news:{sub:'news:sub'}}[v];if(ad&&ad[a.id]){openDoc(ad[a.id]);break;}
      if(v==='todo'&&a.id==='newlist'){const id='l'+(++S.newN);TL.push({id,n:'New list',i:'list',items:[]});todoDocs();S.sec.todo=id;S.so.todo[id]=true;break;}
      if(v==='notes'&&a.id==='newboard'){const id='b'+(++S.newN);D['nb:'+id]=boardDoc('New board',[]);BOARDS.push({id,n:'New board',i:'note',doc:'nb:'+id});S.sec.notes=id;S.so.notes[id]=true;break;}
      /* Private lists open like collections: a click opens one and closes what is not kept open, a double click keeps it open or stops. */
      if(S.so[v]){const k=a.id,sp=S.spin[v]=S.spin[v]||{};
        if(e.detail===2){if(sp[k]){sp[k]=false;S.so[v][k]=false;}else{sp[k]=true;S.so[v][k]=true;}}
        else if(e.detail<2){const o={};for(const x in sp)if(sp[x])o[x]=true;o[k]=true;S.so[v]=o;}}
      S.sec[v]=a.id;break;}
    case 'tadd':{const l=TL.find(x=>x.id===a.id);l.items.push({t:'New to-do',done:false});todoDocs();openDoc(`ti:${l.id}/${l.items.length-1}`);break;}
    case 'priv':openDoc(a.id);break;
    case 'newmenu':{if(hadNew===(a.w!==undefined?i:'bar'))break;S.newAt=a.w!==undefined?i:'bar';if(a.w!==undefined)S.focus=i;render();const f=document.querySelector('[data-quick]');if(f)f.focus();return;}
    case 'newtpl':{const f=document.querySelector('[data-quick]'),v=f&&f.value.trim().split('\n')[0].slice(0,200),x=S.tpls.find(y=>y.id===a.id);
      if(D[S.into]&&D[S.into].db&&x)createPage(S.into,v||unesc(x.n),x.id);break;}
    case 'addtpl':{const x=S.tpls.find(y=>y.id===a.id);if(x&&D[a.c]&&D[a.c].db)createPage(a.c,unesc(x.n),x.id);break;}
    case 'settpl':saveTemplate(t.d,a.id==='offered');break;
    case 'newdoc':if(madeHere(S.into)){toast(madeHere(S.into)+'.');return;}openDoc(createIn(S.into,a.id,''));break;
    case 'into':S.into=a.id;S.newAt=hadNew;render();{const f=document.querySelector('[data-quick]');if(f)f.focus();}return;
    case 'addmenu2':{S.view='workspace';S.rail=true;S.open[a.id]=true;render();openNew(a.id,document.querySelector(`${N3} header[data-cdrop="${a.id}"]`));break;}
    case 'opentab':openTab(a.id);break;
    case 'opencol':S.wins.splice(S.focus+1,0,{tabs:[newTab(null,true),newTab(a.id)],a:1,hist:[]});S.focus++;break;
    case 'addchild':if(D[a.id].db){createPage(a.id,'New page');break;}{const id='new'+(++S.newN);D[id]={t:'page',n:'New page',w:'slim',toc:'marker',tags:[],kids:[],path:'not saved, this is a mockup',by:'you',ch:'now',vis:'As its parent',...none,stat:'new',html:'<p class="text-muted-foreground text-sm">Empty.</p>'};D[a.id].kids.unshift(id);S.tog[a.id]=true;rebuildPlaces();openDoc(id);S.just=id;break;}
    case 'copylink':S.toast='Link copied: '+D[a.id].n;setTimeout(()=>{S.toast='';render();},1600);break;
    case 'opengear':openDoc(a.id);at(S.focus).gear=true;break;
    case 'secup':case 'secdown':{const L=SORT[S.view],k=L.findIndex(x=>x.id===a.id),j=k+(act==='secup'?-1:1);if(k>=0&&j>=0&&j<L.length)[L[k],L[j]]=[L[j],L[k]];break;}
    case 'copen':onlyThis(a.id);break;
    case 'cpin':togglePin(a.id);break;
    case 'newcoll':{if(S.canCreate&&(!a.id||(D[a.id]&&D[a.id].real&&D[a.id].t==='coll'))){const par=a.id&&D[a.id]?a.id:'';
      /* At the top it is its maker's alone until shared: the dialog asks with whom (ADR-29, 2026-11-19). */
      if(!par&&window.UDRights){UDRights.create({title:freeName('New collection'),url:'/api/collections',onMade:node=>placeCollection(node,'',false)});return;}
      createCollection(par,freeName('New collection'));return;}const id='c'+(++S.newN),par=a.id&&D[a.id]?a.id:null;collDoc(id,freeName('New collection'),[],'grid');
      if(par){D[par].kids.push(id);S.tog[par]=true;rebuildPlaces();onlyThis(COLL[id].id);}else{C.push(asTop({id,i:'grid'}));onlyThis(id);}
      rebuildPlaces();S.view='workspace';S.rail=true;S.just=id;renameHere(id);return;}
    /* At the top, nothing set means nobody else in the database (ADR-29, 2026-11-19) and edit in the made-up content;
       below, "as above" takes it from there. */
    case 'cacc':setAccess(a.c,a.id==='inherit'||a.id==='none'||(a.id==='edit'&&!D[a.c].db&&C.some(c=>c.id===a.c&&!c.alias))?'':a.id);return;
    case 'setacc':setAccess(t.d,a.id==='inherit'?'':a.id);return;
    case 'cmt':t.cmt=!t.cmt;t.cb=null;if(t.cmt){t.gear=false;if(D[t.d].blocks)loadComments(t.d);}break;
    case 'cmtreply':case 'cmtedit':case 'cmtres':case 'cmtdel':cmtAct(act,t,i,a.id);return;
    case 'cblk':t.cmt=true;t.gear=false;t.cb=a.id;t.cbl=a.l||a.id;render();{const f=document.querySelector(`[data-addcmt="${i}"]`);if(f)f.focus();}return;
    case 'blocklink':S.toast='Link copied: /o/'+a.id+'. Pasted into another page, the table shows there and stays in one place.';setTimeout(()=>{S.toast='';render();},2600);break;
    case 'secclose':S.so[S.view][a.id]=false;if(S.spin[S.view])S.spin[S.view][a.id]=false;break;
    case 'aid':S.aid=!S.aid;break;
    case 'aiask':S.chat.push({q:a.id,a:aiAnswer()});break;
    case 'focus':S.focus=+a.id;break;
    case 'fdir':case 'fopen':case 'fnewdir':case 'fup':case 'frestore':case 'ftrash':if(!filesAct(act,a))return;break;
    case 'fatt':moveAttachment(a.id,a.att);break;
    case 'sambanew':case 'sambadel':sambaAct(act);return;
    case 'tog':S.tog[a.id]=!kidsOpen(a.id);break;
    case 'open':{const d=D[a.id],fold=d.t==='folder'||d.t==='dir'||d.kids.length>0;
      /* A click opens the object, whatever it is. A double click, or the arrow, opens or closes its children in nav3. */
      if(e.detail===2){if(fold&&b.closest(`${N3} li > div`))S.tog[a.id]=!kidsOpen(a.id);break;}
      openDoc(a.id);break;}
    case 'coll':collClick(a.id,e);break;
    case 'collhead':S.open[a.id]=false;S.pin[a.id]=false;break;
    case 'n2line':case 'n2space':{const id=decoId(),k=C.findIndex(c=>c.id===a.id);decoDoc(id,act==='n2line'?'line':'space');C.splice(k<0?C.length:k+1,0,asTop({id,i:'grid'}));break;}
    case 'linefirst':case 'spacefirst':addDivider(act.startsWith('line')?'line':'space',a.id,'');break;
    case 'lineafter':case 'spaceafter':addDivider(act.startsWith('line')?'line':'space',contOf(a.id),a.id);break;
    case 'decodel':removeDivider(a.id);break;
    case 'delobj':if(!ctx0||!ctx0.ask){S.ctx={...ctx0,ask:true};loadUses(a.id);break;}deleteObject(a.id);break;
    case 'trash':openDoc('trash:');loadTrash();break;
    case 'problems':openDoc('problems:');loadProblems();break;
    case 'trrestore':restoreTrash(a.id);break;
    case 'way':setWay(+a.k,a.way);break;
    case 'waysgo':endWays(true);break;
    case 'waysno':endWays(false);break;
    case 'restoreremoved':removedAct(a.id,'restore');break;
    case 'edselcopy':case 'edselembed':case 'edselvalue':{const e=t&&EDS[t.d];if(!e)return;
      if(act==='edselvalue'){const n=document.querySelector(`[data-edvalname="${t.d}"]`),name=n?n.value.trim():'';if(!name){toast('Type a name for the value first.');if(n)n.focus();return;}markOrCopy(e,true,name);return;}
      markOrCopy(e,act==='edselembed');return;}
    case 'keepcopy':removedAct(a.id,'copy',a.page);break;
    case 'trpurge':if(hadAsk!==a.id){S.trashAsk=a.id;break;}purgeTrash(a.id);break;
    case 'trempty':if(hadAsk!=='all'){S.trashAsk='all';break;}emptyTrash();break;
    case 'winmenu':{if(hadCtx==='win')break;const z=S.scale/100,r=b.getBoundingClientRect();S.ctx={kind:'win',x:Math.round(r.left/z),y:Math.round(r.bottom/z)+2};break;}
    case 'dupl':copyTo([a.id],contOf(a.id)||'',a.id);break;
    case 'export':location.href='/api/objects/'+encodeURIComponent(a.id)+'/export';break;
    case 'tokind':convertKind(a.id,a.k);break;
    case 'show':if(!aliasOf(a.id))C.push(asAlias(a.id));break;
    case 'unshow':{const k=C.findIndex(c=>c.alias&&c.id===a.id);if(k>=0)C.splice(k,1);delete S.pin[a.id];delete S.open[a.id];break;}
    case 'sortaz':case 'sortza':sortKids(a.id,act==='sortaz'?1:-1);break;
    case 'selopen':if(S.sel){if(S.sel.z==='n2')S.sel.ids.forEach(x=>{if(!isDeco(x))S.open[x]=true;});else S.sel.ids.forEach(openAsTab);}S.sel=null;break;
    case 'selpin':if(S.sel)S.sel.ids.forEach(x=>{if(!isDeco(x)){S.pin[x]=true;S.open[x]=true;}});S.sel=null;break;
    case 'selclear':S.sel=null;break;
    /* Settings and name open the overview of the collection, where they are, ready to rename. */
    case 'cgear':{openDoc(a.id);render();const f=document.getElementById('cname');if(f){f.focus();f.select();}return;}
    case 'addmenu':if(hadAdd!==a.id)openNew(a.id,b.closest('header')||b);break;
    case 'lineend':case 'spaceend':{const k=D[a.id].kids;addDivider(act==='lineend'?'line':'space',a.id,k[k.length-1]||'');break;}
    case 'add':if(a.id==='page'&&D[a.c]&&D[a.c].db){S.addIn=null;createPage(a.c,'New page');break;}
      if(a.id==='folder'&&D[a.c]&&D[a.c].db){S.addIn=null;createFolder(a.c,freeName('New folder'));break;}{const id='new'+(++S.newN),k=a.id;D[id]={t:k,n:'New '+TYPE[k][1].toLowerCase(),w:'slim',toc:'marker',tags:[],kids:[],path:'not saved, this is a mockup',by:'you',ch:'now',vis:'As its collection',...none,stat:'empty',html:'<p class="text-muted-foreground text-sm">Empty. Drag it in the list to where it belongs.</p>'};C.find(c=>c.id===a.c).top.push(id);rebuildPlaces();openDoc(id);S.just=id;break;}
    case 's1':S.s1=+a.id;break;
    case 's2':S.s2=+a.id;break;
    case 'w3reset':S.w3=W3;break;
    case 'theme':S.theme=a.id;break;
    case 'openedit':S.openEdit=a.id==='true';break;
    case 'scale':S.scale=+a.id===0?100:Math.min(150,Math.max(70,S.scale+ +a.id));break;
    case 'mod':S.mods[a.id]=!S.mods[a.id];break;
    case 'strat':S.strat=a.id;break;
    case 'ai':S.ai=!S.ai;break;
    case 'filters':S.filters=!S.filters;break;
    case 'recent':S.sq=a.id;break;
    case 'ftag':{const g=esc(a.tag);S.ftags=S.ftags.includes(g)?S.ftags.filter(x=>x!==g):[...S.ftags,g];break;}
    case 'tagdel':{const id=t.d,d=D[id];saveTags(id,d.tags.filter(x=>x!==a.tag).map(unesc));return;}
    case 'tagsearch':if(S.view!=='search')S.prev=S.view;S.view='search';S.rail=true;S.sq='';S.ftype='';S.ftags=[esc(a.tag)];S.filters=true;if(udSearch.real())udSearch.tag(a.tag);break;
    case 'sel':w.a=tk;S.focus=i;break;
    case 'keep':{const c=take(w,0);w.tabs.push(c);w.a=w.tabs.length-1;break;}
    case 'close':if(w.tabs[tk].general)popGeneral(w);else{w.tabs.splice(tk,1);if(w.a>=tk&&w.a>0)w.a--;}compact();break;
    case 'menu':S.menu=hadMenu===i?null:i;break;
    case 'dup':{const s0=w.tabs[tk];w.tabs.splice(tk+1,0,{...s0,general:false,gear:false,fly:false});w.a=tk+1;break;}
    case 'move':placeTab(i,tk,+a.id,Infinity);break;
    case 'newwin':newColumn(i,tk,+a.id);break;
    case 'closeothers':closeWhere(i,j=>j===tk);break;
    case 'closeright':closeWhere(i,j=>j<=tk);break;
    case 'closeleft':closeWhere(i,j=>j>=tk);break;
    case 'closeall':closeWhere(i,()=>false);break;
    case 'closewin':S.wins.splice(i,1);if(!S.wins.length)S.wins=[{tabs:[newTab(null,true)],a:0,hist:[]}];S.focus=Math.min(Math.max(0,i-1),S.wins.length-1);break;
    case 'copypath':S.toast='Path copied: '+D[w.tabs[tk].d].path;setTimeout(()=>{S.toast='';render();},1600);break;
    case 'hist':case 'tablist':{const z=S.scale/100,r=b.getBoundingClientRect();S.menuAt={x:Math.round(r.left/z),y:Math.round(r.bottom/z)+2};
      if(act==='hist')S.hist=hadHist===i?null:i;else S.tlist=hadTl===i?null:i;break;}
    case 'split':S.wins.splice(i+1,0,{tabs:[newTab(null,true),{...t,general:false,gear:false,fly:false}],a:1,hist:[]});S.focus=i+1;break;
    case 'rights':if(window.UDRights&&D[a.id]&&D[a.id].db)UDRights.open(D[a.id].sid,D[a.id].n);return;
    case 'gear':t.gear=!t.gear;if(t.gear){t.cmt=false;if(D[t.d].blocks)loadVersions(t.d);}break;
    case 'edit':setEdit(t,!editing(t));t.src=false;if(editing(t))S.focusNext={d:t.d};break;
    case 'edfmt':{const e=EDS[t.d];if(e)e.ed.format(a.k);return;}
    /* Passwords in the text (ADR-7, secrets.js): asked for in a dialog, kept sealed, and the reference goes where the cursor is. */
    case 'edsecret':{const e=EDS[t.d];if(e&&window.UDSecrets)UDSecrets.make(D[t.d].sid).then(ref=>{if(ref&&EDS[t.d]===e)e.ed.insert(ref);});return;}
    case 'secrets':if(window.UDSecrets)UDSecrets.open(D[a.id].sid,D[a.id].n);return;
    case 'edtable':{const e=EDS[t.d];if(e&&!e.ed.table(a.k))toast(a.k==='rowDelete'?'The header row stays: a table needs one.':a.k==='colDelete'?'The last column stays: a table needs one.':'The cursor is not in a table.');return;}
    case 'edmove':{const e=EDS[t.d];if(e)e.ed.moveBlock(a.id,a.k);break;}
    case 'eddel':{const e=EDS[t.d];if(e)e.ed.deleteBlock(a.id);break;}
    case 'balign':alignBlock(t.d,a.id,a.k);break;
    case 'edcopy':{const text=S.edNote&&S.edNote[t.d];if(text)(navigator.clipboard?navigator.clipboard.writeText(text):Promise.reject()).then(()=>toast('What you wrote is on the clipboard.'),()=>toast('The clipboard could not be written.'));return;}
    case 'ednote':if(S.edNote)delete S.edNote[t.d];break;
    case 'bcmt':commentOn(t.d,a.id);return;
    case 'bmenu':if(!(hadBm&&hadBm.w===i&&hadBm.id===a.id))S.bmenu={w:i,id:a.id};S.bmove=null;break;
    case 'bmoveto':S.bmenu={w:i,id:a.id};S.bmove={w:i,id:a.id};break;
    case 'bmovego':{const sel=document.getElementById('bmovesel'),target=sel&&sel.value;if(!target){toast('Choose a page first.');S.bmenu={w:i,id:a.id};return;}
      moveToPage(i,t.d,a.id,target);return;}
    case 'blkedit':setEdit(t,true);t.src=false;S.focusNext={d:t.d,id:a.id};break;
    case 'blkaddr':{const addr=location.origin+'/o/'+encodeURIComponent(D[t.d].sid)+'#'+a.id;
      (navigator.clipboard?navigator.clipboard.writeText(addr):Promise.reject()).then(()=>toast('Address copied. Paste it alone into a block to embed this one.'),()=>toast(addr));return;}
    case 'src':t.src=!t.src;t.top=0;if(t.src)setEdit(t,false);break;
    case 'find':t.find=!t.find;if(t.find){render();const f=document.querySelector(`[data-findq="${i}"]`);if(f)f.focus();return;}break;
    case 'findnav':t.cur+=+a.id;render();{const c=document.querySelector(`[data-sc="${i}"] mark[aria-current]`);if(c)c.scrollIntoView({block:'center'});}return;
    case 'fly':t.fly=!t.fly;break;
    case 'jump':t.fly=false;render();{const x=document.getElementById(a.id);if(x)x.scrollIntoView({block:'start'});}return;
    case 'settoc':saveView(t.d,'toc','contents',a.id);break;
    case 'setw':saveView(t.d,'w','width',a.id);break;
    case 'slide':D[t.d].cur=+a.id;break;
    case 'clock':S.clock=!S.clock;break;
  }
  render();
},true);
function refreshHits(){const r=document.getElementById('sres');if(r)r.innerHTML=resultsHtml();}
document.addEventListener('input',e=>{
  if(e.target.dataset&&e.target.dataset.nfilter!==undefined){S.addQ=e.target.value;filterNew(S.addQ);return;}
  const x=e.target;
  if(x.id==='search'){S.sq=x.value;searchServer();if(S.view!=='search'){S.prev=S.view;S.view='search';S.rail=true;render();const f=document.getElementById('search');f.focus();f.setSelectionRange(f.value.length,f.value.length);}else refreshHits();}
  if(x.id==='cq'){S.cq=x.value;render();const f=document.getElementById('cq');if(f){f.focus();f.setSelectionRange(f.value.length,f.value.length);}}
  if(x.dataset.findq!==undefined){const i=+x.dataset.findq,t=at(i);t.q=x.value;t.cur=0;render();const f=document.querySelector(`[data-findq="${i}"]`);if(f){f.focus();f.setSelectionRange(f.value.length,f.value.length);}}
});
let rendering=false;
/* The search field of the top row sits in C22's form[role=search]; Enter is handled with the keys, nothing is sent. */
document.addEventListener('submit',e=>{if(e.target.matches('form[role="search"]'))e.preventDefault();});
document.addEventListener('focusout',e=>{const x=e.target;if(!rendering&&x.dataset&&x.dataset.ren!==undefined)renameDone(x);});
document.addEventListener('change',e=>{
  const x=e.target;
  /* A to-do is ticked by its checkbox: one of a private list (tick), or of the to-do list in a window (todo). */
  if(x.type==='checkbox'&&(x.dataset.act==='tick'||x.dataset.act==='todo')){
    if(x.dataset.act==='tick')TL.find(l=>l.id===x.dataset.l).items[+x.dataset.id].done=x.checked;
    else{const w=x.closest('[data-win]'),t=at(w?+w.dataset.win:S.focus);if(t&&D[t.d].items)D[t.d].items[+x.dataset.id].done=x.checked;}
    render();return;}
  if(x.dataset.edtype!==undefined){const e=EDS[x.dataset.edtype];if(e)e.ed.turnInto(x.value);return;}
  if(x.dataset.wmaster!==undefined){if(x.value)setWay(+x.dataset.wmaster,'master',x.value);return;}
  if(x.dataset.wsame!==undefined&&S.ways){S.ways.same=x.checked;const f=S.ways.items[0];if(x.checked&&f.way)setWay(0,f.way,f.master);else render();return;}
  if(x.id==='limit'){S.limit=Math.max(0,+x.value||0);S.tog={};render();}
  if(x.id==='stype'){S.ftype=x.value;render();}
  if(x.dataset.cname!==undefined){const v=x.value.trim(),cid=x.dataset.cname;if(v){D[cid].n=esc(v);if(D[cid].real)renameObject(cid,v);}render();}
});
document.addEventListener('contextmenu',e=>{const tb=e.target.closest&&e.target.closest('.workspace-tab[data-dw]');
  if(tb){e.preventDefault();const i=+tb.dataset.dw,z=S.scale/100,r=tb.getBoundingClientRect();S.ctx=null;S.menu=null;S.newAt=null;S.hist=null;S.tlist=null;S.focus=i;S.tmenu={w:i,t:+tb.dataset.dt,x:Math.round(r.left/z),y:Math.round(r.bottom/z)+2};render();return;}
  const el=e.target.closest?e.target:null;if(!el)return;
  const z=S.scale/100,at0=(kind,id,more={})=>{e.preventDefault();S.menu=null;S.newAt=null;S.hist=null;S.tlist=null;S.tmenu=null;S.ctx={kind,id,x:Math.round(Math.min(e.clientX/z,innerWidth/z-250)),y:Math.round(Math.min(e.clientY/z,innerHeight/z-240)),...more};render();};
  const sl=S.sel&&!S.sel.soft&&S.sel.ids.length>1&&el.closest('[aria-selected="true"]');if(sl)return at0('sel',null);
  const cl=el.closest('[data-ctx="coll"]');if(cl)return at0('coll',cl.dataset.ctxid,{n3:!!cl.closest(N3)});
  const dc=el.closest('[data-ctx="deco"]');if(dc)return at0('deco',dc.dataset.ctxid);
  const rw=el.closest('[data-row]');if(rw)return at0('row',rw.dataset.row);
  const op=el.closest(`${N3} :is(li > div, label.item) [data-act="open"]`);if(op&&D[op.dataset.id])return at0('row',op.dataset.id);
  const sh=el.closest(`${N3} [data-sechead]`);if(sh)return at0('sec',sh.dataset.sechead,{label:sh.textContent.trim(),sort:false});
  const sc=el.closest(`${N2} [data-act="sec"]`);if(sc)return at0('sec',sc.dataset.id,{label:sc.textContent.trim(),sort:!!sc.dataset.secid});
  const vw=el.closest(`${N1} [data-act="view"]`);if(vw)return at0('view',vw.dataset.id,{label:vw.textContent.trim()});
  /* A right click on the free part of nav2 or nav3 offers what can be made there. */
  if(el.closest(N2)){const acts=S.view==='workspace'?[['newcoll','New collection'],['n2line','Line at the end'],['n2space','Space at the end'],...(S.canCreate?[['-'],['problems','Problems'],['trash','Trash']]:[['-'],['problems','Problems']])]:SECTIONS[S.view]?SECTIONS[S.view]().filter(x=>x[3]).map(x=>['sec:'+x[0],x[1]]):[];if(acts.length)return at0('free',null,{acts});}
  if(el.closest(N3)&&S.view==='workspace'){const acts=[...C.filter(c=>S.open[c.id]&&accOf(c.id)==='edit').map(c=>['addmenu2:'+c.id,'New in '+c.n]),['newcoll','New collection']];return at0('free',null,{acts});}
});
/* A middle click closes a tab, a double click on the empty part of the tab row opens New, as in code editors. */
document.addEventListener('auxclick',e=>{const op=e.button===1&&e.target.closest&&e.target.closest(`${N3} [data-act="open"]`);if(op&&D[op.dataset.id]){e.preventDefault();openTab(op.dataset.id);render();return;}
  const tb=e.button===1&&e.target.closest&&e.target.closest('.workspace-tab[data-dw]');if(!tb)return;e.preventDefault();const i=+tb.dataset.dw,k=+tb.dataset.dt,w=S.wins[i];
  if(w.tabs[k].general)popGeneral(w);else{w.tabs.splice(k,1);if(w.a>=k&&w.a>0)w.a--;}compact();render();});
document.addEventListener('dblclick',e=>{const r=e.target.closest&&e.target.closest('.workspace-tabs');if(!r||e.target.closest('.workspace-tab,button'))return;const i=+r.closest('[data-win]').dataset.win,z=S.scale/100;S.newPos={x:Math.round(e.clientX/z),y:Math.round(r.getBoundingClientRect().bottom/z)+2};S.focus=i;S.newAt=i;render();const f=document.querySelector('[data-quick]');if(f)f.focus();});
/* The buttons of the bar and of the handles keep the focus and the selection in the editor. */
document.addEventListener('mousedown',e=>{if(e.target.closest&&e.target.closest('.editor-bar button'))e.preventDefault();});
document.addEventListener('keydown',e=>{
  const x=e.target;
  if(cmtKey(x,e))return;
  if(x.dataset&&x.dataset.grip&&(e.key==='ArrowLeft'||e.key==='ArrowRight')){const n=x.dataset.grip,dir=e.key==='ArrowLeft'?-1:1;if(n==='3')S.w3=clampW3(S.w3+dir*20);else{const k='s'+n;S[k]=Math.min(3,Math.max(0,S[k]+dir));}render();const g=document.querySelector(`[data-grip="${n}"]`);if(g)g.focus();e.preventDefault();return;}
  if(e.key==='/'&&!x.closest('input,select,textarea,[contenteditable]')){e.preventDefault();if(!document.getElementById('search')){S.view='search';S.rail=true;render();}document.getElementById('search').focus();return;}
  if(x.dataset&&x.dataset.nfilter!==undefined&&['ArrowDown','ArrowUp','Enter'].includes(e.key)){e.preventDefault();if(e.key==='Enter'){const n=document.querySelector('[data-menu="new"] [role="menuitem"].active')||[...document.querySelectorAll('[data-menu="new"] [role="menuitem"]')].find(n=>n.getAttribute('aria-hidden')!=='true'&&!n.disabled);if(n)n.click();}else stepNew(e.key==='ArrowDown'?1:-1);return;}
  if(x.dataset&&x.dataset.ren!==undefined&&(e.key==='Enter'||e.key==='Escape')){e.preventDefault();if(e.key==='Escape')x.value=x.defaultValue;x.blur();return;}
  if(e.key==='Escape'&&S.ways){endWays(false);return;}
  if(e.key==='Escape'&&(S.newAt!==null||S.ctx||S.addIn||S.tmenu||S.hist!==null||S.tlist!==null)){S.newAt=null;S.ctx=null;S.addIn=null;S.tmenu=null;S.hist=null;S.tlist=null;render();return;}
  if(e.key==='Escape'&&S.sel&&!S.sel.soft&&!drag){S.sel=null;render();return;}
  if(e.key!=='Enter')return;
  if(x.dataset.quick!==undefined){const v=x.value.trim();if(!v)return;S.newAt=null;if(S.into==='qn'&&S.canCreate){quickNote(v);render();return;}else if(S.into==='qn'){D.qn.notes.unshift({h:'',b:v});openDoc('qn');}else if(D[S.into]&&D[S.into].db)createPage(S.into,v.split('\n')[0].slice(0,200));else if(madeHere(S.into)){toast(madeHere(S.into)+'.');return;}else openDoc(createIn(S.into,'page',v));render();return;}
  if(x.dataset.aiq!==undefined){const v=x.value.trim();if(!v)return;S.chat.push({q:v,a:aiAnswer()});render();const n=document.querySelector('[data-aiq]');if(n)n.focus();return;}
  if(x.id==='search'&&udSearch.real()){const h=udSearch.first();if(h)udSearch.openHit(h.id,h.anchor,h.heading,h.snippet,S.sq);return;}
  if(x.id==='search'){const h=hitsWithServer();if(h&&h.length){openDoc(h[0]);render();const f=document.getElementById('search');if(f)f.focus();}}
  if(x.dataset.addtag!==undefined&&x.dataset.cname===undefined){const v=x.value.trim().toLowerCase().replace(/^#/,''),id=at(+x.dataset.addtag).d,d=D[id];
    if(d.db){if(v&&!d.tags.includes(esc(v)))saveTags(id,[...d.tags.map(unesc),v]);return;}
    if(v&&!d.tags.includes(esc(v)))d.tags.push(esc(v));render();}
  if(x.dataset.addcmt!==undefined){const v=x.value.trim(),d=D[at(+x.dataset.addcmt).d];const tt=at(+x.dataset.addcmt);if(v&&tt.cb)(d.comments=d.comments||[]).push({b:tt.cb,l:tt.cbl,who:'A. Keller',when:'now',t:v});render();const n=document.querySelector('[data-addcmt]');if(n)n.focus();return;}
  if(x.dataset.addnote!==undefined&&D[at(S.focus).d].blocks){const v=x.value.trim();if(v){x.value='';addCard(at(S.focus).d,v);}return;}
  if(x.dataset.addnote!==undefined){const v=x.value.trim(),d=D[at(S.focus).d];if(v)d.notes.unshift({h:'',b:v});render();const n=document.querySelector('[data-addnote]');if(n)n.focus();}
  if(x.dataset.findq!==undefined){const i=+x.dataset.findq,t=at(i);t.cur++;render();const c=document.querySelector(`[data-sc="${i}"] mark[aria-current]`);if(c)c.scrollIntoView({block:'center'});const f=document.querySelector(`[data-findq="${i}"]`);if(f)f.focus();}
});
/* Dragging runs on pointer events: it starts anywhere on the item, also on its text, and the whole row counts as the target. */
let drag=null,noClick=false;
const clampW3=v=>Math.min(640,Math.max(180,Math.round(v)));
/* Where a dragged tab or entry would land in a window: one of C22's drop zones of .workspace-main, shown with its word. */
function markZone(win,kind,word){document.querySelectorAll('.workspace-main > [data-drop-zone]').forEach(n=>{n.hidden=true;});
  const z=win&&win.querySelector(`.workspace-main > [data-drop-zone="${kind}"]`);if(z){z.textContent=word;z.hidden=false;}}
const clearMarks=()=>markZone(null);
function startGrip(g,e){
  const n=g.dataset.grip,z=S.scale/100,x0=e.clientX,w0=n==='3'?S.w3:STAGES[S['s'+n]];
  const move=m=>{const want=w0+(m.clientX-x0)/z;
    if(n==='3'){S.w3=clampW3(want);document.querySelector('.workspace').style.setProperty('--workspace-nav3',S.w3+'px');return;}
    let best=0;STAGES.forEach((w,k)=>{if(Math.abs(w-want)<Math.abs(STAGES[best]-want))best=k;});
    if(best!==S['s'+n]){S['s'+n]=best;render();}};
  const up=()=>{removeEventListener('pointermove',move);removeEventListener('pointerup',up);S.resizing=false;render();};
  S.resizing=true;document.querySelector('.workspace').setAttribute('data-resizing','');addEventListener('pointermove',move);addEventListener('pointerup',up);e.preventDefault();
}
document.addEventListener('pointerdown',e=>{
  noClick=false;
  if(e.button!==0||!e.target.closest)return;
  const g=e.target.closest('[data-grip]');
  if(g){startGrip(g,e);return;}
  if(e.pointerType==='touch'||e.target.closest('.workspace-tab > button:not([role="tab"]),[data-toggle],header .btn,'+POPS+',input,textarea,select'))return;
  const tab=e.target.closest('.workspace-tab[data-dw]');
  if(tab)drag={kind:'tab',src:tab,w:+tab.dataset.dw,t:+tab.dataset.dt,label:tab.querySelector('[role="tab"] span').textContent};
  if(drag){drag.x0=e.clientX;drag.y0=e.clientY;drag.on=false;drag.to=null;}
});
/* Tabs snap: the window comes from the pointer's x, the row has a tolerance around it, and the tab takes its place while dragging. */
function winAt(e){
  return [...document.querySelectorAll('[data-win]')].find(w=>{const r=w.getBoundingClientRect();return e.clientX>=r.left&&e.clientX<r.right&&e.clientY>=r.top-24&&e.clientY<=r.bottom;});
}
function tabTarget(e){
  const w=winAt(e);if(!w)return null;
  const i=+w.dataset.win,row=w.querySelector('.workspace-tabs'),rr=row.getBoundingClientRect();
  if(e.clientY<=rr.bottom+10){
    const tabs=[...row.querySelectorAll('.workspace-tab:not([data-general]):not([data-drop="slot"])')].filter(n=>!(+n.dataset.dw===drag.w&&+n.dataset.dt===drag.t));
    /* The general tab stays while it is dragged, so its slot adds width. Measure as if the slot were not there, or it pushes its own target away. */
    const slot=drag.src.hasAttribute('data-general')&&row.querySelector('#slot'),sw=slot?slot.getBoundingClientRect().width+2:0;
    let seq=tabs.findIndex(n=>{const r=n.getBoundingClientRect(),sh=slot&&(slot.compareDocumentPosition(n)&Node.DOCUMENT_POSITION_FOLLOWING)?sw:0;return e.clientX<r.left-sh+r.width/2;});
    if(seq<0)seq=tabs.length;
    return {type:'row',i,seq,idx:seq+1,row,before:tabs[seq]||null};
  }
  const r=w.getBoundingClientRect(),band=r.width*.3;
  if(S.wins.length<4){if(e.clientX<r.left+band)return {type:'new',pos:i,node:w,zone:'start',word:'New column'};if(e.clientX>r.right-band)return {type:'new',pos:i+1,node:w,zone:'end',word:'New column'};}
  return i===drag.w?null:{type:'win',i,node:w,zone:'here',word:'Move here'};
}
function showSlot(t){
  let s=document.getElementById('slot');
  if(!t||t.type!=='row'){if(s)s.remove();return false;}
  if(!s){s=drag.src.cloneNode(true);s.id='slot';s.style.display='';s.removeAttribute('data-general');s.removeAttribute('data-stack');s.dataset.drop='slot';s.removeAttribute('data-dw');s.removeAttribute('data-dt');}
  const box=t.row.querySelector('[role="tablist"]');
  if(t.before)box.insertBefore(s,t.before);else box.append(s);
  return true;
}
const within=(id,a)=>{for(let x=a;x;x=PARENT[x])if(x===id)return true;return false;};
/* Where a drop puts the entry: the container it goes into. An object of the database goes only into the database, where
   one may edit, and made-up content stays made up. */
function dropCont(t){return t.type==='coll'?t.c:t.type==='into'?t.id:t.type==='nav2'?'':contOf(t.id);}
/* Within its place an entry is reordered by whoever may edit; to another place it goes only with the role manage on it
   (ADR-29), so the gap does not open there for whoever may only edit. */
function fits(id,t){const c=dropCont(t);if(!D[id].db)return c===''||!D[c]||!D[c].db;if(c!==(contOf(id)||'')&&!isOwner(id))return false;if(c==='')return D[id].t==='coll';return !!D[c]&&!!D[c].db&&accOf(c)==='edit'&&!isDeco(c);}
/* Dragging an entry of nav2 or nav3 is C22's (app-workspace-drag.js, approved here on 2026-11-09): it carries the entry
   through the list as it looks when held, up and down between entries, onto one after waiting, deeper to the right. What
   may go where, and what a drop does, is decided here. The entries selected along with it go too; on a collection in
   nav2 an entry goes into it, on a window it opens there as tabs, and with Ctrl held at the drop it is copied. */
const selOf=kind=>kind==='row'?'n3':kind==='fav'?'n2':'sec';
const elOf=(kind,id)=>document.querySelector(kind==='row'?`${N3} [data-row="${id}"]`:kind==='fav'?`#clist [data-favid="${id}"]`:`${N2} [data-secid="${id}"]`);
const kindOfEl=n=>n.matches('[data-row]')?'row':n.matches('[data-favid]')?'fav':'sec';
function depthOf(n){let k=-1;for(let x=n.parentElement;x&&x.id!=='tree';x=x.parentElement)if(x.matches('ul'))k++;return k;}
/* May every dragged entry go into container c? */
function mayInto(d,c,head){
  if(accOf(c)==='read')return false;const probe=head?{type:'coll',c}:{type:'into',id:c};
  /* Private notes stand at the top, never inside another collection. */
  return d.ids.every(x=>fits(x,probe)&&!D[x].private&&!(d.kind==='fav'&&(c===x||aliasOf(x)||(COLL[c]&&COLL[c].id===x)))&&!(d.kind==='row'&&within(x,c)));
}
const DRAG={
  scale:()=>S.scale/100,
  strip:['data-row','data-favid','data-secid'],
  pick(e){
    if(!e.target.closest||e.target.closest('[data-toggle],header .btn,'+POPS+',input,textarea,select,[data-grip]'))return null;
    const el=e.target.closest('[data-row],[data-favid],[data-secid]');if(!el||el.closest('.dropgap'))return null;
    const kind=kindOfEl(el),id=el.dataset.row||el.dataset.favid||el.dataset.secid;
    if(kind==='row'&&contAcc(id)==='read')return null;
    /* An entry of a selection takes the others along, in the order of the list; in nav3 one below another selected one
       goes with that one anyway, and what may only be read stays. */
    const many=S.sel&&S.sel.z===selOf(kind)&&S.sel.ids.includes(id)?S.sel.ids.filter(x=>elOf(kind,x)&&(kind!=='row'||contAcc(x)!=='read')):[id];
    const ids=kind==='row'?many.filter(x=>!many.some(y=>y!==x&&within(y,PARENT[x]))):many;
    ids.sort((a,b)=>elOf(kind,a).compareDocumentPosition(elOf(kind,b))&Node.DOCUMENT_POSITION_FOLLOWING?-1:1);
    return {kind,id,ids,view:S.view,els:ids.map(x=>elOf(kind,x)),el};
  },
  again(d){const el=elOf(d.kind,d.id);return el?{el,els:d.ids.map(x=>elOf(d.kind,x)).filter(Boolean)}:null;},
  hideOf:el=>el.matches('[data-row]')?el.closest('li'):el,
  home:d=>selOf(d.kind),
  depthOf,
  zones(d){
    const z=S.scale/100,out=[],n2=document.querySelector(N2),list=document.getElementById('clist');
    if(list&&d.kind!=='sec')out.push({key:'n2',box:n2,scroll:list,flat:true,rows:[...list.querySelectorAll(':scope>[data-favid]')].map(n=>({el:n,id:n.dataset.favid,hold:!isDeco(n.dataset.favid)}))});
    if(d.kind==='sec'&&n2)out.push({key:'sec',box:n2,scroll:n2.querySelector(':scope>div')||n2,flat:true,rows:[...n2.querySelectorAll('[data-secid]')].map(n=>({el:n,id:n.dataset.secid}))});
    const tr=document.getElementById('tree'),top=tr&&tr.querySelector('ul.workspace-tree');
    if(top&&(d.kind==='row'||(d.kind==='fav'&&!d.ids.some(x=>isDeco(x)||aliasOf(x))))){
      const p=document.createElement('ul');top.append(p);const indent=parseFloat(getComputedStyle(p).paddingLeft)*z;p.remove();
      out.push({key:'n3',box:tr.closest(N3),scroll:tr,flat:false,base:top.getBoundingClientRect().left,indent,
        rows:[...tr.querySelectorAll('header[data-cdrop],li > div[data-row]')].filter(n=>!n.closest('.dropgap')).map(n=>n.matches('header')?{el:n,id:n.dataset.cdrop,head:true,depth:-1,hold:true}:{el:n,id:n.dataset.row,depth:depthOf(n),hold:!isDeco(n.dataset.row)})});
    }
    return out;
  },
  /* In nav2 an entry that is not a collection can only go into one: at once, wherever on it. */
  ontoOnly:(d,z)=>z.key==='n2'&&d.kind==='row'&&!d.ids.every(x=>D[x].t==='coll'),
  waits:z=>z.key!=='sec',
  accept(d,z,at){markZone(null);
    if(at.onto){const c=at.onto.id;if(!mayInto(d,c,at.onto.head))return null;const kids=D[c].kids.filter(x=>!d.ids.includes(x));return {p:c,after:kids[kids.length-1]||''};}
    const after=at.after?at.after.id:'';
    if(z.key==='sec')return {after};
    if(z.key==='n2')return d.ids.every(x=>d.kind==='fav'||fits(x,{type:'nav2'}))?{root:true,after}:null;
    return mayInto(d,at.parent.id,at.parent.head)?{p:at.parent.id,after}:null;
  },
  outside(d,e){const win=d.kind==='sec'||d.ids.some(isDeco)?null:winAt(e);markZone(win,'here','Open here');return win?{win:+win.dataset.win,node:win}:null;},
  drop(d,t,e){d.copy=!!(e&&(e.ctrlKey||e.metaKey));applyDrop(d,t);},
  end(){render();}
};
/* Tabs stay this app's own: a label at the pointer, and the tab takes its place in a row or a new window. */
addEventListener('pointermove',e=>{
  if(!drag)return;
  const z=S.scale/100;
  if(!drag.on){if(Math.hypot(e.clientX-drag.x0,e.clientY-drag.y0)<6)return;drag.on=true;document.body.classList.add('dragging');
    /* The label at the pointer is C22's badge; the general tab stays where it is while it is dragged. */
    const g=document.createElement('div');g.id='ghost';g.className='badge pointer-events-none fixed z-50 shadow-md';g.dataset.variant='secondary';g.textContent=drag.label;document.body.append(g);if(!drag.src.hasAttribute('data-general'))drag.src.style.display='none';}
  const g=document.getElementById('ghost');
  g.style.left=(e.clientX/z+12)+'px';g.style.top=(e.clientY/z+12)+'px';
  drag.to=tabTarget(e);clearMarks();const t=drag.to;
  g.style.display=showSlot(t)?'none':'';
  if(t&&t.node)markZone(t.node,t.zone,t.word);
});
function parentList(id){return PARENT[id]?D[PARENT[id]].kids:COLL[id]?COLL[id].top:C.find(c=>c.id===id)?[]:[];}
/* A drop takes the entries where they are held, in their order, or copies them there with Ctrl. In the database the
   server moves them too, one after the other, and if it refuses, the lists go back. */
function applyDrop(d,t=d.to){
  if(!t)return;
  if(d.kind==='tab'){
    if(t.type==='row'){if(t.i!==d.w||t.idx!==d.t)placeTab(d.w,d.t,t.i,t.idx);else{S.wins[d.w].a=d.t;S.focus=d.w;}}
    else if(t.type==='new')newColumn(d.w,d.t,t.pos);
    else placeTab(d.w,d.t,t.i,Infinity);return;}
  const ids=d.ids;
  if(t.win!==undefined){S.focus=t.win;ids.forEach(openAsTab);return;}
  const w=t.zone.key;
  if(w==='sec'){const L=SORT[d.view],ms=ids.map(id=>L.splice(L.findIndex(x=>x.id===id),1)[0]);L.splice(t.after?L.findIndex(x=>x.id===t.after)+1:0,0,...ms);return;}
  if(d.copy){copyTo(ids.filter(x=>!isDeco(x)||D[x].db),t.root?'':t.p,t.after);return;}
  const dest=t.root?C:D[t.p].kids,froms=ids.map(id=>d.kind==='fav'?C:parentList(id)),snap=[...new Set([C,dest,...froms])].map(l=>[l,l.slice()]);
  const ms=ids.map((id,i)=>{if(d.kind==='fav')return C.splice(C.findIndex(c=>c.id===id),1)[0];froms[i].splice(froms[i].indexOf(id),1);return null;});
  if(t.root)C.splice(t.after?C.findIndex(c=>c.id===t.after)+1:0,0,...ids.map((id,i)=>{if(ms[i])return ms[i];S.open[id]=true;return asTop({id,i:D[id].i||'grid'});}));
  else{ids.forEach((id,i)=>{if(ms[i]){delete S.open[id];delete S.fav[id];delete S.pin[id];}});dest.splice(t.after?dest.indexOf(t.after)+1:0,0,...ids);S.tog[t.p]=true;S.just=d.id;}
  rebuildPlaces();
  /* nav2 keeps its order in the browser; only a move into or out of a collection reaches the server. */
  if(d.kind==='fav'&&t.root)return;
  const moves=ids.filter(id=>D[id].db).map(id=>[id,t.root?'':t.p,t.root?'':(ids.indexOf(id)?ids[ids.indexOf(id)-1]:t.after)]);
  moveAll(moves,()=>{snap.forEach(([l,c])=>{l.length=0;l.push(...c);});rebuildPlaces();render();});
}
function openAsTab(id){if(!D[id]||isDeco(id))return;openTab(id);if(D[id].real&&!D[id].loaded)loadDoc(id);}
addEventListener('pointerup',e=>{
  if(!drag)return;const d=drag;drag=null;
  if(!d.on)return;
  noClick=true;setTimeout(()=>{noClick=false;},60);
  document.body.classList.remove('dragging');const g=document.getElementById('ghost');if(g)g.remove();clearMarks();
  applyDrop(d);render();
});
addEventListener('keydown',e=>{if(e.key==='Escape'&&drag){const g=document.getElementById('ghost');if(g)g.remove();const on=drag.on;drag=null;document.body.classList.remove('dragging');clearMarks();if(on)render();}});
/* Documents that need the helpers above */
const fill=o=>{const def={w:'slim',toc:'marker',tags:[],kids:[],vis:'Only you',...none};for(const k in def)if(!(k in o))o[k]=def[k];return o;};
D['priv:time']=fill({t:'time',n:'This week',w:'wide',path:'not a file, private',place:'Private › Time',by:'you',ch:'today',stat:'31:40 this week',
  get html(){return `<p><button type="button" class="btn" data-variant="outline" data-act="clock" aria-pressed="${S.clock}">${icon('clock')}${S.clock?'Clock out, running since 08:02':'Clock in'}</button></p>${tableHtml(['Day','In','Out','Break','Hours'],[['Mon','07:58','16:40','0:30','8:12'],['Tue','08:05','17:02','0:30','8:27'],['Wed','08:00','16:15','0:30','7:45'],['Thu','08:10','15:56','0:30','7:16']])}`;}});
D['priv:vac']=fill({t:'vacation',n:'Vacation 2026',w:'wide',path:'not a file, private',place:'Private › Vacation',by:'you',ch:'Oct 30',stat:'7 days left',
  html:`<p>30 days this year: 18 taken, 5 planned, 7 left.</p>${tableHtml(['From','To','Days','State'],[['Mar 02','Mar 13','10','✓ taken'],['Jul 20','Jul 27','8','✓ taken'],['Nov 24','Nov 28','5','△ planned']])}`});
function todoDocs(){TL.forEach(l=>{
  D['tl:'+l.id]=fill({t:'todo',n:l.n,path:'not a file, private',place:'To-do',by:'you',ch:'today',
    get stat(){return `${l.items.filter(x=>x.done).length} of ${l.items.length} done`;},
    view:true,get html(){return l.items.length?`<div class="item-group" role="list">${l.items.map((x,k)=>todoItem(x,k,'tick',` data-l="${l.id}"`)).join('')}</div>`:'<p class="text-muted-foreground text-sm">Empty. Use + in nav3.</p>';}});
  l.items.forEach((x,k)=>{D[`ti:${l.id}/${k}`]=fill({t:'todo',n:x.t,path:'not a file, private',place:'To-do › '+l.n,by:'you',ch:'today',get stat(){return x.done?'done':'open';},
    get html(){return `<dl><dt>Due</dt><dd>${x.due||'–'}</dd><dt>List</dt><dd>${esc(l.n)}</dd></dl><div class="item-group" role="list">${todoItem({t:x.t,done:x.done},k,'tick',` data-l="${l.id}"`)}</div>`;}});});
});}
todoDocs();
/* The month is C22's sheet (table.month-sheet): November 2026 starts on a Sunday; today is the 7th, days outside the
   month are empty, an entry is a small in its day. */
const calGrid=cs=>{const ev={};cs.forEach(c=>Object.entries(c.ev).forEach(([d,n])=>(ev[d]=ev[d]||[]).push(n)));
  const cells=[...Array(6).fill(0),...Array.from({length:30},(_,k)=>k+1)];while(cells.length%7)cells.push(0);
  const day=(n,k)=>{const we=k%7>4?' data-weekend':'';return n?`<td${n===7?' aria-current="date"':''}${we}><span>${n}</span>${(ev[n]||[]).map(x=>`<small>${x}</small>`).join('')}</td>`:'<td data-outside></td>';};
  return `<table class="month-sheet"><caption class="sr-only">November 2026</caption><thead><tr>${['Monday','Tuesday','Wednesday','Thursday','Friday','Saturday','Sunday'].map(x=>`<th scope="col" abbr="${x}">${x.slice(0,3)}</th>`).join('')}</tr></thead><tbody>${Array.from({length:cells.length/7},(_,r)=>`<tr>${cells.slice(r*7,r*7+7).map((n,k)=>day(n,k)).join('')}</tr>`).join('')}</tbody></table>`;};
[['all','All calendars',CALS],...CALS.map(c=>[c.id,c.n,[c]])].forEach(([id,n,cs])=>{D['cal:'+id]=fill({t:'cal',n:n+', November 2026',w:'wide',path:'not a file, private',place:'Calendar',by:'you',ch:'today',stat:cs.reduce((a,c)=>a+Object.keys(c.ev).length,0)+' entries',view:true,html:calGrid(cs)+'<p class="text-muted-foreground mt-3 text-sm">Shared calendars from documents come with M-18.</p>'});});
CALS.forEach(c=>Object.entries(c.ev).forEach(([d,n])=>{D[`ev:${c.id}/${d}`]=fill({t:'cal',n,path:'not a file, private',place:'Calendar › '+c.n,by:'you',ch:'today',stat:'1 entry',html:`<dl><dt>When</dt><dd>Nov ${d}, 2026</dd><dt>Calendar</dt><dd>${c.n}</dd></dl>`});}));
D['cal:import']=fill({t:'cal',n:'Import a calendar',w:'wide',path:'not a file',place:'Calendar › Import',by:'you',ch:'now',stat:'nothing imported',html:`<div class="empty border" data-dropzone><header><figure>${icon('down')}</figure><h2>Drop a calendar file here</h2></header><section><button type="button" class="btn" data-variant="outline" disabled>Choose a file</button></section></div><p><input class="input" disabled placeholder="Or the address of a calendar to keep reading from" aria-label="Calendar address"></p><p class="text-muted-foreground text-sm">An imported calendar appears in nav2 next to your own, and can be sorted there.</p>`});
D['time:last']=fill({t:'time',n:'Last week',w:'wide',path:'not a file, private',place:'Time',by:'you',ch:'Nov 01',stat:'38:35',html:tableHtml(['Day','In','Out','Break','Hours'],[['Mon','08:00','16:30','0:30','8:00'],['Tue','07:55','16:30','0:30','8:05'],['Wed','08:00','17:00','0:30','8:30'],['Thu','08:10','16:30','0:30','7:50'],['Fri','08:00','14:40','0:30','6:10']])});
D['time:oct']=fill({t:'time',n:'October',w:'wide',path:'not a file, private',place:'Time',by:'you',ch:'Nov 01',stat:'157:35',html:tableHtml(['Week','Hours'],[['40','39:10'],['41','40:05'],['42','38:20'],['43','40:00']])});
D['time:export']=fill({t:'time',n:'Export the records',path:'not a file',place:'Time',by:'you',ch:'now',stat:'later',html:'<p class="text-muted-foreground text-sm">As a table, for a chosen period. Whether records feed anything official needs its own look first (M-24).</p>'});
D['vac:27']=fill({t:'vacation',n:'Vacation 2027',w:'wide',path:'not a file, private',place:'Vacation',by:'you',ch:'now',stat:'30 days left',html:'<p>30 days, none planned yet.</p>'});
D['vac:new']=fill({t:'vacation',n:'Plan a vacation',path:'not a file, private',place:'Vacation',by:'you',ch:'now',stat:'draft',html:'<p class="text-muted-foreground text-sm">From, to and a note. A request to someone else is a question for M-24.</p>'});
const boardDoc=(n,notes)=>fill({t:'notes',n,w:'full',path:'not a file, private notes',place:'Notes › '+n,by:'you',ch:'today',notes,get stat(){return `${this.notes.length} notes`;},
  get html(){return `<div class="flex flex-col gap-4"><input class="input mx-auto max-w-lg" type="text" data-addnote placeholder="Take a note, then Enter" aria-label="Take a note"><div class="grid grid-cols-2 gap-2 md:grid-cols-3" data-notes>${this.notes.map(x=>`<article class="card" data-size="sm"><section>${x.h?`<h4 class="font-semibold">${esc(x.h)}</h4>`:''}<p class="text-sm">${esc(x.b).replace(/\n/g,'<br>')}</p></section></article>`).join('')}</div></div>`;}});
D['nb:ideas']=boardDoc('Ideas',[{h:'Start page',b:'Show the tower height.'},{h:'',b:'A map of the site.'}]);
Object.assign(TYPE,{ipnet:['net','IP range'],devices:['hd','Device list'],cal:['cal','Calendar'],time:['clock','Time'],vacation:['sun','Vacation']});
const netDoc=(n,used,rows,txt)=>fill({t:'ipnet',n,w:'wide',path:'not a file, lives in the database',by:'A. Keller',ch:'Oct 28',vis:'Group ops, inherited',tags:['network'],stat:used+' used',html:`<p>${txt} The page <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="plan">Network plan</a> can embed this range, and a value can come from an address.</p>`+tableHtml(['Address','Name','Kind','Note'],rows)});
D.ip1=netDoc('192.0.2.0/24','4 of 254',[['192.0.2.1','router','Router','Value router.ip'],['192.0.2.2','core-a','Switch',''],['192.0.2.3','core-b','Switch',''],['192.0.2.20','printer office','Printer','']],'Office.');
D.ip2=netDoc('198.51.100.0/24','2 of 254',[['198.51.100.5','camera gate','Camera',''],['198.51.100.6','sensor tower','Sensor','']],'Devices.');
D.devs=fill({t:'devices',n:'Network devices',w:'wide',path:'not a file, lives in the database',by:'A. Keller',ch:'Oct 28',vis:'Group ops, inherited',tags:['hardware'],stat:'3 devices',html:tableHtml(['Name','Kind','Room','Ports','Address'],[['router','Router','Server room','8','192.0.2.1'],['core-a','Switch','Server room','48','192.0.2.2'],['core-b','Switch','Workshop','48','192.0.2.3']])});
D.net.kids=[...D.net.kids,'ip1','ip2','devs'];
TYPE.search=['search','Saved search'];
const savedHits=o=>Object.keys(D).filter(id=>{const d=D[id];if(id.includes(':')||d.t==='search'||d.gone)return false;if(o.type&&d.t!==o.type)return false;if(!o.ftags.every(t=>(d.tags||[]).includes(t)))return false;return !o.q||docText(d).toLowerCase().includes(o.q.toLowerCase());});
/* A saved search is a document: a query with filters and sources that runs again each time it opens. */
D.ss1=fill({t:'search',n:'Everything on the network',w:'wide',path:'not a file, a saved search',by:'A. Keller',ch:'Oct 30',vis:'Group ops, inherited',desc:'All documents tagged network, wherever they are.',q:'',ftags:['network'],type:'',strat:'Full text',src:['Workspace','Share','Ticket system'],
  get stat(){return savedHits(this).length+' hits';},
  /* What it searches for is C22's filter bar (a part per section, its values pressed), the hits C22's entries with
     their place. The values cannot be changed before M-8, so they are disabled: c22.js would toggle a filter bar's
     buttons by itself otherwise. */
  view:true,get html(){const h=savedHits(this),part=(name,vals)=>`<section aria-label="${name}"><h3>${name}</h3><div>${vals.map(v=>`<button type="button" aria-pressed="true" disabled><span>${v}</span></button>`).join('')}</div></section>`;
    return `<div class="filter-bar" role="group" aria-label="What it searches for">${part('Query',[this.q?esc(this.q):'any text'])}${part('Strategy',[this.strat])}${part('Filter',[...this.ftags.map(t=>'#'+t),...(this.type?[TYPE[this.type][1]]:[])])}${part('Sources',this.src)}</div>
    <p class="text-muted-foreground mt-3 mb-1 text-xs">Runs again each time it opens. Changing the query comes with editing (M-8), and the strategies with M-12.</p>
    <div class="item-group" role="list">${h.map(x=>`<a href="#" class="item" data-size="sm" role="listitem" data-act="open" data-id="${x}"><figure>${icon(TYPE[D[x].t][0])}</figure><section><h4>${D[x].n}</h4><p>${placeOf(x)}</p></section></a>`).join('')}</div>`;}});
C.find(c=>c.id==='ops').top.push('ss1');
TYPE.bookmark=['globe','Bookmark'];TYPE.news=['globe','News entry'];
BM.forEach(f=>f.items.forEach((b,k)=>{D[`bm:${f.id}/${k}`]=fill({t:'bookmark',n:b.n,path:'not a file, a bookmark',place:'Bookmarks › '+f.n,by:'you',ch:'Oct 20',stat:'1 link',tags:f.id==='work'?['hardware']:[],
  html:`<p><a href="${b.u}" target="_blank" rel="noopener noreferrer">${b.u}</a></p><p>${b.d}</p><div class="alert"><section>Nothing is loaded from the site, so no preview and no icon reach a third party. The link opens in a new browser tab.</section></div>`});}));
D['bm:import']=fill({t:'bookmark',n:'Import bookmarks',w:'wide',path:'not a file',place:'Bookmarks › Import',by:'you',ch:'now',stat:'nothing imported',
  html:`<div class="empty border" data-dropzone><header><figure>${icon('down')}</figure><h2>Drop the HTML file of your browser here</h2></header><section><button type="button" class="btn" data-variant="outline" disabled>Choose a file</button></section></div><p class="text-muted-foreground text-sm">Folders stay folders and links already there are skipped. Firefox, Chrome, Edge and Safari all export this format.</p>`});
NEWS.forEach(n=>{D['news:'+n.id]=fill({t:'news',n:n.h,path:'not a file, from a feed',place:'News › '+FEEDS.find(x=>x.id===n.f).n,by:FEEDS.find(x=>x.id===n.f).n,ch:n.date,stat:'1 entry',html:`<p>${n.b}</p><div class="alert"><section>Save it as a document, or send it to a collection, comes with the news module.</section></div>`});});
D['news:sub']=fill({t:'news',n:'Subscribe to a feed',path:'not a file',place:'News › Subscribe',by:'you',ch:'now',stat:'later',html:`<p><input class="input" disabled placeholder="Address of an RSS or Atom feed" aria-label="Feed address"></p><p class="text-muted-foreground text-sm">Later: feeds, and maybe pages that change. Every user has their own.</p>`});
D.plan.comments=[{b:'h3',l:'Radio link',who:'J. Roth',when:'Oct 29',t:'Is the radio link backed up by a second path?'},{b:'h3',l:'Radio link',who:'M. Sander',when:'Oct 30',t:'Not yet, it is on the list for spring.'},{b:'t1',l:'Table: Name, Room, Uplink',who:'A. Keller',when:'Nov 02',t:'The uplink of core-b is 1 Gbit for now.'}];
/* Bookmarks are objects: a folder of links in Operations. The import of a browser file comes from New at a collection. */
D.links=fill({t:'folder',n:'Links',kids:['bk1','bk2','bk3'],path:'not a file, a folder in the workspace',by:'A. Keller',ch:'Oct 20',vis:'Everyone signed in'});
[['bk1','Ticket system','https://tickets.example.com','Open tickets of the team.'],['bk2','Supplier portal','https://supplier.example.net','Orders and licenses.'],['bk3','Switch manual','https://docs.example.com/switch','Firmware 9, chapter on VLANs.']].forEach(([id,n,u,dsc])=>{D[id]=fill({t:'bookmark',n,path:'not a file, a bookmark',by:'A. Keller',ch:'Oct 20',vis:'Everyone signed in',desc:dsc,stat:'1 link',
  html:`<p><a href="${u}" target="_blank" rel="noopener noreferrer">${u}</a></p><div class="alert"><section>Nothing is loaded from the site, so no preview and no icon reach a third party. The link opens in a new browser tab.</section></div>`});});
D.ops.kids.splice(D.ops.kids.indexOf('ss1'),0,'links');D['bm:import'].place='Workspace';
D.rack=fill({t:'page',n:'Rack overview',w:'wide',path:'operations/network/rack.md',by:'A. Keller',ch:'Nov 03',vis:'Everyone signed in',tags:['hardware'],stat:'1 embedded table',desc:'What sits in the rack, with the switch table of the network plan.',
  get html(){const sec=D.plan.doc.secs.find(x=>x[0]==='Core switches'),tb=(sec[1].match(/<div class="table-container">[\s\S]*?<\/table><\/div>/)||[''])[0];
    return `<p>The table below is not a copy. It lives in the network plan and shows here, so a change there shows here too.</p><figure class="editor-embed">${tb}<figcaption>Core switches, from <a href="#" class="font-medium underline underline-offset-4" data-act="open" data-id="plan">Network plan · #t1</a></figcaption></figure><p class="text-muted-foreground text-sm">Paste as a copy instead comes later. A copy gets IDs of its own and no longer follows the source.</p>`;}});
D.net.kids.push('rack');D.plan.usedIn=[...(D.plan.usedIn||[]),'Rack overview (table #t1)'];
/* ---- UnderDoc: the real data in the workspace of the mockup (sprint 2026-11-08) ----
   Who is signed in, the collections and their trees come from /api/state; a page loads from /api/doc/<id> when it opens.
   What has no backend yet keeps the made-up content of the mockup, marked as a demo. */

/* Strict CSP: markup carries data-style, and this puts it into the CSSOM, which the CSP allows. */
const applyStyles=root=>{if(!root||!root.querySelectorAll)return;if(root.dataset&&root.dataset.style!==undefined)root.style.cssText=root.dataset.style;root.querySelectorAll('[data-style]').forEach(n=>{n.style.cssText=n.dataset.style;});};
new MutationObserver(records=>records.forEach(r=>{r.addedNodes.forEach(applyStyles);if(r.type==='attributes')applyStyles(r.target);})).observe(document.documentElement,{childList:true,subtree:true,attributes:true,attributeFilter:['data-style']});

/* Demo content: the made-up collections stay below the real ones, marked. */
C.forEach(c=>{D[c.id].n=D[c.id].n+' (demo)';S.fav[c.id]=false;});
S.open={};S.pin={};

async function loadDoc(id){
  const d=D[id];if(!d||d.loading||(d.t!=='page'&&!(d.t==='notes'&&d.real)))return;d.loading=true;d.html='<p class="text-muted-foreground text-sm">Loading…</p>';
  try{
    const r=await fetch('/api/doc/'+pathOf(d.sid||id),{credentials:'same-origin',headers:{Accept:'application/json'}});
    if(!r.ok)throw new Error(String(r.status));
    const a=await r.json();
    delete d.html;
    /* Names and texts from the server are escaped once, here; the rendered HTML of a page comes clean from the server. */
    d.doc={intro:a.intro,secs:a.secs.map(x=>[esc(x[0]),x[1]])};d.desc=esc(a.desc);d.tags=a.tags.map(esc);d.path=esc(a.path);d.ch=when(a.ch);d.by=d.by||'—';d.vis=a.seenBy?a.seenBy.map(esc).join(', '):'Everyone signed in';
    d.usedIn=a.usedIn.map(esc);d.uses={ext:[],int:a.links.map(esc),data:[],vals:[],db:[],emb:[]};d.loaded=true;
    /* What a page of the database embeds (M-3): a value by its name, everything else by its kind and where it lives. */
    (a.embeds||[]).forEach(e=>{const at=esc(e.title)+(e.ref.includes('#')?' #'+esc(e.ref.split('#')[1]):'');
      if(e.kind==='value')d.uses.vals.push(`${esc(e.name)} <small>${at}</small>`);else d.uses.emb.push({l:EMBED_KINDS[e.kind]||'Embedded',x:at});});
    if(a.blocks){d.blocks=a.blocks.map(blockOf);d.canEdit=!!a.canEdit;d.by=esc(a.owner||'—');d.created=when(a.created||'',false);d.place=esc(a.place||'');d.canComment=!!a.canComment;d.ncmt=a.comments||0;delete d.cmts;syncBlocks(d);if(S.files)loadAttachments(id);}
  }catch(e){d.html='<p class="text-muted-foreground text-sm">This page could not be loaded.</p>';}
  d.loading=false;render();
}

/* One node of /api/state becomes an object of the mockup's data. */
/* The IDs of the server name files and folders, so they may hold any character. The workspace writes its IDs into its
   markup, so an ID that is not safe there, or that the made-up content uses already, gets a key of its own; SID leads
   from the server's ID to that key. */
const SID={};let keyN=0;
const keyOf=sid=>SID[sid]||(SID[sid]=/[<>"'&`\\]/.test(sid)||sid.length>300||D[sid]?'k'+(++keyN):sid);
const pathOf=sid=>sid.split('/').map(encodeURIComponent).join('/');
function addressOf(key){const sid=D[key].sid||key;return sid.startsWith('f:')?'/f/'+pathOf(sid.slice(2)):'/o/'+pathOf(sid);}
function addNode(node){
  const id=keyOf(node.id),kids=(node.kids||[]).map(addNode),n=esc(node.n);
  if(node.t==='line'||node.t==='space'){decoDoc(id,node.t);D[id].real=true;D[id].sid=node.id;D[id].db=true;return id;}
  if(node.t==='collection'||node.t==='coll'){collDoc(id,n,kids,node.private?'lock':node.owner?'grid':'folder');D[id].real=true;D[id].owner=esc(node.owner||'—');D[id].private=node.private;}
  else D[id]=fill({t:node.t==='folder'?'folder':node.t==='notes'?'notes':'page',n,w:node.t==='notes'?'full':'wide',kids,path:esc(node.path||''),by:'—',ch:'',vis:'Everyone signed in',real:true,why:node.t==='folder'&&!node.owner?'A folder of the folder source.':''});
  D[id].sid=node.id;D[id].tags=node.tags?node.tags.map(esc):D[id].tags||[];D[id].db=!!node.owner;D[id].mine=!!node.mine;if(node.role)D[id].role=node.role;
  if(node.owner)D[id].owner=esc(node.owner);if(node.access)D[id].acc=node.access;
  if(node.width)D[id].w=node.width;if(node.contents)D[id].toc=node.contents;D[id].tpl=!!node.template;
  return id;
}

function idFromPath(){
  const dec=p=>p.split('/').map(decodeURIComponent).join('/');
  const m=location.pathname.match(/^\/o\/(.+)$/);if(m)return SID[dec(m[1])];
  const f=location.pathname.match(/^\/f\/(.*)$/);if(f)return SID['f:'+dec(f[1]).replace(/\/$/,'')];
  return null;
}

/* Collections of the database: made and renamed through the API (ADR-19). */
const same={credentials:'same-origin',headers:{'Content-Type':'application/json',Accept:'application/json'}};
/* A folder of the database: made in a collection or folder, named in place. */
async function createFolder(parent,title){
  try{const r=await fetch('/api/folders',{...same,method:'POST',body:JSON.stringify({title,parent:D[parent].sid})});if(!r.ok)throw new Error(String(r.status));
    const node=await r.json();node.owner=S.user.name;node.mine=true;const id=addNode(node);D[parent].kids.push(id);S.tog[parent]=true;rebuildPlaces();S.just=id;renameHere(id);}
  catch(e){toast(e.message==='403'?'Others may not add here.':'The folder could not be made.');}
}
/* A collection turns into a folder and back: the same object, shown in nav2 or only in nav3 (ADR-16). */
/* What others may do (ADR-20): only the owner sets it, and the database keeps it; made-up content keeps it here. If the
   server refuses, the level goes back. */
const isOwner=id=>!D[id].db||!!D[id].mine||RANK[D[id].role]>=4;
async function setAccess(id,level){
  const before=D[id].acc;if(level)D[id].acc=level;else delete D[id].acc;render();
  if(!D[id].db)return;
  try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/access',{...same,method:'POST',body:JSON.stringify({access:level})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){if(before)D[id].acc=before;else delete D[id].acc;render();toast(e.message==='403'?'Only the owner sets what others may do.':'What others may do could not be saved.');}
}
async function convertKind(id,kind){
  if(D[id].db){try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/kind',{...same,method:'POST',body:JSON.stringify({kind})});if(!r.ok)throw new Error(String(r.status));}
    catch(e){toast(e.message==='403'?'Others may not change this.':'It could not be turned into a '+kind+'.');return;}}
  D[id].t=kind==='folder'?'folder':'coll';D[id].i=kind==='folder'?'folder':'grid';rebuildPlaces();render();
}
async function createCollection(parent,title){
  try{const r=await fetch('/api/collections',{...same,method:'POST',body:JSON.stringify({title,parent})});if(!r.ok)throw new Error(String(r.status));
    placeCollection(await r.json(),parent,true);
  }catch(e){S.toast='The collection could not be made.';render();setTimeout(()=>{S.toast='';render();},2400);}
}
/* placeCollection puts a collection the server made where it belongs; rename writes its name in place, which the dialog
   of a new collection at the top has asked already. */
function placeCollection(node,parent,rename){
  node.t='collection';node.owner=S.user.name;node.mine=true;node.role='owner';const id=addNode(node);
  if(parent){D[parent].kids.push(id);S.tog[parent]=true;rebuildPlaces();onlyThis(COLL[id]?COLL[id].id:parent);}
  else{C.splice(C.filter(c=>D[c.id]&&D[c.id].real).length,0,asTop({id,i:'grid'}));rebuildPlaces();onlyThis(id);}
  S.view='workspace';S.rail=true;S.just=id;if(rename)renameHere(id);else render();
}
/* The name of something new is written where it appears, in nav2 or in nav3: Enter or leaving the field keeps it,
   Escape keeps the name it got. */
function renameHere(id){S.ren=id;render();const f=document.querySelector('[data-ren]');if(f){f.focus();f.select();}}
function renameDone(x){const id=x.dataset.ren,v=x.value.trim();if(S.ren!==id)return;S.ren=null;
  if(v&&v!==plain(D[id].n)){D[id].n=esc(v);if(D[id].real)renameObject(id,v);}render();}
async function renameObject(id,title){
  try{const r=await fetch('/api/objects/'+encodeURIComponent(id),{...same,method:'PATCH',body:JSON.stringify({title})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){S.toast='The new name could not be saved.';render();setTimeout(()=>{S.toast='';render();},2400);}
}

/* Pages of the database are blocks of Markdown (ADR-19). The view shows each block rendered by the server. Editing writes
   the whole page in one editor (ADR-21, web/editor, editor.js): CodeMirror 6 with a live preview, the blocks joined with
   a hidden marker line before each, where the handle of the block shows. The editor saves about a second after typing
   stops and when it is left, block by block through the same API. */
const ALIGNS=['center','right'];
const blockOf=b=>({id:esc(b.id),html:b.html,md:esc(b.md),raw:b.md,usedIn:(b.usedIn||[]).map(esc),cmts:b.comments||0,align:ALIGNS.includes(b.align)?b.align:'',
  parts:(b.parts||[]).map(p=>({id:p.id,name:p.name||'',usedIn:p.usedIn||[]}))});
function syncBlocks(d){d.doc={intro:d.blocks.map(b=>b.html).join(''),secs:[]};d.ch=d.ch||'';}
/* The bar sits once above the page (2026-11-09, ADR-21): a bar that follows the cursor jumps with every line and covers
   the text above it, and M-8 asks for a bar above the page. It acts on the selection of the editor; "/" at the start of an
   empty line opens the same kinds as a menu. */
const KINDS=[['p','Text','text'],['h1','Heading 1','heading'],['h2','Heading 2','heading'],['h3','Heading 3','heading'],['ul','Bulleted list','list'],
  ['ol','Numbered list','olist'],['todo','To-do list','check'],['quote','Quote','quote'],['code','Code','code']];
const SLASH=[...KINDS.map(k=>({k:k[0],n:k[1],i:k[2]})),{k:'hr',n:'Divider',i:'hr'},{k:'table',n:'Table',i:'table'},
  {k:'embed',n:'Embed a block or page',i:'link',hint:'Paste the address from Copy address'},{k:'secret',n:'Password',i:'lock',hint:'Kept sealed on the server; the text holds only a reference (ADR-7)'},{k:'image',n:'Image',i:'images',later:'Comes with the store for files'},{k:'file',n:'File',i:'page',later:'Comes with the store for files'}];
/* The table tools of the bar (M-8): shown while the cursor stands in a table; each writes the table again as Markdown. */
Object.assign(I,{rowup:'<rect x="3" y="11" width="18" height="9" rx="1"/><path d="M12 3v6M9 6h6"/>',rowdown:'<rect x="3" y="4" width="18" height="9" rx="1"/><path d="M12 15v6M9 18h6"/>',
  colleft:'<rect x="11" y="3" width="9" height="18" rx="1"/><path d="M3 12h6M6 9v6"/>',colright:'<rect x="4" y="3" width="9" height="18" rx="1"/><path d="M15 12h6M18 9v6"/>',
  rowdel:'<rect x="3" y="7" width="18" height="10" rx="1"/><path d="m9 10 6 4M15 10l-6 4"/>',coldel:'<rect x="7" y="3" width="10" height="18" rx="1"/><path d="m10 9 4 6M14 9l-4 6"/>',
  alleft:'<path d="M4 6h16M4 10h10M4 14h16M4 18h10"/>',alcenter:'<path d="M4 6h16M7 10h10M4 14h16M7 18h10"/>',alright:'<path d="M4 6h16M10 10h10M4 14h16M10 18h10"/>'});
const TABLE=[['rowAbove','Row above','rowup'],['rowBelow','Row below','rowdown'],['colLeft','Column left','colleft'],['colRight','Column right','colright'],
  ['rowDelete','Delete row','rowdel'],['colDelete','Delete column','coldel'],['alignLeft','Align column left','alleft'],['alignCenter','Center column','alcenter'],['alignRight','Align column right','alright']];
const FMT=[['bold','Bold (Ctrl+B)','bold'],['italic','Italic (Ctrl+I)','italic'],['strike','Strikethrough','strike'],['code','Inline code','code'],['link','Link (Ctrl+K)','link']];
/* Every block has a handle at its left, shown on hover, as Docmost has one (the idea only): its menu copies the address
   to embed the block, comments on it and says where it is used. In reading it opens the block for editing; in the editor
   it moves the block and deletes it (2026-11-09). */
function handleHtml(d,w,i,b,inEditor,end){
  const open=S.bmenu&&S.bmenu.w===i&&S.bmenu.id===b.id,may=d.canEdit,cmt=!!d.canComment,used=b.usedIn||[],k=d.blocks.findIndex(x=>x.id===b.id);
  const item=(attrs,ico,label,more='')=>`<button type="button" role="menuitem" ${attrs}>${icon(ico)}<span>${label}</span>${more}</button>`;
  const edit=inEditor?item(`data-act="edmove" data-w="${i}" data-id="${b.id}" data-k="up"${k>0?'':' disabled'}`,'up','Move up')
    +item(`data-act="edmove" data-w="${i}" data-id="${b.id}" data-k="down"${k>=0&&k<d.blocks.length-1?'':' disabled'}`,'down','Move down')
    +item(`data-act="eddel" data-w="${i}" data-id="${b.id}"`,'trash','Delete')
    :item(`data-act="blkedit" data-w="${i}" data-id="${b.id}"${may?'':` disabled title="${noEdit(w.d)}"`}`,'pencil','Edit this block');
  /* The menu is drawn open by this script (c22.js is not loaded): the popover of C22's .dropdown-menu, aria-hidden false. */
  const menu=open?`<div data-popover aria-hidden="false"${end?' data-align="end"':''}><div role="menu" aria-label="Block ${b.id}">
    ${item(`data-act="blkaddr" data-w="${i}" data-id="${b.id}"`,'link','Copy address',`<span class="ms-auto font-mono text-xs text-muted-foreground">#${b.id}</span>`)}
    ${edit}
    ${may?moveToHtml(d,i,b):''}
    ${item(`data-act="bcmt" data-w="${i}" data-id="${b.id}"${cmt?'':' disabled title="Only read here"'}`,'chat','Comment',b.cmts?`<span class="ms-auto text-xs text-muted-foreground">${b.cmts}</span>`:'')}
    ${may?`<div class="button-group w-full" role="group" aria-label="Align the block">${[['left','alleft','Align left'],['center','alcenter','Center'],['right','alright','Align right']].map(x=>`<button type="button" class="btn flex-1" data-variant="outline" data-size="sm" data-act="balign" data-w="${i}" data-id="${b.id}" data-k="${x[0]}" aria-pressed="${(b.align||'left')===x[0]}" aria-label="${x[2]}" title="${x[2]}">${icon(x[1])}</button>`).join('')}</div>`:''}
    <hr role="separator"><div role="heading">${used.length?'Used in '+used.join(', '):'Not used elsewhere'}</div></div></div>`:'';
  /* C22's handle: a .dropdown-menu at the start of the block (in a card at its end), its trigger the grip. */
  /* No white space between its tags: in the editor it stands inside CodeMirror's text, which keeps white space. */
  return `<div class="dropdown-menu editor-handle"${end?' data-align="end"':''}><button type="button" class="btn" data-variant="ghost" data-size="icon-xs" data-act="bmenu" data-w="${i}" data-id="${b.id}"${may?' data-drag':''} aria-haspopup="menu" aria-expanded="${open}" aria-label="Block ${b.id}" title="Block #${b.id}: address, ${inEditor?'moving':'editing'}, comments">${icon('grip')}</button>${menu}</div>`.replace(/>\s+</g,'><');
}
/* Moving a block into another page of the database (M-3, ADR-22): its parts and section go along, every reference
   follows and the old address redirects. The pages offered are those of the database the workspace knows; the server
   refuses one the viewer may not change. It goes to the end of the other page. */
function moveToHtml(d,i,b){
  if(!(S.bmove&&S.bmove.w===i&&S.bmove.id===b.id))return `<button type="button" role="menuitem" data-act="bmoveto" data-w="${i}" data-id="${b.id}">${icon('send')}<span>Move to page</span></button>`;
  const here=Object.keys(D).find(k=>D[k]===d),pages=Object.keys(D).filter(k=>k!==here&&D[k].t==='page'&&D[k].db&&D[k].real&&D[k].sid)
    .sort((x,y)=>String(D[x].n).localeCompare(String(D[y].n)));
  return `<div class="flex flex-wrap items-center gap-1.5 p-1"><span class="text-sm">Move #${b.id} to</span><select class="select" data-size="sm" id="bmovesel" data-keep="bmovesel" aria-label="Page to move the block to"><option value="">Choose a page</option>${pages.map(k=>`<option value="${k}">${D[k].n}</option>`).join('')}</select>
    <button type="button" class="btn" data-variant="outline" data-size="xs" data-act="bmovego" data-w="${i}" data-id="${b.id}">Move</button></div>`;
}
async function moveToPage(i,key,block,target){
  const d=D[key],to=D[target];if(!d||!to||!d.sid||!to.sid)return;
  const e=EDS[key];
  if(e){await e.ed.flush();if(e.ed.dirty){toast('The page is not saved yet. Try again in a moment.');return;}}
  const blocks=await fetch('/api/doc/'+pathOf(to.sid),{credentials:'same-origin',headers:{Accept:'application/json'}}).then(r=>r.ok?r.json():null).catch(()=>null);
  const last=blocks&&blocks.blocks&&blocks.blocks.length?blocks.blocks[blocks.blocks.length-1].id:'';
  const r=await fetch('/api/pages/'+pathOf(d.sid)+'/blocks/'+encodeURIComponent(block)+'/move',{method:'POST',credentials:'same-origin',
    headers:{'Content-Type':'application/json',Accept:'application/json'},body:JSON.stringify({page:to.sid,after:last})}).catch(()=>null);
  if(!r||!r.ok){toast(r&&r.status===403?'You may not change '+to.n+'.':'The block could not be moved.');return;}
  S.bmove=null;S.bmenu=null;
  /* The block leaves the page here too, at once: an editor made again before the page is loaded starts from d.blocks. */
  if(d.blocks)d.blocks=d.blocks.filter(x=>x.id!==block);
  if(EDS[key]===e&&e){delete EDS[key];e.ed.discard();}
  d.loaded=false;to.loaded=false;loadDoc(key);if(S.wins.some(w=>w.tabs.some(x=>x.d===target)))loadDoc(target);
  toast('Moved to '+to.n+'. Its old address leads there.');
}
function blocksHtml(d,w,i){
  const key=Object.keys(D).find(k=>D[k]===d);
  if(d.t==='notes'&&!(editing(w)&&d.canEdit))return notesHtml(d,w,i);
  /* Each block is C22's wrapper in the typeset flow: div[data-block] with its handle, then what the block renders. */
  if(!(editing(w)&&d.canEdit))return d.blocks.length?d.blocks.map(b=>`<div id="w${i}${b.id}" data-block="${b.id}"${b.align?` class="text-${b.align}"`:''}${b.parts&&b.parts.length?` data-parts="${esc(JSON.stringify(b.parts))}"`:''}>${handleHtml(d,w,i,b)}${b.html}</div>`).join(''):`<p class="text-muted-foreground">Empty.${d.canEdit?' Click Reading to write.':''}</p>`;
  return edHtml(d,i,key);
}

/* Quick notes of the database (M-8): a board of small cards, newest first, with a field on top to add one. Each card is a
   block of the board, so it has its handle, comments and versions; editing the board writes its cards as a page. */
function notesHtml(d,w,i){
  /* C22's quick notes: a field to add, a grid of small cards, each a block with its handle in its top corner. */
  const add=d.canEdit?`<input class="input mx-auto max-w-lg" type="text" data-addnote placeholder="Take a note, then Enter" aria-label="Take a note">`:'';
  const cards=d.blocks.map(b=>`<article class="card${b.align?' text-'+b.align:''}" data-size="sm" id="w${i}${b.id}" data-block="${b.id}">${handleHtml(d,w,i,b,false,true)}<section>${b.html}</section></article>`).join('');
  return `<div class="flex flex-col gap-4">${add}${cards?`<div class="grid grid-cols-2 gap-2 md:grid-cols-3" data-notes>${cards}</div>`:'<p class="text-muted-foreground">No notes yet.</p>'}</div>`;
}
async function addCard(key,md){
  const d=D[key];
  try{const r=await fetch(pageURL(key)+'/blocks',{...same,method:'POST',body:JSON.stringify({after:'',md})});if(!r.ok)throw new Error(String(r.status));
    d.blocks.unshift(blockOf(await r.json()));syncBlocks(d);d.ch='now';render();const n=document.querySelector('[data-addnote]');if(n)n.focus();}
  catch(e){toast(e.message==='403'?'Others may not change this.':'The note could not be kept.');}
}
/* New with Quick notes chosen: the card goes on top of the account's board, made in the private notes the first time. */
async function quickNote(md){
  try{const r=await fetch('/api/notes',{...same,method:'POST',body:JSON.stringify({md})});if(!r.ok)throw new Error(String(r.status));
    const a=await r.json();let key=SID[a.board.id];
    if(!key||!D[key]){a.board.t='notes';key=addNode(a.board);const parent=SID[a.in];if(parent&&D[parent]&&!D[parent].kids.includes(key))D[parent].kids.unshift(key);rebuildPlaces();}
    D[key].loaded=false;delete D[key].blocks;await loadDoc(key);openDoc(key);render();}
  catch(e){toast('The note could not be kept.');}
}
/* The editor of a page lives on through every render: render() draws an empty place for it, and mountEditors() puts the
   same editor back there, with its text, selection and undo. An editor whose place is gone is saved and closed. */
const EDS={};
function edHtml(d,i,key){
  const note=S.edNote&&S.edNote[key];
  /* The editor of C22 (.editor) is CodeMirror's root inside the place; a note above it is a question in place (.item). */
  return `<div class="flex flex-col gap-2" data-edpage="${key}">${note?`<article class="item" data-variant="muted" data-size="sm" role="status" data-ednote="${key}"><section><p class="text-sm">This page was changed elsewhere meanwhile, so it was loaded again; what you wrote since it was last saved is not in it.</p></section><aside><button type="button" class="btn" data-variant="outline" data-size="xs" data-act="edcopy" data-w="${i}">Copy what I wrote</button><button type="button" class="btn" data-variant="outline" data-size="xs" data-act="ednote" data-w="${i}">Close</button></aside></article>`:''}
    <div data-cmhost="${key}" data-w="${i}"></div></div>`;
}
/* The bar is a row of the window while its page is written, between the head and the content, outside what scrolls, so
   it stays in place (2026-11-10). It drives the editor of the page; its buttons keep the selection in the editor. It is
   C22's .editor-bar as a row of .workspace-main: the kind first, then its groups (T-12). */
function edBarHtml(key,i){
  const e=EDS[key],b=(act,k,title,ico)=>`<button type="button" class="btn" data-variant="ghost" data-size="icon-sm" data-act="${act}" data-w="${i}"${k?` data-k="${k}"`:''} title="${title}" aria-label="${title}">${icon(ico)}</button>`;
  return `<div class="editor-bar" role="toolbar" aria-label="Write" data-edbar="${key}">
      <select class="select" data-size="sm" data-edtype="${key}" aria-label="Turn into" title="Turn into">${KINDS.map(k=>`<option value="${k[0]}"${k[0]===((e&&e.kind)||'p')?' selected':''}>${k[1]}</option>`).join('')}</select>
      <div role="group" aria-label="Format">${FMT.map(f=>b('edfmt',f[0],f[1],f[2])).join('')}${b('edsecret','','Password: kept sealed, the text holds a reference','lock').replace(' aria-label="Password: kept sealed, the text holds a reference"',' aria-label="Password"')}</div>
      <div role="group" aria-label="Table" data-edtable="${key}"${e&&e.table?'':' hidden'}>${TABLE.map(f=>b('edtable',f[0],f[1],f[2])).join('')}</div>
      <div role="group" aria-label="Marked text" data-edsel="${key}"${e&&e.marked?'':' hidden'}>${b('edselcopy','','Copy as text, with no tie','copy').replace('aria-label="Copy as text, with no tie"','aria-label="Copy as text"')}${b('edselembed','','Embed elsewhere: copies the address of the marked text','link').replace('aria-label="Embed elsewhere: copies the address of the marked text"','aria-label="Embed elsewhere"')}<input class="input" data-size="sm" data-edvalname="${key}" placeholder="Name it as a value" aria-label="Name of the value" maxlength="80">${b('edselvalue','','Make the marked text a value with this name, and copy its address','list').replace('aria-label="Make the marked text a value with this name, and copy its address"','aria-label="Make a value"')}</div>
      <small data-edstatus="${key}">${esc((e&&e.status)||'')}</small></div>`;
}
const edAPI=key=>{
  const call=async(url,init)=>{const r=await fetch(pageURL(key)+url,{...same,...init});
    if(r.status===409){const e=new Error('409');try{e.inUse=(await r.json()).inUse;}catch(x){}throw e;}
    if(!r.ok){const e=new Error(String(r.status));try{e.detail=(await r.json()).error;}catch(x){}throw e;}return r.status===204?null:r.json();};
  return {
    load:async()=>{const r=await fetch('/api/doc/'+pathOf(D[key].sid),{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));return ((await r.json()).blocks||[]).map(b=>({id:b.id,md:b.md,usedIn:b.usedIn}));},
    edit:(id,md,ways)=>call('/blocks/'+encodeURIComponent(id),{method:'PUT',body:JSON.stringify(ways?{md,ways}:{md})}),
    remove:(id,ways)=>call('/blocks/'+encodeURIComponent(id),{method:'DELETE',...(ways?{body:JSON.stringify({ways})}:{})}),
    mark:(id,start,end,name)=>call('/blocks/'+encodeURIComponent(id)+'/parts',{method:'POST',body:JSON.stringify({start,end,name})}),
    excerpt:async(id,start,end)=>(await call('/blocks/'+encodeURIComponent(id)+'/excerpt',{method:'POST',body:JSON.stringify({start,end})})).md,
    lost:async(id,md)=>(await call('/blocks/'+encodeURIComponent(id)+'/lost',{method:'POST',body:JSON.stringify(md===null?{delete:true}:{md})})).inUse,
    move:(id,after)=>call('/blocks/'+encodeURIComponent(id)+'/move',{method:'POST',body:JSON.stringify({after})}),
    add:(after,md)=>call('/blocks',{method:'POST',body:JSON.stringify({after,md})}),
  };
};
/* What the editor saved, the page keeps for reading, as the server answered it. */
function edSaved(key){
  const d=D[key],done=()=>{syncBlocks(d);d.ch='now';if(gearOpen(key))loadVersions(key);};
  return {
    edited:b=>{const x=d.blocks.find(y=>y.id===b.id),n=blockOf(b);if(x){x.html=n.html;x.md=n.md;x.raw=n.raw;x.usedIn=n.usedIn;}done();},
    added:(b,after)=>{const k=after?d.blocks.findIndex(x=>x.id===after)+1:0;d.blocks.splice(k,0,blockOf(b));done();},
    removed:id=>{d.blocks=d.blocks.filter(x=>x.id!==id);done();},
    moved:(id,after)=>{const k=d.blocks.findIndex(x=>x.id===id);if(k<0)return;const [b]=d.blocks.splice(k,1);d.blocks.splice(after?d.blocks.findIndex(x=>x.id===after)+1:0,0,b);done();},
  };
}
function makeEditor(key){
  const d=D[key],e={key,w:S.focus,kind:'p',status:''};
  const show=(sel,v,attr)=>{const n=document.querySelector(`[${sel}="${key}"]`);if(n)n[attr]=v;};
  e.ed=window.UnderDocEditor.create({
    blocks:d.blocks.map(b=>({id:b.id,md:b.raw,usedIn:(b.usedIn||[]).map(plain),align:b.align,parts:b.parts||[]})),
    label:'Page '+plain(d.n)+', Markdown',
    api:edAPI(key),
    handle:id=>{const b=d.blocks.find(x=>x.id===id)||{id,usedIn:[]};return handleHtml(d,at(e.w),e.w,b,true);},
    slash:SLASH.map(x=>({...x,icon:icon(x.i)})),
    embed:ref=>{const [sid,blk]=ref.split('#'),k=Object.keys(D).find(x=>D[x].sid===sid);return {label:(k?plain(D[k].n):'Page '+sid)+(blk?' · #'+blk:''),href:'/o/'+sid+(blk?'#'+blk:'')};},
    /* Passwords in the text (ADR-7): the masks of the editor reveal, copy and change through secrets.js. */
    secret:window.UDSecrets?UDSecrets.forEditor():null,
    on:{
      kind:k=>{e.kind=k;show('data-edtype',k,'value');},
      table:on=>{e.table=on;show('data-edtable',!on,'hidden');},
      selection:on=>{e.marked=on;show('data-edsel',!on,'hidden');},
      status:s=>{e.status=s;show('data-edstatus',s,'textContent');},
      hint:t=>toast(t),
      secret:()=>window.UDSecrets?UDSecrets.make(d.sid):Promise.resolve(null),
      /* A value typed as text that looks like a password becomes one, when the writer says so (ADR-7). */
      secretFrom:v=>window.UDSecrets?UDSecrets.makeFrom(d.sid,v):Promise.resolve(null),
      /* A pasted or dropped image becomes an attachment of the page (ADR-28). */
      upload:f=>uploadAttachment(key,f),
      error:err=>toast(err&&err.message==='403'?'Others may not change this page.':'The page could not be saved. What you wrote stays in the editor.'),
      conflict:text=>{S.edNote=S.edNote||{};S.edNote[key]=text;if(EDS[key]===e){delete EDS[key];e.ed.discard();}d.loaded=false;loadDoc(key);},
      saved:edSaved(key),
      inUse:q=>askWays(q.inUse).then(m=>m&&Object.fromEntries(Object.entries(m).map(([ref,w])=>[ref.split('#')[1],w]))),
    },
  });
  return e;
}
/* The editor is loaded when it is first needed, not with every page: on the way into editing, when the pointer comes to
   the reading or editing button, or in an idle moment when editing is likely (pages open for editing, or an editable page
   is open). Its URL carries the version of the assets, so the browser keeps it until the next release (2026-11-09). */
let editorLoad=null;
function needEditor(){
  if(window.UnderDocEditor)return Promise.resolve();
  if(editorLoad)return editorLoad;
  if(editorLoad===false)return Promise.reject(new Error('editor'));
  const v=(document.querySelector('script[src*="/assets/app/demo.js"]')?.src.split('?v=')[1])||'';
  editorLoad=new Promise((ok,fail)=>{const s=document.createElement('script');s.src='/assets/app/editor/editor.js?v='+v;s.dataset.editor='';s.onload=ok;s.onerror=()=>{editorLoad=null;fail(new Error('editor'));};document.head.append(s);});
  return editorLoad;
}
function editorLikely(){
  if(window.UnderDocEditor||editorLoad)return;
  const t=at(S.focus),d=t&&D[t.d];
  if(S.openEdit||d&&d.blocks&&d.canEdit)(window.requestIdleCallback||(f=>setTimeout(f,1500)))(()=>{needEditor().catch(()=>{});});
}
document.addEventListener('pointerover',e=>{if(e.target.closest&&e.target.closest('[data-mode]'))needEditor().catch(()=>{});});
let edPending=null;
function mountEditors(focusKey,want){
  const seen=new Set();
  editorLikely();
  /* Where the cursor was to go while the editor was still loading, it goes once it is there. */
  if(edPending&&window.UnderDocEditor){focusKey=focusKey||edPending.f;want=want||edPending.w;edPending=null;}
  document.querySelectorAll('[data-cmhost]').forEach(h=>{const key=h.dataset.cmhost;
    if(!window.UnderDocEditor){h.innerHTML=`<p class="text-muted-foreground text-sm">${editorLoad===false?'The editor did not load. Reload the page to try again.':'Loading the editor…'}</p>`;
      if(focusKey||want)edPending={f:focusKey,w:want};
      if(editorLoad!==false)needEditor().then(()=>render(),()=>{editorLoad=false;render();});return;}
    if(seen.has(key)){h.innerHTML='<p class="text-muted-foreground text-sm">This page is being written in another window.</p>';return;}
    seen.add(key);const e=EDS[key]||(EDS[key]=makeEditor(key));e.w=+h.dataset.w;h.append(e.ed.dom);e.ed.refresh();
    const sel=document.querySelector(`[data-edtype="${key}"]`);if(sel)sel.value=e.kind;});
  /* An editor whose page is no longer written is saved and closed; reading shows what it saved. */
  Object.keys(EDS).filter(k=>!seen.has(k)).forEach(k=>{const e=EDS[k];delete EDS[k];Promise.resolve(e.ed.destroy()).finally(()=>render());});
  if(want&&EDS[want.d]){if(want.id)EDS[want.d].ed.focusBlock(want.id);else EDS[want.d].ed.focus();}
  else if(focusKey&&EDS[focusKey])EDS[focusKey].ed.focus();
}
/* A block is dragged by its handle (blockdrag.js), by who may change the page (only their handles carry data-drag). In reading the block itself is hidden and
   a gap with a copy of it (C22's .dropgap) moves between the blocks; a drop moves it through the API, and the view shows
   the new order at once and goes back if the server refuses. In the editor the editor hides it and draws the gap, and a
   drop moves its text with its marker, which saving then moves on the server (2026-11-10). */
const BDRAG={
  pick(h){const i=+h.dataset.w,t=at(i),d=t&&D[t.d];if(!d||!d.blocks)return null;
    if(h.closest('[data-cmhost]')){const e=EDS[t.d];return e?{id:h.dataset.id,key:t.d,ed:e.ed}:null;}
    const el=h.closest('[data-block]');return el?{id:h.dataset.id,key:t.d,el}:null;},
  rows(c){if(c.ed)return c.ed.blockRects();
    return [...c.el.parentElement.querySelectorAll(':scope>[data-block]')].map(n=>{const r=n.getBoundingClientRect();return {id:n.dataset.block,top:r.top,bot:r.bottom};});},
  scroller:c=>(c.ed?c.ed.dom:c.el).closest('[data-sc]'),
  show(c,k){
    if(c.ed){c.ed.dragShow(c.id,k);return;}
    if(!c.el.isConnected)return;
    /* C22 shows only the first lines of the held block in the gap (.dropgap.held in .typeset). */
    if(!c.gap){c.gap=document.createElement('div');c.gap.className='dropgap held';const copy=c.el.cloneNode(true);
      ['id','data-block'].forEach(a=>copy.removeAttribute(a));copy.querySelectorAll('.editor-handle,[data-comment]').forEach(x=>x.remove());copy.querySelectorAll('[data-act]').forEach(x=>x.removeAttribute('data-act'));
      c.gap.append(copy);c.el.style.display='none';}
    const others=[...c.el.parentElement.querySelectorAll(':scope>[data-block]')].filter(n=>n!==c.el);
    if(k<others.length)others[k].before(c.gap);else others[others.length-1].after(c.gap);},
  end(c,after){
    if(c.ed){c.ed.dragEnd(after);return;}
    if(c.gap)c.gap.remove();c.el.style.display='';
    if(after!==null)moveBlockTo(c.key,c.id,after);}
};
/* A block is aligned left, center or right from the menu of its handle (ADR-16, M-8): the alignment belongs to the block,
   not to its Markdown, so the text stays as written. The view changes at once and goes back if the server refuses. */
async function alignBlock(key,id,align){
  const d=D[key],b=d.blocks.find(x=>x.id===id);if(!b)return;const before=b.align,to=align==='left'?'':align;
  const show=a=>{b.align=a;if(EDS[key])EDS[key].ed.setAlign(id,a);render();};
  show(to);
  try{const r=await fetch(pageURL(key)+'/blocks/'+encodeURIComponent(id)+'/align',{...same,method:'PUT',body:JSON.stringify({align:to})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){show(before);toast(e.message==='403'?'Others may not change this page.':'The block could not be aligned.');}
}
async function moveBlockTo(key,id,after){
  const d=D[key],before=d.blocks.slice(),k=d.blocks.findIndex(x=>x.id===id);if(k<0)return;
  const [b]=d.blocks.splice(k,1);d.blocks.splice(after?d.blocks.findIndex(x=>x.id===after)+1:0,0,b);syncBlocks(d);render();
  try{const r=await fetch(pageURL(key)+'/blocks/'+encodeURIComponent(id)+'/move',{...same,method:'POST',body:JSON.stringify({after})});if(!r.ok)throw new Error(String(r.status));}
  catch(e){d.blocks=before;syncBlocks(d);render();toast(e.message==='403'?'Others may not change this page.':'The block could not be moved.');}
}
/* Leaving the page saves what is not saved yet. */
const flushEditors=()=>Object.values(EDS).forEach(e=>e.ed.flush());
document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='hidden')flushEditors();});
addEventListener('pagehide',flushEditors);
/* The versions hold what was taken out, so they are for whoever may edit the page (ADR-29). */
function versionsHtml(d){
  if(!d.canEdit)return '<p class="text-muted-foreground text-xs">Versions are for whoever may edit this page.</p>';
  if(!d.versions)return '<p class="text-muted-foreground text-xs">Loading…</p>';
  if(!d.versions.length)return '<p class="text-muted-foreground text-xs">No changes yet.</p>';
  return factList(d.versions.map(v=>[v.at,`${v.who} · #${v.block} ${v.deleted?'deleted':v.excerpt||'(empty)'}`]));
}
/* Marked text in the editor (ADR-22): Copy puts it on the clipboard as plain text with no tie; Embed makes it a part, or
   finds the part it already is, and puts its address on the clipboard, to paste where it should show. */
async function markOrCopy(e,embed,name=''){
  const put=(text,said)=>(navigator.clipboard?navigator.clipboard.writeText(text):Promise.reject()).then(()=>toast(said),()=>toast(text));
  if(!embed){put(await e.ed.copySelection(),'Copied as text, with no tie to where it came from.');return;}
  const r=await e.ed.markSelection(name);
  if(r.error){toast(r.error);return;}
  window.lastMarked=r;put(location.origin+r.url,'Address copied. Paste it into a paragraph or alone on a line to embed it there.');
}
/* The placeholder of something deleted everywhere (ADR-22): restoring it brings every use back; keeping a copy writes it
   into the page that embeds it. Every open page of the database draws again. */
async function removedAct(id,what,page){
  try{const r=await fetch('/api/removed/'+encodeURIComponent(id)+'/'+what,{...same,method:'POST',...(page?{body:JSON.stringify({page})}:{})});if(!r.ok)throw new Error(String(r.status));
    toast(what==='restore'?'It is back, everywhere.':'The copy is in this page now.');}
  catch(e){toast(e.message==='403'?'Others may not change this page.':'That did not work.');return;}
  for(const k of Object.keys(D))if(D[k].db&&D[k].loaded&&!EDS[k]){D[k].loaded=false;loadDoc(k);}
}
function toast(text){S.toast=text;render();setTimeout(()=>{S.toast='';render();},2600);}
window.UDToast=toast;
const pageURL=id=>'/api/pages/'+encodeURIComponent(D[id].sid);
async function loadVersions(id){
  if(!D[id].canEdit)return;
  try{const r=await fetch(pageURL(id)+'/versions',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    D[id].versions=(await r.json()).map(v=>({at:when(v.at),who:esc(v.who),block:esc(v.block),deleted:!!v.deleted,excerpt:esc(v.excerpt)}));}
  catch(e){D[id].versions=[];}
  render();
}
const gearOpen=id=>S.wins.some(w=>w.tabs.some(t=>t.d===id&&t.gear));
/* A new page lands in the collection, folder or page it was made in, and opens in the editor, ready to write. */
async function createPage(parent,title,template){
  try{const r=await fetch('/api/pages',{...same,method:'POST',body:JSON.stringify({title,parent:D[parent].sid,template:template||''})});if(!r.ok)throw new Error(String(r.status));
    const node=await r.json();node.t='page';node.owner=S.user.name;node.mine=true;const id=addNode(node);D[id].canEdit=true;D[id].canComment=true;
    D[id].loaded=true;D[id].blocks=[];D[id].by=esc(S.user.name);D[id].created='today';D[id].path='not a file, lives in the database';syncBlocks(D[id]);delete D[id].html;
    /* A page from a template has blocks already: they are read from the server before it is written. */
    if(template){D[id].loaded=false;delete D[id].blocks;await loadDoc(id);}
    D[parent].kids.push(id);S.tog[parent]=true;rebuildPlaces();openDoc(id);S.just=id;
    const w=S.wins[S.focus],t=w&&w.tabs[w.a];if(t&&t.d===id)setEdit(t,true);
    S.focusNext={d:id};render();}
  catch(e){toast(e.message==='403'?'Others may not add pages here.':'The page could not be made.');}
}

/* Comments on pages of the database (ADR-18, ADR-20): on the whole page or on one block through its ID, with replies one
   level deep. Whoever sees the page reads them; whoever may comment adds some; each author changes and deletes their own;
   a thread is resolved by its author or by whoever may edit. A block that is gone keeps its comments. The text is plain:
   it is escaped here, never rendered. Made-up documents keep their made-up comments (cmtHtml). */
/* Pages of the folder source keep no comments: they are files, read only. Their speech bubble is greyed and says so,
   instead of taking comments that are never kept (2026-11-09). */
const fromSource=id=>{const d=D[id];return !!(d&&d.real&&!d.db&&!d.blocks);};
const cmtShown=id=>{const d=D[id];return d.blocks?!!d.canComment||cmtCount(d)>0:accOf(id)!=='read'&&!d.tool&&!fromSource(id);};
const cmtCount=d=>d.blocks?d.ncmt||0:(d.comments||[]).length;
const cmtURL=id=>'/api/comments/'+encodeURIComponent(id);
function blockLabel(b){const s=(b.draft!==undefined?b.draft:b.raw||'').replace(/::embed\{ref="([^"]+)"\}/g,'Embedded $1').replace(/[#>*_`|~\[\]()-]+/g,' ').replace(/\s+/g,' ').trim();return s.slice(0,60)||'Empty block';}
function sortCmts(d){d.cmts.sort((x,y)=>(x.parent||x.id)-(y.parent||y.id)||(x.parent?1:0)-(y.parent?1:0)||x.id-y.id);}
/* Open comments count by block, as /api/doc counts them: a resolved thread counts no more, its replies neither. */
function recount(d){
  const res={},n={};d.cmts.forEach(c=>{if(!c.parent)res[c.id]=!!c.resolved;});
  d.cmts.forEach(c=>{if(!res[c.parent||c.id])n[c.block]=(n[c.block]||0)+1;});
  d.ncmt=Object.values(n).reduce((s,x)=>s+x,0);(d.blocks||[]).forEach(b=>{b.cmts=n[b.id]||0;});
}
async function loadComments(key){
  const d=D[key];if(!d||!d.sid||d.cmtLoading)return;d.cmtLoading=true;
  try{const r=await fetch(pageURL(key)+'/comments',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    const a=await r.json();d.cmts=a.comments;d.canComment=!!a.canComment;sortCmts(d);recount(d);}
  catch(e){d.cmts=d.cmts||[];toast('The comments could not be loaded.');}
  d.cmtLoading=false;render();
  /* The drawer opened while its comments were loading: its field takes the typing once it is there. */
  if(S.cmtFocus!==undefined&&S.cmtFocus!==null){const f=document.querySelector(`[data-addcmt="${S.cmtFocus}"]`);S.cmtFocus=null;if(f)f.focus();}
}
const cmtFailed=(e,what)=>toast(e.message==='403'?'Others may only read here, not comment.':e.message==='409'?'Others have replied to this comment. Resolve it instead.':e.message==='400'?'A comment has 1 to 10000 characters.':what);
async function postComment(key,block,body,parent){
  const d=D[key];
  try{const r=await fetch(pageURL(key)+'/comments',{...same,method:'POST',body:JSON.stringify({block:block||'',body,parent:parent||0})});if(!r.ok)throw new Error(String(r.status));
    d.cmts=d.cmts||[];d.cmts.push(await r.json());sortCmts(d);recount(d);return true;}
  catch(e){cmtFailed(e,'The comment could not be saved. The text stays in the field.');return false;}
}
async function changeComment(key,id,patch){
  const d=D[key];
  try{const r=await fetch(cmtURL(id),{...same,method:'PATCH',body:JSON.stringify(patch)});if(!r.ok)throw new Error(String(r.status));
    const c=await r.json(),k=d.cmts.findIndex(x=>x.id===id);if(k>=0)d.cmts[k]=c;recount(d);render();return true;}
  catch(e){cmtFailed(e,'The comment could not be changed.');return false;}
}
async function removeComment(key,id){
  const d=D[key];
  try{const r=await fetch(cmtURL(id),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));
    d.cmts=d.cmts.filter(x=>x.id!==id&&x.parent!==id);recount(d);render();}
  catch(e){cmtFailed(e,'The comment could not be deleted.');}
}
/* A comment is an entry of C22's timeline: who and when in its head, the text, what it can do below; a reply is an entry of
   the timeline inside its comment, a resolved thread data-state="done" with a badge. */
function cmtRowHtml(d,c,t,i,reply,replies=''){
  const IC={cmtreply:'back',cmtres:'check',cmtedit:'pencil',cmtdel:'trash'};
  const B=(act,label)=>`<button type="button" class="btn" data-variant="ghost" data-size="xs" data-act="${act}" data-w="${i}" data-id="${c.id}">${icon(IC[act])}${label}</button>`,acts=[],done=!reply&&!!c.resolved;
  if(!reply&&d.canComment&&!c.resolved)acts.push(B('cmtreply','Reply'));
  if(!reply&&c.canResolve)acts.push(B('cmtres',c.resolved?'Reopen':'Resolve'));
  if(c.mine&&d.canComment)acts.push(B('cmtedit','Edit'),B('cmtdel',t.cdel===c.id?'Delete? Yes':'Delete'));
  return `<li data-cmt="${c.id}"${done?' data-state="done"':''}><header><b>${esc(c.who)}</b><time>${when(c.at)}${c.edited?' · edited':''}</time>${done?'<span class="badge" data-variant="outline">resolved</span>':''}</header>
    ${t.cedit===c.id?`<input class="input" data-size="sm" data-editcmt="${i}" data-id="${c.id}" value="${esc(c.body)}" maxlength="10000" aria-label="Change the comment, then Enter">`:`<p>${esc(c.body)}</p>`}
    ${done?`<p>Resolved by ${esc(c.resolvedBy)}, ${when(c.resolved)}</p>`:''}${acts.length?`<div class="flex flex-wrap gap-1">${acts.join('')}</div>`:''}${replies}</li>`;
}
function dbCmtHtml(t,i){
  const key=t.d,d=D[key],head=`${icon('chat')}Comments`;
  if(!d.cmts){S.cmtFocus=i;if(!d.cmtLoading)setTimeout(()=>loadComments(key));return drawer('Comments',head,hint('Loading…'),'cmt',i);}
  const sel=t.cb||'',blk=sel&&d.blocks.find(b=>b.id===sel),label=k=>{if(!k)return 'The whole page';const b=d.blocks.find(x=>x.id===k);return b?blockLabel(b):'A block that is gone';};
  const tops=d.cmts.filter(c=>!c.parent),thread=c=>{const rs=d.cmts.filter(r=>r.parent===c.id).map(r=>cmtRowHtml(d,r,t,i,true)).join('');
    const field=t.creply===c.id?`<input class="input" data-size="sm" data-replycmt="${i}" data-id="${c.id}" placeholder="Reply, then Enter" maxlength="10000" aria-label="Reply">`:'';
    return cmtRowHtml(d,c,t,i,false,(rs?`<ol class="timeline" data-size="sm">${rs}</ol>`:'')+field);};
  const list=tops.filter(c=>c.block===sel).map(thread).join(''),here=list?`<ol class="timeline" data-size="sm">${list}</ol>`:hint('No comments on this yet.');
  const add=d.canComment&&(!sel||blk)?`<div class="input-group"><input class="input" data-addcmt="${i}" placeholder="Write a comment, then Enter" maxlength="10000" aria-label="Write a comment"></div>`:'';
  const why=!d.canComment?hint('Others may only read here, so comments can be read, not added.'):sel&&!blk?hint('This block is gone. Its comments stay and can still be answered.'):'';
  const pos=k=>!k?-1:(x=>x<0?1e9:x)(d.blocks.findIndex(b=>b.id===k));
  const rest=[...new Set(tops.map(c=>c.block))].filter(k=>k!==sel).sort((x,y)=>pos(x)-pos(y)).map(k=>cmtGo(i,k,label(k),`${k?'#'+esc(k)+' · ':''}${d.cmts.filter(c=>c.block===k).length}`)).join('');
  return drawer('Comments',head,`<h3 data-cfor><span>${esc(label(sel))}</span>${sel?` <small>#${esc(sel)}</small>`:''}</h3>${here}${add}${why}
    ${rest?`<h3>Elsewhere on this page</h3><div class="item-group" role="list">${rest}</div>`:''}${sel?'':hint('A comment can also belong to one block: click the speech bubble beside it, or the one in its bar while editing.')}`,'cmt',i);
}
function cmtAct(act,t,i,id){
  const key=t.d,c=(D[key].cmts||[]).find(x=>String(x.id)===String(id));if(!c)return;
  if(act==='cmtreply'){t.creply=t.creply===c.id?null:c.id;render();const f=document.querySelector(`[data-replycmt="${i}"]`);if(f)f.focus();return;}
  if(act==='cmtedit'){t.cedit=c.id;render();const f=document.querySelector(`[data-editcmt="${i}"]`);if(f){f.focus();f.select();}return;}
  if(act==='cmtres'){changeComment(key,c.id,{resolved:!c.resolved});return;}
  /* Deleting asks in place: the first click turns the button into the question, the second deletes. */
  if(t.cdel!==c.id){t.cdel=c.id;render();return;}
  t.cdel=null;removeComment(key,c.id);
}
/* Enter sends what was written in the drawer of a page of the database, Escape leaves a reply or a change. A field that
   could not be sent keeps its text. */
function cmtKey(x,e){
  const ds=x&&x.dataset;if(!ds)return false;
  const field=ds.addcmt!==undefined?'addcmt':ds.replycmt!==undefined?'replycmt':ds.editcmt!==undefined?'editcmt':null;if(!field)return false;
  const i=+ds[field],t=at(i);if(!t||!t.d||!D[t.d].blocks)return false;
  if(e.key==='Escape'){e.preventDefault();t.creply=null;t.cedit=null;render();return true;}
  if(e.key!=='Enter')return false;
  e.preventDefault();const v=x.value.trim();if(!v)return true;
  const typed=x.value,again=()=>document.querySelector(`[data-${field}="${i}"]`)||document.querySelector(`[data-addcmt="${i}"]`);
  const done=ok=>{if(ok){if(field==='replycmt')t.creply=null;if(field==='editcmt')t.cedit=null;}render();const f=again();if(f){if(!ok)f.value=typed;f.focus();}};
  if(field==='editcmt')changeComment(t.d,+ds.id,{body:v}).then(done);
  else postComment(t.d,field==='addcmt'?(t.cb||''):'',v,field==='replycmt'?+ds.id:0).then(done);
  return true;
}
/* Speech bubbles beside the blocks of a page of the database in reading mode, one per block, with its open comments. */
function dbBubbles(art,i,t){
  const d=D[t.d];
  /* Not the marker lines of the editor, which are wrappers of its blocks as well. A card of the quick notes has none: in a grid a bubble beside it would lie on the next card; its handle has Comment
     with the count. */
  art.querySelectorAll('[data-block]:not(.cm-ud-marker, .card)').forEach(el=>{if(el.closest('.editor-embed'))return;const b=d.blocks.find(x=>x.id===el.dataset.block);if(!b)return;const cnt=b.cmts||0;
    if(!cnt&&!d.canComment)return;
    if(t.cmt&&t.cb===b.id)commentFocus(el);
    el.classList.add('comment-anchor');
    el.append(bubble(i,b.id,blockLabel(b),cnt,cnt?cnt+(cnt===1?' open comment':' open comments')+' on this block':'Comment on this block'));});
}
/* The element whose comments the drawer shows is C22's .comment-anchor[data-comment-focus]. */
function commentFocus(el){el.dataset.commentFocus='';}
/* C22's comment bubble: a ghost icon button, the last child of its .comment-anchor, the count as data-count; faint until
   hovered (and on a touch screen), clear when there are comments. */
function bubble(i,id,label,cnt,title){
  const b=document.createElement('button');b.type='button';b.className='btn';b.dataset.variant='ghost';b.dataset.size='icon-xs';b.dataset.comment='';
  b.dataset.act='cblk';b.dataset.w=i;b.dataset.id=id;b.dataset.l=label;if(cnt)b.dataset.count=cnt;b.title=title;b.setAttribute('aria-label',title);b.innerHTML=icon('chat');
  return b;
}
/* Opens the comments of the focused window on one block of a page; a menu of the block in reading mode calls it. */
function commentOn(docKey,blockId){
  const d=D[docKey];if(!d)return;
  let t=at(S.focus);if(!t||t.d!==docKey){openDoc(docKey);t=at(S.focus);}if(!t)return;
  const b=d.blocks&&d.blocks.find(x=>x.id===blockId);
  t.cmt=true;t.gear=false;t.cb=blockId||null;t.cbl=b?blockLabel(b):blockId;t.creply=null;t.cedit=null;render();
  const f=document.querySelector(`[data-addcmt="${S.focus}"]`);if(f)f.focus();
}

/* Lines and spaces. In nav2 they are the user's own and stay in this browser; in a collection of the database they are
   objects, shared like everything in it (2026-11-08). */
const decoId=()=>'d'+Date.now().toString(36)+Math.random().toString(36).slice(2,6);
function decoDoc(id,kind){D[id]=fill({t:kind,n:kind==='line'?'Line':'Space',kids:[],path:'',by:'',ch:'',vis:''});return id;}
async function addDivider(kind,cont,after){
  if(!canPlace(cont))return;
  const list=D[cont].kids,k=after?list.indexOf(after)+1:0;
  if(!D[cont].db){list.splice(k,0,decoDoc(decoId(),kind));rebuildPlaces();return;}
  try{const r=await fetch('/api/dividers',{...same,method:'POST',body:JSON.stringify({kind,parent:D[cont].sid,after:after?D[after].sid:''})});if(!r.ok)throw new Error(String(r.status));
    const id=addNode(await r.json());list.splice(after?list.indexOf(after)+1:0,0,id);rebuildPlaces();render();}
  catch(e){toast(e.message==='403'?'Others may not change this collection.':'The '+kind+' could not be added.');}
}
async function removeDivider(id){
  const i=C.findIndex(c=>c.id===id);
  if(i>=0){C.splice(i,1);delete D[id];return;}
  const list=parentList(id),k=list.indexOf(id);
  if(D[id].db){try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));}
    catch(e){toast(e.message==='403'?'Others may not change this collection.':'The '+D[id].n.toLowerCase()+' could not be removed.');return;}}
  if(k>=0)list.splice(k,1);rebuildPlaces();render();
}
/* Why the server refused, where it says more than that others may not change it. */
async function refusal(r){try{const m=(await r.json()).error||'';return m.includes(': ')?m.slice(m.indexOf(': ')+2):'';}catch(e){return '';}}
async function moveObject(id,cont,after,undo){
  try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/move',{...same,method:'POST',body:JSON.stringify({parent:cont?D[cont].sid:'',after:after?D[after].sid:''})});if(!r.ok)throw new Error(r.status===403&&await refusal(r)||String(r.status));return true;}
  catch(e){undo();toast(e.message==='403'?'Others may not change this.':/^\d+$/.test(e.message)?'It could not be moved there.':e.message);return false;}
}
/* A copy lands right after what it copies, or where a drop with Ctrl puts it. In the database the server copies the
   whole tree; made-up content is copied here. Only the top copy says it is one. */
function copyLocal(id){const n='k'+(++S.newN),d=D[id];D[n]={...d,kids:d.kids.map(copyLocal)};return n;}
async function copyTo(ids,p,after){
  let prev=after;
  for(const id of ids){
    let n;
    if(D[id].db){try{const r=await fetch('/api/objects/'+encodeURIComponent(D[id].sid)+'/copy',{...same,method:'POST',body:JSON.stringify({parent:p?D[p].sid:'',after:prev?D[prev].sid:''})});if(!r.ok)throw new Error(String(r.status));n=addNode(await r.json());}
      catch(e){toast(e.message==='403'?'Others may not add here.':'It could not be copied.');break;}}
    else{n=copyLocal(id);D[n].n=D[id].n+' (copy)';}
    if(p){const L=D[p].kids;L.splice(prev?L.indexOf(prev)+1:0,0,n);S.tog[p]=true;}
    else C.splice(prev?C.findIndex(c=>c.id===prev)+1:0,0,asTop({id:n,i:D[n].private?'lock':D[n].i||'folder'}));
    prev=n;S.just=n;
  }
  rebuildPlaces();render();
}
/* Deleting takes an entry out of every list, and every window showing it or something below it lets go of it. Its data
   stay, marked, so the made-up content that points at it does not break, and a restore draws it anew. */
function dropLocal(id){
  const gone=[],walk=x=>{gone.push(x);(D[x].kids||[]).forEach(walk);};walk(id);
  const list=parentList(id),k=list.indexOf(id);if(k>=0)list.splice(k,1);
  for(let i=C.length-1;i>=0;i--)if(gone.includes(C[i].id))C.splice(i,1);
  gone.forEach(x=>{D[x].gone=true;delete S.open[x];delete S.pin[x];});
  S.wins.forEach(w=>{w.hist=(w.hist||[]).filter(x=>!gone.includes(x));
    for(let j=w.tabs.length-1;j>=0;j--){if(!gone.includes(w.tabs[j].d))continue;if(w.tabs[j].general)fallback(w);else{w.tabs.splice(j,1);if(w.a>=j&&w.a>0)w.a--;}}});
  compact();S.sel=null;rebuildPlaces();
}
const trashOpen=()=>S.wins.some(w=>w.tabs.some(t=>t.d==='trash:'));
/* What happens where something in use goes (ADR-22): for each thing, delete it everywhere (a placeholder says who removed
   it), a copy in every use, or a new master among the uses one may change. Nothing is chosen in advance, and nothing goes
   before each thing has its way; Keep it gives up. With more than one, "The same for all" takes the choice of the first. */
function askWays(items){
  return new Promise(resolve=>{S.ways={items:items.map(x=>({...x,way:'',master:''})),same:false,resolve};render();});
}
function endWays(ok){
  const w=S.ways;if(!w)return;S.ways=null;render();
  w.resolve(ok?Object.fromEntries(w.items.map(x=>[x.ref,x.way==='master'?{way:'master',master:x.master}:{way:x.way}])):null);
}
function setWay(k,way,master){
  const w=S.ways;if(!w)return;
  const put=x=>{if(way==='master'){if(!x.uses.some(u=>u.canEdit&&u.page+'#'+u.block===master))return;x.master=master;}else x.master='';x.way=way;};
  if(w.same)w.items.forEach(put);else put(w.items[k]);render();
}
function wayLabel(ref){
  const [sid,t='']=ref.split('#'),k=Object.keys(D).find(x=>D[x].sid===sid),page=k?plain(D[k].n):'a page';
  if(!t)return 'The page '+page;const [b,s]=t.split('.');
  return (s==='section'?'The section of #'+b:s?'Part #'+t:'Block #'+b)+' in '+page;
}
function waysHtml(){
  const w=S.ways;if(!w)return '';
  const ready=w.items.every(x=>x.way&&(x.way!=='master'||x.master)),hidden=w.items.some(x=>x.uses.some(u=>!u.canEdit)||x.hidden);
  /* C22's card over the workspace, as the editing block of C22 shows it (a dialog would fade in again with every render):
     a heading, one section per thing with its ways as buttons that stay pressed and the new master as a select, "The
     same for all", then Delete or Keep it. */
  const btn=(attrs,label,v='outline')=>`<button type="button" class="btn" data-variant="${v}" data-size="xs" ${attrs}>${label}</button>`;
  return `<section class="card fixed left-1/2 top-1/2 z-50 w-full max-w-lg max-h-full -translate-x-1/2 -translate-y-1/2 overflow-y-auto shadow-lg" role="dialog" data-ways aria-labelledby="ways-h">
    <header><h3 id="ways-h" class="text-muted-foreground text-xs font-semibold tracking-wider uppercase">Used elsewhere</h3><p class="text-muted-foreground text-sm">Choose what happens where it is used. Nothing is chosen for you.${hidden?' Pages you may not change keep a note that it is gone, and their owners are told.':''}</p></header>
    <section class="flex flex-col gap-4">${w.items.map((x,k)=>{const can=x.uses.filter(u=>u.canEdit);
      return `<div class="flex flex-col gap-1.5 border-t pt-3" data-wayitem="${k}"><h4 class="font-semibold">${esc(wayLabel(x.ref))}</h4><p class="text-muted-foreground text-sm">Used in ${[...x.uses.map(u=>esc(u.title)),...(x.hidden?[x.hidden+(x.hidden===1?' page':' pages')+' you cannot see']:[])].join(', ')}</p>
        <div class="flex flex-wrap items-center gap-1.5">${btn(`data-act="way" data-k="${k}" data-way="everywhere" aria-pressed="${x.way==='everywhere'}"`,'Delete everywhere')}${btn(`data-act="way" data-k="${k}" data-way="copies" aria-pressed="${x.way==='copies'}"`,'A copy in every use')}
        <select class="select" data-size="sm" data-wmaster="${k}" aria-label="New master"${can.length&&!x.ref.endsWith('#')&&x.ref.includes('#')?'':' disabled title="No use you may change"'}><option value="">New master in…</option>${can.map(u=>`<option value="${u.page}#${u.block}"${x.way==='master'&&x.master===u.page+'#'+u.block?' selected':''}>${esc(u.title)}</option>`).join('')}</select></div></div>`;}).join('')}
    ${w.items.length>1?`<label class="label"><input class="input" type="checkbox" data-wsame${w.same?' checked':''}> The same for all</label>`:''}</section>
    <footer class="flex gap-2">${btn(`data-act="waysgo"${ready?'':' disabled title="Choose a way for each"'}`,'Delete','destructive').replace('data-size="xs"','data-size="sm"')}${btn(`data-act="waysno"`,'Keep it','outline').replace('data-size="xs"','data-size="sm"')}</footer></section>`;
}
async function deleteObject(id){
  const d=D[id];if(!d)return;
  if(d.db){try{let r=await fetch('/api/objects/'+encodeURIComponent(d.sid),{...same,method:'DELETE'});
      /* What of it is embedded elsewhere is gone through one by one, with the same for all (ADR-22). */
      if(r.status===409){const ways=await askWays((await r.json()).inUse||[]);if(!ways){toast('Nothing was deleted.');return;}
        r=await fetch('/api/objects/'+encodeURIComponent(d.sid),{...same,method:'DELETE',body:JSON.stringify({ways})});}
      if(!r.ok)throw new Error(r.status===403&&await refusal(r)||String(r.status));}
    catch(e){toast(e.message==='403'?'Others may not delete this.':/^\d+$/.test(e.message)?'It could not be deleted.':e.message);return;}}
  dropLocal(id);toast(d.db?plain(d.n)+' is in the trash.':plain(d.n)+' is deleted.');if(d.db&&trashOpen())loadTrash();
}
/* The trash is a page of the window: what the viewer deleted, and what was theirs, newest first, each with where it lay.
   Restore puts it back there, at the end; deleting for good asks in place as well. */
TYPE.trash=['trash','Trash'];
D['trash:']=fill({t:'trash',n:'Trash',w:'wide',tool:true,path:'not a file, what was deleted',place:'Workspace',by:'',ch:'',vis:'Only you: what you deleted, and what was yours',
  get stat(){return S.trash?S.trash.length+(S.trash.length===1?' entry':' entries'):'Loading…';},
  get html(){const L=S.trash;if(!L)return '<p class="text-muted-foreground text-sm">Loading…</p>';if(!L.length)return '<p class="text-muted-foreground text-sm">The trash is empty.</p>';
    return `<p class="text-muted-foreground text-sm">What you deleted, and what was yours. Restore puts it back where it lay, at the end. What lay in something that is in the trash too comes back after that. After 30 days it goes for good by itself.</p>
      <p><button type="button" class="btn" data-variant="outline" data-size="xs" data-act="trempty" aria-pressed="${S.trashAsk==='all'}">${icon('trash')}${S.trashAsk==='all'?'Empty the trash? Yes':'Empty the trash'}</button></p>
      <div class="table-container"><table class="table"><tr><th>Name</th><th>Where it lay</th><th>Deleted</th><th>By</th><th></th></tr>${L.map(x=>`<tr data-trash="${x.id}"><td>${x.n}</td><td>${x.place||'Workspace'}${x.placeGone?' (in the trash)':''}</td><td>${x.at}</td><td>${x.who}</td>
        <td><button type="button" class="btn" data-variant="outline" data-size="xs" data-act="trrestore" data-id="${x.id}"${x.placeGone?' title="First restore what it lay in"':''}>${icon('back')}Restore</button> <button type="button" class="btn" data-variant="outline" data-size="xs" data-act="trpurge" data-id="${x.id}" aria-pressed="${S.trashAsk===x.id}">${icon('trash')}${S.trashAsk===x.id?'Delete for good? Yes':'Delete for good'}</button></td></tr>`).join('')}</table></div>`;}});
/* The list of problems is a page of the window as well: what does not resolve in the pages one sees, and what the folder
   source ran into (M-3). A broken reference is a visible error, never a silent gap. */
TYPE.problems=['info','Problems'];
D['problems:']=fill({t:'problems',n:'Problems',w:'wide',tool:true,path:'not a file, what does not resolve',place:'Workspace',by:'',ch:'',vis:'Only you: in what you may see',
  get stat(){const P=S.problems;return P?(P.db.length+P.src.length)+' problems':'Loading…';},
  get html(){const P=S.problems;if(!P)return '<p class="text-muted-foreground text-sm">Loading…</p>';
    if(!P.db.length&&!P.src.length)return '<p class="text-muted-foreground text-sm">Nothing is broken.</p>';
    const where=x=>SID[x.page]&&D[SID[x.page]]?`<button type="button" class="btn" data-variant="outline" data-size="xs" data-act="open" data-id="${SID[x.page]}">${icon('page')}${x.title}</button>`:`<a href="/o/${x.page}">${x.title}</a>`;
    return `<p class="text-muted-foreground text-sm">Embeddings and links that find nothing, in the pages you may see, and what the folder source ran into. Each stays visible in its page until it is fixed.</p>
      ${P.db.length?`<h2>Pages</h2><div class="table-container"><table class="table"><tr><th>Page</th><th>Block</th><th>Reference</th><th>Why</th></tr>${P.db.map(x=>`<tr data-problem="${x.page}#${x.block}"><td>${where(x)}</td><td>#${x.block}</td><td><code>${x.ref}</code></td><td>${x.why}</td></tr>`).join('')}</table></div>`:''}
      ${P.src.length?`<h2>Folder source</h2><div class="table-container"><table class="table"><tr><th>File</th><th>Problem</th></tr>${P.src.map(x=>`<tr><td>${x.source}</td><td>${x.message}</td></tr>`).join('')}</table></div>`:''}
      ${P.more?'<p class="text-muted-foreground text-sm">There are more blocks than were read.</p>':''}`;}});
async function loadProblems(){
  try{const r=await fetch('/api/problems',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    const a=await r.json();S.problems={db:a.database.map(x=>({page:esc(x.page),title:esc(x.title),block:esc(x.block),ref:esc(x.ref),why:esc(x.why)})),src:a.source.map(x=>({source:esc(x.source),message:esc(x.message)})),more:!!a.more};}
  catch(e){S.problems={db:[],src:[],more:false};toast('The problems could not be read.');return;}
  render();
}
async function loadTrash(){
  try{const r=await fetch('/api/trash',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));
    S.trash=(await r.json()).map(x=>({id:esc(x.id),n:esc(x.n),t:x.t,place:esc(x.place),placeGone:!!x.placeDeleted,at:when(x.at),who:esc(x.who)}));}
  catch(e){S.trash=[];toast('The trash could not be read.');return;}
  render();
}
async function restoreTrash(sid){
  const x=(S.trash||[]).find(y=>y.id===sid);
  try{const r=await fetch('/api/trash/'+encodeURIComponent(sid)+'/restore',{...same,method:'POST'});
    if(r.status===400){toast(x&&x.placeGone?'First restore '+plain(x.place.split(' › ').pop())+', where it lay.':'It cannot go back there.');return;}
    if(!r.ok)throw new Error(String(r.status));
    const a=await r.json(),id=addNode(a.node),p=a.parent?SID[a.parent]:'';
    if(a.parent&&!(D[p]&&!D[p].gone)){toast('Restored. Reload to see it where it lay.');loadTrash();return;}
    if(p){D[p].kids.push(id);S.tog[p]=true;}else C.splice(C.filter(c=>D[c.id]&&D[c.id].real).length,0,asTop({id,i:D[id].i}));
    rebuildPlaces();const top=COLL[id]?COLL[id].id:id;if(C.some(c=>c.id===top))S.open[top]=true;S.just=id;
    toast(plain(D[id].n)+' is back.');loadTrash();}
  catch(e){toast(e.message==='403'?'Others may not add there.':'It could not be restored.');}
}
async function emptyTrash(){
  try{const r=await fetch('/api/trash',{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));toast('The trash is empty.');}
  catch(e){toast('The trash could not be emptied.');}
  loadTrash();
}
async function purgeTrash(sid){
  try{const r=await fetch('/api/trash/'+encodeURIComponent(sid),{...same,method:'DELETE'});if(!r.ok)throw new Error(String(r.status));toast('Deleted for good.');}
  catch(e){toast('It could not be deleted.');}
  loadTrash();
}
/* Sorting the children by name keeps lines and spaces where they are: each group between them is sorted on its own. */
function sortKids(id,dir){
  const L=D[id].kids,old=L.slice(),groups=[[]];
  L.forEach(x=>{if(isDeco(x))groups.push([x],[]);else groups[groups.length-1].push(x);});
  const by=(a,b)=>dir*plain(D[a].n).localeCompare(plain(D[b].n),undefined,{numeric:true,sensitivity:'base'});
  const out=groups.flatMap(g=>g.length===1&&isDeco(g[0])?g:g.slice().sort(by));
  L.length=0;L.push(...out);rebuildPlaces();render();
  if(D[id].db)moveAll(out.map((x,i)=>[x,id,i?out[i-1]:'']),()=>{L.length=0;L.push(...old);rebuildPlaces();render();});
}
/* Several entries move one after the other, each after the one before, so the server keeps their order. */
async function moveAll(moves,undo){for(const [id,cont,after] of moves)if(!await moveObject(id,cont,after,undo))return;}
/* The order of nav2 with its lines, spaces and shown collections, as the account keeps it. A collection it does not know
   yet keeps its place. */
const n2Of=()=>C.map(c=>isDeco(c.id)?{k:D[c.id].t,id:c.id}:c.alias?{k:'show',id:c.id}:c.id);
function applyN2(list){
  if(!Array.isArray(list))return;const byId={};C.forEach(c=>{byId[c.id]=c;});const out=[];
  list.forEach(x=>{if(x&&typeof x==='object'){if((x.k==='line'||x.k==='space')&&/^d[0-9a-z]{4,20}$/.test(x.id)&&!D[x.id])out.push(asTop({id:decoDoc(x.id,x.k),i:'grid'}));
      else if(x.k==='show'&&typeof x.id==='string'&&D[x.id]&&D[x.id].t==='coll'&&!byId[x.id]&&!out.some(c=>c.id===x.id))out.push(asAlias(x.id));}
    else if(typeof x==='string'&&byId[x]){out.push(byId[x]);delete byId[x];}});
  C.forEach((c,i)=>{if(byId[c.id])out.splice(Math.min(i,out.length),0,c);});
  C.length=0;out.forEach(c=>C.push(c));
}

/* The settings of the workspace: widths, the order of nav2, what is kept open, zoom and theme. */
/* into is where New lands, Quick notes or a collection: the last choice stays (ADR-16, M-8). */
const KEEP=['s1','s2','w3','fav','pin','limit','scale','theme','openEdit','into'];
/* With the database the account keeps them, so they follow it from browser to browser (M-28); a change goes to the
   server a moment later, once, and the browser keeps a copy for the next start and for the workspace without database. */
let sentSettings=null,settingsTimer=0;
function saveSettings(){
  const o={n2:n2Of()};KEEP.forEach(k=>{o[k]=S[k];});const j=JSON.stringify(o);
  try{localStorage.setItem('ud.workspace',j);}catch(e){}
  if(!S.canCreate||j===sentSettings)return;
  clearTimeout(settingsTimer);settingsTimer=setTimeout(()=>{sentSettings=j;fetch('/api/settings',{...same,method:'PUT',body:j}).then(r=>{if(!r.ok)sentSettings=null;}).catch(()=>{sentSettings=null;});},600);
}
/* The account's settings come first; the browser's copy only where the account has none yet. */
function loadSettings(server){
  let o=null;
  if(server&&typeof server==='object'&&!Array.isArray(server)&&Object.keys(server).length)o=server;
  else try{o=JSON.parse(localStorage.getItem('ud.workspace')||'null');}catch(e){o=null;}
  if(!o||typeof o!=='object')return null;
  KEEP.forEach(k=>{if(o[k]!==undefined&&k!=='fav'&&k!=='pin')S[k]=o[k];});return o;
}
/* ---- The view Files (ADR-28): roots, folders and files of the content store ----
   nav2 lists the roots: the folder of the tenant, the user's own folder and collections. nav3 shows one folder at a
   time with its path, and takes uploads by its button or by dropping files on it; a file opens in the window with its
   preview, its versions and a download. Without the store on the server, the view keeps the made-up content. */
/* busy counts the requests of the view under way: each draws the view again when its answer comes. */
const FS={on:false,pending:false,roots:[],dirs:{},cur:{},ups:[],chunk:8<<20,admin:false,form:null,busy:0};
const fbusy=async work=>{FS.busy++;try{return await work();}finally{FS.busy--;}};
TYPE.file=['page','File'];TYPE.fdir=['folder','Folder'];
const fsize=n=>n<1024?n+' B':n<1<<20?Math.round(n/1024)+' KB':n<1<<30?(n/1048576).toFixed(1)+' MB':(n/1073741824).toFixed(1)+' GB';
const fget=async url=>{const r=await fetch(url,{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)throw new Error(String(r.status));return r.json();};
const fsend=async(url,method,body)=>{const r=await fetch(url,{...same,method,body:body===undefined?undefined:JSON.stringify(body)});
  const a=r.status===204?{}:await r.json().catch(()=>({}));if(!r.ok)throw new Error(a.error||String(r.status));return a;};
const loadRoots=()=>fbusy(async()=>{
  let a;try{const r=await fetch('/api/files',{credentials:'same-origin',headers:{Accept:'application/json'}});if(!r.ok)return;a=await r.json();}catch(e){return;}
  FS.on=true;FS.roots=a.roots;FS.admin=a.admin;FS.chunk=a.chunk||FS.chunk;FS.text=a.text||{};
  if(!FS.roots.some(x=>x.id===S.sec.files)&&!['ext','newfcoll'].includes(S.sec.files))S.sec.files=(FS.roots.find(x=>x.role==='user')||FS.roots[0]||{id:'ext'}).id;
  render();
});
const loadDir=id=>fbusy(async()=>{
  FS.dirs[id]=FS.dirs[id]||{loading:true,entries:[]};
  try{const a=await fget('/api/files/'+encodeURIComponent(id)+'/list');FS.dirs[id]={...a,loading:false};}
  catch(e){FS.dirs[id]={failed:true,entries:[]};}
  render();
});
const froot=()=>FS.roots.find(x=>x.id===S.sec.files);
/* The path from the root to the folder shown: each step was opened from the one before, so all of them are known. */
function fcrumbs(root,cur){const out=[];for(let x=cur;x;x=FS.dirs[x]&&FS.dirs[x].parent){out.unshift(x);if(x===root)break;}
  return out.map((x,k)=>k===out.length-1?`<span>${esc(FS.dirs[x]&&FS.dirs[x].n||'')}</span>`:`<a href="#" data-act="fdir" data-id="${x}">${esc(FS.dirs[x].n)}</a>`).join(' › ');}
function filesNav3(){
  const sec=S.sec.files;
  /* A small form of a panel: C22's small field and outline button side by side. */
  const pform=(attrs,keep,value,label)=>`<form class="flex gap-1.5" ${attrs}><input class="input" data-size="sm" name="title" value="${value}" data-keep="${keep}" aria-label="${label}" required maxlength="200"><button type="submit" class="btn" data-variant="outline" data-size="sm">Make</button></form>`;
  if(sec==='newfcoll')return `<div>${panel(`${ptitle('New collection')}${pform('data-fform="coll"','fcoll','Files','Name of the collection')}${hint('A collection of files of your own: the next step asks with whom to share it. Until then nobody else sees it.')}`)}</div>`;
  const r=froot();if(!r)return '<p class="text-muted-foreground text-xs">Loading…</p>';
  const cur=FS.cur[r.id]||r.id,d=FS.dirs[cur];
  if(!d){loadDir(cur);return '<p class="text-muted-foreground text-xs">Loading…</p>';}
  const room=r.limit>0?`${fsize(r.used)} of ${fsize(r.limit)}`:`${fsize(r.used)} used`;
  const rows=d.entries.map(e=>{const t=at(S.focus),key=SID[e.id],sel=t&&key&&t.d===key;
    return e.t==='file'?`<li><div data-frow="${e.id}"><span data-toggle aria-hidden="true"></span><button type="button" data-act="fopen" data-id="${e.id}" title="${esc(e.n)}"${sel?' aria-current="page"':''}>${icon('page')}<span>${esc(e.n)}</span></button><small title="${e.versions} versions">${fsize(e.size)}</small></div></li>`
      :`<li><div data-frow="${e.id}"><button type="button" data-toggle data-act="fdir" data-id="${e.id}" aria-expanded="false" aria-label="Open ${esc(e.n)}">${icon('chev')}</button><button type="button" data-act="fdir" data-id="${e.id}" title="${esc(e.n)}">${icon('folder')}<span>${esc(e.n)}</span></button></div></li>`;}).join('');
  /* An upload under way: its name, C22's progress bar, its state. */
  const ups=FS.ups.filter(u=>u.dir===cur).map(u=>`<div class="flex flex-col gap-1" data-fup="${esc(u.name)}"><span class="text-xs">${esc(u.name)}</span><progress class="progress" max="${u.size||1}" value="${u.done}" aria-label="${esc(u.name)}"></progress><span class="text-muted-foreground text-xs">${u.err?esc(u.err):u.state}</span></div>`).join('');
  const tools=d.canEdit?`<div class="flex flex-wrap gap-1.5">${pbtn(`data-act="fup" data-id="${cur}"`,'Upload','plus')}${pbtn(`data-act="fnewdir" data-id="${cur}"`,'New folder','folder')}</div>
    <input type="file" multiple hidden data-fpick="${cur}" aria-label="Files to upload">`:hint(r.role==='tenant'?'Everyone reads this folder; its writers change it.':'You may only read here.');
  const form=FS.form===cur?pform(`data-fform="dir" data-id="${cur}"`,`fdir:${cur}`,'New folder','Name of the folder'):'';
  return `<div id="tree" class="flex flex-col gap-2" data-fdrop="${d.canEdit?cur:''}">${colLabel(esc(r.n))}${hint(`${fcrumbs(r.id,cur)} · ${room}`,' data-fpath')}${tools}${form}
    <ul class="workspace-tree">${rows||(d.loading?'':'<li class="text-muted-foreground">Empty. Upload files or drop them here.</li>')}</ul>${ups}</div>`;
}
/* A file opens in the window like any document: what the browser may show inline is shown, everything else is a
   download; the versions list who changed it and when, and an older one comes back as a new version. */
const openFile=sid=>fbusy(()=>showFile(sid));
async function showFile(sid){
  const key=keyOf(sid);
  if(!D[key])pseudo(key,{t:'file',n:'…',path:'the content store',vis:'',stat:'',html:'<p class="text-muted-foreground text-sm">Loading…</p>'});
  D[key].sid=sid;openDoc(key);render();
  let f;try{f=await fget('/api/files/'+encodeURIComponent(sid));}catch(e){D[key].html='<p class="text-muted-foreground text-sm">This file could not be loaded.</p>';render();return;}
  const url=f.content,dl=url+'?download=1';
  /* The preview sits in C22's frame (figure.card): never wider than the window. A spreadsheet shows as tables (sheets.js,
     ADR-31), its PDF preview one click away. */
  const fig=(inner,attrs='')=>`<figure class="card items-center p-1"${attrs}>${inner}</figure>`;
  const pdf=f.preview==='pdf'?fig('<p class="text-muted-foreground text-sm">Loading the preview…</p>',` data-pdf="${f.pdf}"`):'';
  const preview=f.sheets?`<div data-sheets="${sid}"></div>${pdf?`<details><summary>PDF preview</summary>${pdf}</details>`:''}`
    :f.preview==='image'?fig(`<a href="${url}" target="_blank" rel="noopener"><img class="max-w-full" src="${f.thumb}" alt="${esc(f.n)}"></a>`)
    :pdf||(f.preview==='video'?fig(`<video class="max-w-full" src="${url}" controls preload="metadata"></video>`):`<div class="alert"><section>No preview of ${esc(f.type)}. Download it to open it.</section></div>`);
  if(f.sheets&&window.UDSheets)UDSheets.forget(sid);
  const vrows=f.history.map((v,k)=>[`${v.n}`,when(v.at),esc(v.who),fsize(v.size),`<a class="btn" data-variant="link" data-size="sm" href="${url}?v=${v.n}&amp;download=1">Download</a>`+(k&&f.canEdit?`<button type="button" class="btn" data-variant="link" data-size="sm" data-act="frestore" data-id="${sid}" data-n="${v.n}">Restore</button>`:'')]);
  Object.assign(D[key],{usedIn:(f.usedIn||[]).map(esc),n:esc(f.n),path:esc(f.place)+' › '+esc(f.n),place:esc(f.place),by:esc(f.by||f.owner),ch:when(f.ch),created:when(f.created,false),vis:'As its folder',share:f.share||null,tags:(f.tags||[]).map(esc),stat:`${fsize(f.size)} · ${f.versions} version${f.versions===1?"":"s"}`,fparent:f.parent,
    html:`${preview}${f.share&&(f.share.open_by||[]).length?note(`Open over Samba by ${esc(f.share.open_by.join(', '))}: the web changes nothing of it until it is closed.`,' data-open-note'):''}<div class="flex flex-wrap gap-1.5" data-file-actions><a class="btn" data-variant="outline" data-size="xs" href="${dl}">${icon('down')}Download</a>${f.canEdit?`${pbtn(`data-act="fup" data-file="${sid}"`,'Upload a new version','plus')}${pbtn(`data-act="ftrash" data-id="${sid}"`,'Move to the trash','trash')}<input type="file" hidden data-fpick-file="${sid}" aria-label="New version">`:''}</div>
      <h2>Versions</h2>${f.canEdit?tableHtml(['Version','When','Who','Size',''],vrows):'<p class="text-muted-foreground text-sm">Versions are for whoever may edit this file.</p>'}`});
  render();
}
/* PDF, and Office files through the optional Office module, show in pdf.js, served by UnderDoc itself
   (assets/app/pdfjs). The pages are drawn once; each redraw of the window takes the same canvases again. */
const PDFS={};
function mountPDFs(){
  document.querySelectorAll('[data-pdf]').forEach(el=>{const url=el.dataset.pdf,p=PDFS[url];
    if(!p){PDFS[url]={state:'loading',nodes:[]};const box=document.createElement('div');
      const v=(document.querySelector('script[src*="/assets/app/demo.js"]')?.src.split('?v=')[1])||'';
      import('/assets/app/pdfjs/viewer.mjs?v='+v).then(m=>m.show(box,url)).then(r=>{PDFS[url]={state:'done',nodes:[...box.children],pages:r.pages,shown:r.shown};render();})
        .catch(e=>{PDFS[url]={state:'failed',nodes:[],error:String(e)};render();});return;}
    if(p.state==='done'&&!el.querySelector('canvas')){el.replaceChildren(...p.nodes);if(p.pages>p.shown){const n=document.createElement('p');n.className='text-muted-foreground text-sm';n.textContent=`The first ${p.shown} of ${p.pages} pages. Download the file for all of them.`;el.append(n);}}
    else if(p.state==='failed'&&!el.dataset.failed){el.dataset.failed='1';el.innerHTML='<div class="alert"><section>The preview could not be drawn. Download the file to open it.</section></div>';}});
}
new MutationObserver(()=>{if(document.querySelector('[data-pdf]'))mountPDFs();}).observe(document.documentElement,{childList:true,subtree:true});
/* Attachments of a page: images pasted into it (ADR-28). They belong to the page; "Move to the files" makes one a file
   in the files of the space, or the user's own, and the page shows the file from then on. */
async function uploadAttachment(key,f){
  const up={name:f.name,size:f.size,done:0,state:'uploading',dir:null};
  const done=await uploadOne(f,{page:D[key].sid},up);loadAttachments(key);return {url:done.url,name:f.name};
}
async function loadAttachments(key){
  try{const r=await fetch('/api/pages/'+encodeURIComponent(D[key].sid)+'/attachments',{credentials:'same-origin',headers:{Accept:'application/json'}});
    if(!r.ok)return;D[key].atts=await r.json();render();}catch(e){}
}
function attachmentsHtml(d){
  if(!d.atts||!d.atts.length)return '';const key=Object.keys(D).find(k=>D[k]===d);
  return `<h3>Attachments</h3>${factList(d.atts.map(a=>[fsize(a.size),`<a href="${a.url}" target="_blank" rel="noopener">${esc(a.name)}</a>${d.canEdit?` <button type="button" class="btn" data-variant="link" data-size="sm" data-act="fatt" data-id="${key}" data-att="${a.id}" data-keep="fatt:${key}:${a.id}">Move to the files</button>`:''}`]))}`;
}
async function moveAttachment(key,att){
  const sid=D[key].sid,top=COLL[key]&&D[COLL[key].id].sid;
  try{let parent=null;
    if(top){const l=await fget('/api/spaces/'+encodeURIComponent(top)+'/files');if(l.root)parent=l.root.id;}
    if(!parent){const a=await fget('/api/files');parent=(a.roots.find(r=>r.role==='user')||{}).id;}
    const f=await fsend('/api/pages/'+encodeURIComponent(sid)+'/attachments/'+encodeURIComponent(att)+'/move','POST',{parent});
    toast(`Moved to the files as ${f.n}.`);D[key].loaded=false;loadDoc(key);}
  catch(e){toast(e.message);}
}
/* Uploads go in chunks, and a broken one resumes where the server says it stands: the browser keeps the ID of each
   upload under its target, name, size and date, so choosing the same file again continues it. */
const upKey=(target,f)=>'ud-upload:'+JSON.stringify([target.parent||target.file||target.page,f.name,f.size,f.lastModified]);
const keep={get:k=>{try{return localStorage.getItem(k);}catch(e){return null;}},set:(k,v)=>{try{localStorage.setItem(k,v);}catch(e){}},del:k=>{try{localStorage.removeItem(k);}catch(e){}}};
const pause=ms=>new Promise(r=>setTimeout(r,ms));
async function uploadOne(f,target,up){
  const k=upKey(target,f);let u=null;
  const kept=keep.get(k);if(kept){try{u=await fget('/api/uploads/'+encodeURIComponent(kept));}catch(e){keep.del(k);}}
  if(!u){u=await fsend('/api/uploads','POST',{...target,name:f.name,size:f.size});keep.set(k,u.id);}
  let off=u.received,tries=0;up.done=off;
  while(off<f.size){
    const end=Math.min(off+(u.chunk||FS.chunk),f.size);let r=null;
    try{r=await fetch('/api/uploads/'+encodeURIComponent(u.id),{method:'PATCH',credentials:'same-origin',headers:{'Upload-Offset':String(off),Accept:'application/json'},body:f.slice(off,end)});}catch(e){r=null;}
    if(r&&(r.ok||r.status===409)){const a=await r.json();off=a.received;up.done=off;tries=0;render();continue;}
    if(r&&r.status!==408&&r.status<500){const a=await r.json().catch(()=>({}));keep.del(k);throw new Error(a.error||String(r.status));}
    /* A break of the network or a server that fails for a moment: a few tries, each waiting longer, then the upload
       waits for the same file to be chosen again. */
    if(++tries>5)throw new Error('the connection broke, choose the file again to resume');
    up.state=`waiting to resume (${tries})`;render();await pause(500*2**tries);
  }
  up.state='finishing';render();
  const done=await fsend('/api/uploads/'+encodeURIComponent(u.id)+'/finish','POST');keep.del(k);return done;
}
/* The way of the browser (ADR-24): the uploader's browser reads the text of a PDF with pdf.js and gives it to the search;
   the server reads it as well where its way is on, and the search knows which text came from where. */
async function browserText(id){
  try{const v=(document.querySelector('script[src*="/assets/app/demo.js"]')?.src.split('?v=')[1])||'';
    const m=await import('/assets/app/pdfjs/viewer.mjs?v='+v),pages=await m.text('/api/files/'+encodeURIComponent(id)+'/pdf');
    const text=pages.join('\n').trim();if(text)await fsend('/api/files/'+encodeURIComponent(id)+'/text','PUT',{text:text.slice(0,250000)});}
  catch(e){/* the server reads it too, where its way is on */}
}
/* The text of an image, read with Tesseract.js (deu+eng) in the same browser; its worker is the one script whose policy
   allows WebAssembly. A browser without WebAssembly SIMD leaves the text to the server. */
const OCR_TYPES=['image/png','image/jpeg','image/webp','image/bmp'];
async function browserOCR(id){
  try{const v=(document.querySelector('script[src*="/assets/app/demo.js"]')?.src.split('?v=')[1])||'';
    const m=await import('/assets/app/tesseract/ocr.mjs?v='+v),text=(await m.text('/api/files/'+encodeURIComponent(id)+'/content')).trim();
    if(text)await fsend('/api/files/'+encodeURIComponent(id)+'/text','PUT',{text:text.slice(0,250000)});}
  catch(e){/* the server reads it too, where its way is on */}
}
async function uploadFiles(files,target,dir){
  for(const f of files){
    const up={name:f.name,size:f.size,done:0,state:'uploading',dir};FS.ups.push(up);render();
    try{const done=await uploadOne(f,target,up);up.state='done';up.result=done;
      if(done.id&&FS.text.browser){if(done.type==='application/pdf')browserText(done.id);else if(OCR_TYPES.includes(done.type))browserOCR(done.id);}}catch(e){up.err=e.message;}
    render();
  }
  setTimeout(()=>{FS.ups=FS.ups.filter(u=>u.err);render();},2500);
  if(dir)await loadDir(dir);
  if(target.file)await openFile(target.file);
  await loadRoots();
}
function filesAct(act,a){
  switch(act){
    case 'fdir':FS.cur[S.sec.files]=a.id;FS.form=null;if(!FS.dirs[a.id]||!FS.dirs[a.id].loading)loadDir(a.id);break;
    case 'fopen':openFile(a.id);break;
    case 'fnewdir':FS.form=FS.form===a.id?null:a.id;break;
    case 'fup':{const pick=a.file?document.querySelector(`[data-fpick-file="${a.file}"]`):document.querySelector(`[data-fpick="${a.id}"]`);if(pick)pick.click();return false;}
    case 'frestore':fsend('/api/files/'+encodeURIComponent(a.id)+'/versions/'+encodeURIComponent(a.n)+'/restore','POST').then(()=>{toast('Restored as a new version.');openFile(a.id);}).catch(e=>toast(e.message));break;
    case 'ftrash':{const key=SID[a.id],parent=D[key]&&D[key].fparent;fsend('/api/objects/'+encodeURIComponent(a.id),'DELETE').then(()=>{toast('Moved to the trash.');const w=S.wins[S.focus];if(w.tabs[0].d===key)fallback(w);if(parent)loadDir(parent);loadRoots();}).catch(e=>toast(e.message));break;}
  }
  return true;
}
document.addEventListener('change',e=>{const n=e.target;if(!n.files||!n.files.length)return;
  if(n.dataset.fpick!==undefined){uploadFiles([...n.files],{parent:n.dataset.fpick},n.dataset.fpick);n.value='';}
  else if(n.dataset.fpickFile!==undefined){uploadFiles([...n.files],{file:n.dataset.fpickFile},D[SID[n.dataset.fpickFile]]&&D[SID[n.dataset.fpickFile]].fparent);n.value='';}});
document.addEventListener('submit',e=>{const f=e.target.closest('[data-fform]');if(!f)return;e.preventDefault();const title=f.elements.title.value.trim();if(!title)return;
  if(f.dataset.fform==='dir'){const parent=f.dataset.id;fsend('/api/folders','POST',{title,parent}).then(()=>{FS.form=null;loadDir(parent);}).catch(e=>toast(e.message));}
  else if(window.UDRights)UDRights.create({title,url:'/api/files/collections',onMade:a=>{S.sec.files=a.id;loadRoots();}});
  else fsend('/api/files/collections','POST',{title}).then(a=>{S.sec.files=a.id;loadRoots();}).catch(e=>toast(e.message));});
/* Files dropped from the desktop onto the folder shown in nav3 are uploaded into it. */
document.addEventListener('dragover',e=>{const z=e.target.closest&&e.target.closest('[data-fdrop]');if(z&&z.dataset.fdrop&&e.dataTransfer&&[...e.dataTransfer.types].includes('Files')){e.preventDefault();e.dataTransfer.dropEffect='copy';}});
document.addEventListener('drop',e=>{const z=e.target.closest&&e.target.closest('[data-fdrop]');if(!z||!z.dataset.fdrop||!e.dataTransfer||!e.dataTransfer.files.length)return;e.preventDefault();
  uploadFiles([...e.dataTransfer.files],{parent:z.dataset.fdrop},z.dataset.fdrop);});

async function boot(){
  let fileStart=null;
  if(window.C22AppWorkspaceDrag)C22AppWorkspaceDrag.attach(DRAG);
  if(window.UnderDocBlockDrag)UnderDocBlockDrag.attach(BDRAG);
  let st=null;
  /* /dev/ui shows only the made-up content of the mockup: every component in use, nothing from the server. */
  if(!document.getElementById('root').hasAttribute('data-demo'))try{const r=await fetch('/api/state',{credentials:'same-origin',headers:{Accept:'application/json'}});if(r.ok)st=await r.json();}catch(e){}
  if(st){
    S.user=st.user;S.modules=st.modules||[];S.real=true;if(window.UDRights)UDRights.setup(render,S.user);S.timeOn=S.modules.some(m=>m.id==='time'&&m.on&&!m.hidden);if(S.timeOn)timePseudos();S.canCreate=st.canCreate;S.files=!!st.files;loadTemplates();if(st.canCreate)loadResources();
    const tops=st.collections.map(addNode);
    tops.slice().reverse().forEach(id=>{C.unshift(asTop({id,i:D[id].i}));S.fav[id]=true;});
    if(!st.user.admin){PROFADM.length=0;}
    if(st.files){FS.pending=true;loadRoots().finally(()=>{FS.pending=false;render();});const m=location.pathname.match(/^\/o\/([0-9a-z]{10})$/);if(m&&!SID[m[1]])fileStart=m[1];}
    /* What a save over SMB could not bring in, said when the workspace opens; the profile lists it (ADR-30). */
    if((st.shareRefused||[]).length){const x=st.shareRefused[0],more=st.shareRefused.length-1;
      setTimeout(()=>{S.toast=`Not saved from the share: ${x.library}/${x.path}, ${x.why}${more?`, and ${more} more`:''}. It stays there without a version; see Profile.`;render();setTimeout(()=>{S.toast='';render();},9000);},0);}
  }
  const kept=loadSettings(st&&st.settings);
  if(kept)applyN2(kept.n2);
  if(kept){Object.keys(kept.fav||{}).forEach(k=>{if(D[k])S.fav[k]=kept.fav[k];});Object.keys(kept.pin||{}).forEach(k=>{if(D[k]){S.pin[k]=kept.pin[k];if(kept.pin[k])S.open[k]=true;}});}
  if(window.udTheme)S.theme=window.udTheme();
  rebuildPlaces();
  if(S.into!=='qn'&&!C.some(c=>c.id===S.into&&!isDeco(c.id)))S.into='qn';
  const start=idFromPath();
  const first=C.find(c=>D[c.id].real);
  if(start&&D[start]){openDoc(start);const top=COLL[start]?COLL[start].id:start;if(C.some(c=>c.id===top))S.open[top]=true;}
  else if(first){S.open[first.id]=true;const home=D[first.id].kids.find(k=>D[k].t==='page');S.wins=[{tabs:[newTab(home||first.id,true)],a:0,hist:[home||first.id]}];if(home)loadDoc(home);}
  S.booted=true;render();
  /* An address of a file opens the view Files with the file in the window. */
  /* With an anchor <sheet>!<range>, as a hit of the search in a spreadsheet carries it, the table view opens there (ADR-31). */
  if(fileStart)fget('/api/files/'+fileStart).then(f=>{S.view='files';S.rail=true;if(location.hash&&window.UDSheets)UDSheets.goto(f.id,decodeURIComponent(location.hash.slice(1)));openFile(f.id);}).catch(()=>{});
}
/* A link inside a page to another page opens it in the window, as a click in the list does; Ctrl or the middle button
   leave it to the browser. */
document.addEventListener('click',e=>{const l=e.target.closest&&e.target.closest('article.typeset a[href^="/o/"]');if(!l||e.ctrlKey||e.metaKey||e.shiftKey||e.button!==0)return;
  const id=SID[l.getAttribute('href').slice(3).split('#')[0].split('/').map(decodeURIComponent).join('/')];if(!id)return;e.preventDefault();e.stopPropagation();openDoc(id);render();},true);
/* A long press on a touch screen is the right click (ADR-17): a finger held still for half a second opens the same menu
   as the right mouse button, so what only the right click offers (Trash, Problems, Delete …) is in reach on a phone too.
   Where the browser opens a menu of its own on a long press, its event comes first and this one stays away, or it comes
   later and is dropped; the finger lifting after the menu opened clicks nothing (M-28, phone width). */
const PRESS={ms:500,slack:10,timer:0,at:null,done:false};
function longPress(){
  const stop=()=>{clearTimeout(PRESS.timer);PRESS.at=null;};
  document.addEventListener('pointerdown',e=>{PRESS.done=false;stop();if(e.pointerType!=='touch'||!e.isPrimary)return;PRESS.at={x:e.clientX,y:e.clientY,el:e.target};
    PRESS.timer=setTimeout(()=>{const p=PRESS.at;PRESS.at=null;if(!p||!p.el.isConnected)return;PRESS.done=true;
      p.el.dispatchEvent(new MouseEvent('contextmenu',{bubbles:true,cancelable:true,clientX:p.x,clientY:p.y,button:2,buttons:2}));},PRESS.ms);},true);
  document.addEventListener('pointermove',e=>{if(e.pointerType==='touch'&&PRESS.at&&Math.hypot(e.clientX-PRESS.at.x,e.clientY-PRESS.at.y)>PRESS.slack)stop();},true);
  ['pointerup','pointercancel'].forEach(k=>document.addEventListener(k,e=>{if(e.pointerType==='touch')stop();},true));
  /* On the window, so it comes before the listeners of the document. */
  addEventListener('contextmenu',e=>{if(!e.isTrusted)return;if(PRESS.done){e.preventDefault();e.stopImmediatePropagation();return;}stop();},true);
  addEventListener('click',e=>{if(!PRESS.done)return;PRESS.done=false;e.preventDefault();e.stopImmediatePropagation();},true);
}
longPress();
window.addEventListener('popstate',()=>{const id=idFromPath();if(id&&D[id]){const w=S.wins[S.focus];const k=w.tabs.findIndex(t=>t.d===id);if(k<0){w.tabs[0]=newTab(id,true);w.a=0;}else w.a=k;if(D[id].real&&!D[id].loaded)loadDoc(id);render();}});
boot();

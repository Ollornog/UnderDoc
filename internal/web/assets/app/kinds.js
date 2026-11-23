// SPDX-License-Identifier: MIT

// Page kinds and their writing rules (T-20, ADR-33, docs/page-kinds.md). The kind of a page sets who it is written for;
// the server checks every write against the rules of the kind and answers warnings with how to fix them. This part draws
// the kind and the warnings in the head of a page, the choice of the kind in its settings, and the writing rules in the
// list of problems. It uses the state and helpers of demo.js (D, render, toast, esc, prow, hint), which loads after it
// and calls it while drawing. Every word it shows stands in T, so a translation (T-32) finds them in one place.
window.UDKinds = (() => {
  'use strict';

  const T = {
    kind: 'Kind',
    noKind: 'No kind',
    names: {
      overview: 'Overview',
      register: 'Register',
      record: 'Record',
      plan: 'Plan',
      source: 'Source',
      reference: 'Reference',
      'agent-notes': 'Agent notes',
      'how-to': 'How-to',
      explanation: 'Explanation',
      tutorial: 'Tutorial',
    },
    groups: [
      ['Structure and records', ['overview', 'register', 'record', 'plan', 'source']],
      ['Agent documentation and reference', ['reference', 'agent-notes']],
      ['Human documentation', ['how-to', 'explanation', 'tutorial']],
    ],
    sides: { structure: 'Structure', agents: 'For agents', people: 'For people' },
    kindHint: 'The kind sets who a page is written for and the rules it is checked against. A page without a kind counts as human documentation.',
    hints: (n) => (n === 1 ? '1 hint on writing' : `${n} hints on writing`),
    errorWord: 'Error',
    warningWord: 'Warning',
    block: (id) => `Block #${id}`,
    saveFailed: 'The kind could not be saved.',
    notAllowed: 'Only who may change this page sets its kind.',
    problems: 'Writing rules',
    problemsHint: 'Pages and folders that break the rules of their kind. A warning blocks nothing; it says how to fix it.',
    page: 'Page',
    folder: 'Folder',
    rule: 'Rule',
    what: 'What, and how to fix it',
  };

  const name = (kind) => (kind ? T.names[kind] || kind : T.noKind);
  const findings = (d) => (Array.isArray(d.findings) ? d.findings : []);

  /** marks are the kind of a page and its side, as badges under its title. */
  function marks(d) {
    if (!d.blocks || d.t === 'notes' || !d.db) return '';
    const side = T.sides[d.audience || 'people'];
    return `<span class="badge" data-variant="outline" data-kind="${esc(d.kind || '')}" title="${esc(T.kindHint)}">${esc(name(d.kind))}</span><span class="badge" data-variant="secondary" data-audience="${esc(d.audience || 'people')}">${esc(side)}</span>`;
  }

  /** findingsHtml lists what the page breaks, for whoever may change it: folded, with the fix of each. */
  function findingsHtml(d) {
    const list = findings(d);
    if (!d.canEdit || !list.length) return '';
    const item = (f) =>
      `<li data-finding="${esc(f.code)}"${f.block ? ` data-block-ref="${esc(f.block)}"` : ''}><span class="badge" data-variant="${f.level === 'error' ? 'destructive' : 'outline'}">${esc(f.level === 'error' ? T.errorWord : T.warningWord)} ${esc(f.code)}</span> ${f.block ? `<small class="text-muted-foreground">${esc(T.block(f.block))}</small> ` : ''}${esc(f.message)} <span class="text-muted-foreground">${esc(f.fix)}</span></li>`;
    return `<div class="alert" data-findings><section><details><summary>${esc(T.hints(list.length))}</summary><ul>${list.map(item).join('')}</ul></details></section></div>`;
  }

  /** head is what stands under the title of a page: its kind and side. */
  function head(d) {
    const m = marks(d);
    return m ? `<div class="flex flex-wrap items-center gap-2" data-kind-head>${m}</div>` : '';
  }

  /** kindRow is the choice of the kind in the settings of a page of the database. */
  function kindRow(key, d, mayChange) {
    if (!d.db || !d.blocks || d.t === 'notes') return '';
    const opt = (k) => `<option value="${esc(k)}"${(d.kind || '') === k ? ' selected' : ''}>${esc(name(k))}</option>`;
    const groups = T.groups.map(([label, kinds]) => `<optgroup label="${esc(label)}">${kinds.map(opt).join('')}</optgroup>`).join('');
    const off = mayChange ? '' : ` disabled title="${esc(T.notAllowed)}"`;
    return `${prow(T.kind, `<select class="select" data-size="sm" data-setkind="${esc(key)}" aria-label="${esc(T.kind)}"${off}>${opt('')}${groups}</select>`)}${hint(esc(T.kindHint))}`;
  }

  async function saveKind(key, kind) {
    const d = D[key];
    if (!d) return;
    try {
      const r = await fetch(`/api/pages/${encodeURIComponent(d.sid)}/kind`, {
        method: 'PUT',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ kind }),
      });
      if (!r.ok) throw new Error(String(r.status));
      const a = await r.json();
      d.kind = a.kind;
      d.audience = a.audience;
      d.findings = a.findings;
    } catch (e) {
      toast(e.message === '403' ? T.notAllowed : T.saveFailed);
    }
    render();
  }

  document.addEventListener('change', (e) => {
    const x = e.target;
    if (x && x.dataset && x.dataset.setkind !== undefined) saveKind(x.dataset.setkind, x.value);
  });

  /** problemsHtml is the part of the list of problems with the writing rules. where draws the link to a page. */
  function problemsHtml(rules, where) {
    if (!rules || !rules.length) return '';
    const rows = rules
      .flatMap((p) =>
        p.findings.map(
          (f) =>
            `<tr data-rule="${esc(p.page)}:${esc(f.code)}"><td>${p.kind === 'folder' ? `${esc(T.folder)} ${esc(p.title)}` : where(p)}</td><td><code>${esc(f.code)}</code>${f.block ? ` <small>#${esc(f.block)}</small>` : ''}</td><td>${esc(f.message)} <span class="text-muted-foreground">${esc(f.fix)}</span></td></tr>`,
        ),
      )
      .join('');
    return `<h2>${esc(T.problems)}</h2>${hint(esc(T.problemsHint))}<div class="table-container"><table class="table"><tr><th>${esc(T.page)}</th><th>${esc(T.rule)}</th><th>${esc(T.what)}</th></tr>${rows}</table></div>`;
  }

  return { T, name, head, findingsHtml, kindRow, problemsHtml };
})();

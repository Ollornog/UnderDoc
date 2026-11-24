// SPDX-License-Identifier: MIT

// Page kinds and their writing rules (T-20, ADR-33, docs/page-kinds.md). The kind of a page sets who it is written for;
// the server checks every write against the rules of the kind and answers warnings with how to fix them. This part draws
// the kind and the warnings in the head of a page, the choice of the kind in its settings, and the writing rules in the
// list of problems. On human documentation it shows the hint "written by AI, not checked" on every block an agent changed
// and nobody checked, and whoever may change the page opens the change as a diff and checks it or takes it back, one at a
// time. It uses the state and helpers of demo.js (D, S, render, toast, esc, prow, hint, loadDoc), which loads after it and
// calls it while drawing. Every word it shows stands in T, so a translation (T-32) finds them in one place.
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
    rules: 'Rules page',
    rulesOn: 'The rules page for agents',
    rulesMake: 'Make it the rules page for agents',
    rulesOff: 'No longer the rules page',
    rulesHint: 'Agents read the rules page before they write, and change nothing of it. Administrators name it; there is one.',
    rulesFailed: 'Only an administrator names the rules page, in the browser, and never a private page or one in an archived space.',
    rulesFromTag: 'Taken over from the tag rules: please confirm it',
    rulesConfirm: 'Confirm as the rules page',
    notAllowed: 'Only who may change this page sets its kind.',
    problems: 'Writing rules',
    problemsHint: 'Pages and folders that break the rules of their kind. A warning blocks nothing; it says how to fix it.',
    page: 'Page',
    folder: 'Folder',
    rule: 'Rule',
    what: 'What, and how to fix it',
    aiHint: 'Written by AI, not checked',
    aiAdded: 'Added by AI, not checked',
    aiWider: 'after reading what fewer may read: check who reads it here',
    aiRemoved: 'Removed by AI, not checked',
    aiMoved: (to) => `Moved by AI to ${to}, not checked`,
    aiBy: (by, at) => `For ${by}, ${at}`,
    review: 'Review',
    maintained: 'Maintained by AI',
    maintainedHint: 'Agent documentation: agents keep it up to date. People may edit it too.',
    checkedBy: (by, at) => `Checked by ${by} on ${at}`,
    reviewTitle: 'Change by AI',
    reviewIntro: 'What an agent changed in this block since a person last saw it. Check it if it is right, or take it back.',
    before: 'Before',
    now: 'Now',
    nothingBefore: 'The block is new.',
    nothingNow: 'The block is gone.',
    check: 'Checked',
    takeBack: 'Take back',
    cancel: 'Cancel',
    loading: 'Loading…',
    reviewFailed: 'The change could not be opened.',
    stale: 'The block changed meanwhile. Open it again to see what changed since.',
    movedAway: 'An agent moved this block to another page. Move it back from there.',
    reviewNoRight: 'Only who may change this page checks changes.',
    inUse: 'Taking it back would take away something other pages embed. Change the block in the editor, where you choose what happens to it.',
    doneFailed: 'This did not work.',
  };

  const name = (kind) => (kind ? T.names[kind] || kind : T.noKind);
  const findings = (d) => (Array.isArray(d.findings) ? d.findings : []);

  const day = (iso) => (iso ? new Date(iso).toLocaleDateString() : '');
  const moment = (iso) => (iso ? new Date(iso).toLocaleString() : '');

  /** marks are the kind of a page and its side, as badges under its title, once "maintained by AI" on the agent side,
   * and who checked the page last. */
  function marks(d) {
    if (!d.blocks || d.t === 'notes' || !d.db) return '';
    const side = T.sides[d.audience || 'people'];
    const ai = d.maintainedByAI ? `<span class="badge" data-variant="secondary" data-maintained title="${esc(T.maintainedHint)}">✦ ${esc(T.maintained)}</span>` : '';
    const checked = d.checked && !d.maintainedByAI ? `<small class="text-muted-foreground" data-checked>${esc(T.checkedBy(d.checked.by, day(d.checked.at)))}</small>` : '';
    return `<span class="badge" data-variant="outline" data-kind="${esc(d.kind || '')}" title="${esc(T.kindHint)}">${esc(name(d.kind))}</span><span class="badge" data-variant="secondary" data-audience="${esc(d.audience || 'people')}">${esc(side)}</span>${ai}${checked}`;
  }

  const reviewButton = (key, block, version) =>
    `<button type="button" class="btn" data-variant="outline" data-size="xs" data-ud-review="${esc(key)}" data-block-id="${esc(block)}" data-version="${esc(String(version))}">${esc(T.review)}</button>`;
  const mayReview = (d) => d.canEdit && !(S.user && S.user.agent);

  /** blockHint is the hint of a block an agent changed and nobody checked, under its text; whoever may change the page
   * opens it from there. */
  function blockHint(d, b, key) {
    if (!b.ai) return '';
    const a = b.ai;
    return `<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground" data-ai-hint="${esc(b.id)}"><span class="badge" data-variant="outline">✦ ${esc(a.added ? T.aiAdded : T.aiHint)}</span>${a.wider ? `<span class="badge" data-variant="outline" data-ai-wider>⚠ ${esc(T.aiWider)}</span>` : ''}<span>${esc(T.aiBy(a.by, moment(a.at)))}</span>${mayReview(d) ? reviewButton(key, b.id, a.version) : ''}</div>`;
  }

  /** removedHtml lists the blocks agents took out of human documentation that nobody checked, under the page. */
  function removedHtml(d, key) {
    const list = Array.isArray(d.removed) ? d.removed : [];
    if (!list.length) return '';
    return list
      .map(
        (r) =>
          `<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground" data-ai-removed="${esc(r.block)}"><span class="badge" data-variant="outline">✦ ${esc(r.moved ? T.aiMoved(r.moved) : T.aiRemoved)}</span><span>#${esc(r.block)} · ${esc(T.aiBy(r.by, moment(r.at)))}</span>${mayReview(d) ? reviewButton(key, r.block, r.version) : ''}</div>`,
      )
      .join('');
  }

  // --- The diff of a change, in a dialog of its own: Checked or Take back, one change at a time (ADR-33).

  /** diffLines compares two texts line by line (the longest common subsequence) and marks what went and what came. */
  function diffLines(before, now) {
    const a = before ? before.split('\n') : [];
    const b = now ? now.split('\n') : [];
    const n = a.length;
    const m = b.length;
    const lcs = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0));
    for (let i = n - 1; i >= 0; i--) for (let j = m - 1; j >= 0; j--) lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    const out = [];
    let i = 0;
    let j = 0;
    while (i < n || j < m) {
      if (i < n && j < m && a[i] === b[j]) {
        out.push(['same', a[i]]);
        i++;
        j++;
      } else if (i < n && (j >= m || lcs[i + 1][j] >= lcs[i][j + 1])) {
        out.push(['went', a[i]]);
        i++;
      } else {
        out.push(['came', b[j]]);
        j++;
      }
    }
    return out;
  }

  function diffHtml(c) {
    const lines = diffLines(c.before, c.now);
    const line = ([kind, text]) =>
      kind === 'went'
        ? `<del data-diff="went">− ${esc(text)}</del>`
        : kind === 'came'
          ? `<ins data-diff="came">+ ${esc(text)}</ins>`
          : `<span data-diff="same">  ${esc(text)}</span>`;
    const note = !c.before ? `<p class="text-xs">${esc(T.nothingBefore)}</p>` : !c.now ? `<p class="text-xs">${esc(c.moved ? T.aiMoved(c.moved) : T.nothingNow)}</p>` : '';
    return `${note}<pre class="code-block" data-diff-view><code>${lines.map(line).join('\n')}</code></pre>`;
  }

  const rv = { key: '', block: '', change: null, error: '' };

  function reviewEl() {
    let el = document.getElementById('udreview');
    if (!el) {
      el = document.createElement('dialog');
      el.id = 'udreview';
      el.className = 'dialog';
      el.setAttribute('aria-labelledby', 'udreview-title');
      document.body.appendChild(el);
    }
    return el;
  }

  function drawReview() {
    const c = rv.change;
    const body = rv.error ? `<p class="text-sm" role="alert">${esc(rv.error)}</p>` : c ? `<p class="text-xs text-muted-foreground">#${esc(c.block)} · ${esc(T.aiBy(c.by, moment(c.at)))}</p>${diffHtml(c)}` : `<p class="text-sm text-muted-foreground">${esc(T.loading)}</p>`;
    const acts = c && !rv.error ? `<button class="btn" data-variant="outline" type="button" data-review-act="takeback">${esc(T.takeBack)}</button><button class="btn" type="button" data-review-act="check">${esc(T.check)}</button>` : '';
    reviewEl().innerHTML = `<div><header><h2 id="udreview-title">${esc(T.reviewTitle)}</h2><p>${esc(T.reviewIntro)}</p></header><section>${body}</section><footer><button class="btn" data-variant="outline" type="button" data-review-act="cancel">${esc(T.cancel)}</button>${acts}</footer></div>`;
  }

  const pageApi = (key) => `/api/pages/${encodeURIComponent(D[key].sid)}/blocks/${encodeURIComponent(rv.block)}`;

  async function openReview(key, block) {
    Object.assign(rv, { key, block, change: null, error: '' });
    drawReview();
    const el = reviewEl();
    if (!el.open) el.showModal();
    try {
      const r = await fetch(`${pageApi(key)}/change`, { credentials: 'same-origin', headers: { Accept: 'application/json' } });
      if (!r.ok) throw new Error(String(r.status));
      rv.change = await r.json();
    } catch (e) {
      rv.error = e.message === '403' ? T.reviewNoRight : T.reviewFailed;
    }
    drawReview();
  }

  async function act(kind) {
    const el = reviewEl();
    if (kind === 'cancel' || !rv.change) {
      el.close();
      return;
    }
    try {
      const r = await fetch(`${pageApi(rv.key)}/${kind === 'check' ? 'check' : 'takeback'}`, {
        method: 'POST',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ version: rv.change.version }),
      });
      if (!r.ok) {
        let answer = {};
        try {
          answer = (await r.json()) || {};
        } catch (x) {
          answer = {};
        }
        throw Object.assign(new Error(String(r.status)), { detail: answer.error || '', inUse: !!answer.inUse });
      }
      el.close();
    } catch (e) {
      rv.error = e.inUse
        ? T.inUse
        : e.message === '409'
          ? e.detail && e.detail.startsWith('an agent moved')
            ? T.movedAway
            : T.stale
          : e.message === '403'
            ? T.reviewNoRight
            : e.detail || T.doneFailed;
      drawReview();
      return;
    }
    const d = D[rv.key];
    d.loaded = false;
    await loadDoc(rv.key);
  }

  document.addEventListener('click', (e) => {
    const open = e.target.closest && e.target.closest('[data-ud-review]');
    if (open) {
      openReview(open.dataset.udReview, open.dataset.blockId);
      return;
    }
    const b = e.target.closest && e.target.closest('#udreview [data-review-act]');
    if (b) act(b.dataset.reviewAct);
  });

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
    return `${prow(T.kind, `<select class="select" data-size="sm" data-setkind="${esc(key)}" aria-label="${esc(T.kind)}"${off}>${opt('')}${groups}</select>`)}${hint(esc(T.kindHint))}${rulesRow(key, d)}`;
  }

  /** rulesRow names the rules page for agents (B-5): administrators, on a page of agent notes or the rules page itself. */
  function rulesRow(key, d) {
    if (!S.user || !S.user.admin || (d.kind !== 'agent-notes' && !d.rulesPage)) return '';
    const label = d.rulesPage ? T.rulesOff : T.rulesMake;
    const state = d.rulesPage ? `<span class="badge" data-variant="secondary" data-rules-page>✅ ${esc(T.rulesOn)}</span> ` : '';
    // A page taken over from the tag rules (migration 038) waits for an administrator to confirm it (B-6).
    const confirm = d.rulesFromTag
      ? `<span class="badge" data-variant="outline" data-rules-from-tag>⚠ ${esc(T.rulesFromTag)}</span> <button type="button" class="btn" data-variant="outline" data-size="xs" data-confirmrules="${esc(key)}">${esc(T.rulesConfirm)}</button> `
      : '';
    return `${prow(T.rules, `${state}${confirm}<button type="button" class="btn" data-variant="outline" data-size="xs" data-setrules="${esc(key)}" aria-pressed="${!!d.rulesPage}">${esc(label)}</button>`)}${hint(esc(T.rulesHint))}`;
  }

  async function saveRules(key, confirm) {
    const d = D[key];
    if (!d) return;
    // Confirming names the same page again; otherwise the button switches it.
    const was = confirm ? false : !!d.rulesPage;
    try {
      const r = await fetch('/api/rules-page', {
        method: 'PUT',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ page: was ? '' : d.sid }),
      });
      if (!r.ok) throw new Error(String(r.status));
      Object.keys(D).forEach((k) => {
        if (D[k] && D[k].rulesPage) D[k].rulesPage = false;
      });
      d.rulesPage = !was;
      d.rulesFromTag = false;
    } catch (e) {
      toast(T.rulesFailed);
    }
    render();
  }

  document.addEventListener('click', (e) => {
    const b = e.target && e.target.closest && e.target.closest('[data-setrules]');
    if (b) saveRules(b.dataset.setrules);
    const c = e.target && e.target.closest && e.target.closest('[data-confirmrules]');
    if (c) saveRules(c.dataset.confirmrules, true);
  });

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

  return { T, name, head, findingsHtml, kindRow, problemsHtml, blockHint, removedHtml, diffLines };
})();

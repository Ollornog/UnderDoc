// SPDX-License-Identifier: MIT

// The web editor of UnderDoc (ADR-21): one CodeMirror 6 editor per page with a live preview after Typora and Obsidian.
// The text stays Markdown; away from the cursor it looks rendered, on the lines the selection touches the Markdown shows.
// The page is its blocks joined with a marker line before each (blocks.js); the editor hides the markers and shows the
// handle of the block there. Saving compares the text with the stored blocks and calls the block API (save.js).
//
// The workspace (demo.js) creates an editor with window.UnderDocEditor.create(options) and draws the bar above it and
// the handles through callbacks; the editor owns the text, the preview, the slash menu and saving.
import { EditorState, EditorSelection, StateEffect, StateField, Transaction, Annotation, MapMode, Prec } from '@codemirror/state';
import { Decoration, EditorView, WidgetType, keymap, drawSelection, showTooltip } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { markdownLanguage, markdownKeymap } from '@codemirror/lang-markdown';
import { LanguageSupport, syntaxTree } from '@codemirror/language';
import { MARKER, markerLine, normalize, joinBlocks, plan, copiedInto, ORIGIN, originOf, idle, kindOf, turnInto, embedFor, inlineEmbedFor, EMBED, PART_MARKER, partsIn, stripParts, imageLine } from './blocks.js';
import { run, Conflict } from './save.js';
import { findPlain } from './plain.js';
import * as tables from './tables.js';
import './editor.css';

/** How long typing has to rest before the page is saved. */
const AUTOSAVE_MS = 1000;

// ---- Changes the editor makes itself: not undone by Ctrl+Z, not saved again, and markers they write are the editor's.
const system = Annotation.define();

// ---- Focus: away from the editor everything looks rendered, the cursor line included.
const setFocus = StateEffect.define();
const focused = StateField.define({
  create: () => false,
  update(value, tr) {
    for (const effect of tr.effects) if (effect.is(setFocus)) value = effect.value;
    return value;
  },
});
const trackFocus = EditorView.focusChangeEffect.of((_, focusing) => setFocus.of(focusing));
// A press into an editor that has the focus without having seen it come (the workspace drew anew around it) tells it so.
const resyncFocus = EditorView.domEventHandlers({
  pointerup(_event, view) {
    setTimeout(() => {
      if (view.hasFocus && !view.state.field(focused)) view.dispatch({ effects: setFocus.of(true) });
    }, 0);
    return false;
  },
});

// ---- The markers of the blocks. Each is a line `<!-- block bN -->` the editor keeps track of: written at the start,
// written by saving, or typed, pasted or brought back by undo as a whole line. A marker whose line changed is gone.
// lost keeps where a marker was last, so a block used elsewhere can be asked about there and put back.
const refresh = StateEffect.define();
const setHover = StateEffect.define();
const setAsks = StateEffect.define();
// The values that look like passwords typed as text and wait for an answer before anything is saved (ADR-7, plain.js).
const setPlain = StateEffect.define();

function scanLines(doc, from, to, adopted, out) {
  for (let pos = doc.lineAt(from).from; pos <= Math.min(to, doc.length); ) {
    const line = doc.lineAt(pos);
    const match = MARKER.exec(line.text);
    if (match && !out.some((m) => m.from === line.from)) out.push({ id: match[1], from: line.from, adopted });
    pos = line.to + 1;
  }
}

const markers = StateField.define({
  create: () => ({ list: [], lost: new Map() }),
  update(value, tr) {
    if (!tr.docChanged) return value;
    const doc = tr.state.doc;
    const list = [];
    const lost = new Map();
    for (const [id, pos] of value.lost) lost.set(id, tr.changes.mapPos(pos, -1));
    for (const m of value.list) {
      const pos = tr.changes.mapPos(m.from, 1, MapMode.TrackDel);
      const line = pos === null ? null : doc.lineAt(pos);
      const match = line && line.from === pos && MARKER.exec(line.text);
      if (match && match[1] === m.id) list.push({ ...m, from: pos });
      else lost.set(m.id, tr.changes.mapPos(m.from, -1));
    }
    const own = !!tr.annotation(system);
    tr.changes.iterChangedRanges((_fa, _ta, from, to) => scanLines(doc, from, to, !own, list), true);
    list.sort((a, b) => a.from - b.from);
    for (const m of list) lost.delete(m.id);
    return { list, lost };
  },
});

/** segments cuts the text at the markers, for the plan of blocks.js. */
function segments(state) {
  const doc = state.doc;
  const list = state.field(markers).list;
  const out = [];
  const firstFrom = list.length ? list[0].from : doc.length;
  if (doc.sliceString(0, firstFrom).trim()) out.push({ id: null, line: null, body: [0, firstFrom], text: doc.sliceString(0, firstFrom) });
  list.forEach((m, k) => {
    const line = doc.lineAt(m.from);
    const from = Math.min(line.to + 1, doc.length);
    const to = k + 1 < list.length ? list[k + 1].from : doc.length;
    out.push({ id: m.id, adopted: m.adopted, line: [line.from, line.to], body: [from, Math.max(from, to)], text: doc.sliceString(from, Math.max(from, to)) });
  });
  return out;
}

/** blockAt names the block the position lies in, or null before the first marker. */
function blockAt(state, pos) {
  let id = null;
  for (const m of state.field(markers).list) {
    if (m.from > pos) break;
    id = m.id;
  }
  return id;
}

const markerLineAt = (state, pos) => {
  const line = state.doc.lineAt(pos);
  return state.field(markers).list.some((m) => m.from === line.from) ? line : null;
};

// ---- What the workspace hands in: the handle of a block, the kinds of the slash menu, what an embedding names.
const host = StateField.define({ create: () => ({}), update: (v) => v });

// ---- Widgets

class MarkerWidget extends WidgetType {
  constructor(id, html, active) {
    super();
    this.id = id;
    this.html = html;
    this.active = active;
  }
  eq(other) {
    return other.id === this.id && other.html === this.html && other.active === this.active;
  }
  toDOM() {
    // The marker is the block's wrapper in C22's sense (div[data-block] with its .editor-handle), only without the lines,
    // which CodeMirror draws as siblings after it; data-active names the block with the cursor or the pointer, whose
    // handle C22 then shows.
    const box = document.createElement('div');
    box.className = 'cm-ud-marker';
    box.dataset.block = this.id;
    box.dataset.edb = this.id;
    if (this.active) box.dataset.active = '';
    box.innerHTML = this.html;
    return box;
  }
  get estimatedHeight() {
    return 0;
  }
  ignoreEvent() {
    return true;
  }
}

// The label of a block used elsewhere, at the end of its first line while the cursor is in it (ADR-22): the small of C22's
// .editor [data-used].
class UsedWidget extends WidgetType {
  constructor(count, where) {
    super();
    this.count = count;
    this.where = where;
  }
  eq(other) {
    return other.count === this.count && other.where === this.where;
  }
  toDOM() {
    const label = document.createElement('small');
    label.textContent = `Used in ${this.count} ${this.count === 1 ? 'place' : 'places'}`;
    label.title = `Changes here show there too: ${this.where}`;
    return label;
  }
  ignoreEvent() {
    return true;
  }
}

/** partMarkers lists the markers of parts in a text, outside tables and embedding lines, as [from, to]. */
function partMarkers(doc) {
  const out = [];
  const text = doc.toString();
  for (const m of text.matchAll(PART_MARKER)) {
    const line = doc.lineAt(m.index);
    if (/^\s*\|/.test(line.text) || MARKER.test(line.text)) continue;
    out.push([m.index, m.index + m[0].length]);
  }
  return out;
}

/** partAt finds the part the cursor stands in inside its block, the innermost: {id, name, from, to} in the document. */
function partAt(state, pos, block) {
  const seg = segments(state).find((s) => s.id === block);
  if (!seg) return null;
  let best = null;
  for (const p of partsIn(seg.text)) {
    const from = seg.body[0] + p.open[1];
    const to = seg.body[0] + p.close[0];
    if (pos >= from && pos <= to && (!best || to - from < best.to - best.from)) best = { id: p.id, name: p.name, from, to };
  }
  return best;
}

class AskWidget extends WidgetType {
  constructor(id, used) {
    super();
    this.id = id;
    this.used = used;
  }
  eq(other) {
    return other.id === this.id && other.used === this.used;
  }
  toDOM() {
    const box = question(`Block #${this.id} is used in ${this.used}. Delete it?`, [
      ['answer', 'delete', 'Choose what happens there', 'destructive'],
      ['answer', 'keep', 'Keep the block', 'outline'],
    ]);
    box.dataset.ask = this.id;
    return box;
  }
  ignoreEvent() {
    return true;
  }
}

// question is a question in place, below a line: C22's .item (muted, small) with the words in its section and the answers as
// buttons in its aside. Each answer is [data key, value, words, variant].
function question(words, answers) {
  const box = document.createElement('article');
  box.className = 'item';
  box.dataset.variant = 'muted';
  box.dataset.size = 'sm';
  box.setAttribute('role', 'alert');
  const section = document.createElement('section');
  const text = document.createElement('p');
  text.className = 'text-sm';
  text.textContent = words;
  section.append(text);
  const aside = document.createElement('aside');
  for (const [key, value, label, variant] of answers) {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'btn';
    b.dataset.variant = variant;
    b.dataset.size = 'xs';
    b.dataset[key] = value;
    b.textContent = label;
    aside.append(b);
  }
  box.append(section, aside);
  return box;
}

// A value that looks like a password typed as text (ADR-7, 2026-11-20): asked about below its line before the page is
// saved. Keep it as a password makes a secret of the page from it; It is not a password saves the text as typed.
class PlainWidget extends WidgetType {
  constructor(k) {
    super();
    this.k = k;
  }
  eq(other) {
    return other.k === this.k;
  }
  toDOM() {
    const box = question('This looks like a password, typed as text. Nothing of the page is saved until you choose.', [
      ['plain', 'secret', 'Keep it as a password', 'outline'],
      ['plain', 'text', 'It is not a password', 'outline'],
    ]);
    box.dataset.plainAsk = String(this.k);
    return box;
  }
  ignoreEvent() {
    return true;
  }
}

// The place where a dragged block would land: C22's .dropgap with the block in it as it looks (blockdrag.js in the
// workspace moves the pointer; the editor shows the gap and hides the block while it is held).
class GapWidget extends WidgetType {
  constructor(drag) {
    super();
    this.drag = drag;
  }
  eq(other) {
    return other.drag.id === this.drag.id && other.drag.look === this.drag.look;
  }
  toDOM() {
    const gap = document.createElement('div');
    gap.className = 'dropgap held cm-ud-gap';
    gap.dataset.gap = this.drag.id;
    for (const line of this.drag.look) gap.append(line.cloneNode(true));
    return gap;
  }
  get estimatedHeight() {
    // C22 caps the gap at its first lines (6.5em).
    return Math.min(this.drag.height, 104);
  }
  ignoreEvent() {
    return true;
  }
}

class BulletWidget extends WidgetType {
  eq() {
    return true;
  }
  toDOM() {
    const span = document.createElement('span');
    span.dataset.md = 'bullet';
    span.textContent = '•';
    return span;
  }
}

class CheckboxWidget extends WidgetType {
  constructor(checked) {
    super();
    this.checked = checked;
  }
  eq(other) {
    return other.checked === this.checked;
  }
  toDOM(view) {
    const box = document.createElement('input');
    box.type = 'checkbox';
    box.dataset.md = 'task';
    box.checked = this.checked;
    box.setAttribute('aria-label', this.checked ? 'Done' : 'To do');
    box.addEventListener('mousedown', (event) => {
      event.preventDefault();
      const pos = view.posAtDOM(box);
      if (/^\[[ xX]\]$/.test(view.state.sliceDoc(pos, pos + 3))) {
        view.dispatch({ changes: { from: pos + 1, to: pos + 2, insert: this.checked ? ' ' : 'x' }, userEvent: 'input' });
      }
    });
    return box;
  }
  ignoreEvent() {
    return true;
  }
}

// Only pictures of this application and data addresses show as pictures; the CSP would block any other, so its alt text
// shows instead, never a request to a third party.
const ownPicture = (src) => /^\/(?!\/)/.test(src) || /^data:image\//.test(src);

class ImageWidget extends WidgetType {
  constructor(src, alt) {
    super();
    this.src = src;
    this.alt = alt;
  }
  eq(other) {
    return other.src === this.src && other.alt === this.alt;
  }
  toDOM() {
    if (!ownPicture(this.src)) {
      const span = document.createElement('span');
      span.dataset.md = 'image-alt';
      span.textContent = this.alt || this.src;
      return span;
    }
    const img = document.createElement('img');
    img.dataset.md = 'image';
    img.src = this.src;
    img.alt = this.alt;
    return img;
  }
}

class RuleWidget extends WidgetType {
  eq() {
    return true;
  }
  toDOM() {
    const hr = document.createElement('span');
    hr.dataset.md = 'rule';
    return hr;
  }
}

class EmbedWidget extends WidgetType {
  constructor(ref, label, href) {
    super();
    this.ref = ref;
    this.label = label;
    this.href = href;
  }
  eq(other) {
    return other.ref === this.ref && other.label === this.label && other.href === this.href;
  }
  toDOM() {
    // C22's badge (secondary) as a link: what it is, muted, then where it comes from.
    const box = document.createElement('a');
    box.className = 'badge';
    box.dataset.variant = 'secondary';
    box.dataset.ref = this.ref;
    box.href = this.href;
    const what = document.createElement('span');
    what.className = 'text-muted-foreground';
    what.textContent = 'Embeds';
    const name = document.createElement('span');
    name.className = 'underline';
    name.textContent = this.label;
    box.append(what, name);
    return box;
  }
  ignoreEvent(event) {
    return !!(event.target && event.target.closest && event.target.closest('a'));
  }
}

// A password in the text (ADR-7): ::secret{id="…"} anywhere in a line. Away from the cursor it shows as a mask with
// Reveal, Copy and Change; the value comes from the server through the workspace (secrets.js) and is masked again after a
// while. At the cursor the reference shows as text.
const SECRET = /::secret\{id="([0-9a-hjkmnp-tv-z]{10})"\}/g;
const SECRET_MASK = '••••••••';

class SecretWidget extends WidgetType {
  constructor(id, secret) {
    super();
    this.id = id;
    this.secret = secret;
  }
  eq(other) {
    return other.id === this.id && other.secret === this.secret;
  }
  toDOM() {
    const box = document.createElement('span');
    box.className = 'inline-flex items-center gap-1';
    box.dataset.edSecret = this.id;
    const mask = document.createElement('span');
    mask.className = 'secret-key';
    mask.textContent = SECRET_MASK;
    mask.setAttribute('aria-label', 'Password, hidden');
    box.append(mask);
    const secret = this.secret;
    if (!secret) return box;
    const button = (label, act) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'btn';
      b.dataset.variant = 'outline';
      b.dataset.size = 'xs';
      b.textContent = label;
      b.dataset.edSact = act;
      box.append(b);
      return b;
    };
    const fail = (err) => (window.UDToast || window.alert)(secret.why ? secret.why(err) : 'The password could not be fetched.');
    const show = button('Reveal', 'show');
    show.addEventListener('click', () => {
      if (mask.dataset.shown) {
        clearTimeout(mask.hideTimer);
        mask.textContent = SECRET_MASK;
        delete mask.dataset.shown;
        show.textContent = 'Reveal';
        return;
      }
      secret.show(this.id).then((value) => {
        mask.textContent = value;
        mask.dataset.shown = '1';
        show.textContent = 'Hide';
        clearTimeout(mask.hideTimer);
        mask.hideTimer = setTimeout(() => {
          mask.textContent = SECRET_MASK;
          delete mask.dataset.shown;
          show.textContent = 'Reveal';
        }, 30000);
      }, fail);
    });
    const copy = button('Copy', 'copy');
    copy.addEventListener('click', () => {
      secret.copy(this.id).then(() => {
        copy.textContent = 'Copied';
        setTimeout(() => (copy.textContent = 'Copy'), 2000);
      }, fail);
    });
    button('Change', 'change').addEventListener('click', () => secret.change(this.id));
    return box;
  }
  ignoreEvent(event) {
    return !!(event.target && event.target.closest && event.target.closest('button'));
  }
}

// cellsOf splits a table row at the pipes that are not escaped.
function cellsOf(line) {
  const cells = [];
  let cell = '';
  const text = line.trim().replace(/^\|/, '').replace(/\|$/, '');
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '\\' && text[i + 1] === '|') {
      cell += '|';
      i++;
    } else if (text[i] === '|') {
      cells.push(cell.trim());
      cell = '';
    } else {
      cell += text[i];
    }
  }
  cells.push(cell.trim());
  return cells;
}

// plain drops the inline markers, for the cells of a table shown as a table.
const plain = (text) => text.replace(/(\*\*|__|\*|_|`|~~)(.+?)\1/g, '$2').replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1');

class TableWidget extends WidgetType {
  constructor(source) {
    super();
    this.source = source;
  }
  eq(other) {
    return other.source === this.source;
  }
  toDOM(view) {
    const lines = this.source.split('\n');
    // A column aligned in Markdown is C22's th/td[align]; one without a colon keeps the table's own alignment.
    const align = cellsOf(lines[1] || '').map((d) => (/^:-+:$/.test(d) ? 'center' : /-:$/.test(d) ? 'right' : /^:-+$/.test(d) ? 'left' : ''));
    // C22's table in the box the page shows it in when read (markdown: classTableBox, table text-sm), frame and all, drawn in
    // place of its source; data-md says what it is.
    const box = document.createElement('div');
    box.className = 'table-container w-full rounded-md border';
    box.dataset.md = 'table';
    const table = document.createElement('table');
    table.className = 'table text-sm';
    box.append(table);
    lines.forEach((line, index) => {
      if (index === 1) return;
      const row = document.createElement('tr');
      cellsOf(line).forEach((text, column) => {
        const cell = document.createElement(index === 0 ? 'th' : 'td');
        if (align[column]) cell.setAttribute('align', align[column]);
        cell.textContent = plain(text);
        row.append(cell);
      });
      table.append(row);
    });
    // A finger on the drawn table puts the cursor into the cell it touched, as the mouse does: on a touch screen the
    // editor leaves the caret to the browser, which cannot put one inside a widget (2026-11-17).
    let touch = false;
    table.addEventListener('pointerdown', (e) => {
      touch = e.pointerType !== 'mouse';
    });
    table.addEventListener('click', (e) => {
      if (!touch) return;
      const cell = e.target.closest && e.target.closest('td, th');
      const row = cell ? cell.parentElement.rowIndex : 0;
      const col = cell ? cell.cellIndex : 0;
      view.dispatch({ selection: { anchor: view.posAtDOM(box) + tables.cellStart(this.source, row, col) } });
      view.focus();
    });
    return box;
  }
  ignoreEvent() {
    return false;
  }
}

// ---- The decorations

// The live preview says what a line, a run of text or a widget is by C22's data-md (the editor block of C22), never by a
// class of its own: C22 gives each its look, the pack its sizes. A line may be two things at once ("quote h2"); CodeMirror
// does not join attributes of two decorations on one line, so the words of a line are gathered and set once.
const hide = Decoration.replace({});
const lineClass = (cls) => Decoration.line({ class: cls });
const mark = (cls) => Decoration.mark({ class: cls });
const md = (word) => Decoration.mark({ attributes: { 'data-md': word } });
const lineAttrs = (attributes) => Decoration.line({ attributes });

function decorate(state) {
  const showAll = !state.field(focused);
  const doc = state.doc;
  const cfg = state.field(host);
  const { list: marks, lost } = state.field(markers);
  const asks = state.field(asking);
  const hover = state.field(hovered);
  const active = new Set();
  if (!showAll) {
    for (const range of state.selection.ranges) {
      for (let n = doc.lineAt(range.from).number; n <= doc.lineAt(range.to).number; n++) active.add(n);
    }
  }
  const isActive = (from, to = from) => {
    if (showAll) return false;
    for (let n = doc.lineAt(from).number; n <= doc.lineAt(to).number; n++) if (active.has(n)) return true;
    return false;
  };
  const markerLines = new Set(marks.map((m) => doc.lineAt(m.from).number));
  const onMarker = (from, to) => {
    for (let n = doc.lineAt(from).number; n <= doc.lineAt(to).number; n++) if (markerLines.has(n)) return true;
    return false;
  };
  const out = [];
  const add = (from, to, deco) => out.push(deco.range(from, to));
  // replace never reaches over a marker: the marker is the block's own widget.
  const replace = (from, to, deco) => {
    if (!onMarker(from, to)) add(from, to, deco);
  };
  const lineWords = new Map();
  const lineEdges = new Map();
  const gather = (into, from, to, word) => {
    for (let n = doc.lineAt(from).number; n <= doc.lineAt(to).number; n++) {
      if (markerLines.has(n)) continue;
      if (!into.has(n)) into.set(n, new Set());
      into.get(n).add(word);
    }
  };
  const lineDeco = (from, to, word) => gather(lineWords, from, to, word);
  const lineEdge = (at, word) => gather(lineEdges, at, at, word);
  const hideMark = (from, to, withSpace) => {
    if (withSpace && doc.sliceString(to, to + 1) === ' ') to++;
    if (to > from) replace(from, to, hide);
  };

  // The markers: hidden, with the handle of their block.
  const current = state.field(focused) ? blockAt(state, state.selection.main.head) : null;
  for (const m of marks) {
    const line = doc.lineAt(m.from);
    const html = cfg.handle ? cfg.handle(m.id) : '';
    add(line.from, line.to, Decoration.replace({ widget: new MarkerWidget(m.id, html, m.id === current || m.id === hover), block: true }));
  }
  // The markers of parts are hidden everywhere (ADR-22); the cursor jumps over them (atomicMarkers).
  for (const [from, to] of partMarkers(doc)) replace(from, to, hide);
  // The part the cursor stands in shows its boundary and where it is used: C22's [data-part] with data-framed, which
  // draws the frame and its name.
  if (current) {
    const p = partAt(state, state.selection.main.head, current);
    if (p && p.to > p.from) {
      const use = (cfg.partUse && cfg.partUse(current, p.id)) || { count: 0, where: '' };
      const label = (p.name ? `Value ${p.name} · ` : '') + (use.count ? `Used in ${use.count} ${use.count === 1 ? 'place' : 'places'}` : 'Not used elsewhere');
      const attributes = { 'data-part': label, 'data-framed': '' };
      if (use.where) attributes.title = `Changes here show there too: ${use.where}`;
      add(p.from, p.to, Decoration.mark({ attributes }));
    }
  }
  // A block aligned center or right shows so on its lines (ADR-16), with C22's text-center and text-right.
  marks.forEach((m, k) => {
    const align = cfg.align && cfg.align(m.id);
    if (!align) return;
    const first = doc.lineAt(m.from).number + 1;
    const last = k + 1 < marks.length ? doc.lineAt(marks[k + 1].from).number - 1 : doc.lines;
    for (let n = first; n <= last; n++) add(doc.line(n).from, doc.line(n).from, lineClass(`text-${align}`));
  });
  // A block used elsewhere shows its boundary and where it is used while the cursor is in it, so whoever writes there
  // knows that other pages change too (ADR-22).
  const usedCount = current && cfg.usedCount ? cfg.usedCount(current) : 0;
  if (usedCount) {
    const k = marks.findIndex((m) => m.id === current);
    const first = doc.lineAt(marks[k].from).number + 1;
    let last = k + 1 < marks.length ? doc.lineAt(marks[k + 1].from).number - 1 : doc.lines;
    while (last >= first && !doc.line(last).text.trim()) last--;
    // C22's .editor [data-used] on each of its lines; the value says where the line stands in the block, so C22 draws one
    // frame round the run of lines without changing their height.
    for (let n = first; n <= last; n++) {
      const l = doc.line(n);
      const where = first === last ? 'only' : n === first ? 'first' : n === last ? 'last' : 'middle';
      add(l.from, l.from, lineAttrs({ 'data-used': where }));
    }
    if (last >= first) {
      const l = doc.line(first);
      add(l.to, l.to, Decoration.widget({ widget: new UsedWidget(usedCount, cfg.usedIn(current)), side: 1 }));
    }
  }
  // A block used elsewhere whose marker is gone asks here first.
  for (const id of asks) {
    if (!lost.has(id)) continue;
    // Before the line where the marker was, or after the line it ended up in, so no picture of a table covers it.
    const at = Math.min(lost.get(id), doc.length);
    const line = doc.lineAt(at);
    const start = at === line.from;
    const used = (cfg.usedIn && cfg.usedIn(id)) || 'other pages';
    const pos = start ? line.from : line.to;
    add(pos, pos, Decoration.widget({ widget: new AskWidget(id, used), block: true, side: start ? -1 : 1 }));
  }
  // A value that looks like a password typed as text is marked, and asked about below its line.
  state.field(plainAsks).forEach((a, k) => {
    add(a.from, a.to, mark('underline decoration-wavy decoration-destructive'));
    const line = doc.lineAt(a.to);
    add(line.to, line.to, Decoration.widget({ widget: new PlainWidget(k), block: true, side: 1 }));
  });

  // A blank line between paragraphs, and so between blocks, is as high as the space between paragraphs when the page is
  // read, not a whole line, as in Typora or Obsidian; on the cursor it is a line again. Blank lines in code stay.
  const tree = syntaxTree(state);
  const inCode = (pos) => {
    for (let node = tree.resolveInner(pos, 1); node; node = node.parent) if (node.name === 'FencedCode' || node.name === 'CodeBlock') return true;
    return false;
  };
  const wiki = [];
  // Wiki links and directives are UnderDoc's own syntax, which the Markdown parser does not know.
  for (let n = 1; n <= doc.lines; n++) {
    if (markerLines.has(n)) continue;
    const l = doc.line(n);
    if (l.length === 0) {
      if (!isActive(l.from) && !inCode(l.from)) lineDeco(l.from, l.from, 'blank');
      continue;
    }
    const embed = EMBED.exec(l.text);
    if (embed) {
      if (!isActive(l.from)) {
        const ref = embed[1] + (embed[2] ? '#' + embed[2] : '');
        const said = (cfg.embed && cfg.embed(ref)) || {};
        add(l.from, l.to, Decoration.replace({ widget: new EmbedWidget(ref, said.label || ref, said.href || `/o/${embed[1]}${embed[2] ? '#' + embed[2] : ''}`) }));
        continue;
      }
      add(l.from, l.to, md('directive'));
    } else if (/^::[a-z]+\{.*\}\s*$/.test(l.text)) {
      add(l.from, l.to, md('directive'));
    }
    if (!inCode(l.from)) {
      for (const match of l.text.matchAll(SECRET)) {
        const from = l.from + match.index;
        const to = from + match[0].length;
        if (!isActive(from, to)) add(from, to, Decoration.replace({ widget: new SecretWidget(match[1], cfg.secret || null) }));
        else add(from, to, md('directive'));
      }
    }
    for (const match of l.text.matchAll(/\[\[([^\]\n]+)\]\]/g)) {
      const from = l.from + match.index;
      const to = from + match[0].length;
      wiki.push([from, to]);
      add(from + 2, to - 2, md('wiki'));
      if (!isActive(from)) {
        add(from, from + 2, hide);
        add(to - 2, to, hide);
      }
    }
  }
  const inWiki = (pos) => wiki.some(([from, to]) => pos >= from && pos < to);
  // A value the editor asks about shows as typed: Markdown that reaches into it is neither hidden nor drawn. `pw-_x_` is emphasis
  // to Markdown, but its marks belong to the value (plain.js); hidden, the question showed `pw-x` while Keep as password
  // seals `pw-_x_` (2026-11-22).
  const asked = state.field(plainAsks);
  const inAsk = (from, to) => asked.some((a) => from < a.to && to > a.from);

  syntaxTree(state).iterate({
    enter(node) {
      const { from, to } = node;
      const name = node.name;
      if (name === 'HTMLBlock' && onMarker(from, to)) return false;
      let match;
      if ((match = /^ATXHeading(\d)$/.exec(name))) {
        lineDeco(from, from, `h${match[1]}`);
        if (!isActive(from)) {
          for (let child = node.node.firstChild; child; child = child.nextSibling) {
            if (child.name === 'HeaderMark') hideMark(child.from, child.to, child.from === from);
          }
        }
        return;
      }
      if ((match = /^SetextHeading(\d)$/.exec(name))) {
        lineDeco(from, from, `h${match[1]}`);
        return;
      }
      // A Markdown sign left visible (on the cursor's line, or one that never hides, such as the number of an ordered list)
      // is muted; where it is hidden, the replacement draws instead of it.
      if (/Mark$/.test(name)) add(from, to, md('mark'));
      switch (name) {
        case 'Emphasis':
        case 'StrongEmphasis':
        case 'Strikethrough':
        case 'InlineCode': {
          if (inAsk(from, to)) return name !== 'InlineCode';
          add(from, to, md({ Emphasis: 'em', StrongEmphasis: 'strong', Strikethrough: 'del', InlineCode: 'code' }[name]));
          if (!isActive(from, to)) {
            for (let child = node.node.firstChild; child; child = child.nextSibling) {
              if (/Mark$/.test(child.name)) hideMark(child.from, child.to, false);
            }
          }
          return name !== 'InlineCode';
        }
        case 'Link': {
          if (inWiki(from)) return false;
          const marks2 = [];
          let url = null;
          for (let child = node.node.firstChild; child; child = child.nextSibling) {
            if (child.name === 'LinkMark') marks2.push(child);
            if (child.name === 'URL') url = child;
          }
          if (marks2.length < 2) return;
          add(marks2[0].to, marks2[1].from, md('link'));
          if (url && !isActive(from, to)) {
            replace(from, marks2[0].to, hide);
            replace(marks2[1].from, to, hide);
          }
          return;
        }
        case 'Image': {
          const text = doc.sliceString(from, to);
          const parts = /^!\[([^\]]*)\]\(([^)\s]+)/.exec(text);
          if (parts && !isActive(from, to)) {
            replace(from, to, Decoration.replace({ widget: new ImageWidget(parts[2], parts[1]) }));
            return false;
          }
          // Its source on the cursor's line: the signs muted, the address as the address of a link.
          for (let child = node.node.firstChild; child; child = child.nextSibling) {
            if (child.name === 'LinkMark') add(child.from, child.to, md('mark'));
            if (child.name === 'URL') add(child.from, child.to, md('url'));
          }
          return false;
        }
        case 'URL':
          add(from, to, md('url'));
          return;
        case 'Blockquote':
          lineDeco(from, to, 'quote');
          return;
        case 'QuoteMark':
          if (!isActive(from)) hideMark(from, to, true);
          return;
        case 'ListItem': {
          const list = node.node.parent && node.node.parent.name;
          const task = node.node.getChild('Task');
          const listMark = node.node.getChild('ListMark');
          if (listMark && !isActive(from)) {
            if (task) hideMark(listMark.from, listMark.to, true);
            else if (list === 'BulletList') replace(listMark.from, listMark.to, Decoration.replace({ widget: new BulletWidget() }));
          }
          return;
        }
        case 'TaskMarker': {
          const checked = /x/i.test(doc.sliceString(from, to));
          if (!isActive(from)) replace(from, to, Decoration.replace({ widget: new CheckboxWidget(checked) }));
          if (checked) add(to, doc.lineAt(to).to, md('done'));
          return;
        }
        case 'FencedCode': {
          lineDeco(from, to, 'codeblock');
          lineEdge(from, 'first');
          lineEdge(to, 'last');
          for (let child = node.node.firstChild; child; child = child.nextSibling) {
            if (child.name === 'CodeMark') {
              if (isActive(from, to)) add(child.from, child.to, md('mark'));
              else hideMark(child.from, child.to, false);
            }
            if (child.name === 'CodeInfo') add(child.from, child.to, md('info'));
          }
          return false;
        }
        case 'CodeBlock':
          lineDeco(from, to, 'codeblock');
          return false;
        case 'HorizontalRule':
          if (!isActive(from)) replace(from, to, Decoration.replace({ widget: new RuleWidget() }));
          return;
        case 'Table':
          if (!isActive(from, to) && !onMarker(from, to)) {
            add(from, to, Decoration.replace({ widget: new TableWidget(doc.sliceString(from, to)), block: true }));
          } else {
            lineDeco(from, to, 'table-source');
          }
          return false;
        default:
          return;
      }
    },
  });
  for (const [n, words] of lineWords) {
    const attributes = { 'data-md': [...words].join(' ') };
    if (lineEdges.has(n)) attributes['data-md-edge'] = [...lineEdges.get(n)].join(' ');
    add(doc.line(n).from, doc.line(n).from, lineAttrs(attributes));
  }
  return Decoration.set(held(state, out), true);
}

// ---- Dragging a block: while it is held, its lines (with the blank line that parts it from the next) are hidden, and
// the gap stands before the block it would land before, or at the end.
const setDrag = StateEffect.define();
const dragging = StateField.define({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setDrag)) value = e.value;
    return value;
  },
});

/** heldRange is where the held block stands in the text, [from, to] over whole lines, or null. */
function heldRange(state, id) {
  const list = state.field(markers).list;
  const k = list.findIndex((m) => m.id === id);
  if (k < 0) return null;
  const doc = state.doc;
  if (k + 1 < list.length) return [list[k].from, list[k + 1].from - 1];
  // The last block takes the blank line before it along instead.
  const from = list[k].from;
  return [from > 0 && doc.lineAt(from - 1).text === '' ? from - 1 : from, doc.length];
}

function held(state, out) {
  const drag = state.field(dragging);
  if (!drag) return out;
  const range = heldRange(state, drag.id);
  if (!range) return out;
  const [from, to] = range;
  const kept = out.filter((r) => r.to < from || r.from > to);
  kept.push(Decoration.replace({ block: true }).range(from, to));
  const before = drag.before && state.field(markers).list.find((m) => m.id === drag.before);
  // At the end: after the text, or, when the held block is the last one, where it was.
  const end = !before && to < state.doc.length;
  const at = before ? before.from : end ? state.doc.length : from;
  kept.push(Decoration.widget({ widget: new GapWidget(drag), block: true, side: end ? 1 : -1 }).range(at));
  return kept;
}

const asking = StateField.define({
  create: () => [],
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setAsks)) value = e.value;
    return value;
  },
});

// plainAsks are the values that wait for an answer, [{from, to, value}], kept in place while the text around them
// changes; one whose text changed goes, and the next save looks again. So does one that is no longer the whole value of
// its line: the editor asks once the start of a value is long enough, and letters typed after it make the value longer
// (2026-11-22). Kept, the question would mark only the start, and Keep as password would leave the rest as text.
const plainAsks = StateField.define({
  create: () => [],
  update(value, tr) {
    if (tr.docChanged) {
      const doc = tr.state.doc;
      const whole = (a) => {
        const line = doc.lineAt(a.from);
        return findPlain(line.text).some((f) => line.from + f.from === a.from && line.from + f.to === a.to);
      };
      value = value
        .map((a) => ({ ...a, from: tr.changes.mapPos(a.from, 1), to: tr.changes.mapPos(a.to, -1) }))
        .filter((a) => a.to > a.from && doc.sliceString(a.from, a.to) === a.value && whole(a));
    }
    for (const e of tr.effects) if (e.is(setPlain)) value = e.value;
    return value;
  },
});

const hovered = StateField.define({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setHover)) value = e.value;
    return value;
  },
});

const livePreview = StateField.define({
  create: (state) => decorate(state),
  update(value, tr) {
    const effects = tr.effects.some((e) => e.is(setFocus) || e.is(refresh) || e.is(setHover) || e.is(setAsks) || e.is(setPlain) || e.is(setDrag));
    if (tr.docChanged || tr.selection || effects || syntaxTree(tr.state) !== syntaxTree(tr.startState)) return decorate(tr.state);
    return value;
  },
  provide: (field) => EditorView.decorations.from(field),
});

// The markers cannot be entered: the cursor jumps over them, as over a picture.
const atomicMarkers = EditorView.atomicRanges.of((view) => {
  const b = [];
  for (const m of view.state.field(markers).list) {
    const line = view.state.doc.lineAt(m.from);
    b.push(Decoration.mark({}).range(line.from, line.to));
  }
  for (const [from, to] of partMarkers(view.state.doc)) b.push(Decoration.mark({}).range(from, to));
  return Decoration.set(b, true);
});

// A selection never ends inside a marker line: typing there would break the marker.
const keepOutOfMarkers = EditorState.transactionFilter.of((tr) => {
  if (!tr.selection) return tr;
  const state = tr.state;
  const old = tr.startState.selection.main.head;
  let moved = false;
  const ranges = tr.selection.ranges.map((r) => {
    const fix = (pos, forward) => {
      const line = markerLineAt(state, pos);
      if (!line) return pos;
      moved = true;
      if (forward && line.to < state.doc.length) return line.to + 1;
      if (line.from > 0) return line.from - 1;
      return Math.min(line.to + 1, state.doc.length);
    };
    const forward = r.head >= old;
    const anchor = fix(r.anchor, forward);
    const head = fix(r.head, forward);
    return EditorSelection.range(anchor, head);
  });
  if (!moved) return tr;
  return [tr, { selection: EditorSelection.create(ranges, tr.selection.mainIndex), sequential: true }];
});

// A marker of a part cannot go on its own while text of the part is left (ADR-22): it is put back where it was. A part
// whose text is all gone loses both markers, and saving asks what happens where it is used.
const keepPartsWhole = EditorState.transactionFilter.of((tr) => {
  if (!tr.docChanged || tr.annotation(system)) return tr;
  const before = partsIn(tr.startState.doc.toString());
  if (!before.length) return tr;
  const after = tr.newDoc.toString();
  const changes = [];
  for (const p of before) {
    const open = tr.startState.doc.sliceString(p.open[0], p.open[1]);
    const close = tr.startState.doc.sliceString(p.close[0], p.close[1]);
    const o = tr.changes.mapPos(p.open[0], 1, MapMode.TrackDel);
    const c = tr.changes.mapPos(p.close[1], -1, MapMode.TrackDel);
    const from = tr.changes.mapPos(p.open[1], 1);
    const to = tr.changes.mapPos(p.close[0], -1);
    const textLeft = to > from && after.slice(from, to).replace(PART_MARKER, '').length > 0;
    const openGone = o === null || after.slice(o, o + open.length) !== open;
    const closeGone = c === null || after.slice(c - close.length, c) !== close;
    // Backspace right after a part, or Delete right before it, took only its marker: it stays, and the character of the
    // part next to it goes instead, as the key meant.
    const onlyMarker = (m) => tr.startState.doc.length - tr.newDoc.length === m.length;
    if (textLeft && openGone && !closeGone) {
      changes.push({ from, insert: open });
      if (onlyMarker(open)) changes.push({ from, to: from + 1 });
    }
    if (textLeft && closeGone && !openGone) {
      if (onlyMarker(close)) changes.push({ from: to - 1, to });
      changes.push({ from: to, insert: close });
    }
    if (!textLeft && !openGone && !closeGone) {
      changes.push({ from: tr.changes.mapPos(p.open[0], 1), to: tr.changes.mapPos(p.open[1], -1) });
      changes.push({ from: tr.changes.mapPos(p.close[0], 1), to: tr.changes.mapPos(p.close[1], -1) });
    }
  }
  return changes.length ? [tr, { changes, sequential: true }] : tr;
});

// Backspace at the start of a block's first line takes away its marker: the block joins the one before it, and its text
// stays where it is. Delete at the end of the line before a marker does the same from the other side.
function joinBackward(view) {
  const { state } = view;
  const sel = state.selection.main;
  if (!sel.empty) return false;
  const line = state.doc.lineAt(sel.head);
  if (sel.head !== line.from || line.number < 2) return false;
  const before = state.doc.line(line.number - 1);
  if (!markerLineAt(state, before.from)) return false;
  // Before the first block there is nothing to join: the key does nothing rather than break the marker.
  if (!state.doc.sliceString(0, before.from).trim()) return true;
  view.dispatch({ changes: { from: before.from, to: line.from }, userEvent: 'delete.backward', scrollIntoView: true });
  return true;
}

function joinForward(view) {
  const { state } = view;
  const sel = state.selection.main;
  if (!sel.empty) return false;
  const line = state.doc.lineAt(sel.head);
  if (sel.head !== line.to || line.number >= state.doc.lines) return false;
  const next = state.doc.line(line.number + 1);
  if (!markerLineAt(state, next.from)) return false;
  view.dispatch({ changes: { from: next.from, to: Math.min(next.to + 1, state.doc.length) }, userEvent: 'delete.forward' });
  return true;
}

// A to-do typed on its own line becomes an item of a list: "[ ] " turns into "- [ ] ".
const todoShortcut = EditorView.inputHandler.of((view, from, to, text) => {
  if (text !== ' ' || from !== to) return false;
  const line = view.state.doc.lineAt(from);
  const match = /^(\s*)\[([ xX]?)\]$/.exec(view.state.sliceDoc(line.from, from));
  if (!match) return false;
  const insert = `${match[1]}- [${match[2] || ' '}] `;
  view.dispatch({ changes: { from: line.from, to, insert }, selection: { anchor: line.from + insert.length }, userEvent: 'input.type' });
  return true;
});

// ---- The slash menu: "/" at the start of an empty line opens the kinds; typing filters, arrows choose, Enter takes.
const setSlash = StateEffect.define();
const slash = StateField.define({
  create: () => null,
  update(value, tr) {
    for (const e of tr.effects) if (e.is(setSlash)) return e.value;
    if (!value) return value;
    const from = tr.changes.mapPos(value.from, 1, MapMode.TrackDel);
    if (from === null) return null;
    const head = tr.state.selection.main.head;
    const line = tr.state.doc.lineAt(from);
    if (tr.state.doc.sliceString(from, from + 1) !== '/' || head <= from || head > line.to || !tr.state.selection.main.empty) return null;
    const query = tr.state.doc.sliceString(from + 1, head);
    if (/\s\s|^\s/.test(query)) return null;
    return { from, query, cur: query === value.query ? value.cur : 0 };
  },
  provide: (field) =>
    showTooltip.from(field, (value) =>
      value ? { pos: value.from, above: false, strictSide: false, arrow: false, create: (view) => slashTooltip(view) } : null,
    ),
});

const openSlash = EditorView.inputHandler.of((view, from, to, text) => {
  if (text !== '/' || from !== to) return false;
  const line = view.state.doc.lineAt(from);
  if (line.text.trim() !== '' || markerLineAt(view.state, from)) return false;
  view.dispatch({
    changes: { from, to, insert: '/' },
    selection: { anchor: from + 1 },
    effects: setSlash.of({ from, query: '', cur: 0 }),
    userEvent: 'input.type',
  });
  return true;
});

function slashItems(state) {
  const value = state.field(slash);
  const all = state.field(host).slash || [];
  const q = ((value && value.query) || '').toLowerCase();
  return all.filter((x) => !q || x.n.toLowerCase().includes(q) || x.k === q);
}

function slashTooltip(view) {
  // C22's .command: a menu of the kinds, the one the arrows chose data-selected, a later one aria-disabled with its word.
  const dom = document.createElement('div');
  dom.className = 'command w-64 border shadow-md';
  dom.setAttribute('aria-label', 'Add a block');
  const draw = () => {
    const value = view.state.field(slash);
    if (!value) return;
    const items = slashItems(view.state);
    const all = view.state.field(host).slash || [];
    const menu = document.createElement('div');
    menu.setAttribute('role', 'menu');
    menu.setAttribute('aria-orientation', 'vertical');
    menu.dataset.empty = 'Nothing fits.';
    items.forEach((x, n) => {
      const b = document.createElement('button');
      b.type = 'button';
      b.setAttribute('role', 'menuitem');
      b.dataset.k = String(all.indexOf(x));
      if (n === value.cur) b.dataset.selected = 'true';
      if (x.later) {
        b.disabled = true;
        b.setAttribute('aria-disabled', 'true');
        b.title = x.later;
      } else if (x.hint) b.title = x.hint;
      b.innerHTML = `${x.icon || ''}<span></span>${x.later ? '<span class="ms-auto text-xs">later</span>' : ''}`;
      b.querySelector('span').textContent = x.n;
      menu.append(b);
    });
    dom.replaceChildren(menu);
  };
  dom.addEventListener('mousedown', (e) => e.preventDefault());
  dom.addEventListener('click', (e) => {
    const b = e.target.closest && e.target.closest('button[data-k]');
    if (b && !b.disabled) pickSlash(view, +b.dataset.k);
  });
  draw();
  return { dom, update: draw };
}

function pickSlash(view, k) {
  const value = view.state.field(slash);
  const item = (view.state.field(host).slash || [])[k];
  if (!value || !item || item.later) return false;
  const from = value.from;
  const to = view.state.selection.main.head;
  if (item.k === 'secret') {
    // The value is asked for outside the editor; its reference comes back and goes where the slash stood.
    view.dispatch({ changes: { from, to, insert: '' }, selection: { anchor: from }, effects: setSlash.of(null), userEvent: 'input.type' });
    const made = call(view, 'secret');
    if (made && made.then) made.then((ref) => ref && insertText(view, ref));
    return true;
  }
  const text = item.k === 'embed' ? '' : turnInto('', item.k);
  const caret = item.k === 'code' ? 4 : item.k === 'table' ? 2 : text.length;
  const end = item.k === 'table' ? 8 : caret;
  view.dispatch({
    changes: { from, to, insert: text },
    selection: EditorSelection.range(from + caret, from + end),
    effects: setSlash.of(null),
    userEvent: 'input.type',
  });
  if (item.k === 'embed') call(view, 'hint', 'Paste the address of a block or page here, as Copy address put it on the clipboard.');
  return true;
}

const slashKeys = Prec.highest(
  keymap.of([
    { key: 'ArrowDown', run: (view) => stepSlash(view, 1) },
    { key: 'ArrowUp', run: (view) => stepSlash(view, -1) },
    {
      key: 'Enter',
      run(view) {
        const value = view.state.field(slash);
        if (!value) return false;
        const item = slashItems(view.state)[value.cur];
        if (!item) return true;
        return pickSlash(view, (view.state.field(host).slash || []).indexOf(item));
      },
    },
    {
      key: 'Escape',
      run(view) {
        if (!view.state.field(slash)) return false;
        view.dispatch({ effects: setSlash.of(null) });
        return true;
      },
    },
  ]),
);

function stepSlash(view, step) {
  const value = view.state.field(slash);
  if (!value) return false;
  const items = slashItems(view.state);
  if (!items.length) return true;
  let cur = value.cur;
  for (let n = 0; n < items.length; n++) {
    cur = (cur + step + items.length) % items.length;
    if (!items[cur].later) break;
  }
  view.dispatch({ effects: setSlash.of({ ...value, cur }) });
  return true;
}

// ---- The bar: every button writes plain Markdown (M-8).

/** format wraps the selection in a Markdown marker, or takes the marker away when it is already there. */
function format(view, kind) {
  const { state } = view;
  const sel = state.selection.main;
  const s0 = sel.from;
  const e0 = sel.to;
  const text = state.sliceDoc(s0, e0);
  if (kind === 'link') {
    const label = text || 'text';
    const out = `[${label}](https://)`;
    view.dispatch({ changes: { from: s0, to: e0, insert: out }, selection: EditorSelection.range(s0 + label.length + 3, s0 + out.length - 1), userEvent: 'input' });
    return true;
  }
  const m = { bold: '**', italic: '_', strike: '~~', code: '`' }[kind];
  if (!m) return false;
  if (state.sliceDoc(s0 - m.length, s0) === m && state.sliceDoc(e0, e0 + m.length) === m) {
    view.dispatch({
      changes: [{ from: s0 - m.length, to: s0 }, { from: e0, to: e0 + m.length }],
      selection: EditorSelection.range(s0 - m.length, e0 - m.length),
      userEvent: 'input',
    });
    return true;
  }
  view.dispatch({
    changes: [{ from: s0, insert: m }, { from: e0, insert: m }],
    selection: EditorSelection.range(s0 + m.length, e0 + m.length),
    userEvent: 'input',
  });
  return true;
}

/** codeAt returns the fenced code around a position, or null. */
function codeAt(state, pos) {
  const tree = syntaxTree(state);
  for (let node = tree.resolveInner(pos, 1); node; node = node.parent) if (node.name === 'FencedCode') return node;
  for (let node = tree.resolveInner(pos, -1); node; node = node.parent) if (node.name === 'FencedCode') return node;
  return null;
}

/** linesOf returns the lines the selection touches, within the block of the cursor. */
function linesOf(state) {
  const sel = state.selection.main;
  const code = codeAt(state, sel.head);
  if (code) return { from: code.from, to: code.to, code: true };
  let from = state.doc.lineAt(sel.from).from;
  let to = state.doc.lineAt(sel.to).to;
  for (const m of state.field(markers).list) {
    const line = state.doc.lineAt(m.from);
    if (line.to < sel.head && line.to + 1 > from) from = line.to + 1;
    if (line.from > sel.head && line.from - 1 < to) to = Math.max(from, line.from - 1);
  }
  return { from, to, code: false };
}

/** tableAt returns the table around the cursor, {from, to}, or null. */
function tableAt(state) {
  const tree = syntaxTree(state);
  const head = state.selection.main.head;
  for (const side of [1, -1]) {
    for (let node = tree.resolveInner(head, side); node; node = node.parent) if (node.name === 'Table') return { from: node.from, to: node.to };
  }
  return null;
}

/** tableTool uses a tool of the bar on the table at the cursor; the cursor goes to the cell the tool names. */
function tableTool(view, tool) {
  const at = tableAt(view.state);
  if (!at) return false;
  const text = view.state.sliceDoc(at.from, at.to);
  const done = tables.apply(text, tables.cellAt(text, view.state.selection.main.head - at.from), tool);
  if (!done) return false;
  view.dispatch({
    changes: { from: at.from, to: at.to, insert: done.text },
    selection: { anchor: at.from + tables.cellStart(done.text, done.row, done.col) },
    userEvent: 'input',
    scrollIntoView: true,
  });
  return true;
}

function kindAt(state) {
  const { from, code } = linesOf(state);
  return kindOf(state.doc.lineAt(from).text, code);
}

function turnLines(view, kind) {
  const { state } = view;
  const { from, to } = linesOf(state);
  const text = turnInto(state.sliceDoc(from, to), kind);
  view.dispatch({ changes: { from, to, insert: text }, selection: { anchor: from + text.length }, userEvent: 'input', scrollIntoView: true });
  return true;
}

// ---- Pasting an address alone on an empty line embeds what it names: nobody types IDs (ADR-18).
const pasteAddress = EditorView.domEventHandlers({
  paste(event, view) {
    const sel = view.state.selection.main;
    if (!sel.empty) return false;
    const line = view.state.doc.lineAt(sel.head);
    const pasted = (event.clipboardData || window.clipboardData).getData('text');
    if (line.text.trim() !== '') {
      // The address of a part pasted inside a paragraph embeds it there (ADR-22).
      const inline = inlineEmbedFor(pasted);
      if (!inline) return false;
      event.preventDefault();
      view.dispatch({ changes: { from: sel.head, insert: inline }, selection: { anchor: sel.head + inline.length }, userEvent: 'input.paste' });
      return true;
    }
    const embed = embedFor(pasted);
    if (!embed) return false;
    event.preventDefault();
    view.dispatch({ changes: { from: line.from, to: line.to, insert: embed }, selection: { anchor: line.from + embed.length }, userEvent: 'input.paste' });
    return true;
  },
  mousemove(event, view) {
    const pos = view.posAtCoords({ x: event.clientX, y: event.clientY }, false);
    const id = pos === null ? null : blockAt(view.state, pos);
    if (id !== view.state.field(hovered)) view.dispatch({ effects: setHover.of(id) });
    return false;
  },
  mouseleave(_event, view) {
    if (view.state.field(hovered) !== null) view.dispatch({ effects: setHover.of(null) });
    return false;
  },
});

// ---- Where a pasted text came from (T-41). Copying from a page of UnderDoc puts, beside the text, the page and its blocks
// on the clipboard (ORIGIN); pasting it into a page notes the stretch it landed in, and saving that stretch tells the
// server, which writes the origin into the version of the block. A copy stays a copy: nothing ties it to its source. Text
// from anywhere else carries no origin, and neither does an origin whose text is not the text pasted.
const setPending = StateEffect.define();
const clearPasted = StateEffect.define();
let pastedCount = 0;
const pasted = StateField.define({
  create: () => ({ pending: null, list: [] }),
  update(v, tr) {
    let { pending, list } = v;
    for (const e of tr.effects) {
      if (e.is(setPending)) pending = e.value;
      if (e.is(clearPasted)) list = list.filter((x) => !e.value.includes(x.n));
    }
    if (tr.docChanged) {
      list = list.map((x) => ({ ...x, from: tr.changes.mapPos(x.from, 1), to: tr.changes.mapPos(x.to, -1) })).filter((x) => x.to > x.from);
      if (pending && tr.isUserEvent('input.paste')) {
        const copied = pending;
        tr.changes.iterChanges((_fa, _ta, from, to) => {
          if (to > from) list = [...list, { n: ++pastedCount, from, to, copied }];
        });
      }
      pending = null;
    }
    return pending === v.pending && list === v.list ? v : { pending, list };
  },
});

// Every paste says whether it carries an origin, before the paste itself is handled.
const pasteOrigin = EditorView.domEventObservers({
  paste(event, view) {
    view.dispatch({ effects: setPending.of(originOf(event.clipboardData)) });
  },
});

// ---- Pasted and dropped images become attachments of the page (ADR-28): the workspace uploads each, and the editor
// puts a line that shows it where it was pasted or dropped. Other files are left to the browser.
function insertImages(view, files, pos) {
  const images = [...(files || [])].filter((f) => /^image\//.test(f.type));
  if (!images.length || !callbacks.get(view) || !callbacks.get(view).upload) return false;
  let at = pos;
  (async () => {
    for (const f of images) {
      let done;
      try {
        done = await call(view, 'upload', f);
      } catch (err) {
        call(view, 'hint', `${f.name} could not be uploaded: ${err && err.message ? err.message : err}`);
        continue;
      }
      const line = view.state.doc.lineAt(Math.min(at, view.state.doc.length));
      const text = imageLine(f.name, done.url);
      // On a line of its own: after the line where it landed, or in its place when that line is empty.
      const insert = line.text.trim() === '' ? text : '\n' + text;
      const from = line.text.trim() === '' ? line.from : line.to;
      view.dispatch({ changes: { from, to: line.text.trim() === '' ? line.to : from, insert }, userEvent: 'input.paste', scrollIntoView: true });
      at = from + insert.length;
    }
  })();
  return true;
}

const pasteImages = EditorView.domEventHandlers({
  paste(event, view) {
    const files = event.clipboardData && event.clipboardData.files;
    if (!insertImages(view, files, view.state.selection.main.head)) return false;
    event.preventDefault();
    return true;
  },
  drop(event, view) {
    const files = event.dataTransfer && event.dataTransfer.files;
    if (!files || !files.length) return false;
    const pos = view.posAtCoords({ x: event.clientX, y: event.clientY }, false) ?? view.state.selection.main.head;
    if (!insertImages(view, files, pos)) return false;
    event.preventDefault();
    return true;
  },
});

// ---- Calling back into the workspace.
const callbacks = new WeakMap();
function call(view, name, ...args) {
  const on = callbacks.get(view);
  if (on && on[name]) return on[name](...args);
  return undefined;
}

// insertText puts text where the cursor is, in place of the selection.
function insertText(view, text) {
  const { from, to } = view.state.selection.main;
  view.dispatch({ changes: { from, to, insert: text }, selection: { anchor: from + text.length }, userEvent: 'input' });
  view.focus();
}

// ---- The editor of one page.

class PageEditor {
  constructor(options) {
    this.options = options;
    this.stored = options.blocks.map((b) => ({ id: b.id, md: normalize(b.md), usedIn: b.usedIn || [] }));
    // Where the parts of each block are used (ADR-22): block id → part id → names of pages.
    this.partUses = new Map(options.blocks.map((b) => [b.id, new Map((b.parts || []).map((p) => [p.id, p.usedIn || []]))]));
    // The alignment of each block: not part of its Markdown, the workspace sets it through the API (setAlign).
    this.aligns = new Map(options.blocks.filter((b) => b.align).map((b) => [b.id, b.align]));
    this.confirmed = new Set();
    // The ways chosen for blocks whose change takes away something in use (ADR-22), until that change is saved.
    this.ways = {};
    this.saving = null;
    this.again = false;
    this.timer = 0;
    this.during = null;
    const text = joinBlocks(this.stored);
    const list = [];
    let pos = 0;
    for (const b of this.stored) {
      list.push({ id: b.id, from: pos, adopted: false });
      pos += markerLine(b.id).length + 1 + normalize(b.md).length + 2;
    }
    const cfg = {
      handle: options.handle,
      slash: options.slash || [],
      embed: options.embed,
      secret: options.secret || null,
      usedIn: (id) => {
        const b = this.stored.find((x) => x.id === id);
        return b && b.usedIn.length ? b.usedIn.join(', ') : '';
      },
      align: (id) => this.aligns.get(id) || '',
      partUse: (block, part) => {
        const where = (this.partUses.get(block) || new Map()).get(part) || [];
        return { count: where.length, where: where.join(', ') };
      },
      usedCount: (id) => {
        const b = this.stored.find((x) => x.id === id);
        return b ? b.usedIn.length : 0;
      },
    };
    this.view = new EditorView({
      state: EditorState.create({
        doc: text,
        selection: { anchor: list.length ? Math.min(markerLine(list[0].id).length + 1, text.length) : 0 },
        extensions: [
          host.init(() => cfg),
          markers.init(() => ({ list, lost: new Map() })),
          asking,
          plainAsks,
          hovered,
          dragging,
          focused,
          trackFocus,
          resyncFocus,
          history(),
          drawSelection(),
          EditorView.lineWrapping,
          EditorView.editorAttributes.of({ class: 'editor' }),
          EditorView.contentAttributes.of({ 'aria-label': options.label || 'Page, Markdown' }),
          new LanguageSupport(markdownLanguage),
          slash,
          slashKeys,
          openSlash,
          Prec.high(
            keymap.of([
              { key: 'Backspace', run: joinBackward },
              { key: 'Delete', run: joinForward },
              { key: 'Mod-b', run: (v) => format(v, 'bold') },
              { key: 'Mod-i', run: (v) => format(v, 'italic') },
              { key: 'Mod-k', run: (v) => format(v, 'link') },
            ]),
          ),
          keymap.of([...markdownKeymap, ...defaultKeymap, ...historyKeymap, indentWithTab]),
          todoShortcut,
          pasted,
          pasteOrigin,
          pasteImages,
          pasteAddress,
          atomicMarkers,
          keepOutOfMarkers,
          keepPartsWhole,
          livePreview,
          EditorView.updateListener.of((u) => this.updated(u)),
        ],
      }),
    });
    callbacks.set(this.view, options.on || {});
    // A copy or cut from this page carries where it came from (T-41). The blocks are read before the editor handles the
    // event (a cut takes the text away), the clipboard is written after it, where its text is.
    let copying = null;
    const before = () => {
      const sel = this.view.state.selection.main;
      copying = sel.empty ? null : this.copiedBlocks(sel.from, sel.to);
    };
    const after = (event) => {
      const data = event.clipboardData;
      const blocks = copying;
      copying = null;
      if (event.udOwn || !data || !blocks || !blocks.length || !this.options.page) return;
      const text = data.getData('text/plain');
      if (text) data.setData(ORIGIN, JSON.stringify({ page: this.options.page, blocks, text }));
    };
    for (const kind of ['copy', 'cut']) {
      this.view.dom.addEventListener(kind, before, true);
      this.view.dom.addEventListener(kind, after);
    }
    this.view.dom.addEventListener('mousedown', (e) => {
      if (e.target.closest && e.target.closest(':is([data-ask], [data-plain-ask], .cm-ud-marker) button')) e.preventDefault();
    });
    this.view.dom.addEventListener('click', (e) => {
      const b = e.target.closest && e.target.closest('[data-ask] button[data-answer]');
      if (b) this.answer(b.closest('[data-ask]').dataset.ask, b.dataset.answer === 'delete');
      const p = e.target.closest && e.target.closest('[data-plain-ask] button[data-plain]');
      if (p) this.answerPlain(p, p.dataset.plain === 'secret');
    });
    // Values the writer said are not passwords, for the life of this editor only: saved as typed, they are stored
    // and so not new any more.
    this.plainOK = new Set();
    this.lastKind = null;
    this.lastTable = false;
  }

  get dom() {
    return this.view.dom;
  }

  updated(u) {
    if (u.docChanged) {
      if (this.saving) this.during = this.during ? this.during.compose(u.changes) : u.changes;
      const mine = u.transactions.every((tr) => tr.annotation(system));
      if (!mine) {
        this.emit('status', 'Not saved yet');
        this.schedule();
      }
    }
    if (u.docChanged || u.selectionSet || u.focusChanged) {
      const kind = kindAt(u.state);
      if (kind !== this.lastKind) {
        this.lastKind = kind;
        this.emit('kind', kind);
      }
      const marked = !u.state.selection.main.empty;
      if (marked !== this.lastMarked) {
        this.lastMarked = marked;
        this.emit('selection', marked);
      }
      const inTable = !!tableAt(u.state);
      if (inTable !== this.lastTable) {
        this.lastTable = inTable;
        this.emit('table', inTable);
      }
    }
    if (u.focusChanged && !u.view.hasFocus) this.schedule(0);
    // The workspace draws its page anew around the editor, which can take the focus away and give it back without a
    // focus event; a selection set by a hand then shows that the editor has the focus after all (2026-11-17).
    if (u.selectionSet && u.view.hasFocus && !u.state.field(focused)) {
      queueMicrotask(() => !this.destroyed && this.view.dispatch({ effects: setFocus.of(this.view.hasFocus) }));
    }
  }

  emit(name, ...args) {
    const on = this.options.on || {};
    if (on[name]) on[name](...args);
  }

  schedule(ms = AUTOSAVE_MS) {
    clearTimeout(this.timer);
    this.timer = setTimeout(() => this.save(), ms);
  }

  /** save writes what changed, once; a change meanwhile saves again afterwards. */
  async save() {
    clearTimeout(this.timer);
    if (this.destroyed && !this.flushing) return;
    if (this.saving) {
      this.again = true;
      return this.saving;
    }
    const state = this.view.state;
    // A value that looks like a password typed as text holds the whole page back until the writer answers: nothing of
    // it may leave the browser before (ADR-7, 2026-11-20).
    const found = this.plainNew(state);
    const held = found.filter((f) => !f.partial);
    // The start of a value still being typed on the line of the cursor waits: saved now, its first letters would stay
    // in a version. A short value the cursor left is saved.
    if (!this.flushing && this.view.hasFocus && found.some((f) => f.partial && state.doc.lineAt(f.to).number === state.doc.lineAt(state.selection.main.head).number)) {
      this.emit('status', 'Not saved yet');
      this.schedule();
      return;
    }
    const asked = state.field(plainAsks);
    if (held.length || asked.length) {
      const same = held.length === asked.length && held.every((h, k) => h.from === asked[k].from && h.value === asked[k].value);
      if (!same) this.view.dispatch({ effects: setPlain.of(held) });
      if (held.length) {
        this.emit('status', 'Not saved: is this a password?');
        return;
      }
    }
    const pastes = state.field(pasted).list;
    const p = copiedInto(plan(this.stored, segments(state), this.confirmed), pastes);
    this.ask(p.asks);
    if (idle(p)) {
      if (!p.asks.length) this.emit('status', this.everSaved ? 'Saved' : '');
      return;
    }
    this.emit('status', 'Saving…');
    this.during = null;
    this.saving = (async () => {
      try {
        const done = await run(p, this.stored, this.options.api, this.options.on && this.options.on.saved, this.ways);
        if (pastes.length && !this.destroyed) this.view.dispatch({ effects: clearPasted.of(pastes.map((x) => x.n)) });
        this.ways = {};
        this.stored = done.stored;
        this.place(done.made);
        this.everSaved = true;
        this.emit('status', 'Saved');
      } catch (err) {
        if (err.stored) this.stored = err.stored;
        if (err.made) this.place(err.made);
        if (err instanceof Conflict) {
          this.emit('conflict', this.view.state.doc.toString());
        } else if (err.inUse) {
          this.emit('status', 'Not saved yet');
          this.chooseWays(err);
        } else {
          this.emit('status', 'Not saved');
          this.emit('error', err);
        }
        this.again = false;
      } finally {
        this.saving = null;
        this.during = null;
      }
      if (this.again) {
        this.again = false;
        await this.save();
      }
    })();
    return this.saving;
  }

  /**
   * plainNew lists the values of the text that look like passwords typed as text and are new: not in the stored text of
   * their block, and not said to be no password. Positions are in the whole text.
   */
  plainNew(state) {
    const out = [];
    for (const seg of segments(state)) {
      const stored = seg.id ? (this.stored.find((b) => b.id === seg.id) || {}).md || '' : '';
      const known = new Set(findPlain(stored).map((f) => f.value));
      for (const f of findPlain(seg.text)) {
        if (!known.has(f.value) && !this.plainOK.has(f.value)) out.push({ from: seg.body[0] + f.from, to: seg.body[0] + f.to, value: f.value, partial: f.partial });
      }
    }
    return out;
  }

  /**
   * answerPlain answers the question about a value that looks like a password: as a password, the workspace makes a
   * secret of the page from it and its reference takes the value's place; as text, it is saved as typed.
   */
  async answerPlain(button, secret) {
    const asks = this.view.state.field(plainAsks);
    const a = asks[Number(button.closest('[data-plain-ask]').dataset.plainAsk)];
    if (!a) return;
    if (!secret) {
      this.plainOK.add(a.value);
      this.view.dispatch({ effects: setPlain.of(asks.filter((x) => x !== a)) });
      this.save();
      return;
    }
    const make = call(this.view, 'secretFrom', a.value);
    const ref = make && make.then ? await make : null;
    if (!ref || this.destroyed) return;
    const now = this.view.state.field(plainAsks).find((x) => x.value === a.value);
    if (!now || this.view.state.doc.sliceString(now.from, now.to) !== a.value) return;
    this.view.dispatch({ changes: { from: now.from, to: now.to, insert: ref }, userEvent: 'input' });
  }

  /** place writes the markers of new blocks into the text, where their text is now. The cursor and the view stay. */
  place(made) {
    if (!made.length || this.destroyed) return;
    const map = (pos, assoc = -1) => (this.during ? this.during.mapPos(pos, assoc) : pos);
    const doc = this.view.state.doc;
    const changes = [];
    for (const { place, id } of made) {
      if (place.insert !== undefined) {
        const at = doc.lineAt(Math.min(map(place.insert), doc.length)).from;
        changes.push({ from: at, insert: markerLine(id) + '\n' });
      } else if (place.replace) {
        changes.push({ from: map(place.replace[0], 1), to: map(place.replace[1], -1), insert: markerLine(id) });
      } else if (place.before) {
        changes.push({ from: map(place.line[0], 1), to: map(place.line[1], -1), insert: markerLine(id) });
        const at = doc.lineAt(Math.min(map(place.keep), doc.length)).from;
        changes.push({ from: at, insert: markerLine(place.before) + '\n' });
      }
    }
    this.view.dispatch({ changes, annotations: [system.of(true), Transaction.addToHistory.of(false)] });
  }

  /** ask shows the question for blocks used elsewhere whose marker is gone; a question that has no block left goes. */
  ask(ids) {
    const now = this.view.state.field(asking);
    if (ids.length === now.length && ids.every((id, k) => now[k] === id)) return;
    this.view.dispatch({ effects: setAsks.of(ids) });
  }

  /**
   * chooseWays asks the workspace what happens where something in use goes (ADR-22): the change of a block that takes away
   * a part in use, or the deletion of a block in use. With the ways chosen it saves again; without, the block keeps its
   * text, or comes back.
   */
  async chooseWays(err) {
    const ask = this.options.on && this.options.on.inUse;
    const ways = ask ? await ask({ kind: err.kind, block: err.block, inUse: err.inUse }) : null;
    if (this.destroyed) return;
    if (ways) {
      this.ways[err.block] = ways;
      this.save();
      return;
    }
    this.confirmed.delete(err.block);
    if (err.kind === 'delete') {
      this.answer(err.block, false);
      return;
    }
    const b = this.stored.find((x) => x.id === err.block);
    const seg = segments(this.view.state).find((s) => s.id === err.block);
    if (!b || !seg) return;
    const last = seg.body[1] >= this.view.state.doc.length;
    this.view.dispatch({ changes: { from: seg.body[0], to: seg.body[1], insert: b.md + (last ? '' : '\n\n') }, userEvent: 'input' });
  }

  /**
   * selectionInBlock names the block the selection lies in and where, in UTF-16 offsets of the block's Markdown as
   * stored, or null when it reaches over more than one block.
   */
  selectionInBlock() {
    const state = this.view.state;
    const sel = state.selection.main;
    if (sel.empty) return null;
    const seg = segments(state).find((s) => s.id && sel.from >= s.body[0] && sel.to <= s.body[1]);
    if (!seg) return null;
    const md = normalize(seg.text);
    const shift = seg.text.indexOf(md);
    return { block: seg.id, start: sel.from - seg.body[0] - shift, end: sel.to - seg.body[0] - shift, seg, md, shift };
  }

  /**
   * copiedBlocks names the blocks of this page a stretch of the text lies in, for the origin of a copy (T-41): only
   * blocks as they are saved, so the origin names what the source holds; none if any of them has changes not yet saved.
   */
  copiedBlocks(from, to) {
    const out = [];
    for (const s of segments(this.view.state)) {
      if (!s.id || to <= s.line[0] || from >= s.body[1]) continue;
      const stored = this.stored.find((b) => b.id === s.id);
      if (!stored || normalize(s.text) !== stored.md) return [];
      if (!out.includes(s.id)) out.push(s.id);
    }
    return out;
  }

  /**
   * markSelection makes the selection a part that can be embedded (ADR-22): it saves first, then the server snaps the
   * edges and puts the markers, and its answer becomes the block's text. Answers {ref, url} or {error}.
   */
  async markSelection(name = '') {
    if (!this.selectionInBlock()) return { error: 'Mark text inside one paragraph; for more, copy the address of a block.' };
    await this.save();
    const at = this.selectionInBlock();
    const stored = at && this.stored.find((b) => b.id === at.block);
    if (!at || !stored || stored.md !== at.md || !this.options.api.mark) return { error: 'The page is not saved yet. Try again in a moment.' };
    let res;
    try {
      res = await this.options.api.mark(at.block, at.start, at.end, name);
    } catch (err) {
      return { error: err.detail || 'This cannot be marked.' };
    }
    const now = segments(this.view.state).find((s) => s.id === at.block);
    if (!now || normalize(now.text) !== at.md) return { error: 'The text changed meanwhile. Try again.' };
    const from = now.body[0] + at.shift;
    this.view.dispatch({
      changes: { from, to: from + at.md.length, insert: res.md },
      selection: { anchor: from + res.start, head: from + res.end },
      annotations: [system.of(true), Transaction.addToHistory.of(false)],
    });
    this.stored = this.stored.map((b) => (b.id === at.block ? { ...b, md: normalize(res.md) } : b));
    const uses = this.partUses.get(at.block) || new Map();
    if (!uses.has(res.part)) uses.set(res.part, []);
    this.partUses.set(at.block, uses);
    return res;
  }

  /**
   * copySelection gives the selection as a copy (ADR-22): plain text with no tie. Inside one block the server closes the
   * formatting open at its edges; over several blocks it is the Markdown as it stands, without markers.
   */
  async copySelection() {
    const at = this.selectionInBlock();
    const sel = this.view.state.selection.main;
    const plainText = () => stripParts(this.view.state.doc.sliceString(sel.from, sel.to)).split('\n').filter((l) => !MARKER.test(l)).join('\n');
    if (!at || !this.options.api.excerpt) return plainText();
    await this.save();
    const now = this.selectionInBlock();
    const stored = now && this.stored.find((b) => b.id === now.block);
    if (!now || !stored || stored.md !== now.md) return plainText();
    try {
      return await this.options.api.excerpt(now.block, now.start, now.end);
    } catch (err) {
      return plainText();
    }
  }

  /** copyOrigin is where the selection comes from, for Copy of the marked text (T-41): {page, blocks}, or null. */
  copyOrigin() {
    const sel = this.view.state.selection.main;
    if (sel.empty || !this.options.page) return null;
    const blocks = this.copiedBlocks(sel.from, sel.to);
    return blocks.length ? { page: this.options.page, blocks } : null;
  }

  /** answer deletes a block used elsewhere, or puts its marker back where it was. */
  answer(id, remove) {
    if (remove) {
      this.confirmed.add(id);
      this.save();
      return;
    }
    const state = this.view.state;
    const b = this.stored.find((x) => x.id === id);
    const lost = state.field(markers).lost.get(id);
    if (!b || lost === undefined) return;
    const doc = state.doc;
    const pos = Math.min(lost, doc.length);
    const line = doc.lineAt(pos);
    let insert;
    let at;
    if (pos === line.from) {
      at = pos;
      const rest = doc.sliceString(pos, Math.min(doc.length, pos + b.md.length + 1));
      insert = markerLine(id) + '\n' + (normalize(rest).startsWith(b.md) && b.md ? '' : b.md + '\n\n');
    } else {
      at = line.to;
      insert = '\n\n' + markerLine(id) + '\n' + b.md;
    }
    this.view.dispatch({ changes: { from: at, insert }, effects: setAsks.of(state.field(asking).filter((x) => x !== id)), userEvent: 'input' });
    this.view.focus();
  }

  // ---- What the workspace calls.

  focus() {
    this.view.focus();
  }

  get hasFocus() {
    return this.view.hasFocus;
  }

  /** insert puts text where the cursor is, such as the reference to a new password (ADR-7). */
  insert(text) {
    insertText(this.view, text);
  }
  /** setAlign shows a block aligned center or right, or left with ''. */
  setAlign(id, align) {
    if (align) this.aligns.set(id, align);
    else this.aligns.delete(id);
    this.refresh();
  }

  /** refresh draws the handles again, after the workspace changed what they show. */
  refresh() {
    this.view.requestMeasure();
    this.view.dispatch({ effects: refresh.of(null) });
  }

  format(kind) {
    format(this.view, kind);
    this.view.focus();
  }

  turnInto(kind) {
    turnLines(this.view, kind);
    this.view.focus();
  }

  kind() {
    return kindAt(this.view.state);
  }

  /** table uses a table tool (tables.TOOLS) at the cursor; false when the cursor is in no table or the tool does not apply. */
  table(tool) {
    const done = tableTool(this.view, tool);
    this.view.focus();
    return done;
  }

  /** inTable tells whether the cursor stands in a table. */
  get inTable() {
    return !!tableAt(this.view.state);
  }

  /** focusBlock puts the cursor at the start of a block's text. */
  focusBlock(id) {
    const m = this.view.state.field(markers).list.find((x) => x.id === id);
    if (!m) return;
    const line = this.view.state.doc.lineAt(m.from);
    const at = Math.min(line.to + 1, this.view.state.doc.length);
    this.view.dispatch({ selection: { anchor: at }, scrollIntoView: true });
    this.view.focus();
  }

  /** moveBlock swaps a block with the one above or below it, in the text; saving moves it. The cursor goes along. */
  moveBlock(id, dir) {
    const segs = segments(this.view.state).filter((s) => s.id);
    const k = segs.findIndex((s) => s.id === id);
    if (k < 0 || (dir === 'up' ? k < 1 : k + 1 >= segs.length)) return;
    const after = dir === 'up' ? (k > 1 ? segs[k - 2].id : '') : segs[k + 1].id;
    this.moveBlockTo(id, after, { follow: true });
  }

  /**
   * moveBlockTo puts a block's text with its marker after another block ('' for first), in the text; saving moves it,
   * with the same ID and the same text. The selection stays where it was in the text of its block; with follow, a cursor
   * outside the moved block goes to its start, and the view follows.
   */
  moveBlockTo(id, after, { follow = false } = {}) {
    const state = this.view.state;
    const segs = segments(state).filter((s) => s.id);
    const k = segs.findIndex((s) => s.id === id);
    if (k < 0 || after === id || (after && !segs.some((s) => s.id === after))) return false;
    const order = segs.filter((s) => s.id !== id);
    order.splice(after ? order.findIndex((s) => s.id === after) + 1 : 0, 0, segs[k]);
    let lo = 0;
    while (lo < segs.length && order[lo] === segs[lo]) lo++;
    if (lo === segs.length) return false;
    let hi = segs.length - 1;
    while (order[hi] === segs[hi]) hi--;
    // Only the blocks from the first to the last that change places are written again, each as marker and text.
    const body = (s) => state.sliceDoc(s.body[0], s.body[1]).replace(/\s+$/, '');
    const from = segs[lo].line[0];
    const to = segs[hi].body[0] + body(segs[hi]).length;
    const start = new Map();
    let insert = '';
    for (let n = lo; n <= hi; n++) {
      if (n > lo) insert += '\n\n';
      start.set(order[n], from + insert.length + markerLine(order[n].id).length + 1);
      insert += markerLine(order[n].id) + '\n' + body(order[n]);
    }
    const changes = state.changes({ from, to, insert });
    const map = (pos) => {
      for (let n = lo; n <= hi; n++) {
        const s = segs[n];
        if (pos >= s.line[0] && pos <= Math.min(s.body[1], to)) return start.get(s) + Math.min(Math.max(0, pos - s.body[0]), body(s).length);
      }
      return changes.mapPos(pos);
    };
    const sel = state.selection.main;
    const inside = sel.head >= segs[k].line[0] && sel.head <= segs[k].body[1];
    const selection = follow && !inside ? { anchor: start.get(segs[k]) } : { anchor: map(sel.anchor), head: map(sel.head) };
    this.view.dispatch({ changes, selection, scrollIntoView: follow, userEvent: 'move' });
    if (follow) this.view.focus();
    this.save();
    return true;
  }

  // ---- Dragging a block by its handle (the workspace's blockdrag.js follows the pointer).

  /** blockRects tells where each block stands on the screen, from its marker to the end of its last line, in the order of
   * the text. A line that carries block widgets (the gap of a held block before a marker) is one line block made of
   * parts: a block starts at the part of its marker, after such a gap, and ends before it. */
  blockRects() {
    const view = this.view;
    const list = view.state.field(markers).list;
    const top = view.documentTop;
    const parts = (pos) => {
      const b = view.lineBlockAt(pos);
      return Array.isArray(b.type) ? b.type : [b];
    };
    const start = (pos) => {
      const all = parts(pos);
      return (all.find((p) => p.widget instanceof MarkerWidget) || all[0]).top;
    };
    const end = (pos) => {
      const all = parts(pos).filter((p) => !(p.widget instanceof GapWidget));
      return all.length ? all[all.length - 1].bottom : view.lineBlockAt(pos).bottom;
    };
    return list.map((m, k) => ({
      id: m.id,
      top: top + start(m.from),
      bot: top + end(k + 1 < list.length ? list[k + 1].from - 1 : view.state.doc.length),
    }));
  }

  /** dragShow holds block id and opens the gap before the k-th of the other blocks (at the end for k past them). */
  dragShow(id, k) {
    const state = this.view.state;
    const others = state.field(markers).list.filter((m) => m.id !== id);
    const before = k < others.length ? others[k].id : null;
    const now = state.field(dragging);
    if (now && now.id === id && now.before === before) return;
    const drag = now && now.id === id ? { ...now, before } : { id, before, look: this.lookOf(id), height: this.heightOf(id) };
    this.view.dispatch({ effects: setDrag.of(drag) });
  }

  /** dragEnd lets go: with after (an ID, or '' for first) the block moves there, otherwise it goes back. */
  dragEnd(after) {
    const drag = this.view.state.field(dragging);
    if (!drag) return;
    this.view.dispatch({ effects: setDrag.of(null) });
    if (after !== null && after !== undefined) this.moveBlockTo(drag.id, after);
  }

  /** lookOf copies the lines of a block as the editor shows them, for the gap. */
  lookOf(id) {
    const out = [];
    const marker = this.view.contentDOM.querySelector(`.cm-ud-marker[data-edb="${id}"]`);
    for (let n = marker && marker.nextElementSibling; n && !n.classList.contains('cm-ud-marker'); n = n.nextElementSibling) {
      if (n.classList.contains('cm-line')) out.push(n.cloneNode(true));
    }
    while (out.length > 1 && !out[out.length - 1].textContent.trim()) out.pop();
    return out;
  }

  heightOf(id) {
    const r = this.blockRects().find((x) => x.id === id);
    return r ? r.bot - r.top : 0;
  }

  /** deleteBlock takes a block out of the text; saving deletes it, after asking when it is used elsewhere. */
  deleteBlock(id) {
    const state = this.view.state;
    const segs = segments(state);
    const k = segs.findIndex((s) => s.id === id);
    if (k < 0) return;
    const s = segs[k];
    const next = segs[k + 1];
    let from = s.line[0];
    const to = next ? next.line ? next.line[0] : next.body[0] : state.doc.length;
    if (!next && from > 0) from = Math.max(0, state.doc.sliceString(0, from).replace(/\s+$/, '').length);
    this.view.dispatch({ changes: { from, to }, userEvent: 'delete' });
    this.view.focus();
    this.save();
  }

  /** text is the page as the editor has it, markers included. */
  text() {
    return this.view.state.doc.toString();
  }

  /** dirty tells whether something is not saved yet. */
  get dirty() {
    return !!this.saving || !idle(plan(this.stored, segments(this.view.state), this.confirmed));
  }

  /** flush saves now, for leaving the page. */
  flush() {
    return this.save();
  }

  /** discard closes the editor without saving, after the page changed elsewhere. */
  discard() {
    clearTimeout(this.timer);
    this.destroyed = true;
    this.view.destroy();
  }

  destroy() {
    this.flushing = true;
    // Closed while a value that looks like a password waits for an answer: the value is left out, everything else is
    // saved. It never leaves the browser (ADR-7, 2026-11-20).
    const held = this.plainNew(this.view.state).filter((f) => !f.partial);
    if (held.length) {
      this.view.dispatch({ changes: held.map((h) => ({ from: h.from, to: h.to, insert: '…' })), effects: setPlain.of([]) });
      this.emit('hint', 'A value that looked like a password was left out of the page, not saved. Use Password to keep one sealed.');
    }
    const done = this.save();
    this.destroyed = true;
    Promise.resolve(done).finally(() => this.view.destroy());
    return done;
  }
}

window.UnderDocEditor = {
  create: (options) => new PageEditor(options),
  // The clipboard format of the origin of a copy (T-41), for copies made outside the editor.
  ORIGIN,
  // For the tests and the documentation: the pure parts.
  joinBlocks,
  markerLine,
  imageLine,
};

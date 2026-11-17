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
import { LanguageSupport, syntaxHighlighting, syntaxTree } from '@codemirror/language';
import { classHighlighter } from '@lezer/highlight';
import { MARKER, markerLine, normalize, joinBlocks, plan, idle, kindOf, turnInto, embedFor, EMBED } from './blocks.js';
import { run, Conflict } from './save.js';
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

// ---- The markers of the blocks. Each is a line `<!-- block bN -->` the editor keeps track of: written at the start,
// written by saving, or typed, pasted or brought back by undo as a whole line. A marker whose line changed is gone.
// lost keeps where a marker was last, so a block used elsewhere can be asked about there and put back.
const refresh = StateEffect.define();
const setHover = StateEffect.define();
const setAsks = StateEffect.define();

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
    const box = document.createElement('div');
    box.className = 'cm-ud-marker';
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

// The label of a block used elsewhere, at the end of its first line while the cursor is in it (ADR-22).
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
    const label = document.createElement('span');
    label.className = 'cm-ud-usedlabel lab';
    label.textContent = `Used in ${this.count} ${this.count === 1 ? 'place' : 'places'}`;
    label.title = `Changes here show there too: ${this.where}`;
    return label;
  }
  ignoreEvent() {
    return true;
  }
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
    const box = document.createElement('div');
    box.className = 'cm-ud-ask note';
    box.setAttribute('role', 'alert');
    box.dataset.ask = this.id;
    const text = document.createElement('span');
    text.textContent = `Block #${this.id} is used in ${this.used}. Delete it there too? `;
    const yes = document.createElement('button');
    yes.className = 'chip';
    yes.dataset.answer = 'delete';
    yes.textContent = 'Delete';
    const no = document.createElement('button');
    no.className = 'chip';
    no.dataset.answer = 'keep';
    no.textContent = 'Keep the block';
    box.append(text, yes, ' ', no);
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
    // The CSS caps the gap at its first lines (6.5em).
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
    span.className = 'cm-lp-bullet-dot';
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
    box.className = 'cm-lp-checkbox';
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
      span.className = 'cm-lp-image-alt';
      span.textContent = this.alt || this.src;
      return span;
    }
    const img = document.createElement('img');
    img.className = 'cm-lp-image';
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
    hr.className = 'cm-lp-hr-line';
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
    const box = document.createElement('span');
    box.className = 'cm-ud-embed';
    box.dataset.ref = this.ref;
    const what = document.createElement('span');
    what.className = 'cm-ud-embed-what';
    what.textContent = 'Embeds';
    const link = document.createElement('a');
    link.href = this.href;
    link.textContent = this.label;
    box.append(what, ' ', link);
    return box;
  }
  ignoreEvent(event) {
    return !!(event.target && event.target.closest && event.target.closest('a'));
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
    const align = cellsOf(lines[1] || '').map((d) => (/^:-+:$/.test(d) ? 'center' : /-:$/.test(d) ? 'right' : 'left'));
    const table = document.createElement('table');
    table.className = 'cm-lp-table-widget';
    lines.forEach((line, index) => {
      if (index === 1) return;
      const row = document.createElement('tr');
      cellsOf(line).forEach((text, column) => {
        const cell = document.createElement(index === 0 ? 'th' : 'td');
        cell.className = `cm-lp-align-${align[column] || 'left'}`;
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
      view.dispatch({ selection: { anchor: view.posAtDOM(table) + tables.cellStart(this.source, row, col) } });
      view.focus();
    });
    return table;
  }
  ignoreEvent() {
    return false;
  }
}

// ---- The decorations

const hide = Decoration.replace({});
const lineClass = (cls) => Decoration.line({ class: cls });
const mark = (cls) => Decoration.mark({ class: cls });

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
  const lineDeco = (from, to, cls) => {
    for (let n = doc.lineAt(from).number; n <= doc.lineAt(to).number; n++) {
      if (markerLines.has(n)) continue;
      const l = doc.line(n);
      add(l.from, l.from, lineClass(cls));
    }
  };
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
    for (let n = first; n <= last; n++) {
      const l = doc.line(n);
      add(l.from, l.from, lineClass('cm-ud-used'));
      if (n === first) add(l.from, l.from, lineClass('cm-ud-used-first'));
      if (n === last) add(l.from, l.from, lineClass('cm-ud-used-last'));
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
      if (!isActive(l.from) && !inCode(l.from)) add(l.from, l.from, lineClass('cm-ud-sep'));
      continue;
    }
    const embed = EMBED.exec(l.text);
    if (embed) {
      add(l.from, l.from, lineClass('cm-lp-directive'));
      if (!isActive(l.from)) {
        const ref = embed[1] + (embed[2] ? '#' + embed[2] : '');
        const said = (cfg.embed && cfg.embed(ref)) || {};
        add(l.from, l.to, Decoration.replace({ widget: new EmbedWidget(ref, said.label || ref, said.href || `/o/${embed[1]}${embed[2] ? '#' + embed[2] : ''}`) }));
        continue;
      }
      add(l.from, l.to, mark('cm-lp-directive-chip'));
    } else if (/^::[a-z]+\{.*\}\s*$/.test(l.text)) {
      add(l.from, l.from, lineClass('cm-lp-directive'));
      add(l.from, l.to, mark('cm-lp-directive-chip'));
    }
    for (const match of l.text.matchAll(/\[\[([^\]\n]+)\]\]/g)) {
      const from = l.from + match.index;
      const to = from + match[0].length;
      wiki.push([from, to]);
      add(from + 2, to - 2, mark('cm-lp-wiki'));
      if (!isActive(from)) {
        add(from, from + 2, hide);
        add(to - 2, to, hide);
      }
    }
  }
  const inWiki = (pos) => wiki.some(([from, to]) => pos >= from && pos < to);

  syntaxTree(state).iterate({
    enter(node) {
      const { from, to } = node;
      const name = node.name;
      if (name === 'HTMLBlock' && onMarker(from, to)) return false;
      let match;
      if ((match = /^ATXHeading(\d)$/.exec(name))) {
        lineDeco(from, from, `cm-lp-h${match[1]}`);
        if (!isActive(from)) {
          for (let child = node.node.firstChild; child; child = child.nextSibling) {
            if (child.name === 'HeaderMark') hideMark(child.from, child.to, child.from === from);
          }
        }
        return;
      }
      if ((match = /^SetextHeading(\d)$/.exec(name))) {
        lineDeco(from, from, `cm-lp-h${match[1]}`);
        return;
      }
      switch (name) {
        case 'Emphasis':
        case 'StrongEmphasis':
        case 'Strikethrough':
        case 'InlineCode': {
          const cls = { Emphasis: 'cm-lp-em', StrongEmphasis: 'cm-lp-strong', Strikethrough: 'cm-lp-strike', InlineCode: 'cm-lp-icode' }[name];
          add(from, to, mark(cls));
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
          add(marks2[0].to, marks2[1].from, mark('cm-lp-link'));
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
          add(from, to, mark('cm-lp-image-source'));
          return false;
        }
        case 'URL':
          add(from, to, mark('cm-lp-url'));
          return;
        case 'Blockquote':
          lineDeco(from, to, 'cm-lp-quote');
          return;
        case 'QuoteMark':
          if (!isActive(from)) hideMark(from, to, true);
          return;
        case 'ListItem': {
          const list = node.node.parent && node.node.parent.name;
          const task = node.node.getChild('Task');
          lineDeco(from, from, task ? 'cm-lp-task' : list === 'OrderedList' ? 'cm-lp-ordered' : 'cm-lp-bullet');
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
          if (checked) add(to, doc.lineAt(to).to, mark('cm-lp-done'));
          return;
        }
        case 'FencedCode': {
          lineDeco(from, to, 'cm-lp-code');
          lineDeco(from, from, 'cm-lp-code-first');
          lineDeco(to, to, 'cm-lp-code-last');
          if (!isActive(from, to)) {
            for (let child = node.node.firstChild; child; child = child.nextSibling) {
              if (child.name === 'CodeMark') hideMark(child.from, child.to, false);
              if (child.name === 'CodeInfo') add(child.from, child.to, mark('cm-lp-code-info'));
            }
          }
          return false;
        }
        case 'CodeBlock':
          lineDeco(from, to, 'cm-lp-code');
          return false;
        case 'HorizontalRule':
          lineDeco(from, from, 'cm-lp-hr');
          if (!isActive(from)) replace(from, to, Decoration.replace({ widget: new RuleWidget() }));
          return;
        case 'Table':
          if (!isActive(from, to) && !onMarker(from, to)) {
            add(from, to, Decoration.replace({ widget: new TableWidget(doc.sliceString(from, to)), block: true }));
          } else {
            lineDeco(from, to, 'cm-lp-table-source');
          }
          return false;
        default:
          return;
      }
    },
  });
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
    const effects = tr.effects.some((e) => e.is(setFocus) || e.is(refresh) || e.is(setHover) || e.is(setAsks) || e.is(setDrag));
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
  const dom = document.createElement('div');
  dom.className = 'cm-ud-slashbox';
  const draw = () => {
    const value = view.state.field(slash);
    if (!value) return;
    const items = slashItems(view.state);
    const all = view.state.field(host).slash || [];
    const box = document.createElement('div');
    box.className = 'bslash';
    box.setAttribute('role', 'listbox');
    box.setAttribute('aria-label', 'Add a block');
    if (!items.length) {
      const none = document.createElement('div');
      none.className = 'lab';
      none.textContent = 'Nothing fits.';
      box.append(none);
    }
    items.forEach((x, n) => {
      const b = document.createElement('button');
      b.setAttribute('role', 'option');
      b.dataset.k = String(all.indexOf(x));
      b.setAttribute('aria-selected', String(n === value.cur));
      if (x.later) {
        b.disabled = true;
        b.title = x.later;
      } else if (x.hint) b.title = x.hint;
      b.innerHTML = `${x.icon || ''}<span></span>${x.later ? '<small>later</small>' : ''}`;
      b.querySelector('span').textContent = x.n;
      box.append(b);
    });
    dom.replaceChildren(box);
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
    if (line.text.trim() !== '') return false;
    const embed = embedFor((event.clipboardData || window.clipboardData).getData('text'));
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

// ---- Calling back into the workspace.
const callbacks = new WeakMap();
function call(view, name, ...args) {
  const on = callbacks.get(view);
  if (on && on[name]) return on[name](...args);
  return undefined;
}

// ---- The editor of one page.

class PageEditor {
  constructor(options) {
    this.options = options;
    this.stored = options.blocks.map((b) => ({ id: b.id, md: normalize(b.md), usedIn: b.usedIn || [] }));
    // The alignment of each block: not part of its Markdown, the workspace sets it through the API (setAlign).
    this.aligns = new Map(options.blocks.filter((b) => b.align).map((b) => [b.id, b.align]));
    this.confirmed = new Set();
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
      usedIn: (id) => {
        const b = this.stored.find((x) => x.id === id);
        return b && b.usedIn.length ? b.usedIn.join(', ') : '';
      },
      align: (id) => this.aligns.get(id) || '',
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
          hovered,
          dragging,
          focused,
          trackFocus,
          history(),
          drawSelection(),
          EditorView.lineWrapping,
          EditorView.editorAttributes.of({ class: 'cm-ud' }),
          EditorView.contentAttributes.of({ 'aria-label': options.label || 'Page, Markdown' }),
          new LanguageSupport(markdownLanguage),
          syntaxHighlighting(classHighlighter),
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
          pasteAddress,
          atomicMarkers,
          keepOutOfMarkers,
          livePreview,
          EditorView.updateListener.of((u) => this.updated(u)),
        ],
      }),
    });
    callbacks.set(this.view, options.on || {});
    this.view.dom.addEventListener('mousedown', (e) => {
      if (e.target.closest && e.target.closest('.cm-ud-ask button, .cm-ud-marker button')) e.preventDefault();
    });
    this.view.dom.addEventListener('click', (e) => {
      const b = e.target.closest && e.target.closest('.cm-ud-ask button[data-answer]');
      if (b) this.answer(b.closest('[data-ask]').dataset.ask, b.dataset.answer === 'delete');
    });
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
      const inTable = !!tableAt(u.state);
      if (inTable !== this.lastTable) {
        this.lastTable = inTable;
        this.emit('table', inTable);
      }
    }
    if (u.focusChanged && !u.view.hasFocus) this.schedule(0);
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
    const p = plan(this.stored, segments(state), this.confirmed);
    this.ask(p.asks);
    if (idle(p)) {
      if (!p.asks.length) this.emit('status', this.everSaved ? 'Saved' : '');
      return;
    }
    this.emit('status', 'Saving…');
    this.during = null;
    this.saving = (async () => {
      try {
        const done = await run(p, this.stored, this.options.api, this.options.on && this.options.on.saved);
        this.stored = done.stored;
        this.place(done.made);
        this.everSaved = true;
        this.emit('status', 'Saved');
      } catch (err) {
        if (err.stored) this.stored = err.stored;
        if (err.made) this.place(err.made);
        if (err instanceof Conflict) {
          this.emit('conflict', this.view.state.doc.toString());
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
    const done = this.save();
    this.destroyed = true;
    Promise.resolve(done).finally(() => this.view.destroy());
    return done;
  }
}

window.UnderDocEditor = {
  create: (options) => new PageEditor(options),
  // For the tests and the documentation: the pure parts.
  joinBlocks,
  markerLine,
};

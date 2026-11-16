// SPDX-License-Identifier: MIT

// Candidate 3: CodeMirror 6 with a live preview after Obsidian and Typora. The text stays Markdown; away from the cursor
// the markers of headings, emphasis, code, links, lists, quotes and rules are hidden and the line looks rendered, and a
// table or an image away from the cursor shows as such. On the lines the selection touches, the Markdown shows.
// The decorations are UnderDoc's own: no MIT package does this well enough.
import { EditorState, StateEffect, StateField } from '@codemirror/state';
import { Decoration, EditorView, WidgetType, keymap, drawSelection } from '@codemirror/view';
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands';
import { markdownLanguage, markdownKeymap } from '@codemirror/lang-markdown';
import { LanguageSupport, syntaxHighlighting, syntaxTree } from '@codemirror/language';
import { classHighlighter } from '@lezer/highlight';
import './codemirror.css';

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

// ---- Widgets

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
    box.addEventListener('mousedown', (event) => {
      event.preventDefault();
      const pos = view.posAtDOM(box);
      const text = view.state.sliceDoc(pos, pos + 3);
      if (/^\[[ xX]\]$/.test(text)) {
        view.dispatch({ changes: { from: pos + 1, to: pos + 2, insert: this.checked ? ' ' : 'x' } });
      }
    });
    return box;
  }
  ignoreEvent() {
    return true;
  }
}

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
  toDOM() {
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
    return table;
  }
  ignoreEvent() {
    return false;
  }
}

// ---- The decorations

const hide = Decoration.replace({});
const line = (cls) => Decoration.line({ class: cls });
const mark = (cls) => Decoration.mark({ class: cls });

function decorate(state) {
  const showAll = !state.field(focused);
  const doc = state.doc;
  // The lines the selection touches show their Markdown.
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
  const out = [];
  const add = (from, to, deco) => out.push(deco.range(from, to));
  const lineDeco = (from, to, cls) => {
    for (let n = doc.lineAt(from).number; n <= doc.lineAt(to).number; n++) {
      const l = doc.line(n);
      add(l.from, l.from, line(cls));
    }
  };
  // hideMark hides a marker and, for block markers, the space after it.
  const hideMark = (from, to, withSpace) => {
    if (withSpace && doc.sliceString(to, to + 1) === ' ') to++;
    if (to > from) add(from, to, hide);
  };
  const wiki = [];

  // Wiki links and directives are UnderDoc's own syntax, which the Markdown parser does not know.
  for (let n = 1; n <= doc.lines; n++) {
    const l = doc.line(n);
    if (/^::[a-z]+\{.*\}\s*$/.test(l.text)) {
      add(l.from, l.from, line('cm-lp-directive'));
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
          const marks = [];
          let url = null;
          for (let child = node.node.firstChild; child; child = child.nextSibling) {
            if (child.name === 'LinkMark') marks.push(child);
            if (child.name === 'URL') url = child;
          }
          if (marks.length < 2) return;
          add(marks[0].to, marks[1].from, mark('cm-lp-link'));
          if (url && !isActive(from, to)) {
            add(from, marks[0].to, hide);
            add(marks[1].from, to, hide);
          }
          return;
        }
        case 'Image': {
          const text = doc.sliceString(from, to);
          const parts = /^!\[([^\]]*)\]\(([^)\s]+)/.exec(text);
          if (parts && !isActive(from, to)) {
            add(from, to, Decoration.replace({ widget: new ImageWidget(parts[2], parts[1]) }));
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
            else if (list === 'BulletList') add(listMark.from, listMark.to, Decoration.replace({ widget: new BulletWidget() }));
          }
          return;
        }
        case 'TaskMarker': {
          const checked = /x/i.test(doc.sliceString(from, to));
          if (!isActive(from)) add(from, to, Decoration.replace({ widget: new CheckboxWidget(checked) }));
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
          if (!isActive(from)) add(from, to, Decoration.replace({ widget: new RuleWidget() }));
          return;
        case 'Table':
          if (!isActive(from, to)) {
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
  return Decoration.set(out, true);
}

const livePreview = StateField.define({
  create: (state) => decorate(state),
  update(value, tr) {
    const focusChanged = tr.effects.some((e) => e.is(setFocus));
    if (tr.docChanged || tr.selection || focusChanged || syntaxTree(tr.state) !== syntaxTree(tr.startState)) {
      return decorate(tr.state);
    }
    return value;
  },
  provide: (field) => EditorView.decorations.from(field),
});

// A to-do typed on its own line becomes an item of a list, as in the other editors: "[ ] " turns into "- [ ] ".
const todoShortcut = EditorView.inputHandler.of((view, from, to, text) => {
  if (text !== ' ' || from !== to) return false;
  const line = view.state.doc.lineAt(from);
  const match = /^(\s*)\[([ xX]?)\]$/.exec(view.state.sliceDoc(line.from, from));
  if (!match) return false;
  const insert = `${match[1]}- [${match[2] || ' '}] `;
  view.dispatch({ changes: { from: line.from, to, insert }, selection: { anchor: line.from + insert.length }, userEvent: 'input.type' });
  return true;
});

// The editor lives in a shadow root. CodeMirror adds its base styles with style-mod, which writes a <style> element into
// a document (blocked by style-src 'self') but uses a constructed style sheet in a shadow root, which the CSP allows.
// The own styles come in as a stylesheet link inside the shadow root.
async function shadowFor(host) {
  const box = document.createElement('div');
  box.className = 'cm-trial-host';
  host.append(box);
  const root = box.attachShadow({ mode: 'open' });
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  const href = new URL('./codemirror.css', import.meta.url);
  href.search = new URL(import.meta.url).search;
  link.href = href.href;
  const loaded = new Promise((resolve) => {
    link.addEventListener('load', resolve);
    link.addEventListener('error', resolve);
  });
  root.append(link);
  await loaded;
  return root;
}

window.editorTrial.register('codemirror', async (host, text) => {
  const root = await shadowFor(host);
  const view = new EditorView({
    parent: root,
    root,
    state: EditorState.create({
      doc: text,
      extensions: [
        focused,
        trackFocus,
        history(),
        drawSelection(),
        EditorView.lineWrapping,
        EditorView.editorAttributes.of({ class: 'cm-trial' }),
        // Markdown with GFM, without markdown(): that would also bundle HTML, CSS and JavaScript for embedded HTML.
        new LanguageSupport(markdownLanguage),
        syntaxHighlighting(classHighlighter),
        keymap.of([...markdownKeymap, ...defaultKeymap, ...historyKeymap, indentWithTab]),
        todoShortcut,
        livePreview,
      ],
    }),
  });
  return {
    getMarkdown: () => view.state.doc.toString(),
    reset(next) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: next }, selection: { anchor: next.length } });
      view.focus();
    },
    focus: () => view.focus(),
    scope: view.contentDOM,
  };
});

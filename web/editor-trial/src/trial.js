// SPDX-License-Identifier: MIT

// The trial page: mounts every registered editor in its column with the same fixture, shows what it gives back, a line
// diff against the fixture, and a note on version, licence and size. It also records CSP violations and errors per
// bundle, so the browser test can tell which editor broke what.
import './trial.css';
import { fixture } from './fixture.js';

const violations = [];
const errors = [];
const editors = {};

const bundleOf = (url) => (String(url || '').match(/editor-trial\/(\w+)(?:\.js|-chunks\/)/) || [])[1] || 'page';

document.addEventListener('securitypolicyviolation', (e) => {
  violations.push({
    bundle: bundleOf(e.sourceFile),
    directive: e.effectiveDirective || e.violatedDirective,
    blocked: e.blockedURI,
    source: e.sourceFile ? `${e.sourceFile.replace(/^.*\/editor-trial\//, '').replace(/\?.*$/, '')}:${e.lineNumber}:${e.columnNumber}` : '',
    sample: e.sample || '',
  });
});
window.addEventListener('error', (e) => {
  errors.push({ bundle: bundleOf(e.filename), message: String(e.message) });
});
window.addEventListener('unhandledrejection', (e) => {
  errors.push({ bundle: 'promise', message: String(e.reason && (e.reason.stack || e.reason.message) || e.reason) });
});

// lineDiff returns the lines that differ, "- " for the fixture and "+ " for the editor, after a longest common
// subsequence of lines. A trailing newline at the very end is ignored.
export function lineDiff(before, after) {
  const a = before.replace(/\n+$/, '').split('\n');
  const b = after.replace(/\n+$/, '').split('\n');
  const n = a.length;
  const m = b.length;
  const lcs = Array.from({ length: n + 1 }, () => new Uint16Array(m + 1));
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      lcs[i][j] = a[i] === b[j] ? lcs[i + 1][j + 1] + 1 : Math.max(lcs[i + 1][j], lcs[i][j + 1]);
    }
  }
  const out = [];
  let i = 0;
  let j = 0;
  while (i < n || j < m) {
    if (i < n && j < m && a[i] === b[j]) {
      i++;
      j++;
    } else if (j < m && (i === n || lcs[i][j + 1] >= lcs[i + 1][j])) {
      out.push(`+ ${b[j++]}`);
    } else {
      out.push(`- ${a[i++]}`);
    }
  }
  return out;
}

function roundTrip(markdown) {
  const diff = lineDiff(fixture, markdown);
  return { identical: diff.length === 0, diff, markdown };
}

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

function showDiff(target, diff) {
  target.replaceChildren();
  if (diff.length === 0) {
    target.append(element('span', 'et-same', '✅ identical to the fixture'));
    return;
  }
  target.append(element('span', 'et-changed', `❌ ${diff.length} differing lines`));
  for (const line of diff) {
    target.append(element('span', line.startsWith('+') ? 'et-add' : 'et-del', line));
  }
}

async function mount(name, factory) {
  const column = document.querySelector(`[data-editor="${name}"]`);
  if (!column) return;
  const host = column.querySelector('[data-host]');
  const out = column.querySelector('[data-out-text]');
  const diff = column.querySelector('[data-diff]');
  column.dataset.state = 'mounting';
  try {
    const editor = await factory(host, fixture);
    // Wait two frames, so that editors that settle their document after mounting have done so.
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
    editor.initial = roundTrip(editor.getMarkdown());
    editors[name] = editor;
    column.dataset.state = 'ready';
    column.querySelector('[data-out]').addEventListener('click', () => {
      const current = editor.getMarkdown();
      out.textContent = current;
      out.hidden = false;
      showDiff(diff, lineDiff(fixture, current));
      diff.hidden = false;
    });
  } catch (error) {
    column.dataset.state = 'failed';
    host.append(element('p', 'et-failed', `❌ Mounting failed: ${error && error.message}`));
    errors.push({ bundle: name, message: `mount: ${error && (error.stack || error.message)}` });
  }
}

const queue = [];
let ready = document.readyState !== 'loading';
document.addEventListener('DOMContentLoaded', () => {
  ready = true;
  for (const [name, factory] of queue.splice(0)) mount(name, factory);
  notes();
});

async function notes() {
  const script = document.querySelector('script[src*="editor-trial/trial.js"]');
  const version = script ? new URL(script.src).searchParams.get('v') : '';
  let meta;
  try {
    meta = await (await fetch(`/assets/app/editor-trial/meta.json?v=${version}`)).json();
  } catch {
    return;
  }
  for (const column of document.querySelectorAll('[data-editor]')) {
    const name = column.dataset.editor;
    const bundle = meta.bundles[name];
    if (!bundle) continue;
    const kb = (bytes) => `${(bytes / 1024).toFixed(0)} KiB`;
    const wanted = column.dataset.main.split(' ');
    const main = bundle.packages.filter((p) => wanted.includes(p.name));
    const licenses = [...new Set(bundle.packages.map((p) => p.license))].join(', ');
    const lazy = bundle.lazyFiles ? ` · on demand ${bundle.lazyFiles} chunks, gzip ${kb(bundle.lazyGzip)}` : '';
    column.querySelector('[data-note]').replaceChildren(
      element('span', '', main.map((p) => `${p.name} ${p.version}`).join(' · ')),
      element('span', '', `Licences of all ${bundle.packages.length} bundled packages: ${licenses}`),
      element('span', '', `JS gzip ${kb(bundle.jsGzip)} (${kb(bundle.js)}) · CSS gzip ${kb(bundle.cssGzip)}${lazy}`),
    );
  }
}

window.editorTrial = {
  fixture,
  editors,
  violations,
  errors,
  lineDiff,
  roundTrip,
  register(name, factory) {
    if (ready) mount(name, factory);
    else queue.push([name, factory]);
  },
};

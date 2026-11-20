// SPDX-License-Identifier: MIT

// A password typed as text (ADR-7, 2026-11-20): `Router password: hunter2` in a page instead of a password of the page.
// The editor asks before such a value is saved, so it never reaches a block, a version, the search or the log. Without
// CodeMirror, so `node --test` checks it.

// The last words of a label that names a password: password, passwort and kennwort also at the end of a word
// (adminpasswort), the others as words of their own.
const LABEL = /(?:password|passwort|kennwort|(?:^|[\s_-])(?:passphrase|pwd?|pin|secret|geheimnis|token|api[\s_-]?key))$/;

// What stands in for a value instead of being one.
const PLACEHOLDER = /^(?:\*+|•+|\.{2,}|…+|x{3,}|-+|_+|\?+|n\/?a|none|keine?s?|tbd|todo|redacted|hidden|changeme|<[^>]*>|\{\{.*\}\}|\[.*\]|\(.*\))$/i;

const FENCE = /^ {0,3}(`{3,}|~{3,})/;

/** blank keeps the length of a text and puts spaces in place of what is not to be looked at. */
const blank = (s) => s.replace(/[^\n]/g, ' ');

/**
 * label tells whether a key, as it stands before `:` or `=` or in a cell of a table, names a password: at most four
 * words and 40 characters, without the marks of Markdown around it.
 */
export function isLabel(key) {
  const k = key
    .replace(/^\s*(?:(?:[-*+]|\d+[.)])\s+|>\s*)*/, '')
    .replace(/[*_~]/g, '')
    .trim()
    .toLowerCase();
  if (!k || k.length > 40 || k.split(/\s+/).length > 4) return false;
  return LABEL.test(k);
}

/**
 * valueAt finds the value that follows a label in text from start on: one word of 4 to 200 characters, maybe in bold or
 * in inline code. It answers [from, to, partial] within text, or null; partial is a word of 1 to 3 characters, which
 * may be the start of a value still being typed.
 */
function valueAt(text, start, end) {
  let from = start;
  while (from < end && /[\s*_]/.test(text[from])) from++;
  let to = end;
  while (to > from && /[\s*_]/.test(text[to - 1])) to--;
  if (text[from] === '`') {
    const close = text.indexOf('`', from + 1);
    if (close < 0 || close + 1 !== to) return null;
    from++;
    to--;
  }
  const value = text.slice(from, to);
  if (!value || value.length > 200 || /\s|`/.test(value)) return null;
  if (PLACEHOLDER.test(value) || value.startsWith('::secret{') || value.includes('://') || value.startsWith('[')) return null;
  return [from, to, value.length < 4];
}

/**
 * findPlain lists the values in md that look like passwords typed as text: [{from, to, value, partial}], positions
 * within md; partial marks a value of 1 to 3 characters, too short to ask about, which the editor does not save while
 * it is being typed. Fenced and indented code and HTML comments are never looked at, nor a label in inline code; a
 * value in inline code after a label outside it counts.
 */
export function findPlain(md) {
  const out = [];
  let fence = null;
  let comment = false;
  let pos = 0;
  for (const line of md.split('\n')) {
    const at = pos;
    pos += line.length + 1;
    const f = FENCE.exec(line);
    if (fence) {
      if (f && f[1][0] === fence[0] && f[1].length >= fence.length && !line.slice(f[0].length).trim()) fence = null;
      continue;
    }
    if (f) {
      fence = f[1];
      continue;
    }
    if (comment) {
      if (line.includes('-->')) comment = false;
      continue;
    }
    if (/^(?: {4}|\t)/.test(line)) continue;
    // Comments and inline code keep their length but are not read: no label hides in them.
    let seen = line.replace(/<!--.*?-->/g, blank);
    const open = seen.indexOf('<!--');
    if (open >= 0) {
      comment = true;
      seen = seen.slice(0, open) + blank(seen.slice(open));
    }
    const code = seen.replace(/`[^`\n]*`/g, blank);
    const found = (range) => range && out.push({ from: at + range[0], to: at + range[1], value: line.slice(range[0], range[1]), partial: range[2] });
    if (/^\s*\|/.test(code)) {
      // A row of a table: a cell with a label, the value in the next one.
      const cells = [];
      let start = code.indexOf('|') + 1;
      for (let k = start; k <= code.length; k++) {
        if (k === code.length || (code[k] === '|' && code[k - 1] !== '\\')) {
          cells.push([start, k]);
          start = k + 1;
        }
      }
      for (let c = 0; c + 1 < cells.length; c++) {
        if (isLabel(code.slice(cells[c][0], cells[c][1]))) found(valueAt(seen, cells[c + 1][0], cells[c + 1][1]));
      }
      continue;
    }
    const sep = code.search(/[:=]/);
    if (sep <= 0 || !isLabel(code.slice(0, sep))) continue;
    found(valueAt(seen, sep + 1, seen.length));
  }
  return out;
}

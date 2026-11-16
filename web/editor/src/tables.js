// SPDX-License-Identifier: MIT

// The table tools of the editor (M-8): a row or a column inserted or deleted, a column aligned. A table is GitHub
// Markdown: a header row, a row of dashes that holds the alignment, and the rows of the body. Every tool writes the table
// again as plain Markdown, so the round trip stays whole (ADR-21); a table no tool touched stays exactly as it was typed.

/** cells splits a row of a table at the pipes that are not escaped; a cell keeps its escapes. */
export function cells(line) {
  let text = line.trim();
  if (text.startsWith('|')) text = text.slice(1);
  if (text.endsWith('|') && !text.endsWith('\\|')) text = text.slice(0, -1);
  const out = [];
  let cell = '';
  for (let i = 0; i < text.length; i++) {
    if (text[i] === '\\' && i + 1 < text.length) {
      cell += text[i] + text[i + 1];
      i++;
    } else if (text[i] === '|') {
      out.push(cell.trim());
      cell = '';
    } else {
      cell += text[i];
    }
  }
  out.push(cell.trim());
  return out;
}

const alignOf = (dashes) => {
  const d = dashes.trim();
  if (/^:-*:$/.test(d)) return 'center';
  if (/^-*:$/.test(d)) return 'right';
  if (/^:-*$/.test(d)) return 'left';
  return '';
};

const dashesOf = (align) => ({ left: ':---', center: ':---:', right: '---:' })[align] || '---';

/** parse reads a table: {head: [cells], align: [..], rows: [[cells]]}, or null when it is not one. */
export function parse(text) {
  const lines = text.replace(/\n+$/, '').split('\n');
  if (lines.length < 2 || !/^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$/.test(lines[1])) return null;
  const head = cells(lines[0]);
  const align = cells(lines[1]).map(alignOf);
  const width = head.length;
  const fit = (row) => {
    const out = row.slice(0, width);
    while (out.length < width) out.push('');
    return out;
  };
  return { head, align: fit(align), rows: lines.slice(2).map((l) => fit(cells(l))) };
}

/** write writes a table as Markdown, one row per line. */
export function write(t) {
  const row = (r) => `| ${r.join(' | ')} |`;
  return [row(t.head), `|${t.align.map(dashesOf).join('|')}|`, ...t.rows.map(row)].join('\n');
}

/**
 * cellAt names the cell at an offset in the text of a table: {row, col}, where row 0 is the header and rows of the body
 * count from 1. The row of dashes counts as the header.
 */
export function cellAt(text, offset) {
  const before = text.slice(0, offset);
  const line = before.split('\n').length - 1;
  const start = before.lastIndexOf('\n') + 1;
  const upTo = text.slice(start, offset);
  let col = 0;
  for (let i = 0; i < upTo.length; i++) {
    if (upTo[i] === '\\') i++;
    else if (upTo[i] === '|') col++;
  }
  if (/^\s*\|/.test(text.slice(start))) col--;
  return { row: line < 2 ? 0 : line - 1, col: Math.max(0, col) };
}

/** cellStart is the offset where the text of a cell starts in a table as write writes it. */
export function cellStart(text, row, col) {
  const lines = text.split('\n');
  const n = row === 0 ? 0 : row + 1;
  let pos = 0;
  for (let k = 0; k < n && k < lines.length; k++) pos += lines[k].length + 1;
  const line = lines[Math.min(n, lines.length - 1)];
  let c = -1;
  for (let i = 0; i < line.length; i++) {
    if (line[i] === '\\') {
      i++;
      continue;
    }
    if (line[i] === '|' && ++c === col) return pos + i + (line[i + 1] === ' ' ? 2 : 1);
  }
  return pos + line.length;
}

/** The tools, as the bar names them. */
export const TOOLS = ['rowAbove', 'rowBelow', 'colLeft', 'colRight', 'rowDelete', 'colDelete', 'alignLeft', 'alignCenter', 'alignRight'];

/**
 * apply uses a tool on a table at a cell. Returns {text, row, col}: the table written again and the cell for the cursor,
 * or null when the tool does not apply there (the header row cannot go, nor the last column).
 */
export function apply(text, at, tool) {
  const t = parse(text);
  if (!t) return null;
  const width = t.head.length;
  let { row, col } = at;
  col = Math.min(col, width - 1);
  row = Math.min(row, t.rows.length);
  const blank = () => Array(t.head.length).fill('');
  switch (tool) {
    case 'rowAbove':
      // Nothing goes above the header: a row "above" it is the first row of the body.
      if (row === 0) {
        t.rows.unshift(blank());
        row = 1;
      } else t.rows.splice(row - 1, 0, blank());
      break;
    case 'rowBelow':
      t.rows.splice(row, 0, blank());
      row += 1;
      break;
    case 'rowDelete':
      if (row === 0) return null;
      t.rows.splice(row - 1, 1);
      row = Math.min(row, t.rows.length);
      break;
    case 'colLeft':
    case 'colRight': {
      const k = tool === 'colLeft' ? col : col + 1;
      t.head.splice(k, 0, '');
      t.align.splice(k, 0, '');
      for (const r of t.rows) r.splice(k, 0, '');
      col = k;
      break;
    }
    case 'colDelete':
      if (width < 2) return null;
      t.head.splice(col, 1);
      t.align.splice(col, 1);
      for (const r of t.rows) r.splice(col, 1);
      col = Math.min(col, width - 2);
      break;
    case 'alignLeft':
    case 'alignCenter':
    case 'alignRight':
      t.align[col] = tool.slice(5).toLowerCase();
      break;
    default:
      return null;
  }
  const out = write(t);
  return { text: out, row, col };
}

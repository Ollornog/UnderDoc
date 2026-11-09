// SPDX-License-Identifier: MIT

// The page as one text and back (ADR-21): the blocks of a page joined with a marker line before each, the same marker as
// in the export (docs/export.md), and the plan that turns a changed text into calls of the block API. Plain functions
// without CodeMirror, so `node --test` checks them on their own.

/** A marker line, exactly as the export writes it. */
export const MARKER = /^<!-- block (b[1-9][0-9]{0,8}) -->$/;

export const markerLine = (id) => `<!-- block ${id} -->`;

/** normalize drops blank lines at the start and the end of a block: they mean nothing in Markdown (docs/export.md). */
export function normalize(text) {
  return String(text || '')
    .replace(/^(?:[ \t]*\r?\n)+/, '')
    .replace(/(?:\r?\n[ \t]*)+$/, '')
    .replace(/^[ \t]+$/, '');
}

/** joinBlocks writes the page: each block after its marker, a blank line between blocks. */
export function joinBlocks(blocks) {
  return blocks.map((b) => `${markerLine(b.id)}\n${normalize(b.md)}`).join('\n\n');
}

const fenceOf = (line) => /^ {0,3}(`{3,}|~{3,})/.exec(line);

/**
 * chunks returns the parts of a text that blank lines outside fenced code separate, as [from, to) offsets, each without
 * the blank lines around it.
 */
export function chunks(text) {
  const out = [];
  let fence = null;
  let start = -1;
  let end = -1;
  let pos = 0;
  for (const line of text.split('\n')) {
    const lineEnd = pos + line.length;
    if (fence) {
      const close = fenceOf(line);
      if (close && close[1][0] === fence[0] && close[1].length >= fence.length && line.trim() === close[1]) fence = null;
      end = lineEnd;
    } else if (line.trim() === '') {
      if (start >= 0) out.push([start, end]);
      start = -1;
    } else {
      if (start < 0) start = pos;
      end = lineEnd;
      const open = fenceOf(line);
      if (open) fence = open[1];
    }
    pos = lineEnd + 1;
  }
  if (start >= 0) out.push([start, end]);
  return out;
}

/** fencesOpen tells whether a text ends inside fenced code it never closes. */
export function fencesOpen(text) {
  let fence = null;
  for (const line of text.split('\n')) {
    const mark = fenceOf(line);
    if (!mark) continue;
    if (!fence) fence = mark[1];
    else if (mark[1][0] === fence[0] && mark[1].length >= fence.length && line.trim() === mark[1]) fence = null;
  }
  return !!fence;
}

/**
 * around finds the stored text of a block again inside its changed text, as whole parts between blank lines. What
 * stands before or after it was written as paragraphs of their own: they become blocks of their own, and the block
 * keeps its text. Returns null when the stored text is not there as a whole.
 */
export function around(text, stored) {
  const old = normalize(stored);
  if (!old || fencesOpen(old)) return null;
  const parts = chunks(text);
  for (let i = 0; i < parts.length; i++) {
    for (let j = i; j < parts.length; j++) {
      if (normalize(text.slice(parts[i][0], parts[j][1])) !== old) continue;
      if (i === 0 && j === parts.length - 1) return null;
      return {
        keep: [parts[i][0], parts[j][1]],
        before: i > 0 ? [parts[0][0], parts[i - 1][1]] : null,
        after: j < parts.length - 1 ? [parts[j + 1][0], parts[parts.length - 1][1]] : null,
      };
    }
  }
  return null;
}

/**
 * plan compares the text of the editor with the stored blocks.
 *
 * stored:   [{id, md, usedIn}] in the stored order.
 * segments: [{id, adopted, line: [from, to] of the marker line or null, body: [from, to], text}] in the order of the
 *           text; a segment without a marker (text before the first one) has id null and line null.
 * confirmed: IDs used elsewhere whose deletion was confirmed.
 *
 * Returns {sequence, edits, deletes, asks, order, moves}:
 * - sequence: the blocks as the text has them, {old: id} or {new: md, place}, where place says how its marker gets into
 *   the text: {insert: pos} puts a marker line at the start of the line at pos, {replace: [from, to]} rewrites a marker
 *   line, {before: id, line, keep} gives the marker line of block id to the new block and puts the marker of id before
 *   its kept text.
 * - edits: [{id, md}] for changed blocks; deletes: IDs to delete; asks: IDs used elsewhere whose marker vanished.
 * - order: the IDs that stay, in the order of the text.
 */
export function plan(stored, segments, confirmed = new Set()) {
  const byId = new Map(stored.map((b) => [b.id, b]));
  const owner = new Map();
  // An ID that is twice in the text stays with the marker that was there first; a copy gets a new block.
  for (const s of segments) {
    if (!s.id || !byId.has(s.id)) continue;
    const had = owner.get(s.id);
    if (!had || (had.adopted && !s.adopted)) owner.set(s.id, s);
  }
  // The text of a block whose marker is gone, found beside another block, was joined to it: that block changed, and no
  // new block comes of it.
  const joined = new Set(stored.filter((b) => !owner.has(b.id)).map((b) => normalize(b.md)).filter(Boolean));
  const sequence = [];
  const edits = [];
  for (const s of segments) {
    const own = s.id && owner.get(s.id) === s;
    if (!own) {
      if (s.line) sequence.push({ new: normalize(s.text), place: { replace: s.line } });
      else for (const [f, t] of chunks(s.text)) sequence.push({ new: s.text.slice(f, t), place: { insert: s.body[0] + f } });
      continue;
    }
    const old = byId.get(s.id);
    if (normalize(s.text) === normalize(old.md)) {
      sequence.push({ old: s.id });
      continue;
    }
    let split = around(s.text, old.md);
    const part = (r) => r && joined.has(normalize(s.text.slice(r[0], r[1])));
    if (split && (part(split.before) || part(split.after))) split = null;
    if (!split) {
      edits.push({ id: s.id, md: normalize(s.text) });
      sequence.push({ old: s.id });
      continue;
    }
    // Each paragraph written before or after the block becomes a block of its own. The first one before it takes the
    // block's marker, and the block's marker moves down to its kept text.
    const base = s.body[0];
    const inside = (r) => chunks(s.text).filter(([f, t]) => f >= r[0] && t <= r[1]);
    if (split.before) {
      inside(split.before).forEach(([f, t], k) => {
        sequence.push({
          new: s.text.slice(f, t),
          place: k ? { insert: base + f } : { before: s.id, line: s.line, keep: base + split.keep[0] },
        });
      });
    }
    sequence.push({ old: s.id });
    if (split.after) for (const [f, t] of inside(split.after)) sequence.push({ new: s.text.slice(f, t), place: { insert: base + f } });
  }
  const present = new Set(owner.keys());
  const deletes = [];
  const asks = [];
  for (const b of stored) {
    if (present.has(b.id)) continue;
    if ((b.usedIn || []).length && !confirmed.has(b.id)) asks.push(b.id);
    else deletes.push(b.id);
  }
  const order = sequence.filter((e) => e.old).map((e) => e.old);
  return { sequence, edits, deletes, asks, order, moves: moves(stored.map((b) => b.id), order) };
}

/** moves returns the calls {id, after} that bring the stored order to the wanted one. */
export function moves(current, wanted) {
  const cur = current.filter((id) => wanted.includes(id));
  const out = [];
  wanted.forEach((id, k) => {
    if (cur[k] === id) return;
    const after = k ? wanted[k - 1] : '';
    out.push({ id, after });
    cur.splice(cur.indexOf(id), 1);
    cur.splice(k, 0, id);
  });
  return out;
}

/** idle tells whether a plan writes nothing. A question that is open writes nothing either. */
export const idle = (p) =>
  !p.edits.length && !p.deletes.length && !p.moves.length && !p.sequence.some((e) => e.new !== undefined);

/** same tells whether two lists of blocks hold the same IDs in the same order with the same text. */
export function same(a, b) {
  return a.length === b.length && a.every((x, k) => x.id === b[k].id && normalize(x.md) === normalize(b[k].md));
}

// ---- Markdown of the bar and the slash menu: every button writes plain Markdown (M-8).

/** kindOf names what a line is, for the bar's "Turn into". */
export function kindOf(line, inCode) {
  if (inCode || /^ {0,3}(```|~~~)/.test(line)) return 'code';
  const h = /^(#{1,3}) /.exec(line);
  if (h) return 'h' + h[1].length;
  if (/^\s*[-*+] \[[ xX]\] /.test(line)) return 'todo';
  if (/^\s*[-*+] /.test(line)) return 'ul';
  if (/^\s*\d+[.)] /.test(line)) return 'ol';
  if (/^>/.test(line)) return 'quote';
  return 'p';
}

/** turnInto writes lines as another kind, as the bar's "Turn into" and the slash menu do. */
export function turnInto(md, kind) {
  let lines = md.split('\n');
  if (/^```/.test(md)) {
    lines = lines.slice(1);
    if (lines.length && /^```\s*$/.test(lines[lines.length - 1])) lines.pop();
  } else lines = lines.map((l) => l.replace(/^(#{1,6} |\s*[-*+] \[[ xX]\] |\s*[-*+] |\s*\d+[.)] |> ?)/, ''));
  const text = lines.join('\n');
  const each = (f) => lines.map((l) => (l.trim() ? f(l) : l)).join('\n');
  switch (kind) {
    case 'h1':
    case 'h2':
    case 'h3':
      return '#'.repeat(+kind[1]) + ' ' + lines.join(' ').trim();
    case 'ul':
      return each((l) => '- ' + l);
    case 'ol': {
      let n = 0;
      return each((l) => ++n + '. ' + l);
    }
    case 'todo':
      return each((l) => '- [ ] ' + l);
    case 'quote':
      return lines.map((l) => '> ' + l).join('\n');
    case 'code':
      return '```\n' + text + '\n```';
    case 'hr':
      return '---';
    case 'table':
      return '| Column | Column |\n|---|---|\n|  |  |';
    default:
      return text;
  }
}

/** The address of a block or page, as Copy address puts it on the clipboard (ADR-18). */
export const ADDRESS = /^\s*(?:https?:\/\/[^/\s]+)?\/o\/([0-9a-hjkmnp-tv-z]{10})(?:\/[^#\s]*)?(?:#(b[1-9][0-9]{0,8}))?\s*$/;

/** embedFor turns a pasted address into the line that embeds it, or null. */
export function embedFor(text) {
  const m = ADDRESS.exec(text || '');
  return m ? `::embed{ref="${m[1]}${m[2] ? '#' + m[2] : ''}"}` : null;
}

/** EMBED reads an embedding line. */
export const EMBED = /^::embed\{ref="([0-9a-hjkmnp-tv-z]{10})(?:#(b[1-9][0-9]{0,8}))?"\}\s*$/;

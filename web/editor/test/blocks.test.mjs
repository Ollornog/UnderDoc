// SPDX-License-Identifier: MIT

// The pure parts of the editor: the page as one text, the plan that turns a changed text into calls of the block API,
// and carrying it out against a made-up API. Run with `node --test test/` (scripts/_js_check.sh).
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { joinBlocks, markerLine, normalize, chunks, around, plan, idle, moves, turnInto, kindOf, embedFor, imageLine } from '../src/blocks.js';
import { run, Conflict } from '../src/save.js';

const stored = [
  { id: 'b1', md: '## Switches', usedIn: [] },
  { id: 'b3', md: 'Two of them.\n\nIn the rack.', usedIn: ['Other page'] },
  { id: 'b4', md: '', usedIn: [] },
];

// segmentsOf cuts a text at marker lines, as the editor does with the markers it tracks.
function segmentsOf(text) {
  const out = [];
  const lines = text.split('\n');
  let pos = 0;
  let cur = { id: null, line: null, body: [0, 0], start: 0 };
  const close = (end) => {
    cur.body = [cur.start, Math.max(cur.start, end)];
    cur.text = text.slice(cur.body[0], cur.body[1]);
    if (cur.id || cur.text.trim()) out.push(cur);
  };
  for (const line of lines) {
    const m = /^<!-- block (b\d+) -->$/.exec(line);
    if (m) {
      close(pos);
      cur = { id: m[1], adopted: false, line: [pos, pos + line.length], start: Math.min(pos + line.length + 1, text.length) };
    }
    pos += line.length + 1;
  }
  close(text.length);
  return out;
}

test('the page text keeps every block and changes nothing when nothing was typed', () => {
  const text = joinBlocks(stored);
  assert.equal(text, '<!-- block b1 -->\n## Switches\n\n<!-- block b3 -->\nTwo of them.\n\nIn the rack.\n\n<!-- block b4 -->\n');
  const p = plan(stored, segmentsOf(text));
  assert.ok(idle(p), JSON.stringify(p));
  assert.deepEqual(p.asks, []);
});

test('blank lines around a block mean nothing', () => {
  assert.equal(normalize('\n\n  \nText\n\n'), 'Text');
  const p = plan([{ id: 'b1', md: '\nText\n', usedIn: [] }], segmentsOf(joinBlocks([{ id: 'b1', md: '\nText\n' }])));
  assert.ok(idle(p));
});

test('a changed block is written', () => {
  const text = joinBlocks(stored).replace('## Switches', '## Switches and routers');
  const p = plan(stored, segmentsOf(text));
  assert.deepEqual(p.edits, [{ id: 'b1', md: '## Switches and routers' }]);
  assert.deepEqual(p.deletes, []);
});

test('a paragraph written after a block becomes a block of its own', () => {
  const text = joinBlocks(stored).replace('## Switches', '## Switches\n\nA new paragraph.');
  const p = plan(stored, segmentsOf(text));
  assert.deepEqual(p.edits, []);
  const added = p.sequence.filter((e) => e.new !== undefined);
  assert.equal(added.length, 1);
  assert.equal(added[0].new, 'A new paragraph.');
  assert.equal(text.slice(added[0].place.insert, added[0].place.insert + 5), 'A new');
  assert.deepEqual(p.sequence.map((e) => e.old || 'new'), ['b1', 'new', 'b3', 'b4']);
});

test('several paragraphs written after a block become one block each', () => {
  const text = joinBlocks(stored).replace('## Switches', '## Switches\n\nOne.\n\n- a\n- b\n\nThree.');
  const p = plan(stored, segmentsOf(text));
  assert.deepEqual(p.sequence.map((e) => e.old || e.new), ['b1', 'One.', '- a\n- b', 'Three.', 'b3', 'b4']);
});

test('a paragraph written before a block takes its marker, and the block keeps its text', () => {
  const text = joinBlocks(stored).replace('## Switches', 'Before.\n\n## Switches');
  const p = plan(stored, segmentsOf(text));
  const added = p.sequence.find((e) => e.new !== undefined);
  assert.equal(added.new, 'Before.');
  assert.equal(added.place.before, 'b1');
  assert.equal(text.slice(added.place.keep, added.place.keep + 11), '## Switches');
});

test('a block that keeps paragraphs of its own stays one block', () => {
  const text = joinBlocks(stored).replace('In the rack.', 'In the rack, left.');
  const p = plan(stored, segmentsOf(text));
  assert.deepEqual(p.edits, [{ id: 'b3', md: 'Two of them.\n\nIn the rack, left.' }]);
});

test('fenced code with blank lines is one part', () => {
  const text = 'a\n\n```\nx\n\ny\n```\n\nb';
  assert.deepEqual(chunks(text).map(([f, t]) => text.slice(f, t)), ['a', '```\nx\n\ny\n```', 'b']);
  assert.equal(around('```\nx\n\ny\n```', 'x'), null);
});

test('a block whose marker is gone is deleted, one used elsewhere is asked about first', () => {
  const text = joinBlocks(stored).replace('<!-- block b4 -->\n', '').replace('<!-- block b3 -->\n', '');
  const p = plan(stored, segmentsOf(text));
  assert.deepEqual(p.deletes, ['b4']);
  assert.deepEqual(p.asks, ['b3']);
  const yes = plan(stored, segmentsOf(text), new Set(['b3']));
  assert.deepEqual(yes.deletes, ['b3', 'b4']);
  assert.deepEqual(yes.asks, []);
});

test('a block joined to the one before it is deleted, and the one before keeps both texts', () => {
  const text = joinBlocks(stored).replace('## Switches\n\n<!-- block b3 -->\n', '## Switches\n\n');
  const p = plan(stored, segmentsOf(text), new Set(['b3']));
  assert.deepEqual(p.edits, [{ id: 'b1', md: '## Switches\n\nTwo of them.\n\nIn the rack.' }]);
  assert.deepEqual(p.deletes, ['b3']);
  assert.equal(p.sequence.filter((e) => e.new !== undefined).length, 0);
});

test('text before the first marker becomes a block at the start', () => {
  const text = 'New first.\n' + joinBlocks(stored);
  const p = plan(stored, segmentsOf(text));
  assert.deepEqual(p.sequence[0], { new: 'New first.', place: { insert: 0 } });
});

test('a copied marker makes a new block, the original keeps its ID', () => {
  const text = joinBlocks(stored) + '\n\n<!-- block b1 -->\nCopy';
  const segs = segmentsOf(text);
  segs[segs.length - 1].adopted = true;
  const p = plan(stored, segs);
  const added = p.sequence.filter((e) => e.new !== undefined);
  assert.equal(added.length, 1);
  assert.ok(added[0].place.replace);
  assert.deepEqual(p.edits, []);
});

test('a changed order becomes moves', () => {
  assert.deepEqual(moves(['b1', 'b3', 'b4'], ['b3', 'b1', 'b4']), [{ id: 'b3', after: '' }]);
  assert.deepEqual(moves(['b1', 'b3', 'b4'], ['b1', 'b4', 'b3']), [{ id: 'b4', after: 'b1' }]);
  assert.deepEqual(moves(['b1', 'b3', 'b4'], ['b1', 'b3', 'b4']), []);
  const text = joinBlocks([stored[1], stored[0], stored[2]]);
  assert.deepEqual(plan(stored, segmentsOf(text)).moves, [{ id: 'b3', after: '' }]);
});

function fakeAPI(blocks) {
  const calls = [];
  let next = 10;
  const server = blocks.map((b) => ({ ...b }));
  return {
    calls,
    server,
    load: async () => server.map((b) => ({ ...b })),
    edit: async (id, md) => {
      calls.push(['edit', id, md]);
      server.find((b) => b.id === id).md = md;
      return { id, md, html: '<p>' + md + '</p>' };
    },
    remove: async (id) => {
      calls.push(['remove', id]);
      server.splice(server.findIndex((b) => b.id === id), 1);
    },
    move: async (id, after) => {
      calls.push(['move', id, after]);
      const [b] = server.splice(server.findIndex((x) => x.id === id), 1);
      server.splice(after ? server.findIndex((x) => x.id === after) + 1 : 0, 0, b);
    },
    add: async (after, md) => {
      const id = 'b' + next++;
      calls.push(['add', after, md, id]);
      server.splice(after ? server.findIndex((x) => x.id === after) + 1 : 0, 0, { id, md });
      return { id, md, html: '' };
    },
  };
}

test('carrying out a plan leaves the server as the text has it', async () => {
  const text = joinBlocks(stored)
    .replace('## Switches', '## Switches!\n\nNew after.')
    .replace('<!-- block b4 -->\n', '');
  const api = fakeAPI(stored);
  const p = plan(stored, segmentsOf(text));
  const done = await run(p, stored, api);
  assert.deepEqual(api.calls, [['edit', 'b1', '## Switches!\n\nNew after.'], ['remove', 'b4']]);
  assert.deepEqual(done.stored.map((b) => b.id), api.server.map((b) => b.id));
  assert.equal(api.server.find((b) => b.id === 'b4'), undefined);
});

test('a page changed elsewhere is never overwritten', async () => {
  const api = fakeAPI(stored);
  api.server[0].md = 'Someone else';
  const text = joinBlocks(stored).replace('## Switches', '## Mine');
  await assert.rejects(run(plan(stored, segmentsOf(text)), stored, api), Conflict);
  assert.deepEqual(api.calls, []);
});

test('new blocks come in the order of the text, after the block before them', async () => {
  const text = 'First.\n\n' + joinBlocks(stored).replace('In the rack.', 'In the rack.\n\nLast of b3.');
  const api = fakeAPI(stored);
  const done = await run(plan(stored, segmentsOf(text)), stored, api);
  assert.deepEqual(api.calls.map((c) => c.slice(0, 3)), [['add', '', 'First.'], ['add', 'b3', 'Last of b3.']]);
  assert.deepEqual(done.stored.map((b) => b.id), ['b10', 'b1', 'b3', 'b11', 'b4']);
  assert.deepEqual(done.made.map((m) => m.id), ['b10', 'b11']);
});

test('the bar and the slash menu write plain Markdown', () => {
  assert.equal(turnInto('Title', 'h2'), '## Title');
  assert.equal(turnInto('## Title', 'p'), 'Title');
  assert.equal(turnInto('a\nb', 'ol'), '1. a\n2. b');
  assert.equal(turnInto('- a', 'todo'), '- [ ] a');
  assert.equal(turnInto('', 'table'), '| Column | Column |\n|---|---|\n|  |  |');
  assert.equal(kindOf('- [ ] x'), 'todo');
  assert.equal(kindOf('### x'), 'h3');
  assert.equal(kindOf('x', true), 'code');
});

test('an address alone becomes an embedding', () => {
  assert.equal(embedFor('https://wiki.example/o/8p3n5v2w6h#b3'), '::embed{ref="8p3n5v2w6h#b3"}');
  assert.equal(embedFor('/o/8p3n5v2w6h'), '::embed{ref="8p3n5v2w6h"}');
  assert.equal(embedFor('see /o/8p3n5v2w6h'), null);
  assert.equal(markerLine('b7'), '<!-- block b7 -->');
});

test('a pasted image is a line of its own text and address', () => {
  assert.equal(imageLine('screen shot.png', '/api/pages/p/attachments/a1'), '![screen shot](/api/pages/p/attachments/a1)');
  assert.equal(imageLine('a]b[c\\d.jpeg', '/x'), '![abcd](/x)');
  assert.equal(imageLine('.png', '/x'), '![image](/x)');
});

// SPDX-License-Identifier: MIT

// The table tools of the editor (M-8): each writes the table again as plain Markdown.
import { test } from 'node:test';
import assert from 'node:assert/strict';
import { cells, parse, write, cellAt, cellStart, apply } from '../src/tables.js';

const table = '| Port | VLAN |\n|---|---:|\n| 1 | 10 |\n| 2 | 20 |';

test('a row is split at the pipes that are not escaped', () => {
  assert.deepEqual(cells('| a | b \\| c | d |'), ['a', 'b \\| c', 'd']);
  assert.deepEqual(cells('a | b'), ['a', 'b']);
  assert.deepEqual(cells('| a | `x\\|y` |'), ['a', '`x\\|y`']);
});

test('a table is read with its alignment, and written back the same', () => {
  const t = parse(table);
  assert.deepEqual(t, { head: ['Port', 'VLAN'], align: ['', 'right'], rows: [['1', '10'], ['2', '20']] });
  assert.equal(write(t), table);
  assert.equal(parse('just text\nmore'), null);
});

test('the cell at the cursor', () => {
  assert.deepEqual(cellAt(table, 2), { row: 0, col: 0 });
  assert.deepEqual(cellAt(table, table.indexOf('VLAN')), { row: 0, col: 1 });
  assert.deepEqual(cellAt(table, table.indexOf('10')), { row: 1, col: 1 });
  assert.deepEqual(cellAt(table, table.indexOf('| 2') + 2), { row: 2, col: 0 });
  assert.equal(table.slice(cellStart(table, 2, 1), cellStart(table, 2, 1) + 2), '20');
  assert.equal(table.slice(cellStart(table, 0, 0), cellStart(table, 0, 0) + 4), 'Port');
});

test('rows go in above and below the cursor, never above the header', () => {
  assert.deepEqual(apply(table, { row: 1, col: 0 }, 'rowBelow'), {
    text: '| Port | VLAN |\n|---|---:|\n| 1 | 10 |\n|  |  |\n| 2 | 20 |', row: 2, col: 0 });
  assert.deepEqual(apply(table, { row: 1, col: 0 }, 'rowAbove'), {
    text: '| Port | VLAN |\n|---|---:|\n|  |  |\n| 1 | 10 |\n| 2 | 20 |', row: 1, col: 0 });
  assert.equal(apply(table, { row: 0, col: 1 }, 'rowAbove').text, '| Port | VLAN |\n|---|---:|\n|  |  |\n| 1 | 10 |\n| 2 | 20 |');
});

test('a row of the body goes, the header stays', () => {
  assert.deepEqual(apply(table, { row: 2, col: 1 }, 'rowDelete'), { text: '| Port | VLAN |\n|---|---:|\n| 1 | 10 |', row: 1, col: 1 });
  assert.equal(apply(table, { row: 0, col: 0 }, 'rowDelete'), null);
});

test('columns go in left and right, with their alignment, and go again', () => {
  assert.deepEqual(apply(table, { row: 1, col: 1 }, 'colLeft'), {
    text: '| Port |  | VLAN |\n|---|---|---:|\n| 1 |  | 10 |\n| 2 |  | 20 |', row: 1, col: 1 });
  assert.deepEqual(apply(table, { row: 1, col: 0 }, 'colRight'), {
    text: '| Port |  | VLAN |\n|---|---|---:|\n| 1 |  | 10 |\n| 2 |  | 20 |', row: 1, col: 1 });
  assert.deepEqual(apply(table, { row: 1, col: 0 }, 'colDelete'), { text: '| VLAN |\n|---:|\n| 10 |\n| 20 |', row: 1, col: 0 });
  assert.equal(apply('| A |\n|---|\n| 1 |', { row: 1, col: 0 }, 'colDelete'), null);
});

test('a column is aligned in the row of dashes', () => {
  assert.equal(apply(table, { row: 1, col: 0 }, 'alignCenter').text, '| Port | VLAN |\n|:---:|---:|\n| 1 | 10 |\n| 2 | 20 |');
  assert.equal(apply(table, { row: 0, col: 1 }, 'alignLeft').text, '| Port | VLAN |\n|---|:---|\n| 1 | 10 |\n| 2 | 20 |');
  assert.equal(apply(table, { row: 0, col: 0 }, 'alignRight').text, '| Port | VLAN |\n|---:|---:|\n| 1 | 10 |\n| 2 | 20 |');
});

test('escaped pipes and short rows survive a tool', () => {
  const t = '| a | b |\n|---|---|\n| x \\| y |';
  assert.equal(apply(t, { row: 1, col: 0 }, 'rowBelow').text, '| a | b |\n|---|---|\n| x \\| y |  |\n|  |  |');
});

// SPDX-License-Identifier: MIT

// Carries out a plan (blocks.js) against the block API, in an order where every "after" names a block that exists:
// changed text first, then deletions, then moves, then new blocks in the order of the text. Without CodeMirror, so
// `node --test` runs it against a made-up API.
import { moves, normalize, same } from './blocks.js';

export class Conflict extends Error {}

/** The opening marker of a part (ADR-22), as it stands in a block. */
const PART = /<!-- part t[1-9][0-9]{0,8}[ >]/g;

/**
 * run carries out plan p.
 *
 * stored: the blocks as stored, [{id, md, usedIn}]; it is not changed, the answer has the new list.
 * api: {load(), edit(id, md, ways), remove(id, ways), move(id, after), add(after, md)}; edit and add answer with the
 * block. ways: {id: ways} for blocks whose change takes away something in use (ADR-22), as the workspace chose them.
 * events: {edited(block), removed(id), moved(id, after), added(block, after)}, called after each call that succeeded.
 *
 * Returns {stored, made: [{place, id}]}: the new list, and for every new block where its marker goes. A call that fails
 * throws, with what succeeded before it already in events; Conflict when the page changed elsewhere meanwhile. A call
 * that fails names its block and kind on the error (err.block, err.kind: 'edit' or 'delete').
 */
export async function run(p, stored, api, events = {}, ways = {}) {
  const fire = (name, ...args) => events[name] && events[name](...args);
  const server = await api.load();
  if (!same(server, stored)) throw new Conflict('changed elsewhere');
  let list = stored.map((b) => ({ ...b }));
  const made = [];
  try {
    await calls();
  } catch (err) {
    err.stored = list;
    err.made = made;
    throw err;
  }
  return { stored: list, made };

  async function calls() {
    const named = (kind, block) => (err) => {
      err.kind = kind;
      err.block = block;
      throw err;
    };
    // Before a change takes away something in use, the workspace asks (ADR-22): a block that goes, or a text without a
    // part its block had. The server refuses such a change without a way as well.
    const ask = async (kind, id, md) => {
      if (ways[id] || !api.lost) return;
      const old = (stored.find((x) => x.id === id) || {}).md || '';
      if (kind === 'edit' && !(old.match(PART) || []).some((m) => !md.includes(m))) return;
      const inUse = await api.lost(id, kind === 'edit' ? md : null);
      if (inUse && inUse.length) named(kind, id)(Object.assign(new Error('409'), { inUse }));
    };
    for (const e of p.edits) {
      await ask('edit', e.id, e.md);
      const b = await api.edit(e.id, e.md, ways[e.id]).catch(named('edit', e.id));
      list = list.map((x) => (x.id === e.id ? { ...x, ...b, md: b.md ?? e.md } : x));
      fire('edited', b);
    }
    for (const id of p.deletes) {
      await ask('delete', id);
      await api.remove(id, ways[id]).catch(named('delete', id));
      list = list.filter((x) => x.id !== id);
      fire('removed', id);
    }
    for (const m of moves(list.map((x) => x.id), p.order)) {
      await api.move(m.id, m.after);
      const k = list.findIndex((x) => x.id === m.id);
      const [b] = list.splice(k, 1);
      list.splice(m.after ? list.findIndex((x) => x.id === m.after) + 1 : 0, 0, b);
      fire('moved', m.id, m.after);
    }
    let after = '';
    for (const e of p.sequence) {
      if (e.old) {
        after = e.old;
        continue;
      }
      const b = await api.add(after, e.new);
      const md = b.md ?? e.new;
      list.splice(after ? list.findIndex((x) => x.id === after) + 1 : 0, 0, { ...b, md: normalize(md), usedIn: b.usedIn || [] });
      fire('added', b, after);
      made.push({ place: e.place, id: b.id });
      after = b.id;
    }
  }
}

// SPDX-License-Identifier: MIT

// Carries out a plan (blocks.js) against the block API, in an order where every "after" names a block that exists:
// changed text first, then deletions, then moves, then new blocks in the order of the text. Without CodeMirror, so
// `node --test` runs it against a made-up API.
import { moves, normalize, same } from './blocks.js';

export class Conflict extends Error {}

/**
 * run carries out plan p.
 *
 * stored: the blocks as stored, [{id, md, usedIn}]; it is not changed, the answer has the new list.
 * api: {load(), edit(id, md), remove(id), move(id, after), add(after, md)}; edit and add answer with the block.
 * events: {edited(block), removed(id), moved(id, after), added(block, after)}, called after each call that succeeded.
 *
 * Returns {stored, made: [{place, id}]}: the new list, and for every new block where its marker goes. A call that fails
 * throws, with what succeeded before it already in events; Conflict when the page changed elsewhere meanwhile.
 */
export async function run(p, stored, api, events = {}) {
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
    for (const e of p.edits) {
      const b = await api.edit(e.id, e.md);
      list = list.map((x) => (x.id === e.id ? { ...x, ...b, md: b.md ?? e.md } : x));
      fire('edited', b);
    }
    for (const id of p.deletes) {
      await api.remove(id);
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

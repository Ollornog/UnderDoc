// SPDX-License-Identifier: MIT

// The boundaries of parts while reading (ADR-22): the pointer over the text of a part, or a finger on it, shows a light
// outline round it and a label that says where it is used, or its name if it is a value. Parts may cut through
// formatting and overlap, so a part is a range of the page between its two markers (empty spans the server writes,
// data-part-open and data-part-close), and the outline is drawn over the boxes of that range, outside the page's text.
(() => {
  'use strict';
  let shown = null;

  function clear() {
    if (!shown) return;
    for (const n of shown.nodes) n.remove();
    shown = null;
  }

  // partsAt finds the parts of a block under a point, innermost first.
  function partsAt(block, x, y) {
    const found = [];
    for (const open of block.querySelectorAll('[data-part-open]')) {
      const id = open.dataset.partOpen;
      const close = block.querySelector(`[data-part-close="${CSS.escape(id)}"]`);
      if (!close || !(open.compareDocumentPosition(close) & Node.DOCUMENT_POSITION_FOLLOWING)) continue;
      const range = document.createRange();
      range.setStartAfter(open);
      range.setEndBefore(close);
      const rects = [...range.getClientRects()].filter((r) => r.width > 0 && r.height > 0);
      if (rects.some((r) => x >= r.left - 1 && x <= r.right + 1 && y >= r.top - 1 && y <= r.bottom + 1)) {
        found.push({ id, rects, size: range.toString().length });
      }
    }
    return found.sort((a, b) => a.size - b.size);
  }

  function label(block, id) {
    let parts = [];
    try {
      parts = JSON.parse(block.dataset.parts || '[]');
    } catch (e) {
      parts = [];
    }
    const p = parts.find((x) => x.id === id) || { usedIn: [] };
    const n = (p.usedIn || []).length;
    const used = n ? `Used in ${n} ${n === 1 ? 'place' : 'places'}` : 'Not used elsewhere';
    return { text: (p.name ? `Value ${p.name} · ` : '') + used, title: n ? `Changes here show there too: ${p.usedIn.join(', ')}` : '' };
  }

  function show(block, part) {
    if (shown && shown.block === block && shown.id === part.id) return;
    clear();
    const nodes = [];
    for (const r of part.rects) {
      const box = document.createElement('div');
      box.className = 'partedge';
      box.style.setProperty('left', `${r.left - 2}px`);
      box.style.setProperty('top', `${r.top - 1}px`);
      box.style.setProperty('width', `${r.width + 4}px`);
      box.style.setProperty('height', `${r.height + 2}px`);
      nodes.push(box);
    }
    const last = part.rects[part.rects.length - 1];
    const said = label(block, part.id);
    const tag = document.createElement('div');
    tag.className = 'partlabel lab';
    tag.dataset.part = part.id;
    tag.textContent = said.text;
    if (said.title) tag.title = said.title;
    tag.style.setProperty('left', `${Math.min(last.right + 6, innerWidth - 180)}px`);
    tag.style.setProperty('top', `${last.bottom + 2}px`);
    nodes.push(tag);
    const host = document.getElementById('root') || document.body;
    for (const n of nodes) host.append(n);
    shown = { block, id: part.id, nodes };
  }

  function at(x, y, target) {
    const block = target && target.closest && target.closest('article.typeset [data-b][data-parts]');
    if (!block) return clear();
    const found = partsAt(block, x, y);
    if (found.length) show(block, found[0]);
    else clear();
  }

  document.addEventListener('pointermove', (e) => {
    if (e.pointerType === 'mouse') at(e.clientX, e.clientY, e.target);
  });
  // On a touch screen there is no hover: a tap on the text of a part shows its boundary, a tap elsewhere hides it.
  document.addEventListener('pointerup', (e) => {
    if (e.pointerType !== 'mouse') at(e.clientX, e.clientY, e.target);
  });
  addEventListener('scroll', clear, true);
  addEventListener('resize', clear);
})();

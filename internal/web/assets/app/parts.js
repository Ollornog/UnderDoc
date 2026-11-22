// SPDX-License-Identifier: MIT

// The boundaries of parts while reading (ADR-22): the pointer over the text of a part, or a finger on it, shows a light
// outline round it and a label that says where it is used, or its name if it is a value. Parts may cut through
// formatting and overlap, so a part is a range of the page between its two markers (empty spans the server writes,
// data-part-open and data-part-close), and the outline is drawn over the boxes of that range, outside the page's text:
// each box is C22's [data-part] with data-framed, placed where the script measured it; the first says the name.
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
    const said = label(block, part.id);
    // One box per line: the pieces of a line that formatting cuts apart make one frame, as in the editor.
    const lines = [];
    for (const r of part.rects) {
      const line = lines.find((l) => r.top < l.bottom && r.bottom > l.top);
      if (line) Object.assign(line, { left: Math.min(line.left, r.left), right: Math.max(line.right, r.right), top: Math.min(line.top, r.top), bottom: Math.max(line.bottom, r.bottom) });
      else lines.push({ left: r.left, right: r.right, top: r.top, bottom: r.bottom });
    }
    lines.forEach((l, k) => {
      const r = { left: l.left, top: l.top, width: l.right - l.left, height: l.bottom - l.top };
      const box = document.createElement('div');
      box.className = 'fixed pointer-events-none z-20';
      box.dataset.part = said.text;
      box.dataset.partId = part.id;
      // The name stands on the first box only; the boxes of further lines are continued frames.
      box.dataset.framed = k ? 'continued' : '';
      if (said.title) box.title = said.title;
      box.style.setProperty('left', `${r.left}px`);
      box.style.setProperty('top', `${r.top}px`);
      box.style.setProperty('width', `${r.width}px`);
      box.style.setProperty('height', `${r.height}px`);
      nodes.push(box);
    });
    const host = document.getElementById('root') || document.body;
    for (const n of nodes) host.append(n);
    shown = { block, id: part.id, nodes };
  }

  function at(x, y, target) {
    const block = target && target.closest && target.closest('article.typeset [data-block][data-parts]');
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

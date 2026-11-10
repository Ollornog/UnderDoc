// SPDX-License-Identifier: MIT

// Dragging a block of a page by its handle (.bhandle[data-drag]), in reading and in the editor (docs/editor.md).
//
// Pressing the handle and moving it lifts the block: it is hidden, and a gap with the block in it (C22's .dropgap) opens
// where it would land, always between blocks, never inside one. Letting go moves it, Escape gives up, and a click without
// moving still opens the menu of the handle. Pointer events, so a finger drags the handle as a mouse does. C22's module
// for the lists (app-workspace-drag.js) does not fit here: it leaves touch to the long press, and it moves elements of
// the list itself, while the editor draws its own lines, so the gap there has to be a widget of the editor.
//
// The places are measured once, when the block is lifted, as if it were not there, so the gap moving never moves the
// place under the pointer. What a drop does is the app's:
//
//   UnderDocBlockDrag.attach({
//     pick(handle)          -> {id, ...} or null: the block of this handle, if it may be moved
//     rows(c)               -> [{id, top, bot}] of every block of the page on the screen, in order, the held one included
//     scroller(c)           -> the element that scrolls the page
//     show(c, k)            -> hide the block, open the gap before the k-th of the other blocks (k = their number: last)
//     end(c, after)         -> let go: after is the ID to put it after, '' for first, or null when nothing moves
//   });
(function () {
  'use strict';
  var THRESHOLD = 6;

  function attach(app) {
    var press = null, drag = null;

    function down(e) {
      if (e.button !== 0 || !e.isPrimary || !e.target.closest) return;
      var handle = e.target.closest('.bhandle[data-drag]');
      var c = handle && app.pick(handle);
      press = c ? { c: c, pointer: e.pointerId, x0: e.clientX, y0: e.clientY } : null;
    }

    function lift() {
      var c = press.c, rows = app.rows(c), k = rows.findIndex(function (r) { return r.id === c.id; });
      if (k < 0 || rows.length < 2) { press = null; return; }
      var h = rows[k].bot - rows[k].top;
      var rest = rows.filter(function (_, j) { return j !== k; }).map(function (r, j) {
        return j < k ? r : { id: r.id, top: r.top - h, bot: r.bot - h };
      });
      var scroller = app.scroller(c);
      drag = { c: c, rows: rest, h: h, k: k, home: k, scroller: scroller, s0: scroller ? scroller.scrollTop : 0 };
      document.body.classList.add('ud-bdrag');
      // The held block hides its handle, so the click that ends the drag lands elsewhere and opens no menu.
      app.show(c, k);
    }

    // The slot under the pointer: within the gap it stays, below the gap the places are as if it were not there.
    function slot(y) {
      var d = drag, n = d.rows.length;
      var yy = y + (d.scroller ? d.scroller.scrollTop - d.s0 : 0);
      var top = d.k < n ? d.rows[d.k].top : d.rows[n - 1].bot;
      if (yy >= top && yy < top + d.h) return d.k;
      var ny = yy >= top + d.h ? yy - d.h : yy, k = 0;
      while (k < n && (d.rows[k].top + d.rows[k].bot) / 2 < ny) k++;
      return k;
    }

    function move(e) {
      if (!press || e.pointerId !== press.pointer) return;
      if (!drag) {
        if (Math.hypot(e.clientX - press.x0, e.clientY - press.y0) < THRESHOLD) return;
        lift();
        if (!drag) return;
      }
      e.preventDefault();
      var k = slot(e.clientY);
      if (k !== drag.k) { drag.k = k; app.show(drag.c, k); }
    }

    function finish(dropped) {
      var d = drag;
      drag = null;
      press = null;
      document.body.classList.remove('ud-bdrag');
      app.end(d.c, dropped && d.k !== d.home ? (d.k > 0 ? d.rows[d.k - 1].id : '') : null);
    }

    function up(e) {
      if (!press || e.pointerId !== press.pointer) return;
      if (drag) finish(true);
      else press = null;
    }

    function cancel(e) {
      if (!press || e.pointerId !== press.pointer) return;
      if (drag) finish(false);
      else press = null;
    }

    function key(e) {
      if (e.key !== 'Escape' || !press) return;
      if (drag) { e.preventDefault(); e.stopPropagation(); finish(false); } else press = null;
    }

    document.addEventListener('pointerdown', down);
    addEventListener('pointermove', move);
    addEventListener('pointerup', up);
    addEventListener('pointercancel', cancel);
    addEventListener('keydown', key, true);
  }

  window.UnderDocBlockDrag = { attach: attach };
})();

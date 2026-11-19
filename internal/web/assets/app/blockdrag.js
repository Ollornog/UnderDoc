// SPDX-License-Identifier: MIT

// Dragging a block of a page by its handle (.bhandle[data-drag]), in reading and in the editor (docs/editor.md).
//
// Pressing the handle and moving it lifts the block: it is hidden, and a gap with the block in it (C22's .dropgap) opens
// where it would land, always between blocks, never inside one. Letting go moves it, Escape gives up, and a click without
// moving still opens the menu of the handle. Pointer events, so a finger drags the handle as a mouse does. C22's module
// for the lists (app-workspace-drag.js) does not fit here: it leaves touch to the long press, and it moves elements of
// the list itself, while the editor draws its own lines, so the gap there has to be a widget of the editor.
//
// Real pages have long blocks, a whole section of an imported file is one (2026-11-10): the gap shows only the first
// lines of the held block (its height is capped by the CSS), so the places around it come within reach, and near the
// top or bottom edge of what scrolls the page scrolls by itself, so a block goes to places that are not in view. The
// press is the module's: it starts no selection of text and no drag of the browser, which would take the pointer away
// when a selection covers the handle.
//
// The places are measured as if the held block were not there, so the gap moving never moves the place under the
// pointer. What a drop does is the app's:
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
  // EDGE: the band at the top and bottom of what scrolls in which the page scrolls by itself; SPEED: pixels per FRAME
  // (a sixtieth of a second) at its outer end. The page scrolls by the time that passed, not by the frames drawn, so a
  // busy browser that draws fewer frames scrolls as far in the same time (2026-11-19: CI drew too few, and the block
  // landed short); LONGEST caps one step after a stall.
  var THRESHOLD = 6, EDGE = 56, SPEED = 22, FRAME = 1000 / 60, LONGEST = 200;

  function attach(app) {
    var press = null, drag = null;

    function down(e) {
      if (e.button !== 0 || !e.isPrimary || !e.target.closest) return;
      var handle = e.target.closest('.bhandle[data-drag]');
      var c = handle && app.pick(handle);
      press = c ? { c: c, pointer: e.pointerId, x0: e.clientX, y0: e.clientY } : null;
      // Neither a selection of text nor the browser's own drag of a selection that covers the handle starts, which
      // would cancel the pointer (pointercancel) before the block lifts.
      if (c) e.preventDefault();
    }

    function lift() {
      var c = press.c, rows = app.rows(c), k = rows.findIndex(function (r) { return r.id === c.id; });
      if (k < 0 || rows.length < 2) { press = null; return; }
      drag = { c: c, h: rows[k].bot - rows[k].top, k: k, home: k, scroller: app.scroller(c), y: press.y0, raf: 0, last: 0 };
      document.body.classList.add('ud-bdrag');
      // The held block hides its handle, so the click that ends the drag lands elsewhere and opens no menu.
      app.show(c, k);
      // The gap is mostly lower than the block: its height is how far the next block now stands below where the held
      // one began.
      if (k + 1 < rows.length) {
        var next = app.rows(c)[k + 1];
        if (next && next.top > rows[k].top) drag.h = next.top - rows[k].top;
      }
    }

    // The places of the other blocks as if the gap were not there: measured anew every time, since the page scrolls and
    // the editor draws what comes into view, but with the gap taken out, so the gap moving never moves the place under
    // the pointer.
    function rest() {
      var d = drag, j = 0;
      return app.rows(d.c).filter(function (r) { return r.id !== d.c.id; }).map(function (r) {
        return j++ < d.k ? r : { id: r.id, top: r.top - d.h, bot: r.bot - d.h };
      });
    }

    // The slot under the pointer: within the gap it stays, elsewhere it is the place between the blocks nearest to it.
    function slot(y) {
      var d = drag, rows = rest(), n = rows.length;
      var top = d.k < n ? rows[d.k].top : rows[n - 1].bot;
      if (y >= top && y < top + d.h) return d.k;
      var ny = y >= top + d.h ? y - d.h : y, k = 0;
      while (k < n && (rows[k].top + rows[k].bot) / 2 < ny) k++;
      return k;
    }

    function place() {
      var k = slot(drag.y);
      if (k !== drag.k) { drag.k = k; app.show(drag.c, k); }
    }

    // How far the page scrolls in a sixtieth of a second: more the deeper the pointer is in the band at the edge, or
    // beyond it.
    function speed() {
      var s = drag.scroller;
      if (!s) return 0;
      var r = s.getBoundingClientRect(), y = drag.y;
      if (y < r.top + EDGE) return -SPEED * Math.min(1, (r.top + EDGE - y) / EDGE);
      if (y > r.bottom - EDGE) return SPEED * Math.min(1, (y - r.bottom + EDGE) / EDGE);
      return 0;
    }

    function roll(now) {
      var d = drag;
      if (!d) return;
      d.raf = 0;
      var v = speed();
      if (!v) { d.last = 0; return; }
      // The first frame of a run scrolls one step; each after it by the time since the one before.
      var step = d.last ? Math.min(now - d.last, LONGEST) / FRAME : 1;
      d.last = now;
      var before = d.scroller.scrollTop;
      d.scroller.scrollTop = before + (v < 0 ? Math.floor(v * step) : Math.ceil(v * step));
      if (d.scroller.scrollTop === before) { d.last = 0; return; }
      place();
      d.raf = requestAnimationFrame(roll);
    }

    function move(e) {
      if (!press || e.pointerId !== press.pointer) return;
      if (!drag) {
        if (Math.hypot(e.clientX - press.x0, e.clientY - press.y0) < THRESHOLD) return;
        lift();
        if (!drag) return;
      }
      e.preventDefault();
      drag.y = e.clientY;
      place();
      if (!drag.raf && speed()) { drag.last = 0; drag.raf = requestAnimationFrame(roll); }
    }

    function finish(dropped) {
      var d = drag, rows = rest();
      drag = null;
      press = null;
      if (d.raf) cancelAnimationFrame(d.raf);
      document.body.classList.remove('ud-bdrag');
      app.end(d.c, dropped && d.k !== d.home ? (d.k > 0 ? rows[d.k - 1].id : '') : null);
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

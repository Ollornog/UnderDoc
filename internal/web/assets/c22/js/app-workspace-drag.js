// C22 app workspace: dragging entries of nav2 and nav3 (approved in UnderDoc, 2026-11-09).
//
// An entry is carried through its list as it looks when held: no picture floats over the rest. Where it would land it
// stands in a .dropgap, framed by a dashed line, and the entries around it slide apart. Up and down it always goes
// between entries; waiting a moment on the middle of an entry puts it below that entry (.drop-into), and in a tree the
// left edge picks the depth at once: level with the entry above, beside it; further right, below it; further left, out
// towards the top. The lists are measured once without the held entries, so the gap moving never moves the place under
// the pointer. Escape gives up. While dragging, entries do not answer the pointer (body.dragging, app-workspace.css), so
// the click that ends a drag lands on no button.
//
// The behaviour is generic; what may go where is the app's. Load the script, then:
//
//   const off = C22AppWorkspaceDrag.attach({
//     pick(e)                 -> {kind, id, ids, els, el} or null: what the pointer takes (els: the entries, in order;
//                                el: the one grabbed)
//     again(drag)             -> {els, el} anew, after the app drew itself while dragging
//     hideOf(el)              -> the element hidden while el is held (default el)
//     zones(drag)             -> [{key, box, scroll, flat, rows: [{el, id, depth, hold, head}], indent, base}]
//     home(drag)              -> the key of the zone the entries come from
//     depthOf(el)             -> the depth of the grabbed entry in a tree (default 0)
//     accept(drag, zone, at)  -> a target or null; at = {k, dep, parent, after} for a place between entries,
//                                {onto: row} for a drop onto an entry
//     ontoOnly(drag, zone)    -> true where an entry can only go onto another (default false)
//     waits(zone)             -> whether waiting puts it onto an entry (default true)
//     outside(drag, e)        -> a target where the lists are not, or null; its node gets .zone-open
//     drop(drag, target, e)   -> the app moves or copies; e.ctrlKey says copy
//     end(drag, dropped)      -> after a drop or Escape, the app draws itself again
//     scale()                 -> the zoom of the page (default 1)
//     strip                   -> attributes taken off the held copies, besides data-act (default [])
//   });
//
// Targets are the app's objects; the module adds {zone, k, dep, top, prev, keep, onto}. A target with keep stays where
// the held entries are, faint (.idle); with onto the entry is framed. Ctrl held shows a plus (.copy).
(function () {
  'use strict';
  var DWELL = 300, STEP = 16, THRESHOLD = 6, SLIDE = 140;

  function attach(app) {
    var drag = null;
    var scale = function () { return app.scale ? app.scale() : 1; };
    var watched = function () {
      var out = [];
      (drag.zones || []).forEach(function (z) { z.rows.forEach(function (r) { out.push(r.el); }); });
      return out.filter(function (n) { return n.isConnected && n.offsetParent; });
    };

    function measure() {
      drag.zones = app.zones(drag).map(function (z) {
        var s0 = z.scroll ? z.scroll.scrollTop : 0;
        var rows = z.rows.filter(function (r) { return r.el.offsetParent !== null; }).map(function (r) {
          var b = r.el.getBoundingClientRect();
          return Object.assign({}, r, { top: b.top, bot: b.bottom });
        });
        return Object.assign({}, z, { s0: s0, rows: rows, step: STEP * scale() });
      });
    }

    // Slot k of a tree at a depth: clamped to what the rows around allow, then the parent and the entry before it.
    function slot(z, k, dep) {
      if (z.flat) {
        var t = app.accept(drag, z, { k: k, dep: 0, parent: null, after: k > 0 ? z.rows[k - 1] : null });
        return t && Object.assign(t, { zone: z, k: k, dep: 0, top: k > 0 ? z.rows[k - 1].bot : z.rows[0].top });
      }
      k = Math.max(1, k);
      var prev = z.rows[k - 1], next = z.rows[k];
      var hi = prev.head ? 0 : prev.depth + (prev.hold ? 1 : 0), lo = next && !next.head ? Math.min(next.depth, hi) : 0;
      dep = Math.max(lo, Math.min(hi, dep));
      var parent = null, after = null;
      for (var j = k - 1; j >= 0; j--) {
        var r = z.rows[j];
        if (r.depth < dep) { parent = r; break; }
        if (r.depth === dep && !after) after = r;
      }
      if (!parent) return null;
      var target = app.accept(drag, z, { k: k, dep: dep, parent: parent, after: after });
      return target && Object.assign(target, { zone: z, k: k, dep: dep, top: prev.bot, prev: prev });
    }

    function onto(z, r) {
      var t = app.accept(drag, z, { onto: r });
      return t && Object.assign(t, { zone: z, keep: true, onto: r.el });
    }

    // Waiting on the middle of an entry: true once the pointer stayed long enough. Until then it asks to be asked
    // again, as the pointer may not move.
    function dwelt(r) {
      var d = drag, now = Date.now();
      if (!d.dwell || d.dwell.el !== r.el) {
        d.dwell = { el: r.el, t: now };
        clearTimeout(d.timer);
        d.timer = setTimeout(function () { if (drag === d && d.last) follow(d.last); }, DWELL + 20);
        return false;
      }
      return now - d.dwell.t >= DWELL;
    }

    function target(e) {
      var d = drag, cy = e.clientY - d.gy + d.gh / 2;
      var z = d.zones.find(function (z) {
        var b = z.box.getBoundingClientRect();
        return e.clientX >= b.left && e.clientX < b.right && e.clientY >= b.top && e.clientY < b.bottom;
      });
      if (!z) {
        d.dwell = null;
        var t = app.outside ? app.outside(d, e) : null;
        return t && Object.assign(t, { keep: true });
      }
      if (!z.rows.length) return null;
      var y = cy + ((z.scroll ? z.scroll.scrollTop : 0) - z.s0), g = d.gap && d.gap.zone === z ? d.gap : null;
      var inGap = g && y >= g.top && y < g.top + d.h, ny = g && y >= g.top + d.h ? y - d.h : y;
      var j = inGap ? -1 : z.rows.findIndex(function (r) { return ny >= r.top && ny < r.bot; });
      var r = j >= 0 ? z.rows[j] : null, f = r ? (ny - r.top) / (r.bot - r.top) : 0;
      var between = function () {
        if (inGap) return g.k;
        if (r) return f < 0.5 && !(r.head && j === 0) ? j : j + 1;
        var k = z.rows.findIndex(function (x) { return ny < (x.top + x.bot) / 2; });
        return k < 0 ? z.rows.length : k;
      };
      if (app.ontoOnly && app.ontoOnly(d, z)) { d.dwell = null; return r && r.hold ? onto(z, r) : null; }
      // On the middle of an entry the held one stays where it is while the pointer waits, so the entry does not slide
      // away from under it; left and right still pick the depth at once.
      var still = false;
      if (r && r.hold && (!app.waits || app.waits(z)) && f >= 0.25 && f <= 0.75) {
        if (dwelt(r)) return onto(z, r);
        still = !!(d.to && d.to.zone === z && !d.to.keep);
      } else d.dwell = null;
      var k = still ? d.to.k : between();
      if (z.flat) return slot(z, k, 0);
      var prev = z.rows[Math.max(1, k) - 1], level = prev.head ? 0 : prev.depth;
      var off = e.clientX - d.gx - (z.base + level * z.indent);
      return slot(z, k, level + Math.trunc(off / z.step));
    }

    // The held entries move to where they would land, and what moves for them slides there; was holds where things
    // stood before. Onto an entry, outside the lists or nowhere, they wait faint where they were.
    function show(t, was0) {
      var d = drag;
      d.to = t;
      document.querySelectorAll('.drop-into').forEach(function (n) { n.classList.remove('drop-into'); });
      if (t && t.onto) t.onto.classList.add('drop-into');
      var place = t && t.zone && !t.keep ? t : null, gap = d.gapEl;
      gap.classList.toggle('idle', !place);
      if (!place) return;
      var o = d.gap;
      if (o && o.zone === place.zone && o.k === place.k && o.dep === place.dep && gap.isConnected) return;
      var z = scale(), all = watched(), was = all.map(function (n) { return was0 && was0.has(n) ? was0.get(n) : n.getBoundingClientRect().top; });
      gap.style.marginLeft = '';
      if (place.zone.flat) {
        var nx = place.zone.rows[place.k];
        if (nx) nx.el.before(gap); else place.zone.rows[place.k - 1].el.after(gap);
      } else if (place.prev.head) place.prev.el.nextElementSibling.prepend(gap);
      else { place.prev.el.after(gap); gap.style.marginLeft = (place.dep - place.prev.depth) * place.zone.indent / z + 'px'; }
      all.forEach(function (n, i) {
        var dy = was[i] - n.getBoundingClientRect().top;
        if (Math.abs(dy) > 0.5) n.animate([{ transform: 'translateY(' + dy / z + 'px)' }, { transform: 'none' }], { duration: SLIDE, easing: 'ease-out' });
      });
      d.gap = place;
    }

    // Taking hold hides the entries and puts them, as they look, where the grabbed one was. Where everything stood is
    // taken before, so what does not move does not slide either. If the app drew itself meanwhile, it takes hold again.
    function lift(first) {
      var d = drag;
      if (!first) { var p = app.again ? app.again(d) : null; if (!p) return false; d.els = p.els; d.el = p.el; }
      d.to = null; d.gap = null;
      if (!d.el) return false;
      var r = d.el.getBoundingClientRect();
      d.h = d.els.reduce(function (a, n) { return a + n.getBoundingClientRect().height; }, 0);
      if (first) { d.gx = d.x0 - r.left; d.gy = d.y0 - r.top; d.gh = r.height; }
      var gap = document.createElement('div');
      gap.className = 'dropgap held';
      d.els.forEach(function (n) {
        var c = n.cloneNode(true);
        (app.strip || []).concat(['aria-selected', 'id']).forEach(function (a) { c.removeAttribute(a); });
        c.querySelectorAll('[data-act]').forEach(function (x) { x.removeAttribute('data-act'); });
        c.removeAttribute('data-act');
        gap.append(c);
      });
      d.gapEl = gap;
      measure();
      var was = new Map(watched().map(function (n) { return [n, n.getBoundingClientRect().top]; }));
      d.els.forEach(function (n) { (app.hideOf ? app.hideOf(n) : n).style.display = 'none'; });
      measure();
      var home = d.zones.find(function (z) { return z.key === app.home(d); });
      var k = home ? home.rows.findIndex(function (x) { return d.el.compareDocumentPosition(x.el) & Node.DOCUMENT_POSITION_FOLLOWING; }) : -1;
      if (home && k < 0) k = home.rows.length;
      show(home ? slot(home, k, app.depthOf ? app.depthOf(d.el) : 0) : null, first ? was : null);
      return true;
    }

    function follow(e) {
      var d = drag;
      d.last = { clientX: e.clientX, clientY: e.clientY, ctrlKey: e.ctrlKey, metaKey: e.metaKey, target: e.target };
      if (!d.el.isConnected && !lift(false)) return;
      document.querySelectorAll('.zone-open').forEach(function (n) { n.classList.remove('zone-open'); });
      var t = target(e);
      if (t && t.node && !t.zone) t.node.classList.add('zone-open');
      show(t);
      d.gapEl.classList.toggle('copy', !!(e.ctrlKey || e.metaKey));
    }

    function finish(dropped, e) {
      var d = drag;
      drag = null;
      clearTimeout(d.timer);
      document.body.classList.remove('dragging');
      document.querySelectorAll('.zone-open,.drop-into').forEach(function (n) { n.classList.remove('zone-open', 'drop-into'); });
      if (dropped && d.to) app.drop(d, d.to, e);
      if (app.end) app.end(d, dropped);
    }

    function down(e) {
      if (e.button !== 0 || e.pointerType === 'touch') return;
      var p = app.pick(e);
      if (!p) return;
      drag = Object.assign({}, p, { x0: e.clientX, y0: e.clientY, on: false });
    }
    function move(e) {
      if (!drag) return;
      if (!drag.on) {
        if (Math.hypot(e.clientX - drag.x0, e.clientY - drag.y0) < THRESHOLD) return;
        drag.on = true;
        document.body.classList.add('dragging');
        if (!lift(true)) { drag = null; document.body.classList.remove('dragging'); return; }
      }
      follow(e);
    }
    function up(e) {
      if (!drag) return;
      if (!drag.on) { drag = null; return; }
      finish(true, e);
    }
    function key(e) { if (e.key === 'Escape' && drag && drag.on) finish(false, e); else if (e.key === 'Escape' && drag) drag = null; }

    document.addEventListener('pointerdown', down);
    addEventListener('pointermove', move);
    addEventListener('pointerup', up);
    addEventListener('keydown', key, true);
    return function detach() {
      document.removeEventListener('pointerdown', down);
      removeEventListener('pointermove', move);
      removeEventListener('pointerup', up);
      removeEventListener('keydown', key, true);
    };
  }

  window.C22AppWorkspaceDrag = { attach: attach };
})();

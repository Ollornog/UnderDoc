// C22 workspace, before the first paint: puts the stored widths of the columns back.
// Load it in <head> WITHOUT defer or async: <script src="…/workspace-boot.js"></script>.
// It watches the parser and sets data-step on nav1 and nav2 and --workspace-nav3 on .workspace as
// soon as they exist, inside the same task, so the first paint already has them. A strict CSP
// (script-src 'self') allows it; it writes no style attribute in the markup, only CSSOM.
// The state is what c22.js stores under data-workspace-store="<key>": {"nav1":0-3,"nav2":0-3,"nav3":px}.
(function () {
  'use strict';
  if (!window.MutationObserver) return;
  var cache = {};
  function stored(key) {
    if (!(key in cache)) {
      var s = null;
      try { s = JSON.parse(localStorage.getItem(key) || 'null'); } catch (err) { s = null; }
      cache[key] = s && typeof s === 'object' ? s : null;
    }
    return cache[key];
  }
  function apply(el) {
    var ws = el.matches('.workspace') ? el : (el.matches('.workspace-nav') ? el.parentElement : null);
    if (!ws || !ws.matches || !ws.matches('.workspace[data-workspace-store]')) return;
    var s = stored(ws.dataset.workspaceStore);
    if (!s) return;
    if (el === ws) {
      if (Number.isFinite(s.nav3) && s.nav3 > 0) ws.style.setProperty('--workspace-nav3', Math.round(s.nav3) + 'px');
      return;
    }
    var v = s['nav' + el.dataset.level];
    if ((el.dataset.level === '1' || el.dataset.level === '2') && Number.isInteger(v) && v >= 0 && v <= 3) el.dataset.step = String(v);
  }
  var watcher = new MutationObserver(function (records) {
    records.forEach(function (r) {
      r.addedNodes.forEach(function (node) {
        if (node.nodeType !== 1) return;
        apply(node);
        if (node.querySelectorAll) node.querySelectorAll('.workspace, .workspace > .workspace-nav').forEach(apply);
      });
    });
  });
  watcher.observe(document.documentElement, { childList: true, subtree: true });
  document.addEventListener('DOMContentLoaded', function () {
    watcher.disconnect();
    document.querySelectorAll('.workspace[data-workspace-store]').forEach(function (ws) { ws.dataset.workspaceRestored = '1'; });
  }, { once: true });
})();

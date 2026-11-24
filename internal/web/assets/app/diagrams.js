// SPDX-License-Identifier: MIT
/* Mermaid diagrams (T-3). The server marks a ```mermaid block as figure[data-mermaid] and keeps its text, with the
   values it embeds already in it. Mermaid writes inline styles, which the policy of the workspace forbids, and it reads
   text anyone may have written; so it draws in a frame of its own (assets/app/mermaid/frame.html): sandboxed, without an
   origin of its own, with a policy that allows inline styles and nothing else. This side sends the text and shows the SVG
   it gets back as an image, where nothing in it can run. A diagram is drawn once per text and theme; each redraw of the
   window takes the same picture again. */
(() => {
  const DRAWN = new Map();
  const waiting = new Map();
  let frame = null, ready = null, next = 0;

  const version = () => (document.querySelector('script[src*="/assets/app/diagrams.js"]')?.src.split('?v=')[1]) || '';
  const theme = () => (document.documentElement.classList.contains('dark') ? 'dark' : 'default');

  window.addEventListener('message', (e) => {
    if (!frame || e.source !== frame.contentWindow) return;
    const d = e.data || {};
    if (d.ready) { ready.resolve(); return; }
    const w = waiting.get(d.id);
    if (!w) return;
    waiting.delete(d.id);
    if (typeof d.svg === 'string') w.ok(d.svg); else w.fail(new Error(String(d.error || 'Mermaid drew nothing.')));
  });

  function opened() {
    if (ready) return ready.promise;
    let resolve, reject;
    const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
    ready = { promise, resolve };
    frame = document.createElement('iframe');
    frame.setAttribute('sandbox', 'allow-scripts');
    frame.setAttribute('aria-hidden', 'true');
    frame.tabIndex = -1;
    frame.title = 'Diagrams';
    frame.dataset.diagrams = '';
    // Out of sight, but laid out: Mermaid measures its text.
    Object.assign(frame.style, { position: 'fixed', left: '-10000px', top: '0', width: '1200px', height: '800px', border: '0', visibility: 'hidden' });
    frame.src = '/assets/app/mermaid/frame.html?v=' + encodeURIComponent(version());
    document.body.append(frame);
    setTimeout(() => reject(new Error('The frame of the diagrams does not answer.')), 20000);
    return promise;
  }

  async function draw(source, look) {
    await opened();
    const id = 'd' + (++next);
    return new Promise((ok, fail) => {
      waiting.set(id, { ok, fail });
      // The frame has no origin of its own, so the message cannot name one; it holds only the text of the diagram.
      frame.contentWindow.postMessage({ id, source, theme: look }, '*');
      setTimeout(() => { if (waiting.delete(id)) fail(new Error('Mermaid took too long.')); }, 20000);
    });
  }

  function mount() {
    for (const fig of document.querySelectorAll('figure[data-mermaid]')) {
      const pre = fig.querySelector('pre'), code = pre && pre.querySelector('code');
      if (!code) continue;
      const look = theme(), source = code.textContent, key = look + '\n' + source;
      if (fig.dataset.drawn === key) continue;
      const d = DRAWN.get(key);
      if (!d) {
        DRAWN.set(key, { state: 'drawing' });
        draw(source, look).then((svg) => DRAWN.set(key, { state: 'done', svg }), (e) => DRAWN.set(key, { state: 'failed', error: String(e.message || e) })).then(mount);
        continue;
      }
      if (d.state === 'drawing') continue;
      fig.dataset.drawn = key;
      fig.querySelectorAll('[data-diagram]').forEach((n) => n.remove());
      if (d.state === 'done') {
        const img = document.createElement('img');
        img.className = 'max-w-full';
        img.dataset.diagram = '';
        img.alt = 'Diagram';
        img.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(d.svg);
        pre.hidden = true;
        fig.prepend(img);
      } else {
        const note = document.createElement('div');
        note.className = 'alert';
        note.dataset.diagram = '';
        note.setAttribute('role', 'status');
        const text = document.createElement('section');
        text.textContent = 'This diagram cannot be drawn: ' + d.error;
        note.append(text);
        pre.hidden = false;
        fig.prepend(note);
      }
    }
  }

  new MutationObserver(() => { if (document.querySelector('figure[data-mermaid]')) mount(); })
    .observe(document.documentElement, { childList: true, subtree: true });
  // A change of the theme draws the diagrams in its colours.
  new MutationObserver(() => mount()).observe(document.documentElement, { attributes: true, attributeFilter: ['class'] });
  window.UDDiagrams = { mount };
})();

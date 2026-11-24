// SPDX-License-Identifier: MIT

// The frame Mermaid draws in (T-3). Mermaid writes inline styles, which the policy of the workspace forbids, and it reads
// the text of a page, which may be anyone's. So it runs here: in a frame without an origin of its own (sandbox without
// allow-same-origin), with a policy that allows inline styles but no inline script and no connection anywhere. The
// workspace sends the source of a diagram, the frame answers with the drawn SVG, and the workspace shows it as an image,
// where nothing in it can run.
import mermaid from 'mermaid';

/** The settings every diagram is drawn with: strict, labels as SVG text (an image shows no HTML inside an SVG). */
export const settings = (theme) => ({
  startOnLoad: false,
  securityLevel: 'strict',
  suppressErrorRendering: true,
  htmlLabels: false,
  flowchart: { htmlLabels: false },
  theme: theme === 'dark' ? 'dark' : 'default',
  maxTextSize: 50000,
});

let count = 0;

/** Draws one diagram and returns its SVG as an image takes it, or throws what Mermaid found wrong in the source. */
export async function draw(source, theme) {
  mermaid.initialize(settings(theme));
  const { svg } = await mermaid.render(`ud-diagram-${++count}`, source);
  return asImage(svg);
}

/**
 * Mermaid hands the SVG as HTML; an image needs XML, and a size of its own instead of the width of a page. So the SVG is
 * read back in this frame, given the size of its view box and written out as XML.
 */
function asImage(svg) {
  // An inert document: nothing in it loads or runs while it is read.
  const el = new DOMParser().parseFromString(svg, 'text/html').querySelector('svg');
  if (!el) throw new Error('Mermaid drew nothing.');
  const view = (el.getAttribute('viewBox') || '').split(/[\s,]+/).map(Number);
  if (view.length === 4 && view[2] > 0 && view[3] > 0) {
    el.setAttribute('width', String(Math.ceil(view[2])));
    el.setAttribute('height', String(Math.ceil(view[3])));
    el.style.removeProperty('max-width');
  }
  el.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  return new XMLSerializer().serializeToString(el);
}

if (typeof window !== 'undefined' && window.parent !== window) {
  window.addEventListener('message', async (event) => {
    // Only the workspace that holds this frame asks.
    if (event.source !== window.parent) return;
    const { id, source, theme } = event.data || {};
    if (typeof id !== 'string' || typeof source !== 'string') return;
    try {
      window.parent.postMessage({ id, svg: await draw(source, theme) }, '*');
    } catch (err) {
      window.parent.postMessage({ id, error: String((err && err.message) || err) }, '*');
    }
  });
  window.parent.postMessage({ ready: true }, '*');
}

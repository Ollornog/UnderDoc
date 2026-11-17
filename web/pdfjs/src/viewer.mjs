// SPDX-License-Identifier: MIT

// The PDF preview of UnderDoc (ADR-28): pdf.js draws the pages of a PDF into canvases. Only the pages are drawn, no layer
// of links or forms, so nothing in a PDF can act; scripts of a PDF are never run, and eval is off.
import * as pdfjs from './pdf.min.mjs';

const base = new URL('./', import.meta.url).href;
pdfjs.GlobalWorkerOptions.workerSrc = base + 'pdf.worker.min.mjs';

/** The options every PDF is opened with: everything comes from UnderDoc itself. */
export const options = (url) => ({
  url,
  withCredentials: true,
  isEvalSupported: false,
  enableXfa: false,
  cMapUrl: base + 'cmaps/',
  cMapPacked: true,
  standardFontDataUrl: base + 'standard_fonts/',
  wasmUrl: base + 'wasm/',
  iccUrl: base + 'iccs/',
});

/** Draws the first pages of the PDF at url into el, as wide as el, and tells how many pages it has. */
export async function show(el, url, { maxPages = 30 } = {}) {
  const task = pdfjs.getDocument(options(url));
  const doc = await task.promise;
  try {
    el.replaceChildren();
    const width = Math.max(320, el.clientWidth || 800);
    const ratio = window.devicePixelRatio || 1;
    for (let n = 1; n <= Math.min(doc.numPages, maxPages); n++) {
      const page = await doc.getPage(n);
      const viewport = page.getViewport({ scale: width / page.getViewport({ scale: 1 }).width });
      const canvas = document.createElement('canvas');
      canvas.width = Math.floor(viewport.width * ratio);
      canvas.height = Math.floor(viewport.height * ratio);
      canvas.style.width = '100%';
      canvas.style.height = 'auto';
      canvas.dataset.page = String(n);
      el.append(canvas);
      await page.render({ canvas, viewport, transform: ratio === 1 ? undefined : [ratio, 0, 0, ratio, 0, 0] }).promise;
    }
    return { pages: doc.numPages, shown: Math.min(doc.numPages, maxPages) };
  } finally {
    await task.destroy();
  }
}

/** Reads the text of every page of the PDF at url, page by page (for the search, ADR-24). */
export async function text(url, { maxPages = 500 } = {}) {
  const task = pdfjs.getDocument(options(url));
  const doc = await task.promise;
  try {
    const pages = [];
    for (let n = 1; n <= Math.min(doc.numPages, maxPages); n++) {
      const content = await (await doc.getPage(n)).getTextContent();
      pages.push(content.items.map((i) => i.str + (i.hasEOL ? '\n' : '')).join(''));
    }
    return pages;
  } finally {
    await task.destroy();
  }
}

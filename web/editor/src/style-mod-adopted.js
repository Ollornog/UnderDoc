// SPDX-License-Identifier: MIT

// CodeMirror writes its base styles with style-mod. In a document, style-mod puts them into a <style> element, which the
// CSP of UnderDoc blocks (style-src 'self', no inline style, no nonce). In a shadow root it uses a constructed style
// sheet instead, which the CSP allows. This module stands in for style-mod in the bundle (build.mjs resolves the import
// here) and hands style-mod a stand-in for the document that has no <head> but the document's adopted style sheets, so
// the same constructed style sheet ends up in the page. Nothing else changes: the editor lives in the page, not in a
// shadow root, and the styles of the workspace reach it.
import { StyleModule as Original } from 'style-mod';

const roots = new WeakMap();

// documentRoot is what style-mod sees instead of a document: ownerDocument for the window, and the document's adopted
// style sheets, read and written through.
function documentRoot(doc) {
  let root = roots.get(doc);
  if (!root) {
    root = {
      ownerDocument: doc,
      get adoptedStyleSheets() {
        return doc.adoptedStyleSheets;
      },
      set adoptedStyleSheets(sheets) {
        doc.adoptedStyleSheets = sheets;
      },
    };
    roots.set(doc, root);
  }
  return root;
}

export class StyleModule extends Original {
  static mount(root, modules, options) {
    const isDocument = root && root.nodeType === 9 && root.adoptedStyleSheets && root.defaultView && root.defaultView.CSSStyleSheet;
    return Original.mount(isDocument ? documentRoot(root) : root, modules, options);
  }
}

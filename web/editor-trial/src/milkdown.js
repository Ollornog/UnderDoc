// SPDX-License-Identifier: MIT

// Candidate 1: Milkdown with its Crepe preset (ProseMirror + remark): slash menu, toolbar, top bar, tables, block
// handle, code blocks with CodeMirror. Built from CrepeBuilder with the features named one by one, so KaTeX (latex)
// and the AI feature are not even bundled: UnderDoc writes no formulas, and nothing may call out. The languages of the
// code blocks load on demand from chunks next to the bundle.
import { CrepeBuilder } from '@milkdown/crepe/builder';
import { blockEdit } from '@milkdown/crepe/feature/block-edit';
import { codeMirror } from '@milkdown/crepe/feature/code-mirror';
import { cursor } from '@milkdown/crepe/feature/cursor';
import { imageBlock } from '@milkdown/crepe/feature/image-block';
import { linkTooltip } from '@milkdown/crepe/feature/link-tooltip';
import { listItem } from '@milkdown/crepe/feature/list-item';
import { placeholder } from '@milkdown/crepe/feature/placeholder';
import { table } from '@milkdown/crepe/feature/table';
import { toolbar } from '@milkdown/crepe/feature/toolbar';
import { topBar } from '@milkdown/crepe/feature/top-bar';
import { languages } from '@codemirror/language-data';
import { editorViewCtx, remarkStringifyOptionsCtx } from '@milkdown/kit/core';
import { replaceAll } from '@milkdown/kit/utils';
import '@milkdown/crepe/theme/common/prosemirror.css';
import '@milkdown/crepe/theme/common/reset.css';
import '@milkdown/crepe/theme/common/block-edit.css';
import '@milkdown/crepe/theme/common/code-mirror.css';
import '@milkdown/crepe/theme/common/cursor.css';
import '@milkdown/crepe/theme/common/image-block.css';
import '@milkdown/crepe/theme/common/link-tooltip.css';
import '@milkdown/crepe/theme/common/list-item.css';
import '@milkdown/crepe/theme/common/placeholder.css';
import '@milkdown/crepe/theme/common/toolbar.css';
import '@milkdown/crepe/theme/common/table.css';
import '@milkdown/crepe/theme/common/top-bar.css';
import '@milkdown/crepe/theme/frame.css';
import './milkdown.css';

// The code blocks of Crepe are CodeMirror editors, whose base styles style-mod writes as a <style> element into a
// document, which style-src 'self' blocks; in a shadow root it uses a constructed style sheet, which the CSP allows. So
// the editor lives in a shadow root, with its stylesheet linked inside. ?milkdown=light mounts it in the page instead,
// to see what the CSP breaks.
async function mountPoint(host) {
  if (new URLSearchParams(location.search).get('milkdown') === 'light') return host;
  const box = document.createElement('div');
  box.className = 'milkdown-trial-host';
  host.append(box);
  const shadow = box.attachShadow({ mode: 'open' });
  const link = document.createElement('link');
  link.rel = 'stylesheet';
  const href = new URL('./milkdown.css', import.meta.url);
  href.search = new URL(import.meta.url).search;
  link.href = href.href;
  const loaded = new Promise((resolve) => {
    link.addEventListener('load', resolve);
    link.addEventListener('error', resolve);
  });
  const root = document.createElement('div');
  shadow.append(link, root);
  await loaded;
  return root;
}

window.editorTrial.register('milkdown', async (host, markdown) => {
  const crepe = new CrepeBuilder({ root: await mountPoint(host), defaultValue: markdown });
  // remark writes "*" for bullets and "***" for rules by default; UnderDoc pages use "-" and "---".
  crepe.editor.config((ctx) => {
    ctx.update(remarkStringifyOptionsCtx, (options) => ({ ...options, bullet: '-', rule: '-' }));
  });
  crepe
    .addFeature(topBar)
    .addFeature(codeMirror, { languages })
    .addFeature(listItem)
    .addFeature(linkTooltip)
    .addFeature(cursor)
    .addFeature(imageBlock)
    .addFeature(blockEdit)
    .addFeature(placeholder)
    .addFeature(toolbar)
    .addFeature(table);
  await crepe.create();
  const focus = () => crepe.editor.action((ctx) => ctx.get(editorViewCtx).focus());
  return {
    getMarkdown: () => crepe.getMarkdown(),
    reset(text) {
      crepe.editor.action(replaceAll(text));
      focus();
    },
    focus,
    scope: crepe.editor.action((ctx) => ctx.get(editorViewCtx).dom),
  };
});

// SPDX-License-Identifier: MIT

// Candidate 2: TipTap, MIT parts only (core, StarterKit, lists, tables, image and the official Markdown extension).
// No Pro extension and no Tiptap Cloud. TipTap brings no look of its own, so tiptap.css gives it a minimal one.
import { Editor } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import { TaskList, TaskItem } from '@tiptap/extension-list';
import { TableKit } from '@tiptap/extension-table';
import Image from '@tiptap/extension-image';
import { Markdown } from '@tiptap/markdown';
import './tiptap.css';

window.editorTrial.register('tiptap', async (host, markdown) => {
  const editor = new Editor({
    element: host,
    // By default TipTap adds its base styles as a <style> element, which style-src 'self' blocks. They are in tiptap.css.
    injectCSS: false,
    extensions: [
      StarterKit.configure({ link: { openOnClick: false } }),
      TaskList,
      TaskItem.configure({ nested: true }),
      TableKit.configure({ table: { resizable: false } }),
      Image,
      Markdown,
    ],
    content: markdown,
    contentType: 'markdown',
    editorProps: { attributes: { class: 'tt-editor' } },
  });
  return {
    getMarkdown: () => editor.getMarkdown(),
    reset(text) {
      editor.commands.setContent(text, { contentType: 'markdown' });
      editor.commands.focus('end');
    },
    focus: () => editor.commands.focus('end'),
    scope: editor.view.dom,
  };
});

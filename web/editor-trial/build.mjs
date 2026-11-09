// SPDX-License-Identifier: MIT

// Bundles the trial page and the three editor candidates into internal/web/assets/app/editor-trial/, and writes
// meta.json with the version and licence of every bundled package and the gzipped size of every bundle.
//
//   npm ci && npm run build
//
// Every entry is built on its own, as an ES module with code splitting: what an editor loads only on demand (the
// languages of Milkdown's code blocks) lands in <entry>-chunks/ next to it, and no two editors share a chunk.
import { build } from 'esbuild';
import { readFileSync, writeFileSync, rmSync, mkdirSync } from 'node:fs';
import { gzipSync } from 'node:zlib';
import { join, dirname, relative } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const out = join(here, '../../internal/web/assets/app/editor-trial');
const entries = ['trial', 'milkdown', 'tiptap', 'codemirror'];
const lock = JSON.parse(readFileSync(join(here, 'package-lock.json'), 'utf8'));

rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });

const gzip = (file) => gzipSync(readFileSync(file), { level: 9 }).length;
const meta = { bundles: {} };

for (const entry of entries) {
  const result = await build({
    entryPoints: { [entry]: join(here, 'src', `${entry}.js`) },
    outdir: out,
    bundle: true,
    splitting: true,
    minify: true,
    format: 'esm',
    target: ['es2022'],
    metafile: true,
    legalComments: 'none',
    chunkNames: `${entry}-chunks/[name]-[hash]`,
    // Fonts and pictures a stylesheet names are copied next to it, so nothing comes from elsewhere.
    loader: { '.woff2': 'file', '.woff': 'file', '.ttf': 'file', '.svg': 'file', '.png': 'file' },
    assetNames: `${entry}-files/[name]-[hash]`,
    logLevel: 'warning',
  });
  const packages = new Map();
  const files = { js: 0, jsGzip: 0, css: 0, cssGzip: 0, lazy: 0, lazyGzip: 0, lazyFiles: 0, other: 0 };
  for (const [file, info] of Object.entries(result.metafile.outputs)) {
    for (const input of Object.keys(info.inputs)) {
      const match = input.match(/node_modules\/((?:@[^/]+\/)?[^/]+)/g);
      if (!match) continue;
      const name = match[match.length - 1].replace(/^node_modules\//, '');
      const locked = lock.packages[match.join('/')] || lock.packages[`node_modules/${name}`] || {};
      packages.set(`${name}@${locked.version}`, { name, version: locked.version || '?', license: locked.license || '?' });
    }
    const full = join(here, file);
    const rel = relative(out, full);
    const size = readFileSync(full).length;
    if (rel === `${entry}.js`) {
      files.js = size;
      files.jsGzip = gzip(full);
    } else if (rel === `${entry}.css`) {
      files.css = size;
      files.cssGzip = gzip(full);
    } else if (rel.endsWith('.js')) {
      files.lazy += size;
      files.lazyGzip += gzip(full);
      files.lazyFiles++;
    } else {
      files.other += size;
    }
  }
  meta.bundles[entry] = { ...files, packages: [...packages.values()].sort((a, b) => a.name.localeCompare(b.name)) };
  const kb = (n) => `${(n / 1024).toFixed(0)} KiB`;
  console.log(
    `${entry.padEnd(11)} JS ${kb(files.js)} (gzip ${kb(files.jsGzip)})  CSS ${kb(files.css)} (gzip ${kb(files.cssGzip)})` +
      `  on demand ${files.lazyFiles} chunks ${kb(files.lazy)} (gzip ${kb(files.lazyGzip)})  other ${kb(files.other)}` +
      `  ${packages.size} packages`,
  );
}
writeFileSync(join(out, 'meta.json'), JSON.stringify(meta, null, 1) + '\n');

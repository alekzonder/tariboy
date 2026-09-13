#!/usr/bin/env node
// Write the DS's design tokens out as a real token stylesheet, so the
// converter has something to point `cfg.tokensPkg` at.
//
// Why this exists: `emitReadme` scans `tokens/*.css` for the README's token
// list and only falls back to scanning the whole of `_ds_bundle.css` when no
// token file was shipped. Tariboy hit that fallback, and the fallback is a
// flat regex with no notion of scope - so the token list the design agent
// reads in its system prompt advertised 220 "tokens", led by flexlayout-react's
// `--color-text`, `--color-background`, `--color-base` (its light.css theme,
// declared on `.flexlayout__layout` under generic names) and Tailwind's
// `--tw-*` internals. None of those resolve outside the subtree that declares
// them: an agent writing `background: var(--color-background)` gets nothing.
//
// So: hoist the real theme tokens - exactly the declarations
// `annotate-tokens.mjs` classifies as tokens, from `:root` / `:root,:host` /
// `.dark` - into one stylesheet, and let `cfg.tokensPkg` pick it up. The
// output is pure metadata as far as rendering goes: `styles.css` imports it
// before `_ds_bundle.css`, which redeclares every one of these names with the
// same value, so the cascade is unchanged.
//
// `copyTokens` resolves `cfg.tokensPkg` under `node_modules/`, so the file is
// staged as a minimal package there. It is a build artifact like the staged
// CSS: gitignored, rewritten on every `prepare-css.sh`, and dropped by
// `npm ci` (which is why `cfg.buildCmd` must run before the converter).
// Nothing imports it as JavaScript - `story-imports.mjs` intercepts the
// `tariboy-ui` specifier by name before esbuild ever consults node_modules.

import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { themeTokenBlocks } from './annotate-tokens.mjs';

const UI = resolve(dirname(fileURLToPath(import.meta.url)), '..');

// Put the app's own theme layer (`src/index.css`, which writes plain `:root`
// and `.dark`) ahead of Tailwind's generated `:root,:host` theme, in place.
//
// This is presentation, and it is the reason the README reads well: emitReadme
// buckets the token names by a regex on the name and prints the first three of
// each bucket in file order. shadcn's semantic tokens carry no `color`/`font`
// in their names, so they all land in the **other** bucket - which in source
// order opened with `--spacing`, `--container-xs`, `--container-sm`. It now
// opens with `--background`, `--foreground`, `--card`: the tokens the DS
// actually styles with, and the ones `conventions.md` tells the design agent
// to reach for.
//
// The **color** bucket stays Tailwind's `--color-*` palette either way - those
// are the only names matching `color`, and they are real. `conventions.md`,
// which rides at the top of the same README, is what steers the agent to the
// semantic tokens over the raw scale.
//
// Cascade-neutral, and checked rather than assumed: reordering could only
// matter if a name were declared in both layers, so that case bails out and
// keeps source order. (`.dark` keeps its position after the `:root` it
// overrides - both layers are moved as a unit.)
function order(blocks) {
  const appLayer = (b) => b.scope === ':root' || b.scope === '.dark';
  const names = (pred) => new Set(blocks.filter(pred).flatMap((b) => b.decls.map((d) => d.split(':')[0].trim())));
  const shared = [...names(appLayer)].filter((n) => names((b) => !appLayer(b)).has(n));
  if (shared.length) {
    console.error(`  (keeping source order: ${shared.length} name(s) declared in both theme layers, e.g. ${shared[0]})`);
    return;
  }
  blocks.sort((a, b) => (appLayer(a) === appLayer(b) ? 0 : appLayer(a) ? -1 : 1));
}

// The kind of a token - `color|spacing|radius|shadow|font|other` - is a design
// decision, so it is written where the decision is: on the token's own
// declaration line in `src/index.css`, as `--panel-radius: 14px; /* @kind
// radius */`. Vite minifies comments out of the compiled stylesheet this
// generator reads, so the labels are picked back up from the source here and
// re-attached to the declaration in the emitted token file - immediately after
// the `;`, the only position the app-side check reads (a marker on its own line
// annotates nothing).
//
// Only tokens whose kind the check cannot infer from the value need a label
// (a shadow list, a bare `14px`, a font stack). An `oklch()` color buckets
// itself; annotating all 80 of them would be noise, not information.
const KIND = /^\s*(--[\w-]+)\s*:[^;]*;\s*\/\* @kind (color|spacing|radius|shadow|font|other) \*\//;

function kinds(file) {
  const map = new Map();
  for (const line of readFileSync(file, 'utf8').split('\n')) {
    const m = line.match(KIND);
    if (m) map.set(m[1], m[2]);
  }
  return map;
}

export function makeTokensPkg({
  cssFile = join(UI, '.design-sync/.cache/css/styles.css'),
  nodeModules = join(UI, 'node_modules'),
  pkgName = JSON.parse(readFileSync(join(UI, 'package.json'), 'utf8')).name,
  themeSrc = join(UI, 'src/index.css'),
} = {}) {
  const { version } = JSON.parse(readFileSync(join(UI, 'package.json'), 'utf8'));
  const blocks = themeTokenBlocks(readFileSync(cssFile, 'utf8'));
  const count = blocks.reduce((n, b) => n + b.decls.length, 0);
  if (!count) throw new Error(`make-tokens: no theme tokens found in ${cssFile} - has the app moved its tokens out of :root/.dark?`);
  order(blocks);
  const kind = kinds(themeSrc);

  const css =
    '/* Tariboy UI design tokens - generated by .design-sync/make-tokens.mjs from the\n' +
    '   compiled app stylesheet. Every name below is declared identically in\n' +
    '   _ds_bundle.css; this file exists so the token list is the theme, and not\n' +
    "   the bundle's third-party and utility internals. Do not edit. */\n\n" +
    blocks
      .map((b) => {
        const decls = b.decls.map((d) => {
          const k = kind.get(d.split(':')[0].trim());
          return `  ${d};${k ? `/* @kind ${k} */` : ''}`;
        });
        return `${b.scope} {\n${decls.join('\n')}\n}`;
      })
      .join('\n\n') +
    '\n';

  const dir = join(nodeModules, pkgName);
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(join(dir, 'dist/css'), { recursive: true });
  // copyTokens reads the version for its log line; `private` keeps npm from
  // ever treating the staged directory as publishable.
  writeFileSync(join(dir, 'package.json'), JSON.stringify({ name: pkgName, version, private: true }, null, 2) + '\n');
  writeFileSync(join(dir, 'dist/css/theme.css'), css);
  const labelled = blocks.reduce((n, b) => n + b.decls.filter((d) => kind.has(d.split(':')[0].trim())).length, 0);
  console.error(
    `staged ${count} design token(s) from ${blocks.map((b) => b.scope).join(' + ')} ` +
      `(${labelled} carrying an @kind label from ${relative(UI, themeSrc)}) ` +
      `→ node_modules/${pkgName}/dist/css/theme.css`,
  );
  return { count, labelled, blocks: blocks.length };
}

if (process.argv[1] && process.argv[1].endsWith('make-tokens.mjs')) makeTokensPkg();

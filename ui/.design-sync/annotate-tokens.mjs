#!/usr/bin/env node
// Annotate non-token custom properties in the staged Tailwind output with
// `/* @kind other */`.
//
// Why: the design-system check scans every custom property reachable from
// styles.css. Tariboy ships no static stylesheet, so ALL of its CSS - theme
// tokens and compiled Tailwind utility output alike - lands in one file
// (_ds_bundle.css). The scan therefore sees, alongside the real theme tokens
// under :root/.dark, several hundred declarations that are not design tokens
// at all:
//
//   --tw-*            Tailwind v4 internals (`*,:before,:after,::backdrop`,
//                     utility classes, and 73 `@property` rules)
//   --color-*, --font-*, --size-*   flexlayout-react's light.css theme, all
//                     declared on `.flexlayout__layout` under generic names
//                     that collide with real token names
//   --splitter-*      terminalWorkspace.css
//   --card-spacing    a Tailwind arbitrary-property utility
//
// The rule applied here is structural, not a name allowlist: a custom
// property declared inside a component or utility selector is not a design
// token. Only declarations whose innermost scope is a theme scope (:root,
// :host, html, .dark, or a comma list of those) stay unannotated.
//
// One narrow exception rides on top of that rule: DEMOTE_NAMES, the handful of
// Tailwind engine defaults Tailwind writes into the app's own `:root,:host`
// block. Scope cannot separate those from real tokens, so they are named.
//
// Every marker is then audited: it must sit immediately after a declaration's
// `;` or an `@property` rule's `{`. A marker attached to nothing is a build
// failure, not a cosmetic flaw.
//
// Nothing is deleted - every declaration still ships and still renders;
// `--tw-*` in particular is load-bearing for the compiled utilities. CSS
// comments are inert, so this is a pure metadata pass.
//
// Idempotent: re-running over annotated CSS is a no-op.

import { readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const MARKER = '/* @kind other */';
const THEME_PART = /^(:root|:host|html|\.dark)$/;

// Custom properties that ARE declared in a theme scope but are not design
// tokens. Tailwind v4 writes its own engine defaults into the very same
// `:root,:host` block as the app's tokens, so *scope* cannot separate these
// from `--background` and friends - only the name can. All four carry no
// design decision a design agent would reach for (they back the `transition-*`
// and `animate-spin` utilities), and their values are exactly the kinds the
// token scan cannot bucket: an easing curve, a keyframes shorthand, a bare
// duration.
//
// Keep this list minimal and name every entry deliberately. A real token whose
// value merely looks unbucketable (a `calc()` line-height, a font weight)
// belongs in the token list, not here.
export const DEMOTE_NAMES = new Set([
  '--ease-in-out',
  '--animate-spin',
  '--default-transition-duration',
  '--default-transition-timing-function',
]);

// The structural half of the same idea. The list above names four engine
// defaults explicitly; this derives the rest, because naming them one at a
// time does not survive ordinary UI work. Tailwind v4 emits a theme default
// into the app's own `:root,:host` block the first time a utility uses it, so
// every new `text-2xl` or `max-w-5xl` in a component silently adds a "token".
// The metal/teal theme pass grew that leak from a handful to 49 of 100 names
// (`--color-amber-400`, `--container-5xl`, `--text-lg--line-height`, ...) -
// every one of which the README would have advertised to the design agent as
// a Tariboy design token.
//
// The discriminator is authorship, not spelling: a design token is a custom
// property this repo's OWN stylesheets declare. Tailwind's defaults are
// materialized into the compiled bundle and appear in no source file, so
// scanning src/**/*.css separates them exactly, with no name list to rot. The
// two names this rule looked likeliest to get wrong were checked by hand and
// are genuinely authored in src/index.css: `--color-background` (:64) and
// `--radius-md` (:66, which button.tsx and select.tsx both read).
//
// This can only NARROW what the scope rule already admitted, so it cannot
// promote a non-token.
const AUTHORED = collectAuthoredNames();

function collectAuthoredNames() {
  const dir = resolve(dirname(fileURLToPath(import.meta.url)), '../src');
  const names = new Set();
  const walk = (d) => {
    for (const e of readdirSync(d, { withFileTypes: true })) {
      const p = join(d, e.name);
      if (e.isDirectory()) walk(p);
      else if (e.name.endsWith('.css')) {
        for (const m of readFileSync(p, 'utf8').matchAll(/(?:^|[;{\s])(--[\w-]+)\s*:/g)) names.add(m[1]);
      }
    }
  };
  walk(dir);
  // Tripwire, in the same spirit as make-tokens.mjs: if the app ever moves its
  // tokens out of .css files, this scan comes back near-empty and would demote
  // every real token in one silent step. Fail loudly instead of shipping that.
  if (names.size < 40) {
    throw new Error(
      `annotate-tokens: only ${names.size} custom properties authored under src/**/*.css - ` +
        'the app appears to have moved its tokens. Fix the scan before trusting the token list.',
    );
  }
  return names;
}

/** A theme-scope custom property is a design token only if this repo authors it. */
export const isDemoted = (name) => DEMOTE_NAMES.has(name) || !AUTHORED.has(name);

// A scope is a theme scope only if EVERY comma-separated part is one.
// `:root,:host` qualifies; `.dark\:scale-0:is(.dark *)` does not.
export const isThemeScope = (sel) => {
  const s = sel.replace(/\/\*[\s\S]*?\*\//g, '').trim();
  if (!s || s.startsWith('@')) return false;
  return s.split(',').every((p) => THEME_PART.test(p.trim()));
};

// The declared name, or null when the text is not a custom-property
// declaration. Comments are stripped first so an already-annotated
// declaration still reports its name.
export const declName = (decl) => decl.replace(/\/\*[\s\S]*?\*\//g, '').match(/^\s*(--[\w-]+)\s*:/)?.[1] ?? null;

function annotate(css) {
  // Idempotence by reconstruction: drop every marker this script has ever
  // written, then re-derive them. Checking "is it already annotated?" during
  // the walk is positionally fragile - a block's first declaration sees no
  // preceding marker in its own text and would be marked twice.
  css = css.split(MARKER).join('');
  let out = '';
  let cur = ''; // text since the last {, } or ; at paren depth 0
  const stack = []; // innermost-last selector preludes
  let paren = 0;
  let counts = { decls: 0, atProperty: 0, skipped: 0, demoted: 0 };

  const scope = () => (stack.length ? stack[stack.length - 1] : '');

  // Flush a declaration, appending the marker when it is a custom property
  // that is either outside every theme scope or named in DEMOTE_NAMES.
  const flushDecl = (terminator) => {
    let text = cur;
    cur = '';
    const name = declName(text);
    if (!name) return (out += text + terminator);
    const demoted = isDemoted(name);
    if (isThemeScope(scope()) && !demoted) {
      counts.skipped++;
      return (out += text + terminator);
    }
    if (text.includes('@kind')) return (out += text + terminator);
    counts.decls++;
    if (demoted) counts.demoted++;
    // Always terminate with `;` so the marker never fuses with the value -
    // a block's last declaration may arrive with `}` as its terminator.
    out += `${text};${MARKER}${terminator === ';' ? '' : terminator}`;
  };

  for (let i = 0; i < css.length; i++) {
    const c = css[i];

    // CSS escapes come first: Tailwind's generated class names escape the
    // punctuation they embed, so selectors carry literal `\'`, `\(`, `\)` and
    // `\/` (e.g. `.\[\&_svg\:not\(\[class\*\=\'size-\'\]\)\]\:size-3`, or
    // `.top-1\/2`). Read as delimiters those would open a phantom string,
    // unbalance the paren count, or start a phantom comment.
    if (c === '\\' && i + 1 < css.length) {
      cur += css.slice(i, i + 2);
      i++;
      continue;
    }

    // Comments and strings pass through verbatim.
    if (c === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2);
      const stop = end === -1 ? css.length : end + 2;
      cur += css.slice(i, stop);
      i = stop - 1;
      continue;
    }
    if (c === '"' || c === "'") {
      let j = i + 1;
      while (j < css.length && css[j] !== c) j += css[j] === '\\' ? 2 : 1;
      cur += css.slice(i, j + 1);
      i = j;
      continue;
    }

    if (c === '(') paren++;
    else if (c === ')') paren = Math.max(0, paren - 1);

    // `;` inside url(data:...;base64,...) is not a declaration terminator.
    if (paren > 0) {
      cur += c;
      continue;
    }

    if (c === '{') {
      const prelude = cur.trim();
      out += cur + '{';
      cur = '';
      stack.push(prelude);
      // `@property --tw-foo { ... }` declares the name in the prelude, not as
      // a declaration - mark the block itself.
      const m = prelude.match(/^@property\s+(--[\w-]+)/);
      if (m && !THEME_NAMES.has(m[1])) {
        out += MARKER;
        counts.atProperty++;
      }
      continue;
    }

    if (c === '}') {
      flushDecl('');
      out += '}';
      stack.pop();
      continue;
    }

    if (c === ';') {
      flushDecl(';');
      continue;
    }

    cur += c;
  }
  out += cur;
  return { css: out, counts };
}

// Pass 1: collect the names declared in theme scopes, so an `@property` rule
// backing a real token is never demoted. DEMOTE_NAMES are held out here, so an
// `@property` rule backing one of them is marked like any other non-token.
const THEME_NAMES = new Set();
function collectThemeNames(css) {
  const re = /([^{}]*)\{([^{}]*)\}/g;
  let m;
  while ((m = re.exec(css))) {
    const sel = m[1].split(/[{}]/).pop();
    if (!isThemeScope(sel)) continue;
    for (const d of m[2].matchAll(/(--[\w-]+)\s*:/g)) {
      if (!isDemoted(d[1])) THEME_NAMES.add(d[1]);
    }
  }
}

// Pass 3: audit the annotated CSS. Every marker must sit *in* something - a
// marker that ends up floating between rules annotates nothing, and reads as
// litter in the shipped stylesheet. Two legal positions, and no third:
//
//   `--x:1px;/* @kind other */`        immediately after a declaration's `;`
//   `@property --x{/* @kind other */`  immediately after an @property `{`
//
// The walker below cannot currently produce anything else (flushDecl always
// emits its own `;` first), so this is a standing guarantee rather than a
// repair: it is what keeps a later edit to the walker from silently shipping
// detached markers. Violations throw, which fails the converter run.
export function audit(css) {
  const attached = [];
  const cps = []; // every custom-property declaration: { name, scope, marked }
  let cur = '';
  const stack = [];
  let paren = 0;
  const scope = () => (stack.length ? stack[stack.length - 1] : '');
  const marked = (i) => css.startsWith(MARKER, i);

  for (let i = 0; i < css.length; i++) {
    const c = css[i];
    if (c === '\\' && i + 1 < css.length) { cur += css.slice(i, i + 2); i++; continue; }
    if (c === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2);
      const stop = end === -1 ? css.length : end + 2;
      cur += css.slice(i, stop);
      i = stop - 1;
      continue;
    }
    if (c === '"' || c === "'") {
      let j = i + 1;
      while (j < css.length && css[j] !== c) j += css[j] === '\\' ? 2 : 1;
      cur += css.slice(i, j + 1);
      i = j;
      continue;
    }
    if (c === '(') paren++;
    else if (c === ')') paren = Math.max(0, paren - 1);
    if (paren > 0) { cur += c; continue; }

    if (c === '{') {
      const prelude = cur.trim();
      cur = '';
      stack.push(prelude);
      if (marked(i + 1)) {
        if (!/^@property\s+--[\w-]+/.test(prelude)) {
          throw new Error(`annotate-tokens: marker opens a non-@property block: ${prelude.slice(-90)} {`);
        }
        attached.push(i + 1);
      }
      continue;
    }
    if (c === '}' || c === ';') {
      const name = declName(cur);
      if (name) cps.push({ name, scope: scope(), marked: c === ';' && marked(i + 1) });
      if (c === ';' && marked(i + 1)) {
        if (!name) throw new Error(`annotate-tokens: marker follows a non-custom-property declaration: ${cur.trim().slice(-90)};`);
        attached.push(i + 1);
      }
      cur = '';
      if (c === '}') stack.pop();
      continue;
    }
    cur += c;
  }

  // Every marker in the file must be one of the attached ones.
  const total = css.split(MARKER).length - 1;
  if (total !== attached.length) {
    const seen = new Set(attached);
    const loose = [];
    for (let p = css.indexOf(MARKER); p !== -1; p = css.indexOf(MARKER, p + 1)) {
      if (!seen.has(p)) loose.push(`line ${css.slice(0, p).split('\n').length}: …${css.slice(Math.max(0, p - 60), p + MARKER.length)}`);
    }
    throw new Error(
      `annotate-tokens: ${total - attached.length} marker(s) attached to nothing:\n  ${loose.slice(0, 8).join('\n  ')}`,
    );
  }

  // And every non-token declaration must carry one.
  const missed = cps.filter((d) => d.marked === false && (DEMOTE_NAMES.has(d.name) || !isThemeScope(d.scope)));
  if (missed.length) {
    throw new Error(
      `annotate-tokens: ${missed.length} non-token declaration(s) left unmarked: ` +
        missed.slice(0, 8).map((d) => `${d.name} in ${d.scope.slice(-50) || '<top level>'}`).join(', '),
    );
  }
  return { markers: total, customProps: cps.length };
}

// Every theme-scope block, as { scope, decls: [text, …] }, carrying only the
// declarations this file classifies as design tokens - the same rule the
// annotation pass applies, read the other way round. `make-tokens.mjs` writes
// these out as the DS's token stylesheet so the converter's README scan has a
// real `tokens/` file to read instead of falling back to the whole compiled
// bundle (where flexlayout's `--color-text` outranks the app's own tokens).
export function themeTokenBlocks(css) {
  const blocks = [];
  let cur = '';
  const stack = [];
  let paren = 0;
  let open = null; // block currently collecting, or null
  const flush = () => {
    if (!open) return;
    const name = declName(cur);
    if (name && !isDemoted(name)) open.decls.push(cur.replace(/\/\*[\s\S]*?\*\//g, '').trim());
  };

  for (let i = 0; i < css.length; i++) {
    const c = css[i];
    if (c === '\\' && i + 1 < css.length) { cur += css.slice(i, i + 2); i++; continue; }
    if (c === '/' && css[i + 1] === '*') {
      const end = css.indexOf('*/', i + 2);
      const stop = end === -1 ? css.length : end + 2;
      cur += css.slice(i, stop);
      i = stop - 1;
      continue;
    }
    if (c === '"' || c === "'") {
      let j = i + 1;
      while (j < css.length && css[j] !== c) j += css[j] === '\\' ? 2 : 1;
      cur += css.slice(i, j + 1);
      i = j;
      continue;
    }
    if (c === '(') paren++;
    else if (c === ')') paren = Math.max(0, paren - 1);
    if (paren > 0) { cur += c; continue; }

    if (c === '{') {
      const prelude = cur.trim();
      stack.push(prelude);
      cur = '';
      // Only the outermost theme block collects: a nested block inside one
      // (there are none today) would carry its own conditions.
      if (!open && isThemeScope(prelude)) open = { scope: prelude, decls: [], depth: stack.length };
      continue;
    }
    if (c === '}') {
      flush();
      cur = '';
      if (open && open.depth === stack.length) {
        if (open.decls.length) blocks.push({ scope: open.scope, decls: open.decls });
        open = null;
      }
      stack.pop();
      continue;
    }
    if (c === ';') { flush(); cur = ''; continue; }
    cur += c;
  }
  return blocks;
}

// Annotate a stylesheet in place. Safe to call on a file that has already
// been annotated, and on one with no custom properties at all.
export function annotateFile(file) {
  const src = readFileSync(file, 'utf8');
  if (!/--[\w-]+\s*:/.test(src)) return null;
  THEME_NAMES.clear();
  collectThemeNames(src);
  const { css, counts } = annotate(src);
  const audited = audit(css); // throws rather than write a stylesheet with loose markers
  writeFileSync(file, css);
  console.error(
    `  @kind other: ${counts.decls} declaration(s) (${counts.demoted} demoted from a theme scope as non-tokens) + ` +
      `${counts.atProperty} @property rule(s) marked; ` +
      `${counts.skipped} theme-scope token(s) left as tokens (${THEME_NAMES.size} distinct); ` +
      `${audited.markers} markers, all attached`,
  );
  return counts;
}

// CLI: annotate-tokens.mjs <styles.css>
if (process.argv[1] && process.argv[1].endsWith('annotate-tokens.mjs')) {
  const file = process.argv[2];
  if (!file) {
    console.error('usage: annotate-tokens.mjs <styles.css>');
    process.exit(1);
  }
  annotateFile(file);
}
